package crm

import (
	"net/http"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the loyalty surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }
	view := admin(domain.PermCRMView)
	manage := admin(domain.PermCRMManage)
	approve := admin(domain.PermCRMApprove)
	adjust := admin(domain.PermCRMAdjust)

	r.Get("/api/admin/crm/overview", h.overview, view)

	r.Get("/api/admin/crm/members", h.listMembers, view)
	r.Get("/api/admin/crm/members/{id}", h.getMember, view)
	// Handing out points by hand is its own grant, for the same reason
	// adjusting somebody's credits is.
	r.Post("/api/admin/crm/members/{id}/adjust", h.adjust, adjust)
	r.Post("/api/admin/crm/members/{id}/redeem", h.redeem, approve)

	r.Get("/api/admin/crm/ledger", h.ledger, view)

	r.Get("/api/admin/crm/tiers", h.listTiers, view)
	r.Put("/api/admin/crm/tiers", h.saveTier, manage)

	r.Get("/api/admin/crm/rules", h.listRules, view)
	r.Put("/api/admin/crm/rules", h.saveRule, manage)

	r.Get("/api/admin/crm/rewards", h.listRewards, view)
	r.Put("/api/admin/crm/rewards", h.saveReward, manage)

	r.Get("/api/admin/crm/redemptions", h.listRedemptions, view)
	r.Post("/api/admin/crm/redemptions/{id}/{action}", h.decideRedemption, approve)

	// Badges: what a member has done, as opposed to what they have spent.
	r.Get("/api/admin/crm/badges", h.listBadges, view)
	r.Put("/api/admin/crm/badges", h.saveBadge, manage)
	r.Get("/api/admin/crm/members/{id}/badges", h.memberBadges, view)
	r.Post("/api/admin/crm/members/{id}/badges", h.awardBadge, adjust)
	r.Delete("/api/admin/crm/members/{id}/badges/{badgeId}", h.revokeBadge, adjust)
	r.Post("/api/admin/crm/members/{id}/badges/evaluate", h.evaluateBadges, manage)

	// Consent. An opt-out is a hard exclusion, not a preference.
	r.Get("/api/admin/crm/members/{id}/consent", h.listConsent, view)
	r.Put("/api/admin/crm/members/{id}/consent", h.setConsent, manage)

	// The inbox.
	r.Get("/api/admin/crm/inbox", h.inboxOverview, view)
	r.Get("/api/admin/crm/conversations", h.listConversations, view)
	r.Post("/api/admin/crm/conversations", h.openConversation, manage)
	r.Get("/api/admin/crm/conversations/{id}", h.getConversation, view)
	r.Put("/api/admin/crm/conversations/{id}", h.updateConversation, manage)
	r.Post("/api/admin/crm/conversations/{id}/messages", h.reply, manage)

	r.Get("/api/admin/crm/templates", h.listTemplates, view)
	r.Put("/api/admin/crm/templates", h.saveTemplate, manage)
	r.Post("/api/admin/crm/templates/{id}/preview", h.previewTemplate, view)

	// Reviews.
	r.Get("/api/admin/crm/reviews", h.listReviews, view)
	r.Get("/api/admin/crm/reviews/summary", h.reviewSummary, view)
	r.Post("/api/admin/crm/reviews/{id}/reply", h.replyToReview, manage)
	r.Put("/api/admin/crm/reviews/{id}/status", h.setReviewStatus, manage)

	// What a campaign actually did, per member.
	r.Get("/api/admin/crm/campaigns/{id}/report", h.campaignReport, view)
	r.Post("/api/admin/crm/campaigns/{id}/mark", h.markCampaign, manage)

	// The member's own view of what they have earned.
	r.Get("/api/me/loyalty", h.myLoyalty, h.guard.RequireMember)
	r.Get("/api/me/badges", h.myBadges, h.guard.RequireMember)
	r.Get("/api/me/consent", h.myConsent, h.guard.RequireMember)
	r.Put("/api/me/consent", h.setMyConsent, h.guard.RequireMember)
	r.Post("/api/me/reviews", h.leaveReview, h.guard.RequireMember)
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.Overview(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summary)
}

// ── Members ──────────────────────────────────────────────────────────────────

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.service.Profiles(r.Context(), ProfileFilter{
		TierCode: strings.ToUpper(httpx.Query(r, "tierCode")),
		Status:   strings.ToUpper(httpx.Query(r, "status")),
		Limit:    httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, profiles)
}

func (h *Handler) getMember(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.MemberDetail(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, detail)
}

