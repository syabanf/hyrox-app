// Package crm is the loyalty side of a membership.
//
// The wallet already answers "what has this member paid for". This answers a
// different question — how much are they worth to us, and what have we
// promised them for it — and the two must never be confused. XP is not money:
// it cannot be topped up, it does not pay for a class, and it never appears in
// the credit ledger. A system where points and credits are interchangeable has
// invented a currency it did not mean to.
package crm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/audit"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Members is the port CRM needs from identity: enough to put a name against a
// profile, and nothing else.
type Members interface {
	Member(ctx context.Context, id string) (domain.Member, error)
	// MembersByIDs names a page of profiles in one round trip.
	MembersByIDs(ctx context.Context, ids []string) (map[string]domain.Member, error)
}

// Service implements the loyalty use cases.
type Service struct {
	db      *database.DB
	repo    *Repository
	members Members
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
	studio  *time.Location
}

func NewService(db *database.DB, repo *Repository, members Members,
	ids id.Generator, c clock.Clock, auditor audit.Recorder, studio *time.Location) *Service {
	if studio == nil {
		studio = time.UTC
	}
	return &Service{db: db, repo: repo, members: members, ids: ids, clock: c,
		auditor: auditor, studio: studio}
}

// Actor identifies who performed an administrative action.
type Actor struct {
	ID   string
	Name string
}

func (s *Service) record(ctx context.Context, entity, entityID, action string, actor Actor, reason *string) {
	_ = s.auditor.Record(ctx, audit.Event{
		EntityType: entity, EntityID: entityID, Action: action,
		ActorID: actor.ID, ActorName: actor.Name, Reason: reason,
	})
}

func shortCode(generated string) string {
	if _, tail, found := strings.Cut(generated, "_"); found && len(tail) >= 6 {
		return tail[len(tail)-6:]
	}
	return generated
}

// baseTier is the band a member starts in: the lowest active rank.
func (s *Service) baseTier(ctx context.Context) (domain.Tier, error) {
	tiers, err := s.repo.Tiers(ctx)
	if err != nil {
		return domain.Tier{}, err
	}
	var lowest domain.Tier
	found := false
	for _, tier := range tiers {
		if !tier.Active {
			continue
		}
		if !found || tier.Rank < lowest.Rank {
			lowest, found = tier, true
		}
	}
	if !found {
		return domain.Tier{}, httpx.Conflict("NO_TIERS",
			"No loyalty tiers are configured, so nobody can earn anything yet.")
	}
	return lowest, nil
}

// profileFor reads a member's loyalty profile, creating it on first touch. A
// profile exists because somebody engaged, not because they registered.
func (s *Service) profileFor(ctx context.Context, memberID string, forUpdate bool) (domain.MemberProfile, error) {
	base, err := s.baseTier(ctx)
	if err != nil {
		return domain.MemberProfile{}, err
	}
	code := "NH-" + strings.ToUpper(shortCode(s.ids.New(id.MemberProfile)))
	return s.repo.Profile(ctx, memberID, s.ids.New(id.MemberProfile), code, base.Code, forUpdate)
}

// ── Earning and spending ─────────────────────────────────────────────────────

// AwardInput is something a member did that might be worth points.
type AwardInput struct {
	MemberID  string
	Channel   domain.XPChannel
	Type      string
	SourceID  string
	BranchID  string
	AmountIDR float64
	Items     int
	// IdempotencyKey makes the same event arriving twice a no-op. Outbox
	// redeliveries and till retries both depend on it.
	IdempotencyKey string
	ReferenceType  string
	ReferenceID    string
	Description    string
}

// AwardResult is what an event earned, and whether it earned it just now.
type AwardResult struct {
	Entry *domain.XPEntry `json:"entry"`
	// Profile is the member's standing afterwards.
	Profile domain.MemberProfile `json:"profile"`
	// Awarded is false when a matching rule found nothing to give, or when the
	// event had already been counted.
	Awarded bool `json:"awarded"`
	// Duplicate says the event had already been posted under the same key.
	Duplicate bool           `json:"duplicate"`
	Rule      *domain.XPRule `json:"rule"`
	// TierChanged names the band the member has just moved into.
	TierChanged *domain.Tier `json:"tierChanged"`
}

