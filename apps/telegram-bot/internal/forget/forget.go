// Package forget implements the bot's /forget command: after an explicit
// confirmation it asks every registered data holder to erase what it
// keeps about the user, and reports back what happened.
//
// The bot registers one Eraser per store (its own session/preferences
// store, the reputation service, the gateway's scan history, ...). Erasers
// run independently so one failure doesn't block the others, and the user
// is told plainly if anything could not be deleted.
package forget

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Eraser deletes everything one component holds about a user.
type Eraser interface {
	Name() string
	Forget(ctx context.Context, userID string) error
}

// Button mirrors an inline keyboard option.
type Button struct{ Label, Data string }

// Reply is what the bot should send.
type Reply struct {
	Text    string
	Buttons [][]Button
}

const (
	ConfirmData = "forget:confirm"
	CancelData  = "forget:cancel"
	confirmTTL  = 5 * time.Minute
)

// Command handles /forget and its confirmation buttons.
type Command struct {
	Erasers []Eraser
	Timeout time.Duration
	Now     func() time.Time

	mu      sync.Mutex
	pending map[string]time.Time
}

// New returns a Command that will run erasers.
func New(erasers ...Eraser) *Command {
	return &Command{Erasers: erasers, Timeout: 15 * time.Second, Now: time.Now, pending: map[string]time.Time{}}
}

// Start is called for "/forget". It never deletes anything by itself.
func (c *Command) Start(userID string) Reply {
	c.mu.Lock()
	c.pending[userID] = c.Now()
	c.mu.Unlock()
	return Reply{
		Text: "This permanently deletes your data from Satark:\n" +
			"- your language and settings\n" +
			"- your scan history\n" +
			"- every scam report you've filed\n\n" +
			"Reports already sent to 1930, cybercrime.gov.in or Chakshu are not affected. Continue?",
		Buttons: [][]Button{{{"Yes, delete my data", ConfirmData}, {"Cancel", CancelData}}},
	}
}

// Callback handles the confirmation buttons.
func (c *Command) Callback(ctx context.Context, userID, data string) Reply {
	c.mu.Lock()
	started, ok := c.pending[userID]
	delete(c.pending, userID)
	c.mu.Unlock()

	if data == CancelData {
		return Reply{Text: "Cancelled. Nothing was deleted."}
	}
	if data != ConfirmData || !ok || c.Now().Sub(started) > confirmTTL {
		return Reply{Text: "That confirmation has expired. Send /forget again."}
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	failed := c.run(ctx, userID)
	if len(failed) == 0 {
		return Reply{Text: "Done. Your data has been deleted. You can keep using Satark; we'll start fresh."}
	}
	return Reply{Text: fmt.Sprintf(
		"Most of your data was deleted, but we couldn't reach: %s. Please send /forget again in a few minutes. "+
			"Anything left is also removed automatically by our retention policy.", strings.Join(failed, ", "))}
}

func (c *Command) run(ctx context.Context, userID string) []string {
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed []string
	)
	for _, e := range c.Erasers {
		wg.Add(1)
		go func(e Eraser) {
			defer wg.Done()
			if err := e.Forget(ctx, userID); err != nil {
				mu.Lock()
				failed = append(failed, e.Name())
				mu.Unlock()
			}
		}(e)
	}
	wg.Wait()
	return failed
}

// HTTPEraser POSTs {"<field>": userID} to a service's forget endpoint and
// treats any 2xx as success. For the reputation service:
//
//	NewHTTPEraser("community reports", "http://reputation.satark.svc:8083/v1/forget", "reporter_id")
type HTTPEraser struct {
	Label, URL, Field string
	HTTP              *http.Client
}

// NewHTTPEraser returns an HTTPEraser with a 5s client timeout.
func NewHTTPEraser(label, url, field string) *HTTPEraser {
	return &HTTPEraser{Label: label, URL: url, Field: field, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

func (h *HTTPEraser) Name() string { return h.Label }

func (h *HTTPEraser) Forget(ctx context.Context, userID string) error {
	body, _ := json.Marshal(map[string]string{h.Field: userID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: status %d", h.Label, resp.StatusCode)
	}
	return nil
}

// FuncEraser adapts a function, e.g. the bot's own session store delete.
type FuncEraser struct {
	Label string
	Fn    func(ctx context.Context, userID string) error
}

func (f FuncEraser) Name() string                                    { return f.Label }
func (f FuncEraser) Forget(ctx context.Context, userID string) error { return f.Fn(ctx, userID) }
