package domain

import "time"

// MemberNotificationType is the reason a member was notified. The member app
// renders an icon and deep link per type.
type MemberNotificationType string

const (
	NotifyBookingConfirmed MemberNotificationType = "BOOKING_CONFIRMED"
	NotifyBookingReminder  MemberNotificationType = "BOOKING_REMINDER"
	NotifyWaitlistPromoted MemberNotificationType = "WAITLIST_PROMOTED"
	NotifyLowBalance       MemberNotificationType = "LOW_BALANCE"
	NotifyCreditExpiry     MemberNotificationType = "CREDIT_EXPIRY"
	NotifyVisitLogged      MemberNotificationType = "VISIT_LOGGED"
	NotifySessionChanged   MemberNotificationType = "SESSION_CHANGED"
	NotifyAnnouncement     MemberNotificationType = "ANNOUNCEMENT"
)

// MemberNotification is one in-app message.
type MemberNotification struct {
	ID        string                 `json:"id"`
	MemberID  string                 `json:"memberId"`
	Type      MemberNotificationType `json:"type"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	CreatedAt time.Time              `json:"createdAt"`
	ReadAt    *time.Time             `json:"readAt"`
}

// MemberSegment is the audience of a campaign.
type MemberSegment string

const (
	SegmentAllActive       MemberSegment = "ALL_ACTIVE"
	SegmentLowBalance      MemberSegment = "LOW_BALANCE"
	SegmentExpiringCredits MemberSegment = "EXPIRING_CREDITS"
	SegmentNewMembersOnly  MemberSegment = "NEW_MEMBERS"
	SegmentNoVisit14D      MemberSegment = "NO_VISIT_14D"
	SegmentCustom          MemberSegment = "CUSTOM"
)

// IsValidSegment validates a segment arriving from a request.
func IsValidSegment(value string) bool {
	switch MemberSegment(value) {
	case SegmentAllActive, SegmentLowBalance, SegmentExpiringCredits,
		SegmentNewMembersOnly, SegmentNoVisit14D, SegmentCustom:
		return true
	}
	return false
}

// SegmentFilter refines a CUSTOM audience. Every field is optional and they
// combine with AND.
type SegmentFilter struct {
	BranchID              *string `json:"branchId"`
	MaxBalance            *int    `json:"maxBalance"`
	MinDaysSinceLastVisit *int    `json:"minDaysSinceLastVisit"`
	JoinedWithinDays      *int    `json:"joinedWithinDays"`
}

// CampaignStatus is the send lifecycle.
type CampaignStatus string

const (
	CampaignDraft      CampaignStatus = "DRAFT"
	CampaignScheduled  CampaignStatus = "SCHEDULED"
	CampaignProcessing CampaignStatus = "PROCESSING"
	CampaignSent       CampaignStatus = "SENT"
	CampaignFailed     CampaignStatus = "FAILED"
	CampaignCancelled  CampaignStatus = "CANCELLED"
)

// CampaignTransitions route every send through PROCESSING, so a crash mid-send
// leaves an obvious state rather than a half-sent DRAFT.
var CampaignTransitions = TransitionMap[CampaignStatus]{
	CampaignDraft:      {CampaignScheduled, CampaignProcessing, CampaignCancelled},
	CampaignScheduled:  {CampaignProcessing, CampaignCancelled},
	CampaignProcessing: {CampaignSent, CampaignFailed},
	CampaignSent:       {},
	CampaignFailed:     {},
	CampaignCancelled:  {},
}

// Campaign is a broadcast to a member segment.
type Campaign struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Segment      MemberSegment  `json:"segment"`
	CustomFilter *SegmentFilter `json:"customFilter"`
	Message      string         `json:"message"`
	DeepLink     *string        `json:"deepLink"`
	ImageURL     *string        `json:"imageUrl"`
	ScheduledAt  *time.Time     `json:"scheduledAt"`
	Status       CampaignStatus `json:"status"`
	SentCount    *int           `json:"sentCount"`
	CreatedAt    time.Time      `json:"createdAt"`
}
