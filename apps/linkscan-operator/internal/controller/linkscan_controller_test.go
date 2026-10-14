/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	nsCounter atomic.Int32
)

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Println("KUBEBUILDER_ASSETS not set; envtest controller tests will be skipped (run `make test`)")
		os.Exit(m.Run())
	}
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := scanv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}
	// The API server rejects pods that name a RuntimeClass that does not exist.
	if err := k8sClient.Create(context.Background(), &nodev1.RuntimeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "gvisor"},
		Handler:    "runsc",
	}); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = testEnv.Stop()
	os.Exit(code)
}

type harness struct {
	t        *testing.T
	ctx      context.Context
	r        *LinkScanReconciler
	recorder *record.FakeRecorder
	ns       string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	if k8sClient == nil {
		t.Skip("envtest not available")
	}
	ctx := context.Background()
	ns := fmt.Sprintf("scan-test-%d", nsCounter.Add(1))
	if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}); err != nil {
		t.Fatal(err)
	}
	rec := record.NewFakeRecorder(32)
	return &harness{
		t: t, ctx: ctx, ns: ns, recorder: rec,
		r: &LinkScanReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Sandbox:  SandboxConfig{Image: "sandbox-browser:test", RuntimeClassName: "gvisor"},
			Recorder: rec,
		},
	}
}

func (h *harness) createScan(name string, mutate func(*scanv1alpha1.LinkScan)) *scanv1alpha1.LinkScan {
	h.t.Helper()
	scan := &scanv1alpha1.LinkScan{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: h.ns},
		Spec:       scanv1alpha1.LinkScanSpec{URL: "https://example.com/login"},
	}
	if mutate != nil {
		mutate(scan)
	}
	if err := k8sClient.Create(h.ctx, scan); err != nil {
		h.t.Fatal(err)
	}
	return scan
}

func (h *harness) reconcile(name string) ctrl.Result {
	h.t.Helper()
	res, err := h.r.Reconcile(h.ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: h.ns, Name: name}})
	if err != nil {
		h.t.Fatalf("reconcile: %v", err)
	}
	return res
}

func (h *harness) getScan(name string) *scanv1alpha1.LinkScan {
	h.t.Helper()
	scan := &scanv1alpha1.LinkScan{}
	if err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.ns, Name: name}, scan); err != nil {
		h.t.Fatal(err)
	}
	return scan
}

func (h *harness) getPod(scan *scanv1alpha1.LinkScan) *corev1.Pod {
	h.t.Helper()
	pod := &corev1.Pod{}
	if err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.ns, Name: podName(scan)}, pod); err != nil {
		h.t.Fatal(err)
	}
	return pod
}

func (h *harness) setPodStatus(pod *corev1.Pod, status corev1.PodStatus) {
	h.t.Helper()
	pod.Status = status
	if err := k8sClient.Status().Update(h.ctx, pod); err != nil {
		h.t.Fatal(err)
	}
}

func terminated(msg string, exit int32) []corev1.ContainerStatus {
	return []corev1.ContainerStatus{{
		Name:  sandboxContainer,
		State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: exit, Message: msg}},
	}}
}

func TestCRDDefaultsApplied(t *testing.T) {
	h := newHarness(t)
	h.createScan("defaults", nil)
	scan := h.getScan("defaults")
	if scan.Spec.TimeoutSeconds != 60 || scan.Spec.TTLSecondsAfterFinished == nil || *scan.Spec.TTLSecondsAfterFinished != 3600 {
		t.Fatalf("defaults not applied: %+v", scan.Spec)
	}
}

