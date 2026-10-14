/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package controller

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

func testScan() *scanv1alpha1.LinkScan {
	return &scanv1alpha1.LinkScan{
		ObjectMeta: metav1.ObjectMeta{Name: "scan-1", Namespace: "satark-sandbox"},
		Spec:       scanv1alpha1.LinkScanSpec{URL: "https://example.com/", TimeoutSeconds: 60},
	}
}

func TestBuildSandboxPodIsHardened(t *testing.T) {
	pod := buildSandboxPod(testScan(), SandboxConfig{Image: "img:1", RuntimeClassName: "gvisor"})
	s := pod.Spec
	if s.RuntimeClassName == nil || *s.RuntimeClassName != "gvisor" {
		t.Fatalf("runtime class not set")
	}
	if *s.AutomountServiceAccountToken || *s.EnableServiceLinks {
		t.Fatalf("service account token or service links enabled")
	}
	if s.RestartPolicy != corev1.RestartPolicyNever || s.DNSPolicy != corev1.DNSNone {
		t.Fatalf("unexpected restart/dns policy")
	}
	c := s.Containers[0]
	sc := c.SecurityContext
	if *sc.AllowPrivilegeEscalation || !*sc.ReadOnlyRootFilesystem || len(sc.Capabilities.Drop) != 1 {
		t.Fatalf("container security context too permissive: %+v", sc)
	}
	if c.Resources.Limits.Memory().IsZero() || c.Resources.Limits.Cpu().IsZero() {
		t.Fatalf("missing limits")
	}
	if pod.Labels[LabelRole] != RoleSandbox || pod.Labels[LabelScan] != "scan-1" {
		t.Fatalf("missing labels: %v", pod.Labels)
	}
}

func TestPodNameTruncated(t *testing.T) {
	scan := testScan()
	scan.Name = strings.Repeat("a", 80)
	if n := podName(scan); len(n) > 63 || !strings.HasSuffix(n, "-sandbox") {
		t.Fatalf("bad pod name %q", n)
	}
}

func TestParseSandboxOutput(t *testing.T) {
	r, err := parseSandboxOutput(`{"final_url":"https://b.example/","redirect_chain":["https://a.example/","https://b.example/"],"forms":{"otp":true}}`)
	if err != nil || r.FinalURL != "https://b.example/" || !r.Forms.OTP || len(r.RedirectChain) != 2 {
		t.Fatalf("unexpected result %+v %v", r, err)
	}
	if _, err := parseSandboxOutput(`{"error":"navigation failed"}`); err == nil {
		t.Fatal("expected error")
	}
	if _, err := parseSandboxOutput(""); err == nil {
		t.Fatal("expected error for empty output")
	}
}

func TestActiveDeadlineFollowsTimeout(t *testing.T) {
	scan := testScan()
	scan.Spec.TimeoutSeconds = 42
	if d := *buildSandboxPod(scan, SandboxConfig{}).Spec.ActiveDeadlineSeconds; d != 42 {
		t.Fatalf("activeDeadlineSeconds=%d", d)
	}
	scan.Spec.TimeoutSeconds = 0
	if d := *buildSandboxPod(scan, SandboxConfig{}).Spec.ActiveDeadlineSeconds; d != defaultTimeoutSeconds {
		t.Fatalf("default activeDeadlineSeconds=%d", d)
	}
}

func TestTTLExpiry(t *testing.T) {
	scan := testScan()
	if _, ok := ttlExpiry(scan); ok {
		t.Fatal("unfinished scan without TTL must not expire")
	}
	ttl := int64(30)
	done := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	scan.Spec.TTLSecondsAfterFinished = &ttl
	scan.Status.CompletionTime = &done
	exp, ok := ttlExpiry(scan)
	if !ok || !exp.Equal(done.Add(30*time.Second)) {
		t.Fatalf("expiry=%v ok=%v", exp, ok)
	}
}
