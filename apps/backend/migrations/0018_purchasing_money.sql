-- Buying, the rest of it: what arrived, what we owe, and who says yes.
--
-- Three gaps, all of them about not collapsing two different facts into one
-- row:
--
--   * A delivery is not a goods receipt. The truck arriving is one event and
--     accepting what was on it is another, often on a different day and always
--     by a different person. Folding them together means a short delivery
--     looks identical to a rejection, and nobody can tell whether the supplier
--     shipped nine or shipped ten and one was broken.
--
--   * A purchase order is not a payment. What was committed, what has arrived
--     and what has actually left the bank are three numbers, and a supplier
--     conversation needs all three.
--
--   * A return is not a decision. Sending goods back costs the relationship
--     something, and it should be somebody's signature rather than whoever was
--     at the receiving bay.

-- ── The truck arrived ────────────────────────────────────────────────────────

CREATE TABLE purchasing.deliveries (
    id              TEXT PRIMARY KEY,
    delivery_number TEXT        NOT NULL UNIQUE,
    order_id        TEXT        NOT NULL REFERENCES purchasing.purchase_orders (id),
    supplier_id     TEXT        NOT NULL REFERENCES purchasing.suppliers (id),
    branch_id       TEXT        NOT NULL,
    -- What the supplier's own paperwork says. The number to quote back at
    -- them when the count disagrees.
    delivery_note_number TEXT,
    driver_name     TEXT,
    vehicle         TEXT,
    arrived_on      DATE        NOT NULL DEFAULT CURRENT_DATE,
    arrived_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    received_by     TEXT,
    received_by_name TEXT,
    status          TEXT        NOT NULL DEFAULT 'ARRIVED'
                    CHECK (status IN ('ARRIVED', 'INSPECTED', 'CANCELLED')),
    note            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX deliveries_order_idx ON purchasing.deliveries (order_id);
CREATE INDEX deliveries_status_idx ON purchasing.deliveries (status, arrived_on DESC);

-- What came off the truck, counted at the bay. No inspection yet, so there is
-- one quantity here and not two: this is what was handed over.
CREATE TABLE purchasing.delivery_items (
    id            TEXT PRIMARY KEY,
    delivery_id   TEXT        NOT NULL REFERENCES purchasing.deliveries (id) ON DELETE CASCADE,
    order_item_id TEXT        NOT NULL REFERENCES purchasing.purchase_order_items (id),
    item_id       TEXT        NOT NULL,
    qty_delivered NUMERIC(15, 3) NOT NULL CHECK (qty_delivered > 0),
    unit          TEXT        NOT NULL DEFAULT 'PCS',
    pack_factor   NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0),
    qty_delivered_base NUMERIC(15, 3) GENERATED ALWAYS AS (qty_delivered * pack_factor) STORED,
    batch_number  TEXT,
    expires_on    DATE,
    note          TEXT,
    UNIQUE (delivery_id, order_item_id)
);

CREATE INDEX delivery_items_delivery_idx ON purchasing.delivery_items (delivery_id);

-- A receipt may now say which delivery it inspected. Nullable, because a
-- receipt raised straight off an order — the small delivery somebody checked
-- as it came in — is still a legitimate way to work.
ALTER TABLE purchasing.goods_receipts
    ADD COLUMN delivery_id TEXT REFERENCES purchasing.deliveries (id);

CREATE INDEX goods_receipts_delivery_idx ON purchasing.goods_receipts (delivery_id);

-- ── What we owe ──────────────────────────────────────────────────────────────

-- When an order is due to be paid, and in what pieces.
--
-- A term is generated from the supplier's default (NET30 and the like) and can
-- then be edited, because "half on order, half on delivery" is a real thing a
-- supplier asks for and no code on the supplier record can express it.
CREATE TABLE purchasing.payment_terms (
    id         TEXT PRIMARY KEY,
    order_id   TEXT        NOT NULL REFERENCES purchasing.purchase_orders (id) ON DELETE CASCADE,
    sequence   INTEGER     NOT NULL DEFAULT 1,
    label      TEXT        NOT NULL DEFAULT '',
    due_on     DATE        NOT NULL,
    -- The share this instalment is, and the money it works out to. Both are
    -- stored: the percentage is what was agreed and the amount is what will
    -- actually be transferred, and rounding means the two do not always
    -- reproduce each other.
    percent    NUMERIC(6, 3) CHECK (percent IS NULL OR (percent > 0 AND percent <= 100)),
    amount_idr NUMERIC(15, 2) NOT NULL CHECK (amount_idr >= 0),
    status     TEXT        NOT NULL DEFAULT 'PENDING'
               CHECK (status IN ('PENDING', 'PARTIAL', 'PAID', 'CANCELLED')),
    paid_idr   NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (paid_idr >= 0),
    note       TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, sequence)
);

CREATE INDEX payment_terms_order_idx ON purchasing.payment_terms (order_id, sequence);
-- The payables run: what is due, soonest first.
CREATE INDEX payment_terms_due_idx ON purchasing.payment_terms (due_on)
    WHERE status IN ('PENDING', 'PARTIAL');

