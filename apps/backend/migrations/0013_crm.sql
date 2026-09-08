-- CRM: the loyalty side of a membership.
--
-- The wallet already answers "what has this member paid for". This answers a
-- different question — "how much are they worth to us, and what have we
-- promised them for it" — and the two must never be confused. XP is not
-- money and does not buy classes; credits are money and do.
--
-- The XP ledger is append-only for the same reason the credit ledger is, with
-- one addition: an idempotency key, because XP is earned from events that
-- arrive more than once.

CREATE SCHEMA IF NOT EXISTS crm;

-- A tier is what a member has earned their way into. Rank orders them; the
-- thresholds decide who is in.
CREATE TABLE crm.tiers (
    id              TEXT PRIMARY KEY,
    code            TEXT        NOT NULL UNIQUE,
    name            TEXT        NOT NULL,
    rank            INTEGER     NOT NULL UNIQUE CHECK (rank > 0),
    min_lifetime_xp INTEGER     NOT NULL DEFAULT 0 CHECK (min_lifetime_xp >= 0),
    min_spend_idr   NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (min_spend_idr >= 0),
    -- Higher tiers earn faster. This is the multiplier applied when a rule
    -- says it should be.
    xp_multiplier   NUMERIC(6, 2) NOT NULL DEFAULT 1 CHECK (xp_multiplier >= 0),
    discount_percent NUMERIC(6, 2) NOT NULL DEFAULT 0
                    CHECK (discount_percent >= 0 AND discount_percent <= 100),
    benefits        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    colour          TEXT        NOT NULL DEFAULT '#5F6B62',
    active          BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX tiers_rank_idx ON crm.tiers (active, rank);

-- The loyalty layer over a member. One row per member, created on their first
-- earned point rather than at sign-up, so the table is people who actually
-- engage rather than everybody who ever registered.
CREATE TABLE crm.member_profiles (
    id           TEXT PRIMARY KEY,
    -- Plain id: members live in the identity schema.
    member_id    TEXT        NOT NULL UNIQUE,
    member_code  TEXT        NOT NULL UNIQUE,
    tier_code    TEXT        NOT NULL REFERENCES crm.tiers (code),
    -- current_xp is spendable and goes down when it is spent. lifetime_xp only
    -- ever grows, which is what makes a tier something you cannot fall out of
    -- by redeeming a reward.
    current_xp   INTEGER     NOT NULL DEFAULT 0 CHECK (current_xp >= 0),
    lifetime_xp  INTEGER     NOT NULL DEFAULT 0 CHECK (lifetime_xp >= 0),
    spent_xp     INTEGER     NOT NULL DEFAULT 0 CHECK (spent_xp >= 0),
    -- What they have paid the studio, ever. The other half of a tier rule.
    lifetime_spend_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (lifetime_spend_idr >= 0),
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ,
    status       TEXT        NOT NULL DEFAULT 'ACTIVE'
                 CHECK (status IN ('ACTIVE', 'INACTIVE', 'SUSPENDED')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Spent plus current cannot exceed what was ever earned. The arithmetic
    -- holds on the row, so a bug cannot mint points.
    CONSTRAINT member_profiles_xp_balances CHECK (current_xp + spent_xp = lifetime_xp)
);

CREATE INDEX member_profiles_tier_idx ON crm.member_profiles (tier_code, status);
CREATE INDEX member_profiles_lifetime_idx ON crm.member_profiles (lifetime_xp DESC);

-- How XP is earned. Rules are data because the answer to "how many points for
-- a class" changes with every campaign.
CREATE TABLE crm.xp_rules (
    id            TEXT PRIMARY KEY,
    code          TEXT        NOT NULL UNIQUE,
    name          TEXT        NOT NULL,
    -- Where the event came from, and what kind it was.
    source_channel TEXT       NOT NULL
                  CHECK (source_channel IN ('POS', 'BOOKING', 'CLASS', 'PAYMENT', 'MANUAL', 'CAMPAIGN')),
    source_type   TEXT        NOT NULL DEFAULT '',
    -- Narrows a rule to one class type, one package, one product.
    source_id     TEXT,
    branch_id     TEXT,
    xp_mode       TEXT        NOT NULL
                  CHECK (xp_mode IN ('FIXED', 'PER_ITEM', 'PER_AMOUNT', 'MULTIPLIER', 'PERCENTAGE')),
    xp_value      NUMERIC(14, 4) NOT NULL CHECK (xp_value >= 0),
    -- For PER_AMOUNT: one xp_value per this many rupiah.
    amount_step   NUMERIC(14, 2) NOT NULL DEFAULT 1 CHECK (amount_step > 0),
    min_amount    NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (min_amount >= 0),
    max_xp_per_event INTEGER CHECK (max_xp_per_event IS NULL OR max_xp_per_event >= 0),
    tier_multiplier_enabled BOOLEAN NOT NULL DEFAULT true,
    -- Highest priority wins when several rules match.
    priority      INTEGER     NOT NULL DEFAULT 100,
    starts_at     TIMESTAMPTZ,
    ends_at       TIMESTAMPTZ,
    active        BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT xp_rules_window CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX xp_rules_match_idx ON crm.xp_rules (source_channel, source_type, active, priority DESC);

-- Every change to a member's XP, forever.
CREATE TABLE crm.xp_ledger (
    id              TEXT PRIMARY KEY,
    member_id       TEXT        NOT NULL,
    direction       TEXT        NOT NULL
                    CHECK (direction IN ('EARN', 'SPEND', 'ADJUST', 'REVERSE', 'EXPIRE')),
    source_channel  TEXT        NOT NULL,
    source_type     TEXT        NOT NULL DEFAULT '',
    source_id       TEXT,
    branch_id       TEXT,
    -- Signed. Never zero, because an entry that changed nothing is not an entry.
    xp_delta        INTEGER     NOT NULL CHECK (xp_delta <> 0),
    balance_before  INTEGER     NOT NULL CHECK (balance_before >= 0),
    balance_after   INTEGER     NOT NULL CHECK (balance_after >= 0),
    lifetime_before INTEGER     NOT NULL DEFAULT 0 CHECK (lifetime_before >= 0),
    lifetime_after  INTEGER     NOT NULL DEFAULT 0 CHECK (lifetime_after >= 0),
    rule_id         TEXT REFERENCES crm.xp_rules (id),
    reference_type  TEXT,
    reference_id    TEXT,
    -- The reason this table differs from the credit ledger. XP is earned from
    -- events that arrive more than once — an outbox redelivery, a till
    -- retrying — and this is what makes posting it twice impossible.
    idempotency_key TEXT,
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT xp_ledger_arithmetic CHECK (balance_after = balance_before + xp_delta)
);

CREATE UNIQUE INDEX xp_ledger_idempotency_idx ON crm.xp_ledger (idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX xp_ledger_member_idx ON crm.xp_ledger (member_id, created_at DESC);
CREATE INDEX xp_ledger_reference_idx ON crm.xp_ledger (reference_type, reference_id);

CREATE TRIGGER xp_ledger_append_only
    BEFORE UPDATE OR DELETE ON crm.xp_ledger
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- What XP buys.
CREATE TABLE crm.rewards (
    id            TEXT PRIMARY KEY,
    code          TEXT        NOT NULL UNIQUE,
    name          TEXT        NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    reward_type   TEXT        NOT NULL
                  CHECK (reward_type IN ('DISCOUNT', 'MERCHANDISE', 'VOUCHER', 'CREDITS', 'CLASS', 'CUSTOM')),
    xp_cost       INTEGER     NOT NULL CHECK (xp_cost >= 0),
    required_tier_code TEXT REFERENCES crm.tiers (code),
    -- What the reward actually hands over: a number of credits, a voucher
    -- code, an inventory item. Shape depends on reward_type.
    reward_value  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    -- NULL is unlimited. stock_redeemed can never pass it.
    stock_total   INTEGER     CHECK (stock_total IS NULL OR stock_total >= 0),
    stock_redeemed INTEGER    NOT NULL DEFAULT 0 CHECK (stock_redeemed >= 0),
    max_per_member INTEGER    CHECK (max_per_member IS NULL OR max_per_member > 0),
    image_url     TEXT,
    starts_at     TIMESTAMPTZ,
    ends_at       TIMESTAMPTZ,
    active        BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT rewards_window CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at),
    -- More cannot be given away than exists.
    CONSTRAINT rewards_stock_not_oversubscribed
        CHECK (stock_total IS NULL OR stock_redeemed <= stock_total)
);

CREATE INDEX rewards_active_idx ON crm.rewards (active, xp_cost);

-- Somebody claiming a reward.
CREATE TABLE crm.redemptions (
    id               TEXT PRIMARY KEY,
    redemption_number TEXT       NOT NULL UNIQUE,
    member_id        TEXT        NOT NULL,
    reward_id        TEXT        NOT NULL REFERENCES crm.rewards (id),
    xp_cost          INTEGER     NOT NULL CHECK (xp_cost >= 0),
    -- The ledger entry that took the points. A redemption without one would be
    -- a promise nobody paid for.
    xp_ledger_id     TEXT REFERENCES crm.xp_ledger (id),
    status           TEXT        NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING', 'APPROVED', 'FULFILLED', 'CANCELLED', 'EXPIRED')),
    voucher_code     TEXT UNIQUE,
    requested_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at      TIMESTAMPTZ,
    fulfilled_at     TIMESTAMPTZ,
    cancelled_at     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ,
    note             TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX redemptions_member_idx ON crm.redemptions (member_id, requested_at DESC);
CREATE INDEX redemptions_status_idx ON crm.redemptions (status, requested_at DESC);
CREATE INDEX redemptions_reward_idx ON crm.redemptions (reward_id);
