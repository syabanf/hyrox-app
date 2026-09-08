// Package hris is the people side of the studio: who works here, when they are
// expected, whether they turned up, and the leave they are owed.
//
// It is deliberately separate from identity. A member is a customer, an admin
// user is a login, and an employee is somebody on the payroll — the same
// person can be all three, and each of the three ends independently. Someone
// leaving the company keeps their membership; a coach losing their login keeps
// their attendance history.
package hris

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/audit"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// Catalog is the port for the branches employees are posted to and the coaches
// they teach as. HRIS names them; it never writes them.
type Catalog interface {
	Branches(ctx context.Context) ([]domain.Branch, error)
	Coaches(ctx context.Context) ([]domain.Coach, error)
}

// Service implements the HR use cases.
type Service struct {
	db      *database.DB
	repo    *Repository
	catalog Catalog
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
	// studio is the timezone that decides what "today" and "07:00" mean. Every
	// calendar date in this module is resolved in it, never in UTC.
	studio *time.Location
}

func NewService(db *database.DB, repo *Repository, catalog Catalog,
	ids id.Generator, c clock.Clock, auditor audit.Recorder, studio *time.Location) *Service {
	if studio == nil {
		studio = time.UTC
	}
	return &Service{db: db, repo: repo, catalog: catalog, ids: ids, clock: c,
		auditor: auditor, studio: studio}
}

// Actor identifies who performed an administrative action.
type Actor struct {
	ID   string
	Name string
}

// Today is the studio's current calendar date.
func (s *Service) Today() domain.Date { return domain.DateOf(s.clock.Now(), s.studio) }

func (s *Service) record(ctx context.Context, entity, entityID, action string, actor Actor, reason *string) {
	// A failed audit write must not undo the action it describes; the recorder
	// logs its own errors.
	_ = s.auditor.Record(ctx, audit.Event{
		EntityType: entity, EntityID: entityID, Action: action,
		ActorID: actor.ID, ActorName: actor.Name, Reason: reason,
	})
}

// ── Organization ─────────────────────────────────────────────────────────────

func (s *Service) Departments(ctx context.Context) ([]domain.Department, error) {
	return s.repo.Departments(ctx)
}

// DepartmentInput creates or replaces a department.
type DepartmentInput struct {
	Name               string
	Code               string
	Description        string
	ParentDepartmentID *string
	CostCenter         *string
	Active             bool
}

func (s *Service) CreateDepartment(ctx context.Context, in DepartmentInput, actor Actor) (domain.Department, error) {
	if in.ParentDepartmentID != nil {
		if _, err := s.repo.Department(ctx, *in.ParentDepartmentID); err != nil {
			return domain.Department{}, err
		}
	}
	created, err := s.repo.InsertDepartment(ctx, domain.Department{
		ID: s.ids.New(id.Department), Name: in.Name, Code: in.Code,
		Description: in.Description, ParentDepartmentID: in.ParentDepartmentID,
		CostCenter: in.CostCenter, Active: in.Active,
	})
	if err != nil {
		return domain.Department{}, err
	}
	s.record(ctx, "hris.department", created.ID, "CREATE", actor, nil)
	return created, nil
}

func (s *Service) UpdateDepartment(ctx context.Context, departmentID string, in DepartmentInput, actor Actor) (domain.Department, error) {
	if in.ParentDepartmentID != nil {
		if *in.ParentDepartmentID == departmentID {
			return domain.Department{}, httpx.Invalid("A department cannot report to itself.")
		}
		if _, err := s.repo.Department(ctx, *in.ParentDepartmentID); err != nil {
			return domain.Department{}, err
		}
	}
	updated, err := s.repo.UpdateDepartment(ctx, domain.Department{
		ID: departmentID, Name: in.Name, Code: in.Code, Description: in.Description,
		ParentDepartmentID: in.ParentDepartmentID, CostCenter: in.CostCenter, Active: in.Active,
	})
	if err != nil {
		return domain.Department{}, err
	}
	s.record(ctx, "hris.department", departmentID, "UPDATE", actor, nil)
	return updated, nil
}

func (s *Service) DeleteDepartment(ctx context.Context, departmentID string, actor Actor) error {
	if err := s.repo.DeleteDepartment(ctx, departmentID); err != nil {
		return err
	}
	s.record(ctx, "hris.department", departmentID, "DELETE", actor, nil)
	return nil
}

