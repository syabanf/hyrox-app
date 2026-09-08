package domain

import (
	"testing"
	"time"
)

// jakarta is the studio's timezone; shift times are wall-clock in it.
var jakarta = time.FixedZone("WIB", 7*60*60)

func mustDate(t *testing.T, value string) Date {
	t.Helper()
	date, err := ParseDate(value)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", value, err)
	}
	return date
}

func mustTimeOfDay(t *testing.T, value string) TimeOfDay {
	t.Helper()
	parsed, err := ParseTimeOfDay(value)
	if err != nil {
		t.Fatalf("ParseTimeOfDay(%q): %v", value, err)
	}
	return parsed
}

func TestDateArithmeticStaysOnCalendarDays(t *testing.T) {
	date := mustDate(t, "2026-02-28")

	if got := date.AddDays(1); got != Date("2026-03-01") {
		t.Fatalf("2026-02-28 + 1 day = %s, want 2026-03-01", got)
	}
	if got := date.AddDays(-28); got != Date("2026-01-31") {
		t.Fatalf("going back a month = %s, want 2026-01-31", got)
	}
	// 2028 is a leap year, so February has a 29th.
	if got := mustDate(t, "2028-02-28").AddDays(1); got != Date("2028-02-29") {
		t.Fatalf("leap day = %s, want 2028-02-29", got)
	}
	if _, err := ParseDate("28/02/2026"); err == nil {
		t.Fatal("a non-ISO date must be rejected")
	}
}

func TestISODayOfWeekPutsMondayFirst(t *testing.T) {
	cases := map[string]int{
		"2026-09-07": 1, // Monday
		"2026-09-12": 6, // Saturday
		"2026-09-13": 7, // Sunday
	}
	for value, want := range cases {
		if got := mustDate(t, value).ISODayOfWeek(); got != want {
			t.Fatalf("%s is day %d, want %d", value, got, want)
		}
	}
	if !mustDate(t, "2026-09-13").IsWeekend() {
		t.Fatal("Sunday should be a weekend")
	}
	if mustDate(t, "2026-09-07").IsWeekend() {
		t.Fatal("Monday should not be a weekend")
	}
}

func morningShift() Shift {
	return Shift{
		ID:                   "shf_morning",
		Name:                 "Shift Pagi",
		StartTime:            TimeOfDay{Hour: 8},
		EndTime:              TimeOfDay{Hour: 16},
		BreakMinutes:         60,
		LateToleranceMinutes: 10,
		Active:               true,
	}
}

func TestComputeLatenessCountsFromShiftStartNotFromTheGrace(t *testing.T) {
	date := mustDate(t, "2026-09-07")
	shift := morningShift()

	onTime := time.Date(2026, 9, 7, 8, 0, 0, 0, jakarta)
	if got := ComputeLateness(onTime, date, shift, jakarta); got.IsLate {
		t.Fatalf("arriving exactly on time reads as late: %+v", got)
	}

	// Inside the ten-minute tolerance: not late at all.
	withinGrace := time.Date(2026, 9, 7, 8, 10, 0, 0, jakarta)
	if got := ComputeLateness(withinGrace, date, shift, jakarta); got.IsLate {
		t.Fatalf("arriving on the tolerance boundary reads as late: %+v", got)
	}

	// Past it: late, and counted from 08:00, not from 08:10. This is the
	// detail that quietly under-reports every lateness if it is got wrong.
	past := time.Date(2026, 9, 7, 8, 12, 0, 0, jakarta)
	got := ComputeLateness(past, date, shift, jakarta)
	if !got.IsLate {
		t.Fatal("arriving after the tolerance should be late")
	}
	if got.LateMinutes != 12 {
		t.Fatalf("late by %d minutes, want 12 measured from the shift start", got.LateMinutes)
	}
}

