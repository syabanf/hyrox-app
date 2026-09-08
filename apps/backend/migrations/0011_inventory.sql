-- Inventory: what the studio has on its shelves, and every movement that put
-- it there or took it away.
--
-- The shape mirrors the credit wallet, because the problem is the same one:
-- a quantity that many things change and nobody may quietly rewrite. Movements
-- are the append-only truth; the level row is a locked cache of their sum, and
-- a test recomputes one from the other.
--
-- Unlike the ERP this is ported from, stock is held **per branch**. That system
-- had a single warehouse; a studio with Senopati and PIK has two, and asking
-- "do we have protein bars" without saying where is not a question.

CREATE SCHEMA IF NOT EXISTS inventory;

CREATE TABLE inventory.categories (
    id         TEXT PRIMARY KEY,
    name       TEXT        NOT NULL,
    code       TEXT        NOT NULL UNIQUE,
    active     BOOLEAN     NOT NULL DEFAULT true,
    sort_order INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- An item is anything the studio counts: merchandise it sells, supplements
-- behind the counter, cleaning supplies it only consumes.
CREATE TABLE inventory.items (
    id          TEXT PRIMARY KEY,
    sku         TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    category_id TEXT REFERENCES inventory.categories (id),
    -- The unit stock is counted in. Purchase orders and sales both speak it.
    unit        TEXT        NOT NULL DEFAULT 'PCS',
    kind        TEXT        NOT NULL DEFAULT 'RETAIL'
                CHECK (kind IN ('RETAIL', 'SUPPLY', 'RAW')),
    -- Weighted average, recalculated on every receipt. Not a purchase price:
    -- it is what the stock on hand actually cost.
    unit_cost_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (unit_cost_idr >= 0),
    -- Something bought but never counted (a service, a one-off) can live in
    -- the catalogue without a stock level.
    track_stock BOOLEAN     NOT NULL DEFAULT true,
    barcode     TEXT,
    image_url   TEXT,
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX items_category_idx ON inventory.items (category_id);
CREATE INDEX items_name_idx ON inventory.items (lower(name));
CREATE INDEX items_active_idx ON inventory.items (active);

-- One row per item per branch. This is the row a sale locks, which is what
-- stops two tills selling the same last unit.
CREATE TABLE inventory.stock_levels (
    item_id          TEXT        NOT NULL REFERENCES inventory.items (id) ON DELETE CASCADE,
    -- Plain id: branches live in the catalog schema and may one day live in
    -- another database.
    branch_id        TEXT        NOT NULL,
    qty_on_hand      NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_on_hand >= 0),
    -- Ordered from a supplier but not yet received. Reorder maths counts it,
    -- so a second purchase order is not raised for stock already coming.
    qty_on_order     NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_on_order >= 0),
    qty_minimum      NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_minimum >= 0),
    qty_maximum      NUMERIC(15, 3) CHECK (qty_maximum IS NULL OR qty_maximum >= qty_minimum),
    bin_location     TEXT,
    last_movement_at TIMESTAMPTZ,
    note             TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, branch_id)
);

-- The reorder screen is the whole point of qty_minimum, so it gets an index.
CREATE INDEX stock_levels_low_idx ON inventory.stock_levels (branch_id, item_id)
    WHERE qty_on_hand <= qty_minimum;
CREATE INDEX stock_levels_branch_idx ON inventory.stock_levels (branch_id);

