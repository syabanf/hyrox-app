package engagement

import (
	"net/http"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// The studio's side of engagement: writing a campaign, seeing who it would
// reach, sending it, and running community challenges.

func (h *Handler) mountAdmin(r *httpx.Router) {
	// Writing to the whole membership is its own grant: it is not the same
	// permission as editing one person's record.
	manage := h.guard.RequireAdmin(string(domain.PermCampaignsManage))
	view := h.guard.RequireAdmin(string(domain.PermEngagementView))

	r.Get("/api/admin/campaigns", h.listCampaigns, view)
	r.Post("/api/admin/campaigns", h.createCampaign, manage)
	r.Patch("/api/admin/campaigns/{id}", h.updateCampaign, manage)
	r.Delete("/api/admin/campaigns/{id}", h.deleteCampaign, manage)
	r.Post("/api/admin/campaigns/{id}/send", h.sendCampaign, manage)
	r.Post("/api/admin/segments/preview", h.previewSegment, view)

	r.Get("/api/admin/challenges", h.listChallenges, view)
	r.Post("/api/admin/challenges", h.createChallenge, manage)
	r.Patch("/api/admin/challenges/{id}", h.updateChallenge, manage)
	r.Delete("/api/admin/challenges/{id}", h.deleteChallenge, manage)

	// One announcement, opened from a notification. Members read it, so it is
	// a member route rather than an admin one.
	r.Get("/api/announcements/{id}", h.announcement, h.guard.RequireMember)
}

// ── Campaigns ────────────────────────────────────────────────────────────────

func (h *Handler) listCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := h.service.Campaigns(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, campaigns)
}

type campaignRequest struct {
	Name         string                `json:"name"`
	Segment      string                `json:"segment"`
	CustomFilter *domain.SegmentFilter `json:"customFilter"`
	Message      string                `json:"message"`
	DeepLink     *string               `json:"deepLink"`
	ImageURL     *string               `json:"imageUrl"`
	ScheduledAt  *time.Time            `json:"scheduledAt"`
}

func (c *campaignRequest) Validate() error {
	if !domain.IsValidSegment(c.Segment) {
		return httpx.Invalid("Pick an audience.")
	}
	return nil
}

func (c campaignRequest) input() CampaignInput {
	return CampaignInput{
		Name: c.Name, Segment: domain.MemberSegment(c.Segment), CustomFilter: c.CustomFilter,
		Message: c.Message, DeepLink: c.DeepLink, ImageURL: c.ImageURL, ScheduledAt: c.ScheduledAt,
	}
}

func (h *Handler) createCampaign(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[campaignRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	campaign, err := h.service.SaveCampaign(r.Context(), "", body.input())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, campaign)
}

func (h *Handler) updateCampaign(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[campaignRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	campaign, err := h.service.SaveCampaign(r.Context(), httpx.Param(r, "id"), body.input())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, campaign)
}

func (h *Handler) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteCampaign(r.Context(), httpx.Param(r, "id")); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

func (h *Handler) sendCampaign(w http.ResponseWriter, r *http.Request) {
	campaign, err := h.service.Send(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, campaign)
}

type segmentPreviewRequest struct {
	Segment      string                `json:"segment"`
	CustomFilter *domain.SegmentFilter `json:"customFilter"`
}

func (s *segmentPreviewRequest) Validate() error {
	if !domain.IsValidSegment(s.Segment) {
		return httpx.Invalid("That is not an audience.")
	}
	return nil
}

func (h *Handler) previewSegment(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[segmentPreviewRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	preview, err := h.service.PreviewAudience(r.Context(),
		domain.MemberSegment(body.Segment), body.CustomFilter)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, preview)
}

// ── Challenges ───────────────────────────────────────────────────────────────

func (h *Handler) listChallenges(w http.ResponseWriter, r *http.Request) {
	challenges, err := h.service.Challenges(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, challenges)
}

type challengeRequest struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	TargetKm    float64   `json:"targetKm"`
	StartsAt    time.Time `json:"startsAt"`
	EndsAt      time.Time `json:"endsAt"`
}

func (c *challengeRequest) Validate() error {
	if c.Type != domain.ChallengeAnyType && !domain.IsValidActivityType(c.Type) {
		return httpx.Invalid("A challenge counts ANY activity, or one type of it.")
	}
	if c.TargetKm <= 0 {
		return httpx.Invalid("A challenge needs a distance to aim at.")
	}
	return nil
}

func (c challengeRequest) input() ChallengeInput {
	return ChallengeInput{
		Name: c.Name, Description: c.Description, Type: c.Type,
		TargetKm: c.TargetKm, StartsAt: c.StartsAt, EndsAt: c.EndsAt,
	}
}

func (h *Handler) createChallenge(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[challengeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	challenge, err := h.service.SaveChallenge(r.Context(), "", body.input())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, challenge)
}

func (h *Handler) updateChallenge(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[challengeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	challenge, err := h.service.SaveChallenge(r.Context(), httpx.Param(r, "id"), body.input())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, challenge)
}

func (h *Handler) deleteChallenge(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteChallenge(r.Context(), httpx.Param(r, "id")); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

// ── Announcements ────────────────────────────────────────────────────────────

// announcement is a sent campaign, opened from the notification that carried
// it. Only sent campaigns are readable: a draft is not an announcement.
func (h *Handler) announcement(w http.ResponseWriter, r *http.Request) {
	campaign, err := h.service.Campaign(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if campaign.Status != domain.CampaignSent {
		httpx.Fail(w, r, httpx.NotFound("announcement"))
		return
	}
	httpx.OK(w, map[string]any{
		"id": campaign.ID, "title": campaign.Name, "message": campaign.Message,
		"deepLink": campaign.DeepLink, "imageUrl": campaign.ImageURL,
		"createdAt": campaign.CreatedAt,
	})
}
