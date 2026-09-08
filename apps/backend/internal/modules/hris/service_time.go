package hris

import (
	"context"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/audit"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// The working day: turning up, being away, and the hours either side.

// ── Shared lookups ───────────────────────────────────────────────────────────

// people indexes the roster by id so a list of rows can name its subjects
// without a join across the boundary.
type people map[string]domain.Employee

func (s *Service) people(ctx context.Context) (people, error) {
	employees, err := s.repo.Employees(ctx, EmployeeFilter{})
	if err != nil {
		return nil, err
	}
	index := make(people, len(employees))
	for _, e := range employees {
		index[e.ID] = e
	}
	return index, nil
}

func (p people) name(employeeID string) string {
	if e, ok := p[employeeID]; ok {
		return e.FullName
	}
	return ""
}

func (p people) number(employeeID string) string {
	if e, ok := p[employeeID]; ok {
		return e.EmployeeNumber
	}
	return ""
}

// holidayIndex loads the published calendar for a range. Drafts are excluded:
// an unapproved import must never change leave arithmetic.
func (s *Service) holidayIndex(ctx context.Context, from, to domain.Date) (domain.HolidayIndex, error) {
	holidays, err := s.repo.Holidays(ctx, from, to, false)
	if err != nil {
		return nil, err
	}
	return domain.IndexHolidays(holidays), nil
}

// ── Attendance ───────────────────────────────────────────────────────────────

// AttendanceView is a day with its subject and shift named.
type AttendanceView struct {
	domain.Attendance
	EmployeeName   string  `json:"employeeName"`
	EmployeeNumber string  `json:"employeeNumber"`
	ShiftName      *string `json:"shiftName"`
}

func (s *Service) attendanceViews(ctx context.Context, rows []domain.Attendance) ([]AttendanceView, error) {
	staff, err := s.people(ctx)
	if err != nil {
		return nil, err
	}
	shifts, err := s.repo.Shifts(ctx, false)
	if err != nil {
		return nil, err
	}
	shiftNames := map[string]string{}
	for _, shift := range shifts {
		shiftNames[shift.ID] = shift.Name
	}

	views := make([]AttendanceView, 0, len(rows))
	for _, row := range rows {
		views = append(views, AttendanceView{
			Attendance:     row,
			EmployeeName:   staff.name(row.EmployeeID),
			EmployeeNumber: staff.number(row.EmployeeID),
			ShiftName:      lookup(shiftNames, row.ShiftID),
		})
	}
	return views, nil
}

func (s *Service) Attendances(ctx context.Context, filter AttendanceFilter) ([]AttendanceView, error) {
	rows, err := s.repo.Attendances(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.attendanceViews(ctx, rows)
}

// expectation is what an employee was supposed to do on a date.
type expectation struct {
	Shift     *domain.Shift
	Start     *time.Time
	End       *time.Time
	Scheduled bool // a pattern row exists for the day
	RestDay   bool // the pattern says the day is off
}

func (s *Service) expectationFor(ctx context.Context, employeeID string, date domain.Date) (expectation, error) {
	rows, err := s.repo.Schedule(ctx, employeeID)
	if err != nil {
		return expectation{}, err
	}
	row, ok := domain.ResolveScheduleRow(rows, date)
	if !ok {
		return expectation{}, nil
	}
	if row.ShiftID == nil {
		return expectation{Scheduled: true, RestDay: true}, nil
	}
	shift, err := s.repo.Shift(ctx, *row.ShiftID)
	if err != nil {
		return expectation{}, err
	}
	start, end := domain.ScheduledWindow(date, shift, s.studio)
	return expectation{Shift: &shift, Start: &start, End: &end, Scheduled: true}, nil
}

// ClockInput records an arrival or a departure.
type ClockInput struct {
	EmployeeID string
	// At defaults to now. Staff correcting a missed punch send a time.
	At     *time.Time
	Notes  *string
	Remote bool
}

// ClockIn opens an employee's day.
//
// The whole judgement happens here rather than at read time: the shift, the
// window and the lateness are frozen onto the row, so editing a shift next
// month never rewrites what somebody was expected to do last Tuesday.
func (s *Service) ClockIn(ctx context.Context, in ClockInput, actor Actor) (AttendanceView, error) {
	at := s.clock.Now()
	if in.At != nil {
		at = *in.At
	}
	date := domain.DateOf(at, s.studio)

	var saved domain.Attendance
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		employee, err := s.repo.Employee(ctx, in.EmployeeID)
		if err != nil {
			return err
		}
		if !employee.Active || !employee.IsEmployedOn(date) {
			return httpx.Conflict("NOT_EMPLOYED", "%s was not on the payroll on %s.", employee.FullName, date)
		}

		existing, found, err := s.repo.AttendanceOn(ctx, in.EmployeeID, date, true)
		if err != nil {
			return err
		}
		if found && existing.ClockIn != nil {
			return httpx.Conflict("ALREADY_CLOCKED_IN", "%s already clocked in on %s.", employee.FullName, date)
		}

		expect, err := s.expectationFor(ctx, in.EmployeeID, date)
		if err != nil {
			return err
		}

		row := existing
		if !found {
			row = domain.Attendance{
				ID: s.ids.New(id.Attendance), EmployeeID: in.EmployeeID, Date: date,
				BreakMinutes: 0,
			}
		}
		row.ClockIn = &at
		row.Notes = in.Notes
		row.Status = domain.AttendancePresent
		if in.Remote {
			row.Status = domain.AttendanceRemote
		}

		if expect.Shift != nil {
			row.ShiftID = &expect.Shift.ID
			row.ScheduledStart = expect.Start
			row.ScheduledEnd = expect.End
			row.BreakMinutes = expect.Shift.BreakMinutes

			lateness := domain.ComputeLateness(at, date, *expect.Shift, s.studio)
			row.IsLate = lateness.IsLate
			row.LateMinutes = lateness.LateMinutes
			if lateness.IsLate && !in.Remote {
				row.Status = domain.AttendanceLate
			}
		}

		if found {
			saved, err = s.repo.UpdateAttendance(ctx, row)
		} else {
			saved, err = s.repo.InsertAttendance(ctx, row)
		}
		return err
	})
	if err != nil {
		return AttendanceView{}, err
	}
	s.record(ctx, "hris.attendance", saved.ID, "CLOCK_IN", actor, nil)

	views, err := s.attendanceViews(ctx, []domain.Attendance{saved})
	if err != nil || len(views) == 0 {
		return AttendanceView{Attendance: saved}, err
	}
	return views[0], nil
}

