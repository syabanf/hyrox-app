package app_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// HRIS tests, driven over real HTTP against a real database like the rest.
//
// Everything here builds its own employee, shift and calendar rather than
// leaning on the demo seed, so a test never depends on which weekday it runs.

// studioTZ is the timezone the backend resolves calendar dates in. Building
// timestamps in it is what makes "clocked in at 08:25" mean the same thing to
// the test and to the server.
func studioTZ(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("loading the studio timezone: %v", err)
	}
	return loc
}

// hrisFixture is an employee rostered onto a known shift every day of the
// week, which removes the calendar from every assertion that follows.
type hrisFixture struct {
	token      string
	employeeID string
	shiftID    string
	studio     *time.Location
}

func (h *harness) newEmployee(t *testing.T, number string) hrisFixture {
	t.Helper()
	token := h.adminToken("adm_super")
	studio := studioTZ(t)

	status, shift := h.request(http.MethodPost, "/api/admin/hris/shifts", token, map[string]any{
		"name": "Test " + number, "startTime": "08:00", "endTime": "16:00",
		"breakMinutes": 60, "lateToleranceMinutes": 10,
	})
	if status != http.StatusCreated {
		t.Fatalf("creating a shift returned %d: %v", status, shift)
	}
	shiftID := shift["id"].(string)

	joined := time.Now().In(studio).AddDate(-2, 0, 0).Format("2006-01-02")
	status, employee := h.request(http.MethodPost, "/api/admin/hris/employees", token, map[string]any{
		"fullName": "Test Person " + number, "employeeNumber": number,
		"email": "test" + number + "@nuhabit.id", "phone": "+628110009999",
		"joinDate": joined, "employmentStatusCode": "PERMANENT",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating an employee returned %d: %v", status, employee)
	}
	employeeID := employee["id"].(string)

	// Every weekday, so no test has to care what day it runs on.
	for day := 1; day <= 7; day++ {
		status, row := h.request(http.MethodPost,
			"/api/admin/hris/employees/"+employeeID+"/schedule", token, map[string]any{
				"dayOfWeek": day, "shiftId": shiftID, "effectiveFrom": joined,
			})
		if status != http.StatusCreated {
			t.Fatalf("assigning day %d returned %d: %v", day, status, row)
		}
	}
	return hrisFixture{token: token, employeeID: employeeID, shiftID: shiftID, studio: studio}
}

// at builds an instant at a wall-clock time on a date in the studio timezone.
func at(studio *time.Location, day time.Time, hour, minute int) string {
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, studio).
		Format(time.RFC3339)
}

func TestAttendanceMeasuresLatenessFromTheShiftStart(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0001")
	today := time.Now().In(f.studio)

	// Twenty-five minutes into a shift with ten minutes' grace is twenty-five
	// minutes late, not fifteen. Getting this backwards under-reports every
	// lateness in the system, so it is asserted rather than assumed.
	status, day := h.request(http.MethodPost, "/api/admin/hris/attendance/clock-in", f.token,
		map[string]any{"employeeId": f.employeeID, "at": at(f.studio, today, 8, 25)})
	if status != http.StatusCreated {
		t.Fatalf("clocking in returned %d: %v", status, day)
	}
	if day["isLate"] != true {
		t.Fatalf("expected a late arrival, got %v", day["isLate"])
	}
	if minutes := day["lateMinutes"].(float64); minutes != 25 {
		t.Fatalf("lateness is measured from the shift start: want 25 minutes, got %v", minutes)
	}
	if day["status"] != "LATE" {
		t.Fatalf("want status LATE, got %v", day["status"])
	}

	// Arriving inside the tolerance is not late at all.
	other := h.newEmployee(t, "TS-0002")
	status, punctual := h.request(http.MethodPost, "/api/admin/hris/attendance/clock-in", other.token,
		map[string]any{"employeeId": other.employeeID, "at": at(other.studio, today, 8, 9)})
	if status != http.StatusCreated {
		t.Fatalf("clocking in returned %d: %v", status, punctual)
	}
	if punctual["isLate"] != false || punctual["status"] != "PRESENT" {
		t.Fatalf("nine minutes is inside the grace period: %v", punctual)
	}
}

