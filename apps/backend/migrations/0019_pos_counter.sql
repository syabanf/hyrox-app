-- The rest of the counter: offers, gift cards, tenders, receipts, and the
-- manager who has to be standing there.

-- ── Promotions ───────────────────────────────────────────────────────────────

-- An offer the till applies by itself.
--
-- Separate from the price break on a product, which says what one thing costs
-- at a quantity. A promotion is about a basket: three for two, a bundle at a
-- fixed price, ten percent off everything this weekend. The distinction
-- matters because a price break is a property of the product and a promotion
-- is a decision with a start and an end.
CREATE TABLE pos.promotions (
    id          TEXT PRIMARY KEY,
    code        TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    kind        TEXT        NOT NULL
                CHECK (kind IN ('PERCENT', 'AMOUNT', 'BUY_X_GET_Y', 'BUNDLE')),
    -- PERCENT: percent off. AMOUNT: rupiah off. BUY_X_GET_Y: buy_qty and
    -- free_qty. BUNDLE: bundle_price_idr for the whole set.
    percent          NUMERIC(5, 2) CHECK (percent IS NULL OR (percent > 0 AND percent <= 100)),
    amount_idr       NUMERIC(15, 2) CHECK (amount_idr IS NULL OR amount_idr > 0),
    buy_qty          NUMERIC(15, 3) CHECK (buy_qty IS NULL OR buy_qty > 0),
    free_qty         NUMERIC(15, 3) CHECK (free_qty IS NULL OR free_qty > 0),
    bundle_price_idr NUMERIC(15, 2) CHECK (bundle_price_idr IS NULL OR bundle_price_idr >= 0),
    -- What the basket has to reach before it applies at all.
    min_spend_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (min_spend_idr >= 0),
    min_qty       NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (min_qty >= 0),
    -- A promotion nobody has to ask for, or a code somebody types.
    requires_code BOOLEAN     NOT NULL DEFAULT false,
    -- Which channels it applies to. Empty means all of them.
    channels    TEXT[]      NOT NULL DEFAULT '{}',
    -- An exclusive promotion is the only one on the basket. Without this, two
    -- generous offers stack into a sale at a loss and nobody notices until the
    -- margin report.
    exclusive   BOOLEAN     NOT NULL DEFAULT false,
    priority    INTEGER     NOT NULL DEFAULT 0,
    starts_on   DATE,
    ends_on     DATE,
    -- How many times it may be used in total, and by one member.
    max_uses          INTEGER CHECK (max_uses IS NULL OR max_uses > 0),
    max_uses_per_member INTEGER CHECK (max_uses_per_member IS NULL OR max_uses_per_member > 0),
    used_count  INTEGER     NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- A window that ends before it starts would silently never apply.
    CONSTRAINT promotions_window CHECK (ends_on IS NULL OR starts_on IS NULL OR ends_on >= starts_on)
);

CREATE INDEX promotions_live_idx ON pos.promotions (priority DESC, id)
    WHERE active;
CREATE INDEX promotions_code_idx ON pos.promotions (code) WHERE requires_code;

-- What a promotion applies to. No rows means the whole basket.
CREATE TABLE pos.promotion_targets (
    id           TEXT PRIMARY KEY,
    promotion_id TEXT        NOT NULL REFERENCES pos.promotions (id) ON DELETE CASCADE,
    -- A product, or everything in a category.
    product_id   TEXT REFERENCES pos.products (id) ON DELETE CASCADE,
    category_id  TEXT REFERENCES pos.categories (id) ON DELETE CASCADE,
    -- For a bundle: how many of this product the bundle contains.
    qty          NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (qty > 0),
    CONSTRAINT promotion_targets_one_of
        CHECK ((product_id IS NULL) <> (category_id IS NULL))
);

CREATE INDEX promotion_targets_promotion_idx ON pos.promotion_targets (promotion_id);
CREATE INDEX promotion_targets_product_idx ON pos.promotion_targets (product_id);

-- Which promotions an order actually got, and what each was worth. Written at
-- completion so a receipt can be reproduced and a promotion's real cost can be
-- reported rather than estimated.
CREATE TABLE pos.order_promotions (
    id           TEXT PRIMARY KEY,
    order_id     TEXT        NOT NULL REFERENCES pos.orders (id) ON DELETE CASCADE,
    promotion_id TEXT        NOT NULL REFERENCES pos.promotions (id),
    code         TEXT        NOT NULL,
    name         TEXT        NOT NULL,
    discount_idr NUMERIC(15, 2) NOT NULL CHECK (discount_idr >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, promotion_id)
);

CREATE INDEX order_promotions_promotion_idx ON pos.order_promotions (promotion_id);

