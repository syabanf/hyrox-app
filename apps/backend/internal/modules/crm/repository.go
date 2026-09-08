package crm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Repository persists loyalty. It reads and writes the `crm` schema only;
// members are referenced by plain id.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── Tiers ────────────────────────────────────────────────────────────────────

const tierColumns = `id, code, name, rank, min_lifetime_xp, min_spend_idr, xp_multiplier,
	discount_percent, benefits, colour, active, created_at, updated_at`

func scanTier(row pgx.Row) (domain.Tier, error) {
	var t domain.Tier
	var benefits []byte
	err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Rank, &t.MinLifetimeXP, &t.MinSpendIDR,
		&t.XPMultiplier, &t.DiscountPercent, &benefits, &t.Colour, &t.Active,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return domain.Tier{}, err
	}
	t.Benefits = []string{}
	_ = json.Unmarshal(benefits, &t.Benefits)
	return t, nil
}

func (r *Repository) Tiers(ctx context.Context) ([]domain.Tier, error) {
	rows, err := r.db.Query(ctx, `SELECT `+tierColumns+` FROM crm.tiers ORDER BY rank`)
	if err != nil {
		return nil, fmt.Errorf("crm: listing tiers: %w", err)
	}
	defer rows.Close()

	out := []domain.Tier{}
	for rows.Next() {
		t, err := scanTier(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning tier: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertTier(ctx context.Context, t domain.Tier) (domain.Tier, error) {
	benefits, _ := json.Marshal(t.Benefits)
	saved, err := scanTier(r.db.QueryRow(ctx, `
		INSERT INTO crm.tiers (id, code, name, rank, min_lifetime_xp, min_spend_idr,
			xp_multiplier, discount_percent, benefits, colour, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, rank = EXCLUDED.rank,
			min_lifetime_xp = EXCLUDED.min_lifetime_xp, min_spend_idr = EXCLUDED.min_spend_idr,
			xp_multiplier = EXCLUDED.xp_multiplier, discount_percent = EXCLUDED.discount_percent,
			benefits = EXCLUDED.benefits, colour = EXCLUDED.colour, active = EXCLUDED.active,
			updated_at = now()
		RETURNING `+tierColumns,
		t.ID, t.Code, t.Name, t.Rank, t.MinLifetimeXP, t.MinSpendIDR, t.XPMultiplier,
		t.DiscountPercent, benefits, t.Colour, t.Active))
	if database.IsUniqueViolation(err) {
		return domain.Tier{}, httpx.Conflict("DUPLICATE", "Another tier already holds that rank.")
	}
	if err != nil {
		return domain.Tier{}, fmt.Errorf("crm: saving tier: %w", err)
	}
	return saved, nil
}

// ── Profiles ─────────────────────────────────────────────────────────────────

const profileColumns = `id, member_id, member_code, tier_code, current_xp, lifetime_xp,
	spent_xp, lifetime_spend_idr, joined_at, last_activity_at, status, created_at, updated_at`

func scanProfile(row pgx.Row) (domain.MemberProfile, error) {
	var p domain.MemberProfile
	err := row.Scan(&p.ID, &p.MemberID, &p.MemberCode, &p.TierCode, &p.CurrentXP,
		&p.LifetimeXP, &p.SpentXP, &p.LifetimeSpendIDR, &p.JoinedAt, &p.LastActivityAt,
		&p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// ProfileFilter narrows the member list.
type ProfileFilter struct {
	TierCode string
	Status   string
	Limit    int
}

func (r *Repository) Profiles(ctx context.Context, filter ProfileFilter) ([]domain.MemberProfile, error) {
	query := `SELECT ` + profileColumns + ` FROM crm.member_profiles WHERE 1 = 1`
	args := []any{}
	if filter.TierCode != "" {
		args = append(args, filter.TierCode)
		query += fmt.Sprintf(` AND tier_code = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY lifetime_xp DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing profiles: %w", err)
	}
	defer rows.Close()

	out := []domain.MemberProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning profile: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Profile reads one, creating it on first touch: a profile exists because
// somebody engaged, not because they registered.
func (r *Repository) Profile(ctx context.Context, memberID, profileID, memberCode, defaultTier string, forUpdate bool) (domain.MemberProfile, error) {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO crm.member_profiles (id, member_id, member_code, tier_code)
		VALUES ($1, $2, $3, $4) ON CONFLICT (member_id) DO NOTHING`,
		profileID, memberID, memberCode, defaultTier); err != nil {
		if database.IsForeignKeyViolation(err) {
			return domain.MemberProfile{}, httpx.NotFound("tier")
		}
		return domain.MemberProfile{}, fmt.Errorf("crm: ensuring profile: %w", err)
	}

	query := `SELECT ` + profileColumns + ` FROM crm.member_profiles WHERE member_id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	p, err := scanProfile(r.db.QueryRow(ctx, query, memberID))
	if err != nil {
		return domain.MemberProfile{}, fmt.Errorf("crm: reading profile: %w", err)
	}
	return p, nil
}

// ProfileByMember reads without creating, for callers that only want to look.
func (r *Repository) ProfileByMember(ctx context.Context, memberID string) (domain.MemberProfile, bool, error) {
	p, err := scanProfile(r.db.QueryRow(ctx,
		`SELECT `+profileColumns+` FROM crm.member_profiles WHERE member_id = $1`, memberID))
	if database.IsNoRows(err) {
		return domain.MemberProfile{}, false, nil
	}
	if err != nil {
		return domain.MemberProfile{}, false, fmt.Errorf("crm: reading profile: %w", err)
	}
	return p, true, nil
}

func (r *Repository) SaveProfile(ctx context.Context, p domain.MemberProfile) (domain.MemberProfile, error) {
	saved, err := scanProfile(r.db.QueryRow(ctx, `
		UPDATE crm.member_profiles SET tier_code = $2, current_xp = $3, lifetime_xp = $4,
			spent_xp = $5, lifetime_spend_idr = $6, last_activity_at = $7, status = $8,
			updated_at = now()
		WHERE member_id = $1 RETURNING `+profileColumns,
		p.MemberID, p.TierCode, p.CurrentXP, p.LifetimeXP, p.SpentXP, p.LifetimeSpendIDR,
		p.LastActivityAt, p.Status))
	if database.IsNoRows(err) {
		return domain.MemberProfile{}, httpx.NotFound("member profile")
	}
	if database.IsCheckViolation(err) {
		return domain.MemberProfile{}, httpx.Conflict("XP_IMBALANCE",
			"That would leave the point balances not adding up.")
	}
	if err != nil {
		return domain.MemberProfile{}, fmt.Errorf("crm: saving profile: %w", err)
	}
	return saved, nil
}

// ── Rules ────────────────────────────────────────────────────────────────────

const ruleColumns = `id, code, name, source_channel, source_type, source_id, branch_id,
	xp_mode, xp_value, amount_step, min_amount, max_xp_per_event, tier_multiplier_enabled,
	priority, starts_at, ends_at, active, created_at, updated_at`

func scanRule(row pgx.Row) (domain.XPRule, error) {
	var r domain.XPRule
	err := row.Scan(&r.ID, &r.Code, &r.Name, &r.SourceChannel, &r.SourceType, &r.SourceID,
		&r.BranchID, &r.XPMode, &r.XPValue, &r.AmountStep, &r.MinAmount, &r.MaxXPPerEvent,
		&r.TierMultiplierEnabled, &r.Priority, &r.StartsAt, &r.EndsAt, &r.Active,
		&r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *Repository) Rules(ctx context.Context, channel string, activeOnly bool) ([]domain.XPRule, error) {
	query := `SELECT ` + ruleColumns + ` FROM crm.xp_rules WHERE 1 = 1`
	args := []any{}
	if channel != "" {
		args = append(args, channel)
		query += fmt.Sprintf(` AND source_channel = $%d`, len(args))
	}
	if activeOnly {
		query += ` AND active`
	}
	query += ` ORDER BY priority DESC, code`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing rules: %w", err)
	}
	defer rows.Close()

	out := []domain.XPRule{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning rule: %w", err)
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertRule(ctx context.Context, rule domain.XPRule) (domain.XPRule, error) {
	saved, err := scanRule(r.db.QueryRow(ctx, `
		INSERT INTO crm.xp_rules (id, code, name, source_channel, source_type, source_id,
			branch_id, xp_mode, xp_value, amount_step, min_amount, max_xp_per_event,
			tier_multiplier_enabled, priority, starts_at, ends_at, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name,
			source_channel = EXCLUDED.source_channel, source_type = EXCLUDED.source_type,
			source_id = EXCLUDED.source_id, branch_id = EXCLUDED.branch_id,
			xp_mode = EXCLUDED.xp_mode, xp_value = EXCLUDED.xp_value,
			amount_step = EXCLUDED.amount_step, min_amount = EXCLUDED.min_amount,
			max_xp_per_event = EXCLUDED.max_xp_per_event,
			tier_multiplier_enabled = EXCLUDED.tier_multiplier_enabled,
			priority = EXCLUDED.priority, starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at, active = EXCLUDED.active, updated_at = now()
		RETURNING `+ruleColumns,
		rule.ID, rule.Code, rule.Name, rule.SourceChannel, rule.SourceType, rule.SourceID,
		rule.BranchID, rule.XPMode, rule.XPValue, rule.AmountStep, rule.MinAmount,
		rule.MaxXPPerEvent, rule.TierMultiplierEnabled, rule.Priority, rule.StartsAt,
		rule.EndsAt, rule.Active))
	if database.IsCheckViolation(err) {
		return domain.XPRule{}, httpx.Invalid("A rule's window must end after it starts.")
	}
	if err != nil {
		return domain.XPRule{}, fmt.Errorf("crm: saving rule: %w", err)
	}
	return saved, nil
}

// ── Ledger ───────────────────────────────────────────────────────────────────

const entryColumns = `id, member_id, direction, source_channel, source_type, source_id,
	branch_id, xp_delta, balance_before, balance_after, lifetime_before, lifetime_after,
	rule_id, reference_type, reference_id, idempotency_key, description, created_at`

func scanEntry(row pgx.Row) (domain.XPEntry, error) {
	var e domain.XPEntry
	err := row.Scan(&e.ID, &e.MemberID, &e.Direction, &e.SourceChannel, &e.SourceType,
		&e.SourceID, &e.BranchID, &e.XPDelta, &e.BalanceBefore, &e.BalanceAfter,
		&e.LifetimeBefore, &e.LifetimeAfter, &e.RuleID, &e.ReferenceType, &e.ReferenceID,
		&e.IdempotencyKey, &e.Description, &e.CreatedAt)
	return e, err
}

func (r *Repository) Entries(ctx context.Context, memberID, direction string, limit int) ([]domain.XPEntry, error) {
	query := `SELECT ` + entryColumns + ` FROM crm.xp_ledger WHERE 1 = 1`
	args := []any{}
	if memberID != "" {
		args = append(args, memberID)
		query += fmt.Sprintf(` AND member_id = $%d`, len(args))
	}
	if direction != "" {
		args = append(args, direction)
		query += fmt.Sprintf(` AND direction = $%d`, len(args))
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing ledger: %w", err)
	}
	defer rows.Close()

	out := []domain.XPEntry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning ledger entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EntryByKey finds an entry already posted for an event, which is how the same
// event arriving twice becomes a no-op rather than a second award.
func (r *Repository) EntryByKey(ctx context.Context, key string) (domain.XPEntry, bool, error) {
	e, err := scanEntry(r.db.QueryRow(ctx,
		`SELECT `+entryColumns+` FROM crm.xp_ledger WHERE idempotency_key = $1`, key))
	if database.IsNoRows(err) {
		return domain.XPEntry{}, false, nil
	}
	if err != nil {
		return domain.XPEntry{}, false, fmt.Errorf("crm: reading ledger entry by key: %w", err)
	}
	return e, true, nil
}

func (r *Repository) InsertEntry(ctx context.Context, e domain.XPEntry) (domain.XPEntry, error) {
	created, err := scanEntry(r.db.QueryRow(ctx, `
		INSERT INTO crm.xp_ledger (id, member_id, direction, source_channel, source_type,
			source_id, branch_id, xp_delta, balance_before, balance_after, lifetime_before,
			lifetime_after, rule_id, reference_type, reference_id, idempotency_key, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING `+entryColumns,
		e.ID, e.MemberID, e.Direction, e.SourceChannel, e.SourceType, e.SourceID, e.BranchID,
		e.XPDelta, e.BalanceBefore, e.BalanceAfter, e.LifetimeBefore, e.LifetimeAfter,
		e.RuleID, e.ReferenceType, e.ReferenceID, e.IdempotencyKey, e.Description))
	if database.IsUniqueViolation(err) {
		return domain.XPEntry{}, httpx.Conflict("ALREADY_POSTED",
			"Those points have already been awarded for that event.")
	}
	if err != nil {
		return domain.XPEntry{}, fmt.Errorf("crm: inserting ledger entry: %w", err)
	}
	return created, nil
}

// SumEntries is what the ledger says a balance should be, for the test that
// proves the profile never drifts from its history.
func (r *Repository) SumEntries(ctx context.Context, memberID string) (int, error) {
	var total int
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(xp_delta), 0) FROM crm.xp_ledger WHERE member_id = $1`,
		memberID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("crm: summing ledger: %w", err)
	}
	return total, nil
}

// ── Rewards ──────────────────────────────────────────────────────────────────

const rewardColumns = `id, code, name, description, reward_type, xp_cost, required_tier_code,
	reward_value, stock_total, stock_redeemed, max_per_member, image_url, starts_at, ends_at,
	active, created_at, updated_at`

func scanReward(row pgx.Row) (domain.Reward, error) {
	var r domain.Reward
	var value []byte
	err := row.Scan(&r.ID, &r.Code, &r.Name, &r.Description, &r.RewardType, &r.XPCost,
		&r.RequiredTierCode, &value, &r.StockTotal, &r.StockRedeemed, &r.MaxPerMember,
		&r.ImageURL, &r.StartsAt, &r.EndsAt, &r.Active, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return domain.Reward{}, err
	}
	r.RewardValue = map[string]any{}
	_ = json.Unmarshal(value, &r.RewardValue)
	return r, nil
}

func (r *Repository) Rewards(ctx context.Context, activeOnly bool) ([]domain.Reward, error) {
	query := `SELECT ` + rewardColumns + ` FROM crm.rewards`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY xp_cost`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("crm: listing rewards: %w", err)
	}
	defer rows.Close()

	out := []domain.Reward{}
	for rows.Next() {
		reward, err := scanReward(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning reward: %w", err)
		}
		out = append(out, reward)
	}
	return out, rows.Err()
}

func (r *Repository) Reward(ctx context.Context, id string, forUpdate bool) (domain.Reward, error) {
	query := `SELECT ` + rewardColumns + ` FROM crm.rewards WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	reward, err := scanReward(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.Reward{}, httpx.NotFound("reward")
	}
	if err != nil {
		return domain.Reward{}, fmt.Errorf("crm: reading reward: %w", err)
	}
	return reward, nil
}

func (r *Repository) UpsertReward(ctx context.Context, reward domain.Reward) (domain.Reward, error) {
	value, _ := json.Marshal(reward.RewardValue)
	saved, err := scanReward(r.db.QueryRow(ctx, `
		INSERT INTO crm.rewards (id, code, name, description, reward_type, xp_cost,
			required_tier_code, reward_value, stock_total, max_per_member, image_url,
			starts_at, ends_at, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name,
			description = EXCLUDED.description, reward_type = EXCLUDED.reward_type,
			xp_cost = EXCLUDED.xp_cost, required_tier_code = EXCLUDED.required_tier_code,
			reward_value = EXCLUDED.reward_value, stock_total = EXCLUDED.stock_total,
			max_per_member = EXCLUDED.max_per_member, image_url = EXCLUDED.image_url,
			starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at,
			active = EXCLUDED.active, updated_at = now()
		RETURNING `+rewardColumns,
		reward.ID, reward.Code, reward.Name, reward.Description, reward.RewardType,
		reward.XPCost, reward.RequiredTierCode, value, reward.StockTotal,
		reward.MaxPerMember, reward.ImageURL, reward.StartsAt, reward.EndsAt, reward.Active))
	if database.IsCheckViolation(err) {
		return domain.Reward{}, httpx.Invalid("A reward cannot have less stock than has been claimed.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.Reward{}, httpx.NotFound("tier")
	}
	if err != nil {
		return domain.Reward{}, fmt.Errorf("crm: saving reward: %w", err)
	}
	return saved, nil
}

// ClaimRewardStock takes one from the shelf. The CHECK is what makes handing
// out more than exists impossible even under a race.
func (r *Repository) ClaimRewardStock(ctx context.Context, rewardID string, delta int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE crm.rewards SET stock_redeemed = GREATEST(0, stock_redeemed + $2), updated_at = now()
		 WHERE id = $1`, rewardID, delta)
	if database.IsCheckViolation(err) {
		return httpx.Conflict("OUT_OF_STOCK", "That reward has all been claimed.")
	}
	if err != nil {
		return fmt.Errorf("crm: claiming reward stock: %w", err)
	}
	return nil
}

// ── Redemptions ──────────────────────────────────────────────────────────────

const redemptionColumns = `id, redemption_number, member_id, reward_id, xp_cost, xp_ledger_id,
	status, voucher_code, requested_at, approved_at, fulfilled_at, cancelled_at, expires_at,
	note, created_at, updated_at`

func scanRedemption(row pgx.Row) (domain.Redemption, error) {
	var r domain.Redemption
	err := row.Scan(&r.ID, &r.RedemptionNumber, &r.MemberID, &r.RewardID, &r.XPCost,
		&r.XPLedgerID, &r.Status, &r.VoucherCode, &r.RequestedAt, &r.ApprovedAt,
		&r.FulfilledAt, &r.CancelledAt, &r.ExpiresAt, &r.Note, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// RedemptionFilter narrows the claim list.
type RedemptionFilter struct {
	MemberID string
	RewardID string
	Status   string
	Limit    int
}

func (r *Repository) Redemptions(ctx context.Context, filter RedemptionFilter) ([]domain.Redemption, error) {
	query := `SELECT ` + redemptionColumns + ` FROM crm.redemptions WHERE 1 = 1`
	args := []any{}
	if filter.MemberID != "" {
		args = append(args, filter.MemberID)
		query += fmt.Sprintf(` AND member_id = $%d`, len(args))
	}
	if filter.RewardID != "" {
		args = append(args, filter.RewardID)
		query += fmt.Sprintf(` AND reward_id = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY requested_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("crm: listing redemptions: %w", err)
	}
	defer rows.Close()

	out := []domain.Redemption{}
	for rows.Next() {
		red, err := scanRedemption(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scanning redemption: %w", err)
		}
		out = append(out, red)
	}
	return out, rows.Err()
}

func (r *Repository) Redemption(ctx context.Context, id string, forUpdate bool) (domain.Redemption, error) {
	query := `SELECT ` + redemptionColumns + ` FROM crm.redemptions WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	red, err := scanRedemption(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.Redemption{}, httpx.NotFound("redemption")
	}
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("crm: reading redemption: %w", err)
	}
	return red, nil
}

func (r *Repository) CountRedemptions(ctx context.Context, memberID, rewardID string) (int, error) {
	var count int
	// Cancelled claims gave the points back, so they do not count against a
	// per-member limit.
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM crm.redemptions
		WHERE member_id = $1 AND reward_id = $2 AND status <> 'CANCELLED'`,
		memberID, rewardID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("crm: counting redemptions: %w", err)
	}
	return count, nil
}

func (r *Repository) InsertRedemption(ctx context.Context, red domain.Redemption) (domain.Redemption, error) {
	created, err := scanRedemption(r.db.QueryRow(ctx, `
		INSERT INTO crm.redemptions (id, redemption_number, member_id, reward_id, xp_cost,
			xp_ledger_id, status, voucher_code, expires_at, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+redemptionColumns,
		red.ID, red.RedemptionNumber, red.MemberID, red.RewardID, red.XPCost,
		red.XPLedgerID, red.Status, red.VoucherCode, red.ExpiresAt, red.Note))
	if database.IsUniqueViolation(err) {
		return domain.Redemption{}, httpx.Conflict("DUPLICATE", "That redemption number is taken.")
	}
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("crm: inserting redemption: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveRedemption(ctx context.Context, red domain.Redemption) (domain.Redemption, error) {
	saved, err := scanRedemption(r.db.QueryRow(ctx, `
		UPDATE crm.redemptions SET status = $2, approved_at = $3, fulfilled_at = $4,
			cancelled_at = $5, voucher_code = $6, note = $7, updated_at = now()
		WHERE id = $1 RETURNING `+redemptionColumns,
		red.ID, red.Status, red.ApprovedAt, red.FulfilledAt, red.CancelledAt,
		red.VoucherCode, red.Note))
	if database.IsNoRows(err) {
		return domain.Redemption{}, httpx.NotFound("redemption")
	}
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("crm: saving redemption: %w", err)
	}
	return saved, nil
}

// ── Summary ──────────────────────────────────────────────────────────────────

// Summary backs the loyalty dashboard.
type Summary struct {
	Members         int            `json:"members"`
	OutstandingXP   int            `json:"outstandingXp"`
	LifetimeXP      int            `json:"lifetimeXp"`
	PendingClaims   int            `json:"pendingClaims"`
	EarnedThisMonth int            `json:"earnedThisMonth"`
	ByTier          map[string]int `json:"byTier"`
}

func (r *Repository) Summary(ctx context.Context, monthStart time.Time) (Summary, error) {
	s := Summary{ByTier: map[string]int{}}
	err := r.db.QueryRow(ctx, `
		SELECT count(*), COALESCE(SUM(current_xp), 0), COALESCE(SUM(lifetime_xp), 0)
		FROM crm.member_profiles WHERE status = 'ACTIVE'`).
		Scan(&s.Members, &s.OutstandingXP, &s.LifetimeXP)
	if err != nil {
		return Summary{}, fmt.Errorf("crm: summarizing profiles: %w", err)
	}

	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM crm.redemptions WHERE status = 'PENDING'`).
		Scan(&s.PendingClaims); err != nil {
		return Summary{}, fmt.Errorf("crm: counting pending claims: %w", err)
	}

	if err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(xp_delta), 0) FROM crm.xp_ledger
		WHERE direction = 'EARN' AND created_at >= $1`, monthStart).
		Scan(&s.EarnedThisMonth); err != nil {
		return Summary{}, fmt.Errorf("crm: summing this month: %w", err)
	}

	rows, err := r.db.Query(ctx,
		`SELECT tier_code, count(*) FROM crm.member_profiles GROUP BY tier_code`)
	if err != nil {
		return Summary{}, fmt.Errorf("crm: counting by tier: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var count int
		if err := rows.Scan(&code, &count); err != nil {
			return Summary{}, fmt.Errorf("crm: scanning tier count: %w", err)
		}
		s.ByTier[code] = count
	}
	return s, rows.Err()
}
