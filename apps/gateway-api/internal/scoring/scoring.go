// Package scoring turns analyzer findings into a 0-100 risk score, a
// verdict bucket, ordered reasons and next steps.
//
// Model:
//
//  1. Signals are grouped by category (the part of the code before the
//     first dot). Within a category the strongest signal counts fully and
//     each further one adds a quarter of its weight, so ten urgency words
//     do not outweigh one OTP request.
//  2. Category scores are combined like independent probabilities
//     (noisy-OR): score = 100 * (1 - prod(1 - c/100)). The result is
//     bounded, monotonic and never reaches 100 from weak signals alone.
//  3. The "Who sent this?" answer nudges the score when there is already
//     at least one signal.
//  4. A few signals are decisive on their own (asking for an OTP/PIN,
//     "scan to receive money"); they floor the score at the scam bucket.
//
// There is no "safe" verdict: the best outcome is no_red_flags.
package scoring

import (
	"math"
	"sort"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
)

// Bucket thresholds (inclusive lower bounds).
const (
	SuspiciousAt = 30
	LikelyScamAt = 70
	MaxReasons   = 5
)

// Signal is one weighted piece of evidence.
type Signal struct {
	Code    string
	Weight  int // 0-100
	Message string
	Source  string
}

func (s Signal) category() string {
	c, _, _ := strings.Cut(s.Code, ".")
	return c
}

// decisive signals floor the score at LikelyScamAt.
var decisive = map[string]bool{
	"qr.receive_money":      true,
	"otp.share":             true,
	"otp.share_hi":          true,
	"otp.share_kn":          true,
	"otp.receive_money_pin": true,
	"remote_access.app":     true,
	"remote_access.app_hi":  true,
	"remote_access.app_kn":  true,
}

// informational signals are shown but never move the score on their own.
var informational = map[string]bool{
	"qr.upi_sends_money": true,
}

// Result is the scored outcome.
type Result struct {
	Score     int
	Verdict   string
	Reasons   []api.Reason
	NextSteps []string
}

// Score computes the result for signals in the given language.
func Score(signals []Signal, senderType, lang string) Result {
	byCat := map[string][]int{}
	floor := 0
	risky := 0
	for _, s := range signals {
		w := clamp(s.Weight, 0, 100)
		if informational[s.Code] {
			continue
		}
		byCat[s.category()] = append(byCat[s.category()], w)
		risky++
		if decisive[s.Code] {
			floor = LikelyScamAt
		}
	}

	keep := 1.0
	for _, ws := range byCat {
		sort.Sort(sort.Reverse(sort.IntSlice(ws)))
		c := float64(ws[0])
		for _, w := range ws[1:] {
			c += 0.25 * float64(w)
		}
		keep *= 1 - math.Min(c, 95)/100
	}
	score := 100 * (1 - keep)

	reasons := make([]api.Reason, 0, len(signals)+1)
	for _, s := range signals {
		reasons = append(reasons, api.Reason{
			Code: s.Code, Message: localize(s.Code, s.Message, lang), Weight: clamp(s.Weight, 0, 100), Source: s.Source,
		})
	}

	if risky > 0 {
		if adj, ok := senderAdjust[senderType]; ok {
			score += float64(adj.points)
			if adj.code != "" {
				reasons = append(reasons, api.Reason{
					Code: adj.code, Message: localize(adj.code, "", lang), Weight: max(adj.points, 0), Source: "sender",
				})
			}
		}
	}

	final := clamp(int(math.Round(score)), 0, 100)
	if final < floor {
		final = floor
	}
	sort.SliceStable(reasons, func(i, j int) bool { return reasons[i].Weight > reasons[j].Weight })
	if len(reasons) > MaxReasons {
		reasons = reasons[:MaxReasons]
	}
	verdict := Bucket(final)
	return Result{Score: final, Verdict: verdict, Reasons: reasons, NextSteps: nextSteps(verdict, byCat, lang)}
}

// Bucket maps a score to a verdict.
func Bucket(score int) string {
	switch {
	case score >= LikelyScamAt:
		return api.VerdictLikelyScam
	case score >= SuspiciousAt:
		return api.VerdictSuspicious
	default:
		return api.VerdictNoRedFlags
	}
}

type adjustment struct {
	points int
	code   string
}

var senderAdjust = map[string]adjustment{
	api.SenderUnknownNumber: {10, "sender.unknown_number"},
	api.SenderGroup:         {5, "sender.group"},
	// A saved contact lowers the score slightly, but contacts' accounts
	// get hijacked, so it never clears real red flags.
	api.SenderSavedContact: {-10, ""},
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