-- Every change to a quantity, forever. qty_before and qty_after are stored so
-- a movement explains itself without replaying the whole table.
CREATE TABLE inventory.stock_movements (
    id            TEXT PRIMARY KEY,
    item_id       TEXT        NOT NULL REFERENCES inventory.items (id),
    branch_id     TEXT        NOT NULL,
    kind          TEXT        NOT NULL
                  CHECK (kind IN ('IN', 'OUT', 'ADJUSTMENT', 'TRANSFER_IN', 'TRANSFER_OUT', 'RETURN')),
    -- Signed: what was added to (or taken from) the level. Never zero, because
    -- a movement that changed nothing is not a movement.
    qty           NUMERIC(15, 3) NOT NULL CHECK (qty <> 0),
    qty_before    NUMERIC(15, 3) NOT NULL CHECK (qty_before >= 0),
    qty_after     NUMERIC(15, 3) NOT NULL CHECK (qty_after >= 0),
    unit_cost_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (unit_cost_idr >= 0),
    total_cost_idr NUMERIC(15, 2) NOT NULL DEFAULT 0,
    -- What caused it: a goods receipt, a sale, a stock take, a transfer.
    reference_type   TEXT,
    reference_id     TEXT,
    reference_number TEXT,
    reason        TEXT,
    note          TEXT,
    actor_id      TEXT,
    actor_name    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The arithmetic has to hold on the row itself, so a bug in the
    -- application cannot write a movement that does not add up.
    CONSTRAINT stock_movements_arithmetic CHECK (qty_after = qty_before + qty)
);

CREATE INDEX stock_movements_item_idx ON inventory.stock_movements (item_id, created_at DESC);
CREATE INDEX stock_movements_branch_idx ON inventory.stock_movements (branch_id, created_at DESC);
CREATE INDEX stock_movements_reference_idx ON inventory.stock_movements (reference_type, reference_id);
CREATE INDEX stock_movements_kind_idx ON inventory.stock_movements (kind, created_at DESC);

-- A miscount is corrected with an ADJUSTMENT that says so, never by editing
-- history. Same rule as the credit ledger, same trigger.
CREATE TRIGGER stock_movements_append_only
    BEFORE UPDATE OR DELETE ON inventory.stock_movements
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- Moving stock between branches is two movements that must agree. The transfer
-- row is what makes them one act.
CREATE TABLE inventory.stock_transfers (
    id              TEXT PRIMARY KEY,
    transfer_number TEXT        NOT NULL UNIQUE,
    item_id         TEXT        NOT NULL REFERENCES inventory.items (id),
    from_branch_id  TEXT        NOT NULL,
    to_branch_id    TEXT        NOT NULL,
    qty             NUMERIC(15, 3) NOT NULL CHECK (qty > 0),
    note            TEXT,
    actor_id        TEXT,
    actor_name      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT stock_transfers_distinct_branches CHECK (from_branch_id <> to_branch_id)
);

CREATE INDEX stock_transfers_item_idx ON inventory.stock_transfers (item_id, created_at DESC);

-- A stock take is a counted truth for one branch on one day: its lines produce
-- ADJUSTMENT movements when it is applied.
CREATE TABLE inventory.stock_takes (
    id           TEXT PRIMARY KEY,
    take_number  TEXT        NOT NULL UNIQUE,
    branch_id    TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'DRAFT'
                 CHECK (status IN ('DRAFT', 'APPLIED', 'CANCELLED')),
    counted_on   DATE        NOT NULL DEFAULT CURRENT_DATE,
    note         TEXT,
    applied_by   TEXT,
    applied_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT stock_takes_applied_has_actor
        CHECK (status <> 'APPLIED' OR (applied_by IS NOT NULL AND applied_at IS NOT NULL))
);

CREATE TABLE inventory.stock_take_lines (
    id           TEXT PRIMARY KEY,
    stock_take_id TEXT       NOT NULL REFERENCES inventory.stock_takes (id) ON DELETE CASCADE,
    item_id      TEXT        NOT NULL REFERENCES inventory.items (id),
    -- Frozen when the line is added, so the variance is against what the system
    -- believed at counting time rather than whatever it says now.
    qty_expected NUMERIC(15, 3) NOT NULL,
    qty_counted  NUMERIC(15, 3) NOT NULL CHECK (qty_counted >= 0),
    note         TEXT,
    CONSTRAINT stock_take_lines_unique UNIQUE (stock_take_id, item_id)
);

CREATE INDEX stock_take_lines_take_idx ON inventory.stock_take_lines (stock_take_id);
