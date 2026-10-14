// Package jobs publishes sandbox scan jobs for suspicious links to NATS
// JetStream, where linkscan-operator turns each one into a LinkScan.
//
// Wire contract (shared with linkscan-operator and telegram-bot):
//
//	subject:  scan.jobs.<id>          stream SCAN_JOBS (work queue)
//	header:   Nats-Msg-Id: <id>       JetStream de-duplication
//	body:     {"id","url","requested_at","timeout_seconds","correlation"}
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// SubjectPrefix is the subject namespace for scan jobs.
	SubjectPrefix = "scan.jobs."
	// StreamName is the JetStream stream that holds scan jobs.
	StreamName = "SCAN_JOBS"
	// MaxURLLength mirrors the LinkScan CRD validation.
	MaxURLLength = 2048
)

var idPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,40}[a-z0-9])?$`)

// Job asks the sandbox to open one URL.
type Job struct {
	ID             string            `json:"id"`
	URL            string            `json:"url"`
	RequestedAt    time.Time         `json:"requested_at"`
	TimeoutSeconds int64             `json:"timeout_seconds,omitempty"`
	Correlation    map[string]string `json:"correlation,omitempty"`
}

// ErrInvalidJob is returned for jobs that the operator would reject.
var ErrInvalidJob = errors.New("invalid scan job")

// NewID returns a random DNS-label-safe job id (also used as the LinkScan name).
func NewID() string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "ls-" + hex.EncodeToString(b[:])
}

// NewJob builds a job with a fresh id.
func NewJob(rawURL string, correlation map[string]string) Job {
	return Job{ID: NewID(), URL: rawURL, RequestedAt: time.Now().UTC(), Correlation: correlation}
}

// Validate checks the job against the LinkScan CRD constraints.
func (j Job) Validate() error {
	if !idPattern.MatchString(j.ID) {
		return fmt.Errorf("%w: id %q is not a DNS label", ErrInvalidJob, j.ID)
	}
	if len(j.URL) == 0 || len(j.URL) > MaxURLLength {
		return fmt.Errorf("%w: url length %d", ErrInvalidJob, len(j.URL))
	}
	u, err := url.Parse(j.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: url must be absolute http(s)", ErrInvalidJob)
	}
	if j.TimeoutSeconds != 0 && (j.TimeoutSeconds < 10 || j.TimeoutSeconds > 300) {
		return fmt.Errorf("%w: timeout_seconds out of range", ErrInvalidJob)
	}
	return nil
}

// Subject returns the NATS subject for the job.
func (j Job) Subject() string { return SubjectPrefix + j.ID }

// Publisher sends scan jobs to JetStream.
type Publisher struct {
	js      jetstream.JetStream
	timeout time.Duration
}

// NewPublisher wraps an existing NATS connection.
func NewPublisher(nc *nats.Conn) (*Publisher, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	return &Publisher{js: js, timeout: 5 * time.Second}, nil
}

// Connect dials NATS with reconnect settings suited to a long-running service.
func Connect(natsURL, name string) (*nats.Conn, error) {
	return nats.Connect(natsURL,
		nats.Name(name),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.Timeout(5*time.Second),
	)
}

// EnsureStream creates the SCAN_JOBS stream if it does not exist. In the
// cluster the stream is declared by the platform; this keeps local runs and
// tests self-contained.
func (p *Publisher) EnsureStream(ctx context.Context) error {
	_, err := p.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       StreamName,
		Subjects:   []string{SubjectPrefix + ">"},
		Retention:  jetstream.WorkQueuePolicy,
		MaxAge:     10 * time.Minute,
		Discard:    jetstream.DiscardNew,
		Duplicates: 2 * time.Minute,
	})
	return err
}

// Publish validates and publishes a job, waiting for the JetStream ack.
// Re-publishing the same id inside the duplicate window is a no-op.
func (p *Publisher) Publish(ctx context.Context, job Job) error {
	job.URL = strings.TrimSpace(job.URL)
	if err := job.Validate(); err != nil {
		return err
	}
	if job.RequestedAt.IsZero() {
		job.RequestedAt = time.Now().UTC()
	}
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	if _, err := p.js.Publish(ctx, job.Subject(), body, jetstream.WithMsgID(job.ID)); err != nil {
		return fmt.Errorf("publish scan job %s: %w", job.ID, err)
	}
	return nil
}