// ClockOut closes the day and computes the hours actually worked.
func (s *Service) ClockOut(ctx context.Context, in ClockInput, actor Actor) (AttendanceView, error) {
	at := s.clock.Now()
	if in.At != nil {
		at = *in.At
	}
	// An overnight shift is clocked out on the following calendar day, so the
	// open row is looked for on both.
	date := domain.DateOf(at, s.studio)

	var saved domain.Attendance
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		row, found, err := s.repo.AttendanceOn(ctx, in.EmployeeID, date, true)
		if err != nil {
			return err
		}
		if !found || row.ClockIn == nil {
			previous, hadPrevious, err := s.repo.AttendanceOn(ctx, in.EmployeeID, date.AddDays(-1), true)
			if err != nil {
				return err
			}
			if !hadPrevious || previous.ClockIn == nil || previous.ClockOut != nil {
				return httpx.Conflict("NOT_CLOCKED_IN", "There is no open attendance record to close.")
			}
			row = previous
		}
		if row.ClockOut != nil {
			return httpx.Conflict("ALREADY_CLOCKED_OUT", "That day is already closed.")
		}
		if at.Before(*row.ClockIn) {
			return httpx.Invalid("A clock-out cannot come before the clock-in.")
		}

		row.ClockOut = &at
		row.WorkHours = domain.WorkHours(*row.ClockIn, at, row.BreakMinutes)
		if in.Notes != nil {
			row.Notes = in.Notes
		}
		saved, err = s.repo.UpdateAttendance(ctx, row)
		return err
	})
	if err != nil {
		return AttendanceView{}, err
	}
	s.record(ctx, "hris.attendance", saved.ID, "CLOCK_OUT", actor, nil)

	views, err := s.attendanceViews(ctx, []domain.Attendance{saved})
	if err != nil || len(views) == 0 {
		return AttendanceView{Attendance: saved}, err
	}
	return views[0], nil
}

// MarkInput is HR recording a day by hand: an absence, a day off, a
// correction to a missed punch.
type MarkInput struct {
	EmployeeID string
	Date       domain.Date
	Status     domain.AttendanceStatus
	ClockIn    *time.Time
	ClockOut   *time.Time
	Notes      *string
}

