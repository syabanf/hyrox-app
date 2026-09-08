package hris

import (
	"net/http"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the HR surface. Everything lives under /api/admin/hris:
// employee records carry addresses, bank accounts and next of kin, so there is
// no public or member-facing route here at all.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }
	view := admin(domain.PermHRISView)
	manage := admin(domain.PermHRISManage)
	attendance := admin(domain.PermHRISAttendance)
	approve := admin(domain.PermHRISApprove)

	r.Get("/api/admin/hris/overview", h.overview, view)

	// Anyone with a staff login may look at their own working day and punch
	// their own clock, which needs no HR permission at all.
	r.Get("/api/admin/hris/me", h.me, h.guard.RequireAnyAdmin)
	r.Post("/api/admin/hris/me/clock-in", h.clockInSelf, h.guard.RequireAnyAdmin)
	r.Post("/api/admin/hris/me/clock-out", h.clockOutSelf, h.guard.RequireAnyAdmin)

	r.Get("/api/admin/hris/departments", h.listDepartments, view)
	r.Post("/api/admin/hris/departments", h.createDepartment, manage)
	r.Put("/api/admin/hris/departments/{id}", h.updateDepartment, manage)
	r.Delete("/api/admin/hris/departments/{id}", h.deleteDepartment, manage)

	r.Get("/api/admin/hris/positions", h.listPositions, view)
	r.Post("/api/admin/hris/positions", h.createPosition, manage)
	r.Put("/api/admin/hris/positions/{id}", h.updatePosition, manage)
	r.Delete("/api/admin/hris/positions/{id}", h.deletePosition, manage)

	r.Get("/api/admin/hris/employment-statuses", h.listEmploymentStatuses, view)

	r.Get("/api/admin/hris/employees", h.listEmployees, view)
	r.Post("/api/admin/hris/employees", h.createEmployee, manage)
	r.Get("/api/admin/hris/employees/{id}", h.getEmployee, view)
	r.Put("/api/admin/hris/employees/{id}", h.updateEmployee, manage)

	r.Get("/api/admin/hris/employees/{id}/schedule", h.getSchedule, view)
	r.Post("/api/admin/hris/employees/{id}/schedule", h.assignShift, manage)
	r.Delete("/api/admin/hris/employees/{id}/schedule/{rowId}", h.removeScheduleRow, manage)

	r.Get("/api/admin/hris/employees/{id}/balance", h.getBalance, view)
	r.Put("/api/admin/hris/employees/{id}/balance", h.setBalance, manage)

	r.Get("/api/admin/hris/shifts", h.listShifts, view)
	r.Post("/api/admin/hris/shifts", h.createShift, manage)
	r.Put("/api/admin/hris/shifts/{id}", h.updateShift, manage)

	r.Get("/api/admin/hris/roster", h.roster, view)
	r.Get("/api/admin/hris/attendance", h.listAttendance, view)
	r.Post("/api/admin/hris/attendance/clock-in", h.clockIn, attendance)
	r.Post("/api/admin/hris/attendance/clock-out", h.clockOut, attendance)
	r.Post("/api/admin/hris/attendance/mark", h.markAttendance, attendance)

	r.Get("/api/admin/hris/leaves", h.listLeaves, view)
	r.Post("/api/admin/hris/leaves", h.requestLeave, attendance)
	r.Post("/api/admin/hris/leaves/{id}/{action}", h.decideLeave, approve)

	r.Get("/api/admin/hris/overtime", h.listOvertime, view)
	r.Post("/api/admin/hris/overtime", h.requestOvertime, attendance)
	r.Post("/api/admin/hris/overtime/{id}/{action}", h.decideOvertime, approve)

	r.Get("/api/admin/hris/holidays", h.listHolidays, view)
	r.Post("/api/admin/hris/holidays", h.createHoliday, manage)
	r.Delete("/api/admin/hris/holidays/{id}", h.deleteHoliday, manage)
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

// parseDate reads an optional date from the query string.
func parseDate(r *http.Request, key string) (domain.Date, error) {
	raw := httpx.Query(r, key)
	if raw == "" {
		return "", nil
	}
	parsed, err := domain.ParseDate(raw)
	if err != nil {
		return "", httpx.Invalid("%s must be a date as YYYY-MM-DD.", key)
	}
	return parsed, nil
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.service.Overview(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, overview)
}

// ── Self service ─────────────────────────────────────────────────────────────

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Admin(r.Context())
	view, err := h.service.Me(r.Context(), principal.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

type punchRequest struct {
	Notes *string `json:"notes"`
}

func (h *Handler) clockInSelf(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[punchRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	principal, _ := auth.Admin(r.Context())
	view, err := h.service.ClockInSelf(r.Context(), principal.ID, body.Notes, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, view)
}

func (h *Handler) clockOutSelf(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[punchRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	principal, _ := auth.Admin(r.Context())
	view, err := h.service.ClockOutSelf(r.Context(), principal.ID, body.Notes, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

// ── Organization ─────────────────────────────────────────────────────────────

func (h *Handler) listDepartments(w http.ResponseWriter, r *http.Request) {
	departments, err := h.service.Departments(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, departments)
}

type departmentRequest struct {
	Name               string  `json:"name"`
	Code               string  `json:"code"`
	Description        string  `json:"description"`
	ParentDepartmentID *string `json:"parentDepartmentId"`
	CostCenter         *string `json:"costCenter"`
	Active             *bool   `json:"active"`
}

func (d *departmentRequest) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return httpx.Invalid("A department needs a name.")
	}
	if strings.TrimSpace(d.Code) == "" {
		return httpx.Invalid("A department needs a code.")
	}
	return nil
}

func (d departmentRequest) toInput() DepartmentInput {
	return DepartmentInput{
		Name: strings.TrimSpace(d.Name), Code: strings.ToUpper(strings.TrimSpace(d.Code)),
		Description: d.Description, ParentDepartmentID: d.ParentDepartmentID,
		CostCenter: d.CostCenter, Active: d.Active == nil || *d.Active,
	}
}

func (h *Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[departmentRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := h.service.CreateDepartment(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) updateDepartment(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[departmentRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := h.service.UpdateDepartment(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, updated)
}

func (h *Handler) deleteDepartment(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteDepartment(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

func (h *Handler) listPositions(w http.ResponseWriter, r *http.Request) {
	positions, err := h.service.Positions(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, positions)
}

type positionRequest struct {
	Title  string `json:"title"`
	Level  string `json:"level"`
	Active *bool  `json:"active"`
}

func (p *positionRequest) Validate() error {
	if strings.TrimSpace(p.Title) == "" {
		return httpx.Invalid("A position needs a title.")
	}
	return nil
}

func (p positionRequest) toInput() PositionInput {
	return PositionInput{
		Title: strings.TrimSpace(p.Title), Level: strings.TrimSpace(p.Level),
		Active: p.Active == nil || *p.Active,
	}
}

func (h *Handler) createPosition(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[positionRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := h.service.CreatePosition(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) updatePosition(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[positionRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := h.service.UpdatePosition(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, updated)
}

func (h *Handler) deletePosition(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeletePosition(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

func (h *Handler) listEmploymentStatuses(w http.ResponseWriter, r *http.Request) {
	statuses, err := h.service.EmploymentStatuses(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, statuses)
}

// ── People ───────────────────────────────────────────────────────────────────

func (h *Handler) listEmployees(w http.ResponseWriter, r *http.Request) {
	employees, err := h.service.Directory(r.Context(), EmployeeFilter{
		Query:        httpx.Query(r, "query"),
		DepartmentID: httpx.Query(r, "departmentId"),
		BranchID:     httpx.Query(r, "branchId"),
		ActiveOnly:   httpx.QueryBool(r, "activeOnly", false),
		Limit:        httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, employees)
}

func (h *Handler) getEmployee(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.EmployeeDetail(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, detail)
}

// employeeRequest is the whole record. An employee is edited as a form, and a
// partial write of a person carrying bank details and next of kin is a good
// way to silently drop one of them.
type employeeRequest struct {
	FullName                 string                `json:"fullName"`
	EmployeeNumber           string                `json:"employeeNumber"`
	Email                    string                `json:"email"`
	Phone                    string                `json:"phone"`
	BirthDate                *string               `json:"birthDate"`
	Gender                   *domain.Gender        `json:"gender"`
	MaritalStatus            *domain.MaritalStatus `json:"maritalStatus"`
	Address                  *string               `json:"address"`
	JoinDate                 string                `json:"joinDate"`
	EndDate                  *string               `json:"endDate"`
	EmploymentStatusCode     string                `json:"employmentStatusCode"`
	Active                   *bool                 `json:"active"`
	DepartmentID             *string               `json:"departmentId"`
	PositionID               *string               `json:"positionId"`
	BranchID                 *string               `json:"branchId"`
	ReportingTo              *string               `json:"reportingTo"`
	CoachID                  *string               `json:"coachId"`
	AdminUserID              *string               `json:"adminUserId"`
	BankName                 *string               `json:"bankName"`
	BankAccount              *string               `json:"bankAccount"`
	EmergencyContactName     *string               `json:"emergencyContactName"`
	EmergencyContactPhone    *string               `json:"emergencyContactPhone"`
	EmergencyContactRelation *string               `json:"emergencyContactRelation"`
	PhotoURL                 *string               `json:"photoUrl"`
	Notes                    *string               `json:"notes"`
}

func (e *employeeRequest) Validate() error {
	if strings.TrimSpace(e.FullName) == "" {
		return httpx.Invalid("An employee needs a name.")
	}
	if strings.TrimSpace(e.EmployeeNumber) == "" {
		return httpx.Invalid("An employee needs a number.")
	}
	if strings.TrimSpace(e.EmploymentStatusCode) == "" {
		return httpx.Invalid("An employment status is required.")
	}
	if _, err := domain.ParseDate(e.JoinDate); err != nil {
		return httpx.Invalid("A join date as YYYY-MM-DD is required.")
	}
	for label, value := range map[string]*string{"birthDate": e.BirthDate, "endDate": e.EndDate} {
		if value == nil || *value == "" {
			continue
		}
		if _, err := domain.ParseDate(*value); err != nil {
			return httpx.Invalid("%s must be a date as YYYY-MM-DD.", label)
		}
	}
	return nil
}

// optionalDateInput turns an absent or blank string into no date at all, so
// clearing a field in the form clears the column.
func optionalDateInput(raw *string) *domain.Date {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	parsed, err := domain.ParseDate(*raw)
	if err != nil {
		return nil
	}
	return &parsed
}

func (e employeeRequest) toInput() EmployeeInput {
	join, _ := domain.ParseDate(e.JoinDate)
	return EmployeeInput{
		FullName: e.FullName, EmployeeNumber: e.EmployeeNumber, Email: e.Email, Phone: e.Phone,
		BirthDate: optionalDateInput(e.BirthDate), Gender: e.Gender, MaritalStatus: e.MaritalStatus,
		Address: e.Address, JoinDate: join, EndDate: optionalDateInput(e.EndDate),
		EmploymentStatusCode: strings.ToUpper(strings.TrimSpace(e.EmploymentStatusCode)),
		Active:               e.Active == nil || *e.Active,
		DepartmentID:         e.DepartmentID, PositionID: e.PositionID, BranchID: e.BranchID,
		ReportingTo: e.ReportingTo, CoachID: e.CoachID, AdminUserID: e.AdminUserID,
		BankName: e.BankName, BankAccount: e.BankAccount,
		EmergencyContactName: e.EmergencyContactName, EmergencyContactPhone: e.EmergencyContactPhone,
		EmergencyContactRelation: e.EmergencyContactRelation,
		PhotoURL:                 e.PhotoURL, Notes: e.Notes,
	}
}

func (h *Handler) createEmployee(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[employeeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := h.service.CreateEmployee(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[employeeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := h.service.UpdateEmployee(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, updated)
}

// ── Schedules ────────────────────────────────────────────────────────────────

func (h *Handler) getSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, err := h.service.Schedule(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, schedule)
}

type scheduleRequest struct {
	DayOfWeek     int     `json:"dayOfWeek"`
	ShiftID       *string `json:"shiftId"`
	EffectiveFrom string  `json:"effectiveFrom"`
	EffectiveTo   *string `json:"effectiveTo"`
}

func (s *scheduleRequest) Validate() error {
	if s.DayOfWeek < 1 || s.DayOfWeek > 7 {
		return httpx.Invalid("A weekday runs from 1 (Monday) to 7 (Sunday).")
	}
	if _, err := domain.ParseDate(s.EffectiveFrom); err != nil {
		return httpx.Invalid("effectiveFrom must be a date as YYYY-MM-DD.")
	}
	if s.EffectiveTo != nil && *s.EffectiveTo != "" {
		if _, err := domain.ParseDate(*s.EffectiveTo); err != nil {
			return httpx.Invalid("effectiveTo must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) assignShift(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[scheduleRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	from, _ := domain.ParseDate(body.EffectiveFrom)
	row, err := h.service.AssignShift(r.Context(), httpx.Param(r, "id"), ScheduleInput{
		DayOfWeek: body.DayOfWeek, ShiftID: body.ShiftID,
		EffectiveFrom: from, EffectiveTo: optionalDateInput(body.EffectiveTo),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, row)
}

func (h *Handler) removeScheduleRow(w http.ResponseWriter, r *http.Request) {
	err := h.service.RemoveScheduleRow(r.Context(), httpx.Param(r, "id"), httpx.Param(r, "rowId"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

// ── Shifts ───────────────────────────────────────────────────────────────────

func (h *Handler) listShifts(w http.ResponseWriter, r *http.Request) {
	shifts, err := h.service.Shifts(r.Context(), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, shifts)
}

type shiftRequest struct {
	Name                 string `json:"name"`
	StartTime            string `json:"startTime"`
	EndTime              string `json:"endTime"`
	BreakMinutes         int    `json:"breakMinutes"`
	LateToleranceMinutes int    `json:"lateToleranceMinutes"`
	IsOvernight          bool   `json:"isOvernight"`
	Active               *bool  `json:"active"`
	SortOrder            int    `json:"sortOrder"`
}

func (s *shiftRequest) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return httpx.Invalid("A shift needs a name.")
	}
	if _, err := domain.ParseTimeOfDay(s.StartTime); err != nil {
		return httpx.Invalid("startTime must be HH:MM.")
	}
	if _, err := domain.ParseTimeOfDay(s.EndTime); err != nil {
		return httpx.Invalid("endTime must be HH:MM.")
	}
	if s.BreakMinutes < 0 || s.LateToleranceMinutes < 0 {
		return httpx.Invalid("Break and tolerance minutes cannot be negative.")
	}
	return nil
}

func (s shiftRequest) toInput() ShiftInput {
	start, _ := domain.ParseTimeOfDay(s.StartTime)
	end, _ := domain.ParseTimeOfDay(s.EndTime)
	return ShiftInput{
		Name: strings.TrimSpace(s.Name), StartTime: start, EndTime: end,
		BreakMinutes: s.BreakMinutes, LateToleranceMinutes: s.LateToleranceMinutes,
		IsOvernight: s.IsOvernight, Active: s.Active == nil || *s.Active, SortOrder: s.SortOrder,
	}
}

func (h *Handler) createShift(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[shiftRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := h.service.CreateShift(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) updateShift(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[shiftRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := h.service.UpdateShift(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, updated)
}

// ── Attendance ───────────────────────────────────────────────────────────────

func (h *Handler) roster(w http.ResponseWriter, r *http.Request) {
	date, err := parseDate(r, "date")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entries, err := h.service.Roster(r.Context(), date, httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, entries)
}

func (h *Handler) listAttendance(w http.ResponseWriter, r *http.Request) {
	from, err := parseDate(r, "from")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	to, err := parseDate(r, "to")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := h.service.Attendances(r.Context(), AttendanceFilter{
		EmployeeID: httpx.Query(r, "employeeId"),
		From:       from, To: to,
		Status: httpx.Query(r, "status"),
		Limit:  httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, rows)
}

type clockRequest struct {
	EmployeeID string  `json:"employeeId"`
	At         *string `json:"at"`
	Notes      *string `json:"notes"`
	Remote     bool    `json:"remote"`
}

func (c *clockRequest) Validate() error {
	if strings.TrimSpace(c.EmployeeID) == "" {
		return httpx.Invalid("An employee is required.")
	}
	if c.At != nil && *c.At != "" {
		if _, err := time.Parse(time.RFC3339, *c.At); err != nil {
			return httpx.Invalid("at must be an RFC 3339 timestamp.")
		}
	}
	return nil
}

func (c clockRequest) toInput() ClockInput {
	in := ClockInput{EmployeeID: c.EmployeeID, Notes: c.Notes, Remote: c.Remote}
	if c.At != nil && *c.At != "" {
		if parsed, err := time.Parse(time.RFC3339, *c.At); err == nil {
			in.At = &parsed
		}
	}
	return in
}

func (h *Handler) clockIn(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[clockRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	view, err := h.service.ClockIn(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, view)
}

func (h *Handler) clockOut(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[clockRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	view, err := h.service.ClockOut(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

type markRequest struct {
	EmployeeID string  `json:"employeeId"`
	Date       string  `json:"date"`
	Status     string  `json:"status"`
	ClockIn    *string `json:"clockIn"`
	ClockOut   *string `json:"clockOut"`
	Notes      *string `json:"notes"`
}

func (m *markRequest) Validate() error {
	if strings.TrimSpace(m.EmployeeID) == "" {
		return httpx.Invalid("An employee is required.")
	}
	if _, err := domain.ParseDate(m.Date); err != nil {
		return httpx.Invalid("A date as YYYY-MM-DD is required.")
	}
	if strings.TrimSpace(m.Status) == "" {
		return httpx.Invalid("A status is required.")
	}
	for label, value := range map[string]*string{"clockIn": m.ClockIn, "clockOut": m.ClockOut} {
		if value == nil || *value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, *value); err != nil {
			return httpx.Invalid("%s must be an RFC 3339 timestamp.", label)
		}
	}
	return nil
}

func optionalTime(raw *string) *time.Time {
	if raw == nil || *raw == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil
	}
	return &parsed
}

func (h *Handler) markAttendance(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[markRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	date, _ := domain.ParseDate(body.Date)
	view, err := h.service.MarkAttendance(r.Context(), MarkInput{
		EmployeeID: body.EmployeeID, Date: date,
		Status:  domain.AttendanceStatus(strings.ToUpper(body.Status)),
		ClockIn: optionalTime(body.ClockIn), ClockOut: optionalTime(body.ClockOut),
		Notes: body.Notes,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

// ── Leave ────────────────────────────────────────────────────────────────────

func (h *Handler) listLeaves(w http.ResponseWriter, r *http.Request) {
	from, err := parseDate(r, "from")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	to, err := parseDate(r, "to")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	leaves, err := h.service.Leaves(r.Context(), LeaveFilter{
		EmployeeID: httpx.Query(r, "employeeId"),
		Status:     strings.ToUpper(httpx.Query(r, "status")),
		Type:       strings.ToUpper(httpx.Query(r, "type")),
		From:       from, To: to,
		Limit: httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, leaves)
}

type leaveRequestBody struct {
	EmployeeID    string  `json:"employeeId"`
	Type          string  `json:"type"`
	StartDate     string  `json:"startDate"`
	EndDate       string  `json:"endDate"`
	Reason        string  `json:"reason"`
	AttachmentURL *string `json:"attachmentUrl"`
}

func (l *leaveRequestBody) Validate() error {
	if strings.TrimSpace(l.EmployeeID) == "" {
		return httpx.Invalid("An employee is required.")
	}
	if !domain.IsValidLeaveType(strings.ToUpper(l.Type)) {
		return httpx.Invalid("%q is not a leave type.", l.Type)
	}
	if _, err := domain.ParseDate(l.StartDate); err != nil {
		return httpx.Invalid("startDate must be a date as YYYY-MM-DD.")
	}
	if _, err := domain.ParseDate(l.EndDate); err != nil {
		return httpx.Invalid("endDate must be a date as YYYY-MM-DD.")
	}
	if strings.TrimSpace(l.Reason) == "" {
		return httpx.Invalid("A reason is required.")
	}
	return nil
}

func (h *Handler) requestLeave(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[leaveRequestBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	start, _ := domain.ParseDate(body.StartDate)
	end, _ := domain.ParseDate(body.EndDate)
	view, err := h.service.RequestLeave(r.Context(), LeaveInput{
		EmployeeID: body.EmployeeID, Type: domain.LeaveType(strings.ToUpper(body.Type)),
		StartDate: start, EndDate: end,
		Reason: strings.TrimSpace(body.Reason), AttachmentURL: body.AttachmentURL,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, view)
}

type decisionRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) decideLeave(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[decisionRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	leaveID := httpx.Param(r, "id")
	actor := actorFrom(r)

	var view LeaveView
	switch strings.ToLower(httpx.Param(r, "action")) {
	case "approve":
		view, err = h.service.DecideLeave(r.Context(), leaveID, true, body.Reason, actor)
	case "reject":
		view, err = h.service.DecideLeave(r.Context(), leaveID, false, body.Reason, actor)
	case "cancel":
		view, err = h.service.CancelLeave(r.Context(), leaveID, actor)
	default:
		err = httpx.Invalid("A leave request is approved, rejected or cancelled.")
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request) {
	balance, err := h.service.Balance(r.Context(), httpx.Param(r, "id"), httpx.QueryInt(r, "year", 0))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, balance)
}

type balanceRequest struct {
	Year        int     `json:"year"`
	AnnualTotal float64 `json:"annualTotal"`
}

func (b *balanceRequest) Validate() error {
	if b.AnnualTotal < 0 {
		return httpx.Invalid("An allowance cannot be negative.")
	}
	return nil
}

func (h *Handler) setBalance(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[balanceRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	balance, err := h.service.SetAnnualTotal(r.Context(), httpx.Param(r, "id"), body.Year, body.AnnualTotal, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, balance)
}

// ── Overtime ─────────────────────────────────────────────────────────────────

func (h *Handler) listOvertime(w http.ResponseWriter, r *http.Request) {
	requests, err := h.service.Overtime(r.Context(),
		httpx.Query(r, "employeeId"), strings.ToUpper(httpx.Query(r, "status")),
		httpx.QueryInt(r, "limit", 100))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, requests)
}

type overtimeRequestBody struct {
	EmployeeID string  `json:"employeeId"`
	Date       string  `json:"date"`
	StartTime  string  `json:"startTime"`
	EndTime    string  `json:"endTime"`
	Source     string  `json:"source"`
	Reason     *string `json:"reason"`
}

func (o *overtimeRequestBody) Validate() error {
	if strings.TrimSpace(o.EmployeeID) == "" {
		return httpx.Invalid("An employee is required.")
	}
	if _, err := domain.ParseDate(o.Date); err != nil {
		return httpx.Invalid("A date as YYYY-MM-DD is required.")
	}
	if _, err := domain.ParseTimeOfDay(o.StartTime); err != nil {
		return httpx.Invalid("startTime must be HH:MM.")
	}
	if _, err := domain.ParseTimeOfDay(o.EndTime); err != nil {
		return httpx.Invalid("endTime must be HH:MM.")
	}
	return nil
}

func (h *Handler) requestOvertime(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[overtimeRequestBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	date, _ := domain.ParseDate(body.Date)
	start, _ := domain.ParseTimeOfDay(body.StartTime)
	end, _ := domain.ParseTimeOfDay(body.EndTime)

	view, err := h.service.RequestOvertime(r.Context(), OvertimeInput{
		EmployeeID: body.EmployeeID, Date: date, StartTime: start, EndTime: end,
		Source: domain.OvertimeSource(strings.ToUpper(body.Source)), Reason: body.Reason,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, view)
}

func (h *Handler) decideOvertime(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[decisionRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	overtimeID := httpx.Param(r, "id")
	actor := actorFrom(r)

	var view OvertimeView
	switch strings.ToLower(httpx.Param(r, "action")) {
	case "approve":
		view, err = h.service.DecideOvertime(r.Context(), overtimeID, true, body.Reason, actor)
	case "reject":
		view, err = h.service.DecideOvertime(r.Context(), overtimeID, false, body.Reason, actor)
	case "cancel":
		view, err = h.service.CancelOvertime(r.Context(), overtimeID, actor)
	default:
		err = httpx.Invalid("An overtime claim is approved, rejected or cancelled.")
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

// ── Holidays ─────────────────────────────────────────────────────────────────

func (h *Handler) listHolidays(w http.ResponseWriter, r *http.Request) {
	from, err := parseDate(r, "from")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	to, err := parseDate(r, "to")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	holidays, err := h.service.Holidays(r.Context(), from, to, httpx.QueryBool(r, "includeDrafts", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, holidays)
}

type holidayRequest struct {
	Date         string  `json:"date"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	DeductsLeave bool    `json:"deductsLeave"`
	Note         *string `json:"note"`
	Draft        bool    `json:"draft"`
}

func (h *holidayRequest) Validate() error {
	if _, err := domain.ParseDate(h.Date); err != nil {
		return httpx.Invalid("A date as YYYY-MM-DD is required.")
	}
	if strings.TrimSpace(h.Name) == "" {
		return httpx.Invalid("A holiday needs a name.")
	}
	return nil
}

func (h *Handler) createHoliday(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[holidayRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	date, _ := domain.ParseDate(body.Date)
	kind := domain.HolidayType(strings.ToUpper(body.Type))
	if kind == "" {
		kind = domain.HolidayNational
	}
	created, err := h.service.CreateHoliday(r.Context(), HolidayInput{
		Date: date, Name: strings.TrimSpace(body.Name), Type: kind,
		DeductsLeave: body.DeductsLeave, Note: body.Note, Draft: body.Draft,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) deleteHoliday(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteHoliday(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}
