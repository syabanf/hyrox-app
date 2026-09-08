package domain

import (
	"testing"
	"time"
)

func testScheme() IncentiveScheme {
	return IncentiveScheme{
		ID:                        "sch_default",
		SessionFeeIDR:             150_000,
		PerAttendeeIDR:            15_000,
		FullClassBonusIDR:         50_000,
		FullClassThresholdPercent: 80,
		NoShowPenaltyIDR:          5_000,
		Active:                    true,
	}
}

func completedSession(id string, startsAt time.Time, capacity int) ClassSession {
	return ClassSession{
		ID: id, ClassTypeID: "clt_1", BranchID: "brn_1", CoachID: "coa_1",
		StartsAt: startsAt, EndsAt: startsAt.Add(time.Hour),
		Capacity: capacity, CreditCost: 1, Status: SessionCompleted,
	}
}

func bookings(statuses ...BookingStatus) []Booking {
	out := make([]Booking, 0, len(statuses))
	for i, s := range statuses {
		out = append(out, Booking{ID: string(rune('a' + i)), Status: s})
	}
	return out
}

func TestComputeCoachStatementPaysFeeAttendeesAndBonus(t *testing.T) {
	period := StatementPeriod{
		Start: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	session := completedSession("ses_1", time.Date(2026, 6, 10, 7, 0, 0, 0, time.UTC), 10)

	statement := ComputeCoachStatement(StatementInput{
		CoachID:  "coa_1",
		Sessions: []ClassSession{session},
		BookingsBySession: map[string][]Booking{
			// 8 of 10 attended: exactly the 80 percent bonus threshold.
			"ses_1": bookings(
				BookingCheckedIn, BookingCheckedIn, BookingCompleted, BookingCompleted,
				BookingCheckedIn, BookingCheckedIn, BookingCompleted, BookingCheckedIn,
				BookingNoShow, BookingCancelled,
			),
		},
		ClassTypeNames: map[string]string{"clt_1": "HYROX Fundamentals"},
		Scheme:         testScheme(),
		Period:         period,
	})

	if len(statement.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(statement.Lines))
	}
	line := statement.Lines[0]
	if line.Attended != 8 {
		t.Fatalf("attended = %d, want 8", line.Attended)
	}
	if line.Booked != 9 {
		t.Fatalf("booked = %d, want 9 (attended plus the no-show)", line.Booked)
	}
	if line.NoShows != 1 {
		t.Fatalf("no-shows = %d, want 1", line.NoShows)
	}
	if line.BonusIDR != 50_000 {
		t.Fatalf("bonus = %d, want 50000 at exactly the threshold", line.BonusIDR)
	}
	// 150000 fee + 8*15000 attendees + 50000 bonus - 1*5000 penalty
	if line.TotalIDR != 315_000 {
		t.Fatalf("total = %d, want 315000", line.TotalIDR)
	}
	if line.ClassTypeName != "HYROX Fundamentals" {
		t.Fatalf("class name = %s", line.ClassTypeName)
	}
	if statement.Totals.TotalIDR != 315_000 || statement.Totals.Sessions != 1 {
		t.Fatalf("totals = %+v", statement.Totals)
	}
}