// Award turns an event into points, if any rule says it is worth some.
//
// It is safe to call twice with the same idempotency key: the second call
// returns what the first one did without awarding anything.
func (s *Service) Award(ctx context.Context, in AwardInput) (AwardResult, error) {
	var result AwardResult
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		if in.IdempotencyKey != "" {
			existing, found, err := s.repo.EntryByKey(ctx, in.IdempotencyKey)
			if err != nil {
				return err
			}
			if found {
				profile, _, err := s.repo.ProfileByMember(ctx, existing.MemberID)
				if err != nil {
					return err
				}
				result = AwardResult{Entry: &existing, Profile: profile, Duplicate: true}
				return nil
			}
		}

		profile, err := s.profileFor(ctx, in.MemberID, true)
		if err != nil {
			return err
		}
		tiers, err := s.repo.Tiers(ctx)
		if err != nil {
			return err
		}
		rules, err := s.repo.Rules(ctx, string(in.Channel), true)
		if err != nil {
			return err
		}

		event := domain.XPEvent{
			Channel: in.Channel, Type: in.Type, SourceID: in.SourceID, BranchID: in.BranchID,
			AmountIDR: in.AmountIDR, Items: in.Items, At: s.clock.Now(),
		}
		rule, matched := domain.SelectXPRule(rules, event)
		if !matched {
			result = AwardResult{Profile: profile}
			return nil
		}

		multiplier := 1.0
		for _, tier := range tiers {
			if tier.Code == profile.TierCode {
				multiplier = tier.XPMultiplier
			}
		}
		points := domain.ComputeXP(rule, event, multiplier)
		if points == 0 {
			result = AwardResult{Profile: profile, Rule: &rule}
			return nil
		}

		entry, updated, tierChange, err := s.post(ctx, profile, points, domain.XPEarn, postDetails{
			Channel: in.Channel, Type: in.Type, SourceID: in.SourceID, BranchID: in.BranchID,
			RuleID: &rule.ID, ReferenceType: in.ReferenceType, ReferenceID: in.ReferenceID,
			IdempotencyKey: in.IdempotencyKey, Description: in.Description,
			SpendIDR: in.AmountIDR,
		})
		if err != nil {
			return err
		}
		result = AwardResult{
			Entry: &entry, Profile: updated, Awarded: true, Rule: &rule, TierChanged: tierChange,
		}
		return nil
	})
	return result, err
}

// postDetails is everything a ledger entry records beyond the amount.
type postDetails struct {
	Channel        domain.XPChannel
	Type           string
	SourceID       string
	BranchID       string
	RuleID         *string
	ReferenceType  string
	ReferenceID    string
	IdempotencyKey string
	Description    string
	// SpendIDR is added to the member's lifetime spend, which is the other
	// half of a tier rule.
	SpendIDR float64
}

