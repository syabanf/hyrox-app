package incentives

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Repository persists incentive schemes and frozen payouts.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

const schemeColumns = `id, coach_id, session_fee_idr, per_attendee_idr, full_class_bonus_idr,
	full_class_threshold_percent, no_show_penalty_idr, active, updated_at`

func scanScheme(row pgx.Row) (domain.IncentiveScheme, error) {
	var s domain.IncentiveScheme
	err := row.Scan(&s.ID, &s.CoachID, &s.SessionFeeIDR, &s.PerAttendeeIDR, &s.FullClassBonusIDR,
		&s.FullClassThresholdPercent, &s.NoShowPenaltyIDR, &s.Active, &s.UpdatedAt)
	return s, err
}

func (r *Repository) Schemes(ctx context.Context) ([]domain.IncentiveScheme, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+schemeColumns+` FROM incentives.schemes ORDER BY coach_id NULLS FIRST`)
	if err != nil {
		return nil, fmt.Errorf("incentives: listing schemes: %w", err)
	}
	defer rows.Close()

	schemes := []domain.IncentiveScheme{}
	for rows.Next() {
		s, err := scanScheme(rows)
		if err != nil {
			return nil, fmt.Errorf("incentives: scanning scheme: %w", err)
		}
		schemes = append(schemes, s)
	}
	return schemes, rows.Err()
}

