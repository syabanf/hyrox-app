package domain

import (
	"sort"
	"time"
)

// BookingStatus is the lifecycle of one member's place in one session.
type BookingStatus string

const (
	BookingPending   BookingStatus = "PENDING"
	BookingConfirmed BookingStatus = "CONFIRMED"
	BookingWaitlist  BookingStatus = "WAITLIST"
	BookingCancelled BookingStatus = "CANCELLED"
	BookingCheckedIn BookingStatus = "CHECKED_IN"
	BookingCompleted BookingStatus = "COMPLETED"
	BookingNoShow    BookingStatus = "NO_SHOW"
)

var BookingTransitions = TransitionMap[BookingStatus]{
	BookingPending:   {BookingConfirmed, BookingWaitlist, BookingCancelled},
	BookingConfirmed: {BookingCancelled, BookingCheckedIn, BookingNoShow},
	BookingWaitlist:  {BookingConfirmed, BookingCancelled},
	BookingCheckedIn: {BookingCompleted},
	BookingCancelled: {},
	BookingCompleted: {},
	BookingNoShow:    {},
}

// ActiveBookingStatuses are the states that hold a slot or a waitlist place.
// A member may hold only one of these per session at a time.
var ActiveBookingStatuses = []BookingStatus{
	BookingPending, BookingConfirmed, BookingWaitlist, BookingCheckedIn,
}

// IsActive reports whether this booking still occupies a place.
func (b Booking) IsActive() bool { return contains(ActiveBookingStatuses, b.Status) }

// BookingSource records whether the member booked themselves or the front desk
// did it for them.
type BookingSource string

const (
	BookingSourceMember BookingSource = "MEMBER"
	BookingSourceAdmin  BookingSource = "ADMIN"
)

