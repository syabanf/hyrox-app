package hris

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Shifts, schedules, attendance, leave, holidays and overtime — everything
// that turns a person into a working day.

// ── Shifts ───────────────────────────────────────────────────────────────────

const shiftColumns = `id, name, start_time, end_time, break_minutes, late_tolerance_minutes,
	is_overnight, active, sort_order`

func scanShift(row pgx.Row) (domain.Shift, error) {
	var s domain.Shift
	var start, end time.Time
	if err := row.Scan(&s.ID, &s.Name, &start, &end, &s.BreakMinutes,
		&s.LateToleranceMinutes, &s.IsOvernight, &s.Active, &s.SortOrder); err != nil {
		return domain.Shift{}, err
	}
	s.StartTime = domain.TimeOfDay{Hour: start.Hour(), Minute: start.Minute(), Second: start.Second()}
	s.EndTime = domain.TimeOfDay{Hour: end.Hour(), Minute: end.Minute(), Second: end.Second()}
	return s, nil
}

// timeValue renders a wall-clock time for a TIME column.
func timeValue(t domain.TimeOfDay) string { return t.String() }

func (r *Repository) Shifts(ctx context.Context, activeOnly bool) ([]domain.Shift, error) {
	query := `SELECT ` + shiftColumns + ` FROM hris.shifts`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY sort_order, start_time`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("hris: listing shifts: %w", err)
	}
	defer rows.Close()

	out := []domain.Shift{}
	for rows.Next() {
		s, err := scanShift(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning shift: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Shift(ctx context.Context, id string) (domain.Shift, error) {
	s, err := scanShift(r.db.QueryRow(ctx, `SELECT `+shiftColumns+` FROM hris.shifts WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Shift{}, httpx.NotFound("shift")
	}
	if err != nil {
		return domain.Shift{}, fmt.Errorf("hris: reading shift: %w", err)
	}
	return s, nil
}

