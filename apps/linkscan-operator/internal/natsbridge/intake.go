/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package natsbridge

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-logr/logr"
	"github.com/nats-io/nats.go/jetstream"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

// Intake pulls scan jobs from JetStream and creates LinkScan objects.
// Each operator replica runs its own Intake (it does not need leader
// election) and keeps at most MaxActive unfinished scans, so sandbox
// capacity scales with the number of replicas (see the KEDA ScaledObject).
type Intake struct {
	Client    client.Client
	Consumer  jetstream.Consumer
	Namespace string
	// Replica identifies this operator pod (label value on created scans).
	Replica   string
	MaxActive int
	Log       logr.Logger
	// PollInterval is how long to wait when at capacity.
	PollInterval time.Duration
}

// NeedLeaderElection lets every replica consume jobs.
func (in *Intake) NeedLeaderElection() bool { return false }

// Start implements manager.Runnable.
func (in *Intake) Start(ctx context.Context) error {
	if in.PollInterval == 0 {
		in.PollInterval = 2 * time.Second
	}
	in.Log.Info("scan job intake started", "namespace", in.Namespace, "maxActive", in.MaxActive)
	for ctx.Err() == nil {
		n, err := in.Step(ctx)
		if err != nil && ctx.Err() == nil {
			in.Log.Error(err, "intake step failed")
		}
		if n == 0 {
			select {
			case <-ctx.Done():
			case <-time.After(in.PollInterval):
			}
		}
	}
	return nil
}

// Step fetches up to the free capacity and handles each job. It returns the
// number of messages processed.
func (in *Intake) Step(ctx context.Context) (int, error) {
	active, err := in.activeScans(ctx)
	if err != nil {
		return 0, err
	}
	free := in.MaxActive - active
	if free <= 0 {
		return 0, nil
	}
	batch, err := in.Consumer.Fetch(free, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		return 0, err
	}
	n := 0
	for msg := range batch.Messages() {
		in.handle(ctx, msg)
		n++
	}
	if err := batch.Error(); err != nil && !errors.Is(err, jetstream.ErrNoMessages) && !errors.Is(err, context.DeadlineExceeded) {
		return n, err
	}
	return n, nil
}

func (in *Intake) handle(ctx context.Context, msg jetstream.Msg) {
	var job Job
	if err := json.Unmarshal(msg.Data(), &job); err != nil || job.ID == "" || job.URL == "" {
		in.Log.Info("dropping malformed scan job", "subject", msg.Subject())
		jobsReceived.WithLabelValues("malformed").Inc()
		_ = msg.Term()
		return
	}
	scan := &scanv1alpha1.LinkScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.ID,
			Namespace: in.Namespace,
			Labels:    map[string]string{LabelIntake: in.Replica},
		},
		Spec: scanv1alpha1.LinkScanSpec{
			URL:            job.URL,
			TimeoutSeconds: job.TimeoutSeconds,
			Correlation:    job.Correlation,
		},
	}
	err := in.Client.Create(ctx, scan)
	switch {
	case err == nil:
		jobsReceived.WithLabelValues("created").Inc()
		_ = msg.Ack()
	case apierrors.IsAlreadyExists(err):
		jobsReceived.WithLabelValues("duplicate").Inc()
		_ = msg.Ack()
	case apierrors.IsInvalid(err):
		// The CRD rejected it (bad URL, out-of-range timeout): never retry.
		in.Log.Info("rejecting invalid scan job", "id", job.ID, "reason", err.Error())
		jobsReceived.WithLabelValues("invalid").Inc()
		_ = msg.Term()
	default:
		in.Log.Error(err, "could not create LinkScan; will retry", "id", job.ID)
		jobsReceived.WithLabelValues("error").Inc()
		_ = msg.NakWithDelay(5 * time.Second)
	}
}

func (in *Intake) activeScans(ctx context.Context) (int, error) {
	list := &scanv1alpha1.LinkScanList{}
	if err := in.Client.List(ctx, list, client.InNamespace(in.Namespace), client.MatchingLabels{LabelIntake: in.Replica}); err != nil {
		return 0, err
	}
	n := 0
	for i := range list.Items {
		if !list.Items[i].Status.Phase.IsFinished() {
			n++
		}
	}
	return n, nil
}
