package scheduling

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Repository persists sessions and bookings.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── Sessions ─────────────────────────────────────────────────────────────────

const sessionColumns = `id, class_type_id, branch_id, coach_id, starts_at, ends_at, capacity,
	credit_cost, booking_opens_at, booking_closes_at, status, area`

func scanSession(row pgx.Row) (domain.ClassSession, error) {
	var s domain.ClassSession
	err := row.Scan(&s.ID, &s.ClassTypeID, &s.BranchID, &s.CoachID, &s.StartsAt, &s.EndsAt,
		&s.Capacity, &s.CreditCost, &s.BookingOpensAt, &s.BookingClosesAt, &s.Status, &s.Area)
	return s, err
}

// SessionFilter narrows the schedule.
type SessionFilter struct {
	BranchID string
	From     *time.Time
	To       *time.Time
	Statuses []domain.SessionStatus
	CoachID  string
	Limit    int
}

func (r *Repository) Sessions(ctx context.Context, filter SessionFilter) ([]domain.ClassSession, error) {
	query := `SELECT ` + sessionColumns + ` FROM scheduling.class_sessions WHERE 1 = 1`
	args := []any{}

	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.CoachID != "" {
		args = append(args, filter.CoachID)
		query += fmt.Sprintf(` AND coach_id = $%d`, len(args))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		query += fmt.Sprintf(` AND starts_at >= $%d`, len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		query += fmt.Sprintf(` AND starts_at <= $%d`, len(args))
	}
	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, s := range filter.Statuses {
			statuses = append(statuses, string(s))
		}
		args = append(args, statuses)
		query += fmt.Sprintf(` AND status = ANY($%d)`, len(args))
	}
	query += ` ORDER BY starts_at`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("scheduling: listing sessions: %w", err)
	}
	defer rows.Close()

	sessions := []domain.ClassSession{}
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduling: scanning session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (r *Repository) Session(ctx context.Context, id string) (domain.ClassSession, error) {
	s, err := scanSession(r.db.QueryRow(ctx, `SELECT `+sessionColumns+` FROM scheduling.class_sessions WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.ClassSession{}, httpx.NotFound("session")
	}
	if err != nil {
		return domain.ClassSession{}, fmt.Errorf("scheduling: reading session: %w", err)
	}
	return s, nil
}

// SessionsByIDs loads a specific set of sessions, for views that already know
// which ones they need.
func (r *Repository) SessionsByIDs(ctx context.Context, ids []string) (map[string]domain.ClassSession, error) {
	if len(ids) == 0 {
		return map[string]domain.ClassSession{}, nil
	}
	rows, err := r.db.Query(ctx, `SELECT `+sessionColumns+` FROM scheduling.class_sessions WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("scheduling: loading sessions: %w", err)
	}
	defer rows.Close()

	out := make(map[string]domain.ClassSession, len(ids))
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduling: scanning session: %w", err)
		}
		out[session.ID] = session
	}
	return out, rows.Err()
}

// LockSession reads a session FOR UPDATE. Booking takes this lock so two
// members cannot both be handed the last remaining place.
func (r *Repository) LockSession(ctx context.Context, id string) (domain.ClassSession, error) {
	s, err := scanSession(r.db.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM scheduling.class_sessions WHERE id = $1 FOR UPDATE`, id))
	if database.IsNoRows(err) {
		return domain.ClassSession{}, httpx.NotFound("session")
	}
	if err != nil {
		return domain.ClassSession{}, fmt.Errorf("scheduling: locking session: %w", err)
	}
	return s, nil
}

func (r *Repository) InsertSession(ctx context.Context, s domain.ClassSession) (domain.ClassSession, error) {
	created, err := scanSession(r.db.QueryRow(ctx, `
		INSERT INTO scheduling.class_sessions (id, class_type_id, branch_id, coach_id, starts_at, ends_at,
			capacity, credit_cost, booking_opens_at, booking_closes_at, status, area)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING `+sessionColumns,
		s.ID, s.ClassTypeID, s.BranchID, s.CoachID, s.StartsAt, s.EndsAt, s.Capacity,
		s.CreditCost, s.BookingOpensAt, s.BookingClosesAt, s.Status, s.Area))
	if err != nil {
		return domain.ClassSession{}, fmt.Errorf("scheduling: inserting session: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateSession(ctx context.Context, s domain.ClassSession) (domain.ClassSession, error) {
	updated, err := scanSession(r.db.QueryRow(ctx, `
		UPDATE scheduling.class_sessions SET class_type_id = $2, branch_id = $3, coach_id = $4,
			starts_at = $5, ends_at = $6, capacity = $7, credit_cost = $8,
			booking_opens_at = $9, booking_closes_at = $10, status = $11, area = $12, updated_at = now()
		WHERE id = $1 RETURNING `+sessionColumns,
		s.ID, s.ClassTypeID, s.BranchID, s.CoachID, s.StartsAt, s.EndsAt, s.Capacity,
		s.CreditCost, s.BookingOpensAt, s.BookingClosesAt, s.Status, s.Area))
	if database.IsNoRows(err) {
		return domain.ClassSession{}, httpx.NotFound("session")
	}
	if err != nil {
		return domain.ClassSession{}, fmt.Errorf("scheduling: updating session: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteSession(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM scheduling.class_sessions WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.ErrInUse.WithMessage("This session has bookings. Cancel it instead.")
	}
	if err != nil {
		return fmt.Errorf("scheduling: deleting session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("session")
	}
	return nil
}

func (r *Repository) CountSessionsForClassType(ctx context.Context, classTypeID string) (int, error) {
	var count int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM scheduling.class_sessions WHERE class_type_id = $1`, classTypeID).Scan(&count); err != nil {
		return 0, fmt.Errorf("scheduling: counting sessions: %w", err)
	}
	return count, nil
}