func (r *Repository) InsertShift(ctx context.Context, s domain.Shift) (domain.Shift, error) {
	created, err := scanShift(r.db.QueryRow(ctx, `
		INSERT INTO hris.shifts (id, name, start_time, end_time, break_minutes,
			late_tolerance_minutes, is_overnight, active, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+shiftColumns,
		s.ID, s.Name, timeValue(s.StartTime), timeValue(s.EndTime), s.BreakMinutes,
		s.LateToleranceMinutes, s.IsOvernight, s.Active, s.SortOrder))
	if err != nil {
		return domain.Shift{}, fmt.Errorf("hris: inserting shift: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateShift(ctx context.Context, s domain.Shift) (domain.Shift, error) {
	updated, err := scanShift(r.db.QueryRow(ctx, `
		UPDATE hris.shifts SET name = $2, start_time = $3, end_time = $4, break_minutes = $5,
			late_tolerance_minutes = $6, is_overnight = $7, active = $8, sort_order = $9, updated_at = now()
		WHERE id = $1 RETURNING `+shiftColumns,
		s.ID, s.Name, timeValue(s.StartTime), timeValue(s.EndTime), s.BreakMinutes,
		s.LateToleranceMinutes, s.IsOvernight, s.Active, s.SortOrder))
	if database.IsNoRows(err) {
		return domain.Shift{}, httpx.NotFound("shift")
	}
	if err != nil {
		return domain.Shift{}, fmt.Errorf("hris: updating shift: %w", err)
	}
	return updated, nil
}

// ── Schedules ────────────────────────────────────────────────────────────────

const employeeShiftColumns = `id, employee_id, day_of_week, shift_id, effective_from, effective_to`

func scanEmployeeShift(row pgx.Row) (domain.EmployeeShift, error) {
	var es domain.EmployeeShift
	var from time.Time
	var to *time.Time
	if err := row.Scan(&es.ID, &es.EmployeeID, &es.DayOfWeek, &es.ShiftID, &from, &to); err != nil {
		return domain.EmployeeShift{}, err
	}
	es.EffectiveFrom = scanDate(&from)
	es.EffectiveTo = optionalDate(to)
	return es, nil
}

// Schedule returns every pattern row for an employee. Resolution to a single
// day happens in the domain, which is why the whole history comes back.
func (r *Repository) Schedule(ctx context.Context, employeeID string) ([]domain.EmployeeShift, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+employeeShiftColumns+` FROM hris.employee_shifts
		WHERE employee_id = $1 ORDER BY day_of_week, effective_from DESC`, employeeID)
	if err != nil {
		return nil, fmt.Errorf("hris: listing schedule: %w", err)
	}
	defer rows.Close()

	out := []domain.EmployeeShift{}
	for rows.Next() {
		es, err := scanEmployeeShift(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning schedule row: %w", err)
		}
		out = append(out, es)
	}
	return out, rows.Err()
}

// SchedulesFor loads patterns for many employees at once, which is what a
// daily roster needs.
func (r *Repository) SchedulesFor(ctx context.Context, employeeIDs []string) (map[string][]domain.EmployeeShift, error) {
	if len(employeeIDs) == 0 {
		return map[string][]domain.EmployeeShift{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+employeeShiftColumns+` FROM hris.employee_shifts
		WHERE employee_id = ANY($1) ORDER BY day_of_week, effective_from DESC`, employeeIDs)
	if err != nil {
		return nil, fmt.Errorf("hris: loading schedules: %w", err)
	}
	defer rows.Close()

	out := map[string][]domain.EmployeeShift{}
	for rows.Next() {
		es, err := scanEmployeeShift(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning schedule row: %w", err)
		}
		out[es.EmployeeID] = append(out[es.EmployeeID], es)
	}
	return out, rows.Err()
}

func (r *Repository) InsertScheduleRow(ctx context.Context, es domain.EmployeeShift) (domain.EmployeeShift, error) {
	created, err := scanEmployeeShift(r.db.QueryRow(ctx, `
		INSERT INTO hris.employee_shifts (id, employee_id, day_of_week, shift_id, effective_from, effective_to)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+employeeShiftColumns,
		es.ID, es.EmployeeID, es.DayOfWeek, es.ShiftID,
		requiredDate(es.EffectiveFrom), dateValue(es.EffectiveTo)))
	if database.IsForeignKeyViolation(err) {
		return domain.EmployeeShift{}, httpx.NotFound("employee or shift")
	}
	if err != nil {
		return domain.EmployeeShift{}, fmt.Errorf("hris: inserting schedule row: %w", err)
	}
	return created, nil
}

func (r *Repository) DeleteScheduleRow(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM hris.employee_shifts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("hris: deleting schedule row: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("schedule row")
	}
	return nil
}

// ── Attendance ───────────────────────────────────────────────────────────────

const attendanceColumns = `id, employee_id, date, clock_in, clock_out, shift_id, scheduled_start,
	scheduled_end, work_hours, break_minutes, status, is_late, late_minutes, overtime_hours,
	notes, created_at, updated_at`

func scanAttendance(row pgx.Row) (domain.Attendance, error) {
	var a domain.Attendance
	var date time.Time
	if err := row.Scan(&a.ID, &a.EmployeeID, &date, &a.ClockIn, &a.ClockOut, &a.ShiftID,
		&a.ScheduledStart, &a.ScheduledEnd, &a.WorkHours, &a.BreakMinutes, &a.Status,
		&a.IsLate, &a.LateMinutes, &a.OvertimeHours, &a.Notes, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return domain.Attendance{}, err
	}
	a.Date = scanDate(&date)
	return a, nil
}

// AttendanceFilter narrows the timesheet.
type AttendanceFilter struct {
	EmployeeID string
	From       domain.Date
	To         domain.Date
	Status     string
	Limit      int
}