func TestClockingInTwiceIsRefused(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0003")
	today := time.Now().In(f.studio)

	h.request(http.MethodPost, "/api/admin/hris/attendance/clock-in", f.token,
		map[string]any{"employeeId": f.employeeID, "at": at(f.studio, today, 8, 0)})

	status, refused := h.request(http.MethodPost, "/api/admin/hris/attendance/clock-in", f.token,
		map[string]any{"employeeId": f.employeeID, "at": at(f.studio, today, 9, 0)})
	if status != http.StatusConflict {
		t.Fatalf("a second clock-in should conflict, got %d: %v", status, refused)
	}
	if code := errorCode(refused); code != "ALREADY_CLOCKED_IN" {
		t.Fatalf("want ALREADY_CLOCKED_IN, got %q", code)
	}
}

func TestWorkHoursExcludeTheBreak(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0004")
	today := time.Now().In(f.studio)

	h.request(http.MethodPost, "/api/admin/hris/attendance/clock-in", f.token,
		map[string]any{"employeeId": f.employeeID, "at": at(f.studio, today, 8, 0)})
	status, closed := h.request(http.MethodPost, "/api/admin/hris/attendance/clock-out", f.token,
		map[string]any{"employeeId": f.employeeID, "at": at(f.studio, today, 16, 0)})
	if status != http.StatusOK {
		t.Fatalf("clocking out returned %d: %v", status, closed)
	}
	// Eight hours on the clock, one of them an unpaid break.
	if hours := closed["workHours"].(float64); hours != 7 {
		t.Fatalf("want 7 work hours, got %v", hours)
	}

	status, again := h.request(http.MethodPost, "/api/admin/hris/attendance/clock-out", f.token,
		map[string]any{"employeeId": f.employeeID, "at": at(f.studio, today, 17, 0)})
	if status != http.StatusConflict || errorCode(again) != "ALREADY_CLOCKED_OUT" {
		t.Fatalf("closing a closed day should conflict, got %d: %v", status, again)
	}
}

