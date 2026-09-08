package domain

import (
	"math"
	"sort"
	"time"
)

// ── Organization ─────────────────────────────────────────────────────────────

// Department is a unit of the org chart. Departments nest, so a studio can
// model "Operations > Coaching" without a second table.
type Department struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Code               string    `json:"code"`
	Description        string    `json:"description"`
	ParentDepartmentID *string   `json:"parentDepartmentId"`
	CostCenter         *string   `json:"costCenter"`
	Active             bool      `json:"active"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// Position is a job title at a level, e.g. "Head Coach" at "Supervisor".
type Position struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Level     string    `json:"level"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
}

// EmploymentStatus is the kind of engagement: probation, permanent, contract.
// It is a table rather than an enum because studios add their own.
type EmploymentStatus struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

// Gender and marital status are optional profile facts kept for payroll and
// statutory reporting.
type MaritalStatus string

const (
	MaritalSingle   MaritalStatus = "SINGLE"
	MaritalMarried  MaritalStatus = "MARRIED"
	MaritalDivorced MaritalStatus = "DIVORCED"
	MaritalWidowed  MaritalStatus = "WIDOWED"
)

// Employee is a person on the payroll.
//
// It is deliberately separate from both Member (a customer) and AdminUser (a
// login). A coach is all three at once and they change independently: someone
// can leave the company while their attendance history stays, and a member who
// is hired does not stop being a member.
type Employee struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	// EmployeeNumber is the studio's own identifier, unique and human-quotable.
	EmployeeNumber string         `json:"employeeNumber"`
	Email          string         `json:"email"`
	Phone          string         `json:"phone"`
	BirthDate      *Date          `json:"birthDate"`
	Gender         *Gender        `json:"gender"`
	MaritalStatus  *MaritalStatus `json:"maritalStatus"`
	Address        *string        `json:"address"`
	JoinDate       Date           `json:"joinDate"`
	EndDate        *Date          `json:"endDate"`
	// EmploymentStatusCode points at the configurable status table.
	EmploymentStatusCode string  `json:"employmentStatusCode"`
	Active               bool    `json:"active"`
	DepartmentID         *string `json:"departmentId"`
	PositionID           *string `json:"positionId"`
	BranchID             *string `json:"branchId"`
	// ReportingTo is another employee, which is what makes the org chart.
	ReportingTo *string `json:"reportingTo"`
	// CoachID links this person to the coach they teach as, so attendance and
	// payroll meet the class schedule. Plain id, no cross-schema constraint.
	CoachID *string `json:"coachId"`
	// AdminUserID links them to their staff login, when they have one.
	AdminUserID              *string   `json:"adminUserId"`
	BankName                 *string   `json:"bankName"`
	BankAccount              *string   `json:"bankAccount"`
	EmergencyContactName     *string   `json:"emergencyContactName"`
	EmergencyContactPhone    *string   `json:"emergencyContactPhone"`
	EmergencyContactRelation *string   `json:"emergencyContactRelation"`
	PhotoURL                 *string   `json:"photoUrl"`
	Notes                    *string   `json:"notes"`
	CreatedAt                time.Time `json:"createdAt"`
	UpdatedAt                time.Time `json:"updatedAt"`
}

// IsEmployedOn reports whether the person was on the payroll on a date, which
// is what decides whether they should appear on a roster or a payroll run.
func (e Employee) IsEmployedOn(date Date) bool {
	if date.Before(e.JoinDate) {
		return false
	}
	if e.EndDate != nil && date.After(*e.EndDate) {
		return false
	}
	return true
}

// ── Shifts ───────────────────────────────────────────────────────────────────

// Shift is a named working window. Overnight shifts end on the following day,
// which is why the flag exists rather than inferring it from end < start.
type Shift struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	StartTime            TimeOfDay `json:"startTime"`
	EndTime              TimeOfDay `json:"endTime"`
	BreakMinutes         int       `json:"breakMinutes"`
	LateToleranceMinutes int       `json:"lateToleranceMinutes"`
	IsOvernight          bool      `json:"isOvernight"`
	Active               bool      `json:"active"`
	SortOrder            int       `json:"sortOrder"`
}