-- The order carries the promotion total apart from the manual and tier
-- discounts, for the same reason those are apart from each other: each is
-- recomputed by a different rule, and one column would make them compound.
ALTER TABLE pos.orders
    ADD COLUMN promo_discount_idr NUMERIC(15, 2) NOT NULL DEFAULT 0
        CHECK (promo_discount_idr >= 0),
    -- The code somebody typed, if they typed one.
    ADD COLUMN promo_code TEXT;

-- ── Gift cards ───────────────────────────────────────────────────────────────

-- A card with money on it.
--
-- Deliberately not the credit wallet: credits buy classes and are a liability
-- measured in sessions, while a gift card is money somebody has already paid
-- for and can spend on anything at the counter. Conflating them makes "what do
-- we owe" unanswerable.
CREATE TABLE pos.gift_cards (
    id          TEXT PRIMARY KEY,
    code        TEXT        NOT NULL UNIQUE,
    -- What is printed on the card, if anything, so a scan finds it too.
    barcode     TEXT UNIQUE,
    -- Whose it is, when it is a named member card rather than a bearer card.
    member_id   TEXT,
    issued_on   DATE        NOT NULL DEFAULT CURRENT_DATE,
    expires_on  DATE,
    initial_idr NUMERIC(15, 2) NOT NULL CHECK (initial_idr >= 0),
    -- A cached balance, the same shape as a stock level: the transactions are
    -- the truth and a test recomputes one from the other.
    balance_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (balance_idr >= 0),
    status      TEXT        NOT NULL DEFAULT 'ACTIVE'
                CHECK (status IN ('ACTIVE', 'FROZEN', 'EXPIRED', 'CANCELLED')),
    issued_by   TEXT,
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX gift_cards_member_idx ON pos.gift_cards (member_id);
CREATE INDEX gift_cards_status_idx ON pos.gift_cards (status);

-- Every movement on a card, forever. Same rule as the credit ledger and the
-- stock ledger: a balance nobody can explain is a balance nobody can defend.
CREATE TABLE pos.gift_card_entries (
    id           TEXT PRIMARY KEY,
    card_id      TEXT        NOT NULL REFERENCES pos.gift_cards (id) ON DELETE CASCADE,
    kind         TEXT        NOT NULL
                 CHECK (kind IN ('ISSUE', 'TOP_UP', 'SPEND', 'REFUND', 'ADJUSTMENT', 'EXPIRY')),
    -- Signed: negative took money off the card.
    amount_idr   NUMERIC(15, 2) NOT NULL CHECK (amount_idr <> 0),
    balance_before NUMERIC(15, 2) NOT NULL CHECK (balance_before >= 0),
    balance_after  NUMERIC(15, 2) NOT NULL CHECK (balance_after >= 0),
    order_id     TEXT REFERENCES pos.orders (id),
    reason       TEXT,
    actor_id     TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The arithmetic holds on the row, so no bug in the application can write
    -- a movement that does not add up.
    CONSTRAINT gift_card_entries_arithmetic
        CHECK (balance_after = balance_before + amount_idr)
);

CREATE INDEX gift_card_entries_card_idx ON pos.gift_card_entries (card_id, created_at DESC);
CREATE INDEX gift_card_entries_order_idx ON pos.gift_card_entries (order_id);

CREATE TRIGGER gift_card_entries_append_only
    BEFORE UPDATE OR DELETE ON pos.gift_card_entries
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- ── Payment methods ──────────────────────────────────────────────────────────

-- How the counter may be paid, as rows rather than as a list in the code.
--
-- A shop that signs up with a new QRIS provider on Tuesday should not need a
-- deployment on Wednesday. The behaviour that cannot be configured — whether a
-- method gives change, whether it settles against something else — stays in
-- the kind, because that is a rule and not a preference.
CREATE TABLE pos.payment_methods (
    id         TEXT PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    kind       TEXT        NOT NULL
               CHECK (kind IN ('CASH', 'CARD', 'QR', 'TRANSFER', 'MEMBER_CREDIT', 'GIFT_CARD')),
    -- Only cash gives change; the rest are charged what they are charged.
    gives_change BOOLEAN   NOT NULL DEFAULT false,
    -- A card terminal's approval code, a transfer's reference.
    needs_reference BOOLEAN NOT NULL DEFAULT false,
    -- Counted in the drawer at cash-up.
    counts_in_drawer BOOLEAN NOT NULL DEFAULT false,
    sort_order INTEGER     NOT NULL DEFAULT 0,
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Change comes out of the drawer, so a method that gives it must be in it.
    CONSTRAINT payment_methods_change_is_cash CHECK (NOT gives_change OR counts_in_drawer)
);

INSERT INTO pos.payment_methods (id, code, name, kind, gives_change, needs_reference, counts_in_drawer, sort_order) VALUES
    ('pmm_cash',  'CASH',   'Cash',           'CASH',          true,  false, true,  1),
    ('pmm_qris',  'QRIS',   'QRIS',           'QR',            false, true,  false, 2),
    ('pmm_debit', 'DEBIT',  'Debit card',     'CARD',          false, true,  false, 3),
    ('pmm_credit','CREDIT', 'Credit card',    'CARD',          false, true,  false, 4),
    ('pmm_xfer',  'TRANSFER','Bank transfer', 'TRANSFER',      false, true,  false, 5),
    ('pmm_wallet','MEMBER_CREDIT','Member credit','MEMBER_CREDIT', false, false, false, 6),
    ('pmm_gift',  'GIFT_CARD','Gift card',    'GIFT_CARD',     false, true,  false, 7);

-- Payments now name a configured method. The old CHECK becomes a foreign key,
-- so a method can be added without a migration but a typo still cannot.
ALTER TABLE pos.payments DROP CONSTRAINT payments_method_check;
ALTER TABLE pos.payments
    ADD CONSTRAINT payments_method_fkey FOREIGN KEY (method) REFERENCES pos.payment_methods (code);
-- The gift card a tender was taken from, when it was one.
ALTER TABLE pos.payments ADD COLUMN gift_card_id TEXT REFERENCES pos.gift_cards (id);

-- ── The manager standing there ───────────────────────────────────────────────

-- A supervisor's PIN, for the override at the till.
--
-- A permission answers "may this person do it"; a PIN answers "is a manager
-- standing here right now". Voiding a paid sale is the second question, and
-- handing a cashier a manager's login so they can answer it is how a manager's
-- login ends up on a sticky note under the till.
ALTER TABLE identity.admin_users
    ADD COLUMN supervisor_pin_hash TEXT,
    ADD COLUMN supervisor_pin_set_at TIMESTAMPTZ;

-- Who authorised what, at the counter.
ALTER TABLE pos.orders
    ADD COLUMN authorised_by TEXT,
    ADD COLUMN authorised_by_name TEXT;

-- ── Receipts ─────────────────────────────────────────────────────────────────

-- What a printed receipt says, per branch.
CREATE TABLE pos.receipt_settings (
    branch_id   TEXT PRIMARY KEY,
    header      TEXT        NOT NULL DEFAULT '',
    footer      TEXT        NOT NULL DEFAULT '',
    -- Printed above the header: the legal name and tax number a receipt needs.
    business_name TEXT      NOT NULL DEFAULT '',
    address     TEXT        NOT NULL DEFAULT '',
    phone       TEXT,
    tax_number  TEXT,
    -- 58mm and 80mm are the two thermal widths that exist in practice.
    paper_width INTEGER     NOT NULL DEFAULT 58 CHECK (paper_width IN (58, 80)),
    show_logo   BOOLEAN     NOT NULL DEFAULT true,
    show_cashier BOOLEAN    NOT NULL DEFAULT true,
    -- Print automatically on completion, or only when somebody asks.
    auto_print  BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A queue a local printer agent drains.
--
-- The server cannot reach a thermal printer on a shop's LAN, so it writes the
-- job and something on the counter picks it up. Keeping it as a row rather
-- than a fire-and-forget call is what makes "it never printed" answerable.
CREATE TABLE pos.print_jobs (
    id         TEXT PRIMARY KEY,
    branch_id  TEXT        NOT NULL,
    kind       TEXT        NOT NULL
               CHECK (kind IN ('RECEIPT', 'SHIFT_REPORT', 'ORDER_COPY')),
    order_id   TEXT REFERENCES pos.orders (id),
    shift_id   TEXT REFERENCES pos.shifts (id),
    -- The rendered document, so a reprint is the same paper as the original
    -- even after the product has been renamed.
    payload    TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'QUEUED'
               CHECK (status IN ('QUEUED', 'PRINTING', 'PRINTED', 'FAILED', 'CANCELLED')),
    attempts   INTEGER     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error      TEXT,
    claimed_at TIMESTAMPTZ,
    printed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The agent's own query: my branch, oldest first.
CREATE INDEX print_jobs_queue_idx ON pos.print_jobs (branch_id, created_at)
    WHERE status = 'QUEUED';
CREATE INDEX print_jobs_order_idx ON pos.print_jobs (order_id);

-- Sending a receipt to a phone. A job as well, for the same reason: a message
-- that was never sent should be visible as one.
CREATE TABLE pos.receipt_sends (
    id         TEXT PRIMARY KEY,
    order_id   TEXT        NOT NULL REFERENCES pos.orders (id) ON DELETE CASCADE,
    channel    TEXT        NOT NULL DEFAULT 'WHATSAPP'
               CHECK (channel IN ('WHATSAPP', 'EMAIL', 'SMS')),
    destination TEXT       NOT NULL,
    body       TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'QUEUED'
               CHECK (status IN ('QUEUED', 'SENT', 'FAILED')),
    error      TEXT,
    sent_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX receipt_sends_order_idx ON pos.receipt_sends (order_id);
CREATE INDEX receipt_sends_queue_idx ON pos.receipt_sends (created_at) WHERE status = 'QUEUED';
