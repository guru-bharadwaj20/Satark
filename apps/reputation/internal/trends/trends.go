// Package trends renders the monthly anonymised scam-trends report.
//
// Only aggregates are published, never a phone number, UPI ID, domain or
// reporter. Any group with fewer than MinReporters distinct reporters is
// folded into "Other / small groups", and counts are rounded, so no single
// person's reports can be picked out of the table.
package trends

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

// MinReporters is the k-anonymity floor for a published row.
const MinReporters = 10

var categoryNames = map[string]string{
	"kyc_fraud": "KYC / account-block threats", "upi_collect": "UPI collect requests",
	"fake_job": "Fake jobs and tasks", "lottery": "Lottery and prizes",
	"impersonation": "Impersonating police, bank or courier", "investment": "Investment and trading",
	"loan_app": "Loan apps", "other": "Other",
}

var kindNames = map[string]string{"phone": "Phone numbers", "upi": "UPI IDs", "domain": "Websites"}

// Round coarsens a count: nearest 5 below 100, nearest 10 above.
func Round(n int) int {
	step := 5
	if n >= 100 {
		step = 10
	}
	return (n + step/2) / step * step
}

type group struct {
	name               string
	reports, reporters int
}

// Render produces the Markdown report for month (any time in the month).
// prev is last month's rows, used for the change column.
func Render(month time.Time, rows, prev []store.TrendRow) string {
	byCat := collapse(rows, func(r store.TrendRow) string { return r.Category }, categoryNames)
	prevCat := map[string]int{}
	for _, g := range collapse(prev, func(r store.TrendRow) string { return r.Category }, categoryNames) {
		prevCat[g.name] = g.reports
	}
	byKind := collapse(rows, func(r store.TrendRow) string { return r.Kind }, kindNames)

	total, reporters := 0, 0
	for _, r := range rows {
		total += r.Reports
		reporters += r.Reporters
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Scam trends: %s\n\n", month.Format("January 2006"))
	fmt.Fprintf(&b, "_Community reports filed through Satark in %s. Published %s._\n\n",
		month.Format("January 2006"), time.Now().UTC().Format("2 January 2006"))
	if total == 0 || reporters < MinReporters {
		b.WriteString("Too few reports this month to publish trends without risking anyone's privacy.\n")
		b.WriteString(footer)
		return b.String()
	}
	fmt.Fprintf(&b, "About **%d reports** this month.\n\n", Round(total))

	b.WriteString("## By scam type\n\n| Scam type | Reports | Change vs last month |\n|---|---|---|\n")
	for _, g := range byCat {
		fmt.Fprintf(&b, "| %s | ~%d | %s |\n", g.name, Round(g.reports), change(g.reports, prevCat[g.name]))
	}
	b.WriteString("\n## By what was reported\n\n| Reported | Reports |\n|---|---|\n")
	for _, g := range byKind {
		fmt.Fprintf(&b, "| %s | ~%d |\n", g.name, Round(g.reports))
	}
	if len(byCat) > 0 && byCat[0].name != otherGroup {
		fmt.Fprintf(&b, "\n## What to watch for\n\nThe most reported type was **%s**. %s\n", byCat[0].name, advice[byCat[0].name])
	}
	b.WriteString(footer)
	return b.String()
}

const otherGroup = "Other / small groups"

func collapse(rows []store.TrendRow, key func(store.TrendRow) string, names map[string]string) []group {
	agg := map[string]*group{}
	for _, r := range rows {
		k := key(r)
		if agg[k] == nil {
			n := names[k]
			if n == "" {
				n = k
			}
			agg[k] = &group{name: n}
		}
		agg[k].reports += r.Reports
		agg[k].reporters += r.Reporters // upper bound on distinct reporters; conservative in neither direction
	}
	other := group{name: otherGroup}
	var out []group
	for k, g := range agg {
		if g.reporters < MinReporters || k == "other" {
			other.reports += g.reports
			other.reporters += g.reporters
			continue
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].reports != out[j].reports {
			return out[i].reports > out[j].reports
		}
		return out[i].name < out[j].name
	})
	if other.reports > 0 && other.reporters >= MinReporters {
		out = append(out, other)
	}
	return out
}

func change(now, before int) string {
	if before == 0 {
		return "new"
	}
	pct := float64(now-before) / float64(before) * 100
	switch {
	case pct >= 10:
		return fmt.Sprintf("up %.0f%%", pct)
	case pct <= -10:
		return fmt.Sprintf("down %.0f%%", -pct)
	}
	return "about the same"
}

var advice = map[string]string{
	"KYC / account-block threats":           "Banks never ask you to update KYC through a link in an SMS or WhatsApp message.",
	"UPI collect requests":                  "You never need to enter your UPI PIN to receive money.",
	"Fake jobs and tasks":                   "Real jobs don't pay you to like videos or ask for a deposit to start.",
	"Lottery and prizes":                    "You can't win a lottery you didn't enter, and real prizes never need a fee.",
	"Impersonating police, bank or courier": "Police don't arrest people over video calls, and couriers don't ask for payment links.",
	"Investment and trading":                "Guaranteed high returns are a warning sign; check SEBI registration.",
	"Loan apps":                             "Use only RBI-registered lenders, and never give an app access to your contacts.",
}

const footer = `
---

**How this is made:** counts are from community reports to Satark, which are stored only as hashes. Groups with fewer than 10 independent reporters are merged, counts are rounded, and no number, UPI ID or website is ever listed. These are reports, not confirmed fraud.

Lost money? Call **1930** immediately or report at **cybercrime.gov.in**. Report fraud calls and messages on **Chakshu** (sancharsaathi.gov.in).
`
