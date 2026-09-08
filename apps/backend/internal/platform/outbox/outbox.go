// Package outbox is the transactional outbox: the seam that lets one module
// cause an effect in another without a distributed transaction.
//
// A module publishes inside its own transaction, so the message commits with
// the state change that justified it. A dispatcher then delivers it. Today
// delivery is an in-process function call; once modules become separate
// services, only the dispatcher changes.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Topics published by the studio modules.
const (
	TopicBookingConfirmed = "booking.confirmed"
	TopicWaitlistPromoted = "booking.waitlist_promoted"
	TopicVisitLogged      = "access.visit_logged"
	TopicPaymentPaid      = "wallet.payment_paid"
	TopicLowBalance       = "wallet.low_balance"
	TopicCreditsExpiring  = "wallet.credits_expiring"
	TopicCampaignQueued   = "engagement.campaign_queued"
)

// Message is one published event.
type Message struct {
	ID      string
	Topic   string
	Payload json.RawMessage
	// IdempotencyKey makes redelivery safe for handlers that must act once.
	IdempotencyKey *string
	Attempts       int
	CreatedAt      time.Time
}

// Publisher is the port modules depend on.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any, idempotencyKey *string) error
}

// Handler consumes one topic. Returning an error schedules a retry with
// backoff; the message is not lost.
type Handler func(ctx context.Context, msg Message) error

// Postgres is the outbox backed by platform.outbox_messages.
type Postgres struct {
	db    *database.DB
	ids   id.Generator
	clock clock.Clock
}

func New(db *database.DB, ids id.Generator, c clock.Clock) *Postgres {
	return &Postgres{db: db, ids: ids, clock: c}
}

// Publish writes a message on the ambient transaction, so it is committed or
// rolled back together with the change that produced it.
func (o *Postgres) Publish(ctx context.Context, topic string, payload any, idempotencyKey *string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox: encoding %s payload: %w", topic, err)
	}
	_, err = o.db.Exec(ctx, `
		INSERT INTO platform.outbox_messages (id, topic, payload, idempotency_key, available_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (topic, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		o.ids.New(id.Outbox), topic, body, idempotencyKey, o.clock.Now())
	if err != nil {
		return fmt.Errorf("outbox: publishing %s: %w", topic, err)
	}
	return nil
}

// Dispatcher drains the outbox and routes messages to their handlers.
type Dispatcher struct {
	db       *database.DB
	clock    clock.Clock
	handlers map[string][]Handler
	batch    int
	interval time.Duration
	maxRetry int
}

func NewDispatcher(db *database.DB, c clock.Clock) *Dispatcher {
	return &Dispatcher{
		db:       db,
		clock:    c,
		handlers: map[string][]Handler{},
		batch:    50,
		interval: 2 * time.Second,
		maxRetry: 8,
	}
}

// Subscribe registers a handler. Several handlers may share a topic; each runs
// in its own transaction so one failure does not roll back the others.
func (d *Dispatcher) Subscribe(topic string, handler Handler) {
	d.handlers[topic] = append(d.handlers[topic], handler)
}

// Run polls until the context is cancelled.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.drain(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "outbox drain failed", "error", err)
			}
		}
	}
}

// DrainOnce delivers every pending message and returns, which is what a test
// needs in place of waiting for the next tick.
func (d *Dispatcher) DrainOnce(ctx context.Context) error {
	return d.drain(ctx)
}

// drain claims a batch and processes it. The claim uses SKIP LOCKED, so
// several instances can run the dispatcher without doing each other's work.
func (d *Dispatcher) drain(ctx context.Context) error {
	messages, err := d.claim(ctx)
	if err != nil {
		return err
	}
	for _, msg := range messages {
		d.process(ctx, msg)
	}
	return nil
}

func (d *Dispatcher) claim(ctx context.Context) ([]Message, error) {
	rows, err := d.db.Query(ctx, `
		UPDATE platform.outbox_messages SET status = 'PROCESSING'
		WHERE id IN (
			SELECT id FROM platform.outbox_messages
			WHERE status IN ('PENDING', 'FAILED') AND available_at <= $1
			ORDER BY available_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, topic, payload, idempotency_key, attempts, created_at`,
		d.clock.Now(), d.batch)
	if err != nil {
		return nil, fmt.Errorf("outbox: claiming messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Topic, &m.Payload, &m.IdempotencyKey, &m.Attempts, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("outbox: scanning message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (d *Dispatcher) process(ctx context.Context, msg Message) {
	handlers := d.handlers[msg.Topic]
	if len(handlers) == 0 {
		// Nothing consumes this topic in this deployment. Mark it done rather
		// than retrying forever: a service that does not subscribe is not
		// failing, it is simply not interested.
		d.markDone(ctx, msg.ID)
		return
	}

	for _, handler := range handlers {
		if err := handler(ctx, msg); err != nil {
			d.markFailed(ctx, msg, err)
			return
		}
	}
	d.markDone(ctx, msg.ID)
}

func (d *Dispatcher) markDone(ctx context.Context, id string) {
	if _, err := d.db.Exec(ctx,
		`UPDATE platform.outbox_messages SET status = 'DONE', processed_at = $2 WHERE id = $1`,
		id, d.clock.Now()); err != nil {
		slog.ErrorContext(ctx, "outbox: marking done", "message_id", id, "error", err)
	}
}

// markFailed backs off exponentially and gives up after maxRetry attempts,
// leaving the row for an operator to inspect rather than deleting it.
func (d *Dispatcher) markFailed(ctx context.Context, msg Message, cause error) {
	attempts := msg.Attempts + 1
	status := "FAILED"
	if attempts >= d.maxRetry {
		status = "DONE"
		slog.ErrorContext(ctx, "outbox: giving up on message",
			"message_id", msg.ID, "topic", msg.Topic, "attempts", attempts, "error", cause)
	} else {
		slog.WarnContext(ctx, "outbox: handler failed, will retry",
			"message_id", msg.ID, "topic", msg.Topic, "attempts", attempts, "error", cause)
	}

	backoff := time.Duration(1<<uint(min(attempts, 6))) * time.Second
	if _, err := d.db.Exec(ctx, `
		UPDATE platform.outbox_messages
		SET status = $2, attempts = $3, last_error = $4, available_at = $5
		WHERE id = $1`,
		msg.ID, status, attempts, cause.Error(), d.clock.Now().Add(backoff)); err != nil {
		slog.ErrorContext(ctx, "outbox: marking failed", "message_id", msg.ID, "error", err)
	}
}

// Decode unmarshals a message payload into a typed value.
func Decode[T any](msg Message) (T, error) {
	var payload T
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return payload, fmt.Errorf("outbox: decoding %s payload: %w", msg.Topic, err)
	}
	return payload, nil
}

// Nop discards published messages, for tests.
type Nop struct{}

func (Nop) Publish(context.Context, string, any, *string) error { return nil }
