package access

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Repository persists QR credentials and the access log.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── QR tokens ────────────────────────────────────────────────────────────────

func (r *Repository) InsertToken(ctx context.Context, t domain.QrToken) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO access.qr_tokens (token, member_id, issued_at, expires_at) VALUES ($1, $2, $3, $4)`,
		t.Token, t.MemberID, t.IssuedAt, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("access: inserting qr token: %w", err)
	}
	return nil
}

func (r *Repository) Token(ctx context.Context, token string) (domain.QrToken, bool, error) {
	var t domain.QrToken
	err := r.db.QueryRow(ctx,
		`SELECT token, member_id, issued_at, expires_at, consumed_at FROM access.qr_tokens WHERE token = $1`, token).
		Scan(&t.Token, &t.MemberID, &t.IssuedAt, &t.ExpiresAt, &t.ConsumedAt)
	if database.IsNoRows(err) {
		return domain.QrToken{}, false, nil
	}
	if err != nil {
		return domain.QrToken{}, false, fmt.Errorf("access: reading qr token: %w", err)
	}
	return t, true, nil
}

// ConsumeToken burns a credential, returning false when it was already used.
// The conditional update is what makes a replayed QR code useless even if two
// scanners present it at the same instant.
func (r *Repository) ConsumeToken(ctx context.Context, token, gateID string, now time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE access.qr_tokens SET consumed_at = $2, consumed_by_gate_id = $3
		WHERE token = $1 AND consumed_at IS NULL`, token, now, gateID)
	if err != nil {
		return false, fmt.Errorf("access: consuming qr token: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// PurgeExpiredTokens keeps the table small; consumed tokens are kept for a
// while so a disputed scan can still be traced.
func (r *Repository) PurgeExpiredTokens(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM access.qr_tokens WHERE expires_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("access: purging qr tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ── Access log ───────────────────────────────────────────────────────────────

const logColumns = `id, member_id, gate_id, branch_id, result, reason_code, credit_delta, mode, booking_id, created_at`

func scanLog(row pgx.Row) (domain.AccessLog, error) {
	var l domain.AccessLog
	err := row.Scan(&l.ID, &l.MemberID, &l.GateID, &l.BranchID, &l.Result, &l.ReasonCode,
		&l.CreditDelta, &l.Mode, &l.BookingID, &l.CreatedAt)
	return l, err
}

func (r *Repository) InsertLog(ctx context.Context, l domain.AccessLog) (domain.AccessLog, error) {
	created, err := scanLog(r.db.QueryRow(ctx, `
		INSERT INTO access.access_logs (id, member_id, gate_id, branch_id, result, reason_code,
			credit_delta, mode, booking_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+logColumns,
		l.ID, l.MemberID, l.GateID, l.BranchID, l.Result, l.ReasonCode,
		l.CreditDelta, l.Mode, l.BookingID, l.CreatedAt))
	if err != nil {
		return domain.AccessLog{}, fmt.Errorf("access: inserting access log: %w", err)
	}
	return created, nil
}

// LogFilter narrows the access log for the admin monitor.
type LogFilter struct {
	MemberID string
	BranchID string
	GateID   string
	Result   string
	Mode     string
	Since    *time.Time
	Limit    int
}

func (r *Repository) Logs(ctx context.Context, filter LogFilter) ([]domain.AccessLog, error) {
	query := `SELECT ` + logColumns + ` FROM access.access_logs WHERE 1 = 1`
	args := []any{}

	if filter.MemberID != "" {
		args = append(args, filter.MemberID)
		query += fmt.Sprintf(` AND member_id = $%d`, len(args))
	}
	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.GateID != "" {
		args = append(args, filter.GateID)
		query += fmt.Sprintf(` AND gate_id = $%d`, len(args))
	}
	switch filter.Result {
	case "":
	case string(domain.AccessAllowed):
		// "Allowed" means anything that actually opened the door, however it
		// was validated.
		args = append(args, []string{
			string(domain.AccessAllowed), string(domain.AccessOfflineAllowed), string(domain.AccessSynced),
		})
		query += fmt.Sprintf(` AND result = ANY($%d)`, len(args))
	default:
		args = append(args, filter.Result)
		query += fmt.Sprintf(` AND result = $%d`, len(args))
	}
	if filter.Mode != "" {
		args = append(args, filter.Mode)
		query += fmt.Sprintf(` AND mode = $%d`, len(args))
	}
	if filter.Since != nil {
		args = append(args, *filter.Since)
		query += fmt.Sprintf(` AND created_at >= $%d`, len(args))
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("access: listing access logs: %w", err)
	}
	defer rows.Close()

	logs := []domain.AccessLog{}
	for rows.Next() {
		l, err := scanLog(rows)
		if err != nil {
			return nil, fmt.Errorf("access: scanning access log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *Repository) Log(ctx context.Context, id string) (domain.AccessLog, error) {
	l, err := scanLog(r.db.QueryRow(ctx, `SELECT `+logColumns+` FROM access.access_logs WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.AccessLog{}, httpx.NotFound("access log")
	}
	if err != nil {
		return domain.AccessLog{}, fmt.Errorf("access: reading access log: %w", err)
	}
	return l, nil
}

// ResolveLog updates a conflicting offline scan after review. This is the only
// update the table allows; deletes are refused outright.
func (r *Repository) ResolveLog(ctx context.Context, id string, result domain.AccessLogState, creditDelta int, bookingID *string) (domain.AccessLog, error) {
	updated, err := scanLog(r.db.QueryRow(ctx, `
		UPDATE access.access_logs SET result = $2, credit_delta = $3, booking_id = COALESCE($4, booking_id)
		WHERE id = $1 RETURNING `+logColumns, id, result, creditDelta, bookingID))
	if database.IsNoRows(err) {
		return domain.AccessLog{}, httpx.NotFound("access log")
	}
	if err != nil {
		return domain.AccessLog{}, fmt.Errorf("access: resolving access log: %w", err)
	}
	return updated, nil
}

// LastAllowedEntry is what anti-passback and the re-entry grace are measured
// against: the member's previous successful entry at this branch.
func (r *Repository) LastAllowedEntry(ctx context.Context, memberID, branchID string) (*time.Time, error) {
	var at time.Time
	err := r.db.QueryRow(ctx, `
		SELECT created_at FROM access.access_logs
		WHERE member_id = $1 AND branch_id = $2
		  AND result IN ('ALLOWED', 'OFFLINE_ALLOWED', 'SYNCED')
		ORDER BY created_at DESC LIMIT 1`, memberID, branchID).Scan(&at)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("access: reading last entry: %w", err)
	}
	return &at, nil
}

// VisitCounts summarizes entries per member, for member lists.
func (r *Repository) VisitCounts(ctx context.Context, memberIDs []string) (map[string]VisitSummary, error) {
	if len(memberIDs) == 0 {
		return map[string]VisitSummary{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT member_id, count(*), max(created_at) FROM access.access_logs
		WHERE member_id = ANY($1) AND result IN ('ALLOWED', 'OFFLINE_ALLOWED', 'SYNCED')
		GROUP BY member_id`, memberIDs)
	if err != nil {
		return nil, fmt.Errorf("access: counting visits: %w", err)
	}
	defer rows.Close()

	out := map[string]VisitSummary{}
	for rows.Next() {
		var memberID string
		var summary VisitSummary
		if err := rows.Scan(&memberID, &summary.Total, &summary.LastVisitAt); err != nil {
			return nil, fmt.Errorf("access: scanning visit counts: %w", err)
		}
		out[memberID] = summary
	}
	return out, rows.Err()
}

// VisitSummary is how often a member has come in.
type VisitSummary struct {
	Total       int        `json:"total"`
	LastVisitAt *time.Time `json:"lastVisitAt"`
}

// DailyVisits counts entries per day for the visits report.
func (r *Repository) DailyVisits(ctx context.Context, since time.Time) (map[string]int, int, int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT to_char(created_at, 'YYYY-MM-DD') AS day,
		       count(*) FILTER (WHERE result IN ('ALLOWED', 'OFFLINE_ALLOWED', 'SYNCED')),
		       count(*) FILTER (WHERE result = 'DENIED'),
		       count(*) FILTER (WHERE mode = 'OFFLINE')
		FROM access.access_logs WHERE created_at >= $1
		GROUP BY day ORDER BY day`, since)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("access: aggregating visits: %w", err)
	}
	defer rows.Close()

	byDay := map[string]int{}
	denied, offline := 0, 0
	for rows.Next() {
		var day string
		var allowed, dailyDenied, dailyOffline int
		if err := rows.Scan(&day, &allowed, &dailyDenied, &dailyOffline); err != nil {
			return nil, 0, 0, fmt.Errorf("access: scanning visits: %w", err)
		}
		byDay[day] = allowed
		denied += dailyDenied
		offline += dailyOffline
	}
	return byDay, denied, offline, rows.Err()
}

// CountToday is the dashboard's visitor count.
func (r *Repository) CountToday(ctx context.Context, since time.Time) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM access.access_logs
		WHERE created_at >= $1 AND result IN ('ALLOWED', 'OFFLINE_ALLOWED', 'SYNCED')`, since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("access: counting today's visits: %w", err)
	}
	return count, nil
}