// MarkAttendance writes a day directly. It is the escape hatch for everything
// the clock cannot express, and it is audited for exactly that reason.
func (s *Service) MarkAttendance(ctx context.Context, in MarkInput, actor Actor) (AttendanceView, error) {
	switch in.Status {
	case domain.AttendancePresent, domain.AttendanceLate, domain.AttendanceAbsent,
		domain.AttendanceHalfDay, domain.AttendanceRemote, domain.AttendanceLeave,
		domain.AttendanceHoliday, domain.AttendanceRest:
	default:
		return AttendanceView{}, httpx.Invalid("%q is not an attendance status.", string(in.Status))
	}
	if in.ClockOut != nil && in.ClockIn == nil {
		return AttendanceView{}, httpx.Invalid("A clock-out needs a clock-in.")
	}
	if in.ClockIn != nil && in.ClockOut != nil && in.ClockOut.Before(*in.ClockIn) {
		return AttendanceView{}, httpx.Invalid("A clock-out cannot come before the clock-in.")
	}

	var saved domain.Attendance
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.Employee(ctx, in.EmployeeID); err != nil {
			return err
		}
		existing, found, err := s.repo.AttendanceOn(ctx, in.EmployeeID, in.Date, true)
		if err != nil {
			return err
		}

		row := existing
		if !found {
			row = domain.Attendance{ID: s.ids.New(id.Attendance), EmployeeID: in.EmployeeID, Date: in.Date}
		}
		row.Status = in.Status
		row.ClockIn = in.ClockIn
		row.ClockOut = in.ClockOut
		row.Notes = in.Notes
		row.WorkHours = 0
		row.IsLate = false
		row.LateMinutes = 0

		expect, err := s.expectationFor(ctx, in.EmployeeID, in.Date)
		if err != nil {
			return err
		}
		if expect.Shift != nil {
			row.ShiftID = &expect.Shift.ID
			row.ScheduledStart = expect.Start
			row.ScheduledEnd = expect.End
			row.BreakMinutes = expect.Shift.BreakMinutes
			if in.ClockIn != nil {
				lateness := domain.ComputeLateness(*in.ClockIn, in.Date, *expect.Shift, s.studio)
				row.IsLate = lateness.IsLate
				row.LateMinutes = lateness.LateMinutes
			}
		}
		if in.ClockIn != nil && in.ClockOut != nil {
			row.WorkHours = domain.WorkHours(*in.ClockIn, *in.ClockOut, row.BreakMinutes)
		}

		if found {
			saved, err = s.repo.UpdateAttendance(ctx, row)
		} else {
			saved, err = s.repo.InsertAttendance(ctx, row)
		}
		return err
	})
	if err != nil {
		return AttendanceView{}, err
	}
	s.record(ctx, "hris.attendance", saved.ID, "MARK_"+string(in.Status), actor, in.Notes)

	views, err := s.attendanceViews(ctx, []domain.Attendance{saved})
	if err != nil || len(views) == 0 {
		return AttendanceView{Attendance: saved}, err
	}
	return views[0], nil
}

// ── The daily roster ─────────────────────────────────────────────────────────

// RosterEntry is one person's day as it stands right now.
type RosterEntry struct {
	EmployeeID     string             `json:"employeeId"`
	EmployeeName   string             `json:"employeeName"`
	EmployeeNumber string             `json:"employeeNumber"`
	DepartmentID   *string            `json:"departmentId"`
	BranchID       *string            `json:"branchId"`
	ShiftName      *string            `json:"shiftName"`
	ScheduledStart *time.Time         `json:"scheduledStart"`
	ScheduledEnd   *time.Time         `json:"scheduledEnd"`
	Attendance     *domain.Attendance `json:"attendance"`
	Status         string             `json:"status"`
	LeaveType      *domain.LeaveType  `json:"leaveType"`
	Holiday        *domain.Holiday    `json:"holiday"`
}

// Roster statuses are the roster's own vocabulary: they say where a person
// stands right now, which is not always an attendance status.
const (
	RosterWorking     = "WORKING"
	RosterDone        = "DONE"
	RosterLate        = "LATE"
	RosterExpected    = "EXPECTED"
	RosterMissing     = "MISSING"
	RosterOnLeave     = "ON_LEAVE"
	RosterHoliday     = "HOLIDAY"
	RosterRestDay     = "REST_DAY"
	RosterUnscheduled = "UNSCHEDULED"
)

// Roster answers "who is meant to be in today, and who actually is".
//
// It reads the schedule, the holiday calendar, approved leave and the day's
// punches together, because any one of them alone gives the wrong answer.
func (s *Service) Roster(ctx context.Context, date domain.Date, branchID string) ([]RosterEntry, error) {
	if date == "" {
		date = s.Today()
	}
	employees, err := s.repo.Employees(ctx, EmployeeFilter{ActiveOnly: true, BranchID: branchID})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(employees))
	for _, e := range employees {
		ids = append(ids, e.ID)
	}
	schedules, err := s.repo.SchedulesFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	shifts, err := s.repo.Shifts(ctx, false)
	if err != nil {
		return nil, err
	}
	byShift := map[string]domain.Shift{}
	for _, shift := range shifts {
		byShift[shift.ID] = shift
	}
	attendance, err := s.repo.Attendances(ctx, AttendanceFilter{From: date, To: date, Limit: 1000})
	if err != nil {
		return nil, err
	}
	byEmployee := map[string]domain.Attendance{}
	for _, row := range attendance {
		byEmployee[row.EmployeeID] = row
	}
	leaves, err := s.repo.Leaves(ctx, LeaveFilter{From: date, To: date, Status: string(domain.LeaveApproved), Limit: 1000})
	if err != nil {
		return nil, err
	}
	leaveByEmployee := map[string]domain.Leave{}
	for _, leave := range leaves {
		leaveByEmployee[leave.EmployeeID] = leave
	}
	holidays, err := s.repo.Holidays(ctx, date, date, false)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	entries := make([]RosterEntry, 0, len(employees))
	for _, employee := range employees {
		if !employee.IsEmployedOn(date) {
			continue
		}
		entry := RosterEntry{
			EmployeeID: employee.ID, EmployeeName: employee.FullName,
			EmployeeNumber: employee.EmployeeNumber,
			DepartmentID:   employee.DepartmentID, BranchID: employee.BranchID,
		}

		row, ok := domain.ResolveScheduleRow(schedules[employee.ID], date)
		switch {
		case !ok:
			entry.Status = RosterUnscheduled
		case row.ShiftID == nil:
			entry.Status = RosterRestDay
		default:
			if shift, found := byShift[*row.ShiftID]; found {
				start, end := domain.ScheduledWindow(date, shift, s.studio)
				name := shift.Name
				entry.ShiftName = &name
				entry.ScheduledStart = &start
				entry.ScheduledEnd = &end
			}
			entry.Status = RosterExpected
		}

		// A holiday and approved leave both excuse the day, and they are worth
		// telling apart on the screen.
		if len(holidays) > 0 && entry.Status == RosterExpected {
			holiday := holidays[0]
			entry.Holiday = &holiday
			entry.Status = RosterHoliday
		}
		if leave, onLeave := leaveByEmployee[employee.ID]; onLeave {
			kind := leave.Type
			entry.LeaveType = &kind
			entry.Status = RosterOnLeave
		}

		if attended, present := byEmployee[employee.ID]; present {
			record := attended
			entry.Attendance = &record
			switch {
			case record.ClockOut != nil:
				entry.Status = RosterDone
			case record.IsLate:
				entry.Status = RosterLate
			case record.ClockIn != nil:
				entry.Status = RosterWorking
			case record.Status == domain.AttendanceAbsent:
				entry.Status = RosterMissing
			}
		} else if entry.Status == RosterExpected && entry.ScheduledEnd != nil && now.After(*entry.ScheduledEnd) {
			// The shift has ended and nobody clocked in. Saying MISSING is a
			// statement of fact; marking it ABSENT is HR's decision, not ours.
			entry.Status = RosterMissing
		}

		entries = append(entries, entry)
	}
	return entries, nil
}