func TestComputeCoachStatementWithholdsBonusBelowThreshold(t *testing.T) {
	period := StatementPeriod{
		Start: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	statement := ComputeCoachStatement(StatementInput{
		CoachID:  "coa_1",
		Sessions: []ClassSession{completedSession("ses_1", time.Date(2026, 6, 10, 7, 0, 0, 0, time.UTC), 10)},
		BookingsBySession: map[string][]Booking{
			"ses_1": bookings(BookingCheckedIn, BookingCheckedIn, BookingCheckedIn),
		},
		Scheme: testScheme(),
		Period: period,
	})

	if statement.Lines[0].BonusIDR != 0 {
		t.Fatalf("bonus = %d, want 0 at 30 percent attendance", statement.Lines[0].BonusIDR)
	}
}

func TestComputeCoachStatementNeverPaysANegativeLine(t *testing.T) {
	period := StatementPeriod{
		Start: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	scheme := testScheme()
	scheme.SessionFeeIDR = 10_000
	scheme.NoShowPenaltyIDR = 50_000

	statement := ComputeCoachStatement(StatementInput{
		CoachID:  "coa_1",
		Sessions: []ClassSession{completedSession("ses_1", time.Date(2026, 6, 10, 7, 0, 0, 0, time.UTC), 10)},
		BookingsBySession: map[string][]Booking{
			"ses_1": bookings(BookingNoShow, BookingNoShow, BookingNoShow),
		},
		Scheme: scheme,
		Period: period,
	})

	if statement.Lines[0].TotalIDR != 0 {
		t.Fatalf("total = %d, want 0 (clamped, not negative)", statement.Lines[0].TotalIDR)
	}
}

func TestComputeCoachStatementCountsOnlyDeliveredSessionsInPeriodForThisCoach(t *testing.T) {
	period := StatementPeriod{
		Start: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	inside := completedSession("ses_in", time.Date(2026, 6, 15, 7, 0, 0, 0, time.UTC), 10)
	beforePeriod := completedSession("ses_before", time.Date(2026, 5, 31, 23, 0, 0, 0, time.UTC), 10)
	// The period end is exclusive, so a session at the boundary belongs to July.
	atEnd := completedSession("ses_end", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 10)
	notCompleted := completedSession("ses_open", time.Date(2026, 6, 20, 7, 0, 0, 0, time.UTC), 10)
	notCompleted.Status = SessionPublished
	otherCoach := completedSession("ses_other", time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC), 10)
	otherCoach.CoachID = "coa_2"

	statement := ComputeCoachStatement(StatementInput{
		CoachID:  "coa_1",
		Sessions: []ClassSession{inside, beforePeriod, atEnd, notCompleted, otherCoach},
		Scheme:   testScheme(),
		Period:   period,
	})

	if len(statement.Lines) != 1 || statement.Lines[0].SessionID != "ses_in" {
		t.Fatalf("lines = %+v, want only ses_in", statement.Lines)
	}
}

func TestResolveSchemeFallsBackToTheOrganizationDefault(t *testing.T) {
	def := testScheme()
	coachID := "coa_1"
	override := testScheme()
	override.ID = "sch_coach"
	override.CoachID = &coachID
	override.SessionFeeIDR = 200_000

	if got := ResolveScheme(def, &override); got.ID != "sch_coach" {
		t.Fatalf("active override should win, got %s", got.ID)
	}

	override.Active = false
	if got := ResolveScheme(def, &override); got.ID != "sch_default" {
		t.Fatalf("inactive override should fall back to the default, got %s", got.ID)
	}
	if got := ResolveScheme(def, nil); got.ID != "sch_default" {
		t.Fatalf("missing override should fall back, got %s", got.ID)
	}
}

func TestMonthPeriodIsHalfOpen(t *testing.T) {
	period, err := MonthPeriod("2026-06", time.UTC)
	if err != nil {
		t.Fatalf("MonthPeriod returned %v", err)
	}
	if !period.Start.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("start = %s", period.Start)
	}
	if !period.End.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("end = %s, want the first of the next month", period.End)
	}
	if _, err := MonthPeriod("June 2026", time.UTC); err == nil {
		t.Fatal("a malformed period must be rejected")
	}
	if got := PeriodMonthOf(time.Date(2026, 6, 30, 23, 0, 0, 0, time.UTC), time.UTC); got != "2026-06" {
		t.Fatalf("period month = %s, want 2026-06", got)
	}
}

func TestPayoutTransitionsFollowTheApprovalChain(t *testing.T) {
	if _, err := Transition(PayoutTransitions, PayoutDraft, PayoutPaid); err == nil {
		t.Fatal("a draft payout must be approved before it can be paid")
	}
	if _, err := Transition(PayoutTransitions, PayoutApproved, PayoutPaid); err != nil {
		t.Fatalf("APPROVED -> PAID should be legal: %v", err)
	}
	if _, err := Transition(PayoutTransitions, PayoutPaid, PayoutVoid); err == nil {
		t.Fatal("a paid payout is final and cannot be voided")
	}
	if PayoutActionTarget[ActionApprove] != PayoutApproved {
		t.Fatal("approve must target APPROVED")
	}
}
