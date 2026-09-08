package crm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Partners, and what they tell us.

const partnerColumns = `id, code, name, kind, contact_name, contact_email, awards_xp,
	active, created_at, updated_at, (secret IS NOT NULL)`

func scanPartner(row pgx.Row) (domain.IntegrationPartner, error) {
	var p domain.IntegrationPartner
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.Kind, &p.ContactName, &p.ContactEmail,
		&p.AwardsXP, &p.Active, &p.CreatedAt, &p.UpdatedAt, &p.HasSecret)
	return p, err
}

func (r *Repository) Partners(ctx context.Context, activeOnly bool) ([]domain.IntegrationPartner, error) {
	query := `SELECT ` + partnerColumns + ` FROM crm.integration_partners`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY name`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("crm: listing partners: %w", err)
	}
	defer rows.Close()

	out := []domain.IntegrationPartner{}
	for rows.Next() {
		p, err := scanPartner(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning partner: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) PartnerByCode(ctx context.Context, code string) (domain.IntegrationPartner, string, error) {
	var p domain.IntegrationPartner
	var secret *string
	err := r.db.QueryRow(ctx, `
		SELECT id, code, name, kind, contact_name, contact_email, awards_xp, active,
		       created_at, updated_at, secret
		FROM crm.integration_partners WHERE code = $1`, code).
		Scan(&p.ID, &p.Code, &p.Name, &p.Kind, &p.ContactName, &p.ContactEmail,
			&p.AwardsXP, &p.Active, &p.CreatedAt, &p.UpdatedAt, &secret)
	if database.IsNoRows(err) {
		return domain.IntegrationPartner{}, "", httpx.NotFound("partner")
	}
	if err != nil {
		return domain.IntegrationPartner{}, "", fmt.Errorf("crm: reading partner: %w", err)
	}
	if secret != nil {
		p.HasSecret = true
		return p, *secret, nil
	}
	return p, "", nil
}

func (r *Repository) UpsertPartner(ctx context.Context, p domain.IntegrationPartner, secret *string) (domain.IntegrationPartner, error) {
	// A nil secret leaves whatever is there: rotating a shared secret should
	// be a deliberate act, not a side effect of renaming a partner.
	saved, err := scanPartner(r.db.QueryRow(ctx, `
		INSERT INTO crm.integration_partners (id, code, name, kind, secret, contact_name,
			contact_email, awards_xp, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind,
			secret = COALESCE(EXCLUDED.secret, crm.integration_partners.secret),
			contact_name = EXCLUDED.contact_name, contact_email = EXCLUDED.contact_email,
			awards_xp = EXCLUDED.awards_xp, active = EXCLUDED.active, updated_at = now()
		RETURNING `+partnerColumns,
		p.ID, p.Code, p.Name, p.Kind, secret, p.ContactName, p.ContactEmail,
		p.AwardsXP, p.Active))
	if err != nil {
		return domain.IntegrationPartner{}, fmt.Errorf("crm: saving partner: %w", err)
	}
	return saved, nil
}

const eventColumns = `id, partner_id, external_id, event_type, subject, member_id,
	occurred_at, payload, status, xp_awarded, error, processed_at, created_at, updated_at`

func scanEvent(row pgx.Row) (domain.ExternalEvent, error) {
	var e domain.ExternalEvent
	var payload []byte
	err := row.Scan(&e.ID, &e.PartnerID, &e.ExternalID, &e.EventType, &e.Subject, &e.MemberID,
		&e.OccurredAt, &payload, &e.Status, &e.XPAwarded, &e.Error, &e.ProcessedAt,
		&e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return domain.ExternalEvent{}, err
	}
	e.Payload = map[string]any{}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &e.Payload)
	}
	return e, nil
}

// EventFilter narrows what partners have told us.
type EventFilter struct {
	PartnerID string
	MemberID  string
	Status    string
	EventType string
	Limit     int
}

func (r *Repository) ExternalEvents(ctx context.Context, filter EventFilter) ([]domain.ExternalEvent, error) {
	query := `SELECT ` + eventColumns + ` FROM crm.external_events WHERE 1 = 1`
	args := []any{}
	for _, clause := range []struct{ column, value string }{
		{"partner_id", filter.PartnerID},
		{"member_id", filter.MemberID},
		{"status", filter.Status},
		{"event_type", filter.EventType},
	} {
		if clause.value == "" {
			continue
		}
		args = append(args, clause.value)
		query += fmt.Sprintf(" AND %s = $%d", clause.column, len(args))
	}
	query += ` ORDER BY occurred_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing external events: %w", err)
	}
	defer rows.Close()

	out := []domain.ExternalEvent{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning external event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) ExternalEvent(ctx context.Context, id string, forUpdate bool) (domain.ExternalEvent, error) {
	query := `SELECT ` + eventColumns + ` FROM crm.external_events WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	e, err := scanEvent(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.ExternalEvent{}, httpx.NotFound("event")
	}
	if err != nil {
		return domain.ExternalEvent{}, fmt.Errorf("crm: reading external event: %w", err)
	}
	return e, nil
}

// InsertEvent stores a claim, once. A partner retrying is the same event
// rather than a second one, and the bool says whether anything was new.
func (r *Repository) InsertEvent(ctx context.Context, e domain.ExternalEvent) (domain.ExternalEvent, bool, error) {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return domain.ExternalEvent{}, false, httpx.Invalid("That event's payload is not JSON.")
	}

	created, err := scanEvent(r.db.QueryRow(ctx, `
		INSERT INTO crm.external_events (id, partner_id, external_id, event_type, subject,
			member_id, occurred_at, payload, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (partner_id, external_id) DO NOTHING
		RETURNING `+eventColumns,
		e.ID, e.PartnerID, e.ExternalID, e.EventType, e.Subject, e.MemberID,
		e.OccurredAt, payload, e.Status))
	if database.IsNoRows(err) {
		return domain.ExternalEvent{}, false, nil
	}
	if database.IsForeignKeyViolation(err) {
		return domain.ExternalEvent{}, false, httpx.NotFound("partner")
	}
	if err != nil {
		return domain.ExternalEvent{}, false, fmt.Errorf("crm: storing external event: %w", err)
	}
	return created, true, nil
}

// SettleEvent records what we decided about a claim. The payload is never
// touched: what a partner sent is what they sent.
func (r *Repository) SettleEvent(ctx context.Context, e domain.ExternalEvent) (domain.ExternalEvent, error) {
	var processedAt any
	if e.ProcessedAt != nil {
		processedAt = *e.ProcessedAt
	}
	saved, err := scanEvent(r.db.QueryRow(ctx, `
		UPDATE crm.external_events SET member_id = $2, status = $3, xp_awarded = $4,
			error = $5, processed_at = $6, updated_at = now()
		WHERE id = $1 RETURNING `+eventColumns,
		e.ID, e.MemberID, e.Status, e.XPAwarded, e.Error, processedAt))
	if database.IsNoRows(err) {
		return domain.ExternalEvent{}, httpx.NotFound("event")
	}
	if err != nil {
		return domain.ExternalEvent{}, fmt.Errorf("crm: settling external event: %w", err)
	}
	return saved, nil
}

// MatchCandidates is the member list an event's subject is matched against.
func (r *Repository) MatchCandidates(ctx context.Context) ([]domain.MatchCandidate, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, COALESCE(email, ''), COALESCE(phone, ''), full_name
		 FROM identity.members WHERE status <> 'ARCHIVED'`)
	if err != nil {
		return nil, fmt.Errorf("crm: listing match candidates: %w", err)
	}
	defer rows.Close()

	out := []domain.MatchCandidate{}
	for rows.Next() {
		var c domain.MatchCandidate
		if err := rows.Scan(&c.MemberID, &c.Email, &c.Phone, &c.FullName); err != nil {
			return nil, fmt.Errorf("crm: scanning match candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
