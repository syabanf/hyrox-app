// Package audit records who changed what.
//
// Every financial and access-control action writes an event here. The table is
// append-only at the database level, so the trail is evidence rather than
// merely a log.
package audit

import (
	"context"
	"fmt"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Event is one recorded change.
type Event struct {
	EntityType    string
	EntityID      string
	Action        string
	PreviousValue *string
	NewValue      *string
	ActorID       string
	ActorName     string
	Reason        *string
}

// Recorder is the port modules depend on. Keeping it an interface means a
// module extracted into its own service can swap this for an HTTP client, or
// drop audit entirely in a test.
type Recorder interface {
	Record(ctx context.Context, event Event) error
}

// PostgresRecorder writes to platform.audit_events. Because it uses the shared
// DB handle, a Record call inside a transaction commits with it: an action and
// its audit trail are never separated.
type PostgresRecorder struct {
	db    *database.DB
	ids   id.Generator
	clock clock.Clock
}

func NewRecorder(db *database.DB, ids id.Generator, c clock.Clock) *PostgresRecorder {
	return &PostgresRecorder{db: db, ids: ids, clock: c}
}

func (r *PostgresRecorder) Record(ctx context.Context, event Event) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO platform.audit_events
			(id, entity_type, entity_id, action, previous_value, new_value, actor_id, actor_name, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		r.ids.New(id.Audit), event.EntityType, event.EntityID, event.Action,
		event.PreviousValue, event.NewValue, event.ActorID, event.ActorName, event.Reason,
		r.clock.Now(),
	)
	if err != nil {
		return fmt.Errorf("audit: recording %s on %s: %w", event.Action, event.EntityType, err)
	}
	return nil
}

// List returns the most recent events, newest first.
func (r *PostgresRecorder) List(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, entity_type, entity_id, action, previous_value, new_value,
		       actor_id, actor_name, reason, created_at
		FROM platform.audit_events ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: listing events: %w", err)
	}
	defer rows.Close()

	events := []domain.AuditEvent{}
	for rows.Next() {
		var e domain.AuditEvent
		if err := rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.Action, &e.PreviousValue,
			&e.NewValue, &e.ActorID, &e.ActorName, &e.Reason, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("audit: scanning event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// Nop discards events, for tests and for services that do not audit.
type Nop struct{}

func (Nop) Record(context.Context, Event) error { return nil }

// Str is a convenience for the optional string fields.
func Str(value string) *string { return &value }
