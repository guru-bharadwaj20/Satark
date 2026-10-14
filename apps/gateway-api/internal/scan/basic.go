package scan

import (
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
)

// SumScorer is a minimal Scorer that adds up rule and QR weights. It is a
// stand-in until the scoring engine is wired in.
type SumScorer struct{}

// Score implements Scorer.
func (SumScorer) Score(f Findings, _ string) api.ScanResponse {
	var resp api.ScanResponse
	total := 0
	if f.Text != nil {
		for _, m := range f.Text.Matches {
			total += m.Weight
			resp.Reasons = append(resp.Reasons, api.Reason{Code: m.ID, Message: m.Reason, Weight: m.Weight, Source: "rules"})
		}
	}
	if f.QR != nil {
		for _, c := range f.QR.Codes {
			for _, fl := range c.Flags {
				total += fl.Weight
				resp.Reasons = append(resp.Reasons, api.Reason{Code: fl.Code, Message: fl.Message, Weight: fl.Weight, Source: "qr"})
			}
		}
	}
	resp.RiskScore = min(total, 100)
	switch {
	case resp.RiskScore >= 70:
		resp.Verdict = api.VerdictLikelyScam
	case resp.RiskScore >= 30:
		resp.Verdict = api.VerdictSuspicious
	default:
		resp.Verdict = api.VerdictNoRedFlags
	}
	return resp
}