// CountUpcomingForCoach counts classes a coach is still expected to teach.
func (r *Repository) CountUpcomingForCoach(ctx context.Context, coachID string, now time.Time) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM scheduling.class_sessions
		WHERE coach_id = $1 AND starts_at > $2 AND status IN ('DRAFT', 'PUBLISHED', 'FULL')`,
		coachID, now).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("scheduling: counting coach sessions: %w", err)
	}
	return count, nil
}

// ── Bookings ─────────────────────────────────────────────────────────────────

const bookingColumns = `id, member_id, session_id, status, waitlist_position, source,
	created_at, updated_at, cancelled_at, checked_in_at, promotion_offered_at`

func scanBooking(row pgx.Row) (domain.Booking, error) {
	var b domain.Booking
	err := row.Scan(&b.ID, &b.MemberID, &b.SessionID, &b.Status, &b.WaitlistPosition, &b.Source,
		&b.CreatedAt, &b.UpdatedAt, &b.CancelledAt, &b.CheckedInAt, &b.PromotionOfferedAt)
	return b, err
}

func (r *Repository) Booking(ctx context.Context, id string) (domain.Booking, error) {
	b, err := scanBooking(r.db.QueryRow(ctx, `SELECT `+bookingColumns+` FROM scheduling.bookings WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Booking{}, httpx.NotFound("booking")
	}
	if err != nil {
		return domain.Booking{}, fmt.Errorf("scheduling: reading booking: %w", err)
	}
	return b, nil
}

