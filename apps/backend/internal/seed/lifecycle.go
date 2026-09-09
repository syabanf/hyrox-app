package seed

import (
	"context"
	"fmt"
)

// The member lifecycle: money in, classes booked, doors opened.
//
// Without this the demo looked broken rather than empty. Every member had a
// zero balance, every class read 0/16, the visits report was blank and the
// dashboard's headline numbers were all zero — a panel full of screens that
// each looked like a bug. The studio core is the one part of the seed where
// the *relationships* matter more than the rows: a visit deduction has to
// belong to a booking, which has to belong to a session the member could
// actually have attended.
//
// It is written in SQL against the seeded sessions rather than as a fixed
// list, because the session ids are generated at seed time and the calendar
// moves with the clock.

// seedLifecycle fills the wallet, the register and the door log.
func (s *Seeder) seedLifecycle(ctx context.Context) (int, error) {
	if err := s.seedTopUps(ctx); err != nil {
		return 0, err
	}
	booked, err := s.seedBookings(ctx)
	if err != nil {
		return 0, err
	}
	if err := s.seedVisits(ctx); err != nil {
		return 0, err
	}
	return booked, nil
}

// topUps is who bought what.
//
// Members buy repeatedly, over months, because a studio's regulars do — and
// because attendance is capped by credits further down. One pack each gave
// the whole roster about forty visits between them, which left most past
// classes with an empty roster and made every attendance screen look broken.
//
// The tail of the list is the unhappy paths: between them they cover every
// payment state the Payments screen can show, so a tester can see a refund
// and a failed charge without having to manufacture one.
var topUps = []struct {
	member, pkg, channel, status string
	credits                      int
	amount                       int64
	daysAgo                      int
	expiryDays                   int
}{
	// The demo account: the longest history, so its wallet has something to
	// scroll and its expiry dates span both sides of today.
	{"mem_demo", "pkg_visit20", "QRIS", "PAID", 20, 2_700_000, 150, 120},
	{"mem_demo", "pkg_visit20", "QRIS", "PAID", 20, 2_700_000, 95, 120},
	{"mem_demo", "pkg_visit10", "EWALLET", "PAID", 10, 1_500_000, 48, 90},
	{"mem_demo", "pkg_starter5", "QRIS", "PAID", 5, 800_000, 12, 60},

	{"mem_natalie", "pkg_trial", "QRIS", "PAID", 1, 200_000, 160, 14},
	{"mem_natalie", "pkg_visit20", "VIRTUAL_ACCOUNT", "PAID", 20, 2_700_000, 140, 120},
	{"mem_natalie", "pkg_visit10", "VIRTUAL_ACCOUNT", "PAID", 10, 1_500_000, 62, 90},
	{"mem_natalie", "pkg_visit10", "QRIS", "PAID", 10, 1_500_000, 20, 90},

	{"mem_lucas", "pkg_visit10", "QRIS", "PAID", 10, 1_500_000, 120, 90},
	{"mem_lucas", "pkg_visit20", "QRIS", "PAID", 20, 2_700_000, 70, 120},
	{"mem_lucas", "pkg_starter5", "CARD", "PAID", 5, 800_000, 22, 60},

	{"mem_jaime", "pkg_starter5", "CARD", "PAID", 5, 800_000, 110, 60},
	{"mem_jaime", "pkg_visit20", "CARD", "PAID", 20, 2_700_000, 78, 120},
	{"mem_jaime", "pkg_visit10", "QRIS", "PAID", 10, 1_500_000, 18, 90},

	{"mem_doris", "pkg_trial", "QRIS", "PAID", 1, 200_000, 100, 14},
	{"mem_doris", "pkg_visit10", "QRIS", "PAID", 10, 1_500_000, 88, 90},
	{"mem_doris", "pkg_visit10", "EWALLET", "PAID", 10, 1_500_000, 33, 90},

	{"mem_luke", "pkg_visit20", "QRIS", "PAID", 20, 2_700_000, 105, 120},
	{"mem_luke", "pkg_race", "QRIS", "PAID", 8, 1_400_000, 55, 60},
	{"mem_luke", "pkg_visit10", "QRIS", "PAID", 10, 1_500_000, 9, 90},

	{"mem_sari", "pkg_visit10", "EWALLET", "PAID", 10, 1_500_000, 90, 90},
	{"mem_sari", "pkg_visit20", "EWALLET", "PAID", 20, 2_700_000, 44, 120},
	{"mem_sari", "pkg_starter5", "EWALLET", "PAID", 5, 800_000, 5, 60},

	// Not everything settles. A pending charge is what the front desk is
	// looking at when a member says they have paid and the credits are missing.
	{"mem_jaime", "pkg_visit10", "QRIS", "PENDING", 10, 1_500_000, 1, 90},
	{"mem_doris", "pkg_starter5", "CARD", "FAILED", 5, 800_000, 6, 60},
	{"mem_lucas", "pkg_trial", "QRIS", "EXPIRED", 1, 200_000, 20, 14},
	// A refund, so the ledger has a reversal in it and Finance has something
	// to look at that is not a happy path.
	{"mem_sari", "pkg_trial", "QRIS", "REFUNDED", 1, 200_000, 26, 14},
}