func TestRosterTellsRestingFromUnscheduled(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0005")
	today := time.Now().In(f.studio)
	date := today.Format("2006-01-02")

	// A person with no pattern at all is unscheduled, not resting.
	joined := today.AddDate(-1, 0, 0).Format("2006-01-02")
	status, drifter := h.request(http.MethodPost, "/api/admin/hris/employees", f.token, map[string]any{
		"fullName": "No Pattern", "employeeNumber": "TS-0006", "email": "nopattern@nuhabit.id",
		"phone": "+628110009998", "joinDate": joined, "employmentStatusCode": "PERMANENT",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating an employee returned %d: %v", status, drifter)
	}
	drifterID := drifter["id"].(string)

	// A rest day is an explicit pattern row with no shift.
	status, resting := h.request(http.MethodPost, "/api/admin/hris/employees", f.token, map[string]any{
		"fullName": "Resting", "employeeNumber": "TS-0007", "email": "resting@nuhabit.id",
		"phone": "+628110009997", "joinDate": joined, "employmentStatusCode": "PERMANENT",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating an employee returned %d: %v", status, resting)
	}
	restingID := resting["id"].(string)
	for day := 1; day <= 7; day++ {
		h.request(http.MethodPost, "/api/admin/hris/employees/"+restingID+"/schedule", f.token,
			map[string]any{"dayOfWeek": day, "effectiveFrom": joined})
	}

	status, roster := h.requestList(http.MethodGet, "/api/admin/hris/roster?date="+date, f.token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the roster returned %d", status)
	}
	statuses := map[string]string{}
	for _, entry := range roster {
		statuses[entry["employeeId"].(string)] = entry["status"].(string)
	}
	if statuses[f.employeeID] != "EXPECTED" && statuses[f.employeeID] != "MISSING" {
		t.Fatalf("a rostered person is expected or missing, got %q", statuses[f.employeeID])
	}
	if statuses[restingID] != "REST_DAY" {
		t.Fatalf("a pattern row with no shift is a rest day, got %q", statuses[restingID])
	}
	if statuses[drifterID] != "UNSCHEDULED" {
		t.Fatalf("no pattern at all is unscheduled, got %q", statuses[drifterID])
	}
}

func TestLeaveSkipsHolidaysAndMovesTheBalanceOnApproval(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0010")

	// Anchor on the next Monday so the five-day window is always Monday to
	// Friday, whatever day the suite runs.
	today := time.Now().In(f.studio)
	monday := today.AddDate(0, 0, (8-int(today.Weekday())%7)%7)
	if monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, (8-int(monday.Weekday())%7)%7)
	}
	day := func(offset int) string { return monday.AddDate(0, 0, offset).Format("2006-01-02") }

	// Wednesday carries both a public holiday and a collective leave day. The
	// national one wins, which is both the legally correct reading and the one
	// that favours the employee.
	for _, holiday := range []map[string]any{
		{"date": day(2), "name": "Test National Holiday", "type": "NATIONAL", "deductsLeave": false},
		{"date": day(2), "name": "Test Collective Leave", "type": "COLLECTIVE", "deductsLeave": true},
	} {
		if status, body := h.request(http.MethodPost, "/api/admin/hris/holidays", f.token, holiday); status != http.StatusCreated {
			t.Fatalf("creating a holiday returned %d: %v", status, body)
		}
	}

	status, leave := h.request(http.MethodPost, "/api/admin/hris/leaves", f.token, map[string]any{
		"employeeId": f.employeeID, "type": "ANNUAL",
		"startDate": day(0), "endDate": day(4), "reason": "Year-end break.",
	})
	if status != http.StatusCreated {
		t.Fatalf("filing leave returned %d: %v", status, leave)
	}
	if days := leave["totalDays"].(float64); days != 4 {
		t.Fatalf("five weekdays minus one public holiday is 4 days, got %v", days)
	}
	if leave["status"] != "PENDING" {
		t.Fatalf("a new request is pending, got %v", leave["status"])
	}
	leaveID := leave["id"].(string)

	// Nothing has been deducted yet.
	_, balance := h.request(http.MethodGet,
		"/api/admin/hris/employees/"+f.employeeID+"/balance", f.token, nil)
	if used := balance["annualUsed"].(float64); used != 0 {
		t.Fatalf("a pending request deducts nothing, got %v used", used)
	}

	// But it is reserved: a second request that would together overspend the
	// twelve-day allowance is refused.
	status, greedy := h.request(http.MethodPost, "/api/admin/hris/leaves", f.token, map[string]any{
		"employeeId": f.employeeID, "type": "ANNUAL",
		"startDate": day(35), "endDate": day(48), "reason": "Too much.",
	})
	if status != http.StatusConflict || errorCode(greedy) != "INSUFFICIENT_BALANCE" {
		t.Fatalf("want INSUFFICIENT_BALANCE, got %d: %v", status, greedy)
	}

	// Overlaps are caught whatever the balance says.
	status, clash := h.request(http.MethodPost, "/api/admin/hris/leaves", f.token, map[string]any{
		"employeeId": f.employeeID, "type": "ANNUAL",
		"startDate": day(3), "endDate": day(3), "reason": "Clash.",
	})
	if status != http.StatusConflict || errorCode(clash) != "OVERLAPS_EXISTING" {
		t.Fatalf("want OVERLAPS_EXISTING, got %d: %v", status, clash)
	}

	status, approved := h.request(http.MethodPost,
		"/api/admin/hris/leaves/"+leaveID+"/approve", f.token, map[string]any{})
	if status != http.StatusOK || approved["status"] != "APPROVED" {
		t.Fatalf("approving returned %d: %v", status, approved)
	}
	_, balance = h.request(http.MethodGet,
		"/api/admin/hris/employees/"+f.employeeID+"/balance", f.token, nil)
	if used := balance["annualUsed"].(float64); used != 4 {
		t.Fatalf("approval deducts the days: want 4 used, got %v", used)
	}

	// Cancelling an approved request hands the allowance back.
	status, cancelled := h.request(http.MethodPost,
		"/api/admin/hris/leaves/"+leaveID+"/cancel", f.token, map[string]any{})
	if status != http.StatusOK || cancelled["status"] != "CANCELLED" {
		t.Fatalf("cancelling returned %d: %v", status, cancelled)
	}
	_, balance = h.request(http.MethodGet,
		"/api/admin/hris/employees/"+f.employeeID+"/balance", f.token, nil)
	if used := balance["annualUsed"].(float64); used != 0 {
		t.Fatalf("a cancelled leave returns the days: want 0 used, got %v", used)
	}
}

