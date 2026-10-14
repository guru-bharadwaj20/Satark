// Package sandboxupdate edits the bot's earlier reply when the link-scan
// sandbox result arrives on NATS (scan.results.<id>).
//
// When telegram-bot asks for a link check it replies immediately with the
// fast verdict and passes {telegram_chat_id, telegram_message_id, lang} as
// scan correlation. linkscan-operator echoes that correlation on the result;
// the Updater uses it to append a "Sandbox check" section to that reply.
package sandboxupdate

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	ResultsStream  = "SCAN_RESULTS"
	ResultsSubject = "scan.results.>"
	ConsumerName   = "telegram-bot"
)

// OriginalStore returns the text of the reply that is being edited, so the
// edit keeps the fast verdict. Implementations may use Valkey with a TTL.
type OriginalStore interface {
	Original(ctx context.Context, chatID, messageID int64) (string, bool)
}

// Updater consumes scan results and edits Telegram replies.
type Updater struct {
	Editor   Editor
	Original OriginalStore // optional
	Log      *slog.Logger
}

// EnsureConsumer creates (or updates) the bot's durable consumer on SCAN_RESULTS.
// New consumers start from new messages only: old results are useless to edit.
func EnsureConsumer(ctx context.Context, js jetstream.JetStream) (jetstream.Consumer, error) {
	return js.CreateOrUpdateConsumer(ctx, ResultsStream, jetstream.ConsumerConfig{
		Durable:       ConsumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    5,
		DeliverPolicy: jetstream.DeliverNewPolicy,
		FilterSubject: ResultsSubject,
	})
}

// Run consumes until ctx is cancelled.
func (u *Updater) Run(ctx context.Context, cons jetstream.Consumer) error {
	cc, err := cons.Consume(func(msg jetstream.Msg) { u.Handle(ctx, msg) })
	if err != nil {
		return err
	}
	<-ctx.Done()
	cc.Stop()
	return nil
}

// Message is the subset of jetstream.Msg the handler needs (eases testing).
type Message interface {
	Data() []byte
	Ack() error
	Term() error
	NakWithDelay(time.Duration) error
}

// Handle processes one result message.
func (u *Updater) Handle(ctx context.Context, msg Message) {
	log := u.Log
	if log == nil {
		log = slog.Default()
	}
	var r Result
	if err := json.Unmarshal(msg.Data(), &r); err != nil {
		log.Warn("dropping malformed scan result", "err", err)
		_ = msg.Term()
		return
	}
	chatID, err1 := strconv.ParseInt(r.Correlation[KeyChatID], 10, 64)
	msgID, err2 := strconv.ParseInt(r.Correlation[KeyMessageID], 10, 64)
	if err1 != nil || err2 != nil {
		// Not a Telegram-originated scan (API or PWA request).
		_ = msg.Ack()
		return
	}
	original := ""
	if u.Original != nil {
		original, _ = u.Original.Original(ctx, chatID, msgID)
	}
	err := u.Editor.EditMessageText(ctx, chatID, msgID, Compose(original, r))
	var ra *RetryAfterError
	switch {
	case err == nil:
		log.Info("edited reply with sandbox result", "scan", r.ID, "phase", r.Phase)
		_ = msg.Ack()
	case errors.As(err, &ra):
		_ = msg.NakWithDelay(ra.After)
	case errors.Is(err, ErrPermanent):
		log.Info("cannot edit reply; giving up", "scan", r.ID, "err", err)
		_ = msg.Term()
	default:
		log.Warn("edit failed; will retry", "scan", r.ID, "err", err)
		_ = msg.NakWithDelay(5 * time.Second)
	}
}