func (r *Repository) BookingsForSession(ctx context.Context, sessionID string) ([]domain.Booking, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+bookingColumns+` FROM scheduling.bookings WHERE session_id = $1 ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("scheduling: listing session bookings: %w", err)
	}
	defer rows.Close()
	return collectBookings(rows)
}

// BookingsForSessions loads bookings for many sessions at once, so a schedule
// view does not issue a query per row.
func (r *Repository) BookingsForSessions(ctx context.Context, sessionIDs []string) (map[string][]domain.Booking, error) {
	if len(sessionIDs) == 0 {
		return map[string][]domain.Booking{}, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+bookingColumns+` FROM scheduling.bookings WHERE session_id = ANY($1) ORDER BY created_at`, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("scheduling: listing bookings: %w", err)
	}
	defer rows.Close()

	bookings, err := collectBookings(rows)
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]domain.Booking, len(sessionIDs))
	for _, b := range bookings {
		grouped[b.SessionID] = append(grouped[b.SessionID], b)
	}
	return grouped, nil
}

func (r *Repository) BookingsForMember(ctx context.Context, memberID string, limit int) ([]domain.Booking, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+bookingColumns+` FROM scheduling.bookings WHERE member_id = $1 ORDER BY created_at DESC LIMIT $2`,
		memberID, limit)
	if err != nil {
		return nil, fmt.Errorf("scheduling: listing member bookings: %w", err)
	}
	defer rows.Close()
	return collectBookings(rows)
}

// ActiveBooking finds the member's live booking for a session, which is what
// the eligibility check means by "already booked".
func (r *Repository) ActiveBooking(ctx context.Context, memberID, sessionID string) (domain.Booking, bool, error) {
	b, err := scanBooking(r.db.QueryRow(ctx, `
		SELECT `+bookingColumns+` FROM scheduling.bookings
		WHERE member_id = $1 AND session_id = $2
		  AND status IN ('PENDING', 'CONFIRMED', 'WAITLIST', 'CHECKED_IN')
		LIMIT 1`, memberID, sessionID))
	if database.IsNoRows(err) {
		return domain.Booking{}, false, nil
	}
	if err != nil {
		return domain.Booking{}, false, fmt.Errorf("scheduling: reading active booking: %w", err)
	}
	return b, true, nil
}

// CheckInCandidate finds the confirmed booking that justifies opening the gate
// for this member right now.
//
// The window is deliberately wide on the early side: members arrive before
// class starts, and a late arrival should still get in.
func (r *Repository) CheckInCandidate(ctx context.Context, memberID, branchID string, now time.Time, earlyWindow, lateWindow time.Duration) (domain.Booking, domain.ClassSession, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT b.id, b.member_id, b.session_id, b.status, b.waitlist_position, b.source,
		       b.created_at, b.updated_at, b.cancelled_at, b.checked_in_at, b.promotion_offered_at,
		       s.id, s.class_type_id, s.branch_id, s.coach_id, s.starts_at, s.ends_at, s.capacity,
		       s.credit_cost, s.booking_opens_at, s.booking_closes_at, s.status, s.area
		FROM scheduling.bookings b
		JOIN scheduling.class_sessions s ON s.id = b.session_id
		WHERE b.member_id = $1
		  AND b.status = 'CONFIRMED'
		  AND s.branch_id = $2
		  AND s.status IN ('PUBLISHED', 'FULL')
		  AND s.starts_at <= $3
		  AND s.ends_at >= $4
		ORDER BY s.starts_at
		LIMIT 1`,
		memberID, branchID, now.Add(earlyWindow), now.Add(-lateWindow))

	var b domain.Booking
	var s domain.ClassSession
	err := row.Scan(&b.ID, &b.MemberID, &b.SessionID, &b.Status, &b.WaitlistPosition, &b.Source,
		&b.CreatedAt, &b.UpdatedAt, &b.CancelledAt, &b.CheckedInAt, &b.PromotionOfferedAt,
		&s.ID, &s.ClassTypeID, &s.BranchID, &s.CoachID, &s.StartsAt, &s.EndsAt, &s.Capacity,
		&s.CreditCost, &s.BookingOpensAt, &s.BookingClosesAt, &s.Status, &s.Area)
	if database.IsNoRows(err) {
		return domain.Booking{}, domain.ClassSession{}, false, nil
	}
	if err != nil {
		return domain.Booking{}, domain.ClassSession{}, false, fmt.Errorf("scheduling: finding check-in candidate: %w", err)
	}
	return b, s, true, nil
}

func (r *Repository) InsertBooking(ctx context.Context, b domain.Booking) (domain.Booking, error) {
	created, err := scanBooking(r.db.QueryRow(ctx, `
		INSERT INTO scheduling.bookings (id, member_id, session_id, status, waitlist_position, source,
			created_at, updated_at, cancelled_at, checked_in_at, promotion_offered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, NULL, NULL, NULL) RETURNING `+bookingColumns,
		b.ID, b.MemberID, b.SessionID, b.Status, b.WaitlistPosition, b.Source, b.CreatedAt))
	if database.IsUniqueViolation(err, "bookings_one_active_per_member_session") {
		return domain.Booking{}, httpx.Conflict(string(domain.DenyAlreadyBooked),
			"You already have a place in this class.")
	}
	if err != nil {
		return domain.Booking{}, fmt.Errorf("scheduling: inserting booking: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateBooking(ctx context.Context, b domain.Booking) (domain.Booking, error) {
	updated, err := scanBooking(r.db.QueryRow(ctx, `
		UPDATE scheduling.bookings SET status = $2, waitlist_position = $3, updated_at = $4,
			cancelled_at = $5, checked_in_at = $6, promotion_offered_at = $7
		WHERE id = $1 RETURNING `+bookingColumns,
		b.ID, b.Status, b.WaitlistPosition, b.UpdatedAt, b.CancelledAt, b.CheckedInAt, b.PromotionOfferedAt))
	if database.IsNoRows(err) {
		return domain.Booking{}, httpx.NotFound("booking")
	}
	if err != nil {
		return domain.Booking{}, fmt.Errorf("scheduling: updating booking: %w", err)
	}
	return updated, nil
}

// SessionCounts is how full a session is.
type SessionCounts struct {
	Confirmed int
	Waitlist  int
	CheckedIn int
	NoShow    int
	Cancelled int
}

func (r *Repository) CountsForSession(ctx context.Context, sessionID string) (SessionCounts, error) {
	var c SessionCounts
	err := r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN ('CONFIRMED', 'CHECKED_IN', 'COMPLETED')),
			count(*) FILTER (WHERE status = 'WAITLIST'),
			count(*) FILTER (WHERE status IN ('CHECKED_IN', 'COMPLETED')),
			count(*) FILTER (WHERE status = 'NO_SHOW'),
			count(*) FILTER (WHERE status = 'CANCELLED')
		FROM scheduling.bookings WHERE session_id = $1`, sessionID).
		Scan(&c.Confirmed, &c.Waitlist, &c.CheckedIn, &c.NoShow, &c.Cancelled)
	if err != nil {
		return SessionCounts{}, fmt.Errorf("scheduling: counting session bookings: %w", err)
	}
	return c, nil
}

// CountsForSessions is the batched form for list views.
func (r *Repository) CountsForSessions(ctx context.Context, sessionIDs []string) (map[string]SessionCounts, error) {
	if len(sessionIDs) == 0 {
		return map[string]SessionCounts{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT session_id,
			count(*) FILTER (WHERE status IN ('CONFIRMED', 'CHECKED_IN', 'COMPLETED')),
			count(*) FILTER (WHERE status = 'WAITLIST'),
			count(*) FILTER (WHERE status IN ('CHECKED_IN', 'COMPLETED')),
			count(*) FILTER (WHERE status = 'NO_SHOW'),
			count(*) FILTER (WHERE status = 'CANCELLED')
		FROM scheduling.bookings WHERE session_id = ANY($1) GROUP BY session_id`, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("scheduling: counting bookings: %w", err)
	}
	defer rows.Close()

	counts := map[string]SessionCounts{}
	for rows.Next() {
		var sessionID string
		var c SessionCounts
		if err := rows.Scan(&sessionID, &c.Confirmed, &c.Waitlist, &c.CheckedIn, &c.NoShow, &c.Cancelled); err != nil {
			return nil, fmt.Errorf("scheduling: scanning counts: %w", err)
		}
		counts[sessionID] = c
	}
	return counts, rows.Err()
}

