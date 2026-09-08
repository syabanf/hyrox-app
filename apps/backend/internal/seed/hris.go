package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
)

// seedHR loads the people side of the demo studio: the org chart, the shift
// patterns everyone works, this year's holidays, and a fortnight of punches so
// the timesheet has something to show on the first screen.
func (s *Seeder) seedHR(ctx context.Context) (int, error) {
	if err := s.seedOrgChart(ctx); err != nil {
		return 0, err
	}
	if err := s.seedShifts(ctx); err != nil {
		return 0, err
	}
	count, err := s.seedEmployees(ctx)
	if err != nil {
		return 0, err
	}
	if err := s.seedHolidays(ctx); err != nil {
		return 0, err
	}
	if err := s.seedTimesheet(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Seeder) seedOrgChart(ctx context.Context) error {
	departments := [][3]string{
		{"dep_ops", "Operations", "OPS"},
		{"dep_coaching", "Coaching", "COACH"},
		{"dep_frontoffice", "Front Office", "FO"},
		{"dep_finance", "Finance", "FIN"},
	}
	for _, d := range departments {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.departments (id, name, code) VALUES ($1, $2, $3)
			ON CONFLICT (id) DO NOTHING`, d[0], d[1], d[2]); err != nil {
			return fmt.Errorf("seed: department %s: %w", d[0], err)
		}
	}

	positions := [][3]string{
		{"pos_manager", "Studio Manager", "MANAGER"},
		{"pos_headcoach", "Head Coach", "SUPERVISOR"},
		{"pos_coach", "Coach", "STAFF"},
		{"pos_frontdesk", "Front Desk Officer", "STAFF"},
		{"pos_finance", "Finance Officer", "STAFF"},
		{"pos_director", "Director", "DIRECTOR"},
	}
	for _, p := range positions {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.positions (id, title, level) VALUES ($1, $2, $3)
			ON CONFLICT (id) DO NOTHING`, p[0], p[1], p[2]); err != nil {
			return fmt.Errorf("seed: position %s: %w", p[0], err)
		}
	}

	statuses := [][3]string{
		{"PERMANENT", "Permanent", "Open-ended contract."},
		{"PROBATION", "Probation", "First three months."},
		{"CONTRACT", "Contract", "Fixed term."},
		{"PART_TIME", "Part time", "Paid by the session."},
	}
	for _, st := range statuses {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.employment_statuses (id, code, name, description)
			VALUES ($1, $1, $2, $3) ON CONFLICT (id) DO NOTHING`,
			st[0], st[1], st[2]); err != nil {
			return fmt.Errorf("seed: employment status %s: %w", st[0], err)
		}
	}
	return nil
}

func (s *Seeder) seedShifts(ctx context.Context) error {
	shifts := []struct {
		id, name, start, end string
		breakMinutes         int
		tolerance            int
		overnight            bool
		order                int
	}{
		{"shf_opening", "Opening", "05:30:00", "13:30:00", 60, 10, false, 1},
		{"shf_middle", "Middle", "10:00:00", "18:00:00", 60, 10, false, 2},
		{"shf_closing", "Closing", "13:00:00", "21:00:00", 60, 10, false, 3},
		{"shf_office", "Office", "09:00:00", "17:00:00", 60, 15, false, 4},
	}
	for _, sh := range shifts {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.shifts (id, name, start_time, end_time, break_minutes,
				late_tolerance_minutes, is_overnight, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO NOTHING`,
			sh.id, sh.name, sh.start, sh.end, sh.breakMinutes, sh.tolerance,
			sh.overnight, sh.order); err != nil {
			return fmt.Errorf("seed: shift %s: %w", sh.id, err)
		}
	}
	return nil
}

// employeeSeed is one person, wired to the login they sign in with and the
// coach they teach as where those exist.
type employeeSeed struct {
	id, number, name, email, phone string
	department, position, branch   string
	adminUser, coach               *string
	status                         string
	joinedYearsAgo                 int
	reportsTo                      *string
	shift                          string
	// workDays are ISO weekdays the person is in; the rest are rest days.
	workDays []int
}

