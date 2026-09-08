package crm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Badges, consent, conversations and reviews.

// ── Badges ───────────────────────────────────────────────────────────────────

const badgeColumns = `id, code, name, description, metric, threshold, bonus_xp,
	icon, sort_order, active`

func scanBadge(row pgx.Row) (domain.Badge, error) {
	var b domain.Badge
	err := row.Scan(&b.ID, &b.Code, &b.Name, &b.Description, &b.Metric, &b.Threshold,
		&b.BonusXP, &b.Icon, &b.SortOrder, &b.Active)
	return b, err
}

func (r *Repository) Badges(ctx context.Context, activeOnly bool) ([]domain.Badge, error) {
	query := `SELECT ` + badgeColumns + ` FROM crm.badges`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY sort_order, threshold`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("crm: listing badges: %w", err)
	}
	defer rows.Close()

	out := []domain.Badge{}
	for rows.Next() {
		b, err := scanBadge(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning badge: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertBadge(ctx context.Context, b domain.Badge) (domain.Badge, error) {
	saved, err := scanBadge(r.db.QueryRow(ctx, `
		INSERT INTO crm.badges (id, code, name, description, metric, threshold, bonus_xp,
			icon, sort_order, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name,
			description = EXCLUDED.description, metric = EXCLUDED.metric,
			threshold = EXCLUDED.threshold, bonus_xp = EXCLUDED.bonus_xp,
			icon = EXCLUDED.icon, sort_order = EXCLUDED.sort_order,
			active = EXCLUDED.active, updated_at = now()
		RETURNING `+badgeColumns,
		b.ID, b.Code, b.Name, b.Description, b.Metric, b.Threshold, b.BonusXP,
		b.Icon, b.SortOrder, b.Active))
	if err != nil {
		return domain.Badge{}, fmt.Errorf("crm: saving badge: %w", err)
	}
	return saved, nil
}

const memberBadgeColumns = `id, member_id, badge_id, earned_value, earned_at, awarded_by, note`

func scanMemberBadge(row pgx.Row) (domain.MemberBadge, error) {
	var b domain.MemberBadge
	err := row.Scan(&b.ID, &b.MemberID, &b.BadgeID, &b.EarnedValue, &b.EarnedAt,
		&b.AwardedBy, &b.Note)
	return b, err
}

func (r *Repository) MemberBadges(ctx context.Context, memberID string) ([]domain.MemberBadge, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+memberBadgeColumns+` FROM crm.member_badges
		 WHERE member_id = $1 ORDER BY earned_at DESC`, memberID)
	if err != nil {
		return nil, fmt.Errorf("crm: listing member badges: %w", err)
	}
	defer rows.Close()

	out := []domain.MemberBadge{}
	for rows.Next() {
		b, err := scanMemberBadge(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning member badge: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AwardBadge records an achievement, once.
//
// A second award of the same badge is not an error worth surfacing — the
// awarding runs on every visit, and most of the time the answer is "already
// has it". The bool says whether anything happened.
func (r *Repository) AwardBadge(ctx context.Context, b domain.MemberBadge) (domain.MemberBadge, bool, error) {
	awarded, err := scanMemberBadge(r.db.QueryRow(ctx, `
		INSERT INTO crm.member_badges (id, member_id, badge_id, earned_value, awarded_by, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (member_id, badge_id) DO NOTHING
		RETURNING `+memberBadgeColumns,
		b.ID, b.MemberID, b.BadgeID, b.EarnedValue, b.AwardedBy, b.Note))
	if database.IsNoRows(err) {
		return domain.MemberBadge{}, false, nil
	}
	if database.IsForeignKeyViolation(err) {
		return domain.MemberBadge{}, false, httpx.NotFound("badge")
	}
	if err != nil {
		return domain.MemberBadge{}, false, fmt.Errorf("crm: awarding badge: %w", err)
	}
	return awarded, true, nil
}

func (r *Repository) RevokeBadge(ctx context.Context, memberID, badgeID string) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM crm.member_badges WHERE member_id = $1 AND badge_id = $2`, memberID, badgeID)
	if err != nil {
		return fmt.Errorf("crm: revoking badge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("badge")
	}
	return nil
}

// ── Consent ──────────────────────────────────────────────────────────────────

const preferenceColumns = `member_id, channel, opted_in, reason, scope, changed_at, changed_by`

func (r *Repository) ContactPreferences(ctx context.Context, memberID string) ([]domain.ContactPreference, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+preferenceColumns+` FROM crm.contact_preferences WHERE member_id = $1
		 ORDER BY channel`, memberID)
	if err != nil {
		return nil, fmt.Errorf("crm: listing contact preferences: %w", err)
	}
	defer rows.Close()

	out := []domain.ContactPreference{}
	for rows.Next() {
		var p domain.ContactPreference
		if err := rows.Scan(&p.MemberID, &p.Channel, &p.OptedIn, &p.Reason, &p.Scope,
			&p.ChangedAt, &p.ChangedBy); err != nil {
			return nil, fmt.Errorf("crm: scanning contact preference: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) SetContactPreference(ctx context.Context, p domain.ContactPreference) (domain.ContactPreference, error) {
	var saved domain.ContactPreference
	err := r.db.QueryRow(ctx, `
		INSERT INTO crm.contact_preferences (member_id, channel, opted_in, reason, scope, changed_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (member_id, channel) DO UPDATE SET opted_in = EXCLUDED.opted_in,
			reason = EXCLUDED.reason, scope = EXCLUDED.scope,
			changed_at = now(), changed_by = EXCLUDED.changed_by
		RETURNING `+preferenceColumns,
		p.MemberID, p.Channel, p.OptedIn, p.Reason, p.Scope, p.ChangedBy).
		Scan(&saved.MemberID, &saved.Channel, &saved.OptedIn, &saved.Reason, &saved.Scope,
			&saved.ChangedAt, &saved.ChangedBy)
	if err != nil {
		return domain.ContactPreference{}, fmt.Errorf("crm: saving contact preference: %w", err)
	}
	return saved, nil
}

// OptedOutOf is every member refusing marketing on one channel.
//
// It returns the exclusion set rather than the inclusion set on purpose: the
// audience is built from members, and consent narrows it. Building the list
// the other way round means a member with no preference row is left out, which
// is the opposite of what an opt-out default should do.
func (r *Repository) OptedOutOf(ctx context.Context, channel domain.ContactChannel) (map[string]bool, error) {
	rows, err := r.db.Query(ctx,
		`SELECT member_id FROM crm.contact_preferences
		 WHERE channel = $1 AND NOT opted_in`, channel)
	if err != nil {
		return nil, fmt.Errorf("crm: listing opt-outs: %w", err)
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("crm: scanning opt-out: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ── Reviews ──────────────────────────────────────────────────────────────────

const reviewColumns = `id, member_id, subject_type, subject_id, rating, comment, verified,
	visit_id, status, reply, replied_by, replied_at, created_at, updated_at`

func scanReview(row pgx.Row) (domain.Review, error) {
	var r domain.Review
	err := row.Scan(&r.ID, &r.MemberID, &r.SubjectType, &r.SubjectID, &r.Rating, &r.Comment,
		&r.Verified, &r.VisitID, &r.Status, &r.Reply, &r.RepliedBy, &r.RepliedAt,
		&r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// ReviewFilter narrows what members have said.
type ReviewFilter struct {
	SubjectType string
	SubjectID   string
	MemberID    string
	Status      string
	// UnansweredOnly is the list somebody should actually work through.
	UnansweredOnly bool
	MaxRating      int
	Limit          int
}

func (r *Repository) Reviews(ctx context.Context, filter ReviewFilter) ([]domain.Review, error) {
	query := `SELECT ` + reviewColumns + ` FROM crm.reviews WHERE 1 = 1`
	args := []any{}

	if filter.SubjectType != "" {
		args = append(args, filter.SubjectType)
		query += fmt.Sprintf(" AND subject_type = $%d", len(args))
	}
	if filter.SubjectID != "" {
		args = append(args, filter.SubjectID)
		query += fmt.Sprintf(" AND subject_id = $%d", len(args))
	}
	if filter.MemberID != "" {
		args = append(args, filter.MemberID)
		query += fmt.Sprintf(" AND member_id = $%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if filter.UnansweredOnly {
		query += " AND reply IS NULL"
	}
	if filter.MaxRating > 0 {
		args = append(args, filter.MaxRating)
		query += fmt.Sprintf(" AND rating <= $%d", len(args))
	}
	query += " ORDER BY created_at DESC"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing reviews: %w", err)
	}
	defer rows.Close()

	out := []domain.Review{}
	for rows.Next() {
		review, err := scanReview(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning review: %w", err)
		}
		out = append(out, review)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	saved, err := scanReview(r.db.QueryRow(ctx, `
		INSERT INTO crm.reviews (id, member_id, subject_type, subject_id, rating, comment,
			verified, visit_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (member_id, subject_type, subject_id) DO UPDATE SET
			rating = EXCLUDED.rating, comment = EXCLUDED.comment,
			verified = EXCLUDED.verified, visit_id = EXCLUDED.visit_id, updated_at = now()
		RETURNING `+reviewColumns,
		review.ID, review.MemberID, review.SubjectType, review.SubjectID, review.Rating,
		review.Comment, review.Verified, review.VisitID, review.Status))
	if err != nil {
		return domain.Review{}, fmt.Errorf("crm: saving review: %w", err)
	}
	return saved, nil
}

func (r *Repository) ReplyToReview(ctx context.Context, reviewID, reply, by string) (domain.Review, error) {
	saved, err := scanReview(r.db.QueryRow(ctx, `
		UPDATE crm.reviews SET reply = $2, replied_by = $3, replied_at = now(), updated_at = now()
		WHERE id = $1 RETURNING `+reviewColumns, reviewID, reply, by))
	if database.IsNoRows(err) {
		return domain.Review{}, httpx.NotFound("review")
	}
	if err != nil {
		return domain.Review{}, fmt.Errorf("crm: replying to review: %w", err)
	}
	return saved, nil
}

func (r *Repository) SetReviewStatus(ctx context.Context, reviewID, status string) (domain.Review, error) {
	saved, err := scanReview(r.db.QueryRow(ctx, `
		UPDATE crm.reviews SET status = $2, updated_at = now()
		WHERE id = $1 RETURNING `+reviewColumns, reviewID, status))
	if database.IsNoRows(err) {
		return domain.Review{}, httpx.NotFound("review")
	}
	if database.IsCheckViolation(err) {
		return domain.Review{}, httpx.Invalid("%q is not a review status.", status)
	}
	if err != nil {
		return domain.Review{}, fmt.Errorf("crm: setting review status: %w", err)
	}
	return saved, nil
}

// ── Conversations ────────────────────────────────────────────────────────────

const conversationColumns = `id, member_id, contact_name, contact_handle, channel, subject,
	status, priority, assigned_to, assigned_name, branch_id, tags, last_member_at,
	last_staff_at, first_response_seconds, resolved_at, created_at, updated_at`

func scanConversation(row pgx.Row) (domain.Conversation, error) {
	var c domain.Conversation
	err := row.Scan(&c.ID, &c.MemberID, &c.ContactName, &c.ContactHandle, &c.Channel,
		&c.Subject, &c.Status, &c.Priority, &c.AssignedTo, &c.AssignedName, &c.BranchID,
		&c.Tags, &c.LastMemberAt, &c.LastStaffAt, &c.FirstResponseSeconds, &c.ResolvedAt,
		&c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ConversationFilter narrows the inbox.
type ConversationFilter struct {
	Status     string
	Channel    string
	AssignedTo string
	MemberID   string
	Query      string
	Limit      int
}

func (r *Repository) Conversations(ctx context.Context, filter ConversationFilter) ([]domain.Conversation, error) {
	query := `SELECT ` + conversationColumns + ` FROM crm.conversations WHERE 1 = 1`
	args := []any{}

	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if filter.Channel != "" {
		args = append(args, filter.Channel)
		query += fmt.Sprintf(" AND channel = $%d", len(args))
	}
	if filter.AssignedTo != "" {
		args = append(args, filter.AssignedTo)
		query += fmt.Sprintf(" AND assigned_to = $%d", len(args))
	}
	if filter.MemberID != "" {
		args = append(args, filter.MemberID)
		query += fmt.Sprintf(" AND member_id = $%d", len(args))
	}
	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		query += fmt.Sprintf(
			" AND (lower(contact_name) LIKE $%d OR lower(subject) LIKE $%d)", len(args), len(args))
	}
	// Oldest unanswered first is the order the desk should work in, so the
	// list arrives that way rather than being re-sorted by whoever renders it.
	query += ` ORDER BY (last_staff_at IS NULL OR last_staff_at < last_member_at) DESC,
	           last_member_at NULLS LAST, updated_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing conversations: %w", err)
	}
	defer rows.Close()

	out := []domain.Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning conversation: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Conversation(ctx context.Context, id string, forUpdate bool) (domain.Conversation, error) {
	query := `SELECT ` + conversationColumns + ` FROM crm.conversations WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	c, err := scanConversation(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.Conversation{}, httpx.NotFound("conversation")
	}
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("crm: reading conversation: %w", err)
	}
	return c, nil
}

// ConversationByHandle finds an open thread with the same person on the same
// channel, so an inbound message continues a conversation rather than starting
// a new one every time somebody writes.
func (r *Repository) ConversationByHandle(ctx context.Context, channel, handle string) (domain.Conversation, bool, error) {
	c, err := scanConversation(r.db.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM crm.conversations
		 WHERE channel = $1 AND contact_handle = $2 AND status <> 'CLOSED'
		 ORDER BY updated_at DESC LIMIT 1`, channel, handle))
	if database.IsNoRows(err) {
		return domain.Conversation{}, false, nil
	}
	if err != nil {
		return domain.Conversation{}, false, fmt.Errorf("crm: finding conversation: %w", err)
	}
	return c, true, nil
}

func (r *Repository) InsertConversation(ctx context.Context, c domain.Conversation) (domain.Conversation, error) {
	created, err := scanConversation(r.db.QueryRow(ctx, `
		INSERT INTO crm.conversations (id, member_id, contact_name, contact_handle, channel,
			subject, status, priority, assigned_to, assigned_name, branch_id, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING `+conversationColumns,
		c.ID, c.MemberID, c.ContactName, c.ContactHandle, c.Channel, c.Subject, c.Status,
		c.Priority, c.AssignedTo, c.AssignedName, c.BranchID, c.Tags))
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("crm: inserting conversation: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveConversation(ctx context.Context, c domain.Conversation) (domain.Conversation, error) {
	saved, err := scanConversation(r.db.QueryRow(ctx, `
		UPDATE crm.conversations SET member_id = $2, contact_name = $3, subject = $4,
			status = $5, priority = $6, assigned_to = $7, assigned_name = $8, tags = $9,
			last_member_at = $10, last_staff_at = $11, first_response_seconds = $12,
			resolved_at = $13, updated_at = now()
		WHERE id = $1 RETURNING `+conversationColumns,
		c.ID, c.MemberID, c.ContactName, c.Subject, c.Status, c.Priority, c.AssignedTo,
		c.AssignedName, c.Tags, c.LastMemberAt, c.LastStaffAt, c.FirstResponseSeconds,
		c.ResolvedAt))
	if database.IsNoRows(err) {
		return domain.Conversation{}, httpx.NotFound("conversation")
	}
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("crm: saving conversation: %w", err)
	}
	return saved, nil
}

const messageColumns = `id, conversation_id, direction, body, author_id, author_name,
	template_id, external_id, internal, created_at`

func scanMessage(row pgx.Row) (domain.ConversationMessage, error) {
	var m domain.ConversationMessage
	err := row.Scan(&m.ID, &m.ConversationID, &m.Direction, &m.Body, &m.AuthorID,
		&m.AuthorName, &m.TemplateID, &m.ExternalID, &m.Internal, &m.CreatedAt)
	return m, err
}

func (r *Repository) Messages(ctx context.Context, conversationID string) ([]domain.ConversationMessage, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+messageColumns+` FROM crm.conversation_messages
		 WHERE conversation_id = $1 ORDER BY created_at`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("crm: listing messages: %w", err)
	}
	defer rows.Close()

	out := []domain.ConversationMessage{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// InsertMessage posts one message. A duplicate external id is not an error:
// providers redeliver, and the second copy is the same message rather than a
// second one.
func (r *Repository) InsertMessage(ctx context.Context, m domain.ConversationMessage) (domain.ConversationMessage, bool, error) {
	created, err := scanMessage(r.db.QueryRow(ctx, `
		INSERT INTO crm.conversation_messages (id, conversation_id, direction, body,
			author_id, author_name, template_id, external_id, internal)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (external_id) WHERE external_id IS NOT NULL DO NOTHING
		RETURNING `+messageColumns,
		m.ID, m.ConversationID, m.Direction, m.Body, m.AuthorID, m.AuthorName,
		m.TemplateID, m.ExternalID, m.Internal))
	if database.IsNoRows(err) {
		return domain.ConversationMessage{}, false, nil
	}
	if database.IsForeignKeyViolation(err) {
		return domain.ConversationMessage{}, false, httpx.NotFound("conversation")
	}
	if err != nil {
		return domain.ConversationMessage{}, false, fmt.Errorf("crm: inserting message: %w", err)
	}
	return created, true, nil
}

// ── Templates ────────────────────────────────────────────────────────────────

const templateColumns = `id, code, name, channel, subject, body, variables, active`

// MessageTemplate is words written once and sent many times.
type MessageTemplate struct {
	ID        string   `json:"id"`
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Channel   string   `json:"channel"`
	Subject   *string  `json:"subject"`
	Body      string   `json:"body"`
	Variables []string `json:"variables"`
	Active    bool     `json:"active"`
}

func scanTemplate(row pgx.Row) (MessageTemplate, error) {
	var t MessageTemplate
	err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Channel, &t.Subject, &t.Body,
		&t.Variables, &t.Active)
	return t, err
}

func (r *Repository) Templates(ctx context.Context, channel string) ([]MessageTemplate, error) {
	query := `SELECT ` + templateColumns + ` FROM engagement.message_templates WHERE 1 = 1`
	args := []any{}
	if channel != "" {
		args = append(args, channel)
		query += fmt.Sprintf(" AND channel = $%d", len(args))
	}
	query += ` ORDER BY name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing templates: %w", err)
	}
	defer rows.Close()

	out := []MessageTemplate{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning template: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) Template(ctx context.Context, id string) (MessageTemplate, error) {
	t, err := scanTemplate(r.db.QueryRow(ctx,
		`SELECT `+templateColumns+` FROM engagement.message_templates WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return MessageTemplate{}, httpx.NotFound("template")
	}
	if err != nil {
		return MessageTemplate{}, fmt.Errorf("crm: reading template: %w", err)
	}
	return t, nil
}

func (r *Repository) UpsertTemplate(ctx context.Context, t MessageTemplate) (MessageTemplate, error) {
	saved, err := scanTemplate(r.db.QueryRow(ctx, `
		INSERT INTO engagement.message_templates (id, code, name, channel, subject, body,
			variables, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, channel = EXCLUDED.channel,
			subject = EXCLUDED.subject, body = EXCLUDED.body, variables = EXCLUDED.variables,
			active = EXCLUDED.active, updated_at = now()
		RETURNING `+templateColumns,
		t.ID, t.Code, t.Name, t.Channel, t.Subject, t.Body, t.Variables, t.Active))
	if err != nil {
		return MessageTemplate{}, fmt.Errorf("crm: saving template: %w", err)
	}
	return saved, nil
}

// ── Campaign delivery ────────────────────────────────────────────────────────

const recipientColumns = `id, campaign_id, member_id, status, skip_reason, notification_id,
	sent_at, opened_at, clicked_at, created_at`

func scanRecipient(row pgx.Row) (domain.CampaignRecipient, error) {
	var r domain.CampaignRecipient
	err := row.Scan(&r.ID, &r.CampaignID, &r.MemberID, &r.Status, &r.SkipReason,
		&r.NotificationID, &r.SentAt, &r.OpenedAt, &r.ClickedAt, &r.CreatedAt)
	return r, err
}

func (r *Repository) CampaignRecipients(ctx context.Context, campaignID string) ([]domain.CampaignRecipient, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+recipientColumns+` FROM engagement.campaign_recipients
		 WHERE campaign_id = $1 ORDER BY created_at`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("crm: listing campaign recipients: %w", err)
	}
	defer rows.Close()

	out := []domain.CampaignRecipient{}
	for rows.Next() {
		recipient, err := scanRecipient(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning campaign recipient: %w", err)
		}
		out = append(out, recipient)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertRecipient(ctx context.Context, rec domain.CampaignRecipient) error {
	var sentAt any
	if rec.SentAt != nil {
		sentAt = *rec.SentAt
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO engagement.campaign_recipients (id, campaign_id, member_id, status,
			skip_reason, notification_id, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (campaign_id, member_id) DO UPDATE SET status = EXCLUDED.status,
			skip_reason = EXCLUDED.skip_reason, notification_id = EXCLUDED.notification_id,
			sent_at = EXCLUDED.sent_at`,
		rec.ID, rec.CampaignID, rec.MemberID, rec.Status, rec.SkipReason,
		rec.NotificationID, sentAt); err != nil {
		return fmt.Errorf("crm: saving campaign recipient: %w", err)
	}
	return nil
}

// MarkRecipient records an open or a click, once. The first one wins: a member
// who opens a message three times opened it, not three times over.
func (r *Repository) MarkRecipient(ctx context.Context, campaignID, memberID, event string, at time.Time) error {
	column := "opened_at"
	status := string(domain.RecipientOpened)
	if event == "CLICKED" {
		column = "clicked_at"
		status = string(domain.RecipientClicked)
	}
	// A click implies an open, so it fills both.
	set := column + " = COALESCE(" + column + ", $3)"
	if event == "CLICKED" {
		set += ", opened_at = COALESCE(opened_at, $3)"
	}
	if _, err := r.db.Exec(ctx, `
		UPDATE engagement.campaign_recipients
		SET `+set+`, status = $4
		WHERE campaign_id = $1 AND member_id = $2`,
		campaignID, memberID, at, status); err != nil {
		return fmt.Errorf("crm: marking campaign recipient: %w", err)
	}
	return nil
}

// MemberMetrics reads the counters a badge is measured against.
//
// One query rather than five: the evaluation runs after every visit and every
// sale, and five round trips per member would make an achievement cost more
// than it is worth.
func (r *Repository) MemberMetrics(ctx context.Context, memberID string) (domain.MemberMetrics, error) {
	var m domain.MemberMetrics
	err := r.db.QueryRow(ctx, `
		SELECT
			-- A visit is an allowed scan. Counting every scan would let
			-- somebody earn a badge at a locked door.
			(SELECT count(*) FROM access.access_logs
			   WHERE member_id = $1 AND result IN ('ALLOWED', 'OFFLINE_ALLOWED')),
			(SELECT count(*) FROM scheduling.bookings WHERE member_id = $1
			   AND status IN ('CONFIRMED', 'CHECKED_IN', 'COMPLETED')),
			COALESCE((SELECT lifetime_spend_idr FROM crm.member_profiles WHERE member_id = $1), 0),
			COALESCE((SELECT lifetime_xp FROM crm.member_profiles WHERE member_id = $1), 0),
			0,
			0`, memberID).
		Scan(&m.Visits, &m.Bookings, &m.SpendIDR, &m.XPEarned, &m.StreakDays, &m.Referrals)
	if err != nil {
		return domain.MemberMetrics{}, fmt.Errorf("crm: reading member metrics: %w", err)
	}
	return m, nil
}

// VisitBehind finds the visit that backs a review, if there is one.
//
// A review of a class the member did not attend is not refused — they may have
// been there and the visit not logged — but it is marked so a report can weigh
// verified and unverified opinions differently.
func (r *Repository) VisitBehind(ctx context.Context, memberID, subjectType, subjectID string) (bool, *string, error) {
	if subjectType != "SESSION" {
		return false, nil, nil
	}
	// The visit is the scan that let them in, found through the booking it
	// was made against.
	var visitID string
	err := r.db.QueryRow(ctx, `
		SELECT l.id FROM access.access_logs l
		JOIN scheduling.bookings b ON b.id = l.booking_id
		WHERE l.member_id = $1 AND b.session_id = $2
		  AND l.result IN ('ALLOWED', 'OFFLINE_ALLOWED')
		ORDER BY l.created_at DESC LIMIT 1`, memberID, subjectID).Scan(&visitID)
	if database.IsNoRows(err) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("crm: finding the visit behind a review: %w", err)
	}
	return true, &visitID, nil
}
