/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package natsbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

// ResultPublisher publishes finished scans to SCAN_RESULTS.
type ResultPublisher struct {
	JS      jetstream.JetStream
	Timeout time.Duration
}

// PublishResult sends the scan's result. The message id is the scan name, so
// a retry after a failed status update is de-duplicated by JetStream.
func (p *ResultPublisher) PublishResult(ctx context.Context, scan *scanv1alpha1.LinkScan) error {
	body, err := json.Marshal(ResultFromScan(scan))
	if err != nil {
		return err
	}
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := p.JS.Publish(ctx, ResultsSubject+scan.Name, body, jetstream.WithMsgID("result-"+scan.Name)); err != nil {
		return fmt.Errorf("publish result %s: %w", scan.Name, err)
	}
	return nil
}

// EnsureStreams creates both streams and the durable consumer when missing.
// In the cluster they are declared by gitops/platform/nats; this keeps local
// runs and tests self-contained.
func EnsureStreams(ctx context.Context, js jetstream.JetStream) error {
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: JobsStream, Subjects: []string{JobsSubject + ">"},
		Retention: jetstream.WorkQueuePolicy, MaxAge: 10 * time.Minute,
		Discard: jetstream.DiscardNew, Duplicates: 2 * time.Minute,
	}); err != nil {
		return err
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: ResultsStream, Subjects: []string{ResultsSubject + ">"},
		Retention: jetstream.LimitsPolicy, MaxAge: time.Hour, Duplicates: 2 * time.Minute,
	}); err != nil {
		return err
	}
	_, err := js.CreateOrUpdateConsumer(ctx, JobsStream, jetstream.ConsumerConfig{
		Durable: ConsumerName, AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 30 * time.Second, MaxDeliver: 5, MaxAckPending: 200,
		FilterSubject: JobsSubject + ">",
	})
	return err
}
