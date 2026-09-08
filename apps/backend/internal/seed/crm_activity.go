package seed

import (
	"context"
	"fmt"
	"time"
)

// The loyalty programme and the inbox, populated from what already happened.
//
// The tiers, rewards, badges and XP rules were already seeded; what was
// missing was anybody having *used* them. Points are derived from the visits
// and top-ups the lifecycle seed wrote rather than invented, so a member's
// points history lines up with their visit history — the first thing anyone
// checks when they think the programme has shortchanged them.

// xpPerVisit and xpPerThousandSpent set the earn rate.
//
// They are the rates the seeded xp_rules describe. Kept as constants here so
// the seeded ledger cannot drift from the rules the panel displays.
const (
	xpPerVisit = 50
	// Per ten thousand rupiah, not per thousand: at the finer rate the whole
	// roster cleared Gold on spend alone and the tier ladder said nothing
	// about anybody. Three Gold, four Silver, two Bronze is a programme.
	xpPerTenThousandSpent = 2
)

// minutes is time.Duration spelled for the message offsets above, where every
// number is a count of minutes and writing it out obscures the timeline.
func minutes(n int) time.Duration { return time.Duration(n) * time.Minute }

func (s *Seeder) seedCRMActivity(ctx context.Context) (int, error) {
	if err := s.seedMemberProfiles(ctx); err != nil {
		return 0, err
	}
	if err := s.seedXP(ctx); err != nil {
		return 0, err
	}
	if err := s.seedRedemptions(ctx); err != nil {
		return 0, err
	}
	if err := s.seedBadgesEarned(ctx); err != nil {
		return 0, err
	}
	if err := s.seedReviews(ctx); err != nil {
		return 0, err
	}
	if err := s.seedContactPreferences(ctx); err != nil {
		return 0, err
	}
	if err := s.seedInbox(ctx); err != nil {
		return 0, err
	}
	if err := s.seedPartners(ctx); err != nil {
		return 0, err
	}
	return s.seedCampaigns(ctx)
}

// seedMemberProfiles gives every member a loyalty account.
//
// Everyone gets one, including the suspended and inactive members: a profile
// is what the programme knows about a person, and losing it when they lapse
// is how a returning member finds their points gone.
func (s *Seeder) seedMemberProfiles(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO crm.member_profiles
			(id, member_id, member_code, tier_code, joined_at, last_activity_at, status)
		SELECT
			'crp_' || m.id,
			m.id,
			'NH-' || lpad((row_number() OVER (ORDER BY m.created_at, m.id))::text, 4, '0'),
			'BRONZE',
			m.created_at,
			(SELECT max(b.checked_in_at) FROM scheduling.bookings b WHERE b.member_id = m.id),
			CASE m.status WHEN 'SUSPENDED' THEN 'SUSPENDED'
			              WHEN 'ACTIVE' THEN 'ACTIVE' ELSE 'INACTIVE' END
		FROM identity.members m
		ON CONFLICT (member_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: member profiles: %w", err)
	}
	return nil
}

