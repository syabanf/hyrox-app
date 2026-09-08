package domain

import (
	"testing"
	"time"
)

func loyaltyTiers() []Tier {
	return []Tier{
		{Code: "BRONZE", Name: "Bronze", Rank: 1, MinLifetimeXP: 0, XPMultiplier: 1, Active: true},
		{Code: "SILVER", Name: "Silver", Rank: 2, MinLifetimeXP: 500, XPMultiplier: 1.25, Active: true},
		{Code: "GOLD", Name: "Gold", Rank: 3, MinLifetimeXP: 2000, MinSpendIDR: 5_000_000,
			XPMultiplier: 1.5, DiscountPercent: 10, Active: true},
	}
}

func TestTierIsTheHighestBandBothThresholdsAllow(t *testing.T) {
	tiers := loyaltyTiers()

	tier, ok := ResolveTier(tiers, 0, 0)
	if !ok || tier.Code != "BRONZE" {
		t.Fatalf("a new member starts at the bottom, got %v", tier.Code)
	}

	tier, _ = ResolveTier(tiers, 900, 0)
	if tier.Code != "SILVER" {
		t.Fatalf("900 XP is silver, got %v", tier.Code)
	}

	// Enough points but not enough spend: both thresholds must be met.
	tier, _ = ResolveTier(tiers, 5000, 1_000_000)
	if tier.Code != "SILVER" {
		t.Fatalf("gold needs the spend as well, got %v", tier.Code)
	}

	tier, _ = ResolveTier(tiers, 5000, 9_000_000)
	if tier.Code != "GOLD" {
		t.Fatalf("want GOLD, got %v", tier.Code)
	}
}

func TestSpendingPointsCannotCostAMemberTheirTier(t *testing.T) {
	// Lifetime XP only ever grows, so redeeming a reward never demotes
	// anybody. A scheme that demotes people for using it teaches them not to.
	profile := MemberProfile{CurrentXP: 900, LifetimeXP: 900}
	posted := PostXP(profile, -800)
	if !posted.Allowed() {
		t.Fatalf("spending 800 of 900 must be allowed, got %q", posted.Rejection)
	}
	if posted.LifetimeAfter != 900 {
		t.Fatalf("lifetime XP must not fall: %v", posted.LifetimeAfter)
	}
	if posted.BalanceAfter != 100 || posted.SpentAfter != 800 {
		t.Fatalf("want 100 left and 800 spent, got %v/%v", posted.BalanceAfter, posted.SpentAfter)
	}

	tier, _ := ResolveTier(loyaltyTiers(), posted.LifetimeAfter, 0)
	if tier.Code != "SILVER" {
		t.Fatalf("the member is still silver, got %v", tier.Code)
	}
}

func TestPointsCannotGoNegativeOrNowhere(t *testing.T) {
	profile := MemberProfile{CurrentXP: 50, LifetimeXP: 50}

	if posted := PostXP(profile, -51); posted.Rejection != XPRejectInsufficient {
		t.Fatalf("want INSUFFICIENT_XP, got %q", posted.Rejection)
	}
	if posted := PostXP(profile, 0); posted.Rejection != XPRejectZero {
		t.Fatalf("want ZERO_DELTA, got %q", posted.Rejection)
	}
	suspended := MemberProfile{CurrentXP: 500, Status: "SUSPENDED"}
	if posted := PostXP(suspended, 10); posted.Rejection != XPRejectSuspended {
		t.Fatalf("want PROFILE_SUSPENDED, got %q", posted.Rejection)
	}
}

func TestEarningRaisesBothBalances(t *testing.T) {
	profile := MemberProfile{CurrentXP: 100, LifetimeXP: 300, SpentXP: 200}
	posted := PostXP(profile, 40)
	if posted.BalanceAfter != 140 || posted.LifetimeAfter != 340 {
		t.Fatalf("want 140 spendable and 340 lifetime, got %v/%v",
			posted.BalanceAfter, posted.LifetimeAfter)
	}
	// The invariant the database CHECK also holds.
	if posted.BalanceAfter+posted.SpentAfter != posted.LifetimeAfter {
		t.Fatal("current + spent must equal lifetime")
	}
}

