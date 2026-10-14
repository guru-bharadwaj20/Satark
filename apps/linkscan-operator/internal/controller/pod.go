/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

const (
	// LabelScan identifies the LinkScan a sandbox pod belongs to.
	LabelScan = "scan.satark.io/linkscan"
	// LabelRole marks sandbox pods so NetworkPolicies can select them.
	LabelRole = "satark.io/role"
	// RoleSandbox is the value of LabelRole on sandbox pods.
	RoleSandbox = "sandbox"

	sandboxContainer = "browser"
	sandboxUID       = int64(10001)
)

// SandboxConfig holds operator-wide settings for sandbox pods.
type SandboxConfig struct {
	// Image is the default sandbox-browser image.
	Image string
	// RuntimeClassName is the RuntimeClass (gVisor) sandbox pods run under.
	// Empty means the cluster default runtime.
	RuntimeClassName string
	// ScreenshotBucket is the S3 bucket for screenshots; empty disables upload.
	ScreenshotBucket string
	// S3SecretName names a Secret in the scan namespace with AWS credentials
	// scoped to PutObject on the screenshot prefix.
	S3SecretName string
	// DNSServers are public resolvers used instead of cluster DNS so the
	// sandbox cannot resolve in-cluster service names.
	DNSServers []string
}

// podName returns the deterministic sandbox pod name for a scan.
func podName(scan *scanv1alpha1.LinkScan) string {
	name := scan.Name
	const suffix = "-sandbox"
	if len(name) > 63-len(suffix) {
		name = name[:63-len(suffix)]
	}
	return name + suffix
}

// buildSandboxPod renders the hardened pod that runs one scan.
func buildSandboxPod(scan *scanv1alpha1.LinkScan, cfg SandboxConfig) *corev1.Pod {
	image := cfg.Image
	if scan.Spec.Image != "" {
		image = scan.Spec.Image
	}
	dns := cfg.DNSServers
	if len(dns) == 0 {
		dns = []string{"1.1.1.1", "9.9.9.9"}
	}

	env := []corev1.EnvVar{
		{Name: "TARGET_URL", Value: scan.Spec.URL},
		{Name: "SCAN_ID", Value: scan.Name},
		{Name: "TIMEOUT_SECONDS", Value: itoa(scan.Spec.TimeoutSeconds)},
		{Name: "RESULT_PATH", Value: "/dev/termination-log"},
		{Name: "HOME", Value: "/tmp"},
	}
	var envFrom []corev1.EnvFromSource
	if cfg.ScreenshotBucket != "" {
		env = append(env, corev1.EnvVar{Name: "SCREENSHOT_BUCKET", Value: cfg.ScreenshotBucket})
		if cfg.S3SecretName != "" {
			envFrom = append(envFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cfg.S3SecretName},
					Optional:             ptr.To(true),
				},
			})
		}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName(scan),
			Namespace: scan.Namespace,
			Labels: map[string]string{
				LabelScan:                      scan.Name,
				LabelRole:                      RoleSandbox,
				"app.kubernetes.io/name":       "sandbox-browser",
				"app.kubernetes.io/managed-by": "linkscan-operator",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyNever,
			AutomountServiceAccountToken:  ptr.To(false),
			EnableServiceLinks:            ptr.To(false),
			HostNetwork:                   false,
			HostPID:                       false,
			HostIPC:                       false,
			ShareProcessNamespace:         ptr.To(false),
			TerminationGracePeriodSeconds: ptr.To(int64(5)),
			DNSPolicy:                     corev1.DNSNone,
			DNSConfig:                     &corev1.PodDNSConfig{Nameservers: dns},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   ptr.To(true),
				RunAsUser:      ptr.To(sandboxUID),
				RunAsGroup:     ptr.To(sandboxUID),
				FSGroup:        ptr.To(sandboxUID),
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{{
				Name:                     sandboxContainer,
				Image:                    image,
				ImagePullPolicy:          corev1.PullIfNotPresent,
				Env:                      env,
				EnvFrom:                  envFrom,
				TerminationMessagePath:   "/dev/termination-log",
				TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr.To(false),
					Privileged:               ptr.To(false),
					ReadOnlyRootFilesystem:   ptr.To(true),
					RunAsNonRoot:             ptr.To(true),
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("250m"),
						corev1.ResourceMemory:           resource.MustParse("512Mi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse("1"),
						corev1.ResourceMemory:           resource.MustParse("1Gi"),
						corev1.ResourceEphemeralStorage: resource.MustParse("256Mi"),
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "tmp", MountPath: "/tmp"},
					{Name: "dshm", MountPath: "/dev/shm"},
				},
			}},
			Volumes: []corev1.Volume{
				{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: ptr.To(resource.MustParse("200Mi")),
				}}},
				{Name: "dshm", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: ptr.To(resource.MustParse("128Mi")),
				}}},
			},
		},
	}
	if cfg.RuntimeClassName != "" {
		pod.Spec.RuntimeClassName = ptr.To(cfg.RuntimeClassName)
	}
	return pod
}
