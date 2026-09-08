package engagement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Campaigns: a message, an audience, and the send that turns one into the
// other.

// AudienceMember is one person a campaign would reach.
type AudienceMember struct {
	ID   string
	Name string
}

// Audience resolves a segment to the members in it.
//
// Segments are defined in terms of balances, visits and join dates, all of
// which belong to other modules. Engagement declares what it needs and the
// application layer supplies it.
type Audience interface {
	Resolve(ctx context.Context, segment domain.MemberSegment, filter *domain.SegmentFilter) ([]AudienceMember, error)
}

// ── Storage ──────────────────────────────────────────────────────────────────

const campaignColumns = `id, name, segment, custom_filter, message, deep_link, image_url,
	scheduled_at, status, sent_count, created_at`

func scanCampaign(scan func(...any) error) (domain.Campaign, error) {
	var c domain.Campaign
	var filter []byte
	if err := scan(&c.ID, &c.Name, &c.Segment, &filter, &c.Message, &c.DeepLink, &c.ImageURL,
		&c.ScheduledAt, &c.Status, &c.SentCount, &c.CreatedAt); err != nil {
		return domain.Campaign{}, err
	}
	if len(filter) > 0 {
		_ = json.Unmarshal(filter, &c.CustomFilter)
	}
	return c, nil
}