// myLoyalty is the member's own standing, which needs no admin permission
// because it is about themselves.
func (h *Handler) myLoyalty(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.MemberDetail(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, detail)
}

type adjustBody struct {
	XPDelta int    `json:"xpDelta"`
	Reason  string `json:"reason"`
}

func (a *adjustBody) Validate() error {
	if a.XPDelta == 0 {
		return httpx.Invalid("An adjustment of nothing is not an adjustment.")
	}
	if strings.TrimSpace(a.Reason) == "" {
		return httpx.Invalid("An adjustment needs a reason.")
	}
	return nil
}

func (h *Handler) adjust(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[adjustBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.service.Adjust(r.Context(), AdjustInput{
		MemberID: httpx.Param(r, "id"), XPDelta: body.XPDelta,
		Reason: strings.TrimSpace(body.Reason),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, result)
}

type redeemBody struct {
	RewardID string `json:"rewardId"`
}

func (b *redeemBody) Validate() error {
	if strings.TrimSpace(b.RewardID) == "" {
		return httpx.Invalid("A reward is required.")
	}
	return nil
}

func (h *Handler) redeem(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[redeemBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	claim, err := h.service.Redeem(r.Context(), httpx.Param(r, "id"), body.RewardID, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, claim)
}

func (h *Handler) ledger(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.Ledger(r.Context(),
		httpx.Query(r, "memberId"), strings.ToUpper(httpx.Query(r, "direction")),
		httpx.QueryInt(r, "limit", 100))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, entries)
}

// ── Configuration ────────────────────────────────────────────────────────────

func (h *Handler) listTiers(w http.ResponseWriter, r *http.Request) {
	tiers, err := h.service.Tiers(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, tiers)
}

type tierBody struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Rank            int      `json:"rank"`
	MinLifetimeXP   int      `json:"minLifetimeXp"`
	MinSpendIDR     float64  `json:"minSpendIdr"`
	XPMultiplier    float64  `json:"xpMultiplier"`
	DiscountPercent float64  `json:"discountPercent"`
	Benefits        []string `json:"benefits"`
	Colour          string   `json:"colour"`
	Active          *bool    `json:"active"`
}

func (t *tierBody) Validate() error {
	if strings.TrimSpace(t.Code) == "" || strings.TrimSpace(t.Name) == "" {
		return httpx.Invalid("A tier needs a code and a name.")
	}
	if t.Rank <= 0 {
		return httpx.Invalid("A tier's rank starts at 1.")
	}
	if t.MinLifetimeXP < 0 || t.MinSpendIDR < 0 {
		return httpx.Invalid("Thresholds cannot be negative.")
	}
	if t.DiscountPercent < 0 || t.DiscountPercent > 100 {
		return httpx.Invalid("A discount runs from 0 to 100 percent.")
	}
	return nil
}

func (h *Handler) saveTier(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[tierBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	benefits := body.Benefits
	if benefits == nil {
		benefits = []string{}
	}
	tier, err := h.service.SaveTier(r.Context(), TierInput{
		Code: body.Code, Name: body.Name, Rank: body.Rank,
		MinLifetimeXP: body.MinLifetimeXP, MinSpendIDR: body.MinSpendIDR,
		XPMultiplier: body.XPMultiplier, DiscountPercent: body.DiscountPercent,
		Benefits: benefits, Colour: body.Colour,
		Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, tier)
}

func (h *Handler) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.service.Rules(r.Context(),
		strings.ToUpper(httpx.Query(r, "channel")), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, rules)
}

type ruleBody struct {
	Code                  string     `json:"code"`
	Name                  string     `json:"name"`
	SourceChannel         string     `json:"sourceChannel"`
	SourceType            string     `json:"sourceType"`
	SourceID              *string    `json:"sourceId"`
	BranchID              *string    `json:"branchId"`
	XPMode                string     `json:"xpMode"`
	XPValue               float64    `json:"xpValue"`
	AmountStep            float64    `json:"amountStep"`
	MinAmount             float64    `json:"minAmount"`
	MaxXPPerEvent         *int       `json:"maxXpPerEvent"`
	TierMultiplierEnabled *bool      `json:"tierMultiplierEnabled"`
	Priority              int        `json:"priority"`
	StartsAt              *time.Time `json:"startsAt"`
	EndsAt                *time.Time `json:"endsAt"`
	Active                *bool      `json:"active"`
}

func (b *ruleBody) Validate() error {
	if strings.TrimSpace(b.Code) == "" || strings.TrimSpace(b.Name) == "" {
		return httpx.Invalid("A rule needs a code and a name.")
	}
	if !domain.IsValidXPChannel(strings.ToUpper(b.SourceChannel)) {
		return httpx.Invalid("%q is not an earning channel.", b.SourceChannel)
	}
	if !domain.IsValidXPMode(strings.ToUpper(b.XPMode)) {
		return httpx.Invalid("%q is not an earning mode.", b.XPMode)
	}
	if b.XPValue < 0 {
		return httpx.Invalid("A rule cannot award negative points.")
	}
	if b.MaxXPPerEvent != nil && *b.MaxXPPerEvent < 0 {
		return httpx.Invalid("A cap cannot be negative.")
	}
	return nil
}

func (h *Handler) saveRule(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[ruleBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rule, err := h.service.SaveRule(r.Context(), RuleInput{
		Code: body.Code, Name: body.Name,
		SourceChannel: domain.XPChannel(strings.ToUpper(body.SourceChannel)),
		SourceType:    strings.ToUpper(body.SourceType),
		SourceID:      body.SourceID, BranchID: body.BranchID,
		XPMode:  domain.XPMode(strings.ToUpper(body.XPMode)),
		XPValue: body.XPValue, AmountStep: body.AmountStep, MinAmount: body.MinAmount,
		MaxXPPerEvent:         body.MaxXPPerEvent,
		TierMultiplierEnabled: body.TierMultiplierEnabled == nil || *body.TierMultiplierEnabled,
		Priority:              body.Priority, StartsAt: body.StartsAt, EndsAt: body.EndsAt,
		Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, rule)
}

func (h *Handler) listRewards(w http.ResponseWriter, r *http.Request) {
	rewards, err := h.service.Rewards(r.Context(), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, rewards)
}

type rewardBody struct {
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	RewardType       string         `json:"rewardType"`
	XPCost           int            `json:"xpCost"`
	RequiredTierCode *string        `json:"requiredTierCode"`
	RewardValue      map[string]any `json:"rewardValue"`
	StockTotal       *int           `json:"stockTotal"`
	MaxPerMember     *int           `json:"maxPerMember"`
	ImageURL         *string        `json:"imageUrl"`
	StartsAt         *time.Time     `json:"startsAt"`
	EndsAt           *time.Time     `json:"endsAt"`
	Active           *bool          `json:"active"`
}

func (b *rewardBody) Validate() error {
	if strings.TrimSpace(b.Code) == "" || strings.TrimSpace(b.Name) == "" {
		return httpx.Invalid("A reward needs a code and a name.")
	}
	if !domain.IsValidRewardType(strings.ToUpper(b.RewardType)) {
		return httpx.Invalid("%q is not a reward type.", b.RewardType)
	}
	if b.XPCost < 0 {
		return httpx.Invalid("A reward cannot cost negative points.")
	}
	if b.StockTotal != nil && *b.StockTotal < 0 {
		return httpx.Invalid("Stock cannot be negative.")
	}
	return nil
}

func (h *Handler) saveReward(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[rewardBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	reward, err := h.service.SaveReward(r.Context(), RewardInput{
		Code: body.Code, Name: body.Name, Description: body.Description,
		RewardType: domain.RewardType(strings.ToUpper(body.RewardType)),
		XPCost:     body.XPCost, RequiredTierCode: body.RequiredTierCode,
		RewardValue: body.RewardValue, StockTotal: body.StockTotal,
		MaxPerMember: body.MaxPerMember, ImageURL: body.ImageURL,
		StartsAt: body.StartsAt, EndsAt: body.EndsAt,
		Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, reward)
}

// ── Redemptions ──────────────────────────────────────────────────────────────

func (h *Handler) listRedemptions(w http.ResponseWriter, r *http.Request) {
	claims, err := h.service.Redemptions(r.Context(), RedemptionFilter{
		MemberID: httpx.Query(r, "memberId"),
		RewardID: httpx.Query(r, "rewardId"),
		Status:   strings.ToUpper(httpx.Query(r, "status")),
		Limit:    httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, claims)
}

type noteBody struct {
	Note *string `json:"note"`
}

func (h *Handler) decideRedemption(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[noteBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var target domain.RedemptionStatus
	switch strings.ToLower(httpx.Param(r, "action")) {
	case "approve":
		target = domain.RedemptionApproved
	case "fulfil", "fulfill":
		target = domain.RedemptionFulfilled
	case "cancel":
		target = domain.RedemptionCancelled
	default:
		httpx.Fail(w, r, httpx.Invalid("A claim is approved, fulfilled or cancelled."))
		return
	}

	claim, err := h.service.DecideRedemption(r.Context(), httpx.Param(r, "id"), target, body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, claim)
}
