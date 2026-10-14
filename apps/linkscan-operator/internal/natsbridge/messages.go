/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

// Package natsbridge connects linkscan-operator to NATS JetStream: it turns
// scan jobs (scan.jobs.<id>) into LinkScan objects and publishes finished
// scans to scan.results.<id>.
package natsbridge

import (
	"time"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

const (
	JobsStream     = "SCAN_JOBS"
	JobsSubject    = "scan.jobs."
	ResultsStream  = "SCAN_RESULTS"
	ResultsSubject = "scan.results."
	// ConsumerName is the durable pull consumer on SCAN_JOBS.
	ConsumerName = "linkscan-operator"
	// LabelIntake records which operator replica accepted a job.
	LabelIntake = "scan.satark.io/intake"
)

// Job is published by url-intel (see apps/url-intel/internal/jobs).
type Job struct {
	ID             string            `json:"id"`
	URL            string            `json:"url"`
	RequestedAt    time.Time         `json:"requested_at"`
	TimeoutSeconds int64             `json:"timeout_seconds,omitempty"`
	Correlation    map[string]string `json:"correlation,omitempty"`
}

// Forms mirrors the sandbox form findings.
type Forms struct {
	Password   bool `json:"password"`
	OTP        bool `json:"otp"`
	CardNumber bool `json:"card_number"`
}

// Result is published when a LinkScan reaches a terminal phase. Consumers
// such as telegram-bot use Correlation to find the message to update.
type Result struct {
	ID            string            `json:"id"`
	URL           string            `json:"url"`
	Phase         string            `json:"phase"`
	Message       string            `json:"message,omitempty"`
	FinalURL      string            `json:"final_url,omitempty"`
	RedirectChain []string          `json:"redirect_chain,omitempty"`
	Title         string            `json:"title,omitempty"`
	Forms         Forms             `json:"forms"`
	Downloads     []string          `json:"downloads,omitempty"`
	ScreenshotKey string            `json:"screenshot_key,omitempty"`
	Correlation   map[string]string `json:"correlation,omitempty"`
	FinishedAt    time.Time         `json:"finished_at"`
}

// ResultFromScan builds the result message for a finished LinkScan.
func ResultFromScan(scan *scanv1alpha1.LinkScan) Result {
	r := Result{
		ID:          scan.Name,
		URL:         scan.Spec.URL,
		Phase:       string(scan.Status.Phase),
		Message:     scan.Status.Message,
		Correlation: scan.Spec.Correlation,
	}
	if scan.Status.CompletionTime != nil {
		r.FinishedAt = scan.Status.CompletionTime.UTC()
	}
	if res := scan.Status.Result; res != nil {
		r.FinalURL = res.FinalURL
		r.RedirectChain = res.RedirectChain
		r.Title = res.Title
		r.Forms = Forms{Password: res.Forms.Password, OTP: res.Forms.OTP, CardNumber: res.Forms.CardNumber}
		r.Downloads = res.Downloads
		r.ScreenshotKey = res.ScreenshotKey
	}
	return r
}
