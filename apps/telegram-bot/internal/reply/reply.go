// Package reply renders scan results as Telegram messages.
//
// Messages are sent as plain text (no parse mode), so content coming from
// the analyzers never needs escaping and can never inject formatting.
package reply

import (
	"fmt"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/gateway"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

// MaxReasons shown in a reply.
const MaxReasons = 4

// CybercrimeURL is the national cybercrime reporting portal.
const CybercrimeURL = "https://cybercrime.gov.in"

// FeedbackPrefix starts feedback callback data: fb:<scan_id>:<up|down>.
const FeedbackPrefix = "fb:"

type verdictText struct{ icon, label string }

var verdicts = map[string]map[string]verdictText{
	"likely_scam": {
		"en": {"🔴", "Likely scam"},
		"hi": {"🔴", "संभवतः धोखाधड़ी"},
		"kn": {"🔴", "ಬಹುಶಃ ವಂಚನೆ"},
	},
	"suspicious": {
		"en": {"🟠", "Suspicious"},
		"hi": {"🟠", "संदिग्ध"},
		"kn": {"🟠", "ಅನುಮಾನಾಸ್ಪದ"},
	},
	"no_red_flags": {
		"en": {"🟢", "No red flags found"},
		"hi": {"🟢", "कोई खतरे का संकेत नहीं मिला"},
		"kn": {"🟢", "ಯಾವುದೇ ಅಪಾಯದ ಸೂಚನೆ ಕಂಡುಬಂದಿಲ್ಲ"},
	},
}

var labels = map[string]map[string]string{
	"risk":       {"en": "risk", "hi": "जोखिम", "kn": "ಅಪಾಯ"},
	"whatToDo":   {"en": "What to do:", "hi": "क्या करें:", "kn": "ಏನು ಮಾಡಬೇಕು:"},
	"partial":    {"en": "Some checks didn't finish in time, so this result may be incomplete.", "hi": "कुछ जाँचें समय पर पूरी नहीं हुईं, इसलिए यह नतीजा अधूरा हो सकता है।", "kn": "ಕೆಲವು ಪರಿಶೀಲನೆಗಳು ಸಮಯಕ್ಕೆ ಮುಗಿಯಲಿಲ್ಲ, ಆದ್ದರಿಂದ ಈ ಫಲಿತಾಂಶ ಅಪೂರ್ಣವಾಗಿರಬಹುದು."},
	"disclaimer": {"en": "Satark gives a risk assessment, not a guarantee.", "hi": "सतर्क जोखिम का आकलन देता है, गारंटी नहीं।", "kn": "ಸತರ್ಕ್ ಅಪಾಯದ ಅಂದಾಜು ನೀಡುತ್ತದೆ, ಖಾತರಿ ಅಲ್ಲ."},
	"helpful":    {"en": "👍 Helpful", "hi": "👍 मददगार", "kn": "👍 ಉಪಯುಕ್ತ"},
	"wrong":      {"en": "👎 Wrong", "hi": "👎 गलत", "kn": "👎 ತಪ್ಪು"},
	"report":     {"en": "Report on cybercrime.gov.in", "hi": "cybercrime.gov.in पर शिकायत करें", "kn": "cybercrime.gov.in ನಲ್ಲಿ ದೂರು ನೀಡಿ"},
	"thanks":     {"en": "Thanks for the feedback!", "hi": "प्रतिक्रिया के लिए धन्यवाद!", "kn": "ಪ್ರತಿಕ್ರಿಯೆಗೆ ಧನ್ಯವಾದಗಳು!"},
}

// Label returns a localized label, falling back to English.
func Label(lang, key string) string {
	m := labels[key]
	if s := m[lang]; s != "" {
		return s
	}
	return m["en"]
}

// Render formats a scan response.
func Render(r gateway.ScanResponse, lang string) string {
	v, ok := verdicts[r.Verdict][lang]
	if !ok {
		v = verdicts["suspicious"]["en"]
		if en, ok := verdicts[r.Verdict]["en"]; ok {
			v = en
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s (%s %d/100)\n", v.icon, v.label, Label(lang, "risk"), r.RiskScore)

	for _, q := range r.QR {
		switch {
		case q.UPI != nil && q.UPI.Summary != "":
			fmt.Fprintf(&sb, "\n💸 %s\n", q.UPI.Summary)
		case q.URL != "":
			fmt.Fprintf(&sb, "\n🔗 %s\n", defang(q.URL))
		}
	}

	if len(r.Reasons) > 0 {
		sb.WriteString("\n")
		for i, reason := range r.Reasons {
			if i == MaxReasons {
				break
			}
			fmt.Fprintf(&sb, "• %s\n", reason.Message)
		}
	}

	if len(r.NextSteps) > 0 {
		fmt.Fprintf(&sb, "\n%s\n", Label(lang, "whatToDo"))
		for _, s := range r.NextSteps {
			fmt.Fprintf(&sb, "• %s\n", s)
		}
	}
	if r.Partial {
		fmt.Fprintf(&sb, "\n⏳ %s\n", Label(lang, "partial"))
	}
	fmt.Fprintf(&sb, "\n%s", Label(lang, "disclaimer"))
	return sb.String()
}

// defang stops Telegram from turning a suspicious URL into a tappable link.
func defang(u string) string {
	u = strings.Replace(u, "://", "[://]", 1)
	return strings.ReplaceAll(u, ".", "[.]")
}

// Keyboard returns feedback buttons, plus a report link for risky verdicts.
func Keyboard(r gateway.ScanResponse, lang string) *telegram.InlineKeyboardMarkup {
	rows := [][]telegram.InlineKeyboardButton{}
	if r.ScanID != "" && len(r.ScanID) <= 48 {
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: Label(lang, "helpful"), CallbackData: FeedbackPrefix + r.ScanID + ":up"},
			{Text: Label(lang, "wrong"), CallbackData: FeedbackPrefix + r.ScanID + ":down"},
		})
	}
	if r.Verdict == "likely_scam" || r.Verdict == "suspicious" {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: Label(lang, "report"), URL: CybercrimeURL}})
	}
	if len(rows) == 0 {
		return nil
	}
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// ParseFeedback parses feedback callback data.
func ParseFeedback(data string) (scanID string, helpful bool, ok bool) {
	rest, found := strings.CutPrefix(data, FeedbackPrefix)
	if !found {
		return "", false, false
	}
	id, vote, found := strings.Cut(rest, ":")
	if !found || id == "" || (vote != "up" && vote != "down") {
		return "", false, false
	}
	return id, vote == "up", true
}