// seedXP posts the points every visit and every purchase earned.
//
// The running balance is computed here rather than left to the reader: the
// ledger's own CHECK insists that balance_after = balance_before + xp_delta,
// so the rows have to be built in order and each one has to know what came
// before it. That is what the window function is doing.
func (s *Seeder) seedXP(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		WITH events AS (
			-- Points for turning up.
			SELECT b.member_id,
			       'crm_xp_v_' || substr(md5(b.id), 1, 16) AS id,
			       $1::int AS xp,
			       b.checked_in_at AS at,
			       'CLASS' AS channel, 'xpr_attend' AS rule,
			       'BOOKING' AS ref_type, b.id AS ref_id,
			       'Class attended' AS description
			FROM scheduling.bookings b
			WHERE b.status = 'COMPLETED' AND b.checked_in_at IS NOT NULL
			UNION ALL
			-- Points for spending. Rounded down: the programme never rounds up
			-- in its own favour, and a member checking the arithmetic should
			-- find it stingy rather than wrong.
			SELECT p.member_id,
			       'crm_xp_p_' || substr(md5(p.id), 1, 16),
			       greatest(1, (p.total_idr / 10000 * $2)::int),
			       p.paid_at,
			       'PAYMENT', 'xpr_topup',
			       'PAYMENT', p.id,
			       'Top-up'
			FROM wallet.payments p
			WHERE p.status = 'PAID' AND p.paid_at IS NOT NULL
		),
		ordered AS (
			SELECT e.*,
			       sum(e.xp) OVER (PARTITION BY e.member_id ORDER BY e.at, e.id
			                       ROWS UNBOUNDED PRECEDING) AS running
			FROM events e
		)
		INSERT INTO crm.xp_ledger
			(id, member_id, direction, source_channel, source_type, source_id,
			 xp_delta, balance_before, balance_after, lifetime_before, lifetime_after,
			 rule_id, reference_type, reference_id, idempotency_key, description, created_at)
		SELECT id, member_id, 'EARN', channel, ref_type, ref_id,
		       xp, running - xp, running, running - xp, running,
		       rule, ref_type, ref_id, id, description, at
		FROM ordered
		ON CONFLICT (id) DO NOTHING`, xpPerVisit, xpPerTenThousandSpent); err != nil {
		return fmt.Errorf("seed: xp ledger: %w", err)
	}

	// Roll the totals up onto the profile, and place each member in the tier
	// their lifetime points have earned them.
	if _, err := s.db.Exec(ctx, `
		WITH totals AS (
			SELECT member_id, sum(xp_delta) AS earned
			FROM crm.xp_ledger WHERE direction = 'EARN'
			GROUP BY member_id
		),
		spend AS (
			SELECT p.member_id, coalesce(sum(p.total_idr), 0) AS paid
			FROM wallet.payments p WHERE p.status = 'PAID'
			GROUP BY p.member_id
		)
		UPDATE crm.member_profiles pr
		SET current_xp = t.earned,
		    lifetime_xp = t.earned,
		    spent_xp = 0,
		    lifetime_spend_idr = coalesce(sp.paid, 0),
		    tier_code = (
		        SELECT ti.code FROM crm.tiers ti
		        WHERE ti.min_lifetime_xp <= t.earned
		          AND ti.min_spend_idr <= coalesce(sp.paid, 0)
		        ORDER BY ti.rank DESC LIMIT 1
		    )
		FROM totals t
		LEFT JOIN spend sp ON sp.member_id = t.member_id
		WHERE pr.member_id = t.member_id`); err != nil {
		return fmt.Errorf("seed: xp rollup: %w", err)
	}
	return nil
}

// seedRedemptions spends some of those points.
//
// Deliberately spread across the statuses: one fulfilled, one approved and
// waiting to be handed over, one still pending. The rewards queue with
// nothing in it tells a front-desk tester nothing about what the queue does.
func (s *Seeder) seedRedemptions(ctx context.Context) error {
	rows := []struct {
		id, number, member, reward, status, code string
		cost                                     int
		daysAgo                                  int
	}{
		{"red_seed_1", "RDM-0001", "mem_demo", "rwd_bar", "FULFILLED", "RWD-8H2K", 150, 21},
		{"red_seed_2", "RDM-0002", "mem_natalie", "rwd_shaker", "FULFILLED", "RWD-J4Q9", 400, 14},
		{"red_seed_3", "RDM-0003", "mem_luke", "rwd_bar", "APPROVED", "RWD-M7X3", 150, 3},
		{"red_seed_4", "RDM-0004", "mem_sari", "rwd_class", "PENDING", "", 800, 1},
		{"red_seed_5", "RDM-0005", "mem_lucas", "rwd_bar", "CANCELLED", "", 150, 9},
	}
	now := s.clock.Now()
	for _, r := range rows {
		at := now.AddDate(0, 0, -r.daysAgo)

		// A cancelled request never took the points, so it never gets a ledger
		// entry. Everything else does, and the profile is moved to match.
		var ledgerID any
		if r.status != "CANCELLED" && r.status != "PENDING" {
			entry := r.id + "_xp"
			if _, err := s.db.Exec(ctx, `
				INSERT INTO crm.xp_ledger
					(id, member_id, direction, source_channel, source_type, source_id,
					 xp_delta, balance_before, balance_after, lifetime_before, lifetime_after,
					 reference_type, reference_id, idempotency_key, description, created_at)
				SELECT $1, $2, 'SPEND', 'LOYALTY', 'REDEMPTION', $3,
				       -($4::int), pr.current_xp, pr.current_xp - $4, pr.lifetime_xp, pr.lifetime_xp,
				       'REDEMPTION', $3, $1, $5, $6
				FROM crm.member_profiles pr
				WHERE pr.member_id = $2 AND pr.current_xp >= $4
				ON CONFLICT (id) DO NOTHING`,
				entry, r.member, r.id, r.cost, "Reward redeemed", at); err != nil {
				return fmt.Errorf("seed: redemption xp %s: %w", r.id, err)
			}
			if _, err := s.db.Exec(ctx, `
				UPDATE crm.member_profiles
				SET current_xp = current_xp - $2, spent_xp = spent_xp + $2
				WHERE member_id = $1 AND current_xp >= $2
				  AND EXISTS (SELECT 1 FROM crm.xp_ledger WHERE id = $3)`,
				r.member, r.cost, entry); err != nil {
				return fmt.Errorf("seed: redemption balance %s: %w", r.id, err)
			}
			ledgerID = entry
		}

		var code any
		if r.code != "" {
			code = r.code
		}
		var approved, fulfilled, cancelled any
		switch r.status {
		case "FULFILLED":
			approved, fulfilled = at, at.AddDate(0, 0, 1)
		case "APPROVED":
			approved = at
		case "CANCELLED":
			cancelled = at
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO crm.redemptions
				(id, redemption_number, member_id, reward_id, xp_cost, xp_ledger_id,
				 status, voucher_code, requested_at, approved_at, fulfilled_at,
				 cancelled_at, expires_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (id) DO NOTHING`,
			r.id, r.number, r.member, r.reward, r.cost, ledgerID, r.status, code,
			at, approved, fulfilled, cancelled, at.AddDate(0, 0, 90)); err != nil {
			return fmt.Errorf("seed: redemption %s: %w", r.id, err)
		}
	}
	return nil
}

