// Package engagement reaches members: in-app notifications and the campaigns
// that produce them.
//
// This is the first slice of the module — the member-facing read side, which
// is what the app needs to render its bell and its announcement feed. Campaign
// authoring and segment sending build on the same tables.
package engagement

import (
	"context"
	"fmt"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
	"github.com/syabanf/nuhabit-backend/internal/platform/outbox"
)

// Repository persists notifications and campaigns.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

const notificationColumns = `id, member_id, type, title, body, created_at, read_at`

func (r *Repository) Notifications(ctx context.Context, memberID string, limit int) ([]domain.MemberNotification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+notificationColumns+` FROM engagement.member_notifications
		WHERE member_id = $1 ORDER BY created_at DESC LIMIT $2`, memberID, limit)
	if err != nil {
		return nil, fmt.Errorf("engagement: listing notifications: %w", err)
	}
	defer rows.Close()

	out := []domain.MemberNotification{}
	for rows.Next() {
		var n domain.MemberNotification
		if err := rows.Scan(&n.ID, &n.MemberID, &n.Type, &n.Title, &n.Body, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, fmt.Errorf("engagement: scanning notification: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *Repository) UnreadCount(ctx context.Context, memberID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM engagement.member_notifications WHERE member_id = $1 AND read_at IS NULL`,
		memberID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("engagement: counting unread: %w", err)
	}
	return count, nil
}

func (r *Repository) MarkAllRead(ctx context.Context, memberID string, at time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE engagement.member_notifications SET read_at = $2 WHERE member_id = $1 AND read_at IS NULL`,
		memberID, at)
	if err != nil {
		return 0, fmt.Errorf("engagement: marking read: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) Insert(ctx context.Context, n domain.MemberNotification, campaignID *string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO engagement.member_notifications (id, member_id, type, title, body, campaign_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		n.ID, n.MemberID, n.Type, n.Title, n.Body, campaignID, n.CreatedAt)
	if err != nil {
		return fmt.Errorf("engagement: inserting notification: %w", err)
	}
	return nil
}

// MarkOnce records that an automatic notification has been sent, so a member
// is never told the same thing twice.
func (r *Repository) MarkOnce(ctx context.Context, key string, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`INSERT INTO engagement.notification_marks (mark_key, created_at) VALUES ($1, $2)
		 ON CONFLICT (mark_key) DO NOTHING`, key, at)
	if err != nil {
		return false, fmt.Errorf("engagement: claiming notification mark: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// SentCampaigns returns the announcements the member feed shows.
func (r *Repository) SentCampaigns(ctx context.Context, limit int) ([]domain.Campaign, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, name, message, deep_link, image_url, created_at
		FROM engagement.campaigns WHERE status = 'SENT'
		ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("engagement: listing campaigns: %w", err)
	}
	defer rows.Close()

	out := []domain.Campaign{}
	for rows.Next() {
		var c domain.Campaign
		if err := rows.Scan(&c.ID, &c.Name, &c.Message, &c.DeepLink, &c.ImageURL, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("engagement: scanning campaign: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Service implements the engagement use cases.
type Service struct {
	repo     *Repository
	audience Audience
	ids      id.Generator
	clock    clock.Clock
}

func NewService(repo *Repository, audience Audience, ids id.Generator, c clock.Clock) *Service {
	return &Service{repo: repo, audience: audience, ids: ids, clock: c}
}

// Campaign reads one campaign by id.
func (s *Service) Campaign(ctx context.Context, id string) (domain.Campaign, error) {
	return s.repo.Campaign(ctx, id)
}

func (s *Service) Notifications(ctx context.Context, memberID string, limit int) ([]domain.MemberNotification, error) {
	return s.repo.Notifications(ctx, memberID, limit)
}

func (s *Service) UnreadCount(ctx context.Context, memberID string) (int, error) {
	return s.repo.UnreadCount(ctx, memberID)
}

func (s *Service) MarkAllRead(ctx context.Context, memberID string) (int64, error) {
	return s.repo.MarkAllRead(ctx, memberID, s.clock.Now())
}

func (s *Service) SentCampaigns(ctx context.Context, limit int) ([]domain.Campaign, error) {
	return s.repo.SentCampaigns(ctx, limit)
}

// Notify writes one in-app message. Passing a key makes it at-most-once, which
// is what the reminder jobs need when they run every few minutes.
func (s *Service) Notify(ctx context.Context, memberID string, kind domain.MemberNotificationType, title, body string, onceKey string) error {
	if onceKey != "" {
		claimed, err := s.repo.MarkOnce(ctx, onceKey, s.clock.Now())
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
	}
	return s.repo.Insert(ctx, domain.MemberNotification{
		ID:        s.ids.New(id.Notification),
		MemberID:  memberID,
		Type:      kind,
		Title:     title,
		Body:      body,
		CreatedAt: s.clock.Now(),
	}, nil)
}

// HandleBookingConfirmed turns the outbox event into a member notification.
// This is the consumer side of the transactional outbox: scheduling published
// the fact, and engagement decides what to say about it.
func (s *Service) HandleBookingConfirmed(ctx context.Context, msg outbox.Message) error {
	payload, err := outbox.Decode[struct {
		BookingID string    `json:"bookingId"`
		MemberID  string    `json:"memberId"`
		StartsAt  time.Time `json:"startsAt"`
	}](msg)
	if err != nil {
		return err
	}
	if payload.MemberID == "" {
		return nil
	}
	return s.Notify(ctx, payload.MemberID, domain.NotifyBookingConfirmed,
		"You are booked in",
		fmt.Sprintf("Your class on %s is confirmed.", payload.StartsAt.Format("Mon 2 Jan, 15:04")),
		"booking-confirmed:"+payload.BookingID)
}

// HandleWaitlistPromoted tells a waiting member a place opened up.
func (s *Service) HandleWaitlistPromoted(ctx context.Context, msg outbox.Message) error {
	payload, err := outbox.Decode[struct {
		BookingID   string    `json:"bookingId"`
		MemberID    string    `json:"memberId"`
		StartsAt    time.Time `json:"startsAt"`
		AutoPromote bool      `json:"autoPromote"`
	}](msg)
	if err != nil {
		return err
	}
	if payload.MemberID == "" {
		return nil
	}
	body := fmt.Sprintf("A place opened up for %s and it is yours.", payload.StartsAt.Format("Mon 2 Jan, 15:04"))
	if !payload.AutoPromote {
		body = fmt.Sprintf("A place opened up for %s. Confirm it before someone else does.",
			payload.StartsAt.Format("Mon 2 Jan, 15:04"))
	}
	return s.Notify(ctx, payload.MemberID, domain.NotifyWaitlistPromoted,
		"You are off the waitlist", body, "waitlist-promoted:"+payload.BookingID)
}

// HandlePaymentPaid confirms a top-up landed.
func (s *Service) HandlePaymentPaid(ctx context.Context, msg outbox.Message) error {
	payload, err := outbox.Decode[struct {
		PaymentID string `json:"paymentId"`
		MemberID  string `json:"memberId"`
		Credits   int    `json:"credits"`
	}](msg)
	if err != nil {
		return err
	}
	if payload.MemberID == "" {
		return nil
	}
	return s.Notify(ctx, payload.MemberID, domain.NotifyLowBalance,
		"Credits added",
		fmt.Sprintf("%d credits are now in your wallet.", payload.Credits),
		"payment-paid:"+payload.PaymentID)
}
