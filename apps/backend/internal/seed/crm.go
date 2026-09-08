package seed

import (
	"context"
	"fmt"
)

// seedLoyalty sets up the scheme: three tiers, the rules that earn points, and
// a catalogue to spend them on.
func (s *Seeder) seedLoyalty(ctx context.Context) (int, error) {
	tiers := []struct {
		id, code, name             string
		rank, minXP                int
		minSpend, multiplier, disc float64
		colour                     string
	}{
		{"tir_bronze", "BRONZE", "Bronze", 1, 0, 0, 1.0, 0, "#A97142"},
		{"tir_silver", "SILVER", "Silver", 2, 500, 0, 1.25, 5, "#8E9AA3"},
		{"tir_gold", "GOLD", "Gold", 3, 2000, 5_000_000, 1.5, 10, "#C9A227"},
	}
	for _, t := range tiers {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO crm.tiers (id, code, name, rank, min_lifetime_xp, min_spend_idr,
				xp_multiplier, discount_percent, colour, benefits)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)
			ON CONFLICT (id) DO NOTHING`,
			t.id, t.code, t.name, t.rank, t.minXP, t.minSpend, t.multiplier, t.disc, t.colour,
			fmt.Sprintf(`["%.0f%% off merchandise", "%.2fx points"]`, t.disc, t.multiplier)); err != nil {
			return 0, fmt.Errorf("seed: tier %s: %w", t.id, err)
		}
	}

	// How points are earned. Booking is worth less than turning up, because
	// the studio would rather people came than reserved.
	rules := []struct {
		id, code, name, channel, sourceType, mode string
		value, step                               float64
		priority                                  int
		tierMultiplier                            bool
	}{
		{"xpr_booking", "BOOKING_BASE", "Booked a class", "BOOKING", "CONFIRMED", "FIXED",
			5, 1, 100, false},
		{"xpr_attend", "CLASS_ATTENDED", "Turned up to a class", "CLASS", "ATTENDED", "FIXED",
			25, 1, 100, true},
		// One point per 10.000 rupiah spent on credits.
		{"xpr_topup", "TOPUP_SPEND", "Topped up the wallet", "PAYMENT", "TOP_UP", "PER_AMOUNT",
			1, 10_000, 100, true},
		// The counter, once POS is selling: one point per 5.000 rupiah.
		{"xpr_pos", "POS_SPEND", "Bought at the counter", "POS", "SALE", "PER_AMOUNT",
			1, 5_000, 100, true},
	}
	for _, r := range rules {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO crm.xp_rules (id, code, name, source_channel, source_type, xp_mode,
				xp_value, amount_step, priority, tier_multiplier_enabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT (id) DO NOTHING`,
			r.id, r.code, r.name, r.channel, r.sourceType, r.mode, r.value, r.step,
			r.priority, r.tierMultiplier); err != nil {
			return 0, fmt.Errorf("seed: xp rule %s: %w", r.id, err)
		}
	}

	// What the points buy. Deliberately spread across the tiers so the
	// catalogue has something to climb towards.
	rewards := []struct {
		id, code, name, kind, tier string
		cost                       int
		value                      string
		stock, perMember           *int
	}{
		{"rwd_shaker", "FREE_SHAKER", "Free protein shaker", "MERCHANDISE", "",
			400, `{"itemId":"itm_shaker"}`, intPtr(50), intPtr(1)},
		{"rwd_bar", "FREE_BAR", "Free protein bar", "MERCHANDISE", "",
			150, `{"itemId":"itm_bar"}`, nil, intPtr(4)},
		{"rwd_class", "FREE_CLASS", "One free class", "CREDITS", "SILVER",
			800, `{"credits":1}`, nil, intPtr(2)},
		{"rwd_tee", "FREE_TEE", "Free training tee", "MERCHANDISE", "SILVER",
			1200, `{"itemId":"itm_tee"}`, intPtr(30), intPtr(1)},
		{"rwd_month", "GOLD_MONTH", "Ten free classes", "CREDITS", "GOLD",
			5000, `{"credits":10}`, intPtr(10), intPtr(1)},
	}
	for _, r := range rewards {
		var tier any
		if r.tier != "" {
			tier = r.tier
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO crm.rewards (id, code, name, reward_type, xp_cost, required_tier_code,
				reward_value, stock_total, max_per_member)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9) ON CONFLICT (id) DO NOTHING`,
			r.id, r.code, r.name, r.kind, r.cost, tier, r.value, r.stock, r.perMember); err != nil {
			return 0, fmt.Errorf("seed: reward %s: %w", r.id, err)
		}
	}
	return len(tiers), nil
}

func intPtr(v int) *int { return &v }