// seedBadgesEarned awards the badges the visit history already justifies.
//
// Derived rather than listed, so a badge is never on a member who has not
// done the thing — the one property a badge has to have to mean anything.
func (s *Seeder) seedBadgesEarned(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		WITH visits AS (
			SELECT member_id, count(*) AS n, min(checked_in_at) AS first_at,
			       max(checked_in_at) AS last_at
			FROM scheduling.bookings
			WHERE status = 'COMPLETED' AND checked_in_at IS NOT NULL
			GROUP BY member_id
		),
		earned AS (
			SELECT member_id, 'bdg_first' AS badge, 1::numeric AS value, first_at AS at
			FROM visits WHERE n >= 1
			UNION ALL
			SELECT member_id, 'bdg_ten', 10, last_at FROM visits WHERE n >= 10
			UNION ALL
			SELECT member_id, 'bdg_fifty', 50, last_at FROM visits WHERE n >= 50
			UNION ALL
			SELECT p.member_id, 'bdg_spender', p.paid, now() - interval '20 days'
			FROM (
				SELECT member_id, sum(total_idr) AS paid FROM wallet.payments
				WHERE status = 'PAID' GROUP BY member_id
			) p WHERE p.paid >= 1000000
		)
		INSERT INTO crm.member_badges (id, member_id, badge_id, earned_value, earned_at)
		SELECT 'mbd_' || substr(md5(member_id || badge), 1, 16), member_id, badge, value, at
		FROM earned
		ON CONFLICT (member_id, badge_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed: member badges: %w", err)
	}
	return nil
}

