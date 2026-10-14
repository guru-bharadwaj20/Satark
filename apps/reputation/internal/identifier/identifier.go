// Package identifier normalises and hashes the things people report:
// Indian mobile numbers, UPI IDs (VPAs) and domains.
package identifier

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
)

// Kind is the type of reported subject.
type Kind string

const (
	Phone  Kind = "phone"
	UPI    Kind = "upi"
	Domain Kind = "domain"
)

// ErrInvalid is returned when a value cannot be normalised for its kind.
var ErrInvalid = errors.New("invalid identifier")

// ParseKind validates a kind string.
func ParseKind(s string) (Kind, error) {
	switch k := Kind(strings.ToLower(strings.TrimSpace(s))); k {
	case Phone, UPI, Domain:
		return k, nil
	}
	return "", ErrInvalid
}

var (
	nonDigit  = regexp.MustCompile(`\D`)
	upiRe     = regexp.MustCompile(`^[a-z0-9._-]{2,256}@[a-z][a-z0-9]{1,63}$`)
	domainRe  = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$`)
	indMobile = regexp.MustCompile(`^[6-9]\d{9}$`)
)

// Normalize returns the canonical form of value for kind.
//
//	phone:  10-digit Indian mobile number without +91/0 prefix
//	upi:    lower-cased VPA ("name@bank")
//	domain: lower-cased ASCII (punycode) host without "www." or scheme
func Normalize(k Kind, value string) (string, error) {
	v := strings.TrimSpace(value)
	switch k {
	case Phone:
		d := nonDigit.ReplaceAllString(v, "")
		switch {
		case len(d) == 12 && strings.HasPrefix(d, "91"):
			d = d[2:]
		case len(d) == 11 && strings.HasPrefix(d, "0"):
			d = d[1:]
		}
		if !indMobile.MatchString(d) {
			return "", ErrInvalid
		}
		return d, nil
	case UPI:
		v = strings.TrimPrefix(strings.ToLower(v), "upi://pay?pa=")
		if !upiRe.MatchString(v) {
			return "", ErrInvalid
		}
		return v, nil
	case Domain:
		v = strings.ToLower(v)
		if strings.Contains(v, "://") {
			u, err := url.Parse(v)
			if err != nil {
				return "", ErrInvalid
			}
			v = u.Hostname()
		} else if i := strings.IndexAny(v, "/?#:"); i >= 0 {
			v = v[:i]
		}
		v = strings.TrimSuffix(strings.TrimPrefix(v, "www."), ".")
		a, err := idna.Lookup.ToASCII(v)
		if err != nil || !domainRe.MatchString(a) {
			return "", ErrInvalid
		}
		return a, nil
	}
	return "", ErrInvalid
}

// Mask hides most of a normalised identifier so lookups never echo back
// full personal data: phone "98XXXXXX21", UPI "xy***@examplebank".
// Domains are public and are returned unchanged.
func Mask(k Kind, normalized string) string {
	switch k {
	case Phone:
		if len(normalized) != 10 {
			return "XXXXXXXXXX"
		}
		return normalized[:2] + "XXXXXX" + normalized[8:]
	case UPI:
		at := strings.LastIndexByte(normalized, '@')
		if at < 0 {
			return "***"
		}
		user := normalized[:at]
		if len(user) > 2 {
			user = user[:2]
		}
		return user + "***" + normalized[at:]
	}
	return normalized
}

// Hasher derives stable, non-reversible keys with a server-side pepper so
// the database never holds raw phone numbers, VPAs or reporter IDs.
type Hasher struct{ pepper []byte }

// NewHasher returns a Hasher using pepper as the HMAC key.
func NewHasher(pepper string) *Hasher { return &Hasher{pepper: []byte(pepper)} }

// Subject hashes a normalised identifier.
func (h *Hasher) Subject(k Kind, normalized string) string {
	return h.sum("subject:" + string(k) + ":" + normalized)
}

// Reporter hashes an opaque reporter identifier (e.g. a Telegram user ID).
func (h *Hasher) Reporter(id string) string { return h.sum("reporter:" + id) }

func (h *Hasher) sum(s string) string {
	m := hmac.New(sha256.New, h.pepper)
	m.Write([]byte(s))
	return hex.EncodeToString(m.Sum(nil))
}