func (r *Repository) Campaigns(ctx context.Context) ([]domain.Campaign, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+campaignColumns+` FROM engagement.campaigns ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("engagement: listing campaigns: %w", err)
	}
	defer rows.Close()

	out := []domain.Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("engagement: scanning campaign: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Campaign(ctx context.Context, id string) (domain.Campaign, error) {
	c, err := scanCampaign(r.db.QueryRow(ctx,
		`SELECT `+campaignColumns+` FROM engagement.campaigns WHERE id = $1`, id).Scan)
	if database.IsNoRows(err) {
		return domain.Campaign{}, httpx.NotFound("campaign")
	}
	if err != nil {
		return domain.Campaign{}, fmt.Errorf("engagement: reading campaign: %w", err)
	}
	return c, nil
}

func (r *Repository) UpsertCampaign(ctx context.Context, c domain.Campaign) (domain.Campaign, error) {
	filter, err := json.Marshal(c.CustomFilter)
	if err != nil {
		return domain.Campaign{}, fmt.Errorf("engagement: encoding filter: %w", err)
	}
	if c.CustomFilter == nil {
		filter = nil
	}
	saved, err := scanCampaign(r.db.QueryRow(ctx, `
		INSERT INTO engagement.campaigns
			(id, name, segment, custom_filter, message, deep_link, image_url, scheduled_at, status, sent_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, segment = EXCLUDED.segment, custom_filter = EXCLUDED.custom_filter,
			message = EXCLUDED.message, deep_link = EXCLUDED.deep_link, image_url = EXCLUDED.image_url,
			scheduled_at = EXCLUDED.scheduled_at, status = EXCLUDED.status,
			sent_count = EXCLUDED.sent_count, updated_at = now()
		RETURNING `+campaignColumns,
		c.ID, c.Name, c.Segment, filter, c.Message, c.DeepLink, c.ImageURL,
		c.ScheduledAt, c.Status, c.SentCount).Scan)
	if err != nil {
		return domain.Campaign{}, fmt.Errorf("engagement: saving campaign: %w", err)
	}
	return saved, nil
}

func (r *Repository) DeleteCampaign(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM engagement.campaigns WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("engagement: deleting campaign: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("campaign")
	}
	return nil
}

// ── Use cases ────────────────────────────────────────────────────────────────

// CampaignInput is a campaign as the panel submits it.
type CampaignInput struct {
	Name         string
	Segment      domain.MemberSegment
	CustomFilter *domain.SegmentFilter
	Message      string
	DeepLink     *string
	ImageURL     *string
	ScheduledAt  *time.Time
}

func (s *Service) Campaigns(ctx context.Context) ([]domain.Campaign, error) {
	return s.repo.Campaigns(ctx)
}

func (s *Service) SaveCampaign(ctx context.Context, campaignID string, in CampaignInput) (domain.Campaign, error) {
	if err := validateCampaign(in); err != nil {
		return domain.Campaign{}, err
	}

	campaign := domain.Campaign{
		ID: s.ids.New(id.Campaign), Status: domain.CampaignDraft,
	}
	if campaignID != "" {
		existing, err := s.repo.Campaign(ctx, campaignID)
		if err != nil {
			return domain.Campaign{}, err
		}
		// A campaign that has gone out is a record of what was said to whom.
		if existing.Status == domain.CampaignSent || existing.Status == domain.CampaignProcessing {
			return domain.Campaign{}, httpx.Conflict("ALREADY_SENT",
				"That campaign has already been sent.")
		}
		campaign = existing
	}

	campaign.Name = strings.TrimSpace(in.Name)
	campaign.Segment = in.Segment
	campaign.CustomFilter = in.CustomFilter
	campaign.Message = strings.TrimSpace(in.Message)
	campaign.DeepLink = in.DeepLink
	campaign.ImageURL = in.ImageURL
	campaign.ScheduledAt = in.ScheduledAt
	// Giving a draft a date schedules it; taking the date away puts it back
	// in the drawer.
	if campaign.Status == domain.CampaignDraft && in.ScheduledAt != nil {
		campaign.Status = domain.CampaignScheduled
	}
	if campaign.Status == domain.CampaignScheduled && in.ScheduledAt == nil {
		campaign.Status = domain.CampaignDraft
	}
	return s.repo.UpsertCampaign(ctx, campaign)
}

func validateCampaign(in CampaignInput) error {
	if len(strings.TrimSpace(in.Name)) < 2 {
		return httpx.Invalid("A campaign needs a name.")
	}
	if len(strings.TrimSpace(in.Message)) < 3 {
		return httpx.Invalid("A campaign needs something to say.")
	}
	if !domain.IsValidSegment(string(in.Segment)) {
		return httpx.Invalid("That is not an audience.")
	}
	if in.Segment == domain.SegmentCustom && in.CustomFilter == nil {
		return httpx.Invalid("A custom audience needs at least one condition.")
	}
	return nil
}

func (s *Service) DeleteCampaign(ctx context.Context, campaignID string) error {
	campaign, err := s.repo.Campaign(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign.Status == domain.CampaignSent {
		return httpx.ErrInUse.WithMessage(
			"A campaign that has been sent is a record of what was said. Cancel it instead.")
	}
	return s.repo.DeleteCampaign(ctx, campaignID)
}

// PreviewAudience is how many people a segment reaches, and a few of them.
type PreviewAudience struct {
	Count  int      `json:"count"`
	Sample []string `json:"sample"`
}

// sampleSize is how many names to show. Enough to recognise the audience,
// few enough that nobody mistakes it for the list.
const sampleSize = 5

func (s *Service) PreviewAudience(ctx context.Context, segment domain.MemberSegment, filter *domain.SegmentFilter) (PreviewAudience, error) {
	if !domain.IsValidSegment(string(segment)) {
		return PreviewAudience{}, httpx.Invalid("That is not an audience.")
	}
	members, err := s.audience.Resolve(ctx, segment, filter)
	if err != nil {
		return PreviewAudience{}, err
	}
	sample := []string{}
	for i, m := range members {
		if i >= sampleSize {
			break
		}
		sample = append(sample, m.Name)
	}
	return PreviewAudience{Count: len(members), Sample: sample}, nil
}

// Send delivers a campaign to its audience as in-app notifications.
//
// The campaign moves through PROCESSING first, so a crash halfway leaves a
// state somebody can see and act on rather than a DRAFT that has already
// reached half the membership. The count of what actually landed is recorded,
// not the size of the audience: those are different numbers when a send fails
// partway.
func (s *Service) Send(ctx context.Context, campaignID string) (domain.Campaign, error) {
	campaign, err := s.repo.Campaign(ctx, campaignID)
	if err != nil {
		return domain.Campaign{}, err
	}
	next, err := domain.Transition(domain.CampaignTransitions, campaign.Status, domain.CampaignProcessing)
	if err != nil {
		return domain.Campaign{}, httpx.Conflict("INVALID_TRANSITION",
			"A %s campaign cannot be sent.", strings.ToLower(string(campaign.Status)))
	}
	campaign.Status = next
	if campaign, err = s.repo.UpsertCampaign(ctx, campaign); err != nil {
		return domain.Campaign{}, err
	}

	members, err := s.audience.Resolve(ctx, campaign.Segment, campaign.CustomFilter)
	if err != nil {
		return s.failSend(ctx, campaign, err)
	}

	sent := 0
	for _, member := range members {
		// One notification per member per campaign, so pressing Send twice
		// after a timeout does not deliver the message twice.
		key := "campaign:" + campaign.ID + ":" + member.ID
		if err := s.Notify(ctx, member.ID, domain.NotifyAnnouncement,
			campaign.Name, campaign.Message, key); err != nil {
			return s.failSend(ctx, campaign, err)
		}
		sent++
	}

	campaign.Status = domain.CampaignSent
	campaign.SentCount = &sent
	return s.repo.UpsertCampaign(ctx, campaign)
}

// failSend parks a campaign in FAILED so the panel shows what happened rather
// than leaving it stuck in PROCESSING for ever.
func (s *Service) failSend(ctx context.Context, campaign domain.Campaign, cause error) (domain.Campaign, error) {
	campaign.Status = domain.CampaignFailed
	if _, err := s.repo.UpsertCampaign(ctx, campaign); err != nil {
		return domain.Campaign{}, err
	}
	return domain.Campaign{}, cause
}