// seedReviews writes what members thought of the classes they attended.
//
// Only against sessions they actually attended, and marked verified with the
// booking that proves it — an unverifiable review is the thing the moderation
// screen exists to catch, so one of these is left unverified on purpose.
func (s *Seeder) seedReviews(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		WITH attended AS (
			SELECT b.member_id, b.session_id, b.id AS booking_id, b.checked_in_at,
			       s.coach_id,
			       row_number() OVER (PARTITION BY b.member_id ORDER BY b.checked_in_at DESC) AS recent
			FROM scheduling.bookings b
			JOIN scheduling.class_sessions s ON s.id = b.session_id
			WHERE b.status = 'COMPLETED' AND b.checked_in_at IS NOT NULL
		),
		picked AS (SELECT * FROM attended WHERE recent <= 2)
		INSERT INTO crm.reviews
			(id, member_id, subject_type, subject_id, rating, comment, verified,
			 visit_id, status, created_at, updated_at)
		SELECT
			'rev_' || substr(md5(member_id || session_id), 1, 16),
			member_id, 'COACH', coach_id,
			-- Mostly good, occasionally not. A five-star-only wall is not
			-- feedback, it is a testimonial page.
			4 + (('x' || substr(md5(member_id || session_id), 1, 2))::bit(8)::int % 5 = 0)::int,
			CASE ('x' || substr(md5(member_id || session_id), 3, 2))::bit(8)::int % 4
			     WHEN 0 THEN 'Great energy, pushed us hard without losing form.'
			     WHEN 1 THEN 'Clear cues and good scaling options. Will book again.'
			     WHEN 2 THEN 'Warm-up felt rushed but the main set was excellent.'
			     ELSE 'Best session of my week.'
			END,
			true, booking_id, 'PUBLISHED', checked_in_at + interval '3 hours',
			checked_in_at + interval '3 hours'
		FROM picked
		ON CONFLICT (member_id, subject_type, subject_id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: reviews: %w", err)
	}

	// One unhappy review, flagged and unverified, so the moderation queue has
	// the case it was built for rather than an empty table.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO crm.reviews
			(id, member_id, subject_type, subject_id, rating, comment, verified,
			 status, created_at, updated_at)
		VALUES
			('rev_flagged', 'mem_dimas', 'BRANCH', 'brn_senopati', 1,
			 'Showed up and could not get in. Nobody explained why.', false,
			 'FLAGGED', now() - interval '2 days', now() - interval '2 days')
		ON CONFLICT (member_id, subject_type, subject_id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: flagged review: %w", err)
	}

	// A reply on one of them, because the reply box is half the screen and an
	// unanswered wall does not show what answering looks like.
	if _, err := s.db.Exec(ctx, `
		UPDATE crm.reviews
		SET reply = 'Thank you — we have added five minutes to the warm-up block.',
		    replied_by = 'adm_branch', replied_at = created_at + interval '1 day'
		WHERE id = (SELECT id FROM crm.reviews WHERE status = 'PUBLISHED'
		            ORDER BY created_at DESC LIMIT 1)`); err != nil {
		return fmt.Errorf("seed: review reply: %w", err)
	}
	return nil
}

