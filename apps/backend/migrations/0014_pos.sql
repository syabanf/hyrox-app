-- The till: selling merchandise, drinks and supplements at the counter.
--
-- A sale is where three modules meet. It takes stock out of inventory, it
-- earns the member loyalty points, and it is the only place in the system
-- where somebody hands over cash. All three happen in one transaction, so an
-- order can never be paid for without the stock moving or the points landing.
--
-- Deliberately not ported from the system this comes from: the kitchen display,
-- table management and reservations. Those are a restaurant's problem, and a
-- studio counter that grows into a café can have them when it does.

CREATE SCHEMA IF NOT EXISTS pos;

CREATE TABLE pos.categories (
    id         TEXT PRIMARY KEY,
    name       TEXT        NOT NULL,
    sort_order INTEGER     NOT NULL DEFAULT 0,
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- What is for sale. A product is not an inventory item: the same shirt can be
-- sold at two prices, and a service like a towel hire has a price but no
-- stock. inventory_item_id is the link where one exists.
CREATE TABLE pos.products (
    id          TEXT PRIMARY KEY,
    sku         TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    category_id TEXT REFERENCES pos.categories (id),
    -- NULL means selling it moves no stock, which is correct for a service.
    inventory_item_id TEXT,
    price_idr   NUMERIC(15, 2) NOT NULL CHECK (price_idr >= 0),
    -- Frozen onto each sold line, so a later price change never rewrites what
    -- last month's margin was.
    cost_idr    NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (cost_idr >= 0),
    tax_percent NUMERIC(5, 2) NOT NULL DEFAULT 0
                CHECK (tax_percent >= 0 AND tax_percent <= 100),
    -- Extra points this product is worth, on top of whatever the spend earns.
    bonus_xp    INTEGER     NOT NULL DEFAULT 0 CHECK (bonus_xp >= 0),
    image_url   TEXT,
    active      BOOLEAN     NOT NULL DEFAULT true,
    -- Temporarily off the menu without being retired.
    available   BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX products_category_idx ON pos.products (category_id);
CREATE INDEX products_item_idx ON pos.products (inventory_item_id);
CREATE INDEX products_active_idx ON pos.products (active, available);

-- A cashier's session at the till. Cash is counted in at the start and out at
-- the end, and the difference between what should be there and what is is the
-- number the whole table exists to produce.
CREATE TABLE pos.shifts (
    id            TEXT PRIMARY KEY,
    shift_number  TEXT        NOT NULL UNIQUE,
    cashier_id    TEXT        NOT NULL,
    cashier_name  TEXT        NOT NULL,
    branch_id     TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'OPEN'
                  CHECK (status IN ('OPEN', 'CLOSED')),
    opened_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at     TIMESTAMPTZ,
    opening_cash_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (opening_cash_idr >= 0),
    closing_cash_idr NUMERIC(15, 2) CHECK (closing_cash_idr IS NULL OR closing_cash_idr >= 0),
    -- What the till should hold: opening plus cash taken, minus cash refunded.
    expected_cash_idr NUMERIC(15, 2) NOT NULL DEFAULT 0,
    variance_idr  NUMERIC(15, 2) GENERATED ALWAYS AS
                  (COALESCE(closing_cash_idr, 0) - expected_cash_idr) STORED,
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT shifts_closed_has_count
        CHECK (status <> 'CLOSED' OR (closed_at IS NOT NULL AND closing_cash_idr IS NOT NULL))
);

-- One open shift per cashier per branch. Two tills under one name is how cash
-- goes missing without anybody being accountable for it.
CREATE UNIQUE INDEX shifts_one_open_idx ON pos.shifts (cashier_id, branch_id)
    WHERE status = 'OPEN';
CREATE INDEX shifts_branch_idx ON pos.shifts (branch_id, opened_at DESC);

CREATE TABLE pos.orders (
    id           TEXT PRIMARY KEY,
    order_number TEXT        NOT NULL UNIQUE,
    branch_id    TEXT        NOT NULL,
    shift_id     TEXT REFERENCES pos.shifts (id),
    cashier_id   TEXT        NOT NULL,
    cashier_name TEXT        NOT NULL,
    -- Naming a member is what earns them points and applies their discount.
    member_id    TEXT,
    order_type   TEXT        NOT NULL DEFAULT 'COUNTER'
                 CHECK (order_type IN ('COUNTER', 'TAKEAWAY', 'DINE_IN')),
    status       TEXT        NOT NULL DEFAULT 'OPEN'
                 CHECK (status IN ('OPEN', 'COMPLETED', 'CANCELLED', 'VOIDED')),
    payment_status TEXT      NOT NULL DEFAULT 'UNPAID'
                 CHECK (payment_status IN ('UNPAID', 'PARTIAL', 'PAID', 'REFUNDED')),
    subtotal_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (subtotal_idr >= 0),
    -- What the cashier took off by hand. Kept apart from the tier discount
    -- because the tier's is recomputed on every change, and adding the two
    -- into one column makes it compound every time a line is scanned.
    discount_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (discount_idr >= 0),
    tier_discount_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (tier_discount_idr >= 0),
    discount_reason TEXT,
    tax_idr      NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (tax_idr >= 0),
    service_charge_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (service_charge_idr >= 0),
    total_idr    NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (total_idr >= 0),
    paid_idr     NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (paid_idr >= 0),
    change_idr   NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (change_idr >= 0),
    -- Frozen at completion: what the goods cost, and what was made on them.
    cost_idr         NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (cost_idr >= 0),
    gross_profit_idr NUMERIC(15, 2) NOT NULL DEFAULT 0,
    xp_earned    INTEGER     NOT NULL DEFAULT 0 CHECK (xp_earned >= 0),
    note         TEXT,
    opened_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    voided_at    TIMESTAMPTZ,
    voided_by    TEXT,
    void_reason  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT orders_voided_has_reason
        CHECK (status <> 'VOIDED' OR void_reason IS NOT NULL)
);

CREATE INDEX orders_branch_idx ON pos.orders (branch_id, opened_at DESC);
CREATE INDEX orders_shift_idx ON pos.orders (shift_id);
CREATE INDEX orders_member_idx ON pos.orders (member_id, opened_at DESC);
CREATE INDEX orders_status_idx ON pos.orders (status, opened_at DESC);

-- A sold line. The product's name, SKU, price and cost are copied onto it,
-- because a receipt has to still be true a year later when the product has
-- been renamed and repriced.
CREATE TABLE pos.order_items (
    id            TEXT PRIMARY KEY,
    order_id      TEXT        NOT NULL REFERENCES pos.orders (id) ON DELETE CASCADE,
    product_id    TEXT        NOT NULL,
    product_name  TEXT        NOT NULL,
    product_sku   TEXT        NOT NULL,
    inventory_item_id TEXT,
    qty           NUMERIC(15, 3) NOT NULL CHECK (qty > 0),
    unit_price_idr NUMERIC(15, 2) NOT NULL CHECK (unit_price_idr >= 0),
    discount_idr  NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (discount_idr >= 0),
    tax_percent   NUMERIC(5, 2) NOT NULL DEFAULT 0,
    line_total_idr NUMERIC(15, 2) GENERATED ALWAYS AS
                  ((qty * unit_price_idr) - discount_idr) STORED,
    unit_cost_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (unit_cost_idr >= 0),
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX order_items_order_idx ON pos.order_items (order_id);
CREATE INDEX order_items_product_idx ON pos.order_items (product_id);

-- How an order was paid for. Several rows is a split payment, which is why
-- this is a table rather than two columns on the order.
CREATE TABLE pos.payments (
    id           TEXT PRIMARY KEY,
    order_id     TEXT        NOT NULL REFERENCES pos.orders (id) ON DELETE CASCADE,
    method       TEXT        NOT NULL
                 CHECK (method IN ('CASH', 'QRIS', 'DEBIT', 'CREDIT', 'TRANSFER', 'MEMBER_CREDIT')),
    amount_idr   NUMERIC(15, 2) NOT NULL CHECK (amount_idr > 0),
    -- Only cash gives change, and only on the tender that overpaid.
    change_idr   NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (change_idr >= 0),
    reference    TEXT,
    cashier_id   TEXT        NOT NULL,
    taken_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payments_change_is_cash_only
        CHECK (change_idr = 0 OR method = 'CASH')
);

CREATE INDEX payments_order_idx ON pos.payments (order_id);
CREATE INDEX payments_method_idx ON pos.payments (method, taken_at DESC);

-- Money is never quietly deleted: a mistaken tender is reversed with a
-- negative-facing refund row rather than by removing the original.
CREATE TRIGGER payments_append_only
    BEFORE DELETE ON pos.payments
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();