// ── Leave ────────────────────────────────────────────────────────────────────

// LeaveView is a request with its subject named and the holidays it skipped.
type LeaveView struct {
	domain.Leave
	EmployeeName     string                   `json:"employeeName"`
	EmployeeNumber   string                   `json:"employeeNumber"`
	ExcludedHolidays []domain.ExcludedHoliday `json:"excludedHolidays,omitempty"`
}

func (s *Service) leaveViews(ctx context.Context, rows []domain.Leave) ([]LeaveView, error) {
	staff, err := s.people(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]LeaveView, 0, len(rows))
	for _, row := range rows {
		views = append(views, LeaveView{
			Leave: row, EmployeeName: staff.name(row.EmployeeID),
			EmployeeNumber: staff.number(row.EmployeeID),
		})
	}
	return views, nil
}

func (s *Service) Leaves(ctx context.Context, filter LeaveFilter) ([]LeaveView, error) {
	rows, err := s.repo.Leaves(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.leaveViews(ctx, rows)
}

// LeaveInput is a request to be away.
type LeaveInput struct {
	EmployeeID    string
	Type          domain.LeaveType
	StartDate     domain.Date
	EndDate       domain.Date
	Reason        string
	AttachmentURL *string
}

// leaveRejectionError turns a domain refusal into the HTTP answer, keeping the
// domain free of transport concerns and the codes stable for the UI.
func leaveRejectionError(rejection domain.LeaveRejection, breakdown domain.LeaveDaysBreakdown, balance domain.LeaveBalance) error {
	switch rejection {
	case domain.LeaveRejectDatesInverted:
		return httpx.Invalid("A leave request cannot end before it starts.")
	case domain.LeaveRejectNoWorkingDays:
		return httpx.Conflict("NO_WORKING_DAYS",
			"Every day in that range is already a weekend or a public holiday.")
	case domain.LeaveRejectOverlaps:
		return httpx.Conflict("OVERLAPS_EXISTING", "That range overlaps a request already on file.")
	case domain.LeaveRejectInsufficient:
		return httpx.Conflict("INSUFFICIENT_BALANCE",
			"That request needs %.1f days and only %.1f are left.",
			breakdown.TotalDays, balance.AnnualRemaining())
	case domain.LeaveRejectNotEmployed:
		return httpx.Conflict("NOT_EMPLOYED", "Those dates fall outside the employment period.")
	default:
		return httpx.Invalid("That leave request cannot be filed.")
	}
}

// RequestLeave files a request. Nothing is deducted yet: the allowance moves
// on approval, and until then the days are only reserved against further
// requests so two pending ones cannot together overspend the year.
func (s *Service) RequestLeave(ctx context.Context, in LeaveInput, actor Actor) (LeaveView, error) {
	if !domain.IsValidLeaveType(string(in.Type)) {
		return LeaveView{}, httpx.Invalid("%q is not a leave type.", string(in.Type))
	}
	if in.Reason == "" {
		return LeaveView{}, httpx.Invalid("A reason is required.")
	}

	var saved domain.Leave
	var breakdown domain.LeaveDaysBreakdown
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		employee, err := s.repo.Employee(ctx, in.EmployeeID)
		if err != nil {
			return err
		}
		year := in.StartDate.Year()
		balance, err := s.repo.Balance(ctx, in.EmployeeID, year, true)
		if err != nil {
			return err
		}
		holidays, err := s.holidayIndex(ctx, in.StartDate, in.EndDate)
		if err != nil {
			return err
		}
		existing, err := s.repo.Leaves(ctx, LeaveFilter{EmployeeID: in.EmployeeID, Limit: 500})
		if err != nil {
			return err
		}

		// Pending annual days are spoken for even though they have not been
		// deducted, so the balance the rules see is the honest one.
		reserved := balance
		for _, leave := range existing {
			if leave.Status == domain.LeavePending && leave.Type == domain.LeaveAnnual &&
				leave.StartDate.Year() == year {
				reserved.AnnualUsed += leave.TotalDays
			}
		}

		decision := domain.EvaluateLeave(domain.LeaveRequest{
			Employee: employee, Type: in.Type, StartDate: in.StartDate, EndDate: in.EndDate,
			Balance: reserved, Holidays: holidays, Existing: existing,
		})
		breakdown = decision.Breakdown
		if !decision.Allowed() {
			return leaveRejectionError(decision.Rejection, decision.Breakdown, reserved)
		}

		saved, err = s.repo.InsertLeave(ctx, domain.Leave{
			ID: s.ids.New(id.Leave), EmployeeID: in.EmployeeID, Type: in.Type,
			StartDate: in.StartDate, EndDate: in.EndDate, TotalDays: decision.Breakdown.TotalDays,
			Reason: in.Reason, AttachmentURL: in.AttachmentURL, Status: domain.LeavePending,
		})
		return err
	})
	if err != nil {
		return LeaveView{}, err
	}
	s.record(ctx, "hris.leave", saved.ID, "REQUEST", actor, audit.Str(in.Reason))

	views, err := s.leaveViews(ctx, []domain.Leave{saved})
	if err != nil || len(views) == 0 {
		return LeaveView{Leave: saved}, err
	}
	views[0].ExcludedHolidays = breakdown.ExcludedHolidays
	return views[0], nil
}

