// Package seed loads a demo studio: two branches, staff for every role,
// coaches, class templates, packages, vouchers, members and a fortnight of
// classes.
//
// It is idempotent. Every insert is ON CONFLICT DO NOTHING, so running it
// twice changes nothing and running it against a live database cannot
// overwrite real data. The single exception is the demo password, which is
// filled in on a staff account that has none — never over one already set.
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
)

// Seeder writes the demo dataset.
type Seeder struct {
	db        *database.DB
	clock     clock.Clock
	passwords *auth.Passwords
}

// New builds a seeder. iterations is the PBKDF2 cost for the demo staff
// passwords; a test run that reseeds for every case passes something small.
func New(db *database.DB, c clock.Clock, iterations int) *Seeder {
	return &Seeder{db: db, clock: c, passwords: auth.NewPasswords(iterations)}
}

// DemoPassword signs in every seeded staff account. It exists so a demo has
// one thing to remember, and it is never written by anything but the seeder —
// which refuses to run against production without an explicit override.
const DemoPassword = "nuhabit-demo-2026"

// Summary reports what the seed produced.
type Summary struct {
	Branches    int
	Coaches     int
	ClassTypes  int
	Packages    int
	Members     int
	AdminUsers  int
	Sessions    int
	Exercises   int
	Employees   int
	StockItems  int
	Suppliers   int
	Tiers       int
	POSProducts int
	Activities  int
	Bookings    int
	Campaigns   int
	Pictures    int
}

// Run loads the demo studio in one transaction: a half-seeded database is
// worse than an empty one.
func (s *Seeder) Run(ctx context.Context) (Summary, error) {
	var summary Summary
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		var err error
		if err = s.seedOrganization(ctx); err != nil {
			return err
		}
		if summary.Branches, err = s.seedBranches(ctx); err != nil {
			return err
		}
		if err = s.seedGates(ctx); err != nil {
			return err
		}
		if summary.Coaches, err = s.seedCoaches(ctx); err != nil {
			return err
		}
		if summary.ClassTypes, err = s.seedClassTypes(ctx); err != nil {
			return err
		}
		if summary.Packages, err = s.seedPackages(ctx); err != nil {
			return err
		}
		if err = s.seedVouchers(ctx); err != nil {
			return err
		}
		if summary.AdminUsers, err = s.seedAdminUsers(ctx); err != nil {
			return err
		}
		if summary.Members, err = s.seedMembers(ctx); err != nil {
			return err
		}
		if summary.Exercises, err = s.seedExercises(ctx); err != nil {
			return err
		}
		if err = s.seedIncentiveScheme(ctx); err != nil {
			return err
		}
		if summary.Sessions, err = s.seedSessions(ctx); err != nil {
			return err
		}
		if summary.Employees, err = s.seedHR(ctx); err != nil {
			return err
		}
		if summary.StockItems, err = s.seedInventory(ctx); err != nil {
			return err
		}
		if summary.Suppliers, err = s.seedPurchasing(ctx); err != nil {
			return err
		}
		if summary.Tiers, err = s.seedLoyalty(ctx); err != nil {
			return err
		}
		if summary.Activities, err = s.seedTraining(ctx); err != nil {
			return err
		}
		if summary.POSProducts, err = s.seedPOS(ctx); err != nil {
			return err
		}
		// After the sessions and the members exist, because it books the one
		// against the other.
		if summary.Bookings, err = s.seedLifecycle(ctx); err != nil {
			return err
		}
		// Points come from visits and purchases, so this runs after both.
		if summary.Campaigns, err = s.seedCRMActivity(ctx); err != nil {
			return err
		}
		// Last, because it fills in pictures for rows the other seeders made.
		if summary.Pictures, err = s.seedMedia(ctx); err != nil {
			return err
		}
		return nil
	})
	return summary, err
}

func (s *Seeder) seedOrganization(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO catalog.organizations (id, name) VALUES ('org_nuhabit', 'NuHabit Studio Jakarta')
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: organization: %w", err)
	}
	return nil
}

type branchSeed struct {
	id, name, address, manager string
}