func (s *Seeder) seedTopUps(ctx context.Context) error {
	now := s.clock.Now()
	for i, t := range topUps {
		paidAt := now.AddDate(0, 0, -t.daysAgo)
		payID := fmt.Sprintf("pay_seed_%02d", i+1)

		var paid, refunded any
		if t.status == "PAID" || t.status == "REFUNDED" {
			paid = paidAt
		}
		if t.status == "REFUNDED" {
			refunded = paidAt.AddDate(0, 0, 2)
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO wallet.payments
				(id, member_id, package_id, credits, amount_idr, discount_idr, total_idr,
				 channel, status, external_id, created_at, paid_at, refunded_at)
			VALUES ($1,$2,$3,$4,$5,0,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (id) DO NOTHING`,
			payID, t.member, t.pkg, t.credits, t.amount, t.channel, t.status,
			"mock_"+payID, paidAt, paid, refunded); err != nil {
			return fmt.Errorf("seed: payment %s: %w", payID, err)
		}

		// Only a settled payment produces credits. That is the whole point of
		// keeping the payment and the ledger apart, so the seed honours it.
		if t.status != "PAID" && t.status != "REFUNDED" {
			continue
		}

		entryID := fmt.Sprintf("led_seed_%02d", i+1)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO wallet.credit_ledger_entries
				(id, member_id, type, amount, description, source_type, source_id, created_at)
			VALUES ($1,$2,'TOP_UP',$3,$4,'PAYMENT',$5,$6)
			ON CONFLICT (id) DO NOTHING`,
			entryID, t.member, t.credits, "Top-up", payID, paidAt); err != nil {
			return fmt.Errorf("seed: ledger %s: %w", entryID, err)
		}

		// The lot is what expiry runs against: credits leave in the order they
		// were bought, so each top-up needs its own dated bucket.
		if _, err := s.db.Exec(ctx, `
			INSERT INTO wallet.top_up_lots
				(id, member_id, ledger_entry_id, package_id, credits, expires_at, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("lot_seed_%02d", i+1), t.member, entryID, t.pkg, t.credits,
			paidAt.AddDate(0, 0, t.expiryDays), paidAt); err != nil {
			return fmt.Errorf("seed: lot %d: %w", i+1, err)
		}

		if t.status == "REFUNDED" {
			// A refund reverses the entry rather than deleting it. The ledger
			// is a record of what happened, including the parts undone.
			reverseID := fmt.Sprintf("led_seed_%02d_rev", i+1)
			if _, err := s.db.Exec(ctx, `
				INSERT INTO wallet.credit_ledger_entries
					(id, member_id, type, amount, description, source_type, source_id,
					 reverses_entry_id, actor_id, reason, created_at)
				VALUES ($1,$2,'REVERSAL',$3,$4,'PAYMENT',$5,$6,'adm_finance',$7,$8)
				ON CONFLICT (id) DO NOTHING`,
				reverseID, t.member, -t.credits, "Refund", payID, entryID,
				"Member cancelled within the cooling-off window",
				paidAt.AddDate(0, 0, 2)); err != nil {
				return fmt.Errorf("seed: reversal %s: %w", reverseID, err)
			}
		}
	}
	return nil
}

// seedBookings fills the register: past classes with attendance already
// settled, and future ones with people signed up.
//
// Attendance is spread on purpose — most turn up, a few do not — because a
// coach statement with no no-shows never exercises the penalty, and a studio
// with perfect attendance is not a studio.
func (s *Seeder) seedBookings(ctx context.Context) (int, error) {
	// Past classes, bounded by what each member actually bought.
	//
	// The first version of this crossed every member with every past class and
	// produced a studio where all seven members were forty credits in debt — a
	// wallet screen showing a negative balance is not a demo, it is a bug
	// report. So attendance is spent out of a budget: a running total of
	// credit_cost, cut off at 60% of what the member has paid for, which leaves
	// everybody with a live balance to book against.
	var completed int64
	tag, err := s.db.Exec(ctx, `
		WITH budget AS (
			SELECT l.member_id, greatest(1, sum(l.amount) * 6 / 10) AS credits
			FROM wallet.credit_ledger_entries l
			JOIN identity.members m ON m.id = l.member_id AND m.status = 'ACTIVE'
			-- Purchases only. A second run must compute the same budget, and
			-- by then the ledger also holds the deductions this one wrote.
			WHERE l.type IN ('TOP_UP', 'REVERSAL')
			GROUP BY l.member_id
			HAVING sum(l.amount) > 0
		),
		past AS (
			SELECT id, starts_at, credit_cost
			FROM scheduling.class_sessions
			WHERE status = 'COMPLETED'
		),
		spend AS (
			SELECT b.member_id, p.id AS session_id, p.starts_at, b.credits,
			       -- Deterministic per member, so each one attends a different
			       -- spread of classes rather than all of them the same ones.
			       md5(b.member_id || p.id) AS shuffle,
			       sum(p.credit_cost) OVER (
			           PARTITION BY b.member_id
			           ORDER BY md5(b.member_id || p.id)
			           ROWS UNBOUNDED PRECEDING
			       ) AS running
			FROM budget b CROSS JOIN past p
		)
		INSERT INTO scheduling.bookings
			(id, member_id, session_id, status, source, created_at, updated_at, checked_in_at)
		SELECT
			'bkg_seed_' || substr(md5(session_id || member_id), 1, 16),
			member_id, session_id,
			-- One booking in eight is a no-show. A studio with perfect
			-- attendance never exercises the penalty, and the coach statement
			-- has nothing on it worth reading.
			CASE WHEN ('x' || substr(shuffle, 1, 2))::bit(8)::int % 8 = 0
			     THEN 'NO_SHOW' ELSE 'COMPLETED' END,
			'MEMBER', starts_at - interval '2 days', starts_at,
			CASE WHEN ('x' || substr(shuffle, 1, 2))::bit(8)::int % 8 = 0
			     THEN NULL ELSE starts_at END
		FROM spend
		WHERE running <= credits
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("seed: past bookings: %w", err)
	}
	completed += tag.RowsAffected()

	// Upcoming classes: confirmed places, so the schedule shows filling
	// classes rather than a fortnight of empty ones.
	tag, err = s.db.Exec(ctx, `
		WITH upcoming AS (
			SELECT id, starts_at, capacity, row_number() OVER (ORDER BY starts_at) AS n
			FROM scheduling.class_sessions
			WHERE status = 'PUBLISHED' AND starts_at > now()
		),
		people AS (
			SELECT id, row_number() OVER (ORDER BY id) AS n
			FROM identity.members WHERE status = 'ACTIVE'
		),
		pairs AS (
			SELECT upcoming.id AS session_id, people.id AS member_id, upcoming.starts_at
			FROM upcoming CROSS JOIN people
			WHERE (upcoming.n + people.n * 2) % 3 = 0
			  -- A member may hold one active booking per session, enforced by a
			  -- partial unique index that ON CONFLICT (id) does not cover: a
			  -- waitlist row from an earlier run has a different id and the
			  -- same pair, so the second run collided on the rule rather than
			  -- the key.
			  AND NOT EXISTS (
				SELECT 1 FROM scheduling.bookings b
				WHERE b.session_id = upcoming.id AND b.member_id = people.id
				  AND b.status IN ('PENDING', 'CONFIRMED', 'WAITLIST', 'CHECKED_IN')
			  )
		)
		INSERT INTO scheduling.bookings
			(id, member_id, session_id, status, source, created_at, updated_at)
		SELECT
			'bkg_seed_' || substr(md5(session_id || member_id), 1, 16),
			member_id, session_id, 'CONFIRMED', 'MEMBER', now() - interval '3 days', now()
		FROM pairs
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("seed: upcoming bookings: %w", err)
	}
	completed += tag.RowsAffected()

	// A class that filled, with a waiting list behind it — the state the
	// booking rules are most interesting in, and one nobody would see
	// otherwise until a real class sold out.
	//
	// The NOT EXISTS is what keeps a second run from filling a second class:
	// the first one is FULL by then, so an unguarded query would just pick the
	// next PUBLISHED session and the studio would sell out a little more every
	// time somebody reseeded.
	if _, err := s.db.Exec(ctx, `
		UPDATE scheduling.class_sessions SET status = 'FULL'
		WHERE id = (
			SELECT s.id FROM scheduling.class_sessions s
			WHERE s.status = 'PUBLISHED' AND s.starts_at > now()
			  AND NOT EXISTS (
				SELECT 1 FROM scheduling.class_sessions f WHERE f.status = 'FULL'
			  )
			ORDER BY s.starts_at LIMIT 1
		)`); err != nil {
		return 0, fmt.Errorf("seed: full session: %w", err)
	}
	tag, err = s.db.Exec(ctx, `
		INSERT INTO scheduling.bookings
			(id, member_id, session_id, status, waitlist_position, source, created_at, updated_at)
		SELECT
			'bkg_seed_wl_' || substr(md5(s.id || m.id), 1, 12),
			m.id, s.id, 'WAITLIST',
			row_number() OVER (ORDER BY m.id),
			'MEMBER', now() - interval '1 day', now()
		FROM scheduling.class_sessions s
		CROSS JOIN identity.members m
		WHERE s.status = 'FULL'
		  AND m.status = 'ACTIVE'
		  AND NOT EXISTS (
			SELECT 1 FROM scheduling.bookings b
			WHERE b.session_id = s.id AND b.member_id = m.id
		  )
		LIMIT 3
		ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("seed: waitlist: %w", err)
	}
	completed += tag.RowsAffected()

	return int(completed), nil
}

// seedVisits writes the door log, and the credit each visit cost.
//
// One log line per attended booking, so the visits report, the member's
// history and the ledger all agree with each other — three screens that
// disagree is worse than three empty ones.
func (s *Seeder) seedVisits(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO access.access_logs
			(id, member_id, gate_id, branch_id, result, credit_delta, mode, booking_id, created_at)
		SELECT
			'acc_seed_' || substr(md5(b.id), 1, 16),
			b.member_id,
			-- Whichever gate belongs to the branch the class was at.
			(SELECT g.id FROM catalog.gates g WHERE g.branch_id = s.branch_id ORDER BY g.id LIMIT 1),
			s.branch_id, 'ALLOWED', -s.credit_cost, 'ONLINE', b.id,
			b.checked_in_at
		FROM scheduling.bookings b
		JOIN scheduling.class_sessions s ON s.id = b.session_id
		WHERE b.status = 'COMPLETED' AND b.checked_in_at IS NOT NULL
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: access logs: %w", err)
	}

	// The deduction each of those visits cost. The ledger is the source of
	// truth for a balance, so a visit that moved a turnstile and not a credit
	// would leave every member richer than they are.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO wallet.credit_ledger_entries
			(id, member_id, type, amount, description, source_type, source_id, created_at)
		SELECT
			'led_visit_' || substr(md5(b.id), 1, 16),
			b.member_id, 'VISIT_DEDUCTION', -s.credit_cost,
			'Class attended', 'ACCESS', b.id, b.checked_in_at
		FROM scheduling.bookings b
		JOIN scheduling.class_sessions s ON s.id = b.session_id
		WHERE b.status = 'COMPLETED' AND b.checked_in_at IS NOT NULL
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: visit deductions: %w", err)
	}

	// One denial, so the gate monitor has something other than green on it.
	// Anti-passback is the denial a front desk actually has to explain.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO access.access_logs
			(id, member_id, gate_id, branch_id, result, reason_code, credit_delta, mode, created_at)
		VALUES
			('acc_seed_denied', 'mem_dimas', 'gat_senopati_a', 'brn_senopati',
			 'DENIED', 'MEMBER_NOT_ACTIVE', 0, 'ONLINE', now() - interval '2 days'),
			('acc_seed_passback', 'mem_demo', 'gat_senopati_a', 'brn_senopati',
			 'DENIED', 'ANTI_PASSBACK', 0, 'ONLINE', now() - interval '5 days')
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: denied entries: %w", err)
	}

	// A door that was offline when somebody scanned, and the sync that caught
	// up afterwards — the conflict the access screens exist to resolve.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO access.access_logs
			(id, member_id, gate_id, branch_id, result, credit_delta, mode, created_at)
		VALUES ('acc_seed_offline', 'mem_natalie', 'gat_pik_a', 'brn_pik',
		        'CONFLICT', 0, 'OFFLINE', now() - interval '3 days')
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: offline entry: %w", err)
	}
	return nil
}