func (r *Repository) Attendances(ctx context.Context, filter AttendanceFilter) ([]domain.Attendance, error) {
	query := `SELECT ` + attendanceColumns + ` FROM hris.attendance WHERE 1 = 1`
	args := []any{}

	if filter.EmployeeID != "" {
		args = append(args, filter.EmployeeID)
		query += fmt.Sprintf(` AND employee_id = $%d`, len(args))
	}
	if filter.From != "" {
		args = append(args, requiredDate(filter.From))
		query += fmt.Sprintf(` AND date >= $%d`, len(args))
	}
	if filter.To != "" {
		args = append(args, requiredDate(filter.To))
		query += fmt.Sprintf(` AND date <= $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY date DESC, employee_id LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hris: listing attendance: %w", err)
	}
	defer rows.Close()

	out := []domain.Attendance{}
	for rows.Next() {
		a, err := scanAttendance(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning attendance: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) Attendance(ctx context.Context, id string) (domain.Attendance, error) {
	a, err := scanAttendance(r.db.QueryRow(ctx, `SELECT `+attendanceColumns+` FROM hris.attendance WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Attendance{}, httpx.NotFound("attendance")
	}
	if err != nil {
		return domain.Attendance{}, fmt.Errorf("hris: reading attendance: %w", err)
	}
	return a, nil
}

// AttendanceOn finds the row for one employee on one date, locking it so two
// clock-ins from two devices cannot both create a day.
func (r *Repository) AttendanceOn(ctx context.Context, employeeID string, date domain.Date, forUpdate bool) (domain.Attendance, bool, error) {
	query := `SELECT ` + attendanceColumns + ` FROM hris.attendance WHERE employee_id = $1 AND date = $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	a, err := scanAttendance(r.db.QueryRow(ctx, query, employeeID, requiredDate(date)))
	if database.IsNoRows(err) {
		return domain.Attendance{}, false, nil
	}
	if err != nil {
		return domain.Attendance{}, false, fmt.Errorf("hris: reading attendance for date: %w", err)
	}
	return a, true, nil
}

func (r *Repository) InsertAttendance(ctx context.Context, a domain.Attendance) (domain.Attendance, error) {
	created, err := scanAttendance(r.db.QueryRow(ctx, `
		INSERT INTO hris.attendance (id, employee_id, date, clock_in, clock_out, shift_id,
			scheduled_start, scheduled_end, work_hours, break_minutes, status, is_late,
			late_minutes, overtime_hours, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING `+attendanceColumns,
		a.ID, a.EmployeeID, requiredDate(a.Date), a.ClockIn, a.ClockOut, a.ShiftID,
		a.ScheduledStart, a.ScheduledEnd, a.WorkHours, a.BreakMinutes, a.Status,
		a.IsLate, a.LateMinutes, a.OvertimeHours, a.Notes))
	if database.IsUniqueViolation(err) {
		return domain.Attendance{}, httpx.Conflict("ALREADY_CLOCKED_IN",
			"That employee already has an attendance record for the day.")
	}
	if err != nil {
		return domain.Attendance{}, fmt.Errorf("hris: inserting attendance: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateAttendance(ctx context.Context, a domain.Attendance) (domain.Attendance, error) {
	updated, err := scanAttendance(r.db.QueryRow(ctx, `
		UPDATE hris.attendance SET clock_in = $2, clock_out = $3, shift_id = $4,
			scheduled_start = $5, scheduled_end = $6, work_hours = $7, break_minutes = $8,
			status = $9, is_late = $10, late_minutes = $11, overtime_hours = $12,
			notes = $13, updated_at = now()
		WHERE id = $1 RETURNING `+attendanceColumns,
		a.ID, a.ClockIn, a.ClockOut, a.ShiftID, a.ScheduledStart, a.ScheduledEnd,
		a.WorkHours, a.BreakMinutes, a.Status, a.IsLate, a.LateMinutes, a.OvertimeHours, a.Notes))
	if database.IsNoRows(err) {
		return domain.Attendance{}, httpx.NotFound("attendance")
	}
	if err != nil {
		return domain.Attendance{}, fmt.Errorf("hris: updating attendance: %w", err)
	}
	return updated, nil
}

// AttendanceSummary counts a period for the summary cards.
type AttendanceSummary struct {
	Present     int     `json:"present"`
	Late        int     `json:"late"`
	Absent      int     `json:"absent"`
	OnLeave     int     `json:"onLeave"`
	TotalHours  float64 `json:"totalHours"`
	LateMinutes int     `json:"lateMinutes"`
}

func (r *Repository) SummarizeAttendance(ctx context.Context, from, to domain.Date) (AttendanceSummary, error) {
	var s AttendanceSummary
	err := r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN ('PRESENT', 'REMOTE', 'HALF_DAY')),
			count(*) FILTER (WHERE status = 'LATE'),
			count(*) FILTER (WHERE status = 'ABSENT'),
			count(*) FILTER (WHERE status = 'ON_LEAVE'),
			COALESCE(SUM(work_hours), 0),
			COALESCE(SUM(late_minutes), 0)
		FROM hris.attendance WHERE date BETWEEN $1 AND $2`,
		requiredDate(from), requiredDate(to)).
		Scan(&s.Present, &s.Late, &s.Absent, &s.OnLeave, &s.TotalHours, &s.LateMinutes)
	if err != nil {
		return AttendanceSummary{}, fmt.Errorf("hris: summarizing attendance: %w", err)
	}
	return s, nil
}

// ── Holidays ─────────────────────────────────────────────────────────────────

const holidayColumns = `id, holiday_date, name, type, deducts_leave, note`

func scanHoliday(row pgx.Row) (domain.Holiday, error) {
	var h domain.Holiday
	var date time.Time
	if err := row.Scan(&h.ID, &date, &h.Name, &h.Type, &h.DeductsLeave, &h.Note); err != nil {
		return domain.Holiday{}, err
	}
	h.Date = scanDate(&date)
	return h, nil
}

// Holidays returns published holidays in a range. Draft rows are excluded
// everywhere: an unapproved import must not silently change leave arithmetic.
func (r *Repository) Holidays(ctx context.Context, from, to domain.Date, includeDrafts bool) ([]domain.Holiday, error) {
	query := `SELECT ` + holidayColumns + ` FROM hris.public_holidays WHERE 1 = 1`
	args := []any{}
	if !includeDrafts {
		query += ` AND status = 'ACTIVE'`
	}
	if from != "" {
		args = append(args, requiredDate(from))
		query += fmt.Sprintf(` AND holiday_date >= $%d`, len(args))
	}
	if to != "" {
		args = append(args, requiredDate(to))
		query += fmt.Sprintf(` AND holiday_date <= $%d`, len(args))
	}
	query += ` ORDER BY holiday_date`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hris: listing holidays: %w", err)
	}
	defer rows.Close()

	out := []domain.Holiday{}
	for rows.Next() {
		h, err := scanHoliday(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning holiday: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *Repository) InsertHoliday(ctx context.Context, h domain.Holiday, status string) (domain.Holiday, error) {
	created, err := scanHoliday(r.db.QueryRow(ctx, `
		INSERT INTO hris.public_holidays (id, holiday_date, name, type, deducts_leave, status, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+holidayColumns,
		h.ID, requiredDate(h.Date), h.Name, h.Type, h.DeductsLeave, status, h.Note))
	if database.IsUniqueViolation(err) {
		return domain.Holiday{}, httpx.Conflict("DUPLICATE", "That holiday is already on the calendar.")
	}
	if err != nil {
		return domain.Holiday{}, fmt.Errorf("hris: inserting holiday: %w", err)
	}
	return created, nil
}

func (r *Repository) DeleteHoliday(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM hris.public_holidays WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("hris: deleting holiday: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("holiday")
	}
	return nil
}

// ── Leave ────────────────────────────────────────────────────────────────────

const leaveColumns = `id, employee_id, type, start_date, end_date, total_days, reason,
	attachment_url, status, approved_by, approved_at, rejection_reason, created_at, updated_at`

func scanLeave(row pgx.Row) (domain.Leave, error) {
	var l domain.Leave
	var start, end time.Time
	if err := row.Scan(&l.ID, &l.EmployeeID, &l.Type, &start, &end, &l.TotalDays, &l.Reason,
		&l.AttachmentURL, &l.Status, &l.ApprovedBy, &l.ApprovedAt, &l.RejectionReason,
		&l.CreatedAt, &l.UpdatedAt); err != nil {
		return domain.Leave{}, err
	}
	l.StartDate = scanDate(&start)
	l.EndDate = scanDate(&end)
	return l, nil
}

// LeaveFilter narrows the leave list.
type LeaveFilter struct {
	EmployeeID string
	Status     string
	Type       string
	From       domain.Date
	To         domain.Date
	Limit      int
}

func (r *Repository) Leaves(ctx context.Context, filter LeaveFilter) ([]domain.Leave, error) {
	query := `SELECT ` + leaveColumns + ` FROM hris.leaves WHERE 1 = 1`
	args := []any{}

	if filter.EmployeeID != "" {
		args = append(args, filter.EmployeeID)
		query += fmt.Sprintf(` AND employee_id = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.Type != "" {
		args = append(args, filter.Type)
		query += fmt.Sprintf(` AND type = $%d`, len(args))
	}
	// A request overlaps the window when it starts before the window ends and
	// ends after the window starts.
	if filter.To != "" {
		args = append(args, requiredDate(filter.To))
		query += fmt.Sprintf(` AND start_date <= $%d`, len(args))
	}
	if filter.From != "" {
		args = append(args, requiredDate(filter.From))
		query += fmt.Sprintf(` AND end_date >= $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY start_date DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hris: listing leave: %w", err)
	}
	defer rows.Close()

	out := []domain.Leave{}
	for rows.Next() {
		l, err := scanLeave(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning leave: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) Leave(ctx context.Context, id string, forUpdate bool) (domain.Leave, error) {
	query := `SELECT ` + leaveColumns + ` FROM hris.leaves WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	l, err := scanLeave(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.Leave{}, httpx.NotFound("leave request")
	}
	if err != nil {
		return domain.Leave{}, fmt.Errorf("hris: reading leave: %w", err)
	}
	return l, nil
}

func (r *Repository) InsertLeave(ctx context.Context, l domain.Leave) (domain.Leave, error) {
	created, err := scanLeave(r.db.QueryRow(ctx, `
		INSERT INTO hris.leaves (id, employee_id, type, start_date, end_date, total_days,
			reason, attachment_url, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+leaveColumns,
		l.ID, l.EmployeeID, l.Type, requiredDate(l.StartDate), requiredDate(l.EndDate),
		l.TotalDays, l.Reason, l.AttachmentURL, l.Status))
	if database.IsForeignKeyViolation(err) {
		return domain.Leave{}, httpx.NotFound("employee")
	}
	if err != nil {
		return domain.Leave{}, fmt.Errorf("hris: inserting leave: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateLeaveStatus(ctx context.Context, l domain.Leave) (domain.Leave, error) {
	updated, err := scanLeave(r.db.QueryRow(ctx, `
		UPDATE hris.leaves SET status = $2, approved_by = $3, approved_at = $4,
			rejection_reason = $5, updated_at = now()
		WHERE id = $1 RETURNING `+leaveColumns,
		l.ID, l.Status, l.ApprovedBy, l.ApprovedAt, l.RejectionReason))
	if database.IsNoRows(err) {
		return domain.Leave{}, httpx.NotFound("leave request")
	}
	if err != nil {
		return domain.Leave{}, fmt.Errorf("hris: updating leave: %w", err)
	}
	return updated, nil
}

// ── Leave balances ───────────────────────────────────────────────────────────

const balanceColumns = `employee_id, year, annual_total, annual_used, sick_used, unpaid_used,
	maternity_used, paternity_used, emergency_used, pilgrimage_used, menstrual_used`

func scanBalance(row pgx.Row) (domain.LeaveBalance, error) {
	var b domain.LeaveBalance
	err := row.Scan(&b.EmployeeID, &b.Year, &b.AnnualTotal, &b.AnnualUsed, &b.SickUsed,
		&b.UnpaidUsed, &b.MaternityUsed, &b.PaternityUsed, &b.EmergencyUsed,
		&b.PilgrimageUsed, &b.MenstrualUsed)
	return b, err
}

// Balance reads an allowance, creating the year's row on first read so a new
// employee always has one rather than a missing-row special case everywhere.
func (r *Repository) Balance(ctx context.Context, employeeID string, year int, forUpdate bool) (domain.LeaveBalance, error) {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO hris.leave_balances (employee_id, year, annual_total)
		VALUES ($1, $2, $3) ON CONFLICT (employee_id, year) DO NOTHING`,
		employeeID, year, domain.DefaultAnnualLeaveDays); err != nil {
		if database.IsForeignKeyViolation(err) {
			return domain.LeaveBalance{}, httpx.NotFound("employee")
		}
		return domain.LeaveBalance{}, fmt.Errorf("hris: ensuring leave balance: %w", err)
	}

	query := `SELECT ` + balanceColumns + ` FROM hris.leave_balances WHERE employee_id = $1 AND year = $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	b, err := scanBalance(r.db.QueryRow(ctx, query, employeeID, year))
	if err != nil {
		return domain.LeaveBalance{}, fmt.Errorf("hris: reading leave balance: %w", err)
	}
	return b, nil
}

// usedColumn maps a leave type to the counter it draws from.
func usedColumn(kind domain.LeaveType) string {
	switch kind {
	case domain.LeaveAnnual:
		return "annual_used"
	case domain.LeaveSick:
		return "sick_used"
	case domain.LeaveUnpaid:
		return "unpaid_used"
	case domain.LeaveMaternity:
		return "maternity_used"
	case domain.LeavePaternity:
		return "paternity_used"
	case domain.LeaveEmergency:
		return "emergency_used"
	case domain.LeavePilgrimage:
		return "pilgrimage_used"
	case domain.LeaveMenstrual:
		return "menstrual_used"
	default:
		return ""
	}
}

// AdjustLeaveUsage moves a type's counter by days, which may be negative when
// approved leave is cancelled and the allowance is handed back.
func (r *Repository) AdjustLeaveUsage(ctx context.Context, employeeID string, year int, kind domain.LeaveType, days float64) (domain.LeaveBalance, error) {
	column := usedColumn(kind)
	if column == "" {
		return domain.LeaveBalance{}, httpx.Invalid("Unknown leave type %q.", kind)
	}
	// The column name comes from the closed switch above, never from input.
	b, err := scanBalance(r.db.QueryRow(ctx, fmt.Sprintf(`
		UPDATE hris.leave_balances
		SET %s = GREATEST(0, %s + $3), updated_at = now()
		WHERE employee_id = $1 AND year = $2 RETURNING %s`, column, column, balanceColumns),
		employeeID, year, days))
	if database.IsNoRows(err) {
		return domain.LeaveBalance{}, httpx.NotFound("leave balance")
	}
	if database.IsCheckViolation(err) {
		return domain.LeaveBalance{}, httpx.Conflict("INSUFFICIENT_BALANCE",
			"That would take the annual allowance past its total.")
	}
	if err != nil {
		return domain.LeaveBalance{}, fmt.Errorf("hris: adjusting leave usage: %w", err)
	}
	return b, nil
}

func (r *Repository) SetAnnualTotal(ctx context.Context, employeeID string, year int, total float64) (domain.LeaveBalance, error) {
	b, err := scanBalance(r.db.QueryRow(ctx, `
		UPDATE hris.leave_balances SET annual_total = $3, updated_at = now()
		WHERE employee_id = $1 AND year = $2 RETURNING `+balanceColumns,
		employeeID, year, total))
	if database.IsNoRows(err) {
		return domain.LeaveBalance{}, httpx.NotFound("leave balance")
	}
	if database.IsCheckViolation(err) {
		return domain.LeaveBalance{}, httpx.Conflict("BELOW_USED",
			"The allowance cannot be set below what has already been taken.")
	}
	if err != nil {
		return domain.LeaveBalance{}, fmt.Errorf("hris: setting annual total: %w", err)
	}
	return b, nil
}

// ── Overtime ─────────────────────────────────────────────────────────────────

const overtimeColumns = `id, employee_id, date, start_time, end_time, hours, source, status,
	reason, decided_by, decided_at, rejection_reason, created_at, updated_at`

func scanOvertime(row pgx.Row) (domain.OvertimeRequest, error) {
	var o domain.OvertimeRequest
	var date, start, end time.Time
	if err := row.Scan(&o.ID, &o.EmployeeID, &date, &start, &end, &o.Hours, &o.Source,
		&o.Status, &o.Reason, &o.DecidedBy, &o.DecidedAt, &o.RejectionReason,
		&o.CreatedAt, &o.UpdatedAt); err != nil {
		return domain.OvertimeRequest{}, err
	}
	o.Date = scanDate(&date)
	o.StartTime = domain.TimeOfDay{Hour: start.Hour(), Minute: start.Minute(), Second: start.Second()}
	o.EndTime = domain.TimeOfDay{Hour: end.Hour(), Minute: end.Minute(), Second: end.Second()}
	return o, nil
}

func (r *Repository) OvertimeRequests(ctx context.Context, employeeID, status string, limit int) ([]domain.OvertimeRequest, error) {
	query := `SELECT ` + overtimeColumns + ` FROM hris.overtime_requests WHERE 1 = 1`
	args := []any{}
	if employeeID != "" {
		args = append(args, employeeID)
		query += fmt.Sprintf(` AND employee_id = $%d`, len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY date DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hris: listing overtime: %w", err)
	}
	defer rows.Close()

	out := []domain.OvertimeRequest{}
	for rows.Next() {
		o, err := scanOvertime(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning overtime: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repository) OvertimeRequest(ctx context.Context, id string) (domain.OvertimeRequest, error) {
	o, err := scanOvertime(r.db.QueryRow(ctx, `SELECT `+overtimeColumns+` FROM hris.overtime_requests WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.OvertimeRequest{}, httpx.NotFound("overtime request")
	}
	if err != nil {
		return domain.OvertimeRequest{}, fmt.Errorf("hris: reading overtime: %w", err)
	}
	return o, nil
}

func (r *Repository) InsertOvertime(ctx context.Context, o domain.OvertimeRequest) (domain.OvertimeRequest, error) {
	created, err := scanOvertime(r.db.QueryRow(ctx, `
		INSERT INTO hris.overtime_requests (id, employee_id, date, start_time, end_time, hours,
			source, status, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+overtimeColumns,
		o.ID, o.EmployeeID, requiredDate(o.Date), timeValue(o.StartTime), timeValue(o.EndTime),
		o.Hours, o.Source, o.Status, o.Reason))
	if database.IsForeignKeyViolation(err) {
		return domain.OvertimeRequest{}, httpx.NotFound("employee")
	}
	if err != nil {
		return domain.OvertimeRequest{}, fmt.Errorf("hris: inserting overtime: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateOvertimeStatus(ctx context.Context, o domain.OvertimeRequest) (domain.OvertimeRequest, error) {
	updated, err := scanOvertime(r.db.QueryRow(ctx, `
		UPDATE hris.overtime_requests SET status = $2, decided_by = $3, decided_at = $4,
			rejection_reason = $5, updated_at = now()
		WHERE id = $1 RETURNING `+overtimeColumns,
		o.ID, o.Status, o.DecidedBy, o.DecidedAt, o.RejectionReason))
	if database.IsNoRows(err) {
		return domain.OvertimeRequest{}, httpx.NotFound("overtime request")
	}
	if err != nil {
		return domain.OvertimeRequest{}, fmt.Errorf("hris: updating overtime: %w", err)
	}
	return updated, nil
}