// DecideLeave approves or rejects a pending request.
func (s *Service) DecideLeave(ctx context.Context, leaveID string, approve bool, reason string, actor Actor) (LeaveView, error) {
	if !approve && reason == "" {
		return LeaveView{}, httpx.Invalid("A rejection needs a reason.")
	}

	var saved domain.Leave
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		leave, err := s.repo.Leave(ctx, leaveID, true)
		if err != nil {
			return err
		}
		target := domain.LeaveRejected
		if approve {
			target = domain.LeaveApproved
		}
		next, err := domain.Transition(domain.LeaveTransitions, leave.Status, target)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s request cannot become %s.", leave.Status, target)
		}

		now := s.clock.Now()
		leave.Status = next
		leave.ApprovedBy = &actor.ID
		leave.ApprovedAt = &now
		if approve {
			leave.RejectionReason = nil
			// The allowance moves only now. The database CHECK is the backstop
			// if two approvals race past the application's own arithmetic.
			if _, err := s.repo.AdjustLeaveUsage(ctx, leave.EmployeeID,
				leave.StartDate.Year(), leave.Type, leave.TotalDays); err != nil {
				return err
			}
		} else {
			leave.RejectionReason = &reason
		}

		saved, err = s.repo.UpdateLeaveStatus(ctx, leave)
		return err
	})
	if err != nil {
		return LeaveView{}, err
	}

	action := "REJECT"
	var note *string
	if approve {
		action = "APPROVE"
	} else {
		note = &reason
	}
	s.record(ctx, "hris.leave", leaveID, action, actor, note)

	views, err := s.leaveViews(ctx, []domain.Leave{saved})
	if err != nil || len(views) == 0 {
		return LeaveView{Leave: saved}, err
	}
	return views[0], nil
}

// CancelLeave withdraws a request, handing back any days already deducted.
func (s *Service) CancelLeave(ctx context.Context, leaveID string, actor Actor) (LeaveView, error) {
	var saved domain.Leave
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		leave, err := s.repo.Leave(ctx, leaveID, true)
		if err != nil {
			return err
		}
		wasApproved := leave.Status == domain.LeaveApproved

		next, err := domain.Transition(domain.LeaveTransitions, leave.Status, domain.LeaveCancelled)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s request cannot be cancelled.", leave.Status)
		}
		leave.Status = next

		if wasApproved {
			if _, err := s.repo.AdjustLeaveUsage(ctx, leave.EmployeeID,
				leave.StartDate.Year(), leave.Type, -leave.TotalDays); err != nil {
				return err
			}
		}
		saved, err = s.repo.UpdateLeaveStatus(ctx, leave)
		return err
	})
	if err != nil {
		return LeaveView{}, err
	}
	s.record(ctx, "hris.leave", leaveID, "CANCEL", actor, nil)

	views, err := s.leaveViews(ctx, []domain.Leave{saved})
	if err != nil || len(views) == 0 {
		return LeaveView{Leave: saved}, err
	}
	return views[0], nil
}

// Balance reads one employee's allowance for a year.
func (s *Service) Balance(ctx context.Context, employeeID string, year int) (domain.LeaveBalance, error) {
	if year == 0 {
		year = s.Today().Year()
	}
	if _, err := s.repo.Employee(ctx, employeeID); err != nil {
		return domain.LeaveBalance{}, err
	}
	return s.repo.Balance(ctx, employeeID, year, false)
}

