/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

// sandboxOutput is the compact JSON document sandbox-browser writes to its
// termination message (kept under the 4 KiB kubelet limit).
type sandboxOutput struct {
	FinalURL      string   `json:"final_url"`
	RedirectChain []string `json:"redirect_chain"`
	Title         string   `json:"title"`
	Forms         struct {
		Password   bool `json:"password"`
		OTP        bool `json:"otp"`
		CardNumber bool `json:"card_number"`
	} `json:"forms"`
	Downloads     []string `json:"downloads"`
	ScreenshotKey string   `json:"screenshot_key"`
	Error         string   `json:"error"`
}

// parseSandboxOutput decodes a termination message into a ScanResult.
// A non-empty "error" field is returned as an error.
func parseSandboxOutput(msg string) (*scanv1alpha1.ScanResult, error) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil, fmt.Errorf("sandbox produced no result")
	}
	var out sandboxOutput
	if err := json.Unmarshal([]byte(msg), &out); err != nil {
		return nil, fmt.Errorf("decode sandbox result: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("sandbox error: %s", out.Error)
	}
	return &scanv1alpha1.ScanResult{
		FinalURL:      out.FinalURL,
		RedirectChain: out.RedirectChain,
		Title:         out.Title,
		Forms: scanv1alpha1.FormFindings{
			Password:   out.Forms.Password,
			OTP:        out.Forms.OTP,
			CardNumber: out.Forms.CardNumber,
		},
		Downloads:     out.Downloads,
		ScreenshotKey: out.ScreenshotKey,
	}, nil
}

// terminationMessage returns the sandbox container's termination message.
func terminationMessage(pod *corev1.Pod) string {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == sandboxContainer && cs.State.Terminated != nil {
			return cs.State.Terminated.Message
		}
	}
	return ""
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
