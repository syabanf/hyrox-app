package domain

import (
	"strings"
	"time"
)

// Things that happened somewhere else.
//
// A race timing system knows somebody finished; a partner gym knows they
// trained. Each is a fact this system did not observe, and holding it honestly
// means keeping who said it, when, and what they sent — separately from what
// we decided it meant.

// PartnerKind is what sort of thing a partner is.
type PartnerKind string

const (
	PartnerRace    PartnerKind = "RACE"
	PartnerGym     PartnerKind = "GYM"
	PartnerHealth  PartnerKind = "HEALTH"
	PartnerRetail  PartnerKind = "RETAIL"
	PartnerPayment PartnerKind = "PAYMENT"
	PartnerOther   PartnerKind = "OTHER"
)

// IsValidPartnerKind validates a kind arriving from a request.
func IsValidPartnerKind(value string) bool {
	switch PartnerKind(value) {
	case PartnerRace, PartnerGym, PartnerHealth, PartnerRetail, PartnerPayment, PartnerOther:
		return true
	}
	return false
}

// IntegrationPartner is somebody who sends us facts.
type IntegrationPartner struct {
	ID           string      `json:"id"`
	Code         string      `json:"code"`
	Name         string      `json:"name"`
	Kind         PartnerKind `json:"kind"`
	ContactName  *string     `json:"contactName"`
	ContactEmail *string     `json:"contactEmail"`
	// AwardsXP separates a partner we merely record from one we pay out on.
	// Recording a fact and acting on it are different levels of trust.
	AwardsXP  bool      `json:"awardsXp"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// HasSecret says whether the partner can post at all, without ever
	// putting the secret itself in a response.
	HasSecret bool `json:"hasSecret"`
}

// ExternalEventStatus is what became of a claim from outside.
type ExternalEventStatus string

const (
	// EventReceived is stored but not yet matched to anybody.
	EventReceived ExternalEventStatus = "RECEIVED"
	EventMatched  ExternalEventStatus = "MATCHED"
	// EventProcessed is matched and acted on — points awarded, badge checked.
	EventProcessed ExternalEventStatus = "PROCESSED"
	// EventUnmatched is about somebody we cannot identify. It is a queue for a
	// person, not an error: the member may join next week.
	EventUnmatched ExternalEventStatus = "UNMATCHED"
	EventIgnored   ExternalEventStatus = "IGNORED"
	EventFailed    ExternalEventStatus = "FAILED"
)

// ExternalEvent is one claim from a partner.
type ExternalEvent struct {
	ID         string `json:"id"`
	PartnerID  string `json:"partnerId"`
	ExternalID string `json:"externalId"`
	EventType  string `json:"eventType"`
	// Subject is who the partner said it was about, in their terms — an email,
	// a bib number, a phone. Kept alongside our match so a wrong match can be
	// corrected without losing what they actually sent.
	Subject     string              `json:"subject"`
	MemberID    *string             `json:"memberId"`
	OccurredAt  time.Time           `json:"occurredAt"`
	Payload     map[string]any      `json:"payload"`
	Status      ExternalEventStatus `json:"status"`
	XPAwarded   int                 `json:"xpAwarded"`
	Error       *string             `json:"error"`
	ProcessedAt *time.Time          `json:"processedAt"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

// MatchCandidate is one member an event's subject might mean.
type MatchCandidate struct {
	MemberID string
	Email    string
	Phone    string
	FullName string
}

// MatchSubject works out which member a partner meant.
//
// Email first, then phone, and never name: two people called Budi Santoso is
// the normal case in a member list of any size, and awarding one of them the
// other's race result is worse than leaving the event unmatched for somebody
// to look at.
func MatchSubject(subject string, candidates []MatchCandidate) (string, bool) {
	needle := strings.ToLower(strings.TrimSpace(subject))
	if needle == "" {
		return "", false
	}

	for _, candidate := range candidates {
		if candidate.Email != "" && strings.EqualFold(candidate.Email, needle) {
			return candidate.MemberID, true
		}
	}

	digits := onlyDigits(needle)
	if len(digits) >= 8 {
		for _, candidate := range candidates {
			// Compared on the last eight digits, so +6281... and 081... are
			// the same phone — which they are, and a country code somebody
			// typed differently should not lose a member their points.
			if tail(onlyDigits(candidate.Phone), 8) == tail(digits, 8) {
				return candidate.MemberID, true
			}
		}
	}
	return "", false
}

func onlyDigits(value string) string {
	var b strings.Builder
	for _, c := range value {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func tail(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[len(value)-n:]
}

// EventOutcome is what should happen to a matched event.
type EventOutcome struct {
	Status ExternalEventStatus
	// AwardXP is whether this event should earn the member points. It is false
	// for a partner we merely record, which is the difference between trusting
	// somebody's word and paying out on it.
	AwardXP bool
}

// DecideEvent works out what to do with a claim.
func DecideEvent(partner IntegrationPartner, matched bool) EventOutcome {
	if !matched {
		return EventOutcome{Status: EventUnmatched}
	}
	if !partner.Active {
		return EventOutcome{Status: EventIgnored}
	}
	return EventOutcome{Status: EventProcessed, AwardXP: partner.AwardsXP}
}
