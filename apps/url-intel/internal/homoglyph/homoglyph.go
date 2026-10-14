// Package homoglyph detects internationalised (punycode) domains and
// Unicode characters that imitate Latin letters, e.g. "hdfcbаnk.com" with
// a Cyrillic "а", which displays like the real bank domain.
package homoglyph

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/net/idna"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/lookalike"
)

// latinLookalikes maps common confusable code points (Unicode TR39 subset)
// to the ASCII letter they resemble.
var latinLookalikes = map[rune]rune{
	// Cyrillic
	'а': 'a', 'в': 'b', 'с': 'c', 'ԁ': 'd', 'е': 'e', 'һ': 'h', 'і': 'i', 'ј': 'j', 'к': 'k',
	'ӏ': 'l', 'м': 'm', 'о': 'o', 'р': 'p', 'ԛ': 'q', 'ѕ': 's', 'т': 't', 'ս': 'u', 'ѵ': 'v',
	'ԝ': 'w', 'х': 'x', 'у': 'y', 'ү': 'y', 'ɡ': 'g', 'п': 'n',
	// Greek
	'α': 'a', 'β': 'b', 'ε': 'e', 'η': 'n', 'ι': 'i', 'κ': 'k', 'ν': 'v', 'ο': 'o', 'ρ': 'p',
	'τ': 't', 'υ': 'u', 'χ': 'x', 'γ': 'y', 'ω': 'w',
	// Latin extensions and accents frequently abused
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a', 'ç': 'c', 'è': 'e',
	'é': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ı': 'i',
	'ñ': 'n', 'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o', 'ō': 'o', 'ù': 'u',
	'ú': 'u', 'û': 'u', 'ü': 'u', 'ý': 'y', 'ÿ': 'y', 'ḷ': 'l', 'ṃ': 'm', 'ṅ': 'n', 'ṭ': 't',
	// Digits and letter-like symbols
	'ℓ': 'l', 'ⅰ': 'i', 'ⅼ': 'l', 'ⅽ': 'c', 'ⅾ': 'd', 'ⅿ': 'm',
}

// Result describes the Unicode properties of a host.
type Result struct {
	Unicode     string   `json:"unicode"`
	IDN         bool     `json:"idn"`
	Scripts     []string `json:"scripts,omitempty"`
	MixedScript bool     `json:"mixed_script"`
	Skeleton    string   `json:"skeleton,omitempty"`
	Brand       string   `json:"brand,omitempty"`
}

// Analyze inspects an ASCII (possibly punycode) host.
func Analyze(host string, brands *lookalike.Detector) Result {
	uni, err := idna.ToUnicode(host)
	if err != nil {
		uni = host
	}
	r := Result{Unicode: uni, IDN: uni != host || strings.Contains(host, "xn--")}
	if !r.IDN {
		return r
	}
	var sk strings.Builder
	for _, label := range strings.Split(uni, ".") {
		scripts := scriptsOf(label)
		if len(scripts) > 1 {
			r.MixedScript = true
		}
		for _, s := range scripts {
			if !contains(r.Scripts, s) {
				r.Scripts = append(r.Scripts, s)
			}
		}
		if sk.Len() > 0 {
			sk.WriteByte('.')
		}
		for _, c := range label {
			if l, ok := latinLookalikes[c]; ok {
				sk.WriteRune(l)
			} else {
				sk.WriteRune(c)
			}
		}
	}
	r.Skeleton = sk.String()
	if brands != nil && r.Skeleton != uni && isASCII(r.Skeleton) {
		if brands.IsOfficial(r.Skeleton) {
			r.Brand = r.Skeleton
		} else if m := brands.Find(r.Skeleton); m != nil {
			r.Brand = m.Brand
		}
	}
	return r
}

func scriptsOf(label string) []string {
	var out []string
	add := func(s string) {
		if !contains(out, s) {
			out = append(out, s)
		}
	}
	for _, c := range label {
		switch {
		case c < 0x80 && (unicode.IsDigit(c) || c == '-'):
			// common to every script
		case unicode.Is(unicode.Latin, c):
			add("Latin")
		case unicode.Is(unicode.Cyrillic, c):
			add("Cyrillic")
		case unicode.Is(unicode.Greek, c):
			add("Greek")
		case unicode.Is(unicode.Devanagari, c):
			add("Devanagari")
		case unicode.Is(unicode.Kannada, c):
			add("Kannada")
		case unicode.Is(unicode.Armenian, c):
			add("Armenian")
		default:
			add("Other")
		}
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Check implements analyzer.Check.
type Check struct{ Brands *lookalike.Detector }

// Name implements analyzer.Check.
func (*Check) Name() string { return "homoglyph" }

// Run implements analyzer.Check.
func (c *Check) Run(_ context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	r := Analyze(t.Host, c.Brands)
	if !r.IDN {
		return f
	}
	f.Data = r
	switch {
	case r.Brand != "":
		f.Signals = append(f.Signals, analyzer.Signal{ID: "homoglyph.brand", Severity: analyzer.SeverityHigh, Weight: 45,
			Detail: fmt.Sprintf("%s is shown as %q, which imitates %s using look-alike characters", t.Host, r.Unicode, r.Brand)})
	case r.MixedScript:
		f.Signals = append(f.Signals, analyzer.Signal{ID: "homoglyph.mixed_script", Severity: analyzer.SeverityHigh, Weight: 30,
			Detail: fmt.Sprintf("domain mixes %s characters, a common disguise", strings.Join(r.Scripts, " and "))})
	default:
		f.Signals = append(f.Signals, analyzer.Signal{ID: "homoglyph.punycode", Severity: analyzer.SeverityLow, Weight: 5,
			Detail: fmt.Sprintf("internationalised domain displayed as %q", r.Unicode)})
	}
	return f
}
