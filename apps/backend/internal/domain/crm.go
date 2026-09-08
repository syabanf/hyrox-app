package domain

import (
	"math"
	"sort"
	"time"
)

// Loyalty: how a member earns standing, and what it buys.
//
// XP is not money. It cannot be topped up, it does not pay for a class, and it
// never appears in the credit ledger. Keeping the two apart is the whole point
// — a system where points and credits are interchangeable has invented a
// currency it did not mean to.

// Tier is a band a member has earned their way into.
type Tier struct {
	ID              string    `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Rank            int       `json:"rank"`
	MinLifetimeXP   int       `json:"minLifetimeXp"`
	MinSpendIDR     float64   `json:"minSpendIdr"`
	XPMultiplier    float64   `json:"xpMultiplier"`
	DiscountPercent float64   `json:"discountPercent"`
	Benefits        []string  `json:"benefits"`
	Colour          string    `json:"colour"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// ResolveTier is the highest band a member currently qualifies for.
//
// Both thresholds must be met, and because lifetime XP and lifetime spend only
// ever grow, a tier is something a member cannot fall out of by spending their
// points. That is deliberate: a loyalty scheme that demotes people for using it
// teaches them not to use it.
func ResolveTier(tiers []Tier, lifetimeXP int, lifetimeSpendIDR float64) (Tier, bool) {
	ranked := make([]Tier, 0, len(tiers))
	for _, tier := range tiers {
		if tier.Active {
			ranked = append(ranked, tier)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Rank > ranked[j].Rank })

	for _, tier := range ranked {
		if lifetimeXP >= tier.MinLifetimeXP && lifetimeSpendIDR >= tier.MinSpendIDR {
			return tier, true
		}
	}
	return Tier{}, false
}

// MemberProfile is the loyalty layer over a member.
type MemberProfile struct {
	ID         string `json:"id"`
	MemberID   string `json:"memberId"`
	MemberCode string `json:"memberCode"`
	TierCode   string `json:"tierCode"`
	// CurrentXP is spendable. LifetimeXP only ever grows, and is what decides
	// the tier.
	CurrentXP        int        `json:"currentXp"`
	LifetimeXP       int        `json:"lifetimeXp"`
	SpentXP          int        `json:"spentXp"`
	LifetimeSpendIDR float64    `json:"lifetimeSpendIdr"`
	JoinedAt         time.Time  `json:"joinedAt"`
	LastActivityAt   *time.Time `json:"lastActivityAt"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// ── Earning ──────────────────────────────────────────────────────────────────

// XPChannel is where an earning event came from.
type XPChannel string

const (
	XPFromPOS      XPChannel = "POS"
	XPFromBooking  XPChannel = "BOOKING"
	XPFromClass    XPChannel = "CLASS"
	XPFromPayment  XPChannel = "PAYMENT"
	XPFromManual   XPChannel = "MANUAL"
	XPFromCampaign XPChannel = "CAMPAIGN"
)

// IsValidXPChannel validates a channel arriving from a request.
func IsValidXPChannel(value string) bool {
	switch XPChannel(value) {
	case XPFromPOS, XPFromBooking, XPFromClass, XPFromPayment, XPFromManual, XPFromCampaign:
		return true
	}
	return false
}

// XPMode is how a rule turns an event into points.
type XPMode string

const (
	// XPFixed is a flat award: five points for turning up.
	XPFixed XPMode = "FIXED"
	// XPPerItem multiplies by the number of things.
	XPPerItem XPMode = "PER_ITEM"
	// XPPerAmount is a point per so many rupiah.
	XPPerAmount XPMode = "PER_AMOUNT"
	// XPMultiplier scales an amount directly.
	XPMultiplier XPMode = "MULTIPLIER"
	// XPPercentage is a percentage of the amount, in points.
	XPPercentage XPMode = "PERCENTAGE"
)

// IsValidXPMode validates a mode arriving from a request.
func IsValidXPMode(value string) bool {
	switch XPMode(value) {
	case XPFixed, XPPerItem, XPPerAmount, XPMultiplier, XPPercentage:
		return true
	}
	return false
}

// XPRule says how much an event is worth.
type XPRule struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	SourceChannel XPChannel `json:"sourceChannel"`
	SourceType    string    `json:"sourceType"`
	// SourceID narrows a rule to one class type, package or product.
	SourceID *string `json:"sourceId"`
	BranchID *string `json:"branchId"`
	XPMode   XPMode  `json:"xpMode"`
	XPValue  float64 `json:"xpValue"`
	// AmountStep is the rupiah per XPValue in PER_AMOUNT mode.
	AmountStep            float64    `json:"amountStep"`
	MinAmount             float64    `json:"minAmount"`
	MaxXPPerEvent         *int       `json:"maxXpPerEvent"`
	TierMultiplierEnabled bool       `json:"tierMultiplierEnabled"`
	Priority              int        `json:"priority"`
	StartsAt              *time.Time `json:"startsAt"`
	EndsAt                *time.Time `json:"endsAt"`
	Active                bool       `json:"active"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

// XPEvent is something a member did that might be worth points.
type XPEvent struct {
	Channel  XPChannel
	Type     string
	SourceID string
	BranchID string
	// AmountIDR is what they spent, where the event involved money.
	AmountIDR float64
	// Items is how many things, for per-item rules.
	Items int
	At    time.Time
}

// MatchesRule reports whether a rule applies to an event.
func MatchesRule(rule XPRule, event XPEvent) bool {
	if !rule.Active {
		return false
	}
	if rule.SourceChannel != event.Channel {
		return false
	}
	if rule.SourceType != "" && rule.SourceType != event.Type {
		return false
	}
	if rule.SourceID != nil && *rule.SourceID != event.SourceID {
		return false
	}
	if rule.BranchID != nil && *rule.BranchID != event.BranchID {
		return false
	}
	if rule.StartsAt != nil && event.At.Before(*rule.StartsAt) {
		return false
	}
	if rule.EndsAt != nil && !event.At.Before(*rule.EndsAt) {
		return false
	}
	if event.AmountIDR < rule.MinAmount {
		return false
	}
	return true
}

// SelectXPRule picks the rule that governs an event.
//
// Highest priority wins; ties break on the more specific rule, so a campaign
// aimed at one class type beats the studio-wide default without anybody having
// to remember to disable the default.
func SelectXPRule(rules []XPRule, event XPEvent) (XPRule, bool) {
	matching := make([]XPRule, 0, len(rules))
	for _, rule := range rules {
		if MatchesRule(rule, event) {
			matching = append(matching, rule)
		}
	}
	if len(matching) == 0 {
		return XPRule{}, false
	}

	specificity := func(r XPRule) int {
		score := 0
		if r.SourceID != nil {
			score += 2
		}
		if r.BranchID != nil {
			score++
		}
		if r.SourceType != "" {
			score++
		}
		return score
	}
	sort.SliceStable(matching, func(i, j int) bool {
		if matching[i].Priority != matching[j].Priority {
			return matching[i].Priority > matching[j].Priority
		}
		return specificity(matching[i]) > specificity(matching[j])
	})
	return matching[0], true
}

// ComputeXP is how many points an event earns under a rule.
//
// The tier multiplier applies only where the rule allows it: a welcome bonus
// should not be worth more to somebody who is already a regular. The cap is
// applied last, after the multiplier, because a cap that a multiplier can
// exceed is not a cap.
func ComputeXP(rule XPRule, event XPEvent, tierMultiplier float64) int {
	var raw float64
	switch rule.XPMode {
	case XPFixed:
		raw = rule.XPValue
	case XPPerItem:
		raw = rule.XPValue * float64(event.Items)
	case XPPerAmount:
		if rule.AmountStep <= 0 {
			return 0
		}
		raw = rule.XPValue * math.Floor(event.AmountIDR/rule.AmountStep)
	case XPMultiplier:
		raw = rule.XPValue * event.AmountIDR
	case XPPercentage:
		raw = event.AmountIDR * rule.XPValue / 100
	default:
		return 0
	}

	if rule.TierMultiplierEnabled && tierMultiplier > 0 {
		raw *= tierMultiplier
	}
	points := int(math.Floor(raw))
	if points < 0 {
		points = 0
	}
	if rule.MaxXPPerEvent != nil && points > *rule.MaxXPPerEvent {
		points = *rule.MaxXPPerEvent
	}
	return points
}

// ── The ledger ───────────────────────────────────────────────────────────────

// XPDirection is what an entry did.
type XPDirection string

const (
	XPEarn    XPDirection = "EARN"
	XPSpend   XPDirection = "SPEND"
	XPAdjust  XPDirection = "ADJUST"
	XPReverse XPDirection = "REVERSE"
	XPExpire  XPDirection = "EXPIRE"
)

// XPEntry is one change to a member's points, kept forever.
type XPEntry struct {
	ID             string      `json:"id"`
	MemberID       string      `json:"memberId"`
	Direction      XPDirection `json:"direction"`
	SourceChannel  XPChannel   `json:"sourceChannel"`
	SourceType     string      `json:"sourceType"`
	SourceID       *string     `json:"sourceId"`
	BranchID       *string     `json:"branchId"`
	XPDelta        int         `json:"xpDelta"`
	BalanceBefore  int         `json:"balanceBefore"`
	BalanceAfter   int         `json:"balanceAfter"`
	LifetimeBefore int         `json:"lifetimeBefore"`
	LifetimeAfter  int         `json:"lifetimeAfter"`
	RuleID         *string     `json:"ruleId"`
	ReferenceType  *string     `json:"referenceType"`
	ReferenceID    *string     `json:"referenceId"`
	// IdempotencyKey is what makes posting the same event twice impossible.
	IdempotencyKey *string   `json:"idempotencyKey"`
	Description    *string   `json:"description"`
	CreatedAt      time.Time `json:"createdAt"`
}

// PostedXP is the arithmetic of applying a change to a profile.
type PostedXP struct {
	Delta          int
	BalanceBefore  int
	BalanceAfter   int
	LifetimeBefore int
	LifetimeAfter  int
	SpentAfter     int
	Rejection      XPRejection
}

// Allowed reports whether the change may be posted.
func (p PostedXP) Allowed() bool { return p.Rejection == "" }

// XPRejection explains why points cannot move.
type XPRejection string

const (
	XPRejectZero         XPRejection = "ZERO_DELTA"
	XPRejectInsufficient XPRejection = "INSUFFICIENT_XP"
	XPRejectSuspended    XPRejection = "PROFILE_SUSPENDED"
)

// PostXP applies a change to a profile.
//
// Earning raises both the spendable balance and the lifetime total; spending
// only lowers the spendable one. That asymmetry is the whole reason there are
// two numbers: a member who redeems a reward has not become less loyal.
func PostXP(profile MemberProfile, delta int) PostedXP {
	if profile.Status == "SUSPENDED" {
		return PostedXP{Rejection: XPRejectSuspended}
	}
	if delta == 0 {
		return PostedXP{Rejection: XPRejectZero}
	}
	if profile.CurrentXP+delta < 0 {
		return PostedXP{Rejection: XPRejectInsufficient}
	}

	posted := PostedXP{
		Delta:          delta,
		BalanceBefore:  profile.CurrentXP,
		BalanceAfter:   profile.CurrentXP + delta,
		LifetimeBefore: profile.LifetimeXP,
		LifetimeAfter:  profile.LifetimeXP,
		SpentAfter:     profile.SpentXP,
	}
	if delta > 0 {
		posted.LifetimeAfter += delta
	} else {
		posted.SpentAfter += -delta
	}
	return posted
}

// ── Rewards ──────────────────────────────────────────────────────────────────

// RewardType is what a reward hands over.
type RewardType string

const (
	RewardDiscount    RewardType = "DISCOUNT"
	RewardMerchandise RewardType = "MERCHANDISE"
	RewardVoucher     RewardType = "VOUCHER"
	// RewardCredits is the one place loyalty touches the wallet, and it only
	// goes one way: points can become credits, credits can never become points.
	RewardCredits RewardType = "CREDITS"
	RewardClass   RewardType = "CLASS"
	RewardCustom  RewardType = "CUSTOM"
)

// IsValidRewardType validates a type arriving from a request.
func IsValidRewardType(value string) bool {
	switch RewardType(value) {
	case RewardDiscount, RewardMerchandise, RewardVoucher,
		RewardCredits, RewardClass, RewardCustom:
		return true
	}
	return false
}

// Reward is something XP buys.
type Reward struct {
	ID               string         `json:"id"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	RewardType       RewardType     `json:"rewardType"`
	XPCost           int            `json:"xpCost"`
	RequiredTierCode *string        `json:"requiredTierCode"`
	RewardValue      map[string]any `json:"rewardValue"`
	StockTotal       *int           `json:"stockTotal"`
	StockRedeemed    int            `json:"stockRedeemed"`
	MaxPerMember     *int           `json:"maxPerMember"`
	ImageURL         *string        `json:"imageUrl"`
	StartsAt         *time.Time     `json:"startsAt"`
	EndsAt           *time.Time     `json:"endsAt"`
	Active           bool           `json:"active"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// StockRemaining is how many are left, and whether there is a limit at all.
func (r Reward) StockRemaining() (int, bool) {
	if r.StockTotal == nil {
		return 0, false
	}
	return *r.StockTotal - r.StockRedeemed, true
}

// RedemptionRejection explains why a reward cannot be claimed.
type RedemptionRejection string

const (
	RedeemRejectInactive     RedemptionRejection = "REWARD_INACTIVE"
	RedeemRejectNotStarted   RedemptionRejection = "NOT_STARTED"
	RedeemRejectEnded        RedemptionRejection = "ENDED"
	RedeemRejectOutOfStock   RedemptionRejection = "OUT_OF_STOCK"
	RedeemRejectTier         RedemptionRejection = "TIER_TOO_LOW"
	RedeemRejectPerMember    RedemptionRejection = "PER_MEMBER_LIMIT"
	RedeemRejectInsufficient RedemptionRejection = "INSUFFICIENT_XP"
	RedeemRejectSuspended    RedemptionRejection = "PROFILE_SUSPENDED"
)

// RedemptionRequest is what the rules judge.
type RedemptionRequest struct {
	Reward  Reward
	Profile MemberProfile
	Tiers   []Tier
	// AlreadyRedeemed is how many the member has claimed of this reward.
	AlreadyRedeemed int
	At              time.Time
}

// EvaluateRedemption decides whether a member may claim a reward.
//
// The order matters: eligibility is checked before affordability, so somebody
// who cannot have a thing is told that rather than being told to earn more
// points for something they will never be allowed.
func EvaluateRedemption(req RedemptionRequest) RedemptionRejection {
	if req.Profile.Status == "SUSPENDED" {
		return RedeemRejectSuspended
	}
	if !req.Reward.Active {
		return RedeemRejectInactive
	}
	if req.Reward.StartsAt != nil && req.At.Before(*req.Reward.StartsAt) {
		return RedeemRejectNotStarted
	}
	if req.Reward.EndsAt != nil && !req.At.Before(*req.Reward.EndsAt) {
		return RedeemRejectEnded
	}
	if req.Reward.RequiredTierCode != nil {
		if !tierAtLeast(req.Tiers, req.Profile.TierCode, *req.Reward.RequiredTierCode) {
			return RedeemRejectTier
		}
	}
	if remaining, limited := req.Reward.StockRemaining(); limited && remaining <= 0 {
		return RedeemRejectOutOfStock
	}
	if req.Reward.MaxPerMember != nil && req.AlreadyRedeemed >= *req.Reward.MaxPerMember {
		return RedeemRejectPerMember
	}
	if req.Profile.CurrentXP < req.Reward.XPCost {
		return RedeemRejectInsufficient
	}
	return ""
}

// tierAtLeast reports whether a member's tier ranks at or above a required one.
func tierAtLeast(tiers []Tier, memberCode, requiredCode string) bool {
	rankOf := map[string]int{}
	for _, tier := range tiers {
		rankOf[tier.Code] = tier.Rank
	}
	member, hasMember := rankOf[memberCode]
	required, hasRequired := rankOf[requiredCode]
	if !hasMember || !hasRequired {
		return false
	}
	return member >= required
}

// RedemptionStatus is where a claim has got to.
type RedemptionStatus string

const (
	RedemptionPending   RedemptionStatus = "PENDING"
	RedemptionApproved  RedemptionStatus = "APPROVED"
	RedemptionFulfilled RedemptionStatus = "FULFILLED"
	RedemptionCancelled RedemptionStatus = "CANCELLED"
	RedemptionExpired   RedemptionStatus = "EXPIRED"
)

// RedemptionTransitions. Cancelling gives the points back, which is why a
// fulfilled claim cannot be cancelled — the member already has the thing.
var RedemptionTransitions = TransitionMap[RedemptionStatus]{
	RedemptionPending:   {RedemptionApproved, RedemptionCancelled, RedemptionExpired},
	RedemptionApproved:  {RedemptionFulfilled, RedemptionCancelled, RedemptionExpired},
	RedemptionFulfilled: {},
	RedemptionCancelled: {},
	RedemptionExpired:   {},
}

// Redemption is somebody claiming a reward.
type Redemption struct {
	ID               string           `json:"id"`
	RedemptionNumber string           `json:"redemptionNumber"`
	MemberID         string           `json:"memberId"`
	RewardID         string           `json:"rewardId"`
	XPCost           int              `json:"xpCost"`
	XPLedgerID       *string          `json:"xpLedgerId"`
	Status           RedemptionStatus `json:"status"`
	VoucherCode      *string          `json:"voucherCode"`
	RequestedAt      time.Time        `json:"requestedAt"`
	ApprovedAt       *time.Time       `json:"approvedAt"`
	FulfilledAt      *time.Time       `json:"fulfilledAt"`
	CancelledAt      *time.Time       `json:"cancelledAt"`
	ExpiresAt        *time.Time       `json:"expiresAt"`
	Note             *string          `json:"note"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
}