func (s *Service) Positions(ctx context.Context) ([]domain.Position, error) {
	return s.repo.Positions(ctx)
}

// PositionInput creates or replaces a job title.
type PositionInput struct {
	Title  string
	Level  string
	Active bool
}

func (s *Service) CreatePosition(ctx context.Context, in PositionInput, actor Actor) (domain.Position, error) {
	created, err := s.repo.InsertPosition(ctx, domain.Position{
		ID: s.ids.New(id.Position), Title: in.Title, Level: in.Level, Active: in.Active,
	})
	if err != nil {
		return domain.Position{}, err
	}
	s.record(ctx, "hris.position", created.ID, "CREATE", actor, nil)
	return created, nil
}

func (s *Service) UpdatePosition(ctx context.Context, positionID string, in PositionInput, actor Actor) (domain.Position, error) {
	updated, err := s.repo.UpdatePosition(ctx, domain.Position{
		ID: positionID, Title: in.Title, Level: in.Level, Active: in.Active,
	})
	if err != nil {
		return domain.Position{}, err
	}
	s.record(ctx, "hris.position", positionID, "UPDATE", actor, nil)
	return updated, nil
}

func (s *Service) DeletePosition(ctx context.Context, positionID string, actor Actor) error {
	if err := s.repo.DeletePosition(ctx, positionID); err != nil {
		return err
	}
	s.record(ctx, "hris.position", positionID, "DELETE", actor, nil)
	return nil
}

func (s *Service) EmploymentStatuses(ctx context.Context) ([]domain.EmploymentStatus, error) {
	return s.repo.EmploymentStatuses(ctx)
}

// ── People ───────────────────────────────────────────────────────────────────

// EmployeeView is an employee with the things it points at named, so a table
// row does not need five more requests to be readable.
type EmployeeView struct {
	domain.Employee
	DepartmentName  *string `json:"departmentName"`
	PositionTitle   *string `json:"positionTitle"`
	BranchName      *string `json:"branchName"`
	ReportingToName *string `json:"reportingToName"`
	CoachName       *string `json:"coachName"`
}

// namer resolves the ids an employee carries into names.
type namer struct {
	departments map[string]string
	positions   map[string]string
	branches    map[string]string
	coaches     map[string]string
	employees   map[string]string
}

func (s *Service) namer(ctx context.Context) (namer, error) {
	n := namer{
		departments: map[string]string{}, positions: map[string]string{},
		branches: map[string]string{}, coaches: map[string]string{}, employees: map[string]string{},
	}
	departments, err := s.repo.Departments(ctx)
	if err != nil {
		return namer{}, err
	}
	for _, d := range departments {
		n.departments[d.ID] = d.Name
	}
	positions, err := s.repo.Positions(ctx)
	if err != nil {
		return namer{}, err
	}
	for _, p := range positions {
		n.positions[p.ID] = p.Title
	}
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return namer{}, err
	}
	for _, b := range branches {
		n.branches[b.ID] = b.Name
	}
	coaches, err := s.catalog.Coaches(ctx)
	if err != nil {
		return namer{}, err
	}
	for _, c := range coaches {
		n.coaches[c.ID] = c.Name
	}
	// Managers are employees, so the whole roster is the lookup for
	// "reports to". It is a small table by construction.
	employees, err := s.repo.Employees(ctx, EmployeeFilter{})
	if err != nil {
		return namer{}, err
	}
	for _, e := range employees {
		n.employees[e.ID] = e.FullName
	}
	return n, nil
}

func lookup(table map[string]string, key *string) *string {
	if key == nil {
		return nil
	}
	if name, ok := table[*key]; ok {
		return &name
	}
	return nil
}

func (n namer) view(e domain.Employee) EmployeeView {
	return EmployeeView{
		Employee:        e,
		DepartmentName:  lookup(n.departments, e.DepartmentID),
		PositionTitle:   lookup(n.positions, e.PositionID),
		BranchName:      lookup(n.branches, e.BranchID),
		ReportingToName: lookup(n.employees, e.ReportingTo),
		CoachName:       lookup(n.coaches, e.CoachID),
	}
}

// Directory is the staff list.
func (s *Service) Directory(ctx context.Context, filter EmployeeFilter) ([]EmployeeView, error) {
	employees, err := s.repo.Employees(ctx, filter)
	if err != nil {
		return nil, err
	}
	names, err := s.namer(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]EmployeeView, 0, len(employees))
	for _, e := range employees {
		views = append(views, names.view(e))
	}
	return views, nil
}