func (s *Seeder) seedEmployees(ctx context.Context) (int, error) {
	staff := []employeeSeed{
		{
			id: "emp_alya", number: "NH-0001", name: "Alya Santoso",
			email: "alya@nuhabit.id", phone: "+628110000001",
			department: "dep_ops", position: "pos_director", branch: "brn_senopati",
			adminUser: strPtr("adm_super"), status: "PERMANENT", joinedYearsAgo: 4,
			shift: "shf_office", workDays: []int{1, 2, 3, 4, 5},
		},
		{
			id: "emp_raka", number: "NH-0002", name: "Raka Wibowo",
			email: "raka@nuhabit.id", phone: "+628110000002",
			department: "dep_ops", position: "pos_manager", branch: "brn_pik",
			adminUser: strPtr("adm_hq"), status: "PERMANENT", joinedYearsAgo: 3,
			reportsTo: strPtr("emp_alya"), shift: "shf_office", workDays: []int{1, 2, 3, 4, 5},
		},
		{
			id: "emp_bima", number: "NH-0003", name: "Bima Prasetyo",
			email: "bima@nuhabit.id", phone: "+628110000003",
			department: "dep_ops", position: "pos_manager", branch: "brn_senopati",
			adminUser: strPtr("adm_branch"), status: "PERMANENT", joinedYearsAgo: 2,
			reportsTo: strPtr("emp_alya"), shift: "shf_middle", workDays: []int{1, 2, 3, 4, 5, 6},
		},
		{
			id: "emp_nadia", number: "NH-0004", name: "Nadia Putri",
			email: "nadia@nuhabit.id", phone: "+628110000004",
			department: "dep_frontoffice", position: "pos_frontdesk", branch: "brn_senopati",
			adminUser: strPtr("adm_desk"), status: "PERMANENT", joinedYearsAgo: 1,
			reportsTo: strPtr("emp_bima"), shift: "shf_opening", workDays: []int{1, 2, 3, 4, 5, 6},
		},
		{
			id: "emp_kevin", number: "NH-0005", name: "Kevin Hartono",
			email: "kevin@nuhabit.id", phone: "+628110000005",
			department: "dep_coaching", position: "pos_headcoach", branch: "brn_senopati",
			adminUser: strPtr("adm_coach"), coach: strPtr("coa_kevin"),
			status: "PERMANENT", joinedYearsAgo: 3,
			reportsTo: strPtr("emp_bima"), shift: "shf_opening", workDays: []int{1, 2, 3, 4, 5},
		},
		{
			id: "emp_sinta", number: "NH-0006", name: "Sinta Halim",
			email: "sinta@nuhabit.id", phone: "+628110000006",
			department: "dep_finance", position: "pos_finance", branch: "brn_senopati",
			adminUser: strPtr("adm_finance"), status: "PERMANENT", joinedYearsAgo: 2,
			reportsTo: strPtr("emp_alya"), shift: "shf_office", workDays: []int{1, 2, 3, 4, 5},
		},
		{
			id: "emp_maya", number: "NH-0007", name: "Maya Kusuma",
			email: "maya@nuhabit.id", phone: "+628110000007",
			department: "dep_coaching", position: "pos_coach", branch: "brn_pik",
			coach: strPtr("coa_maya"), status: "PERMANENT", joinedYearsAgo: 2,
			reportsTo: strPtr("emp_raka"), shift: "shf_closing", workDays: []int{1, 2, 3, 4, 5},
		},
		{
			id: "emp_rizky", number: "NH-0008", name: "Rizky Ramadhan",
			email: "rizky@nuhabit.id", phone: "+628110000008",
			department: "dep_coaching", position: "pos_coach", branch: "brn_senopati",
			coach: strPtr("coa_rizky"), status: "CONTRACT", joinedYearsAgo: 1,
			reportsTo: strPtr("emp_kevin"), shift: "shf_closing", workDays: []int{2, 3, 4, 5, 6},
		},
		{
			id: "emp_tara", number: "NH-0009", name: "Tara Widjaja",
			email: "tara@nuhabit.id", phone: "+628110000009",
			department: "dep_coaching", position: "pos_coach", branch: "brn_pik",
			coach: strPtr("coa_tara"), status: "PART_TIME", joinedYearsAgo: 1,
			reportsTo: strPtr("emp_raka"), shift: "shf_middle", workDays: []int{3, 4, 5, 6},
		},
	}

	studio := s.studioLocation()
	today := domain.DateOf(s.clock.Now(), studio)
	year := today.Year()

	// Two passes: everybody exists before anybody reports to anybody, so the
	// order of the list above never matters.
	for _, e := range staff {
		joined := time.Date(year-e.joinedYearsAgo, 2, 1, 0, 0, 0, 0, studio)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.employees (id, full_name, employee_number, email, phone, join_date,
				employment_status_code, department_id, position_id, branch_id, admin_user_id, coach_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (id) DO NOTHING`,
			e.id, e.name, e.number, e.email, e.phone, joined, e.status,
			e.department, e.position, e.branch, e.adminUser, e.coach); err != nil {
			return 0, fmt.Errorf("seed: employee %s: %w", e.id, err)
		}
	}
	for _, e := range staff {
		if e.reportsTo == nil {
			continue
		}
		if _, err := s.db.Exec(ctx,
			`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1 AND reporting_to IS NULL`,
			e.id, *e.reportsTo); err != nil {
			return 0, fmt.Errorf("seed: reporting line %s: %w", e.id, err)
		}
	}

	// The weekly pattern. A day the person does not work gets an explicit rest
	// row rather than nothing, so the roster can tell resting from unrostered.
	patternFrom := time.Date(year, 1, 1, 0, 0, 0, 0, studio)
	for _, e := range staff {
		working := map[int]bool{}
		for _, day := range e.workDays {
			working[day] = true
		}
		for day := 1; day <= 7; day++ {
			var shift *string
			if working[day] {
				shift = strPtr(e.shift)
			}
			rowID := fmt.Sprintf("esh_%s_%d", e.id[4:], day)
			if _, err := s.db.Exec(ctx, `
				INSERT INTO hris.employee_shifts (id, employee_id, day_of_week, shift_id, effective_from)
				VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`,
				rowID, e.id, day, shift, patternFrom); err != nil {
				return 0, fmt.Errorf("seed: schedule %s: %w", rowID, err)
			}
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.leave_balances (employee_id, year, annual_total, annual_used)
			VALUES ($1, $2, $3, 0) ON CONFLICT (employee_id, year) DO NOTHING`,
			e.id, year, domain.DefaultAnnualLeaveDays); err != nil {
			return 0, fmt.Errorf("seed: leave balance %s: %w", e.id, err)
		}
	}

	return len(staff), nil
}