func TestXPModesComputeWhatTheySay(t *testing.T) {
	event := XPEvent{Channel: XPFromPOS, AmountIDR: 137_500, Items: 3}

	fixed := XPRule{XPMode: XPFixed, XPValue: 25, Active: true}
	if got := ComputeXP(fixed, event, 1); got != 25 {
		t.Fatalf("fixed: want 25, got %v", got)
	}

	perItem := XPRule{XPMode: XPPerItem, XPValue: 5, Active: true}
	if got := ComputeXP(perItem, event, 1); got != 15 {
		t.Fatalf("per item: want 15, got %v", got)
	}

	// One point per 10.000 rupiah: 137.500 earns thirteen, not 13.75.
	perAmount := XPRule{XPMode: XPPerAmount, XPValue: 1, AmountStep: 10_000, Active: true}
	if got := ComputeXP(perAmount, event, 1); got != 13 {
		t.Fatalf("per amount: want 13, got %v", got)
	}

	percentage := XPRule{XPMode: XPPercentage, XPValue: 0.5, Active: true}
	if got := ComputeXP(percentage, event, 1); got != 687 {
		t.Fatalf("percentage: want 687, got %v", got)
	}
}

func TestTheTierMultiplierAppliesOnlyWhereTheRuleAllowsIt(t *testing.T) {
	event := XPEvent{Channel: XPFromPOS, AmountIDR: 100_000}
	rule := XPRule{XPMode: XPFixed, XPValue: 100, TierMultiplierEnabled: true, Active: true}

	if got := ComputeXP(rule, event, 1.5); got != 150 {
		t.Fatalf("want 150 with a gold multiplier, got %v", got)
	}

	// A welcome bonus should not be worth more to somebody who is already a
	// regular, so a rule can turn the multiplier off.
	rule.TierMultiplierEnabled = false
	if got := ComputeXP(rule, event, 1.5); got != 100 {
		t.Fatalf("want a flat 100, got %v", got)
	}
}

func TestTheCapIsAppliedAfterTheMultiplier(t *testing.T) {
	// A cap a multiplier can exceed is not a cap.
	cap := 120
	rule := XPRule{
		XPMode: XPFixed, XPValue: 100, TierMultiplierEnabled: true,
		MaxXPPerEvent: &cap, Active: true,
	}
	if got := ComputeXP(rule, XPEvent{}, 1.5); got != 120 {
		t.Fatalf("want the cap of 120, got %v", got)
	}
}

func TestTheMoreSpecificRuleWinsATie(t *testing.T) {
	classType := "clt_engine"
	general := XPRule{
		Code: "GENERAL", SourceChannel: XPFromClass, XPMode: XPFixed, XPValue: 10,
		Priority: 100, Active: true,
	}
	campaign := XPRule{
		Code: "ENGINE_DOUBLE", SourceChannel: XPFromClass, SourceID: &classType,
		XPMode: XPFixed, XPValue: 20, Priority: 100, Active: true,
	}

	// A campaign aimed at one class type beats the studio-wide default without
	// anybody having to remember to disable the default.
	rule, ok := SelectXPRule([]XPRule{general, campaign},
		XPEvent{Channel: XPFromClass, SourceID: "clt_engine"})
	if !ok || rule.Code != "ENGINE_DOUBLE" {
		t.Fatalf("want the specific rule, got %v", rule.Code)
	}

	// For a different class the general rule still applies.
	rule, ok = SelectXPRule([]XPRule{general, campaign},
		XPEvent{Channel: XPFromClass, SourceID: "clt_mobility"})
	if !ok || rule.Code != "GENERAL" {
		t.Fatalf("want the general rule, got %v", rule.Code)
	}

	// Priority still beats specificity.
	general.Priority = 500
	rule, _ = SelectXPRule([]XPRule{general, campaign},
		XPEvent{Channel: XPFromClass, SourceID: "clt_engine"})
	if rule.Code != "GENERAL" {
		t.Fatalf("higher priority wins, got %v", rule.Code)
	}
}