// EmployeeShift is one row of a weekly pattern: on this weekday, from this
// date, the employee works this shift.
//
// A row with no ShiftID is a scheduled day OFF, which is different from having
// no pattern at all — the roster needs to tell "resting" from "unscheduled".
type EmployeeShift struct {
	ID            string  `json:"id"`
	EmployeeID    string  `json:"employeeId"`
	DayOfWeek     int     `json:"dayOfWeek"`
	ShiftID       *string `json:"shiftId"`
	EffectiveFrom Date    `json:"effectiveFrom"`
	EffectiveTo   *Date   `json:"effectiveTo"`
}

// ResolveScheduleRow finds the pattern row governing a date: the weekday must
// match, the date must fall inside the effective range, and the most recently
// introduced pattern wins.
//
// It returns rest days as they are, so callers can distinguish them.
func ResolveScheduleRow(rows []EmployeeShift, date Date) (EmployeeShift, bool) {
	dow := date.ISODayOfWeek()

	candidates := make([]EmployeeShift, 0, len(rows))
	for _, row := range rows {
		if row.DayOfWeek != dow {
			continue
		}
		if row.EffectiveFrom.After(date) {
			continue
		}
		if row.EffectiveTo != nil && row.EffectiveTo.Before(date) {
			continue
		}
		candidates = append(candidates, row)
	}
	if len(candidates) == 0 {
		return EmployeeShift{}, false
	}
	// Latest pattern wins, so re-rostering someone is an insert, not an edit.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].EffectiveFrom.After(candidates[j].EffectiveFrom)
	})
	return candidates[0], true
}

// ResolveWorkingShift is ResolveScheduleRow narrowed to days the employee is
// actually expected in, which is what lateness is measured against.
func ResolveWorkingShift(rows []EmployeeShift, date Date) (EmployeeShift, bool) {
	row, ok := ResolveScheduleRow(rows, date)
	if !ok || row.ShiftID == nil {
		return EmployeeShift{}, false
	}
	return row, true
}

// ScheduledWindow is the instant range an employee is expected to work, in the
// studio's timezone. An overnight shift ends the next calendar day.
func ScheduledWindow(date Date, shift Shift, loc *time.Location) (start, end time.Time) {
	start = date.At(shift.StartTime, loc)
	endDate := date
	if shift.IsOvernight {
		endDate = date.AddDays(1)
	}
	return start, endDate.At(shift.EndTime, loc)
}

// Lateness is how late an arrival was against a shift.
type Lateness struct {
	IsLate      bool `json:"isLate"`
	LateMinutes int  `json:"lateMinutes"`
}

// ComputeLateness judges an arrival against the shift.
//
// The tolerance decides WHETHER someone is late; the count is measured from
// the shift's start time, not from the end of the tolerance. Arriving twelve
// minutes into a shift with ten minutes' grace is twelve minutes late, not
// two. That is the convention Indonesian payroll expects, and getting it
// backwards quietly under-reports every lateness in the system.
func ComputeLateness(clockIn time.Time, date Date, shift Shift, loc *time.Location) Lateness {
	start, _ := ScheduledWindow(date, shift, loc)
	grace := time.Duration(shift.LateToleranceMinutes) * time.Minute

	if !clockIn.After(start.Add(grace)) {
		return Lateness{}
	}
	minutes := int(math.Ceil(clockIn.Sub(start).Minutes()))
	return Lateness{IsLate: true, LateMinutes: minutes}
}

// WorkHours is time on the clock minus the unpaid break, never negative.
func WorkHours(clockIn, clockOut time.Time, breakMinutes int) float64 {
	worked := clockOut.Sub(clockIn) - time.Duration(breakMinutes)*time.Minute
	if worked <= 0 {
		return 0
	}
	// Two decimals: payroll reports hours, not nanoseconds.
	return math.Round(worked.Hours()*100) / 100
}

// ── Attendance ───────────────────────────────────────────────────────────────

// AttendanceStatus is how a day resolved.
type AttendanceStatus string

const (
	AttendancePresent AttendanceStatus = "PRESENT"
	AttendanceLate    AttendanceStatus = "LATE"
	AttendanceAbsent  AttendanceStatus = "ABSENT"
	AttendanceHalfDay AttendanceStatus = "HALF_DAY"
	AttendanceRemote  AttendanceStatus = "REMOTE"
	AttendanceLeave   AttendanceStatus = "ON_LEAVE"
	AttendanceHoliday AttendanceStatus = "HOLIDAY"
	AttendanceRest    AttendanceStatus = "REST_DAY"
)