func (s *Seeder) seedBranches(ctx context.Context) (int, error) {
	branches := []branchSeed{
		{"brn_senopati", "Senopati", "Jl. Senopati No. 88, Jakarta Selatan", "Alya Santoso"},
		{"brn_pik", "PIK", "Jl. Pantai Indah Kapuk No. 12, Jakarta Utara", "Raka Wibowo"},
	}
	for _, b := range branches {
		_, err := s.db.Exec(ctx, `
			INSERT INTO catalog.branches (id, organization_id, name, address, timezone, operating_hours, status, manager_name)
			VALUES ($1, 'org_nuhabit', $2, $3, 'Asia/Jakarta', '06:00 - 22:00', 'ACTIVE', $4)
			ON CONFLICT (id) DO NOTHING`, b.id, b.name, b.address, b.manager)
		if err != nil {
			return 0, fmt.Errorf("seed: branch %s: %w", b.id, err)
		}
	}
	return len(branches), nil
}

func (s *Seeder) seedGates(ctx context.Context) error {
	gates := [][3]string{
		{"gat_senopati_a", "brn_senopati", "Senopati Gate A"},
		{"gat_senopati_b", "brn_senopati", "Senopati Gate B"},
		{"gat_pik_a", "brn_pik", "PIK Gate A"},
	}
	for _, g := range gates {
		_, err := s.db.Exec(ctx, `
			INSERT INTO catalog.gates (id, branch_id, name, status) VALUES ($1, $2, $3, 'ONLINE')
			ON CONFLICT (id) DO NOTHING`, g[0], g[1], g[2])
		if err != nil {
			return fmt.Errorf("seed: gate %s: %w", g[0], err)
		}
	}
	return nil
}

func (s *Seeder) seedCoaches(ctx context.Context) (int, error) {
	coaches := [][5]string{
		{"coa_kevin", "Kevin Hartono", "Engine and simulation specialist.", "Engine & simulation", "brn_senopati"},
		{"coa_maya", "Maya Kusuma", "Strength coach with a competition background.", "Strength and conditioning", "brn_pik"},
		{"coa_rizky", "Rizky Ramadhan", "Race-day pacing and station technique.", "HYROX race coach", "brn_senopati"},
		{"coa_tara", "Tara Widjaja", "Mobility, recovery and injury prevention.", "Mobility and recovery", "brn_pik"},
	}
	for _, c := range coaches {
		_, err := s.db.Exec(ctx, `
			INSERT INTO catalog.coaches (id, name, bio, specialization, branch_id, status)
			VALUES ($1, $2, $3, $4, $5, 'ACTIVE') ON CONFLICT (id) DO NOTHING`,
			c[0], c[1], c[2], c[3], c[4])
		if err != nil {
			return 0, fmt.Errorf("seed: coach %s: %w", c[0], err)
		}
	}
	return len(coaches), nil
}

type classTypeSeed struct {
	id, name, description string
	duration, cost, cap   int
}

func (s *Seeder) seedClassTypes(ctx context.Context) (int, error) {
	types := []classTypeSeed{
		{"clt_fundamentals", "HYROX Fundamentals", "Learn the eight stations and how to pace them.", 60, 1, 16},
		{"clt_engine", "Engine Builder", "Running-heavy conditioning for race pace.", 60, 1, 20},
		{"clt_strength", "Strength Circuit", "Sled, carry and lunge strength work.", 75, 1, 14},
		{"clt_simulation", "Race Simulation", "A full eight-station simulation against the clock.", 90, 2, 12},
		{"clt_mobility", "Mobility and Recovery", "Post-race recovery and range of motion.", 45, 1, 18},
	}
	for _, t := range types {
		_, err := s.db.Exec(ctx, `
			INSERT INTO catalog.class_types (id, name, description, default_duration_min, default_credit_cost, default_capacity, active)
			VALUES ($1, $2, $3, $4, $5, $6, true) ON CONFLICT (id) DO NOTHING`,
			t.id, t.name, t.description, t.duration, t.cost, t.cap)
		if err != nil {
			return 0, fmt.Errorf("seed: class type %s: %w", t.id, err)
		}
	}
	return len(types), nil
}

