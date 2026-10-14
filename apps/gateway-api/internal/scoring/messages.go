package scoring

import "github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"

// messages localizes reason codes produced outside text-analyzer (which
// already returns localized rule reasons).
var messages = map[string]map[string]string{
	"qr.upi_sends_money": {
		"en": "Scanning this code and entering your UPI PIN sends money from your account",
		"hi": "यह कोड स्कैन करके UPI PIN डालने से आपके खाते से पैसे जाते हैं",
		"kn": "ಈ ಕೋಡ್ ಸ್ಕ್ಯಾನ್ ಮಾಡಿ UPI PIN ಹಾಕಿದರೆ ನಿಮ್ಮ ಖಾತೆಯಿಂದ ಹಣ ಹೋಗುತ್ತದೆ",
	},
	"qr.receive_money": {
		"en": "This QR is shown as a way to receive money, but scanning a UPI QR always sends money",
		"hi": "यह QR पैसे पाने के तरीके के रूप में दिखाया गया है, लेकिन UPI QR स्कैन करने से हमेशा पैसे जाते हैं",
		"kn": "ಈ QR ಅನ್ನು ಹಣ ಪಡೆಯುವ ಮಾರ್ಗವೆಂದು ತೋರಿಸಲಾಗಿದೆ, ಆದರೆ UPI QR ಸ್ಕ್ಯಾನ್ ಮಾಡಿದರೆ ಯಾವಾಗಲೂ ಹಣ ಹೋಗುತ್ತದೆ",
	},
	"qr.high_amount": {
		"en": "The code is pre-filled with a large amount",
		"hi": "कोड में पहले से बड़ी राशि भरी हुई है",
		"kn": "ಕೋಡ್‌ನಲ್ಲಿ ಮೊದಲೇ ದೊಡ್ಡ ಮೊತ್ತ ತುಂಬಲಾಗಿದೆ",
	},
	"qr.malformed": {
		"en": "The payment details in this code are malformed",
		"hi": "इस कोड में भुगतान की जानकारी सही नहीं है",
		"kn": "ಈ ಕೋಡ್‌ನಲ್ಲಿನ ಪಾವತಿ ವಿವರಗಳು ಸರಿಯಾಗಿಲ್ಲ",
	},
	"qr.foreign_currency": {
		"en": "The code asks for a non-rupee payment",
		"hi": "कोड रुपये के अलावा किसी और मुद्रा में भुगतान माँगता है",
		"kn": "ಕೋಡ್ ರೂಪಾಯಿ ಅಲ್ಲದ ಕರೆನ್ಸಿಯಲ್ಲಿ ಪಾವತಿ ಕೇಳುತ್ತದೆ",
	},
	"sender.unknown_number": {
		"en": "It came from a number you don't know",
		"hi": "यह किसी अनजान नंबर से आया है",
		"kn": "ಇದು ನಿಮಗೆ ಗೊತ್ತಿಲ್ಲದ ಸಂಖ್ಯೆಯಿಂದ ಬಂದಿದೆ",
	},
	"sender.group": {
		"en": "It was forwarded in a group, where messages are rarely checked",
		"hi": "यह किसी ग्रुप में फ़ॉरवर्ड किया गया है, जहाँ संदेशों की जाँच कम ही होती है",
		"kn": "ಇದು ಗುಂಪಿನಲ್ಲಿ ಫಾರ್ವರ್ಡ್ ಆಗಿದೆ, ಅಲ್ಲಿ ಸಂದೇಶಗಳನ್ನು ವಿರಳವಾಗಿ ಪರಿಶೀಲಿಸಲಾಗುತ್ತದೆ",
	},
}

func localize(code, fallback, lang string) string {
	if m, ok := messages[code]; ok {
		if s := m[lang]; s != "" {
			return s
		}
		if fallback == "" {
			return m[api.LangEnglish]
		}
	}
	return fallback
}

type steps map[string][]string

