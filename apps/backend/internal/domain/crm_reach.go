package domain

import (
	"math"
	"sort"
	"time"
)

// Reaching a member, and hearing back.
//
// The loyalty half of CRM is about what somebody has earned. This half is
// about whether we may talk to them, what they have achieved that is not
// points, what they said when they wrote in, and what they thought afterwards.
//
// Consent runs through all of it, and it is a hard exclusion rather than a
// preference to be weighed: an opt-out is enforced where the audience is
// built, not remembered by whoever builds it.

// ── Badges ───────────────────────────────────────────────────────────────────

// BadgeMetric is what earns a badge.
type BadgeMetric string

const (
	BadgeVisits   BadgeMetric = "VISITS"
	BadgeBookings BadgeMetric = "BOOKINGS"
	BadgeSpend    BadgeMetric = "SPEND_IDR"
	BadgeXP       BadgeMetric = "XP_EARNED"
	BadgeStreak   BadgeMetric = "STREAK_DAYS"
	BadgeReferral BadgeMetric = "REFERRALS"
	// BadgeManual is awarded by a person, for the things no counter can see.
	BadgeManual BadgeMetric = "MANUAL"
)

// IsValidBadgeMetric validates a metric arriving from a request.
func IsValidBadgeMetric(value string) bool {
	switch BadgeMetric(value) {
	case BadgeVisits, BadgeBookings, BadgeSpend, BadgeXP,
		BadgeStreak, BadgeReferral, BadgeManual:
		return true
	}
	return false
}

// Badge is something a member has done, as opposed to something they have
// spent. A tier moves both ways with activity; a badge is a fact about the
// past that never becomes untrue.
type Badge struct {
	ID          string      `json:"id"`
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Metric      BadgeMetric `json:"metric"`
	Threshold   float64     `json:"threshold"`
	BonusXP     int         `json:"bonusXp"`
	Icon        *string     `json:"icon"`
	SortOrder   int         `json:"sortOrder"`
	Active      bool        `json:"active"`
}

// MemberBadge is one earned.
type MemberBadge struct {
	ID          string    `json:"id"`
	MemberID    string    `json:"memberId"`
	BadgeID     string    `json:"badgeId"`
	EarnedValue float64   `json:"earnedValue"`
	EarnedAt    time.Time `json:"earnedAt"`
	AwardedBy   *string   `json:"awardedBy"`
	Note        *string   `json:"note"`
}

// MemberMetrics is what the counters currently say about one member.
type MemberMetrics struct {
	Visits     float64 `json:"visits"`
	Bookings   float64 `json:"bookings"`
	SpendIDR   float64 `json:"spendIdr"`
	XPEarned   float64 `json:"xpEarned"`
	StreakDays float64 `json:"streakDays"`
	Referrals  float64 `json:"referrals"`
}

// Value reads the counter a badge is measured against.
func (m MemberMetrics) Value(metric BadgeMetric) float64 {
	switch metric {
	case BadgeVisits:
		return m.Visits
	case BadgeBookings:
		return m.Bookings
	case BadgeSpend:
		return m.SpendIDR
	case BadgeXP:
		return m.XPEarned
	case BadgeStreak:
		return m.StreakDays
	case BadgeReferral:
		return m.Referrals
	}
	return 0
}

// EarnedBadges is which badges a member has just qualified for.
//
// Manual badges are never returned: they exist for the things no counter can
// see, and awarding one automatically would defeat the point of having them.
// Already-held badges are skipped rather than re-awarded, because a badge that
// can be earned twice is a counter wearing a badge's clothes.
func EarnedBadges(badges []Badge, metrics MemberMetrics, held map[string]bool) []Badge {
	earned := []Badge{}
	for _, badge := range badges {
		if !badge.Active || badge.Metric == BadgeManual || held[badge.ID] {
			continue
		}
		if metrics.Value(badge.Metric) >= badge.Threshold {
			earned = append(earned, badge)
		}
	}
	sort.Slice(earned, func(a, b int) bool { return earned[a].SortOrder < earned[b].SortOrder })
	return earned
}

// ── Consent ──────────────────────────────────────────────────────────────────