// SetAnnualTotal changes an allowance, which happens when someone's seniority
// or contract earns them more days than the statutory twelve.
func (s *Service) SetAnnualTotal(ctx context.Context, employeeID string, year int, total float64, actor Actor) (domain.LeaveBalance, error) {
	if total < 0 {
		return domain.LeaveBalance{}, httpx.Invalid("An allowance cannot be negative.")
	}
	if year == 0 {
		year = s.Today().Year()
	}
	if _, err := s.repo.Balance(ctx, employeeID, year, false); err != nil {
		return domain.LeaveBalance{}, err
	}
	balance, err := s.repo.SetAnnualTotal(ctx, employeeID, year, total)
	if err != nil {
		return domain.LeaveBalance{}, err
	}
	s.record(ctx, "hris.employee", employeeID, "LEAVE_ALLOWANCE", actor, nil)
	return balance, nil
}

// ── Overtime ─────────────────────────────────────────────────────────────────

// OvertimeView is a claim with its subject named.
type OvertimeView struct {
	domain.OvertimeRequest
	EmployeeName   string `json:"employeeName"`
	EmployeeNumber string `json:"employeeNumber"`
}

func (s *Service) overtimeViews(ctx context.Context, rows []domain.OvertimeRequest) ([]OvertimeView, error) {
	staff, err := s.people(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]OvertimeView, 0, len(rows))
	for _, row := range rows {
		views = append(views, OvertimeView{
			OvertimeRequest: row, EmployeeName: staff.name(row.EmployeeID),
			EmployeeNumber: staff.number(row.EmployeeID),
		})
	}
	return views, nil
}

func (s *Service) Overtime(ctx context.Context, employeeID, status string, limit int) ([]OvertimeView, error) {
	rows, err := s.repo.OvertimeRequests(ctx, employeeID, status, limit)
	if err != nil {
		return nil, err
	}
	return s.overtimeViews(ctx, rows)
}

// OvertimeInput claims extra hours on a date.
type OvertimeInput struct {
	EmployeeID string
	Date       domain.Date
	StartTime  domain.TimeOfDay
	EndTime    domain.TimeOfDay
	Source     domain.OvertimeSource
	Reason     *string
}

func (s *Service) RequestOvertime(ctx context.Context, in OvertimeInput, actor Actor) (OvertimeView, error) {
	if in.Source != domain.OvertimeFromCompany {
		in.Source = domain.OvertimeFromEmployee
	}
	hours := domain.OvertimeHours(in.StartTime, in.EndTime)
	if hours <= 0 {
		return OvertimeView{}, httpx.Invalid("Overtime must cover some time.")
	}

	employee, err := s.repo.Employee(ctx, in.EmployeeID)
	if err != nil {
		return OvertimeView{}, err
	}
	if !employee.IsEmployedOn(in.Date) {
		return OvertimeView{}, httpx.Conflict("NOT_EMPLOYED", "%s was not on the payroll on %s.", employee.FullName, in.Date)
	}

	saved, err := s.repo.InsertOvertime(ctx, domain.OvertimeRequest{
		ID: s.ids.New(id.Overtime), EmployeeID: in.EmployeeID, Date: in.Date,
		StartTime: in.StartTime, EndTime: in.EndTime, Hours: hours,
		Source: in.Source, Status: domain.OvertimePending, Reason: in.Reason,
	})
	if err != nil {
		return OvertimeView{}, err
	}
	s.record(ctx, "hris.overtime", saved.ID, "REQUEST", actor, in.Reason)

	views, err := s.overtimeViews(ctx, []domain.OvertimeRequest{saved})
	if err != nil || len(views) == 0 {
		return OvertimeView{OvertimeRequest: saved}, err
	}
	return views[0], nil
}

// DecideOvertime approves or rejects a claim. Approving it also lands the
// hours on that day's attendance row, so the timesheet and the claim cannot
// disagree.
func (s *Service) DecideOvertime(ctx context.Context, overtimeID string, approve bool, reason string, actor Actor) (OvertimeView, error) {
	if !approve && reason == "" {
		return OvertimeView{}, httpx.Invalid("A rejection needs a reason.")
	}

	var saved domain.OvertimeRequest
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.OvertimeRequest(ctx, overtimeID)
		if err != nil {
			return err
		}
		target := domain.OvertimeRejected
		if approve {
			target = domain.OvertimeApproved
		}
		next, err := domain.Transition(domain.OvertimeTransitions, request.Status, target)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s claim cannot become %s.", request.Status, target)
		}

		now := s.clock.Now()
		request.Status = next
		request.DecidedBy = &actor.ID
		request.DecidedAt = &now
		if approve {
			request.RejectionReason = nil
			row, found, err := s.repo.AttendanceOn(ctx, request.EmployeeID, request.Date, true)
			if err != nil {
				return err
			}
			if found {
				row.OvertimeHours += request.Hours
				if _, err := s.repo.UpdateAttendance(ctx, row); err != nil {
					return err
				}
			}
		} else {
			request.RejectionReason = &reason
		}

		saved, err = s.repo.UpdateOvertimeStatus(ctx, request)
		return err
	})
	if err != nil {
		return OvertimeView{}, err
	}

	action := "REJECT"
	var note *string
	if approve {
		action = "APPROVE"
	} else {
		note = &reason
	}
	s.record(ctx, "hris.overtime", overtimeID, action, actor, note)

	views, err := s.overtimeViews(ctx, []domain.OvertimeRequest{saved})
	if err != nil || len(views) == 0 {
		return OvertimeView{OvertimeRequest: saved}, err
	}
	return views[0], nil
}