func TestCollectiveLeaveStillCostsADay(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0011")

	today := time.Now().In(f.studio)
	monday := today.AddDate(0, 0, (8-int(today.Weekday())%7)%7)
	if monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, (8-int(monday.Weekday())%7)%7)
	}
	date := monday.AddDate(0, 0, 7).Format("2006-01-02")

	if status, body := h.request(http.MethodPost, "/api/admin/hris/holidays", f.token, map[string]any{
		"date": date, "name": "Cuti Bersama Test", "type": "COLLECTIVE", "deductsLeave": true,
	}); status != http.StatusCreated {
		t.Fatalf("creating a holiday returned %d: %v", status, body)
	}

	status, leave := h.request(http.MethodPost, "/api/admin/hris/leaves", f.token, map[string]any{
		"employeeId": f.employeeID, "type": "ANNUAL",
		"startDate": date, "endDate": date, "reason": "Collective leave.",
	})
	if status != http.StatusCreated {
		t.Fatalf("filing leave returned %d: %v", status, leave)
	}
	// Collective leave is drawn from the allowance; a public holiday is not.
	if days := leave["totalDays"].(float64); days != 1 {
		t.Fatalf("collective leave costs a day: want 1, got %v", days)
	}
}

func TestSickLeaveIsRecordedNotRationed(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0012")

	today := time.Now().In(f.studio)
	monday := today.AddDate(0, 0, (8-int(today.Weekday())%7)%7)
	start := monday.Format("2006-01-02")
	end := monday.AddDate(0, 0, 25).Format("2006-01-02")

	// Far more days than the annual allowance. Somebody who is ill is ill.
	status, leave := h.request(http.MethodPost, "/api/admin/hris/leaves", f.token, map[string]any{
		"employeeId": f.employeeID, "type": "SICK",
		"startDate": start, "endDate": end, "reason": "Doctor ordered rest.",
	})
	if status != http.StatusCreated {
		t.Fatalf("sick leave is never refused for balance, got %d: %v", status, leave)
	}
	if days := leave["totalDays"].(float64); days <= 12 {
		t.Fatalf("expected more than the annual allowance, got %v days", days)
	}
}