// post is the one place points move. Everything else — earning, redeeming,
// adjusting, reversing — comes through here, so the ledger is always written
// and the tier is always re-evaluated.
func (s *Service) post(ctx context.Context, profile domain.MemberProfile, delta int,
	direction domain.XPDirection, details postDetails) (domain.XPEntry, domain.MemberProfile, *domain.Tier, error) {

	posted := domain.PostXP(profile, delta)
	if !posted.Allowed() {
		return domain.XPEntry{}, profile, nil, xpRejectionError(posted.Rejection, profile, delta)
	}

	entry := domain.XPEntry{
		ID: s.ids.New(id.XPEntry), MemberID: profile.MemberID, Direction: direction,
		SourceChannel: details.Channel, SourceType: details.Type,
		XPDelta:       posted.Delta,
		BalanceBefore: posted.BalanceBefore, BalanceAfter: posted.BalanceAfter,
		LifetimeBefore: posted.LifetimeBefore, LifetimeAfter: posted.LifetimeAfter,
		RuleID: details.RuleID,
	}
	if details.SourceID != "" {
		entry.SourceID = &details.SourceID
	}
	if details.BranchID != "" {
		entry.BranchID = &details.BranchID
	}
	if details.ReferenceType != "" {
		entry.ReferenceType = &details.ReferenceType
	}
	if details.ReferenceID != "" {
		entry.ReferenceID = &details.ReferenceID
	}
	if details.IdempotencyKey != "" {
		entry.IdempotencyKey = &details.IdempotencyKey
	}
	if details.Description != "" {
		entry.Description = &details.Description
	}

	saved, err := s.repo.InsertEntry(ctx, entry)
	if err != nil {
		return domain.XPEntry{}, profile, nil, err
	}

	now := s.clock.Now()
	profile.CurrentXP = posted.BalanceAfter
	profile.LifetimeXP = posted.LifetimeAfter
	profile.SpentXP = posted.SpentAfter
	profile.LifetimeSpendIDR += details.SpendIDR
	profile.LastActivityAt = &now

	// A tier is re-evaluated on every movement, and because lifetime XP only
	// ever grows it can only ever go up.
	tiers, err := s.repo.Tiers(ctx)
	if err != nil {
		return domain.XPEntry{}, profile, nil, err
	}
	var promoted *domain.Tier
	if tier, ok := domain.ResolveTier(tiers, profile.LifetimeXP, profile.LifetimeSpendIDR); ok {
		if tier.Code != profile.TierCode {
			profile.TierCode = tier.Code
			moved := tier
			promoted = &moved
		}
	}

	updated, err := s.repo.SaveProfile(ctx, profile)
	if err != nil {
		return domain.XPEntry{}, profile, nil, err
	}
	return saved, updated, promoted, nil
}

func xpRejectionError(rejection domain.XPRejection, profile domain.MemberProfile, delta int) error {
	switch rejection {
	case domain.XPRejectInsufficient:
		return httpx.Conflict("INSUFFICIENT_XP",
			"That needs %d points and only %d are available.", -delta, profile.CurrentXP)
	case domain.XPRejectZero:
		return httpx.Invalid("An award of nothing is not an award.")
	case domain.XPRejectSuspended:
		return httpx.Conflict("PROFILE_SUSPENDED", "That member's loyalty account is suspended.")
	default:
		return httpx.Invalid("Those points cannot be moved.")
	}
}

// AdjustInput is a manual correction, which always carries a reason: points
// appearing without one are indistinguishable from a member being favoured.
type AdjustInput struct {
	MemberID string
	XPDelta  int
	Reason   string
}

func (s *Service) Adjust(ctx context.Context, in AdjustInput, actor Actor) (AwardResult, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return AwardResult{}, httpx.Invalid("An adjustment needs a reason.")
	}
	if in.XPDelta == 0 {
		return AwardResult{}, httpx.Invalid("An adjustment of nothing is not an adjustment.")
	}

	var result AwardResult
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		profile, err := s.profileFor(ctx, in.MemberID, true)
		if err != nil {
			return err
		}
		entry, updated, tierChange, err := s.post(ctx, profile, in.XPDelta, domain.XPAdjust, postDetails{
			Channel: domain.XPFromManual, Description: in.Reason,
		})
		if err != nil {
			return err
		}
		result = AwardResult{Entry: &entry, Profile: updated, Awarded: true, TierChanged: tierChange}
		return nil
	})
	if err != nil {
		return AwardResult{}, err
	}
	s.record(ctx, "crm.member", in.MemberID, "XP_ADJUST", actor, &in.Reason)
	return result, nil
}

// ── Reading ──────────────────────────────────────────────────────────────────

// ProfileView is a member's loyalty standing with their name and tier.
type ProfileView struct {
	domain.MemberProfile
	MemberName string       `json:"memberName"`
	Tier       *domain.Tier `json:"tier"`
	// NextTier is what they are climbing towards, and how far off it is.
	NextTier *domain.Tier `json:"nextTier"`
	XPToNext int          `json:"xpToNext"`
}