func (s *Seeder) seedPackages(ctx context.Context) (int, error) {
	type packageSeed struct {
		id, name string
		credits  int
		price    int64
		validity int
		coverage *string
		limitPer *int
	}
	simulationOnly := `["clt_simulation"]`
	one := 1

	packages := []packageSeed{
		{id: "pkg_trial", name: "Trial Class", credits: 1, price: 200_000, validity: 14, limitPer: &one},
		{id: "pkg_starter5", name: "Starter 5", credits: 5, price: 800_000, validity: 60},
		{id: "pkg_visit10", name: "10 Visit Pack", credits: 10, price: 1_500_000, validity: 90},
		{id: "pkg_visit20", name: "20 Visit Pack", credits: 20, price: 2_700_000, validity: 120},
		{id: "pkg_race", name: "Race Prep Block", credits: 8, price: 1_400_000, validity: 60, coverage: &simulationOnly},
	}
	for _, p := range packages {
		_, err := s.db.Exec(ctx, `
			INSERT INTO catalog.credit_packages (id, name, credits, price_idr, validity_days,
				purchase_limit_per_member, applicable_class_type_ids, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'ACTIVE') ON CONFLICT (id) DO NOTHING`,
			p.id, p.name, p.credits, p.price, p.validity, p.limitPer, p.coverage)
		if err != nil {
			return 0, fmt.Errorf("seed: package %s: %w", p.id, err)
		}
	}
	return len(packages), nil
}

func (s *Seeder) seedVouchers(ctx context.Context) error {
	now := s.clock.Now()
	vouchers := []struct {
		id, code, kind string
		value          int64
		segment        string
		status         string
	}{
		{"vou_welcome10", "WELCOME10", "PERCENT", 10, "NEW_MEMBERS", "ACTIVE"},
		{"vou_hyrox100", "HYROX100", "FIXED_IDR", 100_000, "ALL", "ACTIVE"},
		{"vou_buddy", "BUDDYPASS", "FIXED_IDR", 150_000, "ALL", "DRAFT"},
	}
	for _, v := range vouchers {
		_, err := s.db.Exec(ctx, `
			INSERT INTO wallet.vouchers (id, code, type, value, starts_at, ends_at, per_member_limit,
				eligible_segment, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8, $5) ON CONFLICT (id) DO NOTHING`,
			v.id, v.code, v.kind, v.value, now.AddDate(0, 0, -30), now.AddDate(0, 0, 60), v.segment, v.status)
		if err != nil {
			return fmt.Errorf("seed: voucher %s: %w", v.id, err)
		}
	}
	return nil
}

func (s *Seeder) seedAdminUsers(ctx context.Context) (int, error) {
	users := []struct {
		id, name, email, role string
		branch                *string
	}{
		{id: "adm_super", name: "Alya Santoso", email: "alya@nuhabit.id", role: "SUPER_ADMIN"},
		{id: "adm_hq", name: "Raka Wibowo", email: "raka@nuhabit.id", role: "HQ_ADMIN"},
		{id: "adm_branch", name: "Bima Prasetyo", email: "bima@nuhabit.id", role: "BRANCH_MANAGER", branch: strPtr("brn_senopati")},
		{id: "adm_desk", name: "Nadia Putri", email: "nadia@nuhabit.id", role: "FRONT_DESK", branch: strPtr("brn_senopati")},
		{id: "adm_coach", name: "Kevin Hartono", email: "kevin@nuhabit.id", role: "COACH", branch: strPtr("brn_senopati")},
		{id: "adm_finance", name: "Sinta Halim", email: "sinta@nuhabit.id", role: "FINANCE"},
	}
	// Every demo account gets the same password. Hashed once and reused: the
	// salt is per-hash, not per-account, and hashing six times over is a
	// second of test setup for nothing.
	hash, err := s.passwords.Hash(DemoPassword)
	if err != nil {
		return 0, fmt.Errorf("seed: hashing the demo password: %w", err)
	}
	for _, u := range users {
		_, err := s.db.Exec(ctx, `
			INSERT INTO identity.admin_users
				(id, name, email, role, branch_id, password_hash, password_set_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (id) DO UPDATE
				SET password_hash = excluded.password_hash,
				    password_set_at = excluded.password_set_at
				WHERE identity.admin_users.password_hash IS NULL`,
			u.id, u.name, u.email, u.role, u.branch, hash)
		if err != nil {
			return 0, fmt.Errorf("seed: admin user %s: %w", u.id, err)
		}
	}
	return len(users), nil
}