func TestScheduledWindowRollsOvernightShiftsToTheNextDay(t *testing.T) {
	night := Shift{
		StartTime:   TimeOfDay{Hour: 22},
		EndTime:     TimeOfDay{Hour: 6},
		IsOvernight: true,
	}
	start, end := ScheduledWindow(mustDate(t, "2026-09-07"), night, jakarta)

	if start.Day() != 7 || start.Hour() != 22 {
		t.Fatalf("start = %s, want the 7th at 22:00", start)
	}
	if end.Day() != 8 || end.Hour() != 6 {
		t.Fatalf("end = %s, want the 8th at 06:00", end)
	}
	if !end.After(start) {
		t.Fatal("an overnight shift must end after it starts")
	}
}

func TestWorkHoursSubtractTheBreakAndNeverGoNegative(t *testing.T) {
	in := time.Date(2026, 9, 7, 8, 0, 0, 0, jakarta)
	out := time.Date(2026, 9, 7, 16, 30, 0, 0, jakarta)

	if got := WorkHours(in, out, 60); got != 7.5 {
		t.Fatalf("work hours = %v, want 7.5", got)
	}
	// A break longer than the shift cannot produce negative pay.
	if got := WorkHours(in, out, 600); got != 0 {
		t.Fatalf("work hours = %v, want 0", got)
	}
}

func schedule() []EmployeeShift {
	morning := "shf_morning"
	evening := "shf_evening"
	return []EmployeeShift{
		// The original pattern: mornings on Monday, resting Sunday.
		{DayOfWeek: 1, ShiftID: &morning, EffectiveFrom: "2026-01-01"},
		{DayOfWeek: 7, ShiftID: nil, EffectiveFrom: "2026-01-01"},
		// From September, Mondays move to the evening shift.
		{DayOfWeek: 1, ShiftID: &evening, EffectiveFrom: "2026-09-01"},
	}
}

func TestResolveShiftTakesTheMostRecentPattern(t *testing.T) {
	rows := schedule()

	before, ok := ResolveWorkingShift(rows, mustDate(t, "2026-08-31")) // a Monday
	if !ok || *before.ShiftID != "shf_morning" {
		t.Fatalf("August Monday resolved to %+v, want the morning shift", before)
	}

	after, ok := ResolveWorkingShift(rows, mustDate(t, "2026-09-07")) // a Monday
	if !ok || *after.ShiftID != "shf_evening" {
		t.Fatalf("September Monday resolved to %+v, want the evening shift", after)
	}
}

func TestRestDaysAreDistinctFromNoSchedule(t *testing.T) {
	rows := schedule()
	sunday := mustDate(t, "2026-09-13")

	// A rest day HAS a pattern row, it just has no shift on it.
	row, ok := ResolveScheduleRow(rows, sunday)
	if !ok {
		t.Fatal("Sunday should resolve to a pattern row")
	}
	if row.ShiftID != nil {
		t.Fatal("Sunday should be a rest day")
	}
	// The lateness path must not see a rest day as a working day.
	if _, working := ResolveWorkingShift(rows, sunday); working {
		t.Fatal("a rest day must not resolve to a working shift")
	}
	// A weekday with no pattern at all resolves to nothing.
	if _, ok := ResolveScheduleRow(rows, mustDate(t, "2026-09-09")); ok {
		t.Fatal("Wednesday has no pattern and should resolve to nothing")
	}
}

func holidayIndex() HolidayIndex {
	return IndexHolidays([]Holiday{
		// Independence Day: a public holiday, so it costs nobody a leave day.
		{Date: "2026-08-17", Name: "Hari Kemerdekaan", Type: HolidayNational, DeductsLeave: false},
		// Collective leave: a day off that IS drawn from the annual allowance.
		{Date: "2026-08-18", Name: "Cuti Bersama", Type: HolidayCollective, DeductsLeave: true},
	})
}