// CancelOvertime withdraws a claim, taking the hours back off the timesheet if
// it had already been approved.
func (s *Service) CancelOvertime(ctx context.Context, overtimeID string, actor Actor) (OvertimeView, error) {
	var saved domain.OvertimeRequest
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.OvertimeRequest(ctx, overtimeID)
		if err != nil {
			return err
		}
		wasApproved := request.Status == domain.OvertimeApproved

		next, err := domain.Transition(domain.OvertimeTransitions, request.Status, domain.OvertimeCancelled)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s claim cannot be cancelled.", request.Status)
		}
		request.Status = next

		if wasApproved {
			row, found, err := s.repo.AttendanceOn(ctx, request.EmployeeID, request.Date, true)
			if err != nil {
				return err
			}
			if found {
				row.OvertimeHours -= request.Hours
				if row.OvertimeHours < 0 {
					row.OvertimeHours = 0
				}
				if _, err := s.repo.UpdateAttendance(ctx, row); err != nil {
					return err
				}
			}
		}
		saved, err = s.repo.UpdateOvertimeStatus(ctx, request)
		return err
	})
	if err != nil {
		return OvertimeView{}, err
	}
	s.record(ctx, "hris.overtime", overtimeID, "CANCEL", actor, nil)

	views, err := s.overtimeViews(ctx, []domain.OvertimeRequest{saved})
	if err != nil || len(views) == 0 {
		return OvertimeView{OvertimeRequest: saved}, err
	}
	return views[0], nil
}

// ── Holidays ─────────────────────────────────────────────────────────────────

func (s *Service) Holidays(ctx context.Context, from, to domain.Date, includeDrafts bool) ([]domain.Holiday, error) {
	if from == "" && to == "" {
		year := s.Today().Year()
		from = domain.Date(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"))
		to = domain.Date(time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC).Format("2006-01-02"))
	}
	return s.repo.Holidays(ctx, from, to, includeDrafts)
}

// HolidayInput adds a day to the calendar.
type HolidayInput struct {
	Date domain.Date
	Name string
	Type domain.HolidayType
	// DeductsLeave separates collective leave, which is drawn from the annual
	// allowance, from a public holiday, which is not.
	DeductsLeave bool
	Note         *string
	Draft        bool
}

func (s *Service) CreateHoliday(ctx context.Context, in HolidayInput, actor Actor) (domain.Holiday, error) {
	switch in.Type {
	case domain.HolidayNational, domain.HolidayCollective, domain.HolidayCompany:
	default:
		return domain.Holiday{}, httpx.Invalid("%q is not a holiday type.", string(in.Type))
	}
	status := "ACTIVE"
	if in.Draft {
		status = "DRAFT"
	}
	created, err := s.repo.InsertHoliday(ctx, domain.Holiday{
		ID: s.ids.New(id.Holiday), Date: in.Date, Name: in.Name,
		Type: in.Type, DeductsLeave: in.DeductsLeave, Note: in.Note,
	}, status)
	if err != nil {
		return domain.Holiday{}, err
	}
	s.record(ctx, "hris.holiday", created.ID, "CREATE", actor, nil)
	return created, nil
}

func (s *Service) DeleteHoliday(ctx context.Context, holidayID string, actor Actor) error {
	if err := s.repo.DeleteHoliday(ctx, holidayID); err != nil {
		return err
	}
	s.record(ctx, "hris.holiday", holidayID, "DELETE", actor, nil)
	return nil
}

// ── Overview ─────────────────────────────────────────────────────────────────