var reportStep = map[string]string{
	"en": "If you lost money, call 1930 right away or report at cybercrime.gov.in. Report the message on Sanchar Saathi (Chakshu)",
	"hi": "अगर पैसे गए हैं तो तुरंत 1930 पर कॉल करें या cybercrime.gov.in पर शिकायत करें। संदेश की शिकायत संचार साथी (चक्षु) पर करें",
	"kn": "ಹಣ ಕಳೆದುಕೊಂಡಿದ್ದರೆ ತಕ್ಷಣ 1930 ಗೆ ಕರೆ ಮಾಡಿ ಅಥವಾ cybercrime.gov.in ನಲ್ಲಿ ದೂರು ನೀಡಿ. ಸಂದೇಶವನ್ನು ಸಂಚಾರ ಸಾಥಿ (ಚಕ್ಷು) ನಲ್ಲಿ ವರದಿ ಮಾಡಿ",
}

var verdictSteps = map[string]steps{
	api.VerdictLikelyScam: {
		"en": {"Don't reply, call the number, tap any link or scan any code in it", "Never share your OTP, UPI PIN or password with anyone"},
		"hi": {"इसका जवाब न दें, नंबर पर कॉल न करें, कोई लिंक न खोलें और कोई कोड स्कैन न करें", "अपना OTP, UPI PIN या पासवर्ड किसी को न बताएं"},
		"kn": {"ಇದಕ್ಕೆ ಉತ್ತರಿಸಬೇಡಿ, ಸಂಖ್ಯೆಗೆ ಕರೆ ಮಾಡಬೇಡಿ, ಯಾವುದೇ ಲಿಂಕ್ ತೆರೆಯಬೇಡಿ ಅಥವಾ ಕೋಡ್ ಸ್ಕ್ಯಾನ್ ಮಾಡಬೇಡಿ", "ನಿಮ್ಮ OTP, UPI PIN ಅಥವಾ ಪಾಸ್‌ವರ್ಡ್ ಯಾರಿಗೂ ಹೇಳಬೇಡಿ"},
	},
	api.VerdictSuspicious: {
		"en": {"Don't act on it yet. Check directly with the company using the number on its official website or app", "Never share your OTP, UPI PIN or password with anyone"},
		"hi": {"अभी कुछ न करें। कंपनी की आधिकारिक वेबसाइट या ऐप पर दिए नंबर से सीधे पुष्टि करें", "अपना OTP, UPI PIN या पासवर्ड किसी को न बताएं"},
		"kn": {"ಈಗಲೇ ಏನೂ ಮಾಡಬೇಡಿ. ಕಂಪನಿಯ ಅಧಿಕೃತ ವೆಬ್‌ಸೈಟ್ ಅಥವಾ ಆಪ್‌ನಲ್ಲಿರುವ ಸಂಖ್ಯೆಯ ಮೂಲಕ ನೇರವಾಗಿ ಖಚಿತಪಡಿಸಿಕೊಳ್ಳಿ", "ನಿಮ್ಮ OTP, UPI PIN ಅಥವಾ ಪಾಸ್‌ವರ್ಡ್ ಯಾರಿಗೂ ಹೇಳಬೇಡಿ"},
	},
	api.VerdictNoRedFlags: {
		"en": {"No red flags found, but stay careful: if anyone asks for an OTP, PIN or payment, stop and check"},
		"hi": {"कोई खतरे का संकेत नहीं मिला, फिर भी सावधान रहें: अगर कोई OTP, PIN या भुगतान माँगे तो रुकें और जाँच करें"},
		"kn": {"ಯಾವುದೇ ಅಪಾಯದ ಸೂಚನೆ ಕಂಡುಬಂದಿಲ್ಲ, ಆದರೂ ಎಚ್ಚರವಾಗಿರಿ: ಯಾರಾದರೂ OTP, PIN ಅಥವಾ ಪಾವತಿ ಕೇಳಿದರೆ ನಿಲ್ಲಿಸಿ ಪರಿಶೀಲಿಸಿ"},
	},
}