// seedContactPreferences records who may be messaged, and how.
//
// Two members have opted out of marketing. Without them a campaign's audience
// count always equals the member count, and the skip path — the part that
// keeps the studio out of trouble — never runs.
func (s *Seeder) seedContactPreferences(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO crm.contact_preferences (member_id, channel, opted_in, scope, changed_at)
		SELECT m.id, c.channel, true, 'MARKETING', m.created_at
		FROM identity.members m
		CROSS JOIN (VALUES ('PUSH'), ('EMAIL'), ('WHATSAPP')) AS c(channel)
		ON CONFLICT (member_id, channel) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: contact preferences: %w", err)
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE crm.contact_preferences
		SET opted_in = false, reason = 'Too many messages', changed_at = now() - interval '30 days'
		WHERE (member_id, channel) IN (('mem_doris', 'WHATSAPP'), ('mem_rina', 'EMAIL'),
		                               ('mem_rina', 'PUSH'))`); err != nil {
		return fmt.Errorf("seed: opt-outs: %w", err)
	}
	return nil
}

type conversationSeed struct {
	id, member, name, handle, channel, subject, status, priority, assignee, assigneeName string
	tags                                                                                 []string
	daysAgo                                                                              int
	messages                                                                             []messageSeed
}

type messageSeed struct {
	direction, body, authorID, authorName string
	minutesIn                             int
	internal                              bool
}

// inboxSeed covers the states the inbox screen sorts by: an open thread
// waiting on us, one waiting on the member, an urgent complaint, a resolved
// one, and a message from a number nobody recognises.
var inboxSeed = []conversationSeed{
	{
		id: "cnv_seed_1", member: "mem_natalie", name: "Natalie Chandra", handle: "+628121000002",
		channel: "WHATSAPP", subject: "Can I move my Saturday booking?", status: "OPEN",
		priority: "NORMAL", assignee: "adm_desk", assigneeName: "Dewi Front Desk",
		tags: []string{"booking"}, daysAgo: 1,
		messages: []messageSeed{
			{direction: "INBOUND", body: "Hi! Something came up — can I move my Saturday 7am to Sunday?", minutesIn: 0},
			{direction: "OUTBOUND", body: "Of course. Sunday 7am has space, shall I move you?", authorID: "adm_desk", authorName: "Dewi Front Desk", minutesIn: 12},
			{direction: "INBOUND", body: "Yes please 🙏", minutesIn: 40},
		},
	},
	{
		id: "cnv_seed_2", member: "mem_dimas", name: "Dimas Prasetyo", handle: "dimas@example.com",
		channel: "EMAIL", subject: "Denied at the gate", status: "OPEN", priority: "URGENT",
		assignee: "adm_branch", assigneeName: "Rangga Branch Manager",
		tags: []string{"access", "complaint"}, daysAgo: 2,
		messages: []messageSeed{
			{direction: "INBOUND", body: "I was refused entry this morning with no explanation. I would like to know why.", minutesIn: 0},
			{direction: "OUTBOUND", body: "Membership shows as suspended pending the outstanding invoice — checking with Finance now.", authorID: "adm_branch", authorName: "Rangga Branch Manager", minutesIn: 90, internal: true},
		},
	},
	{
		id: "cnv_seed_3", member: "mem_jaime", name: "Jaime Wibowo", handle: "+628121000004",
		channel: "WHATSAPP", subject: "Top-up not showing", status: "PENDING", priority: "HIGH",
		assignee: "adm_finance", assigneeName: "Sinta Finance",
		tags: []string{"payment"}, daysAgo: 1,
		messages: []messageSeed{
			{direction: "INBOUND", body: "I paid for the 10 visit pack an hour ago and my balance has not changed.", minutesIn: 0},
			{direction: "OUTBOUND", body: "The payment is still pending with the provider. I will confirm the moment it settles.", authorID: "adm_finance", authorName: "Sinta Finance", minutesIn: 25},
		},
	},
	{
		id: "cnv_seed_4", name: "Unknown number", handle: "+628990001111",
		channel: "WHATSAPP", subject: "Class schedule?", status: "OPEN", priority: "LOW",
		tags: []string{"enquiry"}, daysAgo: 3,
		messages: []messageSeed{
			{direction: "INBOUND", body: "Halo, kelas hyrox jam berapa aja ya?", minutesIn: 0},
		},
	},
	{
		id: "cnv_seed_5", member: "mem_demo", name: "Demo Member", handle: "demo@nuhabit.id",
		channel: "INBOX", subject: "Locker key returned", status: "RESOLVED", priority: "LOW",
		assignee: "adm_desk", assigneeName: "Dewi Front Desk",
		tags: []string{"facilities"}, daysAgo: 6,
		messages: []messageSeed{
			{direction: "INBOUND", body: "I think I walked off with locker key 12, sorry.", minutesIn: 0},
			{direction: "OUTBOUND", body: "No problem at all — drop it at the desk whenever suits.", authorID: "adm_desk", authorName: "Dewi Front Desk", minutesIn: 8},
			{direction: "INBOUND", body: "Returned it this morning. Thanks!", minutesIn: 1440},
		},
	},
}

func (s *Seeder) seedInbox(ctx context.Context) error {
	now := s.clock.Now()
	for _, c := range inboxSeed {
		start := now.AddDate(0, 0, -c.daysAgo)

		var lastMember, lastStaff any
		var firstResponse any
		var resolved any
		for _, m := range c.messages {
			at := start.Add(minutes(m.minutesIn))
			if m.direction == "INBOUND" {
				lastMember = at
			} else {
				lastStaff = at
				if firstResponse == nil {
					firstResponse = m.minutesIn * 60
				}
			}
		}
		if c.status == "RESOLVED" || c.status == "CLOSED" {
			resolved = start.Add(minutes(c.messages[len(c.messages)-1].minutesIn + 30))
		}

		var member, assignee, assigneeName, handle any
		if c.member != "" {
			member = c.member
		}
		if c.assignee != "" {
			assignee, assigneeName = c.assignee, c.assigneeName
		}
		if c.handle != "" {
			handle = c.handle
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO crm.conversations
				(id, member_id, contact_name, contact_handle, channel, subject, status,
				 priority, assigned_to, assigned_name, branch_id, tags,
				 last_member_at, last_staff_at, first_response_seconds, resolved_at,
				 created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'brn_senopati',$11,$12,$13,$14,$15,$16,$16)
			ON CONFLICT (id) DO NOTHING`,
			c.id, member, c.name, handle, c.channel, c.subject, c.status, c.priority,
			assignee, assigneeName, c.tags, lastMember, lastStaff, firstResponse,
			resolved, start); err != nil {
			return fmt.Errorf("seed: conversation %s: %w", c.id, err)
		}

		for i, m := range c.messages {
			var authorID, authorName any
			if m.authorID != "" {
				authorID, authorName = m.authorID, m.authorName
			}
			if _, err := s.db.Exec(ctx, `
				INSERT INTO crm.conversation_messages
					(id, conversation_id, direction, body, author_id, author_name,
					 internal, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT (id) DO NOTHING`,
				fmt.Sprintf("%s_m%d", c.id, i+1), c.id, m.direction, m.body,
				authorID, authorName, m.internal, start.Add(minutes(m.minutesIn))); err != nil {
				return fmt.Errorf("seed: message %s#%d: %w", c.id, i+1, err)
			}
		}
	}
	return nil
}

