// Package hashing turns user inputs and reported entities into keyed,
// non-reversible identifiers so that raw message text, phone numbers and UPI
// IDs are never stored.
//
// Digests are HMAC-SHA256 with a server-side secret key. A purpose label is
// mixed into every MAC (domain separation), so the same string hashed as a
// "phone" and as "text" gives unrelated digests. Entities are normalized first
// so that "+91 98765 43210" and "09876543210" map to the same identifier.
//
// Wiring (gateway-api main): build one Hasher at startup with
//
//	h, err := hashing.NewFromEnv() // reads SATARK_HMAC_KEY (base64, >= 32 bytes)
//
// and pass it to store.New. The key is mounted from the Secret
// gateway-api-hmac; rotating it breaks report aggregation, so treat it as
// long-lived.
package hashing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode"
)

// Size is the digest length in bytes.
const Size = sha256.Size

// MinKeyLen is the minimum accepted key length in bytes.
const MinKeyLen = 32

// EnvKey is the environment variable holding the base64-encoded key.
const EnvKey = "SATARK_HMAC_KEY"

// Kind identifies the purpose of a digest and is mixed into the MAC.
type Kind string

const (
	KindText     Kind = "text"     // message text submitted for a scan
	KindQR       Kind = "qr"       // decoded QR payload
	KindURL      Kind = "url"      // a URL submitted or extracted
	KindPhone    Kind = "phone"    // reported phone number
	KindUPI      Kind = "upi"      // reported UPI ID (VPA)
	KindDomain   Kind = "domain"   // reported domain
	KindReporter Kind = "reporter" // channel user id of a reporter
)

var (
	ErrShortKey     = errors.New("hashing: key must be at least 32 bytes")
	ErrInvalidValue = errors.New("hashing: value is not valid for its kind")
)

// Hasher computes keyed digests. It is safe for concurrent use.
type Hasher struct {
	key []byte
}

// New returns a Hasher using a copy of key.
func New(key []byte) (*Hasher, error) {
	if len(key) < MinKeyLen {
		return nil, ErrShortKey
	}
	k := make([]byte, len(key))
	copy(k, key)
	return &Hasher{key: k}, nil
}

// NewFromEnv reads a base64 (standard or URL alphabet) key from SATARK_HMAC_KEY.
func NewFromEnv() (*Hasher, error) {
	raw := strings.TrimSpace(os.Getenv(EnvKey))
	if raw == "" {
		return nil, fmt.Errorf("hashing: %s is not set", EnvKey)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		if key, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(raw, "=")); err != nil {
			return nil, fmt.Errorf("hashing: %s is not valid base64", EnvKey)
		}
	}
	return New(key)
}

// Sum returns HMAC-SHA256(key, kind || 0x00 || value) without normalization.
func (h *Hasher) Sum(kind Kind, value string) []byte {
	m := hmac.New(sha256.New, h.key)
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(value))
	return m.Sum(nil)
}

// Input hashes a scan input. Text is whitespace-collapsed and case-folded so
// trivially re-formatted copies of the same message share a digest.
func (h *Hasher) Input(kind Kind, value string) ([]byte, error) {
	switch kind {
	case KindText, KindQR:
		return h.Sum(kind, normalizeText(value)), nil
	case KindURL:
		u, err := NormalizeURL(value)
		if err != nil {
			return nil, err
		}
		return h.Sum(kind, u), nil
	default:
		return nil, fmt.Errorf("hashing: %q is not an input kind", kind)
	}
}

// Entity normalizes value for its kind and returns the digest together with
// the masked form that may be shown to users.
func (h *Hasher) Entity(kind Kind, value string) (digest []byte, masked string, err error) {
	var norm string
	switch kind {
	case KindPhone:
		norm, err = NormalizePhone(value)
		masked = MaskPhone(norm)
	case KindUPI:
		norm, err = NormalizeUPI(value)
		masked = MaskUPI(norm)
	case KindDomain:
		norm, err = NormalizeDomain(value)
		masked = norm // domains are public information
	case KindReporter:
		norm = strings.TrimSpace(value)
		if norm == "" {
			err = ErrInvalidValue
		}
	default:
		return nil, "", fmt.Errorf("hashing: %q is not an entity kind", kind)
	}
	if err != nil {
		return nil, "", err
	}
	return h.Sum(kind, norm), masked, nil
}

// Equal compares two digests in constant time.
func Equal(a, b []byte) bool { return hmac.Equal(a, b) }

func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// NormalizePhone reduces an Indian phone number to its 10 national digits.
// Numbers that are not 10-digit mobiles after stripping +91/91/0 prefixes are
// kept as their full digit string (e.g. landlines with STD codes).
func NormalizePhone(s string) (string, error) {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case len(d) == 12 && strings.HasPrefix(d, "91"):
		d = d[2:]
	case len(d) == 11 && strings.HasPrefix(d, "0"):
		d = d[1:]
	}
	if len(d) < 6 || len(d) > 13 {
		return "", ErrInvalidValue
	}
	return d, nil
}

// NormalizeUPI lower-cases and validates a UPI VPA (handle@psp).
func NormalizeUPI(s string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	at := strings.IndexByte(v, '@')
	if at <= 0 || at != strings.LastIndexByte(v, '@') || at == len(v)-1 {
		return "", ErrInvalidValue
	}
	for _, r := range v {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(".-_@", r)) {
			return "", ErrInvalidValue
		}
	}
	return v, nil
}

// NormalizeDomain lower-cases a host name and strips a trailing dot and a
// leading "www.".
func NormalizeDomain(s string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(s))
	if strings.Contains(d, "://") {
		u, err := url.Parse(d)
		if err != nil {
			return "", ErrInvalidValue
		}
		d = u.Hostname()
	}
	d = strings.TrimSuffix(d, ".")
	d = strings.TrimPrefix(d, "www.")
	if d == "" || !strings.Contains(d, ".") || strings.ContainsAny(d, " /@:") {
		return "", ErrInvalidValue
	}
	return d, nil
}

// NormalizeURL lower-cases scheme and host, drops the fragment and default
// ports, and defaults to https when no scheme is given.
func NormalizeURL(s string) (string, error) {
	raw := strings.TrimSpace(s)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ErrInvalidValue
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	u.Host = host
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// MaskPhone keeps the first two and last two digits: 98XXXXXX21.
func MaskPhone(d string) string {
	if len(d) <= 4 {
		return strings.Repeat("X", len(d))
	}
	return d[:2] + strings.Repeat("X", len(d)-4) + d[len(d)-2:]
}

// MaskUPI keeps the first two characters of the handle: xy***@examplebank.
func MaskUPI(v string) string {
	at := strings.IndexByte(v, '@')
	if at < 0 {
		return "***"
	}
	handle := v[:at]
	keep := 2
	if len(handle) < keep {
		keep = len(handle)
	}
	return handle[:keep] + "***" + v[at:]
}
