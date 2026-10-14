// Package fixture replays recorded HTTP responses so tests exercise real
// upstream payloads (RDAP, crt.sh, Safe Browsing, redirects) without
// touching the network.
//
// A fixture directory holds index.json plus body files:
//
//	[{"method": "GET", "url": "https://rdap.org/domain/x.example",
//	  "status": 200, "headers": {"Content-Type": "application/rdap+json"},
//	  "body_file": "rdap_x.json", "ignore_query": false}]
package fixture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Interaction is one recorded request/response pair.
type Interaction struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Status      int               `json:"status"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodyFile    string            `json:"body_file,omitempty"`
	Body        string            `json:"body,omitempty"`
	IgnoreQuery bool              `json:"ignore_query,omitempty"`
}

// Replay is an http.RoundTripper that serves recorded interactions and
// fails loudly on anything unrecorded.
type Replay struct {
	dir   string
	items []Interaction

	mu   sync.Mutex
	Seen []string
}

// Load reads dir/index.json.
func Load(dir string) (*Replay, error) {
	b, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return nil, err
	}
	r := &Replay{dir: dir}
	if err := json.Unmarshal(b, &r.items); err != nil {
		return nil, fmt.Errorf("index.json: %w", err)
	}
	return r, nil
}

// Client returns an http.Client using the replay transport that does not
// follow redirects (callers that want redirects handle them themselves).
func (r *Replay) Client() *http.Client {
	return &http.Client{Transport: r, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// RoundTrip implements http.RoundTripper.
func (r *Replay) RoundTrip(req *http.Request) (*http.Response, error) {
	full := req.URL.String()
	noQuery := strings.SplitN(full, "?", 2)[0]
	for _, it := range r.items {
		if !strings.EqualFold(it.Method, req.Method) {
			continue
		}
		if it.URL != full && !(it.IgnoreQuery && it.URL == noQuery) {
			continue
		}
		body := []byte(it.Body)
		if it.BodyFile != "" {
			var err error
			if body, err = os.ReadFile(filepath.Join(r.dir, it.BodyFile)); err != nil {
				return nil, err
			}
		}
		h := http.Header{}
		for k, v := range it.Headers {
			h.Set(k, v)
		}
		r.mu.Lock()
		r.Seen = append(r.Seen, req.Method+" "+noQuery)
		r.mu.Unlock()
		return &http.Response{
			StatusCode: it.Status, Status: http.StatusText(it.Status), Header: h, Request: req,
			Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)),
			Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		}, nil
	}
	return nil, fmt.Errorf("fixture: no recorded interaction for %s %s", req.Method, full)
}