func TestOvertimeLandsOnTheTimesheetAndComesBackOff(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0020")
	today := time.Now().In(f.studio)
	date := today.Format("2006-01-02")

	h.request(http.MethodPost, "/api/admin/hris/attendance/clock-in", f.token,
		map[string]any{"employeeId": f.employeeID, "at": at(f.studio, today, 8, 0)})

	// 22:00 to 01:00 rolls past midnight: three hours, not minus twenty-one.
	status, claim := h.request(http.MethodPost, "/api/admin/hris/overtime", f.token, map[string]any{
		"employeeId": f.employeeID, "date": date,
		"startTime": "22:00", "endTime": "01:00", "source": "COMPANY", "reason": "Event teardown.",
	})
	if status != http.StatusCreated {
		t.Fatalf("claiming overtime returned %d: %v", status, claim)
	}
	if hours := claim["hours"].(float64); hours != 3 {
		t.Fatalf("want 3 hours across midnight, got %v", hours)
	}
	claimID := claim["id"].(string)

	h.request(http.MethodPost, "/api/admin/hris/overtime/"+claimID+"/approve", f.token, map[string]any{})
	if hours := h.overtimeOnTimesheet(t, f, date); hours != 3 {
		t.Fatalf("approved overtime belongs on the timesheet: want 3, got %v", hours)
	}

	h.request(http.MethodPost, "/api/admin/hris/overtime/"+claimID+"/cancel", f.token, map[string]any{})
	if hours := h.overtimeOnTimesheet(t, f, date); hours != 0 {
		t.Fatalf("a cancelled claim leaves nothing behind: want 0, got %v", hours)
	}

	status, late := h.request(http.MethodPost,
		"/api/admin/hris/overtime/"+claimID+"/reject", f.token, map[string]any{"reason": "too late"})
	if status != http.StatusConflict || errorCode(late) != "INVALID_TRANSITION" {
		t.Fatalf("a cancelled claim cannot be rejected, got %d: %v", status, late)
	}
}

func (h *harness) overtimeOnTimesheet(t *testing.T, f hrisFixture, date string) float64 {
	t.Helper()
	path := fmt.Sprintf("/api/admin/hris/attendance?employeeId=%s&from=%s&to=%s", f.employeeID, date, date)
	status, rows := h.requestList(http.MethodGet, path, f.token, nil)
	if status != http.StatusOK || len(rows) != 1 {
		t.Fatalf("reading the timesheet returned %d with %d rows", status, len(rows))
	}
	return rows[0]["overtimeHours"].(float64)
}

func TestHRISPermissionsAreEnforcedServerSide(t *testing.T) {
	h := newHarness(t)

	// An employee record holds a home address, a bank account and next of kin.
	// The front desk has no business in one, and neither does a coach.
	for _, user := range []string{"adm_desk", "adm_coach"} {
		token := h.adminToken(user)
		if status, body := h.requestList(http.MethodGet, "/api/admin/hris/employees", token, nil); status != http.StatusForbidden {
			t.Fatalf("%s should not read the staff directory, got %d: %v", user, status, body)
		}
	}

	// Finance may look, for payroll, but may not decide leave.
	finance := h.adminToken("adm_finance")
	if status, _ := h.requestList(http.MethodGet, "/api/admin/hris/employees", finance, nil); status != http.StatusOK {
		t.Fatalf("finance should read the directory, got %d", status)
	}
	status, refused := h.request(http.MethodPost,
		"/api/admin/hris/leaves/lve_missing/approve", finance, map[string]any{})
	if status != http.StatusForbidden {
		t.Fatalf("finance should not approve leave, got %d: %v", status, refused)
	}

	// A branch manager may, and gets as far as the missing record.
	manager := h.adminToken("adm_branch")
	if status, _ := h.request(http.MethodPost,
		"/api/admin/hris/leaves/lve_missing/approve", manager, map[string]any{}); status != http.StatusNotFound {
		t.Fatalf("a branch manager approves leave; want 404 for a missing one, got %d", status)
	}

	// But anybody signed in may look at their own working day.
	for _, user := range []string{"adm_desk", "adm_coach", "adm_finance"} {
		token := h.adminToken(user)
		if status, body := h.request(http.MethodGet, "/api/admin/hris/me", token, nil); status != http.StatusOK {
			t.Fatalf("%s should see their own day, got %d: %v", user, status, body)
		}
	}
}

func TestLeaveAllowanceCannotBeOverspentByTheDatabase(t *testing.T) {
	h := newHarness(t)
	f := h.newEmployee(t, "TS-0030")

	// The application refuses first. This asserts the refusal is still true
	// when the application is bypassed entirely.
	_, err := h.app.DB.Exec(t.Context(), `
		UPDATE hris.leave_balances SET annual_used = annual_total + 1
		WHERE employee_id = $1`, f.employeeID)
	if err == nil {
		t.Fatal("the database must refuse an overspent allowance")
	}
}
