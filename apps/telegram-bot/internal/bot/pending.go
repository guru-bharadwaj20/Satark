package bot

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// pendingText is a message waiting for the "Who sent this?" answer. It is
// held in memory only, for at most the store's TTL, and deleted as soon as
// it is scanned.
type pendingText struct {
	chatID  int64
	text    string
	created time.Time
}

// Pending holds messages awaiting the sender question.
type Pending struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	items map[string]pendingText
	now   func() time.Time
}

// NewPending creates a store whose entries expire after ttl.
func NewPending(ttl time.Duration, max int) *Pending {
	return &Pending{ttl: ttl, max: max, items: map[string]pendingText{}, now: time.Now}
}

// Put stores text for chatID and returns its token.
func (p *Pending) Put(chatID int64, text string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	token := hex.EncodeToString(b[:])
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepLocked()
	if len(p.items) >= p.max {
		var oldestK string
		var oldestT time.Time
		for k, v := range p.items {
			if oldestK == "" || v.created.Before(oldestT) {
				oldestK, oldestT = k, v.created
			}
		}
		delete(p.items, oldestK)
	}
	p.items[token] = pendingText{chatID: chatID, text: text, created: p.now()}
	return token
}

// Take removes and returns the text for token if it belongs to chatID and
// has not expired.
func (p *Pending) Take(chatID int64, token string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	it, ok := p.items[token]
	if !ok || it.chatID != chatID {
		return "", false
	}
	delete(p.items, token)
	if p.now().Sub(it.created) > p.ttl {
		return "", false
	}
	return it.text, true
}

func (p *Pending) sweepLocked() {
	now := p.now()
	for k, v := range p.items {
		if now.Sub(v.created) > p.ttl {
			delete(p.items, k)
		}
	}
}
