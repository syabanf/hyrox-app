package domain

import (
	"fmt"
	"sort"
	"time"
)

// IncentiveScheme is how a coach is paid in IDR for the classes they deliver.
// It is payroll, entirely separate from the member credit ledger.
//
// A row with CoachID nil is the organization default; a coach-specific row
// overrides it.
type IncentiveScheme struct {
	ID      string  `json:"id"`
	CoachID *string `json:"coachId"`
	// SessionFeeIDR is the flat fee for delivering the session at all.
	SessionFeeIDR int64 `json:"sessionFeeIdr"`
	// PerAttendeeIDR is paid for each member who actually checked in.
	PerAttendeeIDR int64 `json:"perAttendeeIdr"`
	// FullClassBonusIDR is paid when attendance reaches the threshold.
	FullClassBonusIDR         int64 `json:"fullClassBonusIdr"`
	FullClassThresholdPercent int   `json:"fullClassThresholdPercent"`
	// NoShowPenaltyIDR is deducted per no-show; a line never goes below zero.
	NoShowPenaltyIDR int64 `json:"noShowPenaltyIdr"`
	// Rates override the session fee and per-attendee rate for named class
	// types. A class type without a rate is paid at the scheme's own figures.
	Rates     []SchemeRate `json:"rates"`
	Active    bool         `json:"active"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// SchemeRate is what one class type pays under a scheme.
//
// The two figures that vary by class are the ones here: a longer, harder class
// is worth more to deliver, and a class where every attendee needs watching is
// worth more per head. The bonus, its threshold and the no-show penalty stay
// with the scheme — they are studio policy, not per-class pricing.
type SchemeRate struct {
	ClassTypeID    string `json:"classTypeId"`
	SessionFeeIDR  int64  `json:"sessionFeeIdr"`
	PerAttendeeIDR int64  `json:"perAttendeeIdr"`
}

// RateFor is what a scheme pays for one class type: its own rate if there is
// one, otherwise the scheme's figures.
func RateFor(scheme IncentiveScheme, classTypeID string) SchemeRate {
	for _, rate := range scheme.Rates {
		if rate.ClassTypeID == classTypeID {
			return rate
		}
	}
	return SchemeRate{
		ClassTypeID:    classTypeID,
		SessionFeeIDR:  scheme.SessionFeeIDR,
		PerAttendeeIDR: scheme.PerAttendeeIDR,
	}
}

// ResolveScheme picks the coach's own scheme when it exists and is active,
// otherwise the organization default.
func ResolveScheme(defaultScheme IncentiveScheme, coachOverride *IncentiveScheme) IncentiveScheme {
	if coachOverride != nil && coachOverride.Active {
		return *coachOverride
	}
	return defaultScheme
}

// StatementPeriod is a half-open interval [Start, End).
type StatementPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MonthPeriod returns the calendar-month bounds for "YYYY-MM" in the studio's
// location, which is how sessions are scheduled and how payroll is counted.
func MonthPeriod(periodMonth string, loc *time.Location) (StatementPeriod, error) {
	parsed, err := time.ParseInLocation("2006-01", periodMonth, loc)
	if err != nil {
		return StatementPeriod{}, fmt.Errorf("invalid period month %q: %w", periodMonth, err)
	}
	return StatementPeriod{Start: parsed, End: parsed.AddDate(0, 1, 0)}, nil
}

// PeriodMonthOf formats a timestamp as the "YYYY-MM" it belongs to.
func PeriodMonthOf(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01")
}

// CoachStatementLine is one delivered session and what it earned.
type CoachStatementLine struct {
	SessionID     string    `json:"sessionId"`
	StartsAt      time.Time `json:"startsAt"`
	ClassTypeName string    `json:"classTypeName"`
	Capacity      int       `json:"capacity"`
	// Booked counts everyone who held a slot, attended or not.
	Booked int `json:"booked"`
	// Attended counts those who actually came through the gate.
	Attended      int   `json:"attended"`
	NoShows       int   `json:"noShows"`
	SessionFeeIDR int64 `json:"sessionFeeIdr"`
	AttendeeIDR   int64 `json:"attendeeIdr"`
	BonusIDR      int64 `json:"bonusIdr"`
	PenaltyIDR    int64 `json:"penaltyIdr"`
	TotalIDR      int64 `json:"totalIdr"`
}

// CoachStatementTotals sums the lines.
type CoachStatementTotals struct {
	Sessions      int   `json:"sessions"`
	Attended      int   `json:"attended"`
	NoShows       int   `json:"noShows"`
	SessionFeeIDR int64 `json:"sessionFeeIdr"`
	AttendeeIDR   int64 `json:"attendeeIdr"`
	BonusIDR      int64 `json:"bonusIdr"`
	PenaltyIDR    int64 `json:"penaltyIdr"`
	TotalIDR      int64 `json:"totalIdr"`
}

// CoachStatement is one coach's earnings over one period.
type CoachStatement struct {
	CoachID string               `json:"coachId"`
	Lines   []CoachStatementLine `json:"lines"`
	Totals  CoachStatementTotals `json:"totals"`
}

// heldSlotStatuses are bookings that occupied a place, however it ended.
var heldSlotStatuses = []BookingStatus{
	BookingConfirmed, BookingCheckedIn, BookingCompleted, BookingNoShow,
}

// attendedStatuses are bookings where the member actually turned up.
var attendedStatuses = []BookingStatus{BookingCheckedIn, BookingCompleted}

// StatementInput is the data one statement is computed from.
type StatementInput struct {
	CoachID           string
	Sessions          []ClassSession
	BookingsBySession map[string][]Booking
	ClassTypeNames    map[string]string
	Scheme            IncentiveScheme
	Period            StatementPeriod
}

// ComputeCoachStatement is the payroll math for one coach over one period.
//
//	line = rate.sessionFee + attended x rate.perAttendee
//	     + (attended >= threshold% of capacity ? fullClassBonus : 0)
//	     - noShows x noShowPenalty          -> clamped at 0
//
// Only COMPLETED sessions count, so a class is paid once it has actually been
// delivered and attendance is final.
func ComputeCoachStatement(in StatementInput) CoachStatement {
	eligible := make([]ClassSession, 0, len(in.Sessions))
	for _, s := range in.Sessions {
		if s.CoachID != in.CoachID || s.Status != SessionCompleted {
			continue
		}
		if s.StartsAt.Before(in.Period.Start) || !s.StartsAt.Before(in.Period.End) {
			continue
		}
		eligible = append(eligible, s)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].StartsAt.Before(eligible[j].StartsAt)
	})

	lines := make([]CoachStatementLine, 0, len(eligible))
	totals := CoachStatementTotals{}
	for _, session := range eligible {
		bookings := in.BookingsBySession[session.ID]
		booked, attended, noShows := 0, 0, 0
		for _, b := range bookings {
			if contains(heldSlotStatuses, b.Status) {
				booked++
			}
			if contains(attendedStatuses, b.Status) {
				attended++
			}
			if b.Status == BookingNoShow {
				noShows++
			}
		}

		// What this class pays: the coach's rate for this class type, or the
		// scheme's own figures when it has none.
		rate := RateFor(in.Scheme, session.ClassTypeID)
		attendeeIDR := int64(attended) * rate.PerAttendeeIDR
		bonusIDR := int64(0)
		if session.Capacity > 0 {
			fillPercent := float64(attended) / float64(session.Capacity) * 100
			if fillPercent >= float64(in.Scheme.FullClassThresholdPercent) {
				bonusIDR = in.Scheme.FullClassBonusIDR
			}
		}
		penaltyIDR := int64(noShows) * in.Scheme.NoShowPenaltyIDR

		total := rate.SessionFeeIDR + attendeeIDR + bonusIDR - penaltyIDR
		if total < 0 {
			total = 0
		}

		className := in.ClassTypeNames[session.ClassTypeID]
		if className == "" {
			className = "Class"
		}
		line := CoachStatementLine{
			SessionID:     session.ID,
			StartsAt:      session.StartsAt,
			ClassTypeName: className,
			Capacity:      session.Capacity,
			Booked:        booked,
			Attended:      attended,
			NoShows:       noShows,
			SessionFeeIDR: rate.SessionFeeIDR,
			AttendeeIDR:   attendeeIDR,
			BonusIDR:      bonusIDR,
			PenaltyIDR:    penaltyIDR,
			TotalIDR:      total,
		}
		lines = append(lines, line)

		totals.Sessions++
		totals.Attended += attended
		totals.NoShows += noShows
		totals.SessionFeeIDR += line.SessionFeeIDR
		totals.AttendeeIDR += line.AttendeeIDR
		totals.BonusIDR += line.BonusIDR
		totals.PenaltyIDR += line.PenaltyIDR
		totals.TotalIDR += line.TotalIDR
	}

	return CoachStatement{CoachID: in.CoachID, Lines: lines, Totals: totals}
}

// PayoutStatus is the approval chain for paying a statement out.
type PayoutStatus string

const (
	PayoutDraft    PayoutStatus = "DRAFT"
	PayoutApproved PayoutStatus = "APPROVED"
	PayoutPaid     PayoutStatus = "PAID"
	PayoutVoid     PayoutStatus = "VOID"
)

var PayoutTransitions = TransitionMap[PayoutStatus]{
	PayoutDraft:    {PayoutApproved, PayoutVoid},
	PayoutApproved: {PayoutPaid, PayoutVoid},
	PayoutPaid:     {},
	PayoutVoid:     {},
}

// PayoutAction is the admin verb; each maps to exactly one target state, and
// the transition map decides whether it is legal from where the payout is.
type PayoutAction string

const (
	ActionApprove PayoutAction = "approve"
	ActionPay     PayoutAction = "pay"
	ActionVoid    PayoutAction = "void"
)

var PayoutActionTarget = map[PayoutAction]PayoutStatus{
	ActionApprove: PayoutApproved,
	ActionPay:     PayoutPaid,
	ActionVoid:    PayoutVoid,
}

// IncentivePayout freezes a coach's statement for one calendar month.
//
// The frozen copy is the point: later attendance corrections must never
// silently change what was already approved or paid. To fix a mistake, void
// the payout and create a new one.
type IncentivePayout struct {
	ID               string         `json:"id"`
	CoachID          string         `json:"coachId"`
	BranchID         string         `json:"branchId"`
	PeriodStart      time.Time      `json:"periodStart"`
	PeriodEnd        time.Time      `json:"periodEnd"`
	Statement        CoachStatement `json:"statement"`
	Status           PayoutStatus   `json:"status"`
	CreatedBy        string         `json:"createdBy"`
	ApprovedBy       *string        `json:"approvedBy"`
	ApprovedAt       *time.Time     `json:"approvedAt"`
	PaidAt           *time.Time     `json:"paidAt"`
	PaymentReference *string        `json:"paymentReference"`
	Note             *string        `json:"note"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}