func (s *Service) view(ctx context.Context, profile domain.MemberProfile) (ProfileView, error) {
	tiers, err := s.repo.Tiers(ctx)
	if err != nil {
		return ProfileView{}, err
	}
	view := ProfileView{MemberProfile: profile}

	member, err := s.members.Member(ctx, profile.MemberID)
	if err == nil {
		view.MemberName = member.FullName
	}

	var current domain.Tier
	for _, tier := range tiers {
		if tier.Code == profile.TierCode {
			held := tier
			view.Tier = &held
			current = tier
		}
	}
	// The next rung up, and the points still needed for it.
	for _, tier := range tiers {
		if !tier.Active || tier.Rank <= current.Rank {
			continue
		}
		if view.NextTier == nil || tier.Rank < view.NextTier.Rank {
			next := tier
			view.NextTier = &next
			view.XPToNext = tier.MinLifetimeXP - profile.LifetimeXP
			if view.XPToNext < 0 {
				view.XPToNext = 0
			}
		}
	}
	return view, nil
}

func (s *Service) Profiles(ctx context.Context, filter ProfileFilter) ([]ProfileView, error) {
	profiles, err := s.repo.Profiles(ctx, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.MemberID)
	}
	members, err := s.members.MembersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	tiers, err := s.repo.Tiers(ctx)
	if err != nil {
		return nil, err
	}
	byCode := map[string]domain.Tier{}
	for _, tier := range tiers {
		byCode[tier.Code] = tier
	}

	views := make([]ProfileView, 0, len(profiles))
	for _, profile := range profiles {
		view := ProfileView{MemberProfile: profile, MemberName: members[profile.MemberID].FullName}
		if tier, ok := byCode[profile.TierCode]; ok {
			held := tier
			view.Tier = &held
		}
		views = append(views, view)
	}
	return views, nil
}

// MemberDetail is one member's loyalty page.
type MemberDetail struct {
	Profile     ProfileView      `json:"profile"`
	Ledger      []domain.XPEntry `json:"ledger"`
	Redemptions []RedemptionView `json:"redemptions"`
}

func (s *Service) MemberDetail(ctx context.Context, memberID string) (MemberDetail, error) {
	profile, found, err := s.repo.ProfileByMember(ctx, memberID)
	if err != nil {
		return MemberDetail{}, err
	}
	if !found {
		// Somebody who has never earned anything has no profile yet, and
		// looking at them should not create one.
		if _, err := s.members.Member(ctx, memberID); err != nil {
			return MemberDetail{}, err
		}
		base, err := s.baseTier(ctx)
		if err != nil {
			return MemberDetail{}, err
		}
		profile = domain.MemberProfile{MemberID: memberID, TierCode: base.Code, Status: "ACTIVE"}
	}

	view, err := s.view(ctx, profile)
	if err != nil {
		return MemberDetail{}, err
	}
	ledger, err := s.repo.Entries(ctx, memberID, "", 50)
	if err != nil {
		return MemberDetail{}, err
	}
	redemptions, err := s.Redemptions(ctx, RedemptionFilter{MemberID: memberID, Limit: 50})
	if err != nil {
		return MemberDetail{}, err
	}
	return MemberDetail{Profile: view, Ledger: ledger, Redemptions: redemptions}, nil
}

func (s *Service) Ledger(ctx context.Context, memberID, direction string, limit int) ([]domain.XPEntry, error) {
	return s.repo.Entries(ctx, memberID, direction, limit)
}

// Overview is the loyalty dashboard.
func (s *Service) Overview(ctx context.Context) (Summary, error) {
	now := s.clock.Now().In(s.studio)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.studio)
	return s.repo.Summary(ctx, monthStart)
}

// ── Configuration ────────────────────────────────────────────────────────────

func (s *Service) Tiers(ctx context.Context) ([]domain.Tier, error) { return s.repo.Tiers(ctx) }

