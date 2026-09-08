-- Wallet: money and credits. The two are deliberately separate — a payment is
-- an IDR transaction, a ledger entry is a credit movement, and a paid payment
-- produces both a TOP_UP entry and the lot it expires from.

CREATE SCHEMA IF NOT EXISTS wallet;

-- The credit ledger. A member's balance is the sum of these rows and is never
-- stored anywhere, so it cannot drift away from its own history.
CREATE TABLE wallet.credit_ledger_entries (
    id                TEXT PRIMARY KEY,
    member_id         TEXT        NOT NULL,
    type              TEXT        NOT NULL
                      CHECK (type IN ('TOP_UP', 'VISIT_DEDUCTION', 'REFUND', 'BONUS',
                                      'PROMO', 'EXPIRATION', 'ADJUSTMENT', 'REVERSAL')),
    -- Signed: positive adds credits, negative consumes them.
    amount            INTEGER     NOT NULL CHECK (amount <> 0),
    description       TEXT        NOT NULL,
    source_type       TEXT CHECK (source_type IN ('PAYMENT', 'BOOKING', 'ACCESS', 'ADMIN', 'SYSTEM')),
    source_id         TEXT,
    -- Set on REVERSAL rows: the entry this one cancels out.
    reverses_entry_id TEXT REFERENCES wallet.credit_ledger_entries (id),
    actor_id          TEXT,
    reason            TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Reading a member's wallet reads their whole ledger, so this index carries
-- the hottest query in the system.
CREATE INDEX ledger_member_idx ON wallet.credit_ledger_entries (member_id, created_at DESC);
CREATE INDEX ledger_source_idx ON wallet.credit_ledger_entries (source_type, source_id);
CREATE INDEX ledger_type_idx ON wallet.credit_ledger_entries (type, created_at DESC);
-- An entry may be reversed at most once; a second attempt hits this index.
CREATE UNIQUE INDEX ledger_reversal_idx ON wallet.credit_ledger_entries (reverses_entry_id)
    WHERE reverses_entry_id IS NOT NULL;

CREATE TRIGGER ledger_append_only
    BEFORE UPDATE OR DELETE ON wallet.credit_ledger_entries
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- A lot is one batch of credits and the date it dies. Lots exist so expiry can
-- be FIFO: the credits closest to expiring are spent first.
CREATE TABLE wallet.top_up_lots (
    id              TEXT PRIMARY KEY,
    member_id       TEXT        NOT NULL,
    ledger_entry_id TEXT        NOT NULL REFERENCES wallet.credit_ledger_entries (id),
    -- NULL for credits that were not purchased (bonus, adjustment).
    package_id      TEXT,
    credits         INTEGER     NOT NULL CHECK (credits > 0),
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX lots_member_idx ON wallet.top_up_lots (member_id, expires_at);
-- The expiry sweep asks for lots past their date across all members.
CREATE INDEX lots_expiry_idx ON wallet.top_up_lots (expires_at);
CREATE UNIQUE INDEX lots_entry_idx ON wallet.top_up_lots (ledger_entry_id);

CREATE TABLE wallet.payments (
    id           TEXT PRIMARY KEY,
    member_id    TEXT        NOT NULL,
    package_id   TEXT        NOT NULL,
    credits      INTEGER     NOT NULL CHECK (credits > 0),
    amount_idr   BIGINT      NOT NULL CHECK (amount_idr >= 0),
    discount_idr BIGINT      NOT NULL DEFAULT 0 CHECK (discount_idr >= 0),
    total_idr    BIGINT      NOT NULL CHECK (total_idr >= 0),
    voucher_code TEXT,
    channel      TEXT        NOT NULL CHECK (channel IN ('QRIS', 'EWALLET', 'VIRTUAL_ACCOUNT', 'CARD')),
    status       TEXT        NOT NULL DEFAULT 'PENDING'
                 CHECK (status IN ('DRAFT', 'PENDING', 'PAID', 'FAILED', 'EXPIRED', 'REFUNDED')),
    -- The gateway's own id, so a callback can be matched back to this row.
    external_id  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at      TIMESTAMPTZ,
    refunded_at  TIMESTAMPTZ,
    -- A discount can never exceed the price.
    CONSTRAINT payments_total_consistent CHECK (total_idr = amount_idr - discount_idr)
);

CREATE INDEX payments_member_idx ON wallet.payments (member_id, created_at DESC);
CREATE INDEX payments_status_idx ON wallet.payments (status, created_at DESC);
CREATE INDEX payments_package_idx ON wallet.payments (package_id);
-- Sales reports scan by settlement date, not creation date.
CREATE INDEX payments_paid_idx ON wallet.payments (paid_at DESC) WHERE paid_at IS NOT NULL;
CREATE UNIQUE INDEX payments_external_idx ON wallet.payments (external_id) WHERE external_id IS NOT NULL;

CREATE TABLE wallet.vouchers (
    id                     TEXT PRIMARY KEY,
    code                   TEXT        NOT NULL,
    type                   TEXT        NOT NULL CHECK (type IN ('FIXED_IDR', 'PERCENT')),
    value                  BIGINT      NOT NULL CHECK (value > 0),
    starts_at              TIMESTAMPTZ NOT NULL,
    ends_at                TIMESTAMPTZ NOT NULL,
    usage_limit            INTEGER CHECK (usage_limit > 0),
    per_member_limit       INTEGER CHECK (per_member_limit > 0),
    eligible_segment       TEXT        NOT NULL DEFAULT 'ALL' CHECK (eligible_segment IN ('ALL', 'NEW_MEMBERS')),
    -- NULL means the code works on every package.
    applicable_package_ids JSONB,
    status                 TEXT        NOT NULL DEFAULT 'DRAFT'
                           CHECK (status IN ('DRAFT', 'SCHEDULED', 'ACTIVE', 'EXPIRED', 'DISABLED')),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT vouchers_window_ordered CHECK (ends_at > starts_at),
    -- A percentage over 100 would pay the member to buy.
    CONSTRAINT vouchers_percent_bounded CHECK (type <> 'PERCENT' OR value <= 100)
);

CREATE UNIQUE INDEX vouchers_code_idx ON wallet.vouchers (upper(code));
CREATE INDEX vouchers_status_idx ON wallet.vouchers (status);

-- Redemptions are rows rather than a counter, so usage is auditable per member
-- and every discount traces to the payment it was applied to.
CREATE TABLE wallet.voucher_redemptions (
    id           TEXT PRIMARY KEY,
    voucher_id   TEXT        NOT NULL REFERENCES wallet.vouchers (id),
    member_id    TEXT        NOT NULL,
    payment_id   TEXT        NOT NULL REFERENCES wallet.payments (id),
    discount_idr BIGINT      NOT NULL CHECK (discount_idr >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX redemptions_voucher_idx ON wallet.voucher_redemptions (voucher_id);
CREATE INDEX redemptions_member_idx ON wallet.voucher_redemptions (voucher_id, member_id);
-- One payment carries at most one discount.
CREATE UNIQUE INDEX redemptions_payment_idx ON wallet.voucher_redemptions (payment_id);
