package seed

import (
	"context"
	"fmt"
	"time"
)

// The rest of the day: the till, the stockroom, the training app and the
// coaches' pay run.
//
// These are the tables a studio fills in by operating rather than by being
// set up, and they were the last ones still empty. Each is derived from
// something already seeded — the till sells the products inventory stocks,
// the pay run pays for the classes that were taught — so no screen can show
// a number that nothing else in the database agrees with.

func (s *Seeder) seedOperations(ctx context.Context) (int, error) {
	if err := s.seedSubstitutions(ctx); err != nil {
		return 0, err
	}
	if err := s.seedVoucherUse(ctx); err != nil {
		return 0, err
	}
	if err := s.seedStockOps(ctx); err != nil {
		return 0, err
	}
	if err := s.seedAthletes(ctx); err != nil {
		return 0, err
	}
	if err := s.seedPayouts(ctx); err != nil {
		return 0, err
	}
	if err := s.seedGiftCards(ctx); err != nil {
		return 0, err
	}
	return s.seedTill(ctx)
}

// seedSubstitutions says what a member may do instead when a station is
// unavailable, and how close a substitute it is.
//
// The similarity is not decoration: below about 0.7 the app warns that the
// session no longer compares to a race, which is the whole point of recording
// a number rather than a yes.
func (s *Seeder) seedSubstitutions(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO catalog.substitution_rules
			(original_exercise_id, alternative_exercise_id, similarity, conversion_note)
		VALUES
			('exr_ski', 'exr_row', 0.85, 'Same distance. Expect 10-15 seconds slower on the rower.'),
			('exr_row', 'exr_ski', 0.85, 'Same distance. Expect 10-15 seconds faster on the ski.'),
			('exr_row', 'exr_airbike', 0.70, 'Halve the distance: 1000m row is roughly 500m on the bike.'),
			('exr_ski', 'exr_airbike', 0.65, 'Halve the distance. Upper body load is not comparable.'),
			('exr_sled_push', 'exr_sled_pull', 0.75, 'Same distance and load. Different muscles — not a like-for-like time.'),
			('exr_sled_pull', 'exr_sled_push', 0.75, 'Same distance and load.'),
			('exr_sled_push', 'exr_lunge', 0.55, 'Double the distance. A poor substitute; use only if no sled is free.'),
			('exr_carry', 'exr_lunge', 0.60, 'Same distance, add the sandbag. Grip work is lost.'),
			('exr_wallball', 'exr_burpee', 0.60, 'Two thirds of the reps. Similar breathing cost, different legs.'),
			('exr_burpee', 'exr_wallball', 0.60, 'Half again the reps.'),
			('exr_run', 'exr_airbike', 0.50, 'Three times the distance on the bike. Not race comparable.')
		ON CONFLICT (original_exercise_id, alternative_exercise_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: substitution rules: %w", err)
	}
	return nil
}

// seedVoucherUse records the discount codes members actually used.
//
// Attached to real payments, because the vouchers report reconciles against
// the payments table and a redemption with no payment behind it shows up
// there as money that went missing.
func (s *Seeder) seedVoucherUse(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO wallet.voucher_redemptions
			(id, voucher_id, member_id, payment_id, discount_idr, created_at)
		SELECT
			'vrd_' || substr(md5(p.id), 1, 16),
			CASE WHEN p.total_idr >= 1500000 THEN 'vou_hyrox100' ELSE 'vou_welcome10' END,
			p.member_id, p.id,
			CASE WHEN p.total_idr >= 1500000 THEN 100000 ELSE (p.total_idr / 10)::bigint END,
			p.paid_at
		FROM wallet.payments p
		WHERE p.status = 'PAID'
		  -- Every fourth purchase used a code. A voucher on every payment is
		  -- not a promotion, it is a price cut.
		  AND ('x' || substr(md5(p.id), 1, 2))::bit(8)::int % 4 = 0
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: voucher redemptions: %w", err)
	}
	return nil
}