// ContactChannel is a way of reaching somebody.
type ContactChannel string

const (
	ContactPush     ContactChannel = "PUSH"
	ContactEmail    ContactChannel = "EMAIL"
	ContactWhatsApp ContactChannel = "WHATSAPP"
	ContactSMS      ContactChannel = "SMS"
)

// IsValidContactChannel validates a channel arriving from a request.
func IsValidContactChannel(value string) bool {
	switch ContactChannel(value) {
	case ContactPush, ContactEmail, ContactWhatsApp, ContactSMS:
		return true
	}
	return false
}

// MessageKind separates what a member can refuse from what they cannot.
type MessageKind string

const (
	// MessageMarketing is refusable: a campaign, an offer, a nudge.
	MessageMarketing MessageKind = "MARKETING"
	// MessageTransactional is the consequence of something they did — a
	// booking confirmation, a payment receipt. Opting out of marketing does
	// not opt out of being told their class is cancelled.
	MessageTransactional MessageKind = "TRANSACTIONAL"
)

// ContactPreference is what a member has said about one channel. Its existence
// is the point: no row means they have not been asked, which is a different
// fact from having said yes.
type ContactPreference struct {
	MemberID  string         `json:"memberId"`
	Channel   ContactChannel `json:"channel"`
	OptedIn   bool           `json:"optedIn"`
	Reason    *string        `json:"reason"`
	Scope     string         `json:"scope"`
	ChangedAt time.Time      `json:"changedAt"`
	ChangedBy *string        `json:"changedBy"`
}

// MayContact decides whether a message may be sent.
//
// Marketing is opt-out: silence means yes, because a member who signed up has
// agreed to hear from the place they signed up to. Transactional messages
// ignore a marketing opt-out entirely — telling somebody their class is
// cancelled is not marketing — but an ALL-scope refusal stops even those on
// that channel, because a member who says "never text me" means it.
func MayContact(prefs []ContactPreference, channel ContactChannel, kind MessageKind) bool {
	for _, pref := range prefs {
		if pref.Channel != channel {
			continue
		}
		if pref.OptedIn {
			return true
		}
		if pref.Scope == "ALL" {
			return false
		}
		return kind != MessageMarketing
	}
	return true
}

// ── Campaign delivery ────────────────────────────────────────────────────────

// RecipientStatus is what happened to one member's copy.
type RecipientStatus string

const (
	RecipientPending RecipientStatus = "PENDING"
	RecipientSent    RecipientStatus = "SENT"
	// RecipientSkipped is somebody deliberately left out, with a reason. It is
	// separate from FAILED because a skip is the system working.
	RecipientSkipped RecipientStatus = "SKIPPED"
	RecipientFailed  RecipientStatus = "FAILED"
	RecipientOpened  RecipientStatus = "OPENED"
	RecipientClicked RecipientStatus = "CLICKED"
)

// CampaignRecipient is one member's copy of one campaign.
type CampaignRecipient struct {
	ID             string          `json:"id"`
	CampaignID     string          `json:"campaignId"`
	MemberID       string          `json:"memberId"`
	Status         RecipientStatus `json:"status"`
	SkipReason     *string         `json:"skipReason"`
	NotificationID *string         `json:"notificationId"`
	SentAt         *time.Time      `json:"sentAt"`
	OpenedAt       *time.Time      `json:"openedAt"`
	ClickedAt      *time.Time      `json:"clickedAt"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// CampaignReport is what a send actually did.
type CampaignReport struct {
	Audience int `json:"audience"`
	Sent     int `json:"sent"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
	Opened   int `json:"opened"`
	Clicked  int `json:"clicked"`
	// Rates are against what was sent, not against the audience: a campaign
	// that reached forty people and was opened by twenty had a 50% open rate,
	// however many were skipped.
	OpenRate  float64 `json:"openRate"`
	ClickRate float64 `json:"clickRate"`
	// SkipReasons counts why people were left out, which is the number that
	// tells you your list is decaying.
	SkipReasons map[string]int `json:"skipReasons"`
}

