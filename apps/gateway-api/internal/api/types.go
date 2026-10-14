// Package api holds the public request and response types of gateway-api.
// They mirror apps/gateway-api/api/openapi.yaml.
package api

// Languages supported for reasons and next steps.
const (
	LangEnglish = "en"
	LangHindi   = "hi"
	LangKannada = "kn"
)

// ValidLang reports whether l is a supported language code.
func ValidLang(l string) bool {
	return l == LangEnglish || l == LangHindi || l == LangKannada
}

// Sender types collected by the "Who sent this?" follow-up.
const (
	SenderUnknown       = "unknown"
	SenderSavedContact  = "saved_contact"
	SenderUnknownNumber = "unknown_number"
	SenderBusiness      = "business_sender"
	SenderGroup         = "group"
)

// ValidSender reports whether s is a supported sender type.
func ValidSender(s string) bool {
	switch s {
	case SenderUnknown, SenderSavedContact, SenderUnknownNumber, SenderBusiness, SenderGroup:
		return true
	}
	return false
}

// Verdict buckets. There is deliberately no "safe" verdict.
const (
	VerdictNoRedFlags = "no_red_flags"
	VerdictSuspicious = "suspicious"
	VerdictLikelyScam = "likely_scam"
)

// Disclaimer is attached to every scan response.
const Disclaimer = "Satark gives a risk assessment, not a guarantee. Never share your OTP, UPI PIN or password."

// TextScanRequest is the body of POST /v1/scan/text.
type TextScanRequest struct {
	Text       string `json:"text"`
	Lang       string `json:"lang,omitempty"`
	SenderType string `json:"sender_type,omitempty"`
	Store      bool   `json:"store,omitempty"`
}

// QRScanRequest is the parsed multipart body of POST /v1/scan/qr.
type QRScanRequest struct {
	Image   []byte
	Lang    string
	Context string
}

// Reason explains one contribution to the risk score.
type Reason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Weight  int    `json:"weight"`
	Source  string `json:"source,omitempty"`
}

// UPIIntent is a parsed upi://pay link with the payee masked.
type UPIIntent struct {
	PayeeAddress string `json:"payee_address,omitempty"`
	PayeeName    string `json:"payee_name,omitempty"`
	Amount       string `json:"amount,omitempty"`
	Currency     string `json:"currency,omitempty"`
	Note         string `json:"note,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

// QRResult describes one decoded QR code.
type QRResult struct {
	Kind   string     `json:"kind"`
	Format string     `json:"format,omitempty"`
	UPI    *UPIIntent `json:"upi,omitempty"`
	URL    string     `json:"url,omitempty"`
}

// ScanResponse is returned by both scan endpoints.
type ScanResponse struct {
	ScanID       string     `json:"scan_id"`
	Verdict      string     `json:"verdict"`
	RiskScore    int        `json:"risk_score"`
	Reasons      []Reason   `json:"reasons"`
	NextSteps    []string   `json:"next_steps"`
	Lang         string     `json:"lang"`
	RedactedText string     `json:"redacted_text,omitempty"`
	QR           []QRResult `json:"qr,omitempty"`
	Partial      bool       `json:"partial,omitempty"`
	Disclaimer   string     `json:"disclaimer"`
}