// seedPartners registers the outside systems that send us events, and a few
// events from each.
//
// One of them is deliberately unmatched: a race result for somebody whose
// email we do not have. That row is the reason the matching screen exists.
func (s *Seeder) seedPartners(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO crm.integration_partners
			(id, code, name, kind, secret, contact_name, contact_email, awards_xp, active)
		VALUES
			('prt_hyrox', 'HYROX_ID', 'HYROX Indonesia', 'RACE', 'seed-secret-hyrox',
			 'Race Ops', 'ops@hyrox.example', true, true),
			('prt_strava', 'STRAVA', 'Strava', 'HEALTH', 'seed-secret-strava',
			 'Partnerships', 'partners@strava.example', true, true),
			('prt_merch', 'MERCH_CO', 'Merch Co', 'RETAIL', NULL,
			 'Wholesale', 'hello@merch.example', false, false)
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: integration partners: %w", err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO crm.external_events
			(id, partner_id, external_id, event_type, subject, member_id, occurred_at,
			 payload, status, xp_awarded, processed_at)
		VALUES
			('xev_seed_1', 'prt_hyrox', 'hx-2026-0431', 'RACE_FINISHED',
			 'demo@nuhabit.id', 'mem_demo', now() - interval '35 days',
			 '{"event":"HYROX Jakarta","division":"Open","time":"01:24:10"}'::jsonb,
			 'PROCESSED', 500, now() - interval '35 days'),
			('xev_seed_2', 'prt_hyrox', 'hx-2026-0512', 'RACE_FINISHED',
			 'natalie@example.com', 'mem_natalie', now() - interval '35 days',
			 '{"event":"HYROX Jakarta","division":"Pro","time":"01:11:52"}'::jsonb,
			 'PROCESSED', 500, now() - interval '35 days'),
			('xev_seed_3', 'prt_strava', 'st-99120031', 'ACTIVITY_SYNCED',
			 'luke@example.com', 'mem_luke', now() - interval '2 days',
			 '{"activity":"Run","distance_km":10.4}'::jsonb,
			 'MATCHED', 0, NULL),
			-- Nobody by that address. The row the matching screen is for.
			('xev_seed_4', 'prt_hyrox', 'hx-2026-0644', 'RACE_FINISHED',
			 'unknown.athlete@example.com', NULL, now() - interval '1 day',
			 '{"event":"HYROX Jakarta","division":"Open","time":"01:39:02"}'::jsonb,
			 'UNMATCHED', 0, NULL),
			('xev_seed_5', 'prt_strava', 'st-99120044', 'ACTIVITY_SYNCED',
			 'sari@example.com', 'mem_sari', now() - interval '6 hours',
			 '{"activity":"Ride","distance_km":31.2}'::jsonb,
			 'FAILED', 0, NULL)
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: external events: %w", err)
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE crm.external_events
		SET error = 'Rate limited by partner; will retry'
		WHERE id = 'xev_seed_5' AND error IS NULL`); err != nil {
		return fmt.Errorf("seed: external event error: %w", err)
	}
	return nil
}

// seedCampaigns sends a few broadcasts, and records who got them.
//
// A sent campaign carries per-recipient rows including the skips, because the
// number that matters on the campaign screen is not how many were sent but
// how many were not, and why.
func (s *Seeder) seedCampaigns(ctx context.Context) (int, error) {
	campaigns := []struct {
		id, name, segment, message, deepLink, status string
		daysAgo                                      int
	}{
		{"cmp_seed_welcome", "Welcome to NüHabit", "NEW_MEMBERS",
			"Your first class is waiting — book it in the app.", "/classes", "SENT", 30},
		{"cmp_seed_lowbal", "Running low on credits", "LOW_BALANCE",
			"You have fewer than 3 credits left. Top up and keep your streak.", "/wallet/topup", "SENT", 7},
		{"cmp_seed_expiring", "Credits expiring soon", "EXPIRING_CREDITS",
			"Some of your credits expire this month. Book before they go.", "/classes", "SENT", 2},
		{"cmp_seed_comeback", "We miss you", "NO_VISIT_14D",
			"It has been a while. Your next session is on us.", "/classes", "SCHEDULED", -3},
		{"cmp_seed_draft", "Ramadan schedule", "ALL_ACTIVE",
			"Adjusted class times through Ramadan — see the new schedule.", "/classes", "DRAFT", 0},
	}

	for _, c := range campaigns {
		at := s.clock.Now().AddDate(0, 0, -c.daysAgo)
		var scheduled, sentCount any
		if c.status != "DRAFT" {
			scheduled = at
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO engagement.campaigns
				(id, name, segment, message, deep_link, scheduled_at, status, sent_count,
				 created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
			ON CONFLICT (id) DO NOTHING`,
			c.id, c.name, c.segment, c.message, c.deepLink, scheduled, c.status,
			sentCount, at.AddDate(0, 0, -1)); err != nil {
			return 0, fmt.Errorf("seed: campaign %s: %w", c.id, err)
		}

		if c.status != "SENT" {
			continue
		}

		// Recipients, with the opted-out members recorded as skipped rather
		// than quietly missing.
		if _, err := s.db.Exec(ctx, `
			INSERT INTO engagement.campaign_recipients
				(id, campaign_id, member_id, status, skip_reason, sent_at, opened_at,
				 clicked_at, created_at)
			SELECT
				$1 || '_' || m.id, $1, m.id,
				CASE
					WHEN NOT coalesce(p.opted_in, true) THEN 'SKIPPED'
					WHEN ('x' || substr(md5($1 || m.id), 1, 2))::bit(8)::int % 5 = 0 THEN 'CLICKED'
					WHEN ('x' || substr(md5($1 || m.id), 1, 2))::bit(8)::int % 2 = 0 THEN 'OPENED'
					ELSE 'SENT'
				END,
				CASE WHEN NOT coalesce(p.opted_in, true)
				     THEN 'Opted out of marketing' END,
				CASE WHEN coalesce(p.opted_in, true) THEN $2::timestamptz END,
				CASE WHEN coalesce(p.opted_in, true)
				      AND ('x' || substr(md5($1 || m.id), 1, 2))::bit(8)::int % 2 = 0
				     THEN $2::timestamptz + interval '40 minutes' END,
				CASE WHEN coalesce(p.opted_in, true)
				      AND ('x' || substr(md5($1 || m.id), 1, 2))::bit(8)::int % 5 = 0
				     THEN $2::timestamptz + interval '45 minutes' END,
				$2
			FROM identity.members m
			LEFT JOIN crm.contact_preferences p
			       ON p.member_id = m.id AND p.channel = 'PUSH'
			WHERE m.status = 'ACTIVE'
			ON CONFLICT (campaign_id, member_id) DO NOTHING`, c.id, at); err != nil {
			return 0, fmt.Errorf("seed: campaign recipients %s: %w", c.id, err)
		}

		if _, err := s.db.Exec(ctx, `
			UPDATE engagement.campaigns SET sent_count = (
				SELECT count(*) FROM engagement.campaign_recipients
				WHERE campaign_id = $1 AND status <> 'SKIPPED'
			) WHERE id = $1`, c.id); err != nil {
			return 0, fmt.Errorf("seed: campaign count %s: %w", c.id, err)
		}
	}

	if err := s.seedNotifications(ctx); err != nil {
		return 0, err
	}
	return len(campaigns), nil
}

