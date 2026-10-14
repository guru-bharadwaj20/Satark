package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func runServer(t *testing.T) *server.Server {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natstest.RunServer(&opts)
	t.Cleanup(s.Shutdown)
	return s
}

func TestValidate(t *testing.T) {
	ok := Job{ID: NewID(), URL: "https://example.com/a"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []Job{
		{ID: "Bad_ID", URL: "https://example.com"},
		{ID: "ls-1", URL: "ftp://example.com"},
		{ID: "ls-1", URL: "/relative"},
		{ID: "ls-1", URL: "https://example.com", TimeoutSeconds: 5},
	}
	for _, j := range bad {
		if err := j.Validate(); !errors.Is(err, ErrInvalidJob) {
			t.Errorf("%+v: expected ErrInvalidJob, got %v", j, err)
		}
	}
}

func TestPublishDeliversAndDedupes(t *testing.T) {
	s := runServer(t)
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	ctx := context.Background()

	p, err := NewPublisher(nc)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureStream(ctx); err != nil {
		t.Fatal(err)
	}

	job := NewJob("https://bit.example/x", map[string]string{"telegram_chat_id": "42"})
	if err := p.Publish(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(ctx, job); err != nil { // duplicate
		t.Fatal(err)
	}

	js, _ := jetstream.New(nc)
	stream, err := js.Stream(ctx, StreamName)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := stream.Info(ctx)
	if info.State.Msgs != 1 {
		t.Fatalf("want 1 message after dedupe, got %d", info.State.Msgs)
	}
	msg, err := stream.GetMsg(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "scan.jobs."+job.ID {
		t.Fatalf("subject %q", msg.Subject)
	}
	var got Job
	if err := json.Unmarshal(msg.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != job.URL || got.Correlation["telegram_chat_id"] != "42" || time.Since(got.RequestedAt) > time.Minute {
		t.Fatalf("unexpected job %+v", got)
	}
}

func TestPublishRejectsInvalid(t *testing.T) {
	s := runServer(t)
	nc, _ := nats.Connect(s.ClientURL())
	defer nc.Close()
	p, _ := NewPublisher(nc)
	if err := p.Publish(context.Background(), Job{ID: "ls-1", URL: "javascript:alert(1)"}); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("expected ErrInvalidJob, got %v", err)
	}
}