-- Money that actually left.
CREATE TABLE purchasing.vendor_payments (
    id             TEXT PRIMARY KEY,
    payment_number TEXT        NOT NULL UNIQUE,
    supplier_id    TEXT        NOT NULL REFERENCES purchasing.suppliers (id),
    -- A payment usually settles one order, but a supplier being paid off in a
    -- lump does not, so this is nullable.
    order_id       TEXT REFERENCES purchasing.purchase_orders (id),
    term_id        TEXT REFERENCES purchasing.payment_terms (id),
    paid_on        DATE        NOT NULL DEFAULT CURRENT_DATE,
    amount_idr     NUMERIC(15, 2) NOT NULL CHECK (amount_idr > 0),
    -- What of that amount was settled with a credit note rather than cash.
    credit_idr     NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (credit_idr >= 0),
    method         TEXT        NOT NULL DEFAULT 'TRANSFER'
                   CHECK (method IN ('TRANSFER', 'CASH', 'CHEQUE', 'CARD', 'CREDIT_NOTE')),
    reference      TEXT,
    status         TEXT        NOT NULL DEFAULT 'DRAFT'
                   CHECK (status IN ('DRAFT', 'POSTED', 'VOIDED')),
    note           TEXT,
    posted_at      TIMESTAMPTZ,
    posted_by      TEXT,
    voided_at      TIMESTAMPTZ,
    void_reason    TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT vendor_payments_credit_within_amount CHECK (credit_idr <= amount_idr),
    CONSTRAINT vendor_payments_voided_has_reason
        CHECK (status <> 'VOIDED' OR void_reason IS NOT NULL)
);

CREATE INDEX vendor_payments_supplier_idx ON purchasing.vendor_payments (supplier_id, paid_on DESC);
CREATE INDEX vendor_payments_order_idx ON purchasing.vendor_payments (order_id);

-- What the supplier owes us back, usually because we sent something back.
--
-- A credit note is not a refund: the money does not move, the next invoice is
-- smaller. Keeping it as its own document is what lets somebody answer "why is
-- this payment 400.000 short" a month later.
CREATE TABLE purchasing.vendor_credits (
    id            TEXT PRIMARY KEY,
    credit_number TEXT        NOT NULL UNIQUE,
    supplier_id   TEXT        NOT NULL REFERENCES purchasing.suppliers (id),
    return_id     TEXT REFERENCES purchasing.purchase_returns (id),
    issued_on     DATE        NOT NULL DEFAULT CURRENT_DATE,
    amount_idr    NUMERIC(15, 2) NOT NULL CHECK (amount_idr > 0),
    applied_idr   NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (applied_idr >= 0),
    status        TEXT        NOT NULL DEFAULT 'OPEN'
                  CHECK (status IN ('OPEN', 'PARTIALLY_APPLIED', 'APPLIED', 'CANCELLED')),
    reason        TEXT,
    expires_on    DATE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- A credit cannot be spent twice.
    CONSTRAINT vendor_credits_applied_within_amount CHECK (applied_idr <= amount_idr)
);

CREATE INDEX vendor_credits_supplier_idx ON purchasing.vendor_credits (supplier_id, issued_on DESC);
CREATE INDEX vendor_credits_open_idx ON purchasing.vendor_credits (supplier_id)
    WHERE status IN ('OPEN', 'PARTIALLY_APPLIED');

-- Which credit paid for which payment. Append-only: a credit that was spent
-- was spent, and unspending it is a cancellation somebody signs for.
CREATE TABLE purchasing.credit_applications (
    id         TEXT PRIMARY KEY,
    credit_id  TEXT        NOT NULL REFERENCES purchasing.vendor_credits (id),
    payment_id TEXT        NOT NULL REFERENCES purchasing.vendor_payments (id) ON DELETE CASCADE,
    amount_idr NUMERIC(15, 2) NOT NULL CHECK (amount_idr > 0),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (credit_id, payment_id)
);

CREATE INDEX credit_applications_credit_idx ON purchasing.credit_applications (credit_id);

CREATE TRIGGER credit_applications_append_only
    BEFORE UPDATE ON purchasing.credit_applications
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- ── Somebody says yes ────────────────────────────────────────────────────────

-- Sending goods back costs the relationship something, so it stops being
-- whatever the receiving bay decided and becomes a signature.
ALTER TABLE purchasing.purchase_returns
    DROP CONSTRAINT purchase_returns_status_check;
ALTER TABLE purchasing.purchase_returns
    ADD CONSTRAINT purchase_returns_status_check
    CHECK (status IN ('DRAFT', 'PENDING_APPROVAL', 'APPROVED', 'REJECTED', 'POSTED', 'CANCELLED'));

ALTER TABLE purchasing.purchase_returns
    ADD COLUMN submitted_at TIMESTAMPTZ,
    ADD COLUMN approved_by  TEXT,
    ADD COLUMN approved_at  TIMESTAMPTZ,
    ADD COLUMN rejected_by  TEXT,
    ADD COLUMN rejected_at  TIMESTAMPTZ,
    ADD COLUMN decision_note TEXT,
    -- Set when the return produces a credit note, so the two documents point
    -- at each other rather than only one way.
    ADD COLUMN credit_id TEXT REFERENCES purchasing.vendor_credits (id);

ALTER TABLE purchasing.purchase_returns
    ADD CONSTRAINT purchase_returns_rejected_has_note
    CHECK (status <> 'REJECTED' OR decision_note IS NOT NULL);