func (s *Seeder) seedMembers(ctx context.Context) (int, error) {
	now := s.clock.Now()
	members := []struct {
		id, name, email, phone, branch, status string
		joinedDaysAgo                          int
	}{
		{"mem_demo", "Fahmi Syaban", "demo@nuhabit.id", "+628123456789", "brn_senopati", "ACTIVE", 90},
		{"mem_natalie", "Natalie Brown", "natalie@example.com", "+628123456701", "brn_pik", "ACTIVE", 60},
		{"mem_lucas", "Lucas Conn", "lucas@example.com", "+628123456702", "brn_senopati", "ACTIVE", 45},
		{"mem_jaime", "Jaime Waelchi", "jaime@example.com", "+628123456703", "brn_senopati", "ACTIVE", 30},
		{"mem_doris", "Doris Conroy", "doris@example.com", "+628123456704", "brn_pik", "ACTIVE", 21},
		{"mem_luke", "Luke Nader", "luke@example.com", "+628123456705", "brn_pik", "ACTIVE", 14},
		{"mem_sari", "Sari Wulandari", "sari@example.com", "+628123456706", "brn_senopati", "ACTIVE", 7},
		{"mem_dimas", "Dimas Anggara", "dimas@example.com", "+628123456707", "brn_senopati", "SUSPENDED", 120},
		{"mem_rina", "Rina Kartika", "rina@example.com", "+628123456708", "brn_pik", "INACTIVE", 200},
	}
	for _, m := range members {
		joined := now.AddDate(0, 0, -m.joinedDaysAgo)
		_, err := s.db.Exec(ctx, `
			INSERT INTO identity.members (id, full_name, email, phone, preferred_branch_id, status,
				waiver_version, waiver_accepted_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, '2026-01', $7, $7, $7)
			ON CONFLICT (id) DO NOTHING`,
			m.id, m.name, m.email, m.phone, m.branch, m.status, joined)
		if err != nil {
			return 0, fmt.Errorf("seed: member %s: %w", m.id, err)
		}
	}
	return len(members), nil
}

func (s *Seeder) seedExercises(ctx context.Context) (int, error) {
	type exerciseSeed struct {
		id, name, category string
		station            *int
		difficulty         int
		spec               string
	}
	station := func(n int) *int { return &n }

	exercises := []exerciseSeed{
		{"exr_ski", "SkiErg", "ERG", station(1), 2, `{"distanceM":1000,"reps":null}`},
		{"exr_sled_push", "Sled Push", "SLED", station(2), 3, `{"distanceM":50,"reps":null}`},
		{"exr_sled_pull", "Sled Pull", "SLED", station(3), 3, `{"distanceM":50,"reps":null}`},
		{"exr_burpee", "Burpee Broad Jump", "JUMP", station(4), 3, `{"distanceM":80,"reps":null}`},
		{"exr_row", "Rowing", "ERG", station(5), 2, `{"distanceM":1000,"reps":null}`},
		{"exr_carry", "Farmers Carry", "CARRY", station(6), 2, `{"distanceM":200,"reps":null}`},
		{"exr_lunge", "Sandbag Lunges", "LUNGE", station(7), 3, `{"distanceM":100,"reps":null}`},
		{"exr_wallball", "Wall Balls", "THROW", station(8), 3, `{"distanceM":null,"reps":100}`},
		{"exr_run", "Run", "RUN", nil, 1, `{"distanceM":1000,"reps":null}`},
		{"exr_airbike", "Air Bike", "CONDITIONING", nil, 2, `{"distanceM":null,"reps":null}`},
	}
	for _, e := range exercises {
		_, err := s.db.Exec(ctx, `
			INSERT INTO catalog.exercises (id, name, category, equipment, hyrox_station_order, difficulty, default_spec)
			VALUES ($1, $2, $3, '[]'::jsonb, $4, $5, $6::jsonb) ON CONFLICT (id) DO NOTHING`,
			e.id, e.name, e.category, e.station, e.difficulty, e.spec)
		if err != nil {
			return 0, fmt.Errorf("seed: exercise %s: %w", e.id, err)
		}
	}
	return len(exercises), nil
}

