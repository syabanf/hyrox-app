package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Repository is the catalog module's persistence. It touches the `catalog`
// schema and nothing else: no other module's tables are readable from here.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── Organization and business rules ──────────────────────────────────────────

const rulesColumns = `default_credit_expiry_days, cancellation_deadline_hours, late_cancellation_policy,
	no_show_policy, re_entry_grace_minutes, anti_passback_minutes, qr_ttl_seconds,
	waitlist_auto_promote, low_balance_threshold, expiry_reminder_days,
	booking_opens_days_before, booking_closes_minutes_before`

func scanRules(row pgx.Row) (domain.BusinessRules, error) {
	var r domain.BusinessRules
	err := row.Scan(
		&r.DefaultCreditExpiryDays, &r.CancellationDeadlineHours, &r.LateCancellationPolicy,
		&r.NoShowPolicy, &r.ReEntryGraceMinutes, &r.AntiPassbackMinutes, &r.QRTTLSeconds,
		&r.WaitlistAutoPromote, &r.LowBalanceThreshold, &r.ExpiryReminderDays,
		&r.BookingOpensDaysBefore, &r.BookingClosesMinutesBefore,
	)
	return r, err
}

func (r *Repository) Rules(ctx context.Context) (domain.BusinessRules, error) {
	rules, err := scanRules(r.db.QueryRow(ctx, `SELECT `+rulesColumns+` FROM catalog.business_rules WHERE id`))
	if err != nil {
		return domain.BusinessRules{}, fmt.Errorf("catalog: reading rules: %w", err)
	}
	return rules, nil
}