// seedNotifications fills each member's bell.
//
// Derived from their own bookings and campaigns, because a notification list
// that does not match what the member did is the one screen where the seam
// between real and demo data shows immediately.
func (s *Seeder) seedNotifications(ctx context.Context) error {
	// Confirmations for upcoming bookings, and a reminder for the next one.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO engagement.member_notifications
			(id, member_id, type, title, body, created_at, read_at)
		SELECT
			'ntf_bk_' || substr(md5(b.id), 1, 16), b.member_id, 'BOOKING_CONFIRMED',
			'Booking confirmed',
			ct.name || ' on ' || to_char(s.starts_at, 'Dy DD Mon at HH24:MI'),
			b.created_at,
			-- Older notifications have been read; the last few days have not.
			CASE WHEN b.created_at < now() - interval '3 days' THEN b.created_at + interval '2 hours' END
		FROM scheduling.bookings b
		JOIN scheduling.class_sessions s ON s.id = b.session_id
		JOIN catalog.class_types ct ON ct.id = s.class_type_id
		WHERE b.status = 'CONFIRMED'
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: booking notifications: %w", err)
	}

	// Visit logged, for the classes they attended.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO engagement.member_notifications
			(id, member_id, type, title, body, created_at, read_at)
		SELECT
			'ntf_vs_' || substr(md5(b.id), 1, 16), b.member_id, 'VISIT_LOGGED',
			'Visit logged',
			'1 credit used for ' || ct.name || '. Nice work.',
			b.checked_in_at, b.checked_in_at + interval '5 hours'
		FROM scheduling.bookings b
		JOIN scheduling.class_sessions s ON s.id = b.session_id
		JOIN catalog.class_types ct ON ct.id = s.class_type_id
		WHERE b.status = 'COMPLETED' AND b.checked_in_at IS NOT NULL
		  AND b.checked_in_at > now() - interval '21 days'
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: visit notifications: %w", err)
	}

	// The campaign pushes, linked back to the campaign that sent them.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO engagement.member_notifications
			(id, member_id, type, title, body, campaign_id, created_at, read_at)
		SELECT
			'ntf_cp_' || substr(md5(r.id), 1, 16), r.member_id, 'ANNOUNCEMENT',
			c.name, c.message, c.id, r.sent_at,
			CASE WHEN r.status IN ('OPENED', 'CLICKED') THEN r.opened_at END
		FROM engagement.campaign_recipients r
		JOIN engagement.campaigns c ON c.id = r.campaign_id
		WHERE r.status <> 'SKIPPED' AND r.sent_at IS NOT NULL
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: campaign notifications: %w", err)
	}

	// A low-balance warning for whoever is actually low, so the one screen
	// that is supposed to be driven by the balance is driven by the balance.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO engagement.member_notifications
			(id, member_id, type, title, body, created_at)
		SELECT 'ntf_low_' || l.member_id, l.member_id, 'LOW_BALANCE',
		       'Running low on credits',
		       'You have ' || sum(l.amount) || ' credits left. Top up to keep booking.',
		       now() - interval '1 day'
		FROM wallet.credit_ledger_entries l
		GROUP BY l.member_id
		HAVING sum(l.amount) BETWEEN 1 AND 10
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed: low balance notifications: %w", err)
	}
	return nil
}
