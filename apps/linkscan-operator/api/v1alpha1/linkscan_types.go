/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LinkScanPhase is the lifecycle phase of a LinkScan.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;TimedOut
type LinkScanPhase string

const (
	PhasePending   LinkScanPhase = "Pending"
	PhaseRunning   LinkScanPhase = "Running"
	PhaseSucceeded LinkScanPhase = "Succeeded"
	PhaseFailed    LinkScanPhase = "Failed"
	PhaseTimedOut  LinkScanPhase = "TimedOut"
)

// IsFinished reports whether the phase is terminal.
func (p LinkScanPhase) IsFinished() bool {
	return p == PhaseSucceeded || p == PhaseFailed || p == PhaseTimedOut
}

// LinkScanSpec defines the link to scan and how the sandbox runs.
type LinkScanSpec struct {
	// URL is the link to open in the sandbox browser.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="url is immutable"
	URL string `json:"url"`

	// TimeoutSeconds bounds how long the sandbox pod may run.
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=300
	// +optional
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`

	// TTLSecondsAfterFinished is how long a finished LinkScan is kept before
	// the operator deletes it (and, through owner references, its pod).
	// +kubebuilder:default=3600
	// +kubebuilder:validation:Minimum=0
	// +optional
	TTLSecondsAfterFinished *int64 `json:"ttlSecondsAfterFinished,omitempty"`

	// Image overrides the sandbox-browser image configured on the operator.
	// +optional
	Image string `json:"image,omitempty"`

	// Correlation carries opaque caller context (for example the Telegram
	// chat and message to edit) that is echoed back on the result message.
	// +optional
	Correlation map[string]string `json:"correlation,omitempty"`
}

// FormFindings lists sensitive input fields seen on the final page.
type FormFindings struct {
	Password   bool `json:"password,omitempty"`
	OTP        bool `json:"otp,omitempty"`
	CardNumber bool `json:"cardNumber,omitempty"`
}

// ScanResult is what the sandbox browser observed.
type ScanResult struct {
	// FinalURL is the URL after all redirects.
	// +optional
	FinalURL string `json:"finalURL,omitempty"`
	// RedirectChain lists each hop, starting with spec.url.
	// +optional
	RedirectChain []string `json:"redirectChain,omitempty"`
	// Title of the final page.
	// +optional
	Title string `json:"title,omitempty"`
	// Forms holds the sensitive form field findings.
	// +optional
	Forms FormFindings `json:"forms,omitempty"`
	// Downloads lists files the page tried to download (for example APKs).
	// +optional
	Downloads []string `json:"downloads,omitempty"`
	// ScreenshotKey is the S3 object key of the full-page screenshot.
	// +optional
	ScreenshotKey string `json:"screenshotKey,omitempty"`
}

// LinkScanStatus defines the observed state of LinkScan.
type LinkScanStatus struct {
	// +optional
	Phase LinkScanPhase `json:"phase,omitempty"`
	// PodName is the sandbox pod running this scan.
	// +optional
	PodName string `json:"podName,omitempty"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// Message is a human-readable reason for the current phase.
	// +optional
	Message string `json:"message,omitempty"`
	// Result is filled in when the scan succeeds.
	// +optional
	Result *ScanResult `json:"result,omitempty"`
	// ResultPublished is true once the result was published to NATS.
	// +optional
	ResultPublished bool `json:"resultPublished,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ls,categories=satark
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Final URL",type=string,JSONPath=`.status.result.finalURL`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// LinkScan is a request to open one URL in an isolated sandbox browser.
type LinkScan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LinkScanSpec   `json:"spec,omitempty"`
	Status LinkScanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LinkScanList contains a list of LinkScan.
type LinkScanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LinkScan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LinkScan{}, &LinkScanList{})
}
