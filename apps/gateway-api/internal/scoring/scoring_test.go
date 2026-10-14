package scoring

import (
	"strings"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
)

func sig(code string, w int) Signal {
	return Signal{Code: code, Weight: w, Message: code, Source: "rules"}
}

func TestBucket(t *testing.T) {
	cases := map[int]string{
		0: api.VerdictNoRedFlags, 29: api.VerdictNoRedFlags,
		30: api.VerdictSuspicious, 69: api.VerdictSuspicious,
		70: api.VerdictLikelyScam, 100: api.VerdictLikelyScam,
	}
	for score, want := range cases {
		if got := Bucket(score); got != want {
			t.Errorf("Bucket(%d)=%s want %s", score, got, want)
		}
	}
}

func TestNoSignals(t *testing.T) {
	r := Score(nil, api.SenderUnknownNumber, "en")
	if r.Score != 0 || r.Verdict != api.VerdictNoRedFlags || len(r.Reasons) != 0 {
		t.Fatalf("%+v", r)
	}
	if len(r.NextSteps) != 1 || strings.Contains(strings.ToLower(r.NextSteps[0]), "safe") {
		t.Fatalf("next steps must not call anything safe: %v", r.NextSteps)
	}
}

func TestSingleSignal(t *testing.T) {
	r := Score([]Signal{sig("urgency.deadline", 20)}, api.SenderUnknown, "en")
	if r.Score != 20 || r.Verdict != api.VerdictNoRedFlags {
		t.Fatalf("%+v", r)
	}
}

func TestSameCategoryDiminishes(t *testing.T) {
	r := Score([]Signal{sig("urgency.deadline", 20), sig("urgency.threat", 20)}, "", "en")
	if r.Score != 25 { // 20 + 0.25*20
		t.Fatalf("got %d", r.Score)
	}
}

func TestCategoriesCombineNoisyOr(t *testing.T) {
	// 1 - (0.65 * 0.75 * 0.8) = 0.61
	r := Score([]Signal{sig("utility.disconnection", 35), sig("utility.call_officer", 25), sig("urgency.deadline", 20)}, "", "en")
	// utility: 35 + 6.25 = 41.25 -> keep .5875; urgency keep .8 -> 1-.47 = 53
	if r.Score != 53 || r.Verdict != api.VerdictSuspicious {
		t.Fatalf("got %+v", r)
	}
}

func TestBoundedBelow100FromManyWeakSignals(t *testing.T) {
	var s []Signal
	for _, c := range []string{"a.x", "b.x", "c.x", "d.x", "e.x"} {
		s = append(s, sig(c, 10))
	}
	if r := Score(s, "", "en"); r.Score >= 50 {
		t.Fatalf("weak signals over-scored: %d", r.Score)
	}
}

func TestDecisiveFloor(t *testing.T) {
	r := Score([]Signal{sig("qr.receive_money", 40)}, "", "en")
	if r.Score != LikelyScamAt || r.Verdict != api.VerdictLikelyScam {
		t.Fatalf("%+v", r)
	}
}

func TestInformationalDoesNotScore(t *testing.T) {
	r := Score([]Signal{{Code: "qr.upi_sends_money", Weight: 10, Source: "qr"}}, api.SenderUnknownNumber, "hi")
	if r.Score != 0 || len(r.Reasons) != 1 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Reasons[0].Message, "UPI PIN") || r.Reasons[0].Message == "" {
		t.Fatalf("expected Hindi message, got %q", r.Reasons[0].Message)
	}
}

func TestSenderAdjustments(t *testing.T) {
	base := []Signal{sig("kyc.update", 30)}
	unknown := Score(base, api.SenderUnknownNumber, "en")
	saved := Score(base, api.SenderSavedContact, "en")
	if unknown.Score != 40 || saved.Score != 20 {
		t.Fatalf("unknown=%d saved=%d", unknown.Score, saved.Score)
	}
	found := false
	for _, r := range unknown.Reasons {
		found = found || r.Code == "sender.unknown_number"
	}
	if !found {
		t.Fatal("missing sender reason")
	}
	// Saved contact never pulls a decisive signal out of the scam bucket.
	if r := Score([]Signal{sig("otp.share", 45)}, api.SenderSavedContact, "en"); r.Verdict != api.VerdictLikelyScam {
		t.Fatalf("%+v", r)
	}
}

func TestReasonsOrderedAndCapped(t *testing.T) {
	var s []Signal
	for i, c := range []string{"a.x", "b.x", "c.x", "d.x", "e.x", "f.x", "g.x"} {
		s = append(s, sig(c, 5+i))
	}
	r := Score(s, "", "en")
	if len(r.Reasons) != MaxReasons || r.Reasons[0].Weight != 11 {
		t.Fatalf("%+v", r.Reasons)
	}
}

func TestNextStepsLocalizedWithCategoryAdvice(t *testing.T) {
	r := Score([]Signal{sig("qr.receive_money", 60)}, "", "kn")
	joined := strings.Join(r.NextSteps, " | ")
	if !strings.Contains(joined, "1930") || !strings.Contains(joined, "QR") || !strings.Contains(joined, "ಹಣ") {
		t.Fatalf("unexpected next steps: %s", joined)
	}
}

func TestNoUnsafeWording(t *testing.T) {
	for _, l := range []map[string]string{reportStep} {
		for _, v := range l {
			if strings.Contains(v, "100%") {
				t.Fatal("100% wording")
			}
		}
	}
	for _, byLang := range messages {
		for _, v := range byLang {
			if strings.Contains(strings.ToLower(v), "scammer") || strings.Contains(v, "100%") {
				t.Fatalf("forbidden wording: %q", v)
			}
		}
	}
}