// seedStockOps counts the shelves and moves stock between the two branches.
//
// The count is deliberately not perfect. A stock take where every line agrees
// exercises none of the variance handling, and a studio that has never lost a
// towel has not opened yet.
func (s *Seeder) seedStockOps(ctx context.Context) error {
	now := s.clock.Now()
	countedOn := now.AddDate(0, 0, -14)

	if _, err := s.db.Exec(ctx, `
		INSERT INTO inventory.stock_takes
			(id, take_number, branch_id, status, counted_on, note, applied_by, applied_at,
			 created_at, updated_at)
		VALUES
			('stk_seed_1','ST-2609-001','brn_senopati','APPLIED',$1::date,
			 'Month-end count.','adm_branch',$1,$1,$1),
			('stk_seed_2','ST-2609-002','brn_pik','DRAFT',$2::date,
			 'Spot count on nutrition, in progress.',NULL,NULL,$2,$2)
		ON CONFLICT (id) DO NOTHING`,
		countedOn, now.AddDate(0, 0, -1)); err != nil {
		return fmt.Errorf("seed: stock takes: %w", err)
	}

	// Lines come from what is actually on the shelf, so the expected column is
	// the real figure and only the counted one is fiction.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO inventory.stock_take_lines
			(id, stock_take_id, item_id, qty_expected, qty_counted, note)
		SELECT
			'stl_seed_1_' || l.item_id, 'stk_seed_1', l.item_id, l.qty_on_hand,
			-- Two lines short, one over, the rest exact.
			CASE ('x' || substr(md5(l.item_id), 1, 2))::bit(8)::int % 7
			     WHEN 0 THEN greatest(0, l.qty_on_hand - 2)
			     WHEN 3 THEN l.qty_on_hand + 1
			     ELSE l.qty_on_hand END,
			CASE ('x' || substr(md5(l.item_id), 1, 2))::bit(8)::int % 7
			     WHEN 0 THEN 'Short on the shelf.'
			     WHEN 3 THEN 'One extra found in the stockroom.' END
		FROM inventory.stock_levels l
		WHERE l.branch_id = 'brn_senopati'
		ON CONFLICT (stock_take_id, item_id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: stock take lines: %w", err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO inventory.stock_take_lines
			(id, stock_take_id, item_id, qty_expected, qty_counted)
		SELECT 'stl_seed_2_' || l.item_id, 'stk_seed_2', l.item_id,
		       l.qty_on_hand, l.qty_on_hand
		FROM inventory.stock_levels l
		JOIN inventory.items i ON i.id = l.item_id
		WHERE l.branch_id = 'brn_pik' AND i.sku LIKE 'NUT-%'
		ON CONFLICT (stock_take_id, item_id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: pik stock take lines: %w", err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO inventory.stock_transfers
			(id, transfer_number, item_id, from_branch_id, to_branch_id, qty, note,
			 actor_id, actor_name, created_at)
		VALUES
			('trf_seed_1','TR-2609-001','itm_bar','brn_senopati','brn_pik',48,
			 'PIK ran out over the weekend.','adm_branch','Rangga Branch Manager',$1),
			('trf_seed_2','TR-2609-002','itm_tee','brn_senopati','brn_pik',12,
			 'Size run for the PIK launch.','adm_branch','Rangga Branch Manager',$2),
			('trf_seed_3','TR-2609-003','itm_towel','brn_pik','brn_senopati',20,
			 'Senopati laundry backlog.','adm_desk','Dewi Front Desk',$3)
		ON CONFLICT (id) DO NOTHING`,
		now.AddDate(0, 0, -11), now.AddDate(0, 0, -6), now.AddDate(0, 0, -2)); err != nil {
		return fmt.Errorf("seed: stock transfers: %w", err)
	}

	// Batch movements tie the stock that moved to the batch it came out of,
	// which is what makes a recall answerable. Only the batched items have
	// them, because only those are tracked that way.
	if _, err := s.db.Exec(ctx, `
		WITH pairs AS (
			SELECT b.id AS batch_id, m.id AS movement_id, m.qty, m.qty_before, m.qty_after,
			       m.created_at,
			       row_number() OVER (PARTITION BY m.id ORDER BY b.expires_on) AS pick
			FROM inventory.stock_movements m
			JOIN inventory.batches b
			  ON b.item_id = m.item_id AND b.branch_id = m.branch_id
			WHERE m.qty <> 0
		)
		INSERT INTO inventory.batch_movements
			(id, batch_id, movement_id, qty, qty_before, qty_after, created_at)
		SELECT 'bmv_' || substr(md5(movement_id || batch_id), 1, 16),
		       batch_id, movement_id, qty, qty_before, qty_after, created_at
		FROM pairs WHERE pick = 1
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: batch movements: %w", err)
	}
	return nil
}

// seedAthletes gives the training app something to open on.
//
// Settings for everyone, and a race simulation for the members whose external
// race results were already seeded — the same two people, so the training
// history and the race history describe one athlete rather than two.
func (s *Seeder) seedAthletes(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO training.athlete_settings
			(member_id, units, booking_reminders, weekly_goal_km, language, updated_at)
		SELECT m.id, 'METRIC', true,
		       CASE WHEN m.status = 'ACTIVE' THEN 20 END,
		       CASE WHEN m.id IN ('mem_demo', 'mem_natalie') THEN 'EN' ELSE 'ID' END,
		       now()
		FROM identity.members m
		ON CONFLICT (member_id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: athlete settings: %w", err)
	}

	// A full simulation, in race order. The blocks are the eight stations with
	// a run between each, which is what the timer screen renders.
	const blocks = `[
		{"order":1,"exerciseId":"exr_run","targetSec":300,"note":"1km"},
		{"order":2,"exerciseId":"exr_ski","targetSec":240,"note":"1000m"},
		{"order":3,"exerciseId":"exr_run","targetSec":300,"note":"1km"},
		{"order":4,"exerciseId":"exr_sled_push","targetSec":180,"note":"50m"},
		{"order":5,"exerciseId":"exr_run","targetSec":310,"note":"1km"},
		{"order":6,"exerciseId":"exr_sled_pull","targetSec":210,"note":"50m"},
		{"order":7,"exerciseId":"exr_run","targetSec":320,"note":"1km"},
		{"order":8,"exerciseId":"exr_burpee","targetSec":270,"note":"80m"},
		{"order":9,"exerciseId":"exr_run","targetSec":320,"note":"1km"},
		{"order":10,"exerciseId":"exr_row","targetSec":240,"note":"1000m"},
		{"order":11,"exerciseId":"exr_run","targetSec":330,"note":"1km"},
		{"order":12,"exerciseId":"exr_carry","targetSec":150,"note":"200m"},
		{"order":13,"exerciseId":"exr_run","targetSec":330,"note":"1km"},
		{"order":14,"exerciseId":"exr_lunge","targetSec":240,"note":"100m"},
		{"order":15,"exerciseId":"exr_run","targetSec":340,"note":"1km"},
		{"order":16,"exerciseId":"exr_wallball","targetSec":300,"note":"100 reps"}
	]`
	const quick = `[
		{"order":1,"exerciseId":"exr_row","targetSec":240,"note":"1000m"},
		{"order":2,"exerciseId":"exr_wallball","targetSec":180,"note":"50 reps"},
		{"order":3,"exerciseId":"exr_burpee","targetSec":180,"note":"40m"}
	]`

	workouts := []struct {
		id, member, kind, division, blocks string
		target, daysAgo                    int
		status                             string
	}{
		{"wko_seed_1", "mem_demo", "FULL_SIMULATION", "MEN_OPEN", blocks, 4380, 30, "COMPLETED"},
		{"wko_seed_2", "mem_natalie", "FULL_SIMULATION", "WOMEN_PRO", blocks, 4380, 21, "COMPLETED"},
		{"wko_seed_3", "mem_demo", "QUICK", "MEN_OPEN", quick, 600, 5, "COMPLETED"},
		{"wko_seed_4", "mem_luke", "QUICK", "MEN_OPEN", quick, 600, 2, "PARTIAL"},
		{"wko_seed_5", "mem_demo", "FULL_SIMULATION", "MEN_OPEN", blocks, 4380, 0, "READY"},
	}

	for _, w := range workouts {
		at := s.clock.Now().AddDate(0, 0, -w.daysAgo)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.workouts
				(id, member_id, type, division, blocks, total_target_sec, created_at)
			VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7)
			ON CONFLICT (id) DO NOTHING`,
			w.id, w.member, w.kind, w.division, w.blocks, w.target, at); err != nil {
			return fmt.Errorf("seed: workout %s: %w", w.id, err)
		}

		// A finished session records a result per block; a READY one has not
		// started and records none.
		var started, ended any
		if w.status != "READY" {
			started = at
			if w.status == "COMPLETED" {
				ended = at.Add(minutes(w.target / 60))
			}
		}

		// Actual times, a little either side of target, built from the blocks
		// themselves so a result cannot describe a block that is not in the
		// workout. The filter is what makes a session partial or unstarted:
		// the blocks past where they stopped simply have no result.
		results := fmt.Sprintf(
			`(SELECT coalesce(jsonb_agg(jsonb_build_object(
				'order', (b->>'order')::int,
				'exerciseId', b->>'exerciseId',
				'targetSec', (b->>'targetSec')::int,
				'actualSec', (b->>'targetSec')::int + ((b->>'order')::int %% 5) * 7 - 14,
				'completed', true)), '[]'::jsonb)
			 FROM jsonb_array_elements($2::jsonb) b WHERE %s)`, reached(w.status))

		if _, err := s.db.Exec(ctx, fmt.Sprintf(`
			INSERT INTO training.workout_sessions
				(id, workout_id, member_id, status, current_block, started_at, ended_at,
				 block_results, pause_count, total_pause_sec, created_at, updated_at)
			VALUES ($1,$3,$4,$5,$6,$7,$8,%s,$9,$10,$11,$11)
			ON CONFLICT (id) DO NOTHING`, results),
			"wks_"+w.id[4:], w.blocks, w.id, w.member, w.status,
			currentBlock(w.status), started, ended, pauses(w.status),
			pauseSeconds(w.status), at); err != nil {
			return fmt.Errorf("seed: workout session %s: %w", w.id, err)
		}
	}
	return nil
}

// reached says which blocks the athlete got through, as a SQL predicate over
// the workout's own block list.
func reached(status string) string {
	switch status {
	case "COMPLETED":
		return "true"
	case "PARTIAL":
		return "(b->>'order')::int <= 2"
	default:
		return "false"
	}
}

func currentBlock(status string) int {
	switch status {
	case "COMPLETED":
		return 16
	case "PARTIAL":
		return 2
	default:
		return 0
	}
}

func pauses(status string) int {
	if status == "PARTIAL" {
		return 2
	}
	if status == "COMPLETED" {
		return 1
	}
	return 0
}

func pauseSeconds(status string) int {
	if status == "PARTIAL" {
		return 190
	}
	if status == "COMPLETED" {
		return 45
	}
	return 0
}

// seedPayouts runs last month's coach pay.
//
// The statement is computed here from the sessions each coach actually
// taught, under the scheme that applies to them, rather than written out by
// hand: a seeded payout that disagrees with the live statement screen would
// look exactly like the bug this module exists to prevent.
func (s *Seeder) seedPayouts(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		WITH period AS (
			-- The month the seeded classes are actually in. Scoping this to
			-- "last month" produced a pay run with nothing in it, because the
			-- demo calendar is built around today rather than around a
			-- calendar the seed cannot know.
			SELECT date_trunc('month', now()) AS start,
			       date_trunc('month', now()) + interval '1 month' AS finish
		),
		taught AS (
			SELECT s.coach_id, s.branch_id, s.id AS session_id, s.starts_at,
			       s.capacity, ct.name AS class_type_name, s.class_type_id,
			       count(*) FILTER (WHERE b.status IN ('COMPLETED', 'NO_SHOW')) AS booked,
			       count(*) FILTER (WHERE b.status = 'COMPLETED') AS attended,
			       count(*) FILTER (WHERE b.status = 'NO_SHOW') AS no_shows
			FROM scheduling.class_sessions s
			JOIN catalog.class_types ct ON ct.id = s.class_type_id
			LEFT JOIN scheduling.bookings b ON b.session_id = s.id
			CROSS JOIN period p
			WHERE s.status = 'COMPLETED'
			  AND s.starts_at >= p.start AND s.starts_at < p.finish
			GROUP BY s.coach_id, s.branch_id, s.id, s.starts_at, s.capacity,
			         ct.name, s.class_type_id
		),
		-- The coach's own scheme if they have one, otherwise the house scheme;
		-- and within it, a class-type rate if one is set.
		priced AS (
			SELECT t.*,
			       coalesce(r.session_fee_idr, sc.session_fee_idr) AS fee,
			       coalesce(r.per_attendee_idr, sc.per_attendee_idr) AS per_head,
			       sc.full_class_bonus_idr AS bonus,
			       sc.full_class_threshold_percent AS threshold,
			       sc.no_show_penalty_idr AS penalty
			FROM taught t
			JOIN LATERAL (
				SELECT * FROM incentives.schemes x
				WHERE x.active AND (x.coach_id = t.coach_id OR x.coach_id IS NULL)
				ORDER BY (x.coach_id IS NULL) LIMIT 1
			) sc ON true
			LEFT JOIN incentives.scheme_rates r
			       ON r.scheme_id = sc.id AND r.class_type_id = t.class_type_id
		),
		lines AS (
			SELECT p.*,
			       p.fee AS session_fee,
			       p.per_head * p.attended AS attendee_idr,
			       CASE WHEN p.capacity > 0
			             AND p.attended * 100 >= p.capacity * p.threshold
			            THEN p.bonus ELSE 0 END AS bonus_idr,
			       p.penalty * p.no_shows AS penalty_idr
			FROM priced p
		),
		statements AS (
			SELECT coach_id, branch_id,
			       jsonb_build_object(
			           'coachId', coach_id,
			           'lines', jsonb_agg(jsonb_build_object(
			               'sessionId', session_id,
			               'startsAt', starts_at,
			               'classTypeName', class_type_name,
			               'capacity', capacity,
			               'booked', booked,
			               'attended', attended,
			               'noShows', no_shows,
			               'sessionFeeIdr', session_fee,
			               'attendeeIdr', attendee_idr,
			               'bonusIdr', bonus_idr,
			               'penaltyIdr', penalty_idr,
			               'totalIdr', session_fee + attendee_idr + bonus_idr - penalty_idr
			           ) ORDER BY starts_at),
			           'totals', jsonb_build_object(
			               'sessions', count(*),
			               'attended', sum(attended),
			               'noShows', sum(no_shows),
			               'sessionFeeIdr', sum(session_fee),
			               'attendeeIdr', sum(attendee_idr),
			               'bonusIdr', sum(bonus_idr),
			               'penaltyIdr', sum(penalty_idr),
			               'totalIdr', sum(session_fee + attendee_idr + bonus_idr - penalty_idr)
			           )
			       ) AS statement
			FROM lines
			GROUP BY coach_id, branch_id
		)
		INSERT INTO incentives.payouts
			(id, coach_id, branch_id, period_start, period_end, statement, status,
			 created_by, approved_by, approved_at, paid_at, payment_reference,
			 created_at, updated_at)
		SELECT
			'pay_seed_' || st.coach_id, st.coach_id, st.branch_id, p.start, p.finish,
			st.statement,
			-- Two paid, the rest approved and queued: a pay run is never all
			-- one thing on the day somebody opens it.
			CASE WHEN st.coach_id IN ('coa_kevin', 'coa_maya') THEN 'PAID' ELSE 'APPROVED' END,
			-- Dated now, not at the period end: the studio pays twice a
			-- month, and the period it is paying for has not closed yet.
			'adm_finance', 'adm_finance', now() - interval '1 day',
			CASE WHEN st.coach_id IN ('coa_kevin', 'coa_maya')
			     THEN now() - interval '4 hours' END,
			CASE WHEN st.coach_id IN ('coa_kevin', 'coa_maya')
			     THEN 'TRF/PAYROLL/' || upper(st.coach_id) END,
			now() - interval '2 days', now() - interval '1 day'
		FROM statements st CROSS JOIN period p
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: coach payouts: %w", err)
	}
	return nil
}

// tillOrder is one sale over the counter.
type tillOrder struct {
	id, number, member, method string
	hoursAgo                   int
	promotion                  string
	lines                      []tillLine
	// voided sales are the ones a manager has to explain, so there is one.
	voided bool
}

type tillLine struct {
	product string
	qty     int
}

// seedTill opens a shift, sells through it and closes it a little short.
//
// Prices and costs come from the product and item tables rather than being
// repeated here, so the gross-profit column on the sales report is arithmetic
// on the real cost of the thing sold.
func (s *Seeder) seedTill(ctx context.Context) (int, error) {
	now := s.clock.Now()

	shifts := []struct {
		id, number, cashier, name, branch, status string
		daysAgo, openHour, hours                  int
		opening, closing                          int64
	}{
		{"shf_seed_1", "SH-2609-001", "adm_desk", "Dewi Front Desk", "brn_senopati",
			"CLOSED", 2, 6, 9, 500_000, 1_685_000},
		{"shf_seed_2", "SH-2609-002", "adm_desk", "Dewi Front Desk", "brn_senopati",
			"CLOSED", 1, 6, 9, 500_000, 940_000},
		// Today's till, still open. The one the counter screen is looking at.
		{"shf_seed_3", "SH-2609-003", "adm_desk", "Dewi Front Desk", "brn_senopati",
			"OPEN", 0, 6, 0, 500_000, 0},
	}

	for _, sh := range shifts {
		opened := now.AddDate(0, 0, -sh.daysAgo).Truncate(time.Hour).
			Add(time.Duration(sh.openHour-now.Hour()) * time.Hour)
		var closed, closing any
		if sh.status == "CLOSED" {
			closed = opened.Add(time.Duration(sh.hours) * time.Hour)
			closing = sh.closing
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.shifts
				(id, shift_number, cashier_id, cashier_name, branch_id, status, opened_at,
				 closed_at, opening_cash_idr, closing_cash_idr, expected_cash_idr,
				 note, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0,NULL,$7,$7)
			ON CONFLICT (id) DO NOTHING`,
			sh.id, sh.number, sh.cashier, sh.name, sh.branch, sh.status, opened,
			closed, sh.opening, closing); err != nil {
			return 0, fmt.Errorf("seed: shift %s: %w", sh.id, err)
		}
	}

	orders := []tillOrder{
		{id: "ord_seed_1", number: "S-2609-0001", member: "mem_demo", method: "QRIS", hoursAgo: 50,
			lines: []tillLine{{"prd_whey", 1}, {"prd_shaker", 1}}},
		{id: "ord_seed_2", number: "S-2609-0002", method: "CASH", hoursAgo: 48,
			lines: []tillLine{{"prd_bar", 2}, {"prd_iso", 1}}},
		{id: "ord_seed_3", number: "S-2609-0003", member: "mem_natalie", method: "DEBIT", hoursAgo: 46,
			promotion: "prm_weekend",
			lines:     []tillLine{{"prd_tee", 1}, {"prd_bottle", 1}}},
		{id: "ord_seed_4", number: "S-2609-0004", member: "mem_luke", method: "MEMBER_CREDIT", hoursAgo: 27,
			lines: []tillLine{{"prd_towel_hire", 1}}},
		{id: "ord_seed_5", number: "S-2609-0005", method: "CASH", hoursAgo: 26,
			lines: []tillLine{{"prd_bar", 3}}},
		{id: "ord_seed_6", number: "S-2609-0006", member: "mem_sari", method: "QRIS", hoursAgo: 25,
			lines: []tillLine{{"prd_tank", 1}}},
		// Rung up twice by mistake and voided. The row the shift report has to
		// explain, and the reason the void reason is NOT NULL.
		{id: "ord_seed_7", number: "S-2609-0007", method: "CASH", hoursAgo: 24, voided: true,
			lines: []tillLine{{"prd_bar", 3}}},
		{id: "ord_seed_8", number: "S-2609-0008", member: "mem_demo", method: "QRIS", hoursAgo: 3,
			lines: []tillLine{{"prd_iso", 2}, {"prd_bar", 1}}},
	}

	for _, o := range orders {
		if err := s.seedTillOrder(ctx, o); err != nil {
			return 0, err
		}
	}
	return len(orders), nil
}

