package doctor

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func newEnv(objs ...runtime.Object) *Env {
	return &Env{
		Client:       fake.NewSimpleClientset(objs...),
		Now:          func() time.Time { return now },
		PendingGrace: 5 * time.Minute,
		RestartWarn:  5,
	}
}

func node(name string, ready corev1.ConditionStatus, extra ...corev1.NodeCondition) *corev1.Node {
	conds := append([]corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready, Reason: "KubeletNotReady"}}, extra...)
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: conds}}
}

func pod(ns, name string, phase corev1.PodPhase, age time.Duration, cs ...corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, CreationTimestamp: metav1.NewTime(now.Add(-age))},
		Status:     corev1.PodStatus{Phase: phase, ContainerStatuses: cs},
	}
}

func byCheck(rs []Result) map[string]Result {
	m := map[string]Result{}
	for _, r := range rs {
		m[r.Check] = r
	}
	return m
}

func TestNodes(t *testing.T) {
	env := newEnv(
		node("cp1", corev1.ConditionTrue),
		node("w1", corev1.ConditionFalse),
		node("w2", corev1.ConditionTrue, corev1.NodeCondition{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue, Message: "ephemeral-storage low"}),
	)
	r := byCheck(Run(context.Background(), env, DefaultChecks()))["nodes"]
	if r.Status != Fail {
		t.Fatalf("want fail, got %s", r.Status)
	}
	if r.Summary != "2/3 nodes Ready" {
		t.Errorf("summary = %q", r.Summary)
	}
	if len(r.Findings) != 2 || r.Findings[0].Severity != Fail {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestPods(t *testing.T) {
	crash := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, RestartCount: 9}
	pull := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "not found"}}}
	oom := corev1.ContainerStatus{Name: "app", RestartCount: 2, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}}}
	env := newEnv(
		pod("satark", "ok", corev1.PodRunning, time.Hour, corev1.ContainerStatus{Name: "app", Ready: true}),
		pod("satark", "crash", corev1.PodRunning, time.Hour, crash),
		pod("satark", "pull", corev1.PodPending, time.Minute, pull),
		pod("satark", "oom", corev1.PodRunning, time.Hour, oom),
		pod("satark", "new-pending", corev1.PodPending, time.Minute),
		pod("satark", "old-pending", corev1.PodPending, time.Hour),
		pod("satark", "done", corev1.PodSucceeded, time.Hour),
	)
	r := byCheck(Run(context.Background(), env, DefaultChecks()))["pods"]
	if r.Status != Fail {
		t.Fatalf("want fail, got %s", r.Status)
	}
	if r.Summary != "3/7 pods healthy" {
		t.Errorf("summary = %q", r.Summary)
	}
	got := map[string]bool{}
	for _, f := range r.Findings {
		got[f.Object] = true
	}
	for _, want := range []string{"pod/crash", "pod/pull", "pod/oom", "pod/old-pending"} {
		if !got[want] {
			t.Errorf("missing finding for %s: %+v", want, r.Findings)
		}
	}
	if got["pod/new-pending"] {
		t.Error("pod within pending grace should not be reported")
	}
}

func TestNamespaceScope(t *testing.T) {
	crash := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}
	env := newEnv(pod("other", "crash", corev1.PodRunning, time.Hour, crash))
	env.Namespaces = []string{"satark"}
	r := byCheck(Run(context.Background(), env, DefaultChecks()))["pods"]
	if r.Status != OK {
		t.Fatalf("out-of-scope pod reported: %+v", r)
	}
}

func TestPVCs(t *testing.T) {
	sc := "local-path"
	env := newEnv(
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "satark-data", Name: "bound"}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "satark-data", Name: "stuck", CreationTimestamp: metav1.NewTime(now.Add(-time.Hour))}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: &sc}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "satark-data", Name: "lost"}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimLost}},
	)
	r := byCheck(Run(context.Background(), env, DefaultChecks()))["pvcs"]
	if r.Status != Fail || r.Summary != "1/3 PVCs Bound" || len(r.Findings) != 2 {
		t.Fatalf("unexpected: %+v", r)
	}
	if !strings.Contains(r.Findings[0].Message+r.Findings[1].Message, "local-path") {
		t.Errorf("storage class not mentioned: %+v", r.Findings)
	}
}

func TestDeployments(t *testing.T) {
	three := int32(3)
	env := newEnv(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "satark", Name: "full"}, Spec: appsv1.DeploymentSpec{Replicas: &three}, Status: appsv1.DeploymentStatus{AvailableReplicas: 3}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "satark", Name: "partial"}, Spec: appsv1.DeploymentSpec{Replicas: &three}, Status: appsv1.DeploymentStatus{AvailableReplicas: 2}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "satark", Name: "down"}, Spec: appsv1.DeploymentSpec{Replicas: &three}, Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Message: "ProgressDeadlineExceeded"}},
		}},
	)
	r := byCheck(Run(context.Background(), env, DefaultChecks()))["deployments"]
	if r.Status != Fail || r.Summary != "1/3 deployments fully available" {
		t.Fatalf("unexpected: %+v", r)
	}
	if len(r.Findings) != 3 {
		t.Errorf("want 3 findings, got %+v", r.Findings)
	}
}

func TestHealthyClusterAndRender(t *testing.T) {
	env := newEnv(node("cp1", corev1.ConditionTrue))
	rs := Run(context.Background(), env, DefaultChecks())
	if Worst(rs) != OK {
		t.Fatalf("want ok, got %s", Worst(rs))
	}
	var buf bytes.Buffer
	RenderText(&buf, rs, false)
	if !strings.Contains(buf.String(), "Overall: OK") || !strings.Contains(buf.String(), "1/1 nodes Ready") {
		t.Errorf("render:\n%s", buf.String())
	}
}
