/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package natsbridge

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var jobsReceived = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "satark_linkscan_jobs_received_total",
	Help: "Scan jobs pulled from NATS, by outcome (created, duplicate, invalid, malformed, error).",
}, []string{"outcome"})

func init() {
	metrics.Registry.MustRegister(jobsReceived)
}