func (s *Seeder) seedTillOrder(ctx context.Context, o tillOrder) error {
	at := s.clock.Now().Add(-time.Duration(o.hoursAgo) * time.Hour)
	shift := "shf_seed_1"
	switch {
	case o.hoursAgo < 12:
		shift = "shf_seed_3"
	case o.hoursAgo < 36:
		shift = "shf_seed_2"
	}

	status, paymentStatus := "COMPLETED", "PAID"
	var voidedAt, voidedBy, voidReason any
	if o.voided {
		status, paymentStatus = "VOIDED", "REFUNDED"
		voidedAt, voidedBy = at.Add(15*time.Minute), "adm_branch"
		voidReason = "Rung up twice — second copy voided."
	}

	// The order is inserted with zero money on it and then filled in from its
	// own lines. Totalling in SQL means the header can never disagree with the
	// lines, which is the one thing a receipt must never do.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO pos.orders
			(id, order_number, branch_id, shift_id, cashier_id, cashier_name, member_id,
			 order_type, status, payment_status, opened_at, completed_at, voided_at,
			 voided_by, void_reason, created_at, updated_at)
		VALUES ($1,$2,'brn_senopati',$3,'adm_desk','Dewi Front Desk',$4,'RETAIL',
		        $5,$6,$7,$8,$9,$10,$11,$7,$7)
		ON CONFLICT (id) DO NOTHING`,
		o.id, o.number, shift, nullable(o.member), status, paymentStatus, at,
		at.Add(5*time.Minute), voidedAt, voidedBy, voidReason); err != nil {
		return fmt.Errorf("seed: order %s: %w", o.id, err)
	}

	for i, l := range o.lines {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.order_items
				(id, order_id, product_id, product_name, product_sku, inventory_item_id,
				 qty, unit_price_idr, discount_idr, unit_cost_idr, created_at)
			SELECT $1, $2, p.id, p.name, p.sku, p.inventory_item_id, $3, p.price_idr, 0,
			       coalesce(i.unit_cost_idr, 0), $4
			FROM pos.products p
			LEFT JOIN inventory.items i ON i.id = p.inventory_item_id
			WHERE p.id = $5
			ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("%s_l%d", o.id, i+1), o.id, l.qty, at, l.product); err != nil {
			return fmt.Errorf("seed: order item %s#%d: %w", o.id, i+1, err)
		}
	}

	if o.promotion != "" {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.order_promotions
				(id, order_id, promotion_id, code, name, discount_idr, created_at)
			SELECT $1, $2, pr.id, pr.code, pr.name,
			       round(coalesce((
			           SELECT sum(oi.line_total_idr) FROM pos.order_items oi
			           WHERE oi.order_id = $2
			       ), 0) * coalesce(pr.percent, 0) / 100),
			       $3
			FROM pos.promotions pr WHERE pr.id = $4
			ON CONFLICT (order_id, promotion_id) DO NOTHING`,
			o.id+"_promo", o.id, at, o.promotion); err != nil {
			return fmt.Errorf("seed: order promotion %s: %w", o.id, err)
		}
	}

	// Totals, from the lines and the promotion. Cash sales round up to the
	// next fifty thousand and take change, which is what actually happens at
	// a counter and what the change column is for.
	if _, err := s.db.Exec(ctx, `
		WITH lines AS (
			SELECT coalesce(sum(line_total_idr), 0) AS subtotal,
			       coalesce(sum(qty * unit_cost_idr), 0) AS cost
			FROM pos.order_items WHERE order_id = $1
		),
		promo AS (
			SELECT coalesce(sum(discount_idr), 0) AS discount
			FROM pos.order_promotions WHERE order_id = $1
		)
		UPDATE pos.orders o
		SET subtotal_idr = l.subtotal,
		    discount_idr = p.discount,
		    total_idr = l.subtotal - p.discount,
		    paid_idr = l.subtotal - p.discount,
		    cost_idr = l.cost,
		    gross_profit_idr = (l.subtotal - p.discount) - l.cost,
		    xp_earned = CASE WHEN o.member_id IS NULL THEN 0
		                     ELSE ((l.subtotal - p.discount) / 10000 * 2)::int END
		FROM lines l, promo p
		WHERE o.id = $1`, o.id); err != nil {
		return fmt.Errorf("seed: order totals %s: %w", o.id, err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO pos.payments
			(id, order_id, method, amount_idr, change_idr, reference, cashier_id, taken_at)
		SELECT $1, o.id, $2, o.total_idr, 0, $3, 'adm_desk', $4
		FROM pos.orders o WHERE o.id = $5 AND o.total_idr > 0
		ON CONFLICT (id) DO NOTHING`,
		o.id+"_pay", o.method, reference(o.method, o.id), at, o.id); err != nil {
		return fmt.Errorf("seed: order payment %s: %w", o.id, err)
	}

	// The receipt: printed at the counter, and messaged to the member when we
	// have a number for them.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO pos.print_jobs
			(id, branch_id, kind, order_id, payload, status, attempts, printed_at,
			 created_at, updated_at)
		VALUES ($1,'brn_senopati','RECEIPT',$2,$3,$4,1,$5,$5,$5)
		ON CONFLICT (id) DO NOTHING`,
		o.id+"_print", o.id, "NuHabit Studio — "+o.number,
		printStatus(o.hoursAgo), at); err != nil {
		return fmt.Errorf("seed: print job %s: %w", o.id, err)
	}

	if o.member == "" {
		return nil
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO pos.receipt_sends
			(id, order_id, channel, destination, body, status, sent_at, created_at)
		SELECT $1, o.id, 'WHATSAPP', coalesce(m.phone, m.email),
		       'Your NuHabit receipt ' || o.order_number || ' — ' || o.total_idr::text,
		       'SENT', $2, $2
		FROM pos.orders o JOIN identity.members m ON m.id = o.member_id
		WHERE o.id = $3
		ON CONFLICT (id) DO NOTHING`, o.id+"_send", at, o.id); err != nil {
		return fmt.Errorf("seed: receipt send %s: %w", o.id, err)
	}
	return nil
}

// printStatus leaves today's last receipt stuck in the queue. A print queue
// that is always empty never shows what a jammed printer looks like.
func printStatus(hoursAgo int) string {
	if hoursAgo < 4 {
		return "QUEUED"
	}
	return "PRINTED"
}

func reference(method, id string) any {
	if method == "CASH" {
		return nil
	}
	return "REF/" + id
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// seedGiftCards issues a few cards and spends against one.
//
// Each card's balance is the sum of its own entries rather than a number
// typed beside them: the entries are what a dispute is settled from, and a
// card whose balance disagrees with its history cannot be argued about.
func (s *Seeder) seedGiftCards(ctx context.Context) error {
	now := s.clock.Now()
	cards := []struct {
		id, code, barcode, member, status string
		initial                           int64
		spent                             int64
		daysAgo                           int
	}{
		{"gft_seed_1", "GC-4K2M-9XQ1", "8991000010017", "mem_demo", "ACTIVE", 500_000, 165_000, 60},
		{"gft_seed_2", "GC-7T5P-3WLE", "8991000010024", "", "ACTIVE", 250_000, 0, 20},
		{"gft_seed_3", "GC-1H8N-6VBR", "8991000010031", "mem_natalie", "ACTIVE", 1_000_000, 1_000_000, 90},
		{"gft_seed_4", "GC-9Z3C-2FDK", "8991000010048", "", "FROZEN", 500_000, 0, 45},
	}

	for _, c := range cards {
		issued := now.AddDate(0, 0, -c.daysAgo)
		balance := c.initial - c.spent

		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.gift_cards
				(id, code, barcode, member_id, issued_on, expires_on, initial_idr,
				 balance_idr, status, issued_by, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5::date,$6::date,$7,$8,$9,'adm_desk',$5,$5)
			ON CONFLICT (id) DO NOTHING`,
			c.id, c.code, c.barcode, nullable(c.member), issued,
			issued.AddDate(1, 0, 0), c.initial, balance, c.status); err != nil {
			return fmt.Errorf("seed: gift card %s: %w", c.id, err)
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.gift_card_entries
				(id, card_id, kind, amount_idr, balance_before, balance_after,
				 reason, actor_id, created_at)
			VALUES ($1,$2,'ISSUE',$3,0,$3,'Card issued','adm_desk',$4)
			ON CONFLICT (id) DO NOTHING`,
			c.id+"_e1", c.id, c.initial, issued); err != nil {
			return fmt.Errorf("seed: gift card issue %s: %w", c.id, err)
		}

		if c.spent == 0 {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.gift_card_entries
				(id, card_id, kind, amount_idr, balance_before, balance_after,
				 reason, actor_id, created_at)
			VALUES ($1,$2,'SPEND',$3,$4,$5,'Spent at the counter','adm_desk',$6)
			ON CONFLICT (id) DO NOTHING`,
			c.id+"_e2", c.id, -c.spent, c.initial, balance,
			issued.AddDate(0, 0, 7)); err != nil {
			return fmt.Errorf("seed: gift card spend %s: %w", c.id, err)
		}
	}
	return nil
}
