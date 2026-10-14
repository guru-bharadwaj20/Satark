// Package i18n holds the bot's user-facing strings in English, Hindi and
// Kannada. User-facing text must never call anything "100% safe" or call
// a person a scammer.
package i18n

import "strings"

// Supported languages, in menu order.
var Langs = []string{"en", "hi", "kn"}

// LangNames are shown on the language buttons, each in its own script.
var LangNames = map[string]string{"en": "English", "hi": "हिन्दी", "kn": "ಕನ್ನಡ"}

// Supported reports whether lang is supported.
func Supported(lang string) bool {
	_, ok := LangNames[lang]
	return ok
}

// FromTelegram maps a Telegram IETF language code to a supported language.
func FromTelegram(code string) string {
	code = strings.ToLower(code)
	base, _, _ := strings.Cut(code, "-")
	if Supported(base) {
		return base
	}
	return "en"
}

// Key identifies a string.
type Key string

// String keys.
const (
	Welcome        Key = "welcome"
	UnknownCommand Key = "unknown_command"
	ScanFailed     Key = "scan_failed"
	BadInput       Key = "bad_input"
	SendAsPhoto    Key = "send_as_photo"
	NoQRFound      Key = "no_qr_found"
	ImageTooLarge  Key = "image_too_large"
	ChooseLanguage Key = "choose_language"
	LanguageSet    Key = "language_set"
	Expired        Key = "expired"
	WhoSent        Key = "who_sent"
	SenderSaved    Key = "sender_saved_contact"
	SenderUnknown  Key = "sender_unknown_number"
	SenderBusiness Key = "sender_business"
	SenderGroup    Key = "sender_group"
	SenderSkip     Key = "sender_skip"
)

