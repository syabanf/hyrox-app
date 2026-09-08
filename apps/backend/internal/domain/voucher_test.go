package domain

import (
	"testing"
	"time"
)

func testPackage(price int64) CreditPackage {
	return CreditPackage{ID: "pkg_1", Name: "Starter 5", Credits: 5, PriceIDR: price, ValidityDays: 60, Status: PackageActive}
}

func testVoucher(mods ...func(*Voucher)) Voucher {
	v := Voucher{
		ID:              "vou_1",
		Code:            "WELCOME10",
		Type:            VoucherPercent,
		Value:           10,
		StartsAt:        testNow.Add(-24 * time.Hour),
		EndsAt:          testNow.Add(24 * time.Hour),
		EligibleSegment: SegmentAll,
		Status:          VoucherActive,
	}
	for _, m := range mods {
		m(&v)
	}
	return v
}

func TestValidateVoucherDiscountMath(t *testing.T) {
	percent := VoucherCheck{Voucher: testVoucher(), Package: testPackage(800_000), Now: testNow}
	discount, rejection := ValidateVoucher(percent)
	if rejection != "" {
		t.Fatalf("rejected with %s", rejection)
	}
	if discount != 80_000 {
		t.Fatalf("percent discount = %d, want 80000", discount)
	}

	fixed := VoucherCheck{
		Voucher: testVoucher(func(v *Voucher) { v.Type = VoucherFixedIDR; v.Value = 100_000 }),
		Package: testPackage(800_000), Now: testNow,
	}
	if discount, _ := ValidateVoucher(fixed); discount != 100_000 {
		t.Fatalf("fixed discount = %d, want 100000", discount)
	}
}

func TestValidateVoucherNeverDiscountsBelowFree(t *testing.T) {
	// A fixed voucher worth more than the package caps at the package price,
	// so a top-up can never produce a negative total.
	oversized := VoucherCheck{
		Voucher: testVoucher(func(v *Voucher) { v.Type = VoucherFixedIDR; v.Value = 999_999 }),
		Package: testPackage(300_000), Now: testNow,
	}
	if discount, _ := ValidateVoucher(oversized); discount != 300_000 {
		t.Fatalf("capped discount = %d, want 300000", discount)
	}

	full := VoucherCheck{
		Voucher: testVoucher(func(v *Voucher) { v.Value = 100 }),
		Package: testPackage(300_000), Now: testNow,
	}
	if discount, _ := ValidateVoucher(full); discount != 300_000 {
		t.Fatalf("100 percent discount = %d, want 300000", discount)
	}
}

func TestValidateVoucherRejectionMatrix(t *testing.T) {
	tests := []struct {
		name  string
		check VoucherCheck
		want  VoucherRejection
	}{
		{
			name:  "draft voucher is not redeemable",
			check: VoucherCheck{Voucher: testVoucher(func(v *Voucher) { v.Status = VoucherDraft }), Package: testPackage(500_000), Now: testNow},
			want:  VoucherNotActive,
		},
		{
			name:  "before the window opens",
			check: VoucherCheck{Voucher: testVoucher(func(v *Voucher) { v.StartsAt = testNow.Add(time.Hour) }), Package: testPackage(500_000), Now: testNow},
			want:  VoucherNotStarted,
		},
		{
			name:  "after the window closes",
			check: VoucherCheck{Voucher: testVoucher(func(v *Voucher) { v.EndsAt = testNow.Add(-time.Hour) }), Package: testPackage(500_000), Now: testNow},
			want:  VoucherEnded,
		},
		{
			name: "global usage limit reached",
			check: VoucherCheck{
				Voucher: testVoucher(func(v *Voucher) { limit := 100; v.UsageLimit = &limit }),
				Package: testPackage(500_000), TotalRedemptionCount: 100, Now: testNow,
			},
			want: VoucherUsageLimitReached,
		},
		{
			name: "per-member limit reached",
			check: VoucherCheck{
				Voucher: testVoucher(func(v *Voucher) { limit := 1; v.PerMemberLimit = &limit }),
				Package: testPackage(500_000), MemberRedemptionCount: 1, Now: testNow,
			},
			want: VoucherPerMemberLimitReached,
		},
		{
			name: "package outside the eligible list",
			check: VoucherCheck{
				Voucher: testVoucher(func(v *Voucher) { v.ApplicablePackageIDs = []string{"pkg_other"} }),
				Package: testPackage(500_000), Now: testNow,
			},
			want: VoucherPackageNotEligible,
		},
		{
			name: "new-member code used by an existing member",
			check: VoucherCheck{
				Voucher: testVoucher(func(v *Voucher) { v.EligibleSegment = SegmentNewMembers }),
				Package: testPackage(500_000), MemberIsNew: false, Now: testNow,
			},
			want: VoucherSegmentNotEligible,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			discount, rejection := ValidateVoucher(tc.check)
			if rejection != tc.want {
				t.Fatalf("rejection = %s, want %s", rejection, tc.want)
			}
			if discount != 0 {
				t.Fatalf("rejected voucher returned a %d discount", discount)
			}
		})
	}
}

func TestValidateVoucherAllowsAnEmptyEligibleListToMeanEverything(t *testing.T) {
	check := VoucherCheck{
		Voucher:     testVoucher(func(v *Voucher) { v.EligibleSegment = SegmentNewMembers }),
		Package:     testPackage(500_000),
		MemberIsNew: true,
		Now:         testNow,
	}
	if _, rejection := ValidateVoucher(check); rejection != "" {
		t.Fatalf("new member rejected with %s", rejection)
	}
}

func TestVoucherTransitionsAllowReenablingButNotResurrection(t *testing.T) {
	if _, err := Transition(VoucherTransitions, VoucherDisabled, VoucherActive); err != nil {
		t.Fatalf("DISABLED -> ACTIVE should be legal: %v", err)
	}
	if _, err := Transition(VoucherTransitions, VoucherExpired, VoucherActive); err == nil {
		t.Fatal("EXPIRED -> ACTIVE must be rejected")
	}
}

func TestPackageCoverageRules(t *testing.T) {
	open := testPackage(500_000)
	if !open.CoversClass("clt_anything") {
		t.Fatal("a package with no class restriction must cover every class")
	}

	restricted := testPackage(500_000)
	restricted.ApplicableClassTypeIDs = []string{"clt_gym"}
	if !restricted.CoversClass("clt_gym") {
		t.Fatal("restricted package must cover its listed class")
	}
	if restricted.CoversClass("clt_hyrox") {
		t.Fatal("restricted package must not cover an unlisted class")
	}

	branchBound := testPackage(500_000)
	branch := "brn_senopati"
	branchBound.BranchID = &branch
	if !branchBound.AvailableAt("brn_senopati") || branchBound.AvailableAt("brn_pik") {
		t.Fatal("branch-bound package availability is wrong")
	}
}
