// Package report implements the bot's "Report a scam" conversation.
//
// It is transport-agnostic: the Telegram handler feeds each user message
// or button press into Flow.Handle and sends back the returned Reply. The
// flow submits the report to the reputation service and then gives the
// user ready-to-copy details for the official channels:
//
//   - 1930, the national cyber-fraud helpline (call immediately if money was lost)
//   - cybercrime.gov.in, the National Cyber Crime Reporting Portal
//   - Chakshu on sancharsaathi.gov.in, for suspected fraud calls, SMS and WhatsApp messages
package report

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Step is the current position in the conversation.
type Step int

const (
	StepKind Step = iota
	StepValue
	StepCategory
	StepMoneyLost
	StepConfirm
	StepDone
)

// Button is an inline keyboard option: Label is shown, Data is sent back.
type Button struct{ Label, Data string }

// Reply is what the bot should send.
type Reply struct {
	Text    string
	Buttons [][]Button
	Done    bool
}

// Submitter files the report with the reputation service.
type Submitter interface {
	Submit(ctx context.Context, kind, value, category, reporterID string) (status string, err error)
}

// Categories offered to the user (keys match the reputation service).
var Categories = []Button{
	{"KYC / account block", "kyc_fraud"},
	{"UPI collect request", "upi_collect"},
	{"Fake job / task", "fake_job"},
	{"Lottery / prize", "lottery"},
	{"Pretending to be police, bank or courier", "impersonation"},
	{"Investment / trading", "investment"},
	{"Loan app", "loan_app"},
	{"Something else", "other"},
}

var kindLabels = map[string]string{"phone": "Phone number", "upi": "UPI ID", "domain": "Website / link"}

// Flow is one user's report conversation. It is not safe for concurrent
// use; the bot keeps one Flow per chat.
type Flow struct {
	ReporterID string // e.g. "tg:<user id>"; hashed by the reputation service
	Submitter  Submitter
	Now        func() time.Time

	step      Step
	kind      string
	value     string
	category  string
	moneyLost bool
}

// New starts a flow for reporterID.
func New(reporterID string, s Submitter) *Flow {
	return &Flow{ReporterID: reporterID, Submitter: s, Now: time.Now}
}

// Start returns the first prompt.
func (f *Flow) Start() Reply {
	f.step = StepKind
	return Reply{
		Text: "What would you like to report?",
		Buttons: [][]Button{
			{{"Phone number", "phone"}, {"UPI ID", "upi"}},
			{{"Website / link", "domain"}},
			{{"Cancel", "cancel"}},
		},
	}
}

// Step reports where the conversation is.
func (f *Flow) Step() Step { return f.step }

// Handle advances the conversation with the user's input.
func (f *Flow) Handle(ctx context.Context, input string) Reply {
	input = strings.TrimSpace(input)
	if strings.EqualFold(input, "cancel") || input == "/cancel" {
		f.step = StepDone
		return Reply{Text: "Report cancelled. Nothing was saved.", Done: true}
	}
	switch f.step {
	case StepKind:
		if _, ok := kindLabels[input]; !ok {
			return f.Start()
		}
		f.kind, f.step = input, StepValue
		return Reply{Text: "Send the " + strings.ToLower(kindLabels[input]) + " exactly as you received it."}
	case StepValue:
		if input == "" || len(input) > 300 {
			return Reply{Text: "Please send the " + strings.ToLower(kindLabels[f.kind]) + ", or tap Cancel."}
		}
		f.value, f.step = input, StepCategory
		rows := make([][]Button, 0, len(Categories))
		for _, c := range Categories {
			rows = append(rows, []Button{c})
		}
		return Reply{Text: "What kind of scam was it?", Buttons: rows}
	case StepCategory:
		if !validCategory(input) {
			return Reply{Text: "Please choose one of the options."}
		}
		f.category, f.step = input, StepMoneyLost
		return Reply{
			Text:    "Did you lose money or share bank, card or UPI details?",
			Buttons: [][]Button{{{"Yes", "yes"}, {"No", "no"}}},
		}
	case StepMoneyLost:
		if input != "yes" && input != "no" {
			return Reply{Text: "Please tap Yes or No."}
		}
		f.moneyLost, f.step = input == "yes", StepConfirm
		return Reply{
			Text: fmt.Sprintf("You're reporting this %s: %s\nCategory: %s\n\nYour Telegram ID is never shown to anyone. Submit?",
				strings.ToLower(kindLabels[f.kind]), f.value, categoryLabel(f.category)),
			Buttons: [][]Button{{{"Submit", "submit"}, {"Cancel", "cancel"}}},
		}
	case StepConfirm:
		if input != "submit" {
			return Reply{Text: "Tap Submit to send the report, or Cancel."}
		}
		f.step = StepDone
		status, err := f.Submitter.Submit(ctx, f.kind, f.value, f.category, f.ReporterID)
		var head string
		switch {
		case err != nil:
			head = "We couldn't save your report right now, but please still report it officially:"
		case status == "duplicate":
			head = "You've already reported this one, thank you. Here's how to report it officially:"
		default:
			head = "Thank you, your report was saved. It only shows up for others once several people report the same thing."
		}
		return Reply{Text: head + "\n\n" + OfficialChannels(f.moneyLost) + "\n\n" + CopyableSummary(f.kind, f.value, f.category, f.Now()), Done: true}
	}
	return f.Start()
}

func validCategory(c string) bool {
	for _, b := range Categories {
		if b.Data == c {
			return true
		}
	}
	return false
}

func categoryLabel(c string) string {
	for _, b := range Categories {
		if b.Data == c {
			return b.Label
		}
	}
	return c
}

// OfficialChannels explains where to report, ordered by urgency.
func OfficialChannels(moneyLost bool) string {
	var b strings.Builder
	if moneyLost {
		b.WriteString("1. Call 1930 now. The first hours matter: the helpline can ask banks to freeze the money.\n")
		b.WriteString("2. File a complaint at https://cybercrime.gov.in (Report Cyber Crime > Financial Fraud) and keep the acknowledgement number.\n")
		b.WriteString("3. Call your bank's official number to block cards or UPI.\n")
		b.WriteString("4. Report the call or message on Chakshu: https://sancharsaathi.gov.in")
	} else {
		b.WriteString("1. Report the call, SMS or WhatsApp message on Chakshu: https://sancharsaathi.gov.in (Citizen Centric Services > Chakshu).\n")
		b.WriteString("2. You can also file a complaint at https://cybercrime.gov.in.\n")
		b.WriteString("If you later lose money, call 1930 immediately.")
	}
	return b.String()
}

// CopyableSummary is a plain-text block the user can paste into the
// cybercrime.gov.in or Chakshu forms.
func CopyableSummary(kind, value, category string, at time.Time) string {
	ist := time.FixedZone("IST", 5*3600+1800)
	return fmt.Sprintf("Copy this into the complaint form:\n```\nSuspected fraud: %s\n%s: %s\nReported via Satark on: %s\nWhat happened: <describe in your own words, with date and time>\n```",
		categoryLabel(category), kindLabels[kind], value, at.In(ist).Format("02 Jan 2006, 15:04 IST"))
}
