package sandboxupdate

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// Result is the scan.results.<id> message published by linkscan-operator.
type Result struct {
	ID            string            `json:"id"`
	URL           string            `json:"url"`
	Phase         string            `json:"phase"`
	Message       string            `json:"message,omitempty"`
	FinalURL      string            `json:"final_url,omitempty"`
	RedirectChain []string          `json:"redirect_chain,omitempty"`
	Title         string            `json:"title,omitempty"`
	Forms         Forms             `json:"forms"`
	Downloads     []string          `json:"downloads,omitempty"`
	ScreenshotKey string            `json:"screenshot_key,omitempty"`
	Correlation   map[string]string `json:"correlation,omitempty"`
}

// Forms are the sensitive fields found on the final page.
type Forms struct {
	Password   bool `json:"password"`
	OTP        bool `json:"otp"`
	CardNumber bool `json:"card_number"`
}

// Correlation keys set by telegram-bot when it asks url-intel for a scan.
const (
	KeyChatID    = "telegram_chat_id"
	KeyMessageID = "telegram_message_id"
	KeyLang      = "lang"
)

type messages struct {
	header, failed, timedOut, redirects, finalSite, asks, password, otp, card,
	download, noFields, caution string
}

var catalog = map[string]messages{
	"en": {
		header:    "🔎 <b>Sandbox check</b>",
		failed:    "The sandbox could not open this link, so it was not checked there.",
		timedOut:  "The page took too long to load in the sandbox.",
		redirects: "Redirects: %d",
		finalSite: "Ends at: <code>%s</code>",
		asks:      "⚠️ The page asks for:",
		password:  "password",
		otp:       "OTP / PIN",
		card:      "card number",
		download:  "⚠️ The page tries to download a file: <code>%s</code>. Do not install apps from links.",
		noFields:  "No password, OTP or card fields were seen. This is not a guarantee: stay careful with this link.",
		caution:   "Never share an OTP, PIN or card details through a link.",
	},
	"hi": {
		header:    "🔎 <b>सैंडबॉक्स जाँच</b>",
		failed:    "सैंडबॉक्स यह लिंक नहीं खोल पाया, इसलिए वहाँ इसकी जाँच नहीं हुई।",
		timedOut:  "सैंडबॉक्स में पेज खुलने में बहुत समय लगा।",
		redirects: "रीडायरेक्ट: %d",
		finalSite: "अंतिम पता: <code>%s</code>",
		asks:      "⚠️ यह पेज माँगता है:",
		password:  "पासवर्ड",
		otp:       "ओटीपी / पिन",
		card:      "कार्ड नंबर",
		download:  "⚠️ यह पेज एक फ़ाइल डाउनलोड कराने की कोशिश करता है: <code>%s</code>। लिंक से ऐप इंस्टॉल न करें।",
		noFields:  "पासवर्ड, ओटीपी या कार्ड का कोई फ़ील्ड नहीं दिखा। इसका मतलब यह नहीं कि लिंक सुरक्षित है।",
		caution:   "किसी लिंक पर कभी ओटीपी, पिन या कार्ड की जानकारी न दें।",
	},
	"kn": {
		header:    "🔎 <b>ಸ್ಯಾಂಡ್‌ಬಾಕ್ಸ್ ಪರಿಶೀಲನೆ</b>",
		failed:    "ಸ್ಯಾಂಡ್‌ಬಾಕ್ಸ್ ಈ ಲಿಂಕ್ ತೆರೆಯಲು ಸಾಧ್ಯವಾಗಲಿಲ್ಲ, ಆದ್ದರಿಂದ ಅಲ್ಲಿ ಪರಿಶೀಲಿಸಲಾಗಿಲ್ಲ.",
		timedOut:  "ಸ್ಯಾಂಡ್‌ಬಾಕ್ಸ್‌ನಲ್ಲಿ ಪುಟ ತೆರೆಯಲು ತುಂಬಾ ಸಮಯ ಹಿಡಿಯಿತು.",
		redirects: "ಮರುನಿರ್ದೇಶನಗಳು: %d",
		finalSite: "ಕೊನೆಯ ವಿಳಾಸ: <code>%s</code>",
		asks:      "⚠️ ಈ ಪುಟ ಕೇಳುತ್ತದೆ:",
		password:  "ಪಾಸ್‌ವರ್ಡ್",
		otp:       "ಒಟಿಪಿ / ಪಿನ್",
		card:      "ಕಾರ್ಡ್ ಸಂಖ್ಯೆ",
		download:  "⚠️ ಈ ಪುಟ ಫೈಲ್ ಡೌನ್‌ಲೋಡ್ ಮಾಡಲು ಪ್ರಯತ್ನಿಸುತ್ತದೆ: <code>%s</code>. ಲಿಂಕ್‌ಗಳಿಂದ ಆ್ಯಪ್ ಇನ್‌ಸ್ಟಾಲ್ ಮಾಡಬೇಡಿ.",
		noFields:  "ಪಾಸ್‌ವರ್ಡ್, ಒಟಿಪಿ ಅಥವಾ ಕಾರ್ಡ್ ಫೀಲ್ಡ್ ಕಾಣಲಿಲ್ಲ. ಇದರರ್ಥ ಲಿಂಕ್ ಸುರಕ್ಷಿತ ಎಂದಲ್ಲ.",
		caution:   "ಯಾವುದೇ ಲಿಂಕ್‌ನಲ್ಲಿ ಒಟಿಪಿ, ಪಿನ್ ಅಥವಾ ಕಾರ್ಡ್ ವಿವರ ಹಂಚಿಕೊಳ್ಳಬೇಡಿ.",
	},
}

// hostOf returns only the host so the reply never re-links the page.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Hostname()
}

// FormatSection renders the sandbox findings as Telegram HTML.
func FormatSection(r Result) string {
	t, ok := catalog[r.Correlation[KeyLang]]
	if !ok {
		t = catalog["en"]
	}
	var b strings.Builder
	b.WriteString(t.header)
	b.WriteString("\n")
	switch r.Phase {
	case "Succeeded":
	case "TimedOut":
		b.WriteString(t.timedOut + "\n" + t.caution)
		return b.String()
	default:
		b.WriteString(t.failed + "\n" + t.caution)
		return b.String()
	}

	if hops := len(r.RedirectChain) - 1; hops > 0 {
		b.WriteString(fmt.Sprintf(t.redirects, hops) + "\n")
	}
	if r.FinalURL != "" {
		b.WriteString(fmt.Sprintf(t.finalSite, html.EscapeString(hostOf(r.FinalURL))) + "\n")
	}
	var asks []string
	if r.Forms.Password {
		asks = append(asks, t.password)
	}
	if r.Forms.OTP {
		asks = append(asks, t.otp)
	}
	if r.Forms.CardNumber {
		asks = append(asks, t.card)
	}
	if len(asks) > 0 {
		b.WriteString(t.asks + " <b>" + strings.Join(asks, ", ") + "</b>\n")
	}
	if len(r.Downloads) > 0 {
		b.WriteString(fmt.Sprintf(t.download, html.EscapeString(r.Downloads[0])) + "\n")
	}
	if len(asks) == 0 && len(r.Downloads) == 0 {
		b.WriteString(t.noFields + "\n")
	}
	b.WriteString(t.caution)
	return b.String()
}

// Compose appends the sandbox section to the bot's original reply.
func Compose(original string, r Result) string {
	section := FormatSection(r)
	if strings.TrimSpace(original) == "" {
		return section
	}
	return strings.TrimRight(original, "\n") + "\n\n" + section
}
