package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Repository persists members, staff accounts and OTP challenges. It reads and
// writes the `identity` schema only.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

const memberColumns = `id, full_name, email, phone, date_of_birth, gender, emergency_contact,
	preferred_branch_id, avatar_url, status, waiver_version, waiver_accepted_at, notes,
	created_at, updated_at`

func scanMember(row pgx.Row) (domain.Member, error) {
	var m domain.Member
	var contact []byte
	if err := row.Scan(&m.ID, &m.FullName, &m.Email, &m.Phone, &m.DateOfBirth, &m.Gender,
		&contact, &m.PreferredBranchID, &m.AvatarURL, &m.Status, &m.WaiverVersion,
		&m.WaiverAcceptedAt, &m.Notes, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return domain.Member{}, err
	}
	if len(contact) > 0 {
		var parsed domain.EmergencyContact
		if err := json.Unmarshal(contact, &parsed); err != nil {
			return domain.Member{}, fmt.Errorf("identity: decoding emergency contact: %w", err)
		}
		m.EmergencyContact = &parsed
	}
	return m, nil
}

// MemberFilter narrows the admin member list.
type MemberFilter struct {
	Query  string
	Status string
	Limit  int
	Offset int
}

// Members returns members matching the filter, newest first.
//
// The search is a single case-insensitive match across name, email and phone,
// which is how the front desk actually looks someone up.
func (r *Repository) Members(ctx context.Context, filter MemberFilter) ([]domain.Member, error) {
	query := `SELECT ` + memberColumns + ` FROM identity.members WHERE 1 = 1`
	args := []any{}

	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		query += fmt.Sprintf(` AND (lower(full_name) LIKE $%d OR lower(email) LIKE $%d OR phone LIKE $%d)`,
			len(args), len(args), len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	query += ` ORDER BY created_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		query += fmt.Sprintf(` OFFSET $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("identity: listing members: %w", err)
	}
	defer rows.Close()

	members := []domain.Member{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, fmt.Errorf("identity: scanning member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *Repository) Member(ctx context.Context, id string) (domain.Member, error) {
	m, err := scanMember(r.db.QueryRow(ctx, `SELECT `+memberColumns+` FROM identity.members WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Member{}, httpx.NotFound("member")
	}
	if err != nil {
		return domain.Member{}, fmt.Errorf("identity: reading member: %w", err)
	}
	return m, nil
}

// MembersByIDs loads several members at once, for views that would otherwise
// query in a loop.
func (r *Repository) MembersByIDs(ctx context.Context, ids []string) (map[string]domain.Member, error) {
	if len(ids) == 0 {
		return map[string]domain.Member{}, nil
	}
	rows, err := r.db.Query(ctx, `SELECT `+memberColumns+` FROM identity.members WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("identity: loading members: %w", err)
	}
	defer rows.Close()

	out := make(map[string]domain.Member, len(ids))
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, fmt.Errorf("identity: scanning member: %w", err)
		}
		out[m.ID] = m
	}
	return out, rows.Err()
}

// MemberByIdentifier finds a member by email or phone, which is what sign-in
// has to work from.
func (r *Repository) MemberByIdentifier(ctx context.Context, identifier string) (domain.Member, error) {
	trimmed := strings.TrimSpace(identifier)
	m, err := scanMember(r.db.QueryRow(ctx, `
		SELECT `+memberColumns+` FROM identity.members
		WHERE (lower(email) = lower($1) OR phone = $1) AND status <> 'ARCHIVED'
		ORDER BY created_at DESC LIMIT 1`, trimmed))
	if database.IsNoRows(err) {
		return domain.Member{}, httpx.NotFound("member")
	}
	if err != nil {
		return domain.Member{}, fmt.Errorf("identity: reading member by identifier: %w", err)
	}
	return m, nil
}

func (r *Repository) InsertMember(ctx context.Context, m domain.Member) (domain.Member, error) {
	contact, err := marshalContact(m.EmergencyContact)
	if err != nil {
		return domain.Member{}, err
	}
	created, err := scanMember(r.db.QueryRow(ctx, `
		INSERT INTO identity.members (id, full_name, email, phone, date_of_birth, gender, emergency_contact,
			preferred_branch_id, avatar_url, status, waiver_version, waiver_accepted_at, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
		RETURNING `+memberColumns,
		m.ID, m.FullName, m.Email, m.Phone, m.DateOfBirth, m.Gender, contact,
		m.PreferredBranchID, m.AvatarURL, m.Status, m.WaiverVersion, m.WaiverAcceptedAt, m.Notes, m.CreatedAt))
	if database.IsUniqueViolation(err) {
		return domain.Member{}, httpx.Conflict("IDENTIFIER_TAKEN",
			"An account already exists with that email or phone number.")
	}
	if err != nil {
		return domain.Member{}, fmt.Errorf("identity: inserting member: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateMember(ctx context.Context, m domain.Member) (domain.Member, error) {
	contact, err := marshalContact(m.EmergencyContact)
	if err != nil {
		return domain.Member{}, err
	}
	updated, err := scanMember(r.db.QueryRow(ctx, `
		UPDATE identity.members SET full_name = $2, email = $3, phone = $4, date_of_birth = $5,
			gender = $6, emergency_contact = $7, preferred_branch_id = $8, avatar_url = $9,
			status = $10, waiver_version = $11, waiver_accepted_at = $12, notes = $13, updated_at = $14
		WHERE id = $1 RETURNING `+memberColumns,
		m.ID, m.FullName, m.Email, m.Phone, m.DateOfBirth, m.Gender, contact,
		m.PreferredBranchID, m.AvatarURL, m.Status, m.WaiverVersion, m.WaiverAcceptedAt, m.Notes, m.UpdatedAt))
	if database.IsNoRows(err) {
		return domain.Member{}, httpx.NotFound("member")
	}
	if database.IsUniqueViolation(err) {
		return domain.Member{}, httpx.Conflict("IDENTIFIER_TAKEN",
			"Another account already uses that email or phone number.")
	}
	if err != nil {
		return domain.Member{}, fmt.Errorf("identity: updating member: %w", err)
	}
	return updated, nil
}

// CountMembers backs the admin summary cards.
func (r *Repository) CountMembers(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT status, count(*) FROM identity.members GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("identity: counting members: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("identity: scanning member counts: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func marshalContact(c *domain.EmergencyContact) ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	body, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("identity: encoding emergency contact: %w", err)
	}
	return body, nil
}

// ── Admin users ──────────────────────────────────────────────────────────────

const adminColumns = `id, name, email, role, branch_id`

func (r *Repository) AdminUsers(ctx context.Context) ([]domain.AdminUser, error) {
	rows, err := r.db.Query(ctx, `SELECT `+adminColumns+` FROM identity.admin_users ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("identity: listing admin users: %w", err)
	}
	defer rows.Close()

	users := []domain.AdminUser{}
	for rows.Next() {
		var u domain.AdminUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.BranchID); err != nil {
			return nil, fmt.Errorf("identity: scanning admin user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repository) AdminUser(ctx context.Context, id string) (domain.AdminUser, error) {
	var u domain.AdminUser
	err := r.db.QueryRow(ctx, `SELECT `+adminColumns+` FROM identity.admin_users WHERE id = $1`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.BranchID)
	if database.IsNoRows(err) {
		return domain.AdminUser{}, httpx.NotFound("user")
	}
	if err != nil {
		return domain.AdminUser{}, fmt.Errorf("identity: reading admin user: %w", err)
	}
	return u, nil
}

func (r *Repository) AdminUserByEmail(ctx context.Context, email string) (domain.AdminUser, error) {
	var u domain.AdminUser
	err := r.db.QueryRow(ctx,
		`SELECT `+adminColumns+` FROM identity.admin_users WHERE lower(email) = lower($1)`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.BranchID)
	if database.IsNoRows(err) {
		return domain.AdminUser{}, httpx.NotFound("user")
	}
	if err != nil {
		return domain.AdminUser{}, fmt.Errorf("identity: reading admin user by email: %w", err)
	}
	return u, nil
}

func (r *Repository) InsertAdminUser(ctx context.Context, u domain.AdminUser) (domain.AdminUser, error) {
	var created domain.AdminUser
	err := r.db.QueryRow(ctx, `
		INSERT INTO identity.admin_users (id, name, email, role, branch_id)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+adminColumns,
		u.ID, u.Name, u.Email, u.Role, u.BranchID).
		Scan(&created.ID, &created.Name, &created.Email, &created.Role, &created.BranchID)
	if database.IsUniqueViolation(err) {
		return domain.AdminUser{}, httpx.Conflict("DUPLICATE", "A staff account already uses that email.")
	}
	if err != nil {
		return domain.AdminUser{}, fmt.Errorf("identity: inserting admin user: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateAdminUser(ctx context.Context, u domain.AdminUser) (domain.AdminUser, error) {
	var updated domain.AdminUser
	err := r.db.QueryRow(ctx, `
		UPDATE identity.admin_users SET name = $2, email = $3, role = $4, branch_id = $5, updated_at = now()
		WHERE id = $1 RETURNING `+adminColumns,
		u.ID, u.Name, u.Email, u.Role, u.BranchID).
		Scan(&updated.ID, &updated.Name, &updated.Email, &updated.Role, &updated.BranchID)
	if database.IsNoRows(err) {
		return domain.AdminUser{}, httpx.NotFound("user")
	}
	if database.IsUniqueViolation(err) {
		return domain.AdminUser{}, httpx.Conflict("DUPLICATE", "Another staff account already uses that email.")
	}
	if err != nil {
		return domain.AdminUser{}, fmt.Errorf("identity: updating admin user: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteAdminUser(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM identity.admin_users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("identity: deleting admin user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("user")
	}
	return nil
}

// CountRole is used to stop the last Super Admin being deleted.
func (r *Repository) CountRole(ctx context.Context, role domain.AdminRole) (int, error) {
	var count int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM identity.admin_users WHERE role = $1`, role).Scan(&count); err != nil {
		return 0, fmt.Errorf("identity: counting role: %w", err)
	}
	return count, nil
}

// ── OTP challenges ───────────────────────────────────────────────────────────

// Challenge is a pending sign-in attempt.
type Challenge struct {
	ID         string
	Identifier string
	CodeHash   string
	MemberID   *string
	Attempts   int
	ConsumedAt *time.Time
	ExpiresAt  time.Time
}

func (r *Repository) InsertChallenge(ctx context.Context, c Challenge) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO identity.otp_challenges (id, identifier, code_hash, member_id, expires_at)
		VALUES ($1, $2, $3, $4, $5)`,
		c.ID, c.Identifier, c.CodeHash, c.MemberID, c.ExpiresAt)
	if err != nil {
		return fmt.Errorf("identity: inserting challenge: %w", err)
	}
	return nil
}

func (r *Repository) Challenge(ctx context.Context, id string) (Challenge, error) {
	var c Challenge
	err := r.db.QueryRow(ctx, `
		SELECT id, identifier, code_hash, member_id, attempts, consumed_at, expires_at
		FROM identity.otp_challenges WHERE id = $1`, id).
		Scan(&c.ID, &c.Identifier, &c.CodeHash, &c.MemberID, &c.Attempts, &c.ConsumedAt, &c.ExpiresAt)
	if database.IsNoRows(err) {
		return Challenge{}, httpx.ErrBadRequest.WithMessage("That sign-in request has expired. Request a new code.")
	}
	if err != nil {
		return Challenge{}, fmt.Errorf("identity: reading challenge: %w", err)
	}
	return c, nil
}

// RecordAttempt counts a failed code so the challenge can be locked out.
func (r *Repository) RecordAttempt(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE identity.otp_challenges SET attempts = attempts + 1 WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("identity: recording attempt: %w", err)
	}
	return nil
}

// ConsumeChallenge burns a challenge, returning false when someone already
// used it: a code is good exactly once.
func (r *Repository) ConsumeChallenge(ctx context.Context, id string, now time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE identity.otp_challenges SET consumed_at = $2 WHERE id = $1 AND consumed_at IS NULL`, id, now)
	if err != nil {
		return false, fmt.Errorf("identity: consuming challenge: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// PurgeExpiredChallenges keeps the table from growing without bound.
func (r *Repository) PurgeExpiredChallenges(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM identity.otp_challenges WHERE expires_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("identity: purging challenges: %w", err)
	}
	return tag.RowsAffected(), nil
}
