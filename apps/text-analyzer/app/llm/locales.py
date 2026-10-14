# ruff: noqa: E501  (translated sentences are kept on one line for reviewers)
"""Reviewed explanation templates in English, Hindi and Kannada.

Used when the LLM is unavailable (rules-only answers) and when the LLM's
explanation is not in the requested script. Wording follows the product
rules: never "100% safe", never call a person a scammer.
"""

from __future__ import annotations

from app.llm.schema import Category

C = Category

EXPLANATIONS: dict[str, dict[Category, str]] = {
    "en": {
        C.KYC_BANKING: "This looks like a fake bank or KYC alert. Banks never ask you to update KYC through a link or call in a message.",
        C.UTILITY_BILL: "This looks like a fake disconnection threat. Power and gas companies do not ask you to call a personal mobile number.",
        C.OTP_PIN: "This message is trying to get your OTP, PIN or password. No real bank, wallet or government office ever asks for these.",
        C.REMOTE_ACCESS: "This message asks you to install an app that lets someone else control your phone. That is a common way money is stolen.",
        C.LOTTERY_PRIZE: "This looks like a fake prize or lottery. You cannot win a draw you never entered, and real prizes never need a fee.",
        C.TASK_JOB: "This looks like a fake task or part-time job offer. These usually ask you to pay or invest before you can withdraw anything.",
        C.DELIVERY_PARCEL: "This looks like a fake parcel or customs notice. Check the tracking number only on the courier's official website or app.",
        C.IMPERSONATION_AUTHORITY: "This message pretends to be from the police, a court or a government agency. They never demand money or video calls to settle a case.",
        C.INVESTMENT: "This looks like an investment scam. Guaranteed or very high returns are a warning sign.",
        C.OTHER_SCAM: "This message shows several signs of a scam.",
        C.LEGITIMATE: "No red flags found. Stay careful: never share an OTP, PIN or password with anyone.",
    },
    "hi": {
        C.KYC_BANKING: "यह नकली बैंक या KYC संदेश लगता है। बैंक कभी भी लिंक या कॉल के ज़रिए KYC अपडेट करने को नहीं कहते।",
        C.UTILITY_BILL: "यह बिजली या गैस कनेक्शन काटने की नकली धमकी लगती है। कंपनियाँ किसी निजी मोबाइल नंबर पर कॉल करने को नहीं कहतीं।",
        C.OTP_PIN: "यह संदेश आपका OTP, PIN या पासवर्ड पाने की कोशिश कर रहा है। कोई भी असली बैंक या सरकारी दफ़्तर इन्हें कभी नहीं माँगता।",
        C.REMOTE_ACCESS: "यह संदेश ऐसा ऐप इंस्टॉल करने को कहता है जिससे कोई और आपका फ़ोन चला सकता है। पैसे चोरी करने का यह आम तरीका है।",
        C.LOTTERY_PRIZE: "यह नकली इनाम या लॉटरी लगती है। जिस ड्रॉ में आपने भाग नहीं लिया, उसे आप नहीं जीत सकते, और असली इनाम के लिए फ़ीस नहीं लगती।",
        C.TASK_JOB: "यह नकली टास्क या पार्ट-टाइम नौकरी का ऑफ़र लगता है। इनमें पैसे निकालने से पहले भुगतान या निवेश करवाया जाता है।",
        C.DELIVERY_PARCEL: "यह नकली पार्सल या कस्टम्स सूचना लगती है। ट्रैकिंग नंबर केवल कूरियर की आधिकारिक वेबसाइट या ऐप पर जाँचें।",
        C.IMPERSONATION_AUTHORITY: "यह संदेश पुलिस, अदालत या सरकारी एजेंसी होने का दिखावा करता है। वे मामला निपटाने के लिए कभी पैसे या वीडियो कॉल की माँग नहीं करते।",
        C.INVESTMENT: "यह निवेश से जुड़ा धोखा लगता है। गारंटीड या बहुत ज़्यादा रिटर्न एक चेतावनी का संकेत है।",
        C.OTHER_SCAM: "इस संदेश में धोखाधड़ी के कई संकेत हैं।",
        C.LEGITIMATE: "कोई खतरे का संकेत नहीं मिला। फिर भी सावधान रहें: अपना OTP, PIN या पासवर्ड किसी को न बताएं।",
    },
    "kn": {
        C.KYC_BANKING: "ಇದು ನಕಲಿ ಬ್ಯಾಂಕ್ ಅಥವಾ KYC ಸಂದೇಶದಂತೆ ಕಾಣುತ್ತದೆ. ಬ್ಯಾಂಕುಗಳು ಲಿಂಕ್ ಅಥವಾ ಕರೆಯ ಮೂಲಕ KYC ನವೀಕರಿಸಲು ಎಂದಿಗೂ ಕೇಳುವುದಿಲ್ಲ.",
        C.UTILITY_BILL: "ಇದು ವಿದ್ಯುತ್ ಅಥವಾ ಗ್ಯಾಸ್ ಸಂಪರ್ಕ ಕಡಿತದ ನಕಲಿ ಬೆದರಿಕೆಯಂತೆ ಕಾಣುತ್ತದೆ. ಕಂಪನಿಗಳು ಖಾಸಗಿ ಮೊಬೈಲ್ ಸಂಖ್ಯೆಗೆ ಕರೆ ಮಾಡಲು ಹೇಳುವುದಿಲ್ಲ.",
        C.OTP_PIN: "ಈ ಸಂದೇಶ ನಿಮ್ಮ OTP, PIN ಅಥವಾ ಪಾಸ್‌ವರ್ಡ್ ಪಡೆಯಲು ಪ್ರಯತ್ನಿಸುತ್ತಿದೆ. ಯಾವುದೇ ನಿಜವಾದ ಬ್ಯಾಂಕ್ ಅಥವಾ ಸರ್ಕಾರಿ ಕಚೇರಿ ಇವುಗಳನ್ನು ಕೇಳುವುದಿಲ್ಲ.",
        C.REMOTE_ACCESS: "ಬೇರೆಯವರು ನಿಮ್ಮ ಫೋನ್ ನಿಯಂತ್ರಿಸಲು ಬಿಡುವ ಆ್ಯಪ್ ಸ್ಥಾಪಿಸಲು ಈ ಸಂದೇಶ ಹೇಳುತ್ತದೆ. ಹಣ ಕದಿಯುವ ಸಾಮಾನ್ಯ ವಿಧಾನ ಇದು.",
        C.LOTTERY_PRIZE: "ಇದು ನಕಲಿ ಬಹುಮಾನ ಅಥವಾ ಲಾಟರಿಯಂತೆ ಕಾಣುತ್ತದೆ. ನೀವು ಭಾಗವಹಿಸದ ಡ್ರಾ ಗೆಲ್ಲಲು ಸಾಧ್ಯವಿಲ್ಲ, ನಿಜವಾದ ಬಹುಮಾನಕ್ಕೆ ಶುಲ್ಕ ಬೇಕಾಗುವುದಿಲ್ಲ.",
        C.TASK_JOB: "ಇದು ನಕಲಿ ಟಾಸ್ಕ್ ಅಥವಾ ಅರೆಕಾಲಿಕ ಉದ್ಯೋಗದ ಆಫರ್‌ನಂತೆ ಕಾಣುತ್ತದೆ. ಹಣ ಹಿಂಪಡೆಯುವ ಮೊದಲು ಪಾವತಿ ಅಥವಾ ಹೂಡಿಕೆ ಮಾಡಿಸುತ್ತಾರೆ.",
        C.DELIVERY_PARCEL: "ಇದು ನಕಲಿ ಪಾರ್ಸೆಲ್ ಅಥವಾ ಕಸ್ಟಮ್ಸ್ ಸೂಚನೆಯಂತೆ ಕಾಣುತ್ತದೆ. ಟ್ರ್ಯಾಕಿಂಗ್ ಸಂಖ್ಯೆಯನ್ನು ಕೊರಿಯರ್‌ನ ಅಧಿಕೃತ ವೆಬ್‌ಸೈಟ್ ಅಥವಾ ಆ್ಯಪ್‌ನಲ್ಲಿ ಮಾತ್ರ ಪರಿಶೀಲಿಸಿ.",
        C.IMPERSONATION_AUTHORITY: "ಈ ಸಂದೇಶ ಪೊಲೀಸ್, ನ್ಯಾಯಾಲಯ ಅಥವಾ ಸರ್ಕಾರಿ ಸಂಸ್ಥೆಯಂತೆ ನಟಿಸುತ್ತಿದೆ. ಅವರು ಪ್ರಕರಣ ಮುಗಿಸಲು ಹಣ ಅಥವಾ ವಿಡಿಯೋ ಕರೆ ಕೇಳುವುದಿಲ್ಲ.",
        C.INVESTMENT: "ಇದು ಹೂಡಿಕೆ ವಂಚನೆಯಂತೆ ಕಾಣುತ್ತದೆ. ಖಚಿತ ಅಥವಾ ತುಂಬಾ ಹೆಚ್ಚಿನ ಲಾಭ ಎಚ್ಚರಿಕೆಯ ಸಂಕೇತ.",
        C.OTHER_SCAM: "ಈ ಸಂದೇಶದಲ್ಲಿ ವಂಚನೆಯ ಹಲವು ಲಕ್ಷಣಗಳಿವೆ.",
        C.LEGITIMATE: "ಯಾವುದೇ ಅಪಾಯದ ಸೂಚನೆ ಕಂಡುಬಂದಿಲ್ಲ. ಆದರೂ ಎಚ್ಚರವಾಗಿರಿ: ನಿಮ್ಮ OTP, PIN ಅಥವಾ ಪಾಸ್‌ವರ್ಡ್ ಯಾರಿಗೂ ಹೇಳಬೇಡಿ.",
    },
}

