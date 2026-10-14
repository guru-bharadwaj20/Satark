package scan

import (
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/scoring"
)

// EngineScorer adapts analyzer findings to the scoring engine.
type EngineScorer struct{}

// Score implements Scorer.
func (EngineScorer) Score(f Findings, lang string) api.ScanResponse {
	var signals []scoring.Signal
	if f.Text != nil {
		for _, m := range f.Text.Matches {
			signals = append(signals, scoring.Signal{Code: m.ID, Weight: m.Weight, Message: m.Reason, Source: "rules"})
		}
	}
	if f.QR != nil {
		for _, c := range f.QR.Codes {
			for _, fl := range c.Flags {
				signals = append(signals, scoring.Signal{Code: fl.Code, Weight: fl.Weight, Message: fl.Message, Source: "qr"})
			}
		}
	}
	r := scoring.Score(signals, f.SenderType, lang)
	return api.ScanResponse{
		Verdict:   r.Verdict,
		RiskScore: r.Score,
		Reasons:   r.Reasons,
		NextSteps: r.NextSteps,
	}
}