// seedHolidays loads the fixed-date Indonesian holidays for the current year,
// plus one collective leave day — the case that matters, because collective
// leave is drawn from the annual allowance and a public holiday is not.
func (s *Seeder) seedHolidays(ctx context.Context) error {
	studio := s.studioLocation()
	year := s.clock.Now().In(studio).Year()

	holidays := []struct {
		id           string
		month        time.Month
		day          int
		name         string
		kind         domain.HolidayType
		deductsLeave bool
	}{
		{"hol_newyear", time.January, 1, "Tahun Baru Masehi", domain.HolidayNational, false},
		{"hol_labour", time.May, 1, "Hari Buruh Internasional", domain.HolidayNational, false},
		{"hol_pancasila", time.June, 1, "Hari Lahir Pancasila", domain.HolidayNational, false},
		{"hol_independence", time.August, 17, "Hari Kemerdekaan RI", domain.HolidayNational, false},
		{"hol_christmas", time.December, 25, "Hari Raya Natal", domain.HolidayNational, false},
		{"hol_collective", time.December, 26, "Cuti Bersama Natal", domain.HolidayCollective, true},
	}
	for _, h := range holidays {
		date := time.Date(year, h.month, h.day, 0, 0, 0, 0, studio)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.public_holidays (id, holiday_date, name, type, deducts_leave)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("%s_%d", h.id, year), date, h.name, h.kind, h.deductsLeave); err != nil {
			return fmt.Errorf("seed: holiday %s: %w", h.id, err)
		}
	}
	return nil
}