NEXT_STEPS: dict[str, dict[str, str]] = {
    "en": {
        "scam": "Don't click links, call back or share any codes. If you lost money, call 1930 or report at cybercrime.gov.in right away.",
        "legit": "If anything still feels off, contact the sender through their official website or app.",
    },
    "hi": {
        "scam": "किसी लिंक पर क्लिक न करें, वापस कॉल न करें और कोई कोड न बताएं। अगर पैसे गए हैं तो तुरंत 1930 पर कॉल करें या cybercrime.gov.in पर शिकायत करें।",
        "legit": "अगर फिर भी कुछ अजीब लगे, तो भेजने वाले से उनकी आधिकारिक वेबसाइट या ऐप के ज़रिए संपर्क करें।",
    },
    "kn": {
        "scam": "ಯಾವುದೇ ಲಿಂಕ್ ಕ್ಲಿಕ್ ಮಾಡಬೇಡಿ, ಮರಳಿ ಕರೆ ಮಾಡಬೇಡಿ, ಯಾವುದೇ ಕೋಡ್ ಹಂಚಿಕೊಳ್ಳಬೇಡಿ. ಹಣ ಕಳೆದುಕೊಂಡಿದ್ದರೆ ತಕ್ಷಣ 1930 ಗೆ ಕರೆ ಮಾಡಿ ಅಥವಾ cybercrime.gov.in ನಲ್ಲಿ ದೂರು ನೀಡಿ.",
        "legit": "ಇನ್ನೂ ಅನುಮಾನವಿದ್ದರೆ, ಕಳುಹಿಸಿದವರನ್ನು ಅವರ ಅಧಿಕೃತ ವೆಬ್‌ಸೈಟ್ ಅಥವಾ ಆ್ಯಪ್ ಮೂಲಕ ಸಂಪರ್ಕಿಸಿ.",
    },
}
