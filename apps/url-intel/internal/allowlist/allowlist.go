// Package allowlist recognises official domains: restricted government and
// bank zones (.gov.in, .bank.in) and verified organisation domains.
package allowlist

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainutil"
)

//go:embed allowlist.txt
var defaultList string

// List is a set of official zones and domains.
type List struct {
	suffixes []string
	domains  map[string]bool
}

// Default returns the embedded allowlist plus extra verified domains.
func Default(extra ...string) *List {
	l, err := Parse(strings.NewReader(defaultList))
	if err != nil {
		panic(err)
	}
	for _, d := range extra {
		l.domains[strings.ToLower(d)] = true
	}
	return l
}

// Parse reads allowlist.txt format.
func Parse(r io.Reader) (*List, error) {
	l := &List{domains: map[string]bool{}}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			return nil, fmt.Errorf("allowlist line %d: want 2 fields", n)
		}
		v := strings.ToLower(f[1])
		switch f[0] {
		case "suffix":
			l.suffixes = append(l.suffixes, v)
		case "domain":
			l.domains[v] = true
		default:
			return nil, fmt.Errorf("allowlist line %d: unknown kind %q", n, f[0])
		}
	}
	return l, sc.Err()
}

// Match reports whether host is official and why.
func (l *List) Match(host string) (string, bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" || domainutil.IsIP(host) {
		return "", false
	}
	for _, s := range l.suffixes {
		if host != s && strings.HasSuffix(host, "."+s) {
			return "official ." + s + " domain", true
		}
	}
	for h := host; h != ""; {
		if l.domains[h] {
			return "verified domain " + h, true
		}
		_, rest, ok := strings.Cut(h, ".")
		if !ok {
			break
		}
		h = rest
	}
	return "", false
}