func TestLeaveDayCountingSkipsWeekendsAndFreeHolidays(t *testing.T) {
	index := holidayIndex()

	// Mon 17 Aug (holiday, free) .. Fri 21 Aug.
	got := DescribeLeaveDays(mustDate(t, "2026-08-17"), mustDate(t, "2026-08-21"), index)
	// 17th is exempt; 18th is collective leave and still deducts; 19-21 count.
	if got.TotalDays != 4 {
		t.Fatalf("total days = %v, want 4", got.TotalDays)
	}
	if len(got.ExcludedHolidays) != 1 || got.ExcludedHolidays[0].Name != "Hari Kemerdekaan" {
		t.Fatalf("excluded = %+v, want only Independence Day", got.ExcludedHolidays)
	}

	// A range across a weekend loses the Saturday and Sunday.
	weekend := CountLeaveDays(mustDate(t, "2026-09-11"), mustDate(t, "2026-09-14"), index)
	if weekend != 2 { // Friday and Monday
		t.Fatalf("across a weekend = %v, want 2", weekend)
	}

	// A request that lands entirely on days off costs nothing.
	if got := CountLeaveDays(mustDate(t, "2026-09-12"), mustDate(t, "2026-09-13"), index); got != 0 {
		t.Fatalf("a weekend-only request = %v, want 0", got)
	}
}

func TestNationalHolidayWinsOverCollectiveLeaveOnTheSameDate(t *testing.T) {
	// Both land on the same day. The employee should not lose the allowance.
	index := IndexHolidays([]Holiday{
		{Date: "2026-08-17", Name: "Cuti Bersama", Type: HolidayCollective, DeductsLeave: true},
		{Date: "2026-08-17", Name: "Hari Kemerdekaan", Type: HolidayNational, DeductsLeave: false},
	})
	if got := CountLeaveDays(mustDate(t, "2026-08-17"), mustDate(t, "2026-08-17"), index); got != 0 {
		t.Fatalf("total days = %v, want 0: the public holiday should win", got)
	}
}

func testEmployee() Employee {
	return Employee{
		ID: "emp_1", FullName: "Kevin Hartono",
		JoinDate: "2025-01-01", Active: true,
	}
}

func fullBalance() LeaveBalance {
	return LeaveBalance{EmployeeID: "emp_1", Year: 2026, AnnualTotal: 12}
}

func TestEvaluateLeaveAcceptsAValidRequest(t *testing.T) {
	decision := EvaluateLeave(LeaveRequest{
		Employee: testEmployee(), Type: LeaveAnnual,
		StartDate: "2026-09-07", EndDate: "2026-09-09",
		Balance: fullBalance(), Holidays: holidayIndex(),
	})
	if !decision.Allowed() {
		t.Fatalf("rejected with %s", decision.Rejection)
	}
	if decision.Breakdown.TotalDays != 3 {
		t.Fatalf("cost = %v days, want 3", decision.Breakdown.TotalDays)
	}
}

func TestEvaluateLeaveRejectionMatrix(t *testing.T) {
	spent := fullBalance()
	spent.AnnualUsed = 11

	cases := []struct {
		name    string
		request LeaveRequest
		want    LeaveRejection
	}{
		{
			name: "end before start",
			request: LeaveRequest{
				Employee: testEmployee(), Type: LeaveAnnual,
				StartDate: "2026-09-09", EndDate: "2026-09-07",
				Balance: fullBalance(), Holidays: holidayIndex(),
			},
			want: LeaveRejectDatesInverted,
		},
		{
			name: "entirely on days off",
			request: LeaveRequest{
				Employee: testEmployee(), Type: LeaveAnnual,
				StartDate: "2026-09-12", EndDate: "2026-09-13",
				Balance: fullBalance(), Holidays: holidayIndex(),
			},
			want: LeaveRejectNoWorkingDays,
		},
		{
			name: "not enough allowance left",
			request: LeaveRequest{
				Employee: testEmployee(), Type: LeaveAnnual,
				StartDate: "2026-09-07", EndDate: "2026-09-09",
				Balance: spent, Holidays: holidayIndex(),
			},
			want: LeaveRejectInsufficient,
		},
		{
			name: "before they were hired",
			request: LeaveRequest{
				Employee: Employee{ID: "emp_1", JoinDate: "2026-10-01"}, Type: LeaveAnnual,
				StartDate: "2026-09-07", EndDate: "2026-09-09",
				Balance: fullBalance(), Holidays: holidayIndex(),
			},
			want: LeaveRejectNotEmployed,
		},
		{
			name: "overlapping an existing request",
			request: LeaveRequest{
				Employee: testEmployee(), Type: LeaveAnnual,
				StartDate: "2026-09-07", EndDate: "2026-09-09",
				Balance: fullBalance(), Holidays: holidayIndex(),
				Existing: []Leave{{
					StartDate: "2026-09-08", EndDate: "2026-09-10", Status: LeaveApproved,
				}},
			},
			want: LeaveRejectOverlaps,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateLeave(tc.request)
			if got.Rejection != tc.want {
				t.Fatalf("rejection = %q, want %q", got.Rejection, tc.want)
			}
		})
	}
}

