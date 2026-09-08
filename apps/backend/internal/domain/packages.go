package domain

import "time"

// PackageStatus keeps sold packages around forever: one with transaction
// history is ARCHIVED, never deleted, so old payments still resolve.
type PackageStatus string

const (
	PackageActive   PackageStatus = "ACTIVE"
	PackageArchived PackageStatus = "ARCHIVED"
)

// CreditPackage is a purchasable bundle of credits.
type CreditPackage struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Credits      int    `json:"credits"`
	PriceIDR     int64  `json:"priceIdr"`
	ValidityDays int    `json:"validityDays"`
	// BranchID nil means the package is sold at every branch.
	BranchID               *string `json:"branchId"`
	PurchaseLimitPerMember *int    `json:"purchaseLimitPerMember"`
	// ApplicableClassTypeIDs nil means the credits book any class; a non-nil
	// list restricts them to those class types.
	ApplicableClassTypeIDs []string      `json:"applicableClassTypeIds"`
	Status                 PackageStatus `json:"status"`
	CreatedAt              time.Time     `json:"createdAt"`
}

// CoversClass reports whether this package's credits may book a class type.
func (p CreditPackage) CoversClass(classTypeID string) bool {
	if p.ApplicableClassTypeIDs == nil {
		return true
	}
	return contains(p.ApplicableClassTypeIDs, classTypeID)
}

// AvailableAt reports whether the package is on sale at a branch.
func (p CreditPackage) AvailableAt(branchID string) bool {
	return p.BranchID == nil || *p.BranchID == branchID
}
