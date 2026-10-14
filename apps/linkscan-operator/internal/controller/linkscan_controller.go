/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

const conditionReady = "Ready"

// scheduleGrace is extra time allowed on top of the scan timeout for the pod
// to be scheduled and pull its image; activeDeadlineSeconds only starts
// counting once the kubelet has accepted the pod.
const scheduleGrace = 60 * time.Second

// LinkScanReconciler reconciles a LinkScan object.
type LinkScanReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Sandbox  SandboxConfig
	Recorder record.EventRecorder
	// Results, when set, receives every finished scan exactly once before
	// the TTL may delete it.
	Results ResultSink
}

// ResultSink publishes finished scans (implemented by natsbridge.ResultPublisher).
type ResultSink interface {
	PublishResult(ctx context.Context, scan *scanv1alpha1.LinkScan) error
}

// event records a Kubernetes Event on the scan when a recorder is set.
func (r *LinkScanReconciler) event(scan *scanv1alpha1.LinkScan, eventType, reason, msg string) {
	if r.Recorder != nil {
		r.Recorder.Event(scan, eventType, reason, msg)
	}
}

// +kubebuilder:rbac:groups=scan.satark.io,resources=linkscans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scan.satark.io,resources=linkscans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scan.satark.io,resources=linkscans/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile drives a LinkScan through its lifecycle: create the sandbox pod,
// mirror its progress and record the result.
func (r *LinkScanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	scan := &scanv1alpha1.LinkScan{}
	if err := r.Get(ctx, req.NamespacedName, scan); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !scan.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	if scan.Status.Phase.IsFinished() {
		return r.handleFinished(ctx, scan)
	}

	pod := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Namespace: scan.Namespace, Name: podName(scan)}, pod)
	if apierrors.IsNotFound(err) {
		if scan.Status.PodName != "" {
			// The pod existed and vanished before finishing (evicted or deleted).
			return ctrl.Result{}, r.finish(ctx, scan, scanv1alpha1.PhaseFailed, "PodLost", "sandbox pod disappeared before completing", nil)
		}
		pod = buildSandboxPod(scan, r.Sandbox)
		if err := controllerutil.SetControllerReference(scan, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
			podCreateErrors.Inc()
			r.event(scan, corev1.EventTypeWarning, "PodCreateFailed", fmt.Sprintf("could not create sandbox pod: %v", err))
			return ctrl.Result{}, fmt.Errorf("create sandbox pod: %w", err)
		}
		log.Info("created sandbox pod", "pod", pod.Name)
		podsCreated.Inc()
		r.event(scan, corev1.EventTypeNormal, "PodCreated", "created sandbox pod "+pod.Name)
		scan.Status.Phase = scanv1alpha1.PhasePending
		scan.Status.PodName = pod.Name
		scan.Status.Message = "sandbox pod created"
		setCondition(scan, metav1.ConditionFalse, "PodCreated", "sandbox pod created")
		return ctrl.Result{}, r.Status().Update(ctx, scan)
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.syncFromPod(ctx, scan, pod); err != nil {
		return ctrl.Result{}, err
	}
	if scan.Status.Phase.IsFinished() {
		return ctrl.Result{}, nil
	}

	// Hard upper bound: catches pods stuck Pending (unschedulable, image
	// pull failures) that activeDeadlineSeconds never sees.
	deadline := scan.CreationTimestamp.Add(time.Duration(timeoutSeconds(scan))*time.Second + scheduleGrace)
	if remaining := time.Until(deadline); remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}
	if err := r.Delete(ctx, pod, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.finish(ctx, scan, scanv1alpha1.PhaseTimedOut, "DeadlineExceeded",
		fmt.Sprintf("sandbox did not finish within %ds", timeoutSeconds(scan)), nil)
}