// MemberBookingsForSessions returns the caller's own bookings across a set of
// sessions, so a schedule can show "you are booked" without another round trip.
func (r *Repository) MemberBookingsForSessions(ctx context.Context, memberID string, sessionIDs []string) (map[string]domain.Booking, error) {
	if memberID == "" || len(sessionIDs) == 0 {
		return map[string]domain.Booking{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+bookingColumns+` FROM scheduling.bookings
		WHERE member_id = $1 AND session_id = ANY($2)
		  AND status IN ('PENDING', 'CONFIRMED', 'WAITLIST', 'CHECKED_IN')`, memberID, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("scheduling: listing member bookings: %w", err)
	}
	defer rows.Close()

	bookings, err := collectBookings(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Booking, len(bookings))
	for _, b := range bookings {
		out[b.SessionID] = b
	}
	return out, nil
}

// DueForNoShow finds confirmed bookings on finished sessions, which the
// attendance sweep marks as no-shows.
func (r *Repository) DueForNoShow(ctx context.Context, before time.Time, limit int) ([]domain.Booking, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.member_id, b.session_id, b.status, b.waitlist_position, b.source,
		       b.created_at, b.updated_at, b.cancelled_at, b.checked_in_at, b.promotion_offered_at
		FROM scheduling.bookings b
		JOIN scheduling.class_sessions s ON s.id = b.session_id
		WHERE b.status = 'CONFIRMED' AND s.ends_at < $1 AND s.status <> 'CANCELLED'
		ORDER BY s.ends_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("scheduling: finding no-shows: %w", err)
	}
	defer rows.Close()
	return collectBookings(rows)
}

// UpcomingForReminders finds confirmed bookings starting inside the window.
func (r *Repository) UpcomingForReminders(ctx context.Context, from, to time.Time) ([]domain.Booking, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.member_id, b.session_id, b.status, b.waitlist_position, b.source,
		       b.created_at, b.updated_at, b.cancelled_at, b.checked_in_at, b.promotion_offered_at
		FROM scheduling.bookings b
		JOIN scheduling.class_sessions s ON s.id = b.session_id
		WHERE b.status = 'CONFIRMED' AND s.starts_at BETWEEN $1 AND $2 AND s.status IN ('PUBLISHED', 'FULL')
		ORDER BY s.starts_at`, from, to)
	if err != nil {
		return nil, fmt.Errorf("scheduling: finding reminder bookings: %w", err)
	}
	defer rows.Close()
	return collectBookings(rows)
}

func collectBookings(rows pgx.Rows) ([]domain.Booking, error) {
	bookings := []domain.Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduling: scanning booking: %w", err)
		}
		bookings = append(bookings, b)
	}
	return bookings, rows.Err()
}