// TierInput creates or replaces a tier.
type TierInput struct {
	Code            string
	Name            string
	Rank            int
	MinLifetimeXP   int
	MinSpendIDR     float64
	XPMultiplier    float64
	DiscountPercent float64
	Benefits        []string
	Colour          string
	Active          bool
}

func (s *Service) SaveTier(ctx context.Context, in TierInput, actor Actor) (domain.Tier, error) {
	if in.Rank <= 0 {
		return domain.Tier{}, httpx.Invalid("A tier's rank starts at 1.")
	}
	colour := in.Colour
	if colour == "" {
		colour = "#5F6B62"
	}
	multiplier := in.XPMultiplier
	if multiplier == 0 {
		multiplier = 1
	}
	saved, err := s.repo.UpsertTier(ctx, domain.Tier{
		ID: s.ids.New(id.Tier), Code: strings.ToUpper(in.Code), Name: in.Name, Rank: in.Rank,
		MinLifetimeXP: in.MinLifetimeXP, MinSpendIDR: in.MinSpendIDR,
		XPMultiplier: multiplier, DiscountPercent: in.DiscountPercent,
		Benefits: in.Benefits, Colour: colour, Active: in.Active,
	})
	if err != nil {
		return domain.Tier{}, err
	}
	s.record(ctx, "crm.tier", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

func (s *Service) Rules(ctx context.Context, channel string, activeOnly bool) ([]domain.XPRule, error) {
	return s.repo.Rules(ctx, channel, activeOnly)
}

// RuleInput creates or replaces an earning rule.
type RuleInput struct {
	Code                  string
	Name                  string
	SourceChannel         domain.XPChannel
	SourceType            string
	SourceID              *string
	BranchID              *string
	XPMode                domain.XPMode
	XPValue               float64
	AmountStep            float64
	MinAmount             float64
	MaxXPPerEvent         *int
	TierMultiplierEnabled bool
	Priority              int
	StartsAt              *time.Time
	EndsAt                *time.Time
	Active                bool
}

func (s *Service) SaveRule(ctx context.Context, in RuleInput, actor Actor) (domain.XPRule, error) {
	step := in.AmountStep
	if step <= 0 {
		step = 1
	}
	priority := in.Priority
	if priority == 0 {
		priority = 100
	}
	saved, err := s.repo.UpsertRule(ctx, domain.XPRule{
		ID: s.ids.New(id.XPRule), Code: strings.ToUpper(in.Code), Name: in.Name,
		SourceChannel: in.SourceChannel, SourceType: in.SourceType, SourceID: in.SourceID,
		BranchID: in.BranchID, XPMode: in.XPMode, XPValue: in.XPValue, AmountStep: step,
		MinAmount: in.MinAmount, MaxXPPerEvent: in.MaxXPPerEvent,
		TierMultiplierEnabled: in.TierMultiplierEnabled, Priority: priority,
		StartsAt: in.StartsAt, EndsAt: in.EndsAt, Active: in.Active,
	})
	if err != nil {
		return domain.XPRule{}, err
	}
	s.record(ctx, "crm.rule", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

func (s *Service) Rewards(ctx context.Context, activeOnly bool) ([]domain.Reward, error) {
	return s.repo.Rewards(ctx, activeOnly)
}

// RewardInput creates or replaces a reward.
type RewardInput struct {
	Code             string
	Name             string
	Description      string
	RewardType       domain.RewardType
	XPCost           int
	RequiredTierCode *string
	RewardValue      map[string]any
	StockTotal       *int
	MaxPerMember     *int
	ImageURL         *string
	StartsAt         *time.Time
	EndsAt           *time.Time
	Active           bool
}

func (s *Service) SaveReward(ctx context.Context, in RewardInput, actor Actor) (domain.Reward, error) {
	value := in.RewardValue
	if value == nil {
		value = map[string]any{}
	}
	saved, err := s.repo.UpsertReward(ctx, domain.Reward{
		ID: s.ids.New(id.Reward), Code: strings.ToUpper(in.Code), Name: in.Name,
		Description: in.Description, RewardType: in.RewardType, XPCost: in.XPCost,
		RequiredTierCode: in.RequiredTierCode, RewardValue: value,
		StockTotal: in.StockTotal, MaxPerMember: in.MaxPerMember, ImageURL: in.ImageURL,
		StartsAt: in.StartsAt, EndsAt: in.EndsAt, Active: in.Active,
	})
	if err != nil {
		return domain.Reward{}, err
	}
	s.record(ctx, "crm.reward", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// ── Redeeming ────────────────────────────────────────────────────────────────

// RedemptionView is a claim with the reward and member named.
type RedemptionView struct {
	domain.Redemption
	MemberName string `json:"memberName"`
	RewardName string `json:"rewardName"`
	RewardType string `json:"rewardType"`
}

func (s *Service) Redemptions(ctx context.Context, filter RedemptionFilter) ([]RedemptionView, error) {
	redemptions, err := s.repo.Redemptions(ctx, filter)
	if err != nil {
		return nil, err
	}
	rewards, err := s.repo.Rewards(ctx, false)
	if err != nil {
		return nil, err
	}
	byID := map[string]domain.Reward{}
	for _, reward := range rewards {
		byID[reward.ID] = reward
	}
	ids := make([]string, 0, len(redemptions))
	for _, red := range redemptions {
		ids = append(ids, red.MemberID)
	}
	members, err := s.members.MembersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	views := make([]RedemptionView, 0, len(redemptions))
	for _, red := range redemptions {
		reward := byID[red.RewardID]
		views = append(views, RedemptionView{
			Redemption: red, MemberName: members[red.MemberID].FullName,
			RewardName: reward.Name, RewardType: string(reward.RewardType),
		})
	}
	return views, nil
}

// Redeem claims a reward, taking the points in the same transaction that takes
// the stock. A claim that exists without a ledger entry would be a promise
// nobody paid for.
func (s *Service) Redeem(ctx context.Context, memberID, rewardID string, actor Actor) (RedemptionView, error) {
	var claimID string
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		reward, err := s.repo.Reward(ctx, rewardID, true)
		if err != nil {
			return err
		}
		profile, err := s.profileFor(ctx, memberID, true)
		if err != nil {
			return err
		}
		tiers, err := s.repo.Tiers(ctx)
		if err != nil {
			return err
		}
		already, err := s.repo.CountRedemptions(ctx, memberID, rewardID)
		if err != nil {
			return err
		}

		rejection := domain.EvaluateRedemption(domain.RedemptionRequest{
			Reward: reward, Profile: profile, Tiers: tiers,
			AlreadyRedeemed: already, At: s.clock.Now(),
		})
		if rejection != "" {
			return redemptionRejectionError(rejection, reward, profile)
		}

		entry, _, _, err := s.post(ctx, profile, -reward.XPCost, domain.XPSpend, postDetails{
			Channel: domain.XPFromManual, Type: "REDEMPTION", SourceID: reward.ID,
			ReferenceType: "REWARD", ReferenceID: reward.ID,
			Description: "Redeemed " + reward.Name,
		})
		if err != nil {
			return err
		}
		if err := s.repo.ClaimRewardStock(ctx, rewardID, 1); err != nil {
			return err
		}

		claim, err := s.repo.InsertRedemption(ctx, domain.Redemption{
			ID: s.ids.New(id.LoyaltyRedemption),
			RedemptionNumber: fmt.Sprintf("RDM-%s-%s",
				s.clock.Now().In(s.studio).Format("20060102"),
				shortCode(s.ids.New(id.LoyaltyRedemption))),
			MemberID: memberID, RewardID: rewardID, XPCost: reward.XPCost,
			XPLedgerID: &entry.ID, Status: domain.RedemptionPending,
		})
		if err != nil {
			return err
		}
		claimID = claim.ID
		return nil
	})
	if err != nil {
		return RedemptionView{}, err
	}
	s.record(ctx, "crm.member", memberID, "REDEEM", actor, nil)

	views, err := s.Redemptions(ctx, RedemptionFilter{MemberID: memberID, Limit: 50})
	if err != nil {
		return RedemptionView{}, err
	}
	for _, view := range views {
		if view.ID == claimID {
			return view, nil
		}
	}
	return RedemptionView{}, httpx.NotFound("redemption")
}

func redemptionRejectionError(rejection domain.RedemptionRejection, reward domain.Reward, profile domain.MemberProfile) error {
	switch rejection {
	case domain.RedeemRejectInsufficient:
		return httpx.Conflict("INSUFFICIENT_XP",
			"%s costs %d points and only %d are available.", reward.Name, reward.XPCost, profile.CurrentXP)
	case domain.RedeemRejectTier:
		return httpx.Conflict("TIER_TOO_LOW", "%s is not available at that tier.", reward.Name)
	case domain.RedeemRejectOutOfStock:
		return httpx.Conflict("OUT_OF_STOCK", "%s has all been claimed.", reward.Name)
	case domain.RedeemRejectPerMember:
		return httpx.Conflict("PER_MEMBER_LIMIT", "That member has already claimed %s as often as allowed.", reward.Name)
	case domain.RedeemRejectInactive:
		return httpx.Conflict("REWARD_INACTIVE", "%s is not currently offered.", reward.Name)
	case domain.RedeemRejectNotStarted:
		return httpx.Conflict("NOT_STARTED", "%s is not available yet.", reward.Name)
	case domain.RedeemRejectEnded:
		return httpx.Conflict("ENDED", "%s is no longer available.", reward.Name)
	case domain.RedeemRejectSuspended:
		return httpx.Conflict("PROFILE_SUSPENDED", "That member's loyalty account is suspended.")
	default:
		return httpx.Invalid("That reward cannot be claimed.")
	}
}

// DecideRedemption moves a claim along, or cancels it and gives the points
// back. A fulfilled claim is final: the member already has the thing.
func (s *Service) DecideRedemption(ctx context.Context, redemptionID string,
	target domain.RedemptionStatus, note *string, actor Actor) (RedemptionView, error) {

	var memberID, claimID string
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		claim, err := s.repo.Redemption(ctx, redemptionID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.RedemptionTransitions, claim.Status, target)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That claim is %s and cannot become %s.",
				strings.ToLower(string(claim.Status)), strings.ToLower(string(target)))
		}

		now := s.clock.Now()
		claim.Status = next
		claim.Note = note
		switch target {
		case domain.RedemptionApproved:
			claim.ApprovedAt = &now
		case domain.RedemptionFulfilled:
			claim.FulfilledAt = &now
		case domain.RedemptionCancelled, domain.RedemptionExpired:
			claim.CancelledAt = &now
			// The points go back, and so does the stock.
			profile, err := s.profileFor(ctx, claim.MemberID, true)
			if err != nil {
				return err
			}
			if _, _, _, err := s.post(ctx, profile, claim.XPCost, domain.XPReverse, postDetails{
				Channel: domain.XPFromManual, Type: "REDEMPTION_REVERSED",
				ReferenceType: "REDEMPTION", ReferenceID: claim.ID,
				Description: "Redemption " + claim.RedemptionNumber + " reversed",
			}); err != nil {
				return err
			}
			if err := s.repo.ClaimRewardStock(ctx, claim.RewardID, -1); err != nil {
				return err
			}
		}

		saved, err := s.repo.SaveRedemption(ctx, claim)
		if err != nil {
			return err
		}
		memberID, claimID = saved.MemberID, saved.ID
		return nil
	})
	if err != nil {
		return RedemptionView{}, err
	}
	s.record(ctx, "crm.redemption", redemptionID, string(target), actor, note)

	views, err := s.Redemptions(ctx, RedemptionFilter{MemberID: memberID, Limit: 50})
	if err != nil {
		return RedemptionView{}, err
	}
	for _, view := range views {
		if view.ID == claimID {
			return view, nil
		}
	}
	return RedemptionView{}, httpx.NotFound("redemption")
}