func (s *Seeder) seedIncentiveScheme(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO incentives.schemes (id, coach_id, session_fee_idr, per_attendee_idr,
			full_class_bonus_idr, full_class_threshold_percent, no_show_penalty_idr, active)
		VALUES ('sch_default', NULL, 150000, 15000, 50000, 80, 0, true)
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: default incentive scheme: %w", err)
	}

	// One coach on their own terms, so the demo shows what a per-coach fee
	// looks like: Rizky runs the race simulations, which are ninety minutes
	// and a lot of setup, and is paid accordingly for those specifically.
	_, err = s.db.Exec(ctx, `
		INSERT INTO incentives.schemes (id, coach_id, session_fee_idr, per_attendee_idr,
			full_class_bonus_idr, full_class_threshold_percent, no_show_penalty_idr, active)
		VALUES ('sch_rizky', 'coa_rizky', 200000, 18000, 75000, 80, 0, true)
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: coach incentive scheme: %w", err)
	}

	rates := []struct {
		id, scheme, classType string
		sessionFee, perHead   int64
	}{
		{"scr_rizky_sim", "sch_rizky", "clt_simulation", 400_000, 25_000},
		{"scr_rizky_engine", "sch_rizky", "clt_engine", 180_000, 15_000},
		// The studio pays more for a simulation whoever runs it.
		{"scr_default_sim", "sch_default", "clt_simulation", 275_000, 20_000},
	}
	for _, rate := range rates {
		_, err := s.db.Exec(ctx, `
			INSERT INTO incentives.scheme_rates
				(id, scheme_id, class_type_id, session_fee_idr, per_attendee_idr)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (scheme_id, class_type_id) DO NOTHING`,
			rate.id, rate.scheme, rate.classType, rate.sessionFee, rate.perHead)
		if err != nil {
			return fmt.Errorf("seed: class rate %s: %w", rate.id, err)
		}
	}
	return nil
}

// seedSessions lays out a fortnight of classes: a week behind, so reports and
// payroll have history, and a week ahead so there is something to book.
func (s *Seeder) seedSessions(ctx context.Context) (int, error) {
	now := s.clock.Now()
	rules := domain.DefaultBusinessRules()

	// Classes are scheduled in the studio's local time, not the server's. A
	// 07:00 class means seven in the morning in Jakarta.
	studio, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return 0, fmt.Errorf("seed: loading studio timezone: %w", err)
	}
	local := now.In(studio)

	type slot struct {
		hour, minute         int
		classTypeID, coachID string
		branchID             string
	}
	slots := []slot{
		{7, 0, "clt_fundamentals", "coa_kevin", "brn_senopati"},
		{12, 0, "clt_engine", "coa_rizky", "brn_senopati"},
		{18, 0, "clt_strength", "coa_kevin", "brn_senopati"},
		{19, 30, "clt_simulation", "coa_rizky", "brn_senopati"},
		{7, 30, "clt_engine", "coa_maya", "brn_pik"},
		{17, 0, "clt_fundamentals", "coa_maya", "brn_pik"},
		{19, 0, "clt_mobility", "coa_tara", "brn_pik"},
	}

	classTypes := map[string]struct {
		duration, cost, capacity int
	}{
		"clt_fundamentals": {60, 1, 16},
		"clt_engine":       {60, 1, 20},
		"clt_strength":     {75, 1, 14},
		"clt_simulation":   {90, 2, 12},
		"clt_mobility":     {45, 1, 18},
	}

	created := 0
	for dayOffset := -7; dayOffset <= 7; dayOffset++ {
		day := local.AddDate(0, 0, dayOffset)
		for i, sl := range slots {
			startsAt := time.Date(day.Year(), day.Month(), day.Day(), sl.hour, sl.minute, 0, 0, studio)
			spec := classTypes[sl.classTypeID]

			// Past classes are completed; future ones are open for booking.
			status := domain.SessionPublished
			if startsAt.Before(now.Add(-time.Duration(spec.duration) * time.Minute)) {
				status = domain.SessionCompleted
			}
			opens, closes := domain.DeriveBookingWindow(startsAt, rules)
			sessionID := fmt.Sprintf("ses_%s_%d_%d", sl.branchID[4:], dayOffset+7, i)

			_, err = s.db.Exec(ctx, `
				INSERT INTO scheduling.class_sessions (id, class_type_id, branch_id, coach_id, starts_at, ends_at,
					capacity, credit_cost, booking_opens_at, booking_closes_at, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT (id) DO NOTHING`,
				sessionID, sl.classTypeID, sl.branchID, sl.coachID, startsAt,
				startsAt.Add(time.Duration(spec.duration)*time.Minute),
				spec.capacity, spec.cost, opens, closes, status)
			if err != nil {
				return 0, fmt.Errorf("seed: session %s: %w", sessionID, err)
			}
			created++
		}
	}
	return created, nil
}

func strPtr(v string) *string { return &v }
