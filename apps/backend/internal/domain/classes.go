package domain

import "time"

// ClassType is the reusable template; sessions are the scheduled instances.
// Keeping them separate means editing a template never rewrites history.
type ClassType struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	DefaultDurationMin int    `json:"defaultDurationMin"`
	DefaultCreditCost  int    `json:"defaultCreditCost"`
	DefaultCapacity    int    `json:"defaultCapacity"`
	Active             bool   `json:"active"`
}

// SessionStatus is the lifecycle of one scheduled class.
type SessionStatus string

const (
	SessionDraft     SessionStatus = "DRAFT"
	SessionPublished SessionStatus = "PUBLISHED"
	SessionFull      SessionStatus = "FULL"
	SessionCompleted SessionStatus = "COMPLETED"
	SessionCancelled SessionStatus = "CANCELLED"
)

// SessionTransitions allows FULL to fall back to PUBLISHED when a cancellation
// frees a slot, which is why the pair is bidirectional.
var SessionTransitions = TransitionMap[SessionStatus]{
	SessionDraft:     {SessionPublished, SessionCancelled},
	SessionPublished: {SessionFull, SessionCompleted, SessionCancelled},
	SessionFull:      {SessionPublished, SessionCompleted, SessionCancelled},
	SessionCompleted: {},
	SessionCancelled: {},
}

// ClassSession is one class at one time in one branch. Capacity and credit
// cost are copied from the class type at creation so later template edits do
// not change what an already-booked member agreed to.
type ClassSession struct {
	ID              string        `json:"id"`
	ClassTypeID     string        `json:"classTypeId"`
	BranchID        string        `json:"branchId"`
	CoachID         string        `json:"coachId"`
	StartsAt        time.Time     `json:"startsAt"`
	EndsAt          time.Time     `json:"endsAt"`
	Capacity        int           `json:"capacity"`
	CreditCost      int           `json:"creditCost"`
	BookingOpensAt  time.Time     `json:"bookingOpensAt"`
	BookingClosesAt time.Time     `json:"bookingClosesAt"`
	Status          SessionStatus `json:"status"`
	Area            *string       `json:"area"`
}

// IsBookable reports whether the session accepts bookings or waitlist entries.
// FULL still counts: it is where the waitlist forms.
func (s ClassSession) IsBookable() bool {
	return s.Status == SessionPublished || s.Status == SessionFull
}

// DeriveBookingWindow computes when booking opens and closes for a session
// start, from the resolved business rules.
func DeriveBookingWindow(startsAt time.Time, rules BusinessRules) (opens, closes time.Time) {
	opens = startsAt.AddDate(0, 0, -rules.BookingOpensDaysBefore)
	closes = startsAt.Add(-time.Duration(rules.BookingClosesMinutesBefore) * time.Minute)
	return opens, closes
}