// Overview is the HR dashboard: the numbers a manager checks each morning.
type Overview struct {
	Date            domain.Date       `json:"date"`
	ActiveEmployees int               `json:"activeEmployees"`
	TotalEmployees  int               `json:"totalEmployees"`
	Today           AttendanceSummary `json:"today"`
	Month           AttendanceSummary `json:"month"`
	PendingLeave    int               `json:"pendingLeave"`
	PendingOvertime int               `json:"pendingOvertime"`
	OnLeaveToday    int               `json:"onLeaveToday"`
	ExpectedToday   int               `json:"expectedToday"`
	MissingToday    int               `json:"missingToday"`
	NextHolidays    []domain.Holiday  `json:"nextHolidays"`
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	today := s.Today()
	active, total, err := s.repo.CountEmployees(ctx)
	if err != nil {
		return Overview{}, err
	}
	todaySummary, err := s.repo.SummarizeAttendance(ctx, today, today)
	if err != nil {
		return Overview{}, err
	}
	monthStart := domain.Date(string(today)[:8] + "01")
	monthSummary, err := s.repo.SummarizeAttendance(ctx, monthStart, today)
	if err != nil {
		return Overview{}, err
	}
	pendingLeave, err := s.repo.Leaves(ctx, LeaveFilter{Status: string(domain.LeavePending), Limit: 500})
	if err != nil {
		return Overview{}, err
	}
	pendingOvertime, err := s.repo.OvertimeRequests(ctx, "", string(domain.OvertimePending), 500)
	if err != nil {
		return Overview{}, err
	}
	roster, err := s.Roster(ctx, today, "")
	if err != nil {
		return Overview{}, err
	}
	// The next few holidays, however far out they are: an empty list because
	// the window was too short tells a manager nothing.
	upcoming, err := s.repo.Holidays(ctx, today, today.AddDays(365), false)
	if err != nil {
		return Overview{}, err
	}
	if len(upcoming) > 5 {
		upcoming = upcoming[:5]
	}

	overview := Overview{
		Date: today, ActiveEmployees: active, TotalEmployees: total,
		Today: todaySummary, Month: monthSummary,
		PendingLeave: len(pendingLeave), PendingOvertime: len(pendingOvertime),
		NextHolidays: upcoming,
	}
	for _, entry := range roster {
		switch entry.Status {
		case RosterOnLeave:
			overview.OnLeaveToday++
		case RosterExpected, RosterWorking, RosterLate:
			overview.ExpectedToday++
		case RosterMissing:
			overview.MissingToday++
		}
	}
	return overview, nil
}

// ── Self service ─────────────────────────────────────────────────────────────

// SelfView is what a signed-in staff member sees about their own working day.
//
// Every field is optional because a login is not necessarily on the payroll:
// an HQ account with no employee record gets an empty answer, not a 404.
type SelfView struct {
	Employee   *EmployeeView        `json:"employee"`
	Balance    *domain.LeaveBalance `json:"balance"`
	Today      *domain.Attendance   `json:"today"`
	Date       domain.Date          `json:"date"`
	ShiftName  *string              `json:"shiftName"`
	Expected   *time.Time           `json:"expectedStart"`
	ExpectedTo *time.Time           `json:"expectedEnd"`
	RestDay    bool                 `json:"restDay"`
	Scheduled  bool                 `json:"scheduled"`
	Leaves     []domain.Leave       `json:"leaves"`
}

// employeeForLogin resolves a staff login to the person behind it.
func (s *Service) employeeForLogin(ctx context.Context, adminUserID string) (domain.Employee, error) {
	employee, found, err := s.repo.EmployeeByAdminUser(ctx, adminUserID)
	if err != nil {
		return domain.Employee{}, err
	}
	if !found {
		return domain.Employee{}, httpx.Conflict("NOT_AN_EMPLOYEE",
			"That account is not linked to an employee record.")
	}
	return employee, nil
}

// Me is the signed-in staff member's own day.
func (s *Service) Me(ctx context.Context, adminUserID string) (SelfView, error) {
	today := s.Today()
	view := SelfView{Date: today}

	employee, found, err := s.repo.EmployeeByAdminUser(ctx, adminUserID)
	if err != nil {
		return SelfView{}, err
	}
	if !found {
		return view, nil
	}

	names, err := s.namer(ctx)
	if err != nil {
		return SelfView{}, err
	}
	employeeView := names.view(employee)
	view.Employee = &employeeView

	balance, err := s.repo.Balance(ctx, employee.ID, today.Year(), false)
	if err != nil {
		return SelfView{}, err
	}
	view.Balance = &balance

	if row, present, err := s.repo.AttendanceOn(ctx, employee.ID, today, false); err != nil {
		return SelfView{}, err
	} else if present {
		view.Today = &row
	}

	expect, err := s.expectationFor(ctx, employee.ID, today)
	if err != nil {
		return SelfView{}, err
	}
	view.Scheduled = expect.Scheduled
	view.RestDay = expect.RestDay
	view.Expected = expect.Start
	view.ExpectedTo = expect.End
	if expect.Shift != nil {
		name := expect.Shift.Name
		view.ShiftName = &name
	}

	leaves, err := s.repo.Leaves(ctx, LeaveFilter{EmployeeID: employee.ID, Limit: 10})
	if err != nil {
		return SelfView{}, err
	}
	view.Leaves = leaves

	return view, nil
}

// ClockInSelf and ClockOutSelf are the same punches, made by the person
// themselves rather than by the front desk on their behalf.
func (s *Service) ClockInSelf(ctx context.Context, adminUserID string, notes *string, actor Actor) (AttendanceView, error) {
	employee, err := s.employeeForLogin(ctx, adminUserID)
	if err != nil {
		return AttendanceView{}, err
	}
	return s.ClockIn(ctx, ClockInput{EmployeeID: employee.ID, Notes: notes}, actor)
}

func (s *Service) ClockOutSelf(ctx context.Context, adminUserID string, notes *string, actor Actor) (AttendanceView, error) {
	employee, err := s.employeeForLogin(ctx, adminUserID)
	if err != nil {
		return AttendanceView{}, err
	}
	return s.ClockOut(ctx, ClockInput{EmployeeID: employee.ID, Notes: notes}, actor)
}
