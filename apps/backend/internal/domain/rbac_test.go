package domain

import "testing"

func TestSuperAdminHoldsEveryPermission(t *testing.T) {
	for _, p := range Permissions {
		if !HasPermission(RoleSuperAdmin, p) {
			t.Fatalf("SUPER_ADMIN is missing %s", p)
		}
	}
}

func TestHQAdminIsBlockedFromReshapingTheSystem(t *testing.T) {
	// HQ Admin runs the business but cannot create staff accounts or rewrite
	// the business rules; those stay with Super Admin.
	if HasPermission(RoleHQAdmin, PermUsersManage) {
		t.Fatal("HQ_ADMIN must not manage staff accounts")
	}
	if HasPermission(RoleHQAdmin, PermRulesUpdate) {
		t.Fatal("HQ_ADMIN must not change business rules")
	}
	if !HasPermission(RoleHQAdmin, PermRefundsManage) {
		t.Fatal("HQ_ADMIN should still handle refunds")
	}
	if len(PermissionsFor(RoleHQAdmin)) != len(Permissions)-2 {
		t.Fatalf("HQ_ADMIN holds %d permissions, want %d", len(PermissionsFor(RoleHQAdmin)), len(Permissions)-2)
	}
}

func TestSeparationOfDutiesBetweenFinanceAndOperations(t *testing.T) {
	// Money is Finance's; the schedule is not.
	if !HasPermission(RoleFinance, PermRefundsManage) {
		t.Fatal("FINANCE should manage refunds")
	}
	if HasPermission(RoleFinance, PermSessionsManage) {
		t.Fatal("FINANCE must not schedule classes")
	}

	// The floor is Branch Manager's; the money is not.
	if !HasPermission(RoleBranchManager, PermSessionsManage) {
		t.Fatal("BRANCH_MANAGER should schedule classes")
	}
	if HasPermission(RoleBranchManager, PermRefundsManage) {
		t.Fatal("BRANCH_MANAGER must not issue refunds")
	}
	if HasPermission(RoleBranchManager, PermMembersAdjustCredit) {
		t.Fatal("BRANCH_MANAGER must not hand out credits")
	}
}

func TestFrontDeskAndCoachAreTightlyScoped(t *testing.T) {
	allowed := map[Permission]bool{
		PermDashboardView: true, PermMembersView: true, PermLedgerView: true,
		PermOperationsView: true, PermBookingsManage: true, PermAttendanceManage: true,
		PermAccessView: true, PermAccessSimulate: true, PermCommercialView: true,
		PermPaymentsView: true,
		// The desk runs the till and answers "have you got this in a medium",
		// so it sells, and it can look at stock and at a member's tier.
		PermPOSView: true, PermPOSSell: true,
		PermInventoryView: true, PermCRMView: true,
	}
	for _, p := range Permissions {
		if got := HasPermission(RoleFrontDesk, p); got != allowed[p] {
			t.Fatalf("FRONT_DESK %s = %v, want %v", p, got, allowed[p])
		}
	}

	// The lines that matter on the other side of that: taking money is not the
	// same as unwinding a sale, seeing a shelf is not the same as rewriting
	// what is on it, and nobody at the counter signs for a purchase or hands
	// out loyalty points by hand.
	for _, denied := range []Permission{
		PermPOSVoid, PermInventoryManage, PermInventoryCount,
		PermPurchasingView, PermPurchasingApprove, PermPurchasingReceive,
		PermCRMAdjust, PermCRMManage,
	} {
		if HasPermission(RoleFrontDesk, denied) {
			t.Fatalf("FRONT_DESK must not hold %s", denied)
		}
	}

	// A coach sees their classes and marks attendance, nothing more.
	if !HasPermission(RoleCoach, PermAttendanceManage) {
		t.Fatal("COACH should mark attendance")
	}
	if HasPermission(RoleCoach, PermPaymentsView) || HasPermission(RoleCoach, PermBookingsManage) {
		t.Fatal("COACH must not reach payments or bookings")
	}
	// A coach teaches. The till, the stockroom and the purchase ledger are
	// somebody else's job entirely.
	for _, denied := range []Permission{
		PermPOSView, PermPOSSell, PermInventoryView, PermPurchasingView, PermCRMView,
	} {
		if HasPermission(RoleCoach, denied) {
			t.Fatalf("COACH must not hold %s", denied)
		}
	}
}

func TestPermissionsForIsACopyNotTheLiveMatrix(t *testing.T) {
	perms := PermissionsFor(RoleCoach)
	if len(perms) == 0 {
		t.Fatal("COACH should hold some permissions")
	}
	perms[0] = "tampered"
	if RolePermissions[RoleCoach][0] == "tampered" {
		t.Fatal("PermissionsFor must not expose the underlying matrix")
	}
	if len(PermissionsFor("NOT_A_ROLE")) != 0 {
		t.Fatal("an unknown role must hold nothing")
	}
}

func TestResolveRulesMergesBranchOverrides(t *testing.T) {
	base := DefaultBusinessRules()
	ttl := 90
	policy := PolicyFree
	override := &RulesOverride{QRTTLSeconds: &ttl, NoShowPolicy: &policy}

	merged := ResolveRules(base, override)
	if merged.QRTTLSeconds != 90 {
		t.Fatalf("qr ttl = %d, want the branch override 90", merged.QRTTLSeconds)
	}
	if merged.NoShowPolicy != PolicyFree {
		t.Fatalf("no-show policy = %s, want FREE", merged.NoShowPolicy)
	}
	// Untouched rules keep the organization value.
	if merged.AntiPassbackMinutes != base.AntiPassbackMinutes {
		t.Fatalf("anti-passback drifted to %d", merged.AntiPassbackMinutes)
	}
	if got := ResolveRules(base, nil); got != base {
		t.Fatal("a nil override must leave the defaults untouched")
	}
}