func TestSickLeaveIsNotRationedAgainstTheAnnualAllowance(t *testing.T) {
	// Somebody who is ill is ill; only annual leave has an allowance to spend.
	spent := fullBalance()
	spent.AnnualUsed = 12

	decision := EvaluateLeave(LeaveRequest{
		Employee: testEmployee(), Type: LeaveSick,
		StartDate: "2026-09-07", EndDate: "2026-09-09",
		Balance: spent, Holidays: holidayIndex(),
	})
	if !decision.Allowed() {
		t.Fatalf("sick leave refused with %s even though it is not rationed", decision.Rejection)
	}
}

func TestCancelledLeaveDoesNotBlockANewRequest(t *testing.T) {
	decision := EvaluateLeave(LeaveRequest{
		Employee: testEmployee(), Type: LeaveAnnual,
		StartDate: "2026-09-07", EndDate: "2026-09-09",
		Balance: fullBalance(), Holidays: holidayIndex(),
		Existing: []Leave{{
			StartDate: "2026-09-08", EndDate: "2026-09-10", Status: LeaveCancelled,
		}},
	})
	if !decision.Allowed() {
		t.Fatalf("a cancelled request blocked a new one: %s", decision.Rejection)
	}
}

func TestLeaveTransitionsMakeADecisionFinal(t *testing.T) {
	if _, err := Transition(LeaveTransitions, LeavePending, LeaveApproved); err != nil {
		t.Fatalf("PENDING -> APPROVED should be legal: %v", err)
	}
	if _, err := Transition(LeaveTransitions, LeaveRejected, LeaveApproved); err == nil {
		t.Fatal("a rejected request must not be approved later")
	}
	// Approved leave can still be cancelled, which is how the balance is
	// handed back.
	if _, err := Transition(LeaveTransitions, LeaveApproved, LeaveCancelled); err != nil {
		t.Fatalf("APPROVED -> CANCELLED should be legal: %v", err)
	}
}

func TestOvertimeHoursRollPastMidnight(t *testing.T) {
	if got := OvertimeHours(mustTimeOfDay(t, "17:00"), mustTimeOfDay(t, "19:30")); got != 2.5 {
		t.Fatalf("evening overtime = %v, want 2.5", got)
	}
	// A late finish that crosses into the next day is still one span.
	if got := OvertimeHours(mustTimeOfDay(t, "22:00"), mustTimeOfDay(t, "01:00")); got != 3 {
		t.Fatalf("overnight overtime = %v, want 3", got)
	}
}

func TestEmployedOnBoundsTheEmploymentPeriod(t *testing.T) {
	leaver := testEmployee()
	end := Date("2026-06-30")
	leaver.EndDate = &end

	if !leaver.IsEmployedOn("2026-06-30") {
		t.Fatal("the last day of employment counts as employed")
	}
	if leaver.IsEmployedOn("2026-07-01") {
		t.Fatal("the day after leaving does not count")
	}
	if leaver.IsEmployedOn("2024-12-31") {
		t.Fatal("before joining does not count")
	}
}