func TestCRDRejectsNonHTTPURL(t *testing.T) {
	h := newHarness(t)
	err := k8sClient.Create(h.ctx, &scanv1alpha1.LinkScan{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: h.ns},
		Spec:       scanv1alpha1.LinkScanSpec{URL: "file:///etc/passwd"},
	})
	if !apierrors.IsInvalid(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestCreatesOwnedHardenedPod(t *testing.T) {
	h := newHarness(t)
	h.createScan("create", nil)
	h.reconcile("create")

	scan := h.getScan("create")
	if scan.Status.Phase != scanv1alpha1.PhasePending || scan.Status.PodName == "" {
		t.Fatalf("unexpected status %+v", scan.Status)
	}
	pod := h.getPod(scan)
	owner := metav1.GetControllerOf(pod)
	if owner == nil || owner.UID != scan.UID || owner.Kind != "LinkScan" {
		t.Fatalf("pod not owned by scan: %+v", owner)
	}
	if pod.Spec.ActiveDeadlineSeconds == nil || *pod.Spec.ActiveDeadlineSeconds != 60 {
		t.Fatalf("activeDeadlineSeconds not set")
	}
	if ptr.Deref(pod.Spec.RuntimeClassName, "") != "gvisor" {
		t.Fatalf("runtime class missing")
	}

	// A second reconcile is idempotent.
	h.reconcile("create")
}

func TestSuccessfulScanRecordsResult(t *testing.T) {
	h := newHarness(t)
	h.createScan("ok", nil)
	h.reconcile("ok")
	scan := h.getScan("ok")
	pod := h.getPod(scan)

	h.setPodStatus(pod, corev1.PodStatus{Phase: corev1.PodRunning})
	h.reconcile("ok")
	if got := h.getScan("ok").Status.Phase; got != scanv1alpha1.PhaseRunning {
		t.Fatalf("phase=%s, want Running", got)
	}

	pod = h.getPod(scan)
	h.setPodStatus(pod, corev1.PodStatus{
		Phase:             corev1.PodSucceeded,
		ContainerStatuses: terminated(`{"final_url":"https://phish.example/otp","redirect_chain":["https://example.com/login","https://phish.example/otp"],"forms":{"otp":true,"card_number":true},"downloads":["app-release.apk"],"screenshot_key":"screenshots/ok.png"}`, 0),
	})
	h.reconcile("ok")

	scan = h.getScan("ok")
	if scan.Status.Phase != scanv1alpha1.PhaseSucceeded || scan.Status.CompletionTime == nil {
		t.Fatalf("unexpected status %+v", scan.Status)
	}
	r := scan.Status.Result
	if r == nil || r.FinalURL != "https://phish.example/otp" || !r.Forms.OTP || !r.Forms.CardNumber || len(r.Downloads) != 1 {
		t.Fatalf("unexpected result %+v", r)
	}
	expectEvent(t, h.recorder, "Normal Completed")
}

func TestFailedPodMarksScanFailed(t *testing.T) {
	h := newHarness(t)
	h.createScan("fail", nil)
	h.reconcile("fail")
	pod := h.getPod(h.getScan("fail"))
	h.setPodStatus(pod, corev1.PodStatus{
		Phase:             corev1.PodFailed,
		ContainerStatuses: terminated(`{"error":"net::ERR_NAME_NOT_RESOLVED"}`, 1),
	})
	h.reconcile("fail")
	scan := h.getScan("fail")
	if scan.Status.Phase != scanv1alpha1.PhaseFailed {
		t.Fatalf("phase=%s, want Failed", scan.Status.Phase)
	}
	expectEvent(t, h.recorder, "Warning PodFailed")
}

func TestDeadlineExceededMarksTimedOut(t *testing.T) {
	h := newHarness(t)
	h.createScan("slow", nil)
	h.reconcile("slow")
	pod := h.getPod(h.getScan("slow"))
	h.setPodStatus(pod, corev1.PodStatus{Phase: corev1.PodFailed, Reason: "DeadlineExceeded"})
	h.reconcile("slow")
	if got := h.getScan("slow").Status.Phase; got != scanv1alpha1.PhaseTimedOut {
		t.Fatalf("phase=%s, want TimedOut", got)
	}
	expectEvent(t, h.recorder, "Warning DeadlineExceeded")
}

func TestRunningScanRequeuesUntilDeadline(t *testing.T) {
	h := newHarness(t)
	h.createScan("wait", nil)
	h.reconcile("wait")
	pod := h.getPod(h.getScan("wait"))
	h.setPodStatus(pod, corev1.PodStatus{Phase: corev1.PodRunning})
	if res := h.reconcile("wait"); res.RequeueAfter <= 0 {
		t.Fatalf("expected requeue for deadline check, got %+v", res)
	}
}

func TestTTLDeletesFinishedScan(t *testing.T) {
	h := newHarness(t)
	h.createScan("ttl", func(s *scanv1alpha1.LinkScan) { s.Spec.TTLSecondsAfterFinished = ptr.To(int64(0)) })
	h.reconcile("ttl")
	pod := h.getPod(h.getScan("ttl"))
	h.setPodStatus(pod, corev1.PodStatus{Phase: corev1.PodFailed, Reason: "DeadlineExceeded"})
	h.reconcile("ttl") // -> TimedOut
	h.reconcile("ttl") // -> TTL expired, delete

	err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.ns, Name: "ttl"}, &scanv1alpha1.LinkScan{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected scan deleted, got %v", err)
	}
}

func TestTTLRequeuesBeforeExpiry(t *testing.T) {
	h := newHarness(t)
	h.createScan("keep", nil)
	h.reconcile("keep")
	pod := h.getPod(h.getScan("keep"))
	h.setPodStatus(pod, corev1.PodStatus{Phase: corev1.PodFailed})
	h.reconcile("keep")
	if res := h.reconcile("keep"); res.RequeueAfter <= 0 {
		t.Fatalf("expected TTL requeue, got %+v", res)
	}
}

func expectEvent(t *testing.T, rec *record.FakeRecorder, prefix string) {
	t.Helper()
	for {
		select {
		case e := <-rec.Events:
			if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
				return
			}
		default:
			t.Fatalf("no event with prefix %q", prefix)
		}
	}
}

type fakeSink struct {
	got  []string
	fail bool
}

func (f *fakeSink) PublishResult(_ context.Context, scan *scanv1alpha1.LinkScan) error {
	if f.fail {
		return fmt.Errorf("nats unavailable")
	}
	f.got = append(f.got, scan.Name+"="+string(scan.Status.Phase))
	return nil
}

func TestResultPublishedOnceBeforeTTL(t *testing.T) {
	h := newHarness(t)
	sink := &fakeSink{fail: true}
	h.r.Results = sink
	h.createScan("pub", func(s *scanv1alpha1.LinkScan) { s.Spec.TTLSecondsAfterFinished = ptr.To(int64(0)) })
	h.reconcile("pub")
	pod := h.getPod(h.getScan("pub"))
	h.setPodStatus(pod, corev1.PodStatus{Phase: corev1.PodFailed, Reason: "DeadlineExceeded"})
	h.reconcile("pub") // -> TimedOut

	// Publishing fails: the scan must survive its TTL until the result is out.
	if _, err := h.r.Reconcile(h.ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: h.ns, Name: "pub"}}); err == nil {
		t.Fatal("expected publish error")
	}
	if h.getScan("pub").Status.ResultPublished {
		t.Fatal("must not be marked published")
	}

	sink.fail = false
	h.reconcile("pub")
	if len(sink.got) != 1 || sink.got[0] != "pub=TimedOut" {
		t.Fatalf("published %v", sink.got)
	}
	err := k8sClient.Get(h.ctx, types.NamespacedName{Namespace: h.ns, Name: "pub"}, &scanv1alpha1.LinkScan{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected deletion after publish, got %v", err)
	}
}