// EmployeeDetail is everything one person's page shows.
type EmployeeDetail struct {
	Employee      EmployeeView             `json:"employee"`
	Balance       domain.LeaveBalance      `json:"balance"`
	Schedule      []ScheduleRowView        `json:"schedule"`
	Attendance    []domain.Attendance      `json:"attendance"`
	Leaves        []domain.Leave           `json:"leaves"`
	Overtime      []domain.OvertimeRequest `json:"overtime"`
	DirectReports []EmployeeView           `json:"directReports"`
}

// ScheduleRowView is a pattern row with its shift resolved.
type ScheduleRowView struct {
	domain.EmployeeShift
	ShiftName *string `json:"shiftName"`
	DayName   string  `json:"dayName"`
}

var weekdayNames = [8]string{"", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

func dayName(dow int) string {
	if dow < 1 || dow > 7 {
		return ""
	}
	return weekdayNames[dow]
}

func (s *Service) EmployeeDetail(ctx context.Context, employeeID string) (EmployeeDetail, error) {
	employee, err := s.repo.Employee(ctx, employeeID)
	if err != nil {
		return EmployeeDetail{}, err
	}
	names, err := s.namer(ctx)
	if err != nil {
		return EmployeeDetail{}, err
	}

	today := s.Today()
	balance, err := s.repo.Balance(ctx, employeeID, today.Year(), false)
	if err != nil {
		return EmployeeDetail{}, err
	}
	schedule, err := s.scheduleView(ctx, employeeID)
	if err != nil {
		return EmployeeDetail{}, err
	}
	attendance, err := s.repo.Attendances(ctx, AttendanceFilter{EmployeeID: employeeID, Limit: 30})
	if err != nil {
		return EmployeeDetail{}, err
	}
	leaves, err := s.repo.Leaves(ctx, LeaveFilter{EmployeeID: employeeID, Limit: 30})
	if err != nil {
		return EmployeeDetail{}, err
	}
	overtime, err := s.repo.OvertimeRequests(ctx, employeeID, "", 30)
	if err != nil {
		return EmployeeDetail{}, err
	}

	reports := []EmployeeView{}
	all, err := s.repo.Employees(ctx, EmployeeFilter{})
	if err != nil {
		return EmployeeDetail{}, err
	}
	for _, candidate := range all {
		if candidate.ReportingTo != nil && *candidate.ReportingTo == employeeID {
			reports = append(reports, names.view(candidate))
		}
	}

	return EmployeeDetail{
		Employee: names.view(employee), Balance: balance, Schedule: schedule,
		Attendance: attendance, Leaves: leaves, Overtime: overtime, DirectReports: reports,
	}, nil
}

// EmployeeInput is a whole employee record. Updates replace rather than patch:
// the admin form holds the entire person, and a partial write of a record that
// carries bank details and next of kin is a good way to lose one of them.
type EmployeeInput struct {
	FullName                 string
	EmployeeNumber           string
	Email                    string
	Phone                    string
	BirthDate                *domain.Date
	Gender                   *domain.Gender
	MaritalStatus            *domain.MaritalStatus
	Address                  *string
	JoinDate                 domain.Date
	EndDate                  *domain.Date
	EmploymentStatusCode     string
	Active                   bool
	DepartmentID             *string
	PositionID               *string
	BranchID                 *string
	ReportingTo              *string
	CoachID                  *string
	AdminUserID              *string
	BankName                 *string
	BankAccount              *string
	EmergencyContactName     *string
	EmergencyContactPhone    *string
	EmergencyContactRelation *string
	PhotoURL                 *string
	Notes                    *string
}

func (s *Service) validateEmployee(ctx context.Context, in EmployeeInput, selfID string) error {
	if in.EndDate != nil && in.EndDate.Before(in.JoinDate) {
		return httpx.Invalid("An employee cannot leave before they joined.")
	}
	if in.DepartmentID != nil {
		if _, err := s.repo.Department(ctx, *in.DepartmentID); err != nil {
			return err
		}
	}
	if in.ReportingTo != nil {
		if *in.ReportingTo == selfID {
			return httpx.Invalid("An employee cannot report to themselves.")
		}
		if _, err := s.repo.Employee(ctx, *in.ReportingTo); err != nil {
			return err
		}
	}
	if in.BranchID != nil {
		branches, err := s.catalog.Branches(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, b := range branches {
			if b.ID == *in.BranchID {
				found = true
				break
			}
		}
		if !found {
			return httpx.NotFound("branch")
		}
	}
	return nil
}

func (in EmployeeInput) apply(e domain.Employee) domain.Employee {
	e.FullName = strings.TrimSpace(in.FullName)
	e.EmployeeNumber = strings.TrimSpace(in.EmployeeNumber)
	e.Email = strings.ToLower(strings.TrimSpace(in.Email))
	e.Phone = strings.TrimSpace(in.Phone)
	e.BirthDate = in.BirthDate
	e.Gender = in.Gender
	e.MaritalStatus = in.MaritalStatus
	e.Address = in.Address
	e.JoinDate = in.JoinDate
	e.EndDate = in.EndDate
	e.EmploymentStatusCode = in.EmploymentStatusCode
	e.Active = in.Active
	e.DepartmentID = in.DepartmentID
	e.PositionID = in.PositionID
	e.BranchID = in.BranchID
	e.ReportingTo = in.ReportingTo
	e.CoachID = in.CoachID
	e.AdminUserID = in.AdminUserID
	e.BankName = in.BankName
	e.BankAccount = in.BankAccount
	e.EmergencyContactName = in.EmergencyContactName
	e.EmergencyContactPhone = in.EmergencyContactPhone
	e.EmergencyContactRelation = in.EmergencyContactRelation
	e.PhotoURL = in.PhotoURL
	e.Notes = in.Notes
	return e
}

func (s *Service) CreateEmployee(ctx context.Context, in EmployeeInput, actor Actor) (EmployeeView, error) {
	if err := s.validateEmployee(ctx, in, ""); err != nil {
		return EmployeeView{}, err
	}
	employee := in.apply(domain.Employee{ID: s.ids.New(id.Employee)})

	created, err := s.repo.InsertEmployee(ctx, employee)
	if err != nil {
		return EmployeeView{}, err
	}
	// The allowance row is created with the person, so a new hire can file
	// leave on their first day without HR seeding anything.
	if _, err := s.repo.Balance(ctx, created.ID, s.Today().Year(), false); err != nil {
		return EmployeeView{}, err
	}
	s.record(ctx, "hris.employee", created.ID, "CREATE", actor, nil)

	names, err := s.namer(ctx)
	if err != nil {
		return EmployeeView{}, err
	}
	return names.view(created), nil
}

func (s *Service) UpdateEmployee(ctx context.Context, employeeID string, in EmployeeInput, actor Actor) (EmployeeView, error) {
	existing, err := s.repo.Employee(ctx, employeeID)
	if err != nil {
		return EmployeeView{}, err
	}
	if err := s.validateEmployee(ctx, in, employeeID); err != nil {
		return EmployeeView{}, err
	}
	updated, err := s.repo.UpdateEmployee(ctx, in.apply(existing))
	if err != nil {
		return EmployeeView{}, err
	}

	action := "UPDATE"
	if existing.Active && !updated.Active {
		// Someone leaving is worth finding in the audit trail on its own.
		action = "OFFBOARD"
	}
	s.record(ctx, "hris.employee", employeeID, action, actor, nil)

	names, err := s.namer(ctx)
	if err != nil {
		return EmployeeView{}, err
	}
	return names.view(updated), nil
}

// ── Shifts and schedules ─────────────────────────────────────────────────────

func (s *Service) Shifts(ctx context.Context, activeOnly bool) ([]domain.Shift, error) {
	return s.repo.Shifts(ctx, activeOnly)
}

// ShiftInput creates or replaces a working window.
type ShiftInput struct {
	Name                 string
	StartTime            domain.TimeOfDay
	EndTime              domain.TimeOfDay
	BreakMinutes         int
	LateToleranceMinutes int
	IsOvernight          bool
	Active               bool
	SortOrder            int
}

func (in ShiftInput) apply(shift domain.Shift) domain.Shift {
	shift.Name = in.Name
	shift.StartTime = in.StartTime
	shift.EndTime = in.EndTime
	shift.BreakMinutes = in.BreakMinutes
	shift.LateToleranceMinutes = in.LateToleranceMinutes
	shift.IsOvernight = in.IsOvernight
	shift.Active = in.Active
	shift.SortOrder = in.SortOrder
	return shift
}

func (s *Service) CreateShift(ctx context.Context, in ShiftInput, actor Actor) (domain.Shift, error) {
	created, err := s.repo.InsertShift(ctx, in.apply(domain.Shift{ID: s.ids.New(id.Shift)}))
	if err != nil {
		return domain.Shift{}, err
	}
	s.record(ctx, "hris.shift", created.ID, "CREATE", actor, nil)
	return created, nil
}

func (s *Service) UpdateShift(ctx context.Context, shiftID string, in ShiftInput, actor Actor) (domain.Shift, error) {
	existing, err := s.repo.Shift(ctx, shiftID)
	if err != nil {
		return domain.Shift{}, err
	}
	updated, err := s.repo.UpdateShift(ctx, in.apply(existing))
	if err != nil {
		return domain.Shift{}, err
	}
	s.record(ctx, "hris.shift", shiftID, "UPDATE", actor, nil)
	return updated, nil
}

func (s *Service) scheduleView(ctx context.Context, employeeID string) ([]ScheduleRowView, error) {
	rows, err := s.repo.Schedule(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	shifts, err := s.repo.Shifts(ctx, false)
	if err != nil {
		return nil, err
	}
	byID := map[string]domain.Shift{}
	for _, shift := range shifts {
		byID[shift.ID] = shift
	}

	views := make([]ScheduleRowView, 0, len(rows))
	for _, row := range rows {
		view := ScheduleRowView{EmployeeShift: row, DayName: dayName(row.DayOfWeek)}
		if row.ShiftID != nil {
			if shift, ok := byID[*row.ShiftID]; ok {
				name := shift.Name
				view.ShiftName = &name
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// Schedule is one employee's weekly pattern, including its history.
func (s *Service) Schedule(ctx context.Context, employeeID string) ([]ScheduleRowView, error) {
	if _, err := s.repo.Employee(ctx, employeeID); err != nil {
		return nil, err
	}
	return s.scheduleView(ctx, employeeID)
}

// ScheduleInput assigns a weekday. A nil ShiftID is a rest day, which is not
// the same as having no pattern: the roster shows one as resting and the other
// as unscheduled.
type ScheduleInput struct {
	DayOfWeek     int
	ShiftID       *string
	EffectiveFrom domain.Date
	EffectiveTo   *domain.Date
}

func (s *Service) AssignShift(ctx context.Context, employeeID string, in ScheduleInput, actor Actor) (ScheduleRowView, error) {
	if in.DayOfWeek < 1 || in.DayOfWeek > 7 {
		return ScheduleRowView{}, httpx.Invalid("A weekday runs from 1 (Monday) to 7 (Sunday).")
	}
	if _, err := s.repo.Employee(ctx, employeeID); err != nil {
		return ScheduleRowView{}, err
	}
	var shiftName *string
	if in.ShiftID != nil {
		shift, err := s.repo.Shift(ctx, *in.ShiftID)
		if err != nil {
			return ScheduleRowView{}, err
		}
		shiftName = &shift.Name
	}
	if in.EffectiveTo != nil && in.EffectiveTo.Before(in.EffectiveFrom) {
		return ScheduleRowView{}, httpx.Invalid("A pattern cannot end before it starts.")
	}

	created, err := s.repo.InsertScheduleRow(ctx, domain.EmployeeShift{
		ID: s.ids.New(id.EmployeeShift), EmployeeID: employeeID, DayOfWeek: in.DayOfWeek,
		ShiftID: in.ShiftID, EffectiveFrom: in.EffectiveFrom, EffectiveTo: in.EffectiveTo,
	})
	if err != nil {
		return ScheduleRowView{}, err
	}
	s.record(ctx, "hris.employee", employeeID, "SCHEDULE_ASSIGN", actor,
		audit.Str(fmt.Sprintf("%s from %s", dayName(in.DayOfWeek), in.EffectiveFrom)))

	return ScheduleRowView{EmployeeShift: created, ShiftName: shiftName, DayName: dayName(created.DayOfWeek)}, nil
}

func (s *Service) RemoveScheduleRow(ctx context.Context, employeeID, rowID string, actor Actor) error {
	if err := s.repo.DeleteScheduleRow(ctx, rowID); err != nil {
		return err
	}
	s.record(ctx, "hris.employee", employeeID, "SCHEDULE_REMOVE", actor, nil)
	return nil
}
