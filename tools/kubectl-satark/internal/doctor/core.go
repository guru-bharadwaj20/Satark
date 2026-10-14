package doctor

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var badWaitingReasons = map[string]Severity{
	"CrashLoopBackOff":           Fail,
	"ImagePullBackOff":           Fail,
	"ErrImagePull":               Fail,
	"CreateContainerConfigError": Fail,
	"InvalidImageName":           Fail,
	"CreateContainerError":       Fail,
	"RunContainerError":          Fail,
}

func checkNodes(ctx context.Context, env *Env) ([]Finding, string, error) {
	nodes, err := env.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", err
	}
	var out []Finding
	ready := 0
	for _, n := range nodes.Items {
		name := obj("node", n.Name)
		isReady := false
		for _, c := range n.Status.Conditions {
			switch c.Type {
			case corev1.NodeReady:
				if c.Status == corev1.ConditionTrue {
					isReady = true
				} else {
					out = append(out, Finding{Fail, "", name, fmt.Sprintf("NotReady: %s", firstNonEmpty(c.Message, c.Reason, string(c.Status)))})
				}
			case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
				if c.Status == corev1.ConditionTrue {
					out = append(out, Finding{Warn, "", name, fmt.Sprintf("%s: %s", c.Type, c.Message)})
				}
			case corev1.NodeNetworkUnavailable:
				if c.Status == corev1.ConditionTrue {
					out = append(out, Finding{Fail, "", name, "NetworkUnavailable: " + c.Message})
				}
			}
		}
		if isReady {
			ready++
		}
		if n.Spec.Unschedulable {
			out = append(out, Finding{Warn, "", name, "cordoned (SchedulingDisabled)"})
		}
	}
	return out, fmt.Sprintf("%d/%d nodes Ready", ready, len(nodes.Items)), nil
}

func checkPods(ctx context.Context, env *Env) ([]Finding, string, error) {
	pods, err := env.Client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", err
	}
	var out []Finding
	total, healthy := 0, 0
	for _, p := range pods.Items {
		if !env.inScope(p.Namespace) {
			continue
		}
		total++
		before := len(out)
		name := obj("pod", p.Name)

		switch p.Status.Phase {
		case corev1.PodFailed:
			if p.Status.Reason == "Evicted" {
				out = append(out, Finding{Warn, p.Namespace, name, "evicted: " + p.Status.Message})
			} else {
				out = append(out, Finding{Fail, p.Namespace, name, "Failed: " + firstNonEmpty(p.Status.Message, p.Status.Reason)})
			}
		case corev1.PodPending:
			age := env.now().Sub(p.CreationTimestamp.Time)
			if age > env.PendingGrace {
				msg := "Pending for " + age.Round(1e9).String()
				for _, c := range p.Status.Conditions {
					if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
						msg += ": " + c.Message
					}
				}
				out = append(out, Finding{Fail, p.Namespace, name, msg})
			}
		case corev1.PodSucceeded:
			healthy++
			continue
		}

		statuses := append(append([]corev1.ContainerStatus{}, p.Status.InitContainerStatuses...), p.Status.ContainerStatuses...)
		for _, cs := range statuses {
			if w := cs.State.Waiting; w != nil {
				if sev, bad := badWaitingReasons[w.Reason]; bad {
					out = append(out, Finding{sev, p.Namespace, name, fmt.Sprintf("container %s %s: %s", cs.Name, w.Reason, w.Message)})
				}
			}
			if t := cs.LastTerminationState.Terminated; t != nil && t.Reason == "OOMKilled" {
				out = append(out, Finding{Warn, p.Namespace, name, fmt.Sprintf("container %s was OOMKilled (restarts: %d)", cs.Name, cs.RestartCount)})
			} else if env.RestartWarn > 0 && cs.RestartCount >= env.RestartWarn {
				out = append(out, Finding{Warn, p.Namespace, name, fmt.Sprintf("container %s restarted %d times", cs.Name, cs.RestartCount)})
			}
		}
		if len(out) == before {
			healthy++
		}
	}
	return out, fmt.Sprintf("%d/%d pods healthy", healthy, total), nil
}

func checkPVCs(ctx context.Context, env *Env) ([]Finding, string, error) {
	pvcs, err := env.Client.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", err
	}
	var out []Finding
	total, bound := 0, 0
	for _, c := range pvcs.Items {
		if !env.inScope(c.Namespace) {
			continue
		}
		total++
		name := obj("pvc", c.Name)
		switch c.Status.Phase {
		case corev1.ClaimBound:
			bound++
		case corev1.ClaimLost:
			out = append(out, Finding{Fail, c.Namespace, name, "Lost: bound volume no longer exists"})
		default:
			sev := Warn
			if env.now().Sub(c.CreationTimestamp.Time) > env.PendingGrace {
				sev = Fail
			}
			out = append(out, Finding{sev, c.Namespace, name, fmt.Sprintf("%s (storageClass %s)", c.Status.Phase, strOrDefault(c.Spec.StorageClassName))})
		}
		for _, cond := range c.Status.Conditions {
			if cond.Type == corev1.PersistentVolumeClaimResizing || cond.Type == corev1.PersistentVolumeClaimFileSystemResizePending {
				out = append(out, Finding{Warn, c.Namespace, name, string(cond.Type)})
			}
		}
	}
	return out, fmt.Sprintf("%d/%d PVCs Bound", bound, total), nil
}

func checkDeployments(ctx context.Context, env *Env) ([]Finding, string, error) {
	deps, err := env.Client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", err
	}
	var out []Finding
	total, ok := 0, 0
	for _, d := range deps.Items {
		if !env.inScope(d.Namespace) {
			continue
		}
		total++
		name := obj("deployment", d.Name)
		want := int32(1)
		if d.Spec.Replicas != nil {
			want = *d.Spec.Replicas
		}
		before := len(out)
		if want > 0 && d.Status.AvailableReplicas == 0 {
			out = append(out, Finding{Fail, d.Namespace, name, fmt.Sprintf("0/%d replicas available", want)})
		} else if d.Status.AvailableReplicas < want {
			out = append(out, Finding{Warn, d.Namespace, name, fmt.Sprintf("%d/%d replicas available", d.Status.AvailableReplicas, want)})
		}
		for _, c := range d.Status.Conditions {
			if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse {
				out = append(out, Finding{Fail, d.Namespace, name, "rollout stuck: " + c.Message})
			}
		}
		if len(out) == before {
			ok++
		}
	}
	return out, fmt.Sprintf("%d/%d deployments fully available", ok, total), nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func strOrDefault(p *string) string {
	if p == nil || *p == "" {
		return "<default>"
	}
	return *p
}