func TestRulesOutsideTheirWindowOrMinimumDoNotMatch(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-48 * time.Hour)
	future := now.Add(48 * time.Hour)

	notYet := XPRule{SourceChannel: XPFromPOS, StartsAt: &future, Active: true}
	if MatchesRule(notYet, XPEvent{Channel: XPFromPOS, At: now}) {
		t.Fatal("a rule that has not started must not match")
	}
	over := XPRule{SourceChannel: XPFromPOS, EndsAt: &past, Active: true}
	if MatchesRule(over, XPEvent{Channel: XPFromPOS, At: now}) {
		t.Fatal("a finished rule must not match")
	}
	tooSmall := XPRule{SourceChannel: XPFromPOS, MinAmount: 50_000, Active: true}
	if MatchesRule(tooSmall, XPEvent{Channel: XPFromPOS, AmountIDR: 20_000, At: now}) {
		t.Fatal("a spend below the minimum must not match")
	}
	if !MatchesRule(tooSmall, XPEvent{Channel: XPFromPOS, AmountIDR: 50_000, At: now}) {
		t.Fatal("exactly the minimum does match")
	}
}

func TestRedemptionChecksEligibilityBeforeAffordability(t *testing.T) {
	tiers := loyaltyTiers()
	gold := "GOLD"
	reward := Reward{
		Code: "GOLD_TOWEL", Name: "Gold towel", XPCost: 5_000,
		RequiredTierCode: &gold, Active: true,
	}
	// Somebody who cannot have a thing is told that, rather than told to earn
	// more points for something they will never be allowed.
	bronze := MemberProfile{TierCode: "BRONZE", CurrentXP: 10}
	if got := EvaluateRedemption(RedemptionRequest{Reward: reward, Profile: bronze, Tiers: tiers}); got != RedeemRejectTier {
		t.Fatalf("want TIER_TOO_LOW, got %q", got)
	}

	// The right tier but not enough points.
	poorGold := MemberProfile{TierCode: "GOLD", CurrentXP: 100}
	if got := EvaluateRedemption(RedemptionRequest{Reward: reward, Profile: poorGold, Tiers: tiers}); got != RedeemRejectInsufficient {
		t.Fatalf("want INSUFFICIENT_XP, got %q", got)
	}

	richGold := MemberProfile{TierCode: "GOLD", CurrentXP: 9_000}
	if got := EvaluateRedemption(RedemptionRequest{Reward: reward, Profile: richGold, Tiers: tiers}); got != "" {
		t.Fatalf("want it allowed, got %q", got)
	}
}

func TestRedemptionRespectsStockAndPerMemberLimits(t *testing.T) {
	tiers := loyaltyTiers()
	profile := MemberProfile{TierCode: "SILVER", CurrentXP: 10_000}

	total, limit := 5, 2
	reward := Reward{
		Code: "SHAKER", XPCost: 500, Active: true,
		StockTotal: &total, StockRedeemed: 5, MaxPerMember: &limit,
	}
	if got := EvaluateRedemption(RedemptionRequest{Reward: reward, Profile: profile, Tiers: tiers}); got != RedeemRejectOutOfStock {
		t.Fatalf("want OUT_OF_STOCK, got %q", got)
	}

	reward.StockRedeemed = 1
	if got := EvaluateRedemption(RedemptionRequest{
		Reward: reward, Profile: profile, Tiers: tiers, AlreadyRedeemed: 2,
	}); got != RedeemRejectPerMember {
		t.Fatalf("want PER_MEMBER_LIMIT, got %q", got)
	}

	// Unlimited stock has no remaining count to run out of.
	reward.StockTotal = nil
	if got := EvaluateRedemption(RedemptionRequest{
		Reward: reward, Profile: profile, Tiers: tiers, AlreadyRedeemed: 1,
	}); got != "" {
		t.Fatalf("want it allowed, got %q", got)
	}
}

func TestAFulfilledRedemptionCannotBeCancelled(t *testing.T) {
	// Cancelling gives the points back, and the member already has the thing.
	if _, err := Transition(RedemptionTransitions, RedemptionFulfilled, RedemptionCancelled); err == nil {
		t.Fatal("a fulfilled redemption cannot be cancelled")
	}
	if _, err := Transition(RedemptionTransitions, RedemptionApproved, RedemptionCancelled); err != nil {
		t.Fatalf("an approved one can be: %v", err)
	}
}
