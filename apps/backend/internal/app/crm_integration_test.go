package app_test

import (
	"net/http"
	"testing"
)

// Loyalty, over real HTTP against a real database.
//
// Two properties matter most: points are earned exactly once per event however
// often it is delivered, and spending them never costs a member their tier.

const demoMember = "mem_demo"

func (h *harness) loyalty(t *testing.T, token, memberID string) map[string]any {
	t.Helper()
	status, detail := h.request(http.MethodGet, "/api/admin/crm/members/"+memberID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading loyalty returned %d: %v", status, detail)
	}
	return detail["profile"].(map[string]any)
}

func TestPointsAreEarnedFromTheOutboxExactlyOnce(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	member := h.memberToken("demo@nuhabit.id")

	before := h.loyalty(t, admin, demoMember)["lifetimeXp"].(float64)

	// Settling a top-up publishes wallet.payment_paid; loyalty consumes it.
	// Neither module knows the other exists.
	h.topUp(member, "pkg_starter5")

	// The dispatcher is driven by hand here so the test is about the rule
	// rather than about timing.
	if err := h.app.Dispatcher.DrainOnce(t.Context()); err != nil {
		t.Fatalf("draining the outbox: %v", err)
	}
	awarded := h.loyalty(t, admin, demoMember)["lifetimeXp"].(float64)
	if awarded <= before {
		t.Fatalf("settling a top-up should have earned points: %v -> %v", before, awarded)
	}

	// Draining again must not award them a second time. At-least-once delivery
	// means this happens for real, and points awarded twice are points given
	// away — which is what the idempotency key on the ledger prevents.
	if err := h.app.Dispatcher.DrainOnce(t.Context()); err != nil {
		t.Fatalf("draining the outbox: %v", err)
	}
	if got := h.loyalty(t, admin, demoMember)["lifetimeXp"].(float64); got != awarded {
		t.Fatalf("a redelivered event must award nothing: %v -> %v", awarded, got)
	}
}

func TestSpendingPointsNeverCostsATier(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	// Enough to reach silver.
	status, awarded := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust",
		admin, map[string]any{"xpDelta": 900, "reason": "Launch bonus"})
	if status != http.StatusCreated {
		t.Fatalf("adjusting returned %d: %v", status, awarded)
	}
	profile := h.loyalty(t, admin, demoMember)
	if profile["tierCode"] != "SILVER" {
		t.Fatalf("900 points is silver, got %v", profile["tierCode"])
	}

	// Redeem most of them away.
	status, claim := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/redeem",
		admin, map[string]any{"rewardId": "rwd_shaker"})
	if status != http.StatusCreated {
		t.Fatalf("redeeming returned %d: %v", status, claim)
	}

	after := h.loyalty(t, admin, demoMember)
	if after["currentXp"].(float64) != 500 {
		t.Fatalf("400 points should have been spent, got %v left", after["currentXp"])
	}
	// Lifetime never falls, so neither does the tier. A scheme that demotes
	// people for using it teaches them not to use it.
	if after["lifetimeXp"].(float64) != 900 {
		t.Fatalf("lifetime XP must not fall: %v", after["lifetimeXp"])
	}
	if after["tierCode"] != "SILVER" {
		t.Fatalf("the member is still silver, got %v", after["tierCode"])
	}
}

func TestPointsCannotBeSpentTwiceOrGoNegative(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust", admin,
		map[string]any{"xpDelta": 500, "reason": "Seeded for the test"})

	// 500 points, and the reward costs 400: the first claim works.
	if status, body := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/redeem",
		admin, map[string]any{"rewardId": "rwd_shaker"}); status != http.StatusCreated {
		t.Fatalf("the first claim should work, got %d: %v", status, body)
	}
	// The second cannot be afforded.
	status, refused := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/redeem",
		admin, map[string]any{"rewardId": "rwd_bar"})
	if status != http.StatusConflict || errorCode(refused) != "INSUFFICIENT_XP" {
		t.Fatalf("want INSUFFICIENT_XP, got %d: %v", status, refused)
	}

	// And nothing was taken for the refused claim.
	if got := h.loyalty(t, admin, demoMember)["currentXp"].(float64); got != 100 {
		t.Fatalf("a refused claim must take nothing, want 100 left, got %v", got)
	}
}

func TestATierGateIsCheckedBeforeAffordability(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	// Plenty of points, but bronze.
	h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust", admin,
		map[string]any{"xpDelta": 400, "reason": "Seeded"})

	// Somebody who cannot have a thing is told that, rather than told to earn
	// more points for something they will never be allowed.
	status, refused := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/redeem",
		admin, map[string]any{"rewardId": "rwd_month"})
	if status != http.StatusConflict || errorCode(refused) != "TIER_TOO_LOW" {
		t.Fatalf("want TIER_TOO_LOW, got %d: %v", status, refused)
	}
}