// Booking is a member's place in a session.
type Booking struct {
	ID               string        `json:"id"`
	MemberID         string        `json:"memberId"`
	SessionID        string        `json:"sessionId"`
	Status           BookingStatus `json:"status"`
	WaitlistPosition *int          `json:"waitlistPosition"`
	Source           BookingSource `json:"source"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
	CancelledAt      *time.Time    `json:"cancelledAt"`
	CheckedInAt      *time.Time    `json:"checkedInAt"`
	// PromotionOfferedAt is set when a freed slot was offered to this
	// waitlisted member and is waiting for them to confirm.
	PromotionOfferedAt *time.Time `json:"promotionOfferedAt"`
}

// BookingDenialReason explains a refused booking. These strings are API error
// codes: the member app maps each to its own message.
type BookingDenialReason string

const (
	DenyMemberNotActive     BookingDenialReason = "MEMBER_NOT_ACTIVE"
	DenySessionNotBookable  BookingDenialReason = "SESSION_NOT_BOOKABLE"
	DenyBookingNotOpenYet   BookingDenialReason = "BOOKING_NOT_OPEN_YET"
	DenyBookingWindowClosed BookingDenialReason = "BOOKING_WINDOW_CLOSED"
	DenyAlreadyBooked       BookingDenialReason = "ALREADY_BOOKED"
	DenyInsufficientCredits BookingDenialReason = "INSUFFICIENT_CREDITS"
	DenyPackageNotCovered   BookingDenialReason = "PACKAGE_NOT_COVERED"
)

// BookingDecisionKind is the outcome of the eligibility pipeline.
type BookingDecisionKind string

const (
	DecisionConfirm  BookingDecisionKind = "CONFIRM"
	DecisionWaitlist BookingDecisionKind = "WAITLIST"
	DecisionDeny     BookingDecisionKind = "DENY"
)

// BookingDecision carries the outcome plus whichever detail it implies: a
// waitlist position, or the reason for refusal.
type BookingDecision struct {
	Kind     BookingDecisionKind
	Position int
	Reason   BookingDenialReason
}

// BookingRequest is everything the eligibility rules need. Gathering it is the
// caller's job, which keeps this function pure and exhaustively testable.
type BookingRequest struct {
	Member                   Member
	Session                  ClassSession
	Balance                  int
	ConfirmedCount           int
	WaitlistCount            int
	HasExistingActiveBooking bool
	Now                      time.Time
}

// EvaluateBooking decides whether a member may take a slot.
//
// Credits are checked but NOT deducted here: deduction happens at the gate on
// check-in. Booking only proves the member could pay and holds the place.
func EvaluateBooking(req BookingRequest) BookingDecision {
	deny := func(reason BookingDenialReason) BookingDecision {
		return BookingDecision{Kind: DecisionDeny, Reason: reason}
	}
	switch {
	case !req.Member.IsActive():
		return deny(DenyMemberNotActive)
	case !req.Session.IsBookable():
		return deny(DenySessionNotBookable)
	case req.Now.Before(req.Session.BookingOpensAt):
		return deny(DenyBookingNotOpenYet)
	case req.Now.After(req.Session.BookingClosesAt):
		return deny(DenyBookingWindowClosed)
	case req.HasExistingActiveBooking:
		return deny(DenyAlreadyBooked)
	case req.Balance < req.Session.CreditCost:
		return deny(DenyInsufficientCredits)
	case req.ConfirmedCount >= req.Session.Capacity:
		return BookingDecision{Kind: DecisionWaitlist, Position: req.WaitlistCount + 1}
	default:
		return BookingDecision{Kind: DecisionConfirm}
	}
}

// CancellationKind distinguishes a cancellation inside the deadline from one
// after it.
type CancellationKind string

const (
	CancelReleased CancellationKind = "RELEASED"
	CancelLate     CancellationKind = "LATE"
)

// CancellationOutcome reports what the member's cancellation costs them.
type CancellationOutcome struct {
	Kind           CancellationKind
	Deadline       time.Time
	PenaltyCredits int
}

// CancellationDeadline is the last moment a member can cancel for free.
func CancellationDeadline(session ClassSession, rules BusinessRules) time.Time {
	return session.StartsAt.Add(-time.Duration(rules.CancellationDeadlineHours) * time.Hour)
}

// EvaluateCancellation applies the late-cancellation policy.
func EvaluateCancellation(session ClassSession, rules BusinessRules, now time.Time) CancellationOutcome {
	deadline := CancellationDeadline(session, rules)
	if now.After(deadline) {
		penalty := 0
		if rules.LateCancellationPolicy == PolicyForfeit {
			penalty = session.CreditCost
		}
		return CancellationOutcome{Kind: CancelLate, Deadline: deadline, PenaltyCredits: penalty}
	}
	return CancellationOutcome{Kind: CancelReleased, Deadline: deadline}
}

// NoShowPenalty is what a missed class costs under the current policy.
func NoShowPenalty(session ClassSession, rules BusinessRules) int {
	if rules.NoShowPolicy == PolicyForfeit {
		return session.CreditCost
	}
	return 0
}

// PickWaitlistPromotion chooses who gets a freed slot: lowest waitlist
// position first, ties broken by who joined the waitlist earlier.
func PickWaitlistPromotion(waitlist []Booking) *Booking {
	candidates := make([]Booking, 0, len(waitlist))
	for _, b := range waitlist {
		if b.Status == BookingWaitlist {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		pi, pj := positionOf(candidates[i]), positionOf(candidates[j])
		if pi != pj {
			return pi < pj
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	winner := candidates[0]
	return &winner
}

// NextWaitlistPosition is the position the next joiner receives.
func NextWaitlistPosition(waitlist []Booking) int {
	highest := 0
	for _, b := range waitlist {
		if b.Status != BookingWaitlist || b.WaitlistPosition == nil {
			continue
		}
		if *b.WaitlistPosition > highest {
			highest = *b.WaitlistPosition
		}
	}
	return highest + 1
}

// positionOf treats an unset position as last in line.
func positionOf(b Booking) int {
	if b.WaitlistPosition == nil {
		return int(^uint(0) >> 1)
	}
	return *b.WaitlistPosition
}