func (r *Repository) UpdateRules(ctx context.Context, rules domain.BusinessRules) (domain.BusinessRules, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE catalog.business_rules SET
			default_credit_expiry_days = $1, cancellation_deadline_hours = $2,
			late_cancellation_policy = $3, no_show_policy = $4,
			re_entry_grace_minutes = $5, anti_passback_minutes = $6, qr_ttl_seconds = $7,
			waitlist_auto_promote = $8, low_balance_threshold = $9, expiry_reminder_days = $10,
			booking_opens_days_before = $11, booking_closes_minutes_before = $12,
			updated_at = now()
		WHERE id
		RETURNING `+rulesColumns,
		rules.DefaultCreditExpiryDays, rules.CancellationDeadlineHours,
		rules.LateCancellationPolicy, rules.NoShowPolicy,
		rules.ReEntryGraceMinutes, rules.AntiPassbackMinutes, rules.QRTTLSeconds,
		rules.WaitlistAutoPromote, rules.LowBalanceThreshold, rules.ExpiryReminderDays,
		rules.BookingOpensDaysBefore, rules.BookingClosesMinutesBefore,
	)
	updated, err := scanRules(row)
	if err != nil {
		return domain.BusinessRules{}, fmt.Errorf("catalog: updating rules: %w", err)
	}
	return updated, nil
}

func (r *Repository) Organization(ctx context.Context) (domain.Organization, error) {
	var org domain.Organization
	err := r.db.QueryRow(ctx, `SELECT id, name FROM catalog.organizations ORDER BY created_at LIMIT 1`).
		Scan(&org.ID, &org.Name)
	if err != nil {
		return domain.Organization{}, fmt.Errorf("catalog: reading organization: %w", err)
	}
	return org, nil
}

// ── Branches ─────────────────────────────────────────────────────────────────

const branchColumns = `id, organization_id, name, address, timezone, operating_hours, status, manager_name, rules_override`

func scanBranch(row pgx.Row) (domain.Branch, error) {
	var b domain.Branch
	var override []byte
	if err := row.Scan(&b.ID, &b.OrganizationID, &b.Name, &b.Address, &b.Timezone,
		&b.OperatingHours, &b.Status, &b.ManagerName, &override); err != nil {
		return domain.Branch{}, err
	}
	if len(override) > 0 {
		var parsed domain.RulesOverride
		if err := json.Unmarshal(override, &parsed); err != nil {
			return domain.Branch{}, fmt.Errorf("catalog: decoding branch rules override: %w", err)
		}
		b.RulesOverride = &parsed
	}
	return b, nil
}

func (r *Repository) Branches(ctx context.Context) ([]domain.Branch, error) {
	rows, err := r.db.Query(ctx, `SELECT `+branchColumns+` FROM catalog.branches ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("catalog: listing branches: %w", err)
	}
	defer rows.Close()

	branches := []domain.Branch{}
	for rows.Next() {
		b, err := scanBranch(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: scanning branch: %w", err)
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

func (r *Repository) Branch(ctx context.Context, id string) (domain.Branch, error) {
	b, err := scanBranch(r.db.QueryRow(ctx, `SELECT `+branchColumns+` FROM catalog.branches WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Branch{}, httpx.NotFound("branch")
	}
	if err != nil {
		return domain.Branch{}, fmt.Errorf("catalog: reading branch: %w", err)
	}
	return b, nil
}

func (r *Repository) InsertBranch(ctx context.Context, b domain.Branch) (domain.Branch, error) {
	override, err := marshalOverride(b.RulesOverride)
	if err != nil {
		return domain.Branch{}, err
	}
	created, err := scanBranch(r.db.QueryRow(ctx, `
		INSERT INTO catalog.branches (id, organization_id, name, address, timezone, operating_hours, status, manager_name, rules_override)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+branchColumns,
		b.ID, b.OrganizationID, b.Name, b.Address, b.Timezone, b.OperatingHours, b.Status, b.ManagerName, override))
	if err != nil {
		return domain.Branch{}, fmt.Errorf("catalog: inserting branch: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateBranch(ctx context.Context, b domain.Branch) (domain.Branch, error) {
	override, err := marshalOverride(b.RulesOverride)
	if err != nil {
		return domain.Branch{}, err
	}
	updated, err := scanBranch(r.db.QueryRow(ctx, `
		UPDATE catalog.branches
		SET name = $2, address = $3, timezone = $4, operating_hours = $5,
		    status = $6, manager_name = $7, rules_override = $8, updated_at = now()
		WHERE id = $1
		RETURNING `+branchColumns,
		b.ID, b.Name, b.Address, b.Timezone, b.OperatingHours, b.Status, b.ManagerName, override))
	if database.IsNoRows(err) {
		return domain.Branch{}, httpx.NotFound("branch")
	}
	if err != nil {
		return domain.Branch{}, fmt.Errorf("catalog: updating branch: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteBranch(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM catalog.branches WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.ErrInUse.WithMessage("Gates or coaches belong to this branch. Move or remove them first.")
	}
	if err != nil {
		return fmt.Errorf("catalog: deleting branch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("branch")
	}
	return nil
}

func marshalOverride(o *domain.RulesOverride) ([]byte, error) {
	if o == nil || o.IsEmpty() {
		return nil, nil
	}
	body, err := json.Marshal(o)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding rules override: %w", err)
	}
	return body, nil
}

// ── Gates ────────────────────────────────────────────────────────────────────

const gateColumns = `id, branch_id, name, status`

func (r *Repository) Gates(ctx context.Context) ([]domain.Gate, error) {
	rows, err := r.db.Query(ctx, `SELECT `+gateColumns+` FROM catalog.gates ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("catalog: listing gates: %w", err)
	}
	defer rows.Close()

	gates := []domain.Gate{}
	for rows.Next() {
		var g domain.Gate
		if err := rows.Scan(&g.ID, &g.BranchID, &g.Name, &g.Status); err != nil {
			return nil, fmt.Errorf("catalog: scanning gate: %w", err)
		}
		gates = append(gates, g)
	}
	return gates, rows.Err()
}

func (r *Repository) Gate(ctx context.Context, id string) (domain.Gate, error) {
	var g domain.Gate
	err := r.db.QueryRow(ctx, `SELECT `+gateColumns+` FROM catalog.gates WHERE id = $1`, id).
		Scan(&g.ID, &g.BranchID, &g.Name, &g.Status)
	if database.IsNoRows(err) {
		return domain.Gate{}, httpx.NotFound("gate")
	}
	if err != nil {
		return domain.Gate{}, fmt.Errorf("catalog: reading gate: %w", err)
	}
	return g, nil
}

func (r *Repository) InsertGate(ctx context.Context, g domain.Gate) (domain.Gate, error) {
	var created domain.Gate
	err := r.db.QueryRow(ctx, `
		INSERT INTO catalog.gates (id, branch_id, name, status) VALUES ($1, $2, $3, $4)
		RETURNING `+gateColumns,
		g.ID, g.BranchID, g.Name, g.Status).
		Scan(&created.ID, &created.BranchID, &created.Name, &created.Status)
	if database.IsForeignKeyViolation(err) {
		return domain.Gate{}, httpx.NotFound("branch")
	}
	if err != nil {
		return domain.Gate{}, fmt.Errorf("catalog: inserting gate: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateGate(ctx context.Context, g domain.Gate) (domain.Gate, error) {
	var updated domain.Gate
	err := r.db.QueryRow(ctx, `
		UPDATE catalog.gates SET branch_id = $2, name = $3, status = $4, updated_at = now()
		WHERE id = $1 RETURNING `+gateColumns,
		g.ID, g.BranchID, g.Name, g.Status).
		Scan(&updated.ID, &updated.BranchID, &updated.Name, &updated.Status)
	if database.IsNoRows(err) {
		return domain.Gate{}, httpx.NotFound("gate")
	}
	if err != nil {
		return domain.Gate{}, fmt.Errorf("catalog: updating gate: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteGate(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM catalog.gates WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("catalog: deleting gate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("gate")
	}
	return nil
}

// ── Coaches ──────────────────────────────────────────────────────────────────

const coachColumns = `id, name, bio, specialization, branch_id, status`

func (r *Repository) Coaches(ctx context.Context) ([]domain.Coach, error) {
	rows, err := r.db.Query(ctx, `SELECT `+coachColumns+` FROM catalog.coaches ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("catalog: listing coaches: %w", err)
	}
	defer rows.Close()

	coaches := []domain.Coach{}
	for rows.Next() {
		var c domain.Coach
		if err := rows.Scan(&c.ID, &c.Name, &c.Bio, &c.Specialization, &c.BranchID, &c.Status); err != nil {
			return nil, fmt.Errorf("catalog: scanning coach: %w", err)
		}
		coaches = append(coaches, c)
	}
	return coaches, rows.Err()
}

func (r *Repository) Coach(ctx context.Context, id string) (domain.Coach, error) {
	var c domain.Coach
	err := r.db.QueryRow(ctx, `SELECT `+coachColumns+` FROM catalog.coaches WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.Bio, &c.Specialization, &c.BranchID, &c.Status)
	if database.IsNoRows(err) {
		return domain.Coach{}, httpx.NotFound("coach")
	}
	if err != nil {
		return domain.Coach{}, fmt.Errorf("catalog: reading coach: %w", err)
	}
	return c, nil
}

func (r *Repository) InsertCoach(ctx context.Context, c domain.Coach) (domain.Coach, error) {
	var created domain.Coach
	err := r.db.QueryRow(ctx, `
		INSERT INTO catalog.coaches (id, name, bio, specialization, branch_id, status)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+coachColumns,
		c.ID, c.Name, c.Bio, c.Specialization, c.BranchID, c.Status).
		Scan(&created.ID, &created.Name, &created.Bio, &created.Specialization, &created.BranchID, &created.Status)
	if database.IsForeignKeyViolation(err) {
		return domain.Coach{}, httpx.NotFound("branch")
	}
	if err != nil {
		return domain.Coach{}, fmt.Errorf("catalog: inserting coach: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateCoach(ctx context.Context, c domain.Coach) (domain.Coach, error) {
	var updated domain.Coach
	err := r.db.QueryRow(ctx, `
		UPDATE catalog.coaches SET name = $2, bio = $3, specialization = $4, branch_id = $5,
		       status = $6, updated_at = now()
		WHERE id = $1 RETURNING `+coachColumns,
		c.ID, c.Name, c.Bio, c.Specialization, c.BranchID, c.Status).
		Scan(&updated.ID, &updated.Name, &updated.Bio, &updated.Specialization, &updated.BranchID, &updated.Status)
	if database.IsNoRows(err) {
		return domain.Coach{}, httpx.NotFound("coach")
	}
	if err != nil {
		return domain.Coach{}, fmt.Errorf("catalog: updating coach: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteCoach(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM catalog.coaches WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("catalog: deleting coach: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("coach")
	}
	return nil
}

// ── Class types ──────────────────────────────────────────────────────────────

const classTypeColumns = `id, name, description, default_duration_min, default_credit_cost, default_capacity, active`

func scanClassType(row pgx.Row) (domain.ClassType, error) {
	var t domain.ClassType
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.DefaultDurationMin,
		&t.DefaultCreditCost, &t.DefaultCapacity, &t.Active)
	return t, err
}

// ClassTypes lists templates; activeOnly serves the member app, which must not
// see retired classes.
func (r *Repository) ClassTypes(ctx context.Context, activeOnly bool) ([]domain.ClassType, error) {
	query := `SELECT ` + classTypeColumns + ` FROM catalog.class_types`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY name`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("catalog: listing class types: %w", err)
	}
	defer rows.Close()

	types := []domain.ClassType{}
	for rows.Next() {
		t, err := scanClassType(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: scanning class type: %w", err)
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

func (r *Repository) ClassType(ctx context.Context, id string) (domain.ClassType, error) {
	t, err := scanClassType(r.db.QueryRow(ctx, `SELECT `+classTypeColumns+` FROM catalog.class_types WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.ClassType{}, httpx.NotFound("class type")
	}
	if err != nil {
		return domain.ClassType{}, fmt.Errorf("catalog: reading class type: %w", err)
	}
	return t, nil
}

func (r *Repository) InsertClassType(ctx context.Context, t domain.ClassType) (domain.ClassType, error) {
	created, err := scanClassType(r.db.QueryRow(ctx, `
		INSERT INTO catalog.class_types (id, name, description, default_duration_min, default_credit_cost, default_capacity, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+classTypeColumns,
		t.ID, t.Name, t.Description, t.DefaultDurationMin, t.DefaultCreditCost, t.DefaultCapacity, t.Active))
	if err != nil {
		return domain.ClassType{}, fmt.Errorf("catalog: inserting class type: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateClassType(ctx context.Context, t domain.ClassType) (domain.ClassType, error) {
	updated, err := scanClassType(r.db.QueryRow(ctx, `
		UPDATE catalog.class_types SET name = $2, description = $3, default_duration_min = $4,
		       default_credit_cost = $5, default_capacity = $6, active = $7, updated_at = now()
		WHERE id = $1 RETURNING `+classTypeColumns,
		t.ID, t.Name, t.Description, t.DefaultDurationMin, t.DefaultCreditCost, t.DefaultCapacity, t.Active))
	if database.IsNoRows(err) {
		return domain.ClassType{}, httpx.NotFound("class type")
	}
	if err != nil {
		return domain.ClassType{}, fmt.Errorf("catalog: updating class type: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteClassType(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM catalog.class_types WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("catalog: deleting class type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("class type")
	}
	return nil
}

// ── Credit packages ──────────────────────────────────────────────────────────

const packageColumns = `id, name, credits, price_idr, validity_days, branch_id,
	purchase_limit_per_member, applicable_class_type_ids, status, created_at`

func scanPackage(row pgx.Row) (domain.CreditPackage, error) {
	var p domain.CreditPackage
	var coverage []byte
	if err := row.Scan(&p.ID, &p.Name, &p.Credits, &p.PriceIDR, &p.ValidityDays, &p.BranchID,
		&p.PurchaseLimitPerMember, &coverage, &p.Status, &p.CreatedAt); err != nil {
		return domain.CreditPackage{}, err
	}
	if len(coverage) > 0 {
		if err := json.Unmarshal(coverage, &p.ApplicableClassTypeIDs); err != nil {
			return domain.CreditPackage{}, fmt.Errorf("catalog: decoding package coverage: %w", err)
		}
	}
	return p, nil
}

func (r *Repository) Packages(ctx context.Context, activeOnly bool) ([]domain.CreditPackage, error) {
	query := `SELECT ` + packageColumns + ` FROM catalog.credit_packages`
	if activeOnly {
		query += ` WHERE status = 'ACTIVE'`
	}
	query += ` ORDER BY price_idr`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("catalog: listing packages: %w", err)
	}
	defer rows.Close()

	packages := []domain.CreditPackage{}
	for rows.Next() {
		p, err := scanPackage(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: scanning package: %w", err)
		}
		packages = append(packages, p)
	}
	return packages, rows.Err()
}

func (r *Repository) Package(ctx context.Context, id string) (domain.CreditPackage, error) {
	p, err := scanPackage(r.db.QueryRow(ctx, `SELECT `+packageColumns+` FROM catalog.credit_packages WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.CreditPackage{}, httpx.NotFound("package")
	}
	if err != nil {
		return domain.CreditPackage{}, fmt.Errorf("catalog: reading package: %w", err)
	}
	return p, nil
}

func (r *Repository) InsertPackage(ctx context.Context, p domain.CreditPackage) (domain.CreditPackage, error) {
	coverage, err := marshalIDs(p.ApplicableClassTypeIDs)
	if err != nil {
		return domain.CreditPackage{}, err
	}
	created, err := scanPackage(r.db.QueryRow(ctx, `
		INSERT INTO catalog.credit_packages (id, name, credits, price_idr, validity_days, branch_id,
			purchase_limit_per_member, applicable_class_type_ids, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+packageColumns,
		p.ID, p.Name, p.Credits, p.PriceIDR, p.ValidityDays, p.BranchID,
		p.PurchaseLimitPerMember, coverage, p.Status))
	if err != nil {
		return domain.CreditPackage{}, fmt.Errorf("catalog: inserting package: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdatePackage(ctx context.Context, p domain.CreditPackage) (domain.CreditPackage, error) {
	coverage, err := marshalIDs(p.ApplicableClassTypeIDs)
	if err != nil {
		return domain.CreditPackage{}, err
	}
	updated, err := scanPackage(r.db.QueryRow(ctx, `
		UPDATE catalog.credit_packages SET name = $2, credits = $3, price_idr = $4, validity_days = $5,
		       branch_id = $6, purchase_limit_per_member = $7, applicable_class_type_ids = $8,
		       status = $9, updated_at = now()
		WHERE id = $1 RETURNING `+packageColumns,
		p.ID, p.Name, p.Credits, p.PriceIDR, p.ValidityDays, p.BranchID,
		p.PurchaseLimitPerMember, coverage, p.Status))
	if database.IsNoRows(err) {
		return domain.CreditPackage{}, httpx.NotFound("package")
	}
	if err != nil {
		return domain.CreditPackage{}, fmt.Errorf("catalog: updating package: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeletePackage(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM catalog.credit_packages WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("catalog: deleting package: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("package")
	}
	return nil
}

// marshalIDs encodes an id list, preserving the nil-means-everything rule: a
// nil slice becomes SQL NULL, an empty slice becomes an empty JSON array.
func marshalIDs(ids []string) ([]byte, error) {
	if ids == nil {
		return nil, nil
	}
	body, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding id list: %w", err)
	}
	return body, nil
}