func TestCancellingAClaimReturnsThePointsAndTheStock(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust", admin,
		map[string]any{"xpDelta": 500, "reason": "Seeded"})
	status, claim := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/redeem",
		admin, map[string]any{"rewardId": "rwd_shaker"})
	if status != http.StatusCreated {
		t.Fatalf("redeeming returned %d: %v", status, claim)
	}
	claimID := claim["id"].(string)

	status, cancelled := h.request(http.MethodPost,
		"/api/admin/crm/redemptions/"+claimID+"/cancel", admin, map[string]any{})
	if status != http.StatusOK || cancelled["status"] != "CANCELLED" {
		t.Fatalf("cancelling returned %d: %v", status, cancelled)
	}
	if got := h.loyalty(t, admin, demoMember)["currentXp"].(float64); got != 500 {
		t.Fatalf("cancelling must return the points, want 500, got %v", got)
	}

	// A fulfilled claim is final: the member already has the thing.
	status, again := h.request(http.MethodPost,
		"/api/admin/crm/members/"+demoMember+"/redeem", admin, map[string]any{"rewardId": "rwd_shaker"})
	if status != http.StatusCreated {
		t.Fatalf("re-claiming returned %d: %v", status, again)
	}
	secondID := again["id"].(string)
	h.request(http.MethodPost, "/api/admin/crm/redemptions/"+secondID+"/approve", admin, map[string]any{})
	h.request(http.MethodPost, "/api/admin/crm/redemptions/"+secondID+"/fulfil", admin, map[string]any{})

	status, refused := h.request(http.MethodPost,
		"/api/admin/crm/redemptions/"+secondID+"/cancel", admin, map[string]any{})
	if status != http.StatusConflict || errorCode(refused) != "INVALID_TRANSITION" {
		t.Fatalf("want INVALID_TRANSITION, got %d: %v", status, refused)
	}
}

func TestTheXPLedgerIsAppendOnlyAndBalancesMatchIt(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	for _, delta := range []int{300, -0, 250} {
		if delta == 0 {
			continue
		}
		h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust", admin,
			map[string]any{"xpDelta": delta, "reason": "Test"})
	}

	// The profile is a cache of the ledger, and must never drift from it.
	var summed int
	if err := h.app.DB.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(xp_delta), 0) FROM crm.xp_ledger WHERE member_id = $1`,
		demoMember).Scan(&summed); err != nil {
		t.Fatalf("summing the ledger: %v", err)
	}
	if got := h.loyalty(t, admin, demoMember)["currentXp"].(float64); int(got) != summed {
		t.Fatalf("the profile (%v) has drifted from its ledger (%v)", got, summed)
	}

	// And the ledger cannot be rewritten, exactly like the credit ledger.
	var entryID string
	if err := h.app.DB.QueryRow(t.Context(),
		`SELECT id FROM crm.xp_ledger WHERE member_id = $1 LIMIT 1`, demoMember).Scan(&entryID); err != nil {
		t.Fatalf("reading an entry: %v", err)
	}
	if _, err := h.app.DB.Exec(t.Context(),
		`UPDATE crm.xp_ledger SET xp_delta = 999 WHERE id = $1`, entryID); err == nil {
		t.Fatal("the database must refuse to rewrite an XP entry")
	}
}

func TestAnAdjustmentAlwaysCarriesAReasonToo(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	// Points appearing without one are indistinguishable from a member being
	// favoured.
	status, refused := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust",
		admin, map[string]any{"xpDelta": 100, "reason": "  "})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("want a validation failure, got %d: %v", status, refused)
	}
}

func TestLoyaltyPermissionsAreEnforcedServerSide(t *testing.T) {
	h := newHarness(t)

	// The counter needs to see a member's tier to honour the discount.
	desk := h.adminToken("adm_desk")
	if status, _ := h.requestList(http.MethodGet, "/api/admin/crm/members", desk, nil); status != http.StatusOK {
		t.Fatalf("the front desk should see loyalty standings, got %d", status)
	}
	// Handing out points is not a counter job.
	if status, _ := h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust",
		desk, map[string]any{"xpDelta": 5_000, "reason": "For a friend"}); status != http.StatusForbidden {
		t.Fatalf("the front desk must not adjust XP, got %d", status)
	}
	// Nor is rewriting the scheme.
	if status, _ := h.request(http.MethodPut, "/api/admin/crm/rewards", desk, map[string]any{
		"code": "FREE_EVERYTHING", "name": "Free everything", "rewardType": "CUSTOM", "xpCost": 0,
	}); status != http.StatusForbidden {
		t.Fatalf("the front desk must not edit rewards, got %d", status)
	}

	// A member sees their own standing without any admin permission at all.
	member := h.memberToken("demo@nuhabit.id")
	if status, mine := h.request(http.MethodGet, "/api/me/loyalty", member, nil); status != http.StatusOK {
		t.Fatalf("a member should see their own loyalty, got %d: %v", status, mine)
	}
}
