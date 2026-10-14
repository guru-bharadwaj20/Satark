package trends

import (
	"strings"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

func TestRound(t *testing.T) {
	for in, want := range map[int]int{12: 10, 13: 15, 97: 95, 104: 100, 106: 110} {
		if got := Round(in); got != want {
			t.Errorf("Round(%d)=%d want %d", in, got, want)
		}
	}
}

func TestRenderSuppressesSmallGroups(t *testing.T) {
	month := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows := []store.TrendRow{
		{Kind: "upi", Category: "upi_collect", Reports: 120, Reporters: 80},
		{Kind: "phone", Category: "kyc_fraud", Reports: 60, Reporters: 40},
		{Kind: "domain", Category: "loan_app", Reports: 4, Reporters: 3}, // below k
	}
	prev := []store.TrendRow{{Kind: "upi", Category: "upi_collect", Reports: 60, Reporters: 50}}
	out := Render(month, rows, prev)
	for _, want := range []string{"September 2026", "UPI collect requests | ~120 | up 100%", "KYC / account-block threats | ~60 | new", "1930"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Loan apps") {
		t.Error("group below k-anonymity floor must not be listed")
	}
	if strings.Contains(strings.ToLower(out), "scammer") {
		t.Error("must not label anyone")
	}
}

func TestRenderTooFew(t *testing.T) {
	out := Render(time.Now(), []store.TrendRow{{Kind: "upi", Category: "upi_collect", Reports: 5, Reporters: 4}}, nil)
	if !strings.Contains(out, "Too few reports") || strings.Contains(out, "## By scam type") {
		t.Fatalf("got:\n%s", out)
	}
}
