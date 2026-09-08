package domain

import (
	"math"
	"time"
)

// VoucherStatus is the publication state of a discount code.
type VoucherStatus string

const (
	VoucherDraft     VoucherStatus = "DRAFT"
	VoucherScheduled VoucherStatus = "SCHEDULED"
	VoucherActive    VoucherStatus = "ACTIVE"
	VoucherExpired   VoucherStatus = "EXPIRED"
	VoucherDisabled  VoucherStatus = "DISABLED"
)

// VoucherTransitions allow a disabled code to be switched back on, but an
// expired one is done: reissue instead.
var VoucherTransitions = TransitionMap[VoucherStatus]{
	VoucherDraft:     {VoucherScheduled, VoucherActive, VoucherDisabled},
	VoucherScheduled: {VoucherActive, VoucherDisabled},
	VoucherActive:    {VoucherExpired, VoucherDisabled},
	VoucherExpired:   {},
	VoucherDisabled:  {VoucherActive},
}

// VoucherType is how the discount is calculated.
type VoucherType string

const (
	VoucherFixedIDR VoucherType = "FIXED_IDR"
	VoucherPercent  VoucherType = "PERCENT"
)

// VoucherSegment limits who may redeem a code.
type VoucherSegment string

const (
	SegmentAll        VoucherSegment = "ALL"
	SegmentNewMembers VoucherSegment = "NEW_MEMBERS"
)

// Voucher is a discount code applied at checkout.
type Voucher struct {
	ID              string         `json:"id"`
	Code            string         `json:"code"`
	Type            VoucherType    `json:"type"`
	Value           int64          `json:"value"`
	StartsAt        time.Time      `json:"startsAt"`
	EndsAt          time.Time      `json:"endsAt"`
	UsageLimit      *int           `json:"usageLimit"`
	PerMemberLimit  *int           `json:"perMemberLimit"`
	EligibleSegment VoucherSegment `json:"eligibleSegment"`
	// ApplicablePackageIDs nil means the code works on every package.
	ApplicablePackageIDs []string      `json:"applicablePackageIds"`
	Status               VoucherStatus `json:"status"`
	CreatedAt            time.Time     `json:"createdAt"`
}

// VoucherRedemption records one use. Redemptions are separate rows rather than
// a counter on the voucher, so usage is auditable per member and per payment.
type VoucherRedemption struct {
	ID          string    `json:"id"`
	VoucherID   string    `json:"voucherId"`
	MemberID    string    `json:"memberId"`
	PaymentID   string    `json:"paymentId"`
	DiscountIDR int64     `json:"discountIdr"`
	CreatedAt   time.Time `json:"createdAt"`
}

// VoucherRejection is why a code was refused, surfaced to the client as
// VOUCHER_<REASON>.
type VoucherRejection string

const (
	VoucherNotActive             VoucherRejection = "NOT_ACTIVE"
	VoucherNotStarted            VoucherRejection = "NOT_STARTED"
	VoucherEnded                 VoucherRejection = "ENDED"
	VoucherUsageLimitReached     VoucherRejection = "USAGE_LIMIT_REACHED"
	VoucherPerMemberLimitReached VoucherRejection = "PER_MEMBER_LIMIT_REACHED"
	VoucherPackageNotEligible    VoucherRejection = "PACKAGE_NOT_ELIGIBLE"
	VoucherSegmentNotEligible    VoucherRejection = "SEGMENT_NOT_ELIGIBLE"
)

// VoucherCheck is the context a redemption is judged against.
type VoucherCheck struct {
	Voucher               Voucher
	Package               CreditPackage
	MemberIsNew           bool
	MemberRedemptionCount int
	TotalRedemptionCount  int
	Now                   time.Time
}

// ValidateVoucher returns the discount in IDR, or the reason for refusal.
// The discount can never exceed the package price.
func ValidateVoucher(c VoucherCheck) (int64, VoucherRejection) {
	v := c.Voucher
	switch {
	case v.Status != VoucherActive:
		return 0, VoucherNotActive
	case c.Now.Before(v.StartsAt):
		return 0, VoucherNotStarted
	case c.Now.After(v.EndsAt):
		return 0, VoucherEnded
	case v.UsageLimit != nil && c.TotalRedemptionCount >= *v.UsageLimit:
		return 0, VoucherUsageLimitReached
	case v.PerMemberLimit != nil && c.MemberRedemptionCount >= *v.PerMemberLimit:
		return 0, VoucherPerMemberLimitReached
	case v.ApplicablePackageIDs != nil && !contains(v.ApplicablePackageIDs, c.Package.ID):
		return 0, VoucherPackageNotEligible
	case v.EligibleSegment == SegmentNewMembers && !c.MemberIsNew:
		return 0, VoucherSegmentNotEligible
	}

	if v.Type == VoucherFixedIDR {
		return min64(v.Value, c.Package.PriceIDR), ""
	}
	discount := int64(math.Round(float64(c.Package.PriceIDR) * float64(v.Value) / 100))
	return min64(discount, c.Package.PriceIDR), ""
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