// handleFinished garbage-collects a finished LinkScan once its TTL expires.
// Deleting the LinkScan also removes its sandbox pod via the owner reference.
func (r *LinkScanReconciler) handleFinished(ctx context.Context, scan *scanv1alpha1.LinkScan) (ctrl.Result, error) {
	if r.Results != nil && !scan.Status.ResultPublished {
		if err := r.Results.PublishResult(ctx, scan); err != nil {
			resultsPublished.WithLabelValues("error").Inc()
			r.event(scan, corev1.EventTypeWarning, "ResultPublishFailed", err.Error())
			return ctrl.Result{}, err
		}
		resultsPublished.WithLabelValues("ok").Inc()
		scan.Status.ResultPublished = true
		if err := r.Status().Update(ctx, scan); err != nil {
			// The publish is de-duplicated by message id, so a retry is safe.
			return ctrl.Result{}, err
		}
	}
	expiry, ok := ttlExpiry(scan)
	if !ok {
		return ctrl.Result{}, nil
	}
	if remaining := time.Until(expiry); remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}
	logf.FromContext(ctx).Info("deleting expired LinkScan", "completed", scan.Status.CompletionTime)
	if err := r.Delete(ctx, scan, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// ttlExpiry returns when a finished scan should be deleted. It returns false
// when the scan has no TTL or has not finished.
func ttlExpiry(scan *scanv1alpha1.LinkScan) (time.Time, bool) {
	if scan.Spec.TTLSecondsAfterFinished == nil || scan.Status.CompletionTime == nil {
		return time.Time{}, false
	}
	return scan.Status.CompletionTime.Add(time.Duration(*scan.Spec.TTLSecondsAfterFinished) * time.Second), true
}

// syncFromPod maps the sandbox pod state onto the LinkScan status.
func (r *LinkScanReconciler) syncFromPod(ctx context.Context, scan *scanv1alpha1.LinkScan, pod *corev1.Pod) error {
	switch pod.Status.Phase {
	case corev1.PodRunning:
		if scan.Status.Phase == scanv1alpha1.PhaseRunning {
			return nil
		}
		scan.Status.Phase = scanv1alpha1.PhaseRunning
		scan.Status.Message = "sandbox browser running"
		if scan.Status.StartTime == nil {
			now := metav1.Now()
			if pod.Status.StartTime != nil {
				now = *pod.Status.StartTime
			}
			scan.Status.StartTime = &now
		}
		setCondition(scan, metav1.ConditionFalse, "Running", "sandbox browser running")
		return r.Status().Update(ctx, scan)
	case corev1.PodSucceeded:
		result, err := parseSandboxOutput(terminationMessage(pod))
		if err != nil {
			return r.finish(ctx, scan, scanv1alpha1.PhaseFailed, "InvalidResult", err.Error(), nil)
		}
		return r.finish(ctx, scan, scanv1alpha1.PhaseSucceeded, "Completed", "scan completed", result)
	case corev1.PodFailed:
		if pod.Status.Reason == "DeadlineExceeded" {
			return r.finish(ctx, scan, scanv1alpha1.PhaseTimedOut, "DeadlineExceeded",
				fmt.Sprintf("sandbox exceeded activeDeadlineSeconds=%d", timeoutSeconds(scan)), nil)
		}
		msg := "sandbox pod failed"
		if tm := terminationMessage(pod); tm != "" {
			if _, perr := parseSandboxOutput(tm); perr != nil {
				msg = perr.Error()
			}
		} else if pod.Status.Message != "" {
			msg = pod.Status.Message
		}
		return r.finish(ctx, scan, scanv1alpha1.PhaseFailed, "PodFailed", msg, nil)
	}
	return nil
}

// finish moves a scan to a terminal phase.
func (r *LinkScanReconciler) finish(ctx context.Context, scan *scanv1alpha1.LinkScan, phase scanv1alpha1.LinkScanPhase, reason, msg string, result *scanv1alpha1.ScanResult) error {
	now := metav1.Now()
	scan.Status.Phase = phase
	scan.Status.Message = truncate(msg, 1024)
	scan.Status.CompletionTime = &now
	scan.Status.Result = result
	status := metav1.ConditionFalse
	if phase == scanv1alpha1.PhaseSucceeded {
		status = metav1.ConditionTrue
	}
	setCondition(scan, status, reason, scan.Status.Message)
	if phase == scanv1alpha1.PhaseSucceeded {
		r.event(scan, corev1.EventTypeNormal, reason, scan.Status.Message)
	} else {
		r.event(scan, corev1.EventTypeWarning, reason, fmt.Sprintf("scan %s: %s", phase, scan.Status.Message))
	}
	if err := r.Status().Update(ctx, scan); err != nil {
		return err
	}
	observeFinished(scan, reason)
	return nil
}

func setCondition(scan *scanv1alpha1.LinkScan, status metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&scan.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             status,
		Reason:             reason,
		Message:            truncate(msg, 1024),
		ObservedGeneration: scan.Generation,
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// SetupWithManager sets up the controller with the Manager.
func (r *LinkScanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scanv1alpha1.LinkScan{}).
		Owns(&corev1.Pod{}).
		Named("linkscan").
		Complete(r)
}