var table = map[Key]map[string]string{
	Welcome: {
		"en": "Namaste! I'm Satark.\n\nForward me a suspicious SMS or message, or send a screenshot of a QR code, and I'll tell you how risky it looks and what to do.\n\nI will never ask for your OTP, UPI PIN or password.\n\nChange language: /language",
		"hi": "नमस्ते! मैं सतर्क हूँ।\n\nकोई संदिग्ध SMS या संदेश मुझे फ़ॉरवर्ड करें, या QR कोड का स्क्रीनशॉट भेजें। मैं बताऊँगा कि यह कितना जोखिम भरा लगता है और आपको क्या करना चाहिए।\n\nमैं कभी आपका OTP, UPI PIN या पासवर्ड नहीं माँगूँगा।\n\nभाषा बदलें: /language",
		"kn": "ನಮಸ್ಕಾರ! ನಾನು ಸತರ್ಕ್.\n\nಅನುಮಾನಾಸ್ಪದ SMS ಅಥವಾ ಸಂದೇಶವನ್ನು ನನಗೆ ಫಾರ್ವರ್ಡ್ ಮಾಡಿ, ಅಥವಾ QR ಕೋಡ್‌ನ ಸ್ಕ್ರೀನ್‌ಶಾಟ್ ಕಳುಹಿಸಿ. ಅದು ಎಷ್ಟು ಅಪಾಯಕಾರಿ ಎಂದು ಮತ್ತು ನೀವು ಏನು ಮಾಡಬೇಕು ಎಂದು ತಿಳಿಸುತ್ತೇನೆ.\n\nನಾನು ಎಂದಿಗೂ ನಿಮ್ಮ OTP, UPI PIN ಅಥವಾ ಪಾಸ್‌ವರ್ಡ್ ಕೇಳುವುದಿಲ್ಲ.\n\nಭಾಷೆ ಬದಲಿಸಿ: /language",
	},
	UnknownCommand: {
		"en": "Sorry, I don't know that command. Send /help to see what I can do.",
		"hi": "माफ़ कीजिए, यह कमांड मुझे नहीं पता। मैं क्या कर सकता हूँ, यह जानने के लिए /help भेजें।",
		"kn": "ಕ್ಷಮಿಸಿ, ಈ ಆಜ್ಞೆ ನನಗೆ ತಿಳಿದಿಲ್ಲ. ನಾನು ಏನು ಮಾಡಬಲ್ಲೆ ಎಂದು ತಿಳಿಯಲು /help ಕಳುಹಿಸಿ.",
	},
	ScanFailed: {
		"en": "Sorry, I couldn't check that right now. Please try again in a minute.\n\nMeanwhile: never share your OTP, UPI PIN or password.",
		"hi": "माफ़ कीजिए, अभी जाँच नहीं हो पाई। कृपया एक मिनट बाद फिर कोशिश करें।\n\nतब तक: अपना OTP, UPI PIN या पासवर्ड किसी को न बताएं।",
		"kn": "ಕ್ಷಮಿಸಿ, ಈಗ ಪರಿಶೀಲಿಸಲು ಆಗಲಿಲ್ಲ. ದಯವಿಟ್ಟು ಒಂದು ನಿಮಿಷದ ನಂತರ ಮತ್ತೆ ಪ್ರಯತ್ನಿಸಿ.\n\nಅಲ್ಲಿಯವರೆಗೆ: ನಿಮ್ಮ OTP, UPI PIN ಅಥವಾ ಪಾಸ್‌ವರ್ಡ್ ಯಾರಿಗೂ ಹೇಳಬೇಡಿ.",
	},
	BadInput: {
		"en": "Sorry, I couldn't read that. Please send the message text, or a clear screenshot of the QR code.",
		"hi": "माफ़ कीजिए, मैं इसे पढ़ नहीं पाया। कृपया संदेश का टेक्स्ट या QR कोड का साफ़ स्क्रीनशॉट भेजें।",
		"kn": "ಕ್ಷಮಿಸಿ, ಅದನ್ನು ಓದಲು ಆಗಲಿಲ್ಲ. ದಯವಿಟ್ಟು ಸಂದೇಶದ ಪಠ್ಯ ಅಥವಾ QR ಕೋಡ್‌ನ ಸ್ಪಷ್ಟ ಸ್ಕ್ರೀನ್‌ಶಾಟ್ ಕಳುಹಿಸಿ.",
	},
	SendAsPhoto: {
		"en": "Please send the QR code as a photo or an image file (PNG or JPEG).",
		"hi": "कृपया QR कोड को फ़ोटो या इमेज फ़ाइल (PNG या JPEG) के रूप में भेजें।",
		"kn": "ದಯವಿಟ್ಟು QR ಕೋಡ್ ಅನ್ನು ಫೋಟೋ ಅಥವಾ ಚಿತ್ರ ಫೈಲ್ (PNG ಅಥವಾ JPEG) ಆಗಿ ಕಳುಹಿಸಿ.",
	},
	NoQRFound: {
		"en": "I couldn't find a QR code in that image. Try a sharper, closer screenshot.",
		"hi": "इस तस्वीर में मुझे कोई QR कोड नहीं मिला। ज़्यादा साफ़ और पास से लिया गया स्क्रीनशॉट भेजें।",
		"kn": "ಈ ಚಿತ್ರದಲ್ಲಿ QR ಕೋಡ್ ಕಂಡುಬಂದಿಲ್ಲ. ಹೆಚ್ಚು ಸ್ಪಷ್ಟವಾದ, ಹತ್ತಿರದ ಸ್ಕ್ರೀನ್‌ಶಾಟ್ ಕಳುಹಿಸಿ.",
	},
	ImageTooLarge: {
		"en": "That image is too large. Please send a screenshot under 5 MB.",
		"hi": "यह तस्वीर बहुत बड़ी है। कृपया 5 MB से छोटा स्क्रीनशॉट भेजें।",
		"kn": "ಈ ಚಿತ್ರ ತುಂಬಾ ದೊಡ್ಡದಾಗಿದೆ. ದಯವಿಟ್ಟು 5 MB ಗಿಂತ ಚಿಕ್ಕ ಸ್ಕ್ರೀನ್‌ಶಾಟ್ ಕಳುಹಿಸಿ.",
	},
	ChooseLanguage: {
		"en": "Choose your language:",
		"hi": "अपनी भाषा चुनें:",
		"kn": "ನಿಮ್ಮ ಭಾಷೆಯನ್ನು ಆಯ್ಕೆಮಾಡಿ:",
	},
	LanguageSet: {
		"en": "Language set to English.",
		"hi": "भाषा हिन्दी कर दी गई है।",
		"kn": "ಭಾಷೆಯನ್ನು ಕನ್ನಡಕ್ಕೆ ಬದಲಾಯಿಸಲಾಗಿದೆ.",
	},
	Expired: {
		"en": "That request has expired. Please forward the message again.",
		"hi": "यह अनुरोध समाप्त हो गया है। कृपया संदेश फिर से फ़ॉरवर्ड करें।",
		"kn": "ಈ ವಿನಂತಿಯ ಅವಧಿ ಮುಗಿದಿದೆ. ದಯವಿಟ್ಟು ಸಂದೇಶವನ್ನು ಮತ್ತೆ ಫಾರ್ವರ್ಡ್ ಮಾಡಿ.",
	},
}

func init() {
	for k, v := range map[Key]map[string]string{
		WhoSent: {
			"en": "Who sent this?",
			"hi": "यह किसने भेजा?",
			"kn": "ಇದನ್ನು ಯಾರು ಕಳುಹಿಸಿದರು?",
		},
		SenderSaved:    {"en": "A saved contact", "hi": "सेव किया हुआ संपर्क", "kn": "ಉಳಿಸಿದ ಸಂಪರ್ಕ"},
		SenderUnknown:  {"en": "Unknown number", "hi": "अनजान नंबर", "kn": "ಅಪರಿಚಿತ ಸಂಖ್ಯೆ"},
		SenderBusiness: {"en": "Bank / company SMS", "hi": "बैंक / कंपनी का SMS", "kn": "ಬ್ಯಾಂಕ್ / ಕಂಪನಿ SMS"},
		SenderGroup:    {"en": "A group chat", "hi": "कोई ग्रुप चैट", "kn": "ಗುಂಪು ಚಾಟ್"},
		SenderSkip:     {"en": "Not sure, just check", "hi": "पता नहीं, बस जाँचें", "kn": "ಗೊತ್ತಿಲ್ಲ, ಪರಿಶೀಲಿಸಿ"},
	} {
		table[k] = v
	}
}

// T returns the string for key in lang, falling back to English.
func T(lang string, key Key) string {
	m := table[key]
	if s := m[lang]; s != "" {
		return s
	}
	return m["en"]
}
