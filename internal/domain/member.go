package domain

import "time"

// MemberStatus controls whether a member may book and enter.
type MemberStatus string

const (
	MemberActive    MemberStatus = "ACTIVE"
	MemberSuspended MemberStatus = "SUSPENDED"
	MemberInactive  MemberStatus = "INACTIVE"
	MemberArchived  MemberStatus = "ARCHIVED"
)

// MemberTransitions keeps status changes reversible except for archival, which
// is terminal so historical records always resolve to a member.
var MemberTransitions = TransitionMap[MemberStatus]{
	MemberActive:    {MemberSuspended, MemberInactive, MemberArchived},
	MemberSuspended: {MemberActive, MemberArchived},
	MemberInactive:  {MemberActive, MemberArchived},
	MemberArchived:  {},
}

// Gender is optional profile data.
type Gender string

const (
	GenderMale   Gender = "MALE"
	GenderFemale Gender = "FEMALE"
	GenderOther  Gender = "OTHER"
)

// EmergencyContact is captured at registration alongside the waiver.
type EmergencyContact struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Relation string `json:"relation"`
}

// Member is a studio customer. The credit balance is deliberately absent: it
// is always derived from the ledger, never stored on the member row.
type Member struct {
	ID                string            `json:"id"`
	FullName          string            `json:"fullName"`
	Email             string            `json:"email"`
	Phone             string            `json:"phone"`
	DateOfBirth       *time.Time        `json:"dateOfBirth"`
	Gender            *Gender           `json:"gender"`
	EmergencyContact  *EmergencyContact `json:"emergencyContact"`
	PreferredBranchID *string           `json:"preferredBranchId"`
	AvatarURL         *string           `json:"avatarUrl"`
	Status            MemberStatus      `json:"status"`
	WaiverVersion     *string           `json:"waiverVersion"`
	WaiverAcceptedAt  *time.Time        `json:"waiverAcceptedAt"`
	Notes             *string           `json:"notes"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
}

// IsActive reports whether the member may book classes and pass the gate.
func (m Member) IsActive() bool { return m.Status == MemberActive }

// IsValidMemberStatus validates a status arriving from a request.
func IsValidMemberStatus(value string) bool {
	switch MemberStatus(value) {
	case MemberActive, MemberSuspended, MemberInactive, MemberArchived:
		return true
	}
	return false
}
