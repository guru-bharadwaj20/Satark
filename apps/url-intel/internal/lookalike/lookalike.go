// Package lookalike detects domains that imitate well-known bank and brand
// domains: typosquats (edit distance), character swaps such as 0/o and rn/m,
// and "combo" domains that embed a brand name (sbi-kyc-update.example).
package lookalike

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainutil"
)

//go:embed brands.txt
var defaultBrands string

// Brand is an impersonation target.
type Brand struct {
	Label    string
	Official []string
}

// Match explains why a host resembles a brand.
type Match struct {
	Brand    string `json:"brand"`
	Kind     string `json:"kind"` // typosquat | brand_in_domain | brand_in_subdomain
	Distance int    `json:"distance,omitempty"`
}

// Detector holds the brand list.
type Detector struct{ brands []Brand }

// Default returns a Detector with the embedded brand list.
func Default() *Detector {
	d, err := Parse(strings.NewReader(defaultBrands))
	if err != nil {
		panic(err)
	}
	return d
}

// Parse reads a brand list in brands.txt format.
func Parse(r io.Reader) (*Detector, error) {
	d := &Detector{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			return nil, fmt.Errorf("brands line %d: want 2 fields, got %d", n, len(f))
		}
		d.brands = append(d.brands, Brand{Label: strings.ToLower(f[0]), Official: strings.Split(strings.ToLower(f[1]), ",")})
	}
	return d, sc.Err()
}

// OfficialDomains lists every official domain of every brand.
func (d *Detector) OfficialDomains() []string {
	var out []string
	for _, b := range d.brands {
		out = append(out, b.Official...)
	}
	return out
}

// IsOfficial reports whether host belongs to any listed brand.
func (d *Detector) IsOfficial(host string) bool {
	for _, b := range d.brands {
		if b.owns(host) {
			return true
		}
	}
	return false
}

func (b Brand) owns(host string) bool {
	for _, o := range b.Official {
		if domainutil.IsSubdomainOf(host, o) {
			return true
		}
	}
	return false
}

// Find returns the strongest match for host, or nil.
func (d *Detector) Find(host string) *Match {
	host = strings.ToLower(host)
	if domainutil.IsIP(host) || d.IsOfficial(host) {
		return nil
	}
	label := domainutil.Label(host)
	reg := domainutil.Registrable(host)
	sub := strings.TrimSuffix(strings.TrimSuffix(host, reg), ".")
	skel := Skeleton(label)

	var best *Match
	for _, b := range d.brands {
		// Typosquat: the registrable label is a few edits away from the brand.
		if lim := maxDistance(b.Label); lim > 0 && label != b.Label {
			dist := Distance(label, b.Label)
			if sd := Distance(skel, b.Label); sd < dist {
				dist = sd
			}
			if dist <= lim && (best == nil || best.Kind != "typosquat" || dist < best.Distance) {
				best = &Match{Brand: b.Label, Kind: "typosquat", Distance: dist}
				continue
			}
		}
		if label == b.Label || skel == b.Label {
			// Brand label on a different suffix, e.g. hdfcbank.co or sbi.top.
			best = pick(best, &Match{Brand: b.Label, Kind: "brand_in_domain"})
			continue
		}
		if containsToken(label, b.Label) || containsToken(skel, b.Label) {
			best = pick(best, &Match{Brand: b.Label, Kind: "brand_in_domain"})
			continue
		}
		if sub != "" && containsToken(sub, b.Label) {
			best = pick(best, &Match{Brand: b.Label, Kind: "brand_in_subdomain"})
		}
	}
	return best
}

func pick(cur, m *Match) *Match {
	if cur == nil {
		return m
	}
	return cur
}

// maxDistance is the edit budget for a brand; short names are too
// collision-prone for fuzzy matching.
func maxDistance(label string) int {
	switch n := len(label); {
	case n < 5:
		return 0
	case n <= 8:
		return 1
	default:
		return 2
	}
}

// containsToken reports whether brand appears in s. Short brands (under 5
// characters, e.g. "sbi") must be delimited by '-', '.' or the string edge
// so that words like "nsbi" are not flagged.
func containsToken(s, brand string) bool {
	if len(brand) >= 5 {
		return strings.Contains(s, brand)
	}
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '.' || r == '_' }) {
		if tok == brand {
			return true
		}
	}
	return false
}

var confusables = strings.NewReplacer(
	"rn", "m", "vv", "w", "cl", "d", "0", "o", "1", "l", "3", "e", "4", "a", "5", "s", "7", "t", "8", "b", "@", "a", "$", "s",
)

// Skeleton maps ASCII look-alike sequences to the letters they imitate.
func Skeleton(s string) string { return confusables.Replace(strings.ToLower(s)) }

// Distance is the optimal-string-alignment (Damerau-Levenshtein) distance.
func Distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev2 := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(rb)]
}

// Check implements analyzer.Check.
type Check struct{ D *Detector }

// Name implements analyzer.Check.
func (*Check) Name() string { return "lookalike" }

// Run implements analyzer.Check.
func (c *Check) Run(_ context.Context, t *analyzer.Target) analyzer.Finding {
	var f analyzer.Finding
	m := c.D.Find(t.Host)
	if m == nil {
		return f
	}
	f.Data = m
	switch m.Kind {
	case "typosquat":
		f.Signals = append(f.Signals, analyzer.Signal{ID: "lookalike.typosquat", Severity: analyzer.SeverityHigh, Weight: 35,
			Detail: fmt.Sprintf("%s looks like a misspelling of %s", t.Host, m.Brand)})
	case "brand_in_domain":
		f.Signals = append(f.Signals, analyzer.Signal{ID: "lookalike.brand_in_domain", Severity: analyzer.SeverityHigh, Weight: 30,
			Detail: fmt.Sprintf("%s uses the name %q but is not an official domain", t.Host, m.Brand)})
	case "brand_in_subdomain":
		f.Signals = append(f.Signals, analyzer.Signal{ID: "lookalike.brand_in_subdomain", Severity: analyzer.SeverityHigh, Weight: 30,
			Detail: fmt.Sprintf("%s puts %q in front of an unrelated domain", t.Host, m.Brand)})
	}
	return f
}
