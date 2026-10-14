package reply

import (
	"strings"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/gateway"
)

var scam = gateway.ScanResponse{
	ScanID: "abc123", Verdict: "likely_scam", RiskScore: 91,
	Reasons: []gateway.Reason{
		{Message: "It gives a deadline of tonight to rush you"},
		{Message: "It asks you to call a personal mobile number"},
	},
	NextSteps: []string{"Don't call the number", "Report it on 1930"},
}

func TestRenderScam(t *testing.T) {
	out := Render(scam, "en")
	for _, want := range []string{"🔴 Likely scam (risk 91/100)", "• It gives a deadline", "What to do:", "• Report it on 1930", "not a guarantee"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderLocalizedAndNeverSafe(t *testing.T) {
	r := gateway.ScanResponse{Verdict: "no_red_flags", RiskScore: 4}
	for _, lang := range []string{"en", "hi", "kn"} {
		out := Render(r, lang)
		if strings.Contains(strings.ToLower(out), "safe") || strings.Contains(out, "100%") {
			t.Fatalf("%s: forbidden wording in %q", lang, out)
		}
	}
	if !strings.Contains(Render(r, "hi"), "कोई खतरे का संकेत नहीं मिला") {
		t.Fatal("Hindi verdict missing")
	}
}

func TestRenderQRAndPartial(t *testing.T) {
	r := gateway.ScanResponse{Verdict: "suspicious", RiskScore: 50, Partial: true, QR: []gateway.QRResult{
		{Kind: "upi", UPI: &gateway.UPIIntent{Summary: "This SENDS ₹14,999 FROM your account to xy***@examplebank"}},
		{Kind: "url", URL: "https://bad.example.com/x"},
	}}
	out := Render(r, "en")
	if !strings.Contains(out, "💸 This SENDS ₹14,999") || !strings.Contains(out, "bad[.]example[.]com") || !strings.Contains(out, "⏳") {
		t.Fatalf("unexpected:\n%s", out)
	}
}

func TestReasonsCapped(t *testing.T) {
	r := gateway.ScanResponse{Verdict: "suspicious"}
	for range 10 {
		r.Reasons = append(r.Reasons, gateway.Reason{Message: "x"})
	}
	if n := strings.Count(Render(r, "en"), "• x"); n != MaxReasons {
		t.Fatalf("got %d reasons", n)
	}
}

func TestKeyboard(t *testing.T) {
	kb := Keyboard(scam, "kn")
	if len(kb.InlineKeyboard) != 2 || kb.InlineKeyboard[1][0].URL != CybercrimeURL {
		t.Fatalf("%+v", kb)
	}
	id, up, ok := ParseFeedback(kb.InlineKeyboard[0][0].CallbackData)
	if !ok || !up || id != "abc123" {
		t.Fatal("feedback round trip failed")
	}
	if kb := Keyboard(gateway.ScanResponse{Verdict: "no_red_flags", ScanID: "x"}, "en"); len(kb.InlineKeyboard) != 1 {
		t.Fatal("no report link expected for no_red_flags")
	}
}

func TestParseFeedbackRejectsGarbage(t *testing.T) {
	for _, d := range []string{"fb:", "fb:x", "fb:x:maybe", "lang:hi", "fb::up"} {
		if _, _, ok := ParseFeedback(d); ok {
			t.Errorf("accepted %q", d)
		}
	}
}
