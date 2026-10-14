package bot

import (
	"sync"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/i18n"
)

// Prefs stores each chat's chosen language. Only the chat ID and a
// two-letter code are kept, in memory.
type Prefs struct {
	mu   sync.RWMutex
	lang map[int64]string
}

// NewPrefs creates an empty store.
func NewPrefs() *Prefs { return &Prefs{lang: map[int64]string{}} }

// Lang returns the chat's language, or the fallback derived from the
// user's Telegram client language.
func (p *Prefs) Lang(chatID int64, telegramCode string) string {
	p.mu.RLock()
	l, ok := p.lang[chatID]
	p.mu.RUnlock()
	if ok {
		return l
	}
	return i18n.FromTelegram(telegramCode)
}

// SetLang stores the chat's language. Unsupported codes are ignored.
func (p *Prefs) SetLang(chatID int64, lang string) bool {
	if !i18n.Supported(lang) {
		return false
	}
	p.mu.Lock()
	p.lang[chatID] = lang
	p.mu.Unlock()
	return true
}
