/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

var (
	scansFinished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "satark_linkscan_scans_total",
		Help: "LinkScans that reached a terminal phase, by phase and reason.",
	}, []string{"phase", "reason"})

	scanDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "satark_linkscan_scan_duration_seconds",
		Help:    "Time from LinkScan creation to a terminal phase.",
		Buckets: []float64{2, 5, 10, 15, 20, 30, 45, 60, 90, 120, 300},
	}, []string{"phase"})

	podsCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satark_linkscan_sandbox_pods_created_total",
		Help: "Sandbox pods created by the operator.",
	})

	podCreateErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satark_linkscan_sandbox_pod_create_errors_total",
		Help: "Failed attempts to create a sandbox pod.",
	})

	findings = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "satark_linkscan_findings_total",
		Help: "Sensitive findings in successful scans (password, otp, card_number, download).",
	}, []string{"type"})

	resultsPublished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "satark_linkscan_results_published_total",
		Help: "Scan results published to NATS, by outcome.",
	}, []string{"outcome"})
)

func init() {
	metrics.Registry.MustRegister(scansFinished, scanDuration, podsCreated, podCreateErrors, findings, resultsPublished)
}

// observeFinished records metrics for a scan that just reached a terminal phase.
func observeFinished(scan *scanv1alpha1.LinkScan, reason string) {
	phase := string(scan.Status.Phase)
	scansFinished.WithLabelValues(phase, reason).Inc()
	if scan.Status.CompletionTime != nil {
		scanDuration.WithLabelValues(phase).Observe(
			scan.Status.CompletionTime.Sub(scan.CreationTimestamp.Time).Seconds())
	}
	if res := scan.Status.Result; res != nil {
		if res.Forms.Password {
			findings.WithLabelValues("password").Inc()
		}
		if res.Forms.OTP {
			findings.WithLabelValues("otp").Inc()
		}
		if res.Forms.CardNumber {
			findings.WithLabelValues("card_number").Inc()
		}
		if len(res.Downloads) > 0 {
			findings.WithLabelValues("download").Inc()
		}
	}
}