// SummarizeCampaign counts a send.
//
// Opened and clicked are cumulative, not exclusive: somebody who clicked also
// opened, and reporting them as separate populations would make every campaign
// look worse than it was.
func SummarizeCampaign(recipients []CampaignRecipient) CampaignReport {
	report := CampaignReport{Audience: len(recipients), SkipReasons: map[string]int{}}
	for _, recipient := range recipients {
		switch recipient.Status {
		case RecipientSkipped:
			report.Skipped++
			reason := "unspecified"
			if recipient.SkipReason != nil && *recipient.SkipReason != "" {
				reason = *recipient.SkipReason
			}
			report.SkipReasons[reason]++
			continue
		case RecipientFailed:
			report.Failed++
			continue
		case RecipientPending:
			continue
		}
		report.Sent++
		if recipient.OpenedAt != nil {
			report.Opened++
		}
		if recipient.ClickedAt != nil {
			report.Clicked++
		}
	}
	if report.Sent > 0 {
		report.OpenRate = round2(float64(report.Opened) / float64(report.Sent) * 100)
		report.ClickRate = round2(float64(report.Clicked) / float64(report.Sent) * 100)
	}
	return report
}

// ── Conversations ────────────────────────────────────────────────────────────

// ConversationStatus is where a thread stands.
type ConversationStatus string

const (
	ConversationOpen ConversationStatus = "OPEN"
	// ConversationPending is waiting on the member, not on us.
	ConversationPending  ConversationStatus = "PENDING"
	ConversationResolved ConversationStatus = "RESOLVED"
	ConversationClosed   ConversationStatus = "CLOSED"
)

// ConversationTransitions. A closed thread is final; a resolved one reopens
// when the member writes again, which is the whole reason the two differ.
var ConversationTransitions = TransitionMap[ConversationStatus]{
	ConversationOpen:     {ConversationPending, ConversationResolved, ConversationClosed},
	ConversationPending:  {ConversationOpen, ConversationResolved, ConversationClosed},
	ConversationResolved: {ConversationOpen, ConversationClosed},
	ConversationClosed:   {},
}

// Conversation is one thread with one person.
type Conversation struct {
	ID                   string             `json:"id"`
	MemberID             *string            `json:"memberId"`
	ContactName          string             `json:"contactName"`
	ContactHandle        *string            `json:"contactHandle"`
	Channel              string             `json:"channel"`
	Subject              string             `json:"subject"`
	Status               ConversationStatus `json:"status"`
	Priority             string             `json:"priority"`
	AssignedTo           *string            `json:"assignedTo"`
	AssignedName         *string            `json:"assignedName"`
	BranchID             *string            `json:"branchId"`
	Tags                 []string           `json:"tags"`
	LastMemberAt         *time.Time         `json:"lastMemberAt"`
	LastStaffAt          *time.Time         `json:"lastStaffAt"`
	FirstResponseSeconds *int               `json:"firstResponseSeconds"`
	ResolvedAt           *time.Time         `json:"resolvedAt"`
	CreatedAt            time.Time          `json:"createdAt"`
	UpdatedAt            time.Time          `json:"updatedAt"`
}

// Waiting reports whether the member is waiting on us rather than the reverse.
//
// A thread nobody has answered is waiting; so is one where the member has
// written since our last reply. That second case is the one a status field
// alone gets wrong, because nobody remembers to set it back.
func (c Conversation) Waiting() bool {
	if c.Status == ConversationResolved || c.Status == ConversationClosed {
		return false
	}
	if c.LastMemberAt == nil {
		return false
	}
	return c.LastStaffAt == nil || c.LastStaffAt.Before(*c.LastMemberAt)
}

// WaitedSeconds is how long the member has been waiting, at a given moment.
func (c Conversation) WaitedSeconds(now time.Time) int {
	if !c.Waiting() || c.LastMemberAt == nil {
		return 0
	}
	return int(now.Sub(*c.LastMemberAt).Seconds())
}

// ConversationMessage is one thing said.
type ConversationMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Direction      string    `json:"direction"`
	Body           string    `json:"body"`
	AuthorID       *string   `json:"authorId"`
	AuthorName     *string   `json:"authorName"`
	TemplateID     *string   `json:"templateId"`
	ExternalID     *string   `json:"externalId"`
	Internal       bool      `json:"internal"`
	CreatedAt      time.Time `json:"createdAt"`
}