// seedTimesheet walks the last fortnight and records the day each employee was
// rostered for, using the same rules the live clock does — so the demo data is
// consistent with anything created afterwards rather than merely plausible.
func (s *Seeder) seedTimesheet(ctx context.Context) error {
	studio := s.studioLocation()
	now := s.clock.Now()
	today := domain.DateOf(now, studio)

	shifts, err := s.loadShifts(ctx)
	if err != nil {
		return err
	}
	patterns, err := s.loadPatterns(ctx)
	if err != nil {
		return err
	}
	holidays, err := s.loadHolidayDates(ctx)
	if err != nil {
		return err
	}

	// A small deterministic wobble: most people are on time, one arrives late
	// each week, and one day is missed entirely. Nothing random, so two seeds
	// of the same database produce the same timesheet.
	lateness := map[string]int{"emp_nadia": 18, "emp_rizky": 7}

	for offset := -14; offset < 0; offset++ {
		date := today.AddDays(offset)
		if holidays[date] {
			continue
		}
		for employeeID, rows := range patterns {
			row, ok := domain.ResolveWorkingShift(rows, date)
			if !ok {
				continue
			}
			shift, ok := shifts[*row.ShiftID]
			if !ok {
				continue
			}
			// One deliberate absence, so the roster has a MISSING row to show.
			if employeeID == "emp_tara" && offset == -3 {
				continue
			}

			start, end := domain.ScheduledWindow(date, shift, studio)
			minutesLate := 0
			if extra, unlucky := lateness[employeeID]; unlucky && offset%7 == -3 {
				minutesLate = extra
			}
			clockIn := start.Add(time.Duration(minutesLate) * time.Minute)
			clockOut := end

			late := domain.ComputeLateness(clockIn, date, shift, studio)
			status := domain.AttendancePresent
			if late.IsLate {
				status = domain.AttendanceLate
			}

			id := fmt.Sprintf("att_%s_%s", employeeID[4:], date)
			if _, err := s.db.Exec(ctx, `
				INSERT INTO hris.attendance (id, employee_id, date, clock_in, clock_out, shift_id,
					scheduled_start, scheduled_end, work_hours, break_minutes, status,
					is_late, late_minutes)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
				ON CONFLICT (employee_id, date) DO NOTHING`,
				id, employeeID, start, clockIn, clockOut, shift.ID, start, end,
				domain.WorkHours(clockIn, clockOut, shift.BreakMinutes), shift.BreakMinutes,
				status, late.IsLate, late.LateMinutes); err != nil {
				return fmt.Errorf("seed: attendance %s: %w", id, err)
			}
		}
	}

	// One approved leave in the past and one waiting for a decision, so the
	// approval screen is not empty on a fresh install.
	approvedStart := today.AddDays(-9)
	approvedEnd := today.AddDays(-8)
	pendingStart := today.AddDays(7)
	pendingEnd := today.AddDays(9)

	leaves := []struct {
		id, employee string
		kind         domain.LeaveType
		start, end   domain.Date
		days         float64
		reason       string
		status       domain.LeaveStatus
		approvedBy   *string
	}{
		{"lve_demo_approved", "emp_maya", domain.LeaveAnnual, approvedStart, approvedEnd, 2,
			"Family trip to Bandung.", domain.LeaveApproved, strPtr("adm_hq")},
		{"lve_demo_pending", "emp_nadia", domain.LeaveAnnual, pendingStart, pendingEnd, 3,
			"Sister's wedding.", domain.LeavePending, nil},
	}
	for _, l := range leaves {
		var approvedAt *time.Time
		if l.status == domain.LeaveApproved {
			decided := now.AddDate(0, 0, -12)
			approvedAt = &decided
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO hris.leaves (id, employee_id, type, start_date, end_date, total_days,
				reason, status, approved_by, approved_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT (id) DO NOTHING`,
			l.id, l.employee, l.kind, dateOf(l.start, studio), dateOf(l.end, studio),
			l.days, l.reason, l.status, l.approvedBy, approvedAt); err != nil {
			return fmt.Errorf("seed: leave %s: %w", l.id, err)
		}
		if l.status == domain.LeaveApproved {
			if _, err := s.db.Exec(ctx, `
				UPDATE hris.leave_balances SET annual_used = $3
				WHERE employee_id = $1 AND year = $2 AND annual_used = 0`,
				l.employee, l.start.Year(), l.days); err != nil {
				return fmt.Errorf("seed: leave balance for %s: %w", l.id, err)
			}
		}
	}
	return nil
}

// ── Small loaders, so the seeder computes from the same data the app reads ───

func (s *Seeder) loadShifts(ctx context.Context) (map[string]domain.Shift, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, start_time, end_time, break_minutes, late_tolerance_minutes,
			is_overnight FROM hris.shifts`)
	if err != nil {
		return nil, fmt.Errorf("seed: loading shifts: %w", err)
	}
	defer rows.Close()

	shifts := map[string]domain.Shift{}
	for rows.Next() {
		var shift domain.Shift
		var start, end time.Time
		if err := rows.Scan(&shift.ID, &shift.Name, &start, &end, &shift.BreakMinutes,
			&shift.LateToleranceMinutes, &shift.IsOvernight); err != nil {
			return nil, fmt.Errorf("seed: scanning shift: %w", err)
		}
		shift.StartTime = domain.TimeOfDay{Hour: start.Hour(), Minute: start.Minute()}
		shift.EndTime = domain.TimeOfDay{Hour: end.Hour(), Minute: end.Minute()}
		shifts[shift.ID] = shift
	}
	return shifts, rows.Err()
}

func (s *Seeder) loadPatterns(ctx context.Context) (map[string][]domain.EmployeeShift, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, employee_id, day_of_week, shift_id, effective_from, effective_to
		FROM hris.employee_shifts`)
	if err != nil {
		return nil, fmt.Errorf("seed: loading schedules: %w", err)
	}
	defer rows.Close()

	patterns := map[string][]domain.EmployeeShift{}
	for rows.Next() {
		var row domain.EmployeeShift
		var from time.Time
		var to *time.Time
		if err := rows.Scan(&row.ID, &row.EmployeeID, &row.DayOfWeek, &row.ShiftID, &from, &to); err != nil {
			return nil, fmt.Errorf("seed: scanning schedule: %w", err)
		}
		row.EffectiveFrom = domain.Date(from.Format("2006-01-02"))
		if to != nil {
			end := domain.Date(to.Format("2006-01-02"))
			row.EffectiveTo = &end
		}
		patterns[row.EmployeeID] = append(patterns[row.EmployeeID], row)
	}
	return patterns, rows.Err()
}

func (s *Seeder) loadHolidayDates(ctx context.Context) (map[domain.Date]bool, error) {
	rows, err := s.db.Query(ctx,
		`SELECT holiday_date FROM hris.public_holidays WHERE status = 'ACTIVE' AND deducts_leave = false`)
	if err != nil {
		return nil, fmt.Errorf("seed: loading holidays: %w", err)
	}
	defer rows.Close()

	dates := map[domain.Date]bool{}
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("seed: scanning holiday: %w", err)
		}
		dates[domain.Date(date.Format("2006-01-02"))] = true
	}
	return dates, rows.Err()
}

// studioLocation is where "today" happens. The seeder falls back to UTC rather
// than failing, because a missing tzdata should not stop a demo load.
func (s *Seeder) studioLocation() *time.Location {
	studio, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.UTC
	}
	return studio
}

// dateOf turns a calendar date into the midnight instant a DATE column stores.
func dateOf(date domain.Date, loc *time.Location) time.Time {
	return date.At(domain.TimeOfDay{}, loc)
}
