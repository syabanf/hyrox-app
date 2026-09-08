package hris

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Repository persists staff records. It reads and writes the `hris` schema
// only; branches, coaches and staff logins are referenced by plain id.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// dateValue converts a calendar date for the driver, mapping the empty date to
// SQL NULL so an optional date does not become year zero.
func dateValue(d *domain.Date) any {
	if d == nil || *d == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", string(*d))
	if err != nil {
		return nil
	}
	return parsed
}

func requiredDate(d domain.Date) any { return dateValue(&d) }

// scanDate reads a DATE column back into a calendar date.
func scanDate(t *time.Time) domain.Date {
	if t == nil {
		return ""
	}
	return domain.Date(t.Format("2006-01-02"))
}

func optionalDate(t *time.Time) *domain.Date {
	if t == nil {
		return nil
	}
	d := scanDate(t)
	return &d
}

// ── Departments ──────────────────────────────────────────────────────────────

const departmentColumns = `id, name, code, description, parent_department_id, cost_center, active, created_at, updated_at`

func scanDepartment(row pgx.Row) (domain.Department, error) {
	var d domain.Department
	err := row.Scan(&d.ID, &d.Name, &d.Code, &d.Description, &d.ParentDepartmentID,
		&d.CostCenter, &d.Active, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (r *Repository) Departments(ctx context.Context) ([]domain.Department, error) {
	rows, err := r.db.Query(ctx, `SELECT `+departmentColumns+` FROM hris.departments ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("hris: listing departments: %w", err)
	}
	defer rows.Close()

	out := []domain.Department{}
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning department: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repository) Department(ctx context.Context, id string) (domain.Department, error) {
	d, err := scanDepartment(r.db.QueryRow(ctx, `SELECT `+departmentColumns+` FROM hris.departments WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Department{}, httpx.NotFound("department")
	}
	if err != nil {
		return domain.Department{}, fmt.Errorf("hris: reading department: %w", err)
	}
	return d, nil
}

func (r *Repository) InsertDepartment(ctx context.Context, d domain.Department) (domain.Department, error) {
	created, err := scanDepartment(r.db.QueryRow(ctx, `
		INSERT INTO hris.departments (id, name, code, description, parent_department_id, cost_center, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+departmentColumns,
		d.ID, d.Name, d.Code, d.Description, d.ParentDepartmentID, d.CostCenter, d.Active))
	if database.IsUniqueViolation(err) {
		return domain.Department{}, httpx.Conflict("DUPLICATE", "A department already uses that code.")
	}
	if err != nil {
		return domain.Department{}, fmt.Errorf("hris: inserting department: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateDepartment(ctx context.Context, d domain.Department) (domain.Department, error) {
	updated, err := scanDepartment(r.db.QueryRow(ctx, `
		UPDATE hris.departments SET name = $2, code = $3, description = $4,
			parent_department_id = $5, cost_center = $6, active = $7, updated_at = now()
		WHERE id = $1 RETURNING `+departmentColumns,
		d.ID, d.Name, d.Code, d.Description, d.ParentDepartmentID, d.CostCenter, d.Active))
	if database.IsNoRows(err) {
		return domain.Department{}, httpx.NotFound("department")
	}
	if database.IsUniqueViolation(err) {
		return domain.Department{}, httpx.Conflict("DUPLICATE", "Another department already uses that code.")
	}
	if err != nil {
		return domain.Department{}, fmt.Errorf("hris: updating department: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteDepartment(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM hris.departments WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.ErrInUse.WithMessage("Employees or sub-departments belong to this department.")
	}
	if err != nil {
		return fmt.Errorf("hris: deleting department: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("department")
	}
	return nil
}

// ── Positions ────────────────────────────────────────────────────────────────

const positionColumns = `id, title, level, active, created_at`

func (r *Repository) Positions(ctx context.Context) ([]domain.Position, error) {
	rows, err := r.db.Query(ctx, `SELECT `+positionColumns+` FROM hris.positions ORDER BY title`)
	if err != nil {
		return nil, fmt.Errorf("hris: listing positions: %w", err)
	}
	defer rows.Close()

	out := []domain.Position{}
	for rows.Next() {
		var p domain.Position
		if err := rows.Scan(&p.ID, &p.Title, &p.Level, &p.Active, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("hris: scanning position: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) InsertPosition(ctx context.Context, p domain.Position) (domain.Position, error) {
	var created domain.Position
	err := r.db.QueryRow(ctx, `
		INSERT INTO hris.positions (id, title, level, active) VALUES ($1, $2, $3, $4)
		RETURNING `+positionColumns, p.ID, p.Title, p.Level, p.Active).
		Scan(&created.ID, &created.Title, &created.Level, &created.Active, &created.CreatedAt)
	if err != nil {
		return domain.Position{}, fmt.Errorf("hris: inserting position: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdatePosition(ctx context.Context, p domain.Position) (domain.Position, error) {
	var updated domain.Position
	err := r.db.QueryRow(ctx, `
		UPDATE hris.positions SET title = $2, level = $3, active = $4, updated_at = now()
		WHERE id = $1 RETURNING `+positionColumns, p.ID, p.Title, p.Level, p.Active).
		Scan(&updated.ID, &updated.Title, &updated.Level, &updated.Active, &updated.CreatedAt)
	if database.IsNoRows(err) {
		return domain.Position{}, httpx.NotFound("position")
	}
	if err != nil {
		return domain.Position{}, fmt.Errorf("hris: updating position: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeletePosition(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM hris.positions WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.ErrInUse.WithMessage("Employees hold this position.")
	}
	if err != nil {
		return fmt.Errorf("hris: deleting position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("position")
	}
	return nil
}

// ── Employment statuses ──────────────────────────────────────────────────────

func (r *Repository) EmploymentStatuses(ctx context.Context) ([]domain.EmploymentStatus, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, code, name, description, active FROM hris.employment_statuses ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("hris: listing employment statuses: %w", err)
	}
	defer rows.Close()

	out := []domain.EmploymentStatus{}
	for rows.Next() {
		var s domain.EmploymentStatus
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Description, &s.Active); err != nil {
			return nil, fmt.Errorf("hris: scanning employment status: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) InsertEmploymentStatus(ctx context.Context, s domain.EmploymentStatus) (domain.EmploymentStatus, error) {
	var created domain.EmploymentStatus
	err := r.db.QueryRow(ctx, `
		INSERT INTO hris.employment_statuses (id, code, name, description, active)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, code, name, description, active`,
		s.ID, s.Code, s.Name, s.Description, s.Active).
		Scan(&created.ID, &created.Code, &created.Name, &created.Description, &created.Active)
	if database.IsUniqueViolation(err) {
		return domain.EmploymentStatus{}, httpx.Conflict("DUPLICATE", "That status code already exists.")
	}
	if err != nil {
		return domain.EmploymentStatus{}, fmt.Errorf("hris: inserting employment status: %w", err)
	}
	return created, nil
}

// ── Employees ────────────────────────────────────────────────────────────────

const employeeColumns = `id, full_name, employee_number, email, phone, birth_date, gender,
	marital_status, address, join_date, end_date, employment_status_code, active,
	department_id, position_id, branch_id, coach_id, admin_user_id, reporting_to,
	bank_name, bank_account, emergency_contact_name, emergency_contact_phone,
	emergency_contact_relation, photo_url, notes, created_at, updated_at`

func scanEmployee(row pgx.Row) (domain.Employee, error) {
	var e domain.Employee
	var birth, join, end *time.Time
	err := row.Scan(&e.ID, &e.FullName, &e.EmployeeNumber, &e.Email, &e.Phone, &birth, &e.Gender,
		&e.MaritalStatus, &e.Address, &join, &end, &e.EmploymentStatusCode, &e.Active,
		&e.DepartmentID, &e.PositionID, &e.BranchID, &e.CoachID, &e.AdminUserID, &e.ReportingTo,
		&e.BankName, &e.BankAccount, &e.EmergencyContactName, &e.EmergencyContactPhone,
		&e.EmergencyContactRelation, &e.PhotoURL, &e.Notes, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return domain.Employee{}, err
	}
	e.BirthDate = optionalDate(birth)
	e.JoinDate = scanDate(join)
	e.EndDate = optionalDate(end)
	return e, nil
}

// EmployeeFilter narrows the staff list.
type EmployeeFilter struct {
	Query        string
	DepartmentID string
	BranchID     string
	ActiveOnly   bool
	Limit        int
}

func (r *Repository) Employees(ctx context.Context, filter EmployeeFilter) ([]domain.Employee, error) {
	query := `SELECT ` + employeeColumns + ` FROM hris.employees WHERE 1 = 1`
	args := []any{}

	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		query += fmt.Sprintf(
			` AND (lower(full_name) LIKE $%d OR lower(email) LIKE $%d OR lower(employee_number) LIKE $%d)`,
			len(args), len(args), len(args))
	}
	if filter.DepartmentID != "" {
		args = append(args, filter.DepartmentID)
		query += fmt.Sprintf(` AND department_id = $%d`, len(args))
	}
	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.ActiveOnly {
		query += ` AND active`
	}
	query += ` ORDER BY full_name`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hris: listing employees: %w", err)
	}
	defer rows.Close()

	out := []domain.Employee{}
	for rows.Next() {
		e, err := scanEmployee(rows)
		if err != nil {
			return nil, fmt.Errorf("hris: scanning employee: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) Employee(ctx context.Context, id string) (domain.Employee, error) {
	e, err := scanEmployee(r.db.QueryRow(ctx, `SELECT `+employeeColumns+` FROM hris.employees WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Employee{}, httpx.NotFound("employee")
	}
	if err != nil {
		return domain.Employee{}, fmt.Errorf("hris: reading employee: %w", err)
	}
	return e, nil
}

// EmployeeByCoach finds the staff record behind a coach, which is how the
// class schedule reaches payroll and attendance.
func (r *Repository) EmployeeByCoach(ctx context.Context, coachID string) (domain.Employee, bool, error) {
	e, err := scanEmployee(r.db.QueryRow(ctx,
		`SELECT `+employeeColumns+` FROM hris.employees WHERE coach_id = $1`, coachID))
	if database.IsNoRows(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, fmt.Errorf("hris: reading employee by coach: %w", err)
	}
	return e, true, nil
}

// EmployeeByAdminUser finds the staff record behind a login, which is how
// someone signed into the admin panel reaches their own timesheet.
func (r *Repository) EmployeeByAdminUser(ctx context.Context, adminUserID string) (domain.Employee, bool, error) {
	e, err := scanEmployee(r.db.QueryRow(ctx,
		`SELECT `+employeeColumns+` FROM hris.employees WHERE admin_user_id = $1`, adminUserID))
	if database.IsNoRows(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, fmt.Errorf("hris: reading employee by login: %w", err)
	}
	return e, true, nil
}

func (r *Repository) InsertEmployee(ctx context.Context, e domain.Employee) (domain.Employee, error) {
	created, err := scanEmployee(r.db.QueryRow(ctx, `
		INSERT INTO hris.employees (id, full_name, employee_number, email, phone, birth_date, gender,
			marital_status, address, join_date, end_date, employment_status_code, active,
			department_id, position_id, branch_id, coach_id, admin_user_id, reporting_to,
			bank_name, bank_account, emergency_contact_name, emergency_contact_phone,
			emergency_contact_relation, photo_url, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25, $26)
		RETURNING `+employeeColumns,
		e.ID, e.FullName, e.EmployeeNumber, e.Email, e.Phone, dateValue(e.BirthDate), e.Gender,
		e.MaritalStatus, e.Address, requiredDate(e.JoinDate), dateValue(e.EndDate),
		e.EmploymentStatusCode, e.Active, e.DepartmentID, e.PositionID, e.BranchID, e.CoachID,
		e.AdminUserID, e.ReportingTo, e.BankName, e.BankAccount, e.EmergencyContactName,
		e.EmergencyContactPhone, e.EmergencyContactRelation, e.PhotoURL, e.Notes))
	if database.IsUniqueViolation(err, "employees_coach_idx") {
		return domain.Employee{}, httpx.Conflict("COACH_TAKEN", "Another employee is already linked to that coach.")
	}
	if database.IsUniqueViolation(err) {
		return domain.Employee{}, httpx.Conflict("DUPLICATE", "An employee already uses that number or email.")
	}
	if err != nil {
		return domain.Employee{}, fmt.Errorf("hris: inserting employee: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateEmployee(ctx context.Context, e domain.Employee) (domain.Employee, error) {
	updated, err := scanEmployee(r.db.QueryRow(ctx, `
		UPDATE hris.employees SET full_name = $2, employee_number = $3, email = $4, phone = $5,
			birth_date = $6, gender = $7, marital_status = $8, address = $9, join_date = $10,
			end_date = $11, employment_status_code = $12, active = $13, department_id = $14,
			position_id = $15, branch_id = $16, coach_id = $17, admin_user_id = $18,
			reporting_to = $19, bank_name = $20, bank_account = $21, emergency_contact_name = $22,
			emergency_contact_phone = $23, emergency_contact_relation = $24, photo_url = $25,
			notes = $26, updated_at = now()
		WHERE id = $1 RETURNING `+employeeColumns,
		e.ID, e.FullName, e.EmployeeNumber, e.Email, e.Phone, dateValue(e.BirthDate), e.Gender,
		e.MaritalStatus, e.Address, requiredDate(e.JoinDate), dateValue(e.EndDate),
		e.EmploymentStatusCode, e.Active, e.DepartmentID, e.PositionID, e.BranchID, e.CoachID,
		e.AdminUserID, e.ReportingTo, e.BankName, e.BankAccount, e.EmergencyContactName,
		e.EmergencyContactPhone, e.EmergencyContactRelation, e.PhotoURL, e.Notes))
	if database.IsNoRows(err) {
		return domain.Employee{}, httpx.NotFound("employee")
	}
	if database.IsUniqueViolation(err, "employees_coach_idx") {
		return domain.Employee{}, httpx.Conflict("COACH_TAKEN", "Another employee is already linked to that coach.")
	}
	if database.IsUniqueViolation(err) {
		return domain.Employee{}, httpx.Conflict("DUPLICATE", "Another employee already uses that number or email.")
	}
	if err != nil {
		return domain.Employee{}, fmt.Errorf("hris: updating employee: %w", err)
	}
	return updated, nil
}

// CountEmployees backs the summary cards.
func (r *Repository) CountEmployees(ctx context.Context) (active int, total int, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE active), count(*) FROM hris.employees`).Scan(&active, &total)
	if err != nil {
		return 0, 0, fmt.Errorf("hris: counting employees: %w", err)
	}
	return active, total, nil
}