// InboxMetrics is how the desk is doing.
type InboxMetrics struct {
	Open     int `json:"open"`
	Pending  int `json:"pending"`
	Waiting  int `json:"waiting"`
	Resolved int `json:"resolved"`
	// Unassigned is the number nobody has picked up, which is the queue that
	// actually goes unanswered.
	Unassigned int `json:"unassigned"`
	// MedianFirstResponseSeconds, not the mean: one thread answered a week
	// late would otherwise make a good day look terrible.
	MedianFirstResponseSeconds int `json:"medianFirstResponseSeconds"`
	LongestWaitSeconds         int `json:"longestWaitSeconds"`
}

// SummarizeInbox counts the desk at one moment.
func SummarizeInbox(conversations []Conversation, now time.Time) InboxMetrics {
	metrics := InboxMetrics{}
	responses := []int{}

	for _, conversation := range conversations {
		switch conversation.Status {
		case ConversationOpen:
			metrics.Open++
		case ConversationPending:
			metrics.Pending++
		case ConversationResolved:
			metrics.Resolved++
		}
		if conversation.Waiting() {
			metrics.Waiting++
			if conversation.AssignedTo == nil {
				metrics.Unassigned++
			}
			if waited := conversation.WaitedSeconds(now); waited > metrics.LongestWaitSeconds {
				metrics.LongestWaitSeconds = waited
			}
		}
		if conversation.FirstResponseSeconds != nil {
			responses = append(responses, *conversation.FirstResponseSeconds)
		}
	}

	metrics.MedianFirstResponseSeconds = median(responses)
	return metrics
}

func median(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// ── Reviews ──────────────────────────────────────────────────────────────────

// Review is what a member thought.
type Review struct {
	ID          string     `json:"id"`
	MemberID    string     `json:"memberId"`
	SubjectType string     `json:"subjectType"`
	SubjectID   string     `json:"subjectId"`
	Rating      int        `json:"rating"`
	Comment     *string    `json:"comment"`
	Verified    bool       `json:"verified"`
	VisitID     *string    `json:"visitId"`
	Status      string     `json:"status"`
	Reply       *string    `json:"reply"`
	RepliedBy   *string    `json:"repliedBy"`
	RepliedAt   *time.Time `json:"repliedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// IsValidReviewSubject validates a subject arriving from a request.
func IsValidReviewSubject(value string) bool {
	switch value {
	case "SESSION", "COACH", "BRANCH", "PRODUCT":
		return true
	}
	return false
}

// ReviewSummary is how a thing is rated.
type ReviewSummary struct {
	Count   int     `json:"count"`
	Average float64 `json:"average"`
	// Distribution is counts by star, indexed 1..5, so a chart does not have
	// to recount them.
	Distribution map[int]int `json:"distribution"`
	// Verified are from members whose visit could be found. An average built
	// mostly from unverified reviews means something different.
	Verified    int `json:"verified"`
	Unanswered  int `json:"unanswered"`
	NegativeNew int `json:"negativeNew"`
}

// SummarizeReviews averages what was said.
//
// Hidden and flagged reviews are excluded: an average that counts reviews
// nobody can see is an average of a different set than the one on the page.
func SummarizeReviews(reviews []Review) ReviewSummary {
	summary := ReviewSummary{Distribution: map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}}
	total := 0
	for _, review := range reviews {
		if review.Status != "PUBLISHED" {
			continue
		}
		summary.Count++
		total += review.Rating
		summary.Distribution[review.Rating]++
		if review.Verified {
			summary.Verified++
		}
		if review.Reply == nil {
			summary.Unanswered++
			// Three stars or fewer and nobody has replied: the list somebody
			// should actually work through.
			if review.Rating <= 3 {
				summary.NegativeNew++
			}
		}
	}
	if summary.Count > 0 {
		summary.Average = math.Round(float64(total)/float64(summary.Count)*100) / 100
	}
	return summary
}