func (r *Repository) Scheme(ctx context.Context, id string) (domain.IncentiveScheme, error) {
	s, err := scanScheme(r.db.QueryRow(ctx, `SELECT `+schemeColumns+` FROM incentives.schemes WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.IncentiveScheme{}, httpx.NotFound("scheme")
	}
	if err != nil {
		return domain.IncentiveScheme{}, fmt.Errorf("incentives: reading scheme: %w", err)
	}
	return s, nil
}

// DefaultScheme is the organization-wide row every coach falls back to.
func (r *Repository) DefaultScheme(ctx context.Context) (domain.IncentiveScheme, error) {
	s, err := scanScheme(r.db.QueryRow(ctx,
		`SELECT `+schemeColumns+` FROM incentives.schemes WHERE coach_id IS NULL LIMIT 1`))
	if database.IsNoRows(err) {
		return domain.IncentiveScheme{}, httpx.Conflict("NO_DEFAULT_SCHEME",
			"No organization default incentive scheme has been configured.")
	}
	if err != nil {
		return domain.IncentiveScheme{}, fmt.Errorf("incentives: reading default scheme: %w", err)
	}
	return s, nil
}

func (r *Repository) InsertScheme(ctx context.Context, s domain.IncentiveScheme) (domain.IncentiveScheme, error) {
	created, err := scanScheme(r.db.QueryRow(ctx, `
		INSERT INTO incentives.schemes (id, coach_id, session_fee_idr, per_attendee_idr, full_class_bonus_idr,
			full_class_threshold_percent, no_show_penalty_idr, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+schemeColumns,
		s.ID, s.CoachID, s.SessionFeeIDR, s.PerAttendeeIDR, s.FullClassBonusIDR,
		s.FullClassThresholdPercent, s.NoShowPenaltyIDR, s.Active))
	if database.IsUniqueViolation(err) {
		return domain.IncentiveScheme{}, httpx.Conflict("DUPLICATE",
			"That coach already has a scheme; edit it instead.")
	}
	if err != nil {
		return domain.IncentiveScheme{}, fmt.Errorf("incentives: inserting scheme: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateScheme(ctx context.Context, s domain.IncentiveScheme) (domain.IncentiveScheme, error) {
	updated, err := scanScheme(r.db.QueryRow(ctx, `
		UPDATE incentives.schemes SET session_fee_idr = $2, per_attendee_idr = $3, full_class_bonus_idr = $4,
			full_class_threshold_percent = $5, no_show_penalty_idr = $6, active = $7, updated_at = now()
		WHERE id = $1 RETURNING `+schemeColumns,
		s.ID, s.SessionFeeIDR, s.PerAttendeeIDR, s.FullClassBonusIDR,
		s.FullClassThresholdPercent, s.NoShowPenaltyIDR, s.Active))
	if database.IsNoRows(err) {
		return domain.IncentiveScheme{}, httpx.NotFound("scheme")
	}
	if err != nil {
		return domain.IncentiveScheme{}, fmt.Errorf("incentives: updating scheme: %w", err)
	}
	return updated, nil
}

// ── Payouts ──────────────────────────────────────────────────────────────────

const payoutColumns = `id, coach_id, branch_id, period_start, period_end, statement, status,
	created_by, approved_by, approved_at, paid_at, payment_reference, note, created_at, updated_at`

func scanPayout(row pgx.Row) (domain.IncentivePayout, error) {
	var p domain.IncentivePayout
	var statement []byte
	if err := row.Scan(&p.ID, &p.CoachID, &p.BranchID, &p.PeriodStart, &p.PeriodEnd, &statement,
		&p.Status, &p.CreatedBy, &p.ApprovedBy, &p.ApprovedAt, &p.PaidAt, &p.PaymentReference,
		&p.Note, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return domain.IncentivePayout{}, err
	}
	if len(statement) > 0 {
		if err := json.Unmarshal(statement, &p.Statement); err != nil {
			return domain.IncentivePayout{}, fmt.Errorf("incentives: decoding statement: %w", err)
		}
	}
	return p, nil
}

// PayoutFilter narrows the payout list.
type PayoutFilter struct {
	CoachID     string
	Status      string
	PeriodStart *time.Time
	BranchID    string
}

func (r *Repository) Payouts(ctx context.Context, filter PayoutFilter) ([]domain.IncentivePayout, error) {
	query := `SELECT ` + payoutColumns + ` FROM incentives.payouts WHERE 1 = 1`
	args := []any{}
	if filter.CoachID != "" {
		args = append(args, filter.CoachID)
		query += fmt.Sprintf(` AND coach_id = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.PeriodStart != nil {
		args = append(args, *filter.PeriodStart)
		query += fmt.Sprintf(` AND period_start = $%d`, len(args))
	}
	query += ` ORDER BY period_start DESC, created_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("incentives: listing payouts: %w", err)
	}
	defer rows.Close()

	payouts := []domain.IncentivePayout{}
	for rows.Next() {
		p, err := scanPayout(rows)
		if err != nil {
			return nil, fmt.Errorf("incentives: scanning payout: %w", err)
		}
		payouts = append(payouts, p)
	}
	return payouts, rows.Err()
}

func (r *Repository) Payout(ctx context.Context, id string) (domain.IncentivePayout, error) {
	p, err := scanPayout(r.db.QueryRow(ctx, `SELECT `+payoutColumns+` FROM incentives.payouts WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.IncentivePayout{}, httpx.NotFound("payout")
	}
	if err != nil {
		return domain.IncentivePayout{}, fmt.Errorf("incentives: reading payout: %w", err)
	}
	return p, nil
}

func (r *Repository) InsertPayout(ctx context.Context, p domain.IncentivePayout) (domain.IncentivePayout, error) {
	statement, err := json.Marshal(p.Statement)
	if err != nil {
		return domain.IncentivePayout{}, fmt.Errorf("incentives: encoding statement: %w", err)
	}
	created, err := scanPayout(r.db.QueryRow(ctx, `
		INSERT INTO incentives.payouts (id, coach_id, branch_id, period_start, period_end, statement,
			status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9) RETURNING `+payoutColumns,
		p.ID, p.CoachID, p.BranchID, p.PeriodStart, p.PeriodEnd, statement,
		p.Status, p.CreatedBy, p.CreatedAt))
	if database.IsUniqueViolation(err, "payouts_period_idx") {
		return domain.IncentivePayout{}, httpx.Conflict("PAYOUT_EXISTS",
			"A payout already exists for that coach and period.")
	}
	if err != nil {
		return domain.IncentivePayout{}, fmt.Errorf("incentives: inserting payout: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdatePayoutStatus(ctx context.Context, p domain.IncentivePayout) (domain.IncentivePayout, error) {
	updated, err := scanPayout(r.db.QueryRow(ctx, `
		UPDATE incentives.payouts SET status = $2, approved_by = $3, approved_at = $4, paid_at = $5,
			payment_reference = $6, note = $7, updated_at = now()
		WHERE id = $1 RETURNING `+payoutColumns,
		p.ID, p.Status, p.ApprovedBy, p.ApprovedAt, p.PaidAt, p.PaymentReference, p.Note))
	if database.IsNoRows(err) {
		return domain.IncentivePayout{}, httpx.NotFound("payout")
	}
	if err != nil {
		return domain.IncentivePayout{}, fmt.Errorf("incentives: updating payout: %w", err)
	}
	return updated, nil
}

// PayableIDR totals what is owed but not yet paid in a period, for the
// dashboard's liability figure.
func (r *Repository) PayableIDR(ctx context.Context, periodStart time.Time) (int64, error) {
	var total int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM((statement -> 'totals' ->> 'totalIdr')::bigint), 0)
		FROM incentives.payouts
		WHERE period_start = $1 AND status IN ('DRAFT', 'APPROVED')`, periodStart).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("incentives: totalling payable: %w", err)
	}
	return total, nil
}