// Attendance is one employee's day: at most one row per employee per date.
type Attendance struct {
	ID         string     `json:"id"`
	EmployeeID string     `json:"employeeId"`
	Date       Date       `json:"date"`
	ClockIn    *time.Time `json:"clockIn"`
	ClockOut   *time.Time `json:"clockOut"`
	ShiftID    *string    `json:"shiftId"`
	// ScheduledStart and ScheduledEnd are frozen at clock-in, so later edits to
	// a shift never rewrite what someone was actually expected to do that day.
	ScheduledStart *time.Time       `json:"scheduledStart"`
	ScheduledEnd   *time.Time       `json:"scheduledEnd"`
	WorkHours      float64          `json:"workHours"`
	BreakMinutes   int              `json:"breakMinutes"`
	Status         AttendanceStatus `json:"status"`
	IsLate         bool             `json:"isLate"`
	LateMinutes    int              `json:"lateMinutes"`
	OvertimeHours  float64          `json:"overtimeHours"`
	Notes          *string          `json:"notes"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// ── Holidays ─────────────────────────────────────────────────────────────────

// HolidayType separates a statutory holiday from a collective leave day and
// from a studio closure.
type HolidayType string

const (
	HolidayNational   HolidayType = "NATIONAL"
	HolidayCollective HolidayType = "COLLECTIVE"
	HolidayCompany    HolidayType = "COMPANY"
)

// Holiday is a non-working day.
//
// DeductsLeave is the whole reason this table exists: Indonesian collective
// leave ("cuti bersama") is drawn from the employee's annual allowance, while
// a national holiday is not. The government moves these dates by decree, so HR
// must be able to correct them without a deploy.
type Holiday struct {
	ID           string      `json:"id"`
	Date         Date        `json:"date"`
	Name         string      `json:"name"`
	Type         HolidayType `json:"type"`
	DeductsLeave bool        `json:"deductsLeave"`
	Note         *string     `json:"note"`
}

// HolidayIndex groups holidays by date for repeated lookups.
type HolidayIndex map[Date][]Holiday

// IndexHolidays builds the lookup. Callers filter out drafts before this.
func IndexHolidays(holidays []Holiday) HolidayIndex {
	index := make(HolidayIndex, len(holidays))
	for _, h := range holidays {
		index[h.Date] = append(index[h.Date], h)
	}
	return index
}

// On returns the holidays falling on a date.
func (h HolidayIndex) On(date Date) []Holiday { return h[date] }

// IsHoliday reports whether anything falls on the date.
func (h HolidayIndex) IsHoliday(date Date) bool { return len(h[date]) > 0 }

// LeaveDaysBreakdown is how many days a request costs, and which dates were
// spared.
type LeaveDaysBreakdown struct {
	TotalDays float64 `json:"totalDays"`
	// ExcludedHolidays explains the difference to the employee: "17 August did
	// not count". Weekends are left out on purpose — they are not named days
	// and nobody is surprised by them.
	ExcludedHolidays []ExcludedHoliday `json:"excludedHolidays"`
}

type ExcludedHoliday struct {
	Date Date   `json:"date"`
	Name string `json:"name"`
}

// DescribeLeaveDays counts the days a leave request actually costs.
//
// A date is free if it carries an active holiday that does not deduct leave,
// or if it falls on a weekend. Where a national holiday and a collective leave
// day land on the same date the national one wins: it genuinely is a public
// holiday, and the reading that favours the employee is also the correct one.
//
// The result can be zero, for a request entirely inside a holiday period. What
// that means is the caller's decision, not this function's.
func DescribeLeaveDays(start, end Date, index HolidayIndex) LeaveDaysBreakdown {
	breakdown := LeaveDaysBreakdown{ExcludedHolidays: []ExcludedHoliday{}}

	for _, date := range DatesBetween(start, end) {
		exempt := false
		for _, holiday := range index.On(date) {
			if !holiday.DeductsLeave {
				breakdown.ExcludedHolidays = append(breakdown.ExcludedHolidays,
					ExcludedHoliday{Date: date, Name: holiday.Name})
				exempt = true
				break
			}
		}
		if exempt || date.IsWeekend() {
			continue
		}
		breakdown.TotalDays++
	}
	return breakdown
}

// CountLeaveDays is DescribeLeaveDays without the explanation.
func CountLeaveDays(start, end Date, index HolidayIndex) float64 {
	return DescribeLeaveDays(start, end, index).TotalDays
}

// ── Leave ────────────────────────────────────────────────────────────────────

// LeaveType is the kind of absence. Each draws on its own counter.
type LeaveType string

const (
	LeaveAnnual     LeaveType = "ANNUAL"
	LeaveSick       LeaveType = "SICK"
	LeaveMaternity  LeaveType = "MATERNITY"
	LeavePaternity  LeaveType = "PATERNITY"
	LeaveUnpaid     LeaveType = "UNPAID"
	LeaveEmergency  LeaveType = "EMERGENCY"
	LeavePilgrimage LeaveType = "PILGRIMAGE"
	LeaveMenstrual  LeaveType = "MENSTRUAL"
)

// IsValidLeaveType validates a type arriving from a request.
func IsValidLeaveType(value string) bool {
	switch LeaveType(value) {
	case LeaveAnnual, LeaveSick, LeaveMaternity, LeavePaternity,
		LeaveUnpaid, LeaveEmergency, LeavePilgrimage, LeaveMenstrual:
		return true
	}
	return false
}

// LeaveStatus is the approval state.
type LeaveStatus string

const (
	LeavePending   LeaveStatus = "PENDING"
	LeaveApproved  LeaveStatus = "APPROVED"
	LeaveRejected  LeaveStatus = "REJECTED"
	LeaveCancelled LeaveStatus = "CANCELLED"
)

// LeaveTransitions: a decided request is final. Changing an approved leave
// means cancelling it and filing again, so the balance ledger stays honest.
var LeaveTransitions = TransitionMap[LeaveStatus]{
	LeavePending:   {LeaveApproved, LeaveRejected, LeaveCancelled},
	LeaveApproved:  {LeaveCancelled},
	LeaveRejected:  {},
	LeaveCancelled: {},
}

// Leave is one absence request.
type Leave struct {
	ID              string      `json:"id"`
	EmployeeID      string      `json:"employeeId"`
	Type            LeaveType   `json:"type"`
	StartDate       Date        `json:"startDate"`
	EndDate         Date        `json:"endDate"`
	TotalDays       float64     `json:"totalDays"`
	Reason          string      `json:"reason"`
	AttachmentURL   *string     `json:"attachmentUrl"`
	Status          LeaveStatus `json:"status"`
	ApprovedBy      *string     `json:"approvedBy"`
	ApprovedAt      *time.Time  `json:"approvedAt"`
	RejectionReason *string     `json:"rejectionReason"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

// LeaveBalance is one employee's allowance for one year.
//
// Only annual leave has an allowance to run out; the other types are counted
// so HR can see usage, not to refuse a request. Somebody who is ill is ill.
type LeaveBalance struct {
	EmployeeID     string  `json:"employeeId"`
	Year           int     `json:"year"`
	AnnualTotal    float64 `json:"annualTotal"`
	AnnualUsed     float64 `json:"annualUsed"`
	SickUsed       float64 `json:"sickUsed"`
	UnpaidUsed     float64 `json:"unpaidUsed"`
	MaternityUsed  float64 `json:"maternityUsed"`
	PaternityUsed  float64 `json:"paternityUsed"`
	EmergencyUsed  float64 `json:"emergencyUsed"`
	PilgrimageUsed float64 `json:"pilgrimageUsed"`
	MenstrualUsed  float64 `json:"menstrualUsed"`
}

// AnnualRemaining is the allowance left to spend.
func (b LeaveBalance) AnnualRemaining() float64 { return b.AnnualTotal - b.AnnualUsed }

// DefaultAnnualLeaveDays is the statutory minimum in Indonesia.
const DefaultAnnualLeaveDays = 12

// LeaveRejection explains why a request cannot be filed.
type LeaveRejection string

const (
	LeaveRejectDatesInverted LeaveRejection = "DATES_INVERTED"
	LeaveRejectNoWorkingDays LeaveRejection = "NO_WORKING_DAYS"
	LeaveRejectOverlaps      LeaveRejection = "OVERLAPS_EXISTING"
	LeaveRejectInsufficient  LeaveRejection = "INSUFFICIENT_BALANCE"
	LeaveRejectNotEmployed   LeaveRejection = "NOT_EMPLOYED"
)

// LeaveRequest is what the rules judge.
type LeaveRequest struct {
	Employee  Employee
	Type      LeaveType
	StartDate Date
	EndDate   Date
	Balance   LeaveBalance
	Holidays  HolidayIndex
	// Existing are the employee's other live requests, used to catch overlaps.
	Existing []Leave
}

// LeaveDecision is the outcome of validating a request.
type LeaveDecision struct {
	Breakdown LeaveDaysBreakdown
	Rejection LeaveRejection
}

// Allowed reports whether the request may be filed.
func (d LeaveDecision) Allowed() bool { return d.Rejection == "" }

// EvaluateLeave decides whether a leave request can be filed, and what it costs.
//
// Only annual leave is checked against a balance: the other types are recorded
// rather than rationed.
func EvaluateLeave(req LeaveRequest) LeaveDecision {
	if req.EndDate.Before(req.StartDate) {
		return LeaveDecision{Rejection: LeaveRejectDatesInverted}
	}
	if !req.Employee.IsEmployedOn(req.StartDate) || !req.Employee.IsEmployedOn(req.EndDate) {
		return LeaveDecision{Rejection: LeaveRejectNotEmployed}
	}

	breakdown := DescribeLeaveDays(req.StartDate, req.EndDate, req.Holidays)
	if breakdown.TotalDays == 0 {
		// Everything in the range was already a day off, so there is nothing
		// to grant.
		return LeaveDecision{Breakdown: breakdown, Rejection: LeaveRejectNoWorkingDays}
	}

	for _, existing := range req.Existing {
		if existing.Status != LeavePending && existing.Status != LeaveApproved {
			continue
		}
		if !existing.StartDate.After(req.EndDate) && !existing.EndDate.Before(req.StartDate) {
			return LeaveDecision{Breakdown: breakdown, Rejection: LeaveRejectOverlaps}
		}
	}

	if req.Type == LeaveAnnual && breakdown.TotalDays > req.Balance.AnnualRemaining() {
		return LeaveDecision{Breakdown: breakdown, Rejection: LeaveRejectInsufficient}
	}
	return LeaveDecision{Breakdown: breakdown}
}

// ── Overtime ─────────────────────────────────────────────────────────────────

// OvertimeSource says who asked for the extra hours, which matters because
// company-directed overtime is payable on different terms from volunteered.
type OvertimeSource string

const (
	OvertimeFromEmployee OvertimeSource = "EMPLOYEE"
	OvertimeFromCompany  OvertimeSource = "COMPANY"
)

// OvertimeStatus mirrors the leave approval chain.
type OvertimeStatus string

const (
	OvertimePending   OvertimeStatus = "PENDING"
	OvertimeApproved  OvertimeStatus = "APPROVED"
	OvertimeRejected  OvertimeStatus = "REJECTED"
	OvertimeCancelled OvertimeStatus = "CANCELLED"
)

var OvertimeTransitions = TransitionMap[OvertimeStatus]{
	OvertimePending:   {OvertimeApproved, OvertimeRejected, OvertimeCancelled},
	OvertimeApproved:  {OvertimeCancelled},
	OvertimeRejected:  {},
	OvertimeCancelled: {},
}

// OvertimeRequest is extra hours claimed on a date.
type OvertimeRequest struct {
	ID              string         `json:"id"`
	EmployeeID      string         `json:"employeeId"`
	Date            Date           `json:"date"`
	StartTime       TimeOfDay      `json:"startTime"`
	EndTime         TimeOfDay      `json:"endTime"`
	Hours           float64        `json:"hours"`
	Source          OvertimeSource `json:"source"`
	Status          OvertimeStatus `json:"status"`
	Reason          *string        `json:"reason"`
	DecidedBy       *string        `json:"decidedBy"`
	DecidedAt       *time.Time     `json:"decidedAt"`
	RejectionReason *string        `json:"rejectionReason"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// OvertimeHours is the span between two wall-clock times on one date, rolling
// past midnight when the end is earlier than the start.
func OvertimeHours(start, end TimeOfDay) float64 {
	startMinutes := start.Hour*60 + start.Minute
	endMinutes := end.Hour*60 + end.Minute
	if endMinutes <= startMinutes {
		endMinutes += 24 * 60
	}
	return math.Round(float64(endMinutes-startMinutes)/60*100) / 100
}
