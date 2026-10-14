/*
Copyright 2026 The Satark Authors.

Licensed under the MIT License. See LICENSE in the repository root.
*/

package natsbridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-logr/logr"
	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scanv1alpha1 "github.com/guru-bharadwaj20/satark/apps/linkscan-operator/api/v1alpha1"
)

func setup(t *testing.T) (jetstream.JetStream, client.Client) {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natstest.RunServer(&opts)
	t.Cleanup(s.Shutdown)
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureStreams(context.Background(), js); err != nil {
		t.Fatal(err)
	}
	scheme := runtime.NewScheme()
	_ = scanv1alpha1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&scanv1alpha1.LinkScan{}).Build()
	return js, c
}

func publishJob(t *testing.T, js jetstream.JetStream, job Job) {
	t.Helper()
	b, _ := json.Marshal(job)
	subject := JobsSubject + job.ID
	if job.ID == "" {
		subject = JobsSubject + "malformed"
	}
	if _, err := js.Publish(context.Background(), subject, b); err != nil {
		t.Fatal(err)
	}
}

func newIntake(t *testing.T, js jetstream.JetStream, c client.Client, maxActive int) *Intake {
	cons, err := js.Consumer(context.Background(), JobsStream, ConsumerName)
	if err != nil {
		t.Fatal(err)
	}
	return &Intake{Client: c, Consumer: cons, Namespace: "satark-sandbox", Replica: "op-0", MaxActive: maxActive, Log: logr.Discard()}
}

func TestIntakeCreatesLinkScans(t *testing.T) {
	js, c := setup(t)
	ctx := context.Background()
	publishJob(t, js, Job{ID: "ls-a", URL: "https://a.example/", Correlation: map[string]string{"telegram_chat_id": "1"}})
	publishJob(t, js, Job{ID: "ls-a", URL: "https://a.example/"}) // redelivered duplicate
	publishJob(t, js, Job{ID: "", URL: ""})                       // malformed

	in := newIntake(t, js, c, 10)
	if _, err := in.Step(ctx); err != nil {
		t.Fatal(err)
	}
	scan := &scanv1alpha1.LinkScan{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "satark-sandbox", Name: "ls-a"}, scan); err != nil {
		t.Fatal(err)
	}
	if scan.Spec.URL != "https://a.example/" || scan.Spec.Correlation["telegram_chat_id"] != "1" || scan.Labels[LabelIntake] != "op-0" {
		t.Fatalf("unexpected scan %+v", scan)
	}
	// Work-queue stream: everything acked or terminated is removed.
	stream, _ := js.Stream(ctx, JobsStream)
	deadline := time.Now().Add(2 * time.Second)
	for {
		info, _ := stream.Info(ctx)
		if info.State.Msgs == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d jobs left in stream", info.State.Msgs)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestIntakeRespectsCapacity(t *testing.T) {
	js, c := setup(t)
	ctx := context.Background()
	for _, id := range []string{"ls-1", "ls-2", "ls-3"} {
		publishJob(t, js, Job{ID: id, URL: "https://x.example/" + id})
	}
	in := newIntake(t, js, c, 2)
	n, err := in.Step(ctx)
	if err != nil || n != 2 {
		t.Fatalf("first step handled %d (%v), want 2", n, err)
	}
	if n, _ := in.Step(ctx); n != 0 {
		t.Fatalf("at capacity but handled %d", n)
	}
	// Finish one scan: capacity frees up.
	scan := &scanv1alpha1.LinkScan{}
	_ = c.Get(ctx, client.ObjectKey{Namespace: "satark-sandbox", Name: "ls-1"}, scan)
	scan.Status.Phase = scanv1alpha1.PhaseSucceeded
	if err := c.Status().Update(ctx, scan); err != nil {
		t.Fatal(err)
	}
	if n, _ := in.Step(ctx); n != 1 {
		t.Fatalf("handled %d after a scan finished, want 1", n)
	}
}

func TestPublishResult(t *testing.T) {
	js, _ := setup(t)
	ctx := context.Background()
	now := metav1.NewTime(time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC))
	scan := &scanv1alpha1.LinkScan{
		ObjectMeta: metav1.ObjectMeta{Name: "ls-r", Namespace: "satark-sandbox"},
		Spec:       scanv1alpha1.LinkScanSpec{URL: "https://bit.example/q", Correlation: map[string]string{"telegram_message_id": "9"}},
		Status: scanv1alpha1.LinkScanStatus{
			Phase:          scanv1alpha1.PhaseSucceeded,
			CompletionTime: &now,
			Result: &scanv1alpha1.ScanResult{
				FinalURL: "https://phish.example/", Forms: scanv1alpha1.FormFindings{OTP: true},
				Downloads: []string{"kyc.apk"},
			},
		},
	}
	p := &ResultPublisher{JS: js}
	if err := p.PublishResult(ctx, scan); err != nil {
		t.Fatal(err)
	}
	if err := p.PublishResult(ctx, scan); err != nil { // retry is de-duplicated
		t.Fatal(err)
	}
	stream, _ := js.Stream(ctx, ResultsStream)
	info, _ := stream.Info(ctx)
	if info.State.Msgs != 1 {
		t.Fatalf("want 1 result, got %d", info.State.Msgs)
	}
	msg, _ := stream.GetMsg(ctx, 1)
	var r Result
	if err := json.Unmarshal(msg.Data, &r); err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "scan.results.ls-r" || r.Phase != "Succeeded" || !r.Forms.OTP || r.Correlation["telegram_message_id"] != "9" || !r.FinishedAt.Equal(now.Time) {
		t.Fatalf("unexpected result %s %+v", msg.Subject, r)
	}
}