var categorySteps = map[string]steps{
	"utility": {
		"en": {"Check your bill on the official website or app of your electricity provider"},
		"hi": {"अपना बिल बिजली कंपनी की आधिकारिक वेबसाइट या ऐप पर जाँचें"},
		"kn": {"ನಿಮ್ಮ ಬಿಲ್ ಅನ್ನು ವಿದ್ಯುತ್ ಕಂಪನಿಯ ಅಧಿಕೃತ ವೆಬ್‌ಸೈಟ್ ಅಥವಾ ಆಪ್‌ನಲ್ಲಿ ಪರಿಶೀಲಿಸಿ"},
	},
	"kyc": {
		"en": {"For KYC, visit your bank branch or use the bank's official app only"},
		"hi": {"KYC के लिए केवल अपनी बैंक शाखा जाएं या बैंक का आधिकारिक ऐप इस्तेमाल करें"},
		"kn": {"KYC ಗಾಗಿ ನಿಮ್ಮ ಬ್ಯಾಂಕ್ ಶಾಖೆಗೆ ಮಾತ್ರ ಹೋಗಿ ಅಥವಾ ಬ್ಯಾಂಕಿನ ಅಧಿಕೃತ ಆಪ್ ಮಾತ್ರ ಬಳಸಿ"},
	},
	"remote_access": {
		"en": {"If you installed a screen-sharing app, uninstall it and call your bank"},
		"hi": {"अगर आपने स्क्रीन-शेयरिंग ऐप इंस्टॉल किया है, तो उसे हटाएं और अपने बैंक को कॉल करें"},
		"kn": {"ನೀವು ಸ್ಕ್ರೀನ್-ಶೇರಿಂಗ್ ಆಪ್ ಇನ್‌ಸ್ಟಾಲ್ ಮಾಡಿದ್ದರೆ, ಅದನ್ನು ತೆಗೆದುಹಾಕಿ ಮತ್ತು ನಿಮ್ಮ ಬ್ಯಾಂಕಿಗೆ ಕರೆ ಮಾಡಿ"},
	},
	"qr": {
		"en": {"To receive money you never need to scan a QR code or enter your UPI PIN"},
		"hi": {"पैसे पाने के लिए कभी QR कोड स्कैन करने या UPI PIN डालने की ज़रूरत नहीं होती"},
		"kn": {"ಹಣ ಪಡೆಯಲು QR ಕೋಡ್ ಸ್ಕ್ಯಾನ್ ಮಾಡುವ ಅಥವಾ UPI PIN ಹಾಕುವ ಅಗತ್ಯವೇ ಇಲ್ಲ"},
	},
	"task_job": {
		"en": {"Real jobs never ask you to pay a deposit or 'prepaid task' fee"},
		"hi": {"असली नौकरी में कभी जमा राशि या 'प्रीपेड टास्क' फीस नहीं माँगी जाती"},
		"kn": {"ನಿಜವಾದ ಉದ್ಯೋಗಗಳು ಎಂದಿಗೂ ಠೇವಣಿ ಅಥವಾ 'ಪ್ರೀಪೇಯ್ಡ್ ಟಾಸ್ಕ್' ಶುಲ್ಕ ಕೇಳುವುದಿಲ್ಲ"},
	},
}

func nextSteps(verdict string, byCat map[string][]int, lang string) []string {
	pick := func(s steps) []string {
		if v := s[lang]; len(v) > 0 {
			return v
		}
		return s[api.LangEnglish]
	}
	out := append([]string{}, pick(verdictSteps[verdict])...)
	if verdict != api.VerdictNoRedFlags {
		for _, cat := range []string{"qr", "remote_access", "kyc", "utility", "task_job"} {
			if _, ok := byCat[cat]; ok {
				out = append(out, pick(categorySteps[cat])...)
			}
		}
		r := reportStep[lang]
		if r == "" {
			r = reportStep[api.LangEnglish]
		}
		out = append(out, r)
	}
	return out
}
