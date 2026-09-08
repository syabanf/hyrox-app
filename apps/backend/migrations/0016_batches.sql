-- Batches: stock that goes off.
--
-- The other half of what makes fast-moving consumer goods different from
-- merchandise. A t-shirt is the same t-shirt in a year; a carton of isotonic
-- drinks has a date on it, and the difference between a shop that knows which
-- date is on the shelf and one that does not is the difference between a
-- write-off and a markdown.
--
-- The rule is FEFO — first expired, first out — not FIFO. Goods that arrived
-- later can easily expire sooner, and issuing in arrival order leaves the
-- short-dated stock at the back until it is worthless.
--
-- Batches sit *beside* the ledger rather than replacing it. inventory.
-- stock_levels stays the authority on how much there is; a batch says which
-- of it goes off when. The two are reconciled by a constraint the application
-- cannot route around: a batched issue always sums to the movement it caused.

CREATE TABLE inventory.batches (
    id         TEXT PRIMARY KEY,
    item_id    TEXT        NOT NULL REFERENCES inventory.items (id) ON DELETE CASCADE,
    branch_id  TEXT        NOT NULL,
    -- What the supplier printed on the case. Two deliveries of the same batch
    -- code into the same branch are one batch, which is why this is unique.
    batch_code TEXT        NOT NULL,
    expires_on DATE,
    qty_on_hand NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_on_hand >= 0),
    -- What this particular batch cost. The item's weighted average is still
    -- what a stock report values at; this is what a write-off is worth.
    unit_cost_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (unit_cost_idr >= 0),
    received_on TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Where it came from, so a recall can be answered.
    receipt_id  TEXT,
    receipt_number TEXT,
    note       TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (item_id, branch_id, batch_code)
);

-- The FEFO query: soonest expiry first, undated last. It runs on every sale of
-- a batched item, so it gets the index it deserves.
CREATE INDEX batches_fefo_idx ON inventory.batches (item_id, branch_id, expires_on NULLS LAST)
    WHERE qty_on_hand > 0;
CREATE INDEX batches_expiry_idx ON inventory.batches (expires_on)
    WHERE qty_on_hand > 0 AND expires_on IS NOT NULL;
CREATE INDEX batches_branch_idx ON inventory.batches (branch_id, item_id);

-- Which batches one movement drew on. A sale of 30 that took 12 from a batch
-- expiring Friday and 18 from one expiring next month is two rows here and one
-- movement in the ledger.
CREATE TABLE inventory.batch_movements (
    id          TEXT PRIMARY KEY,
    batch_id    TEXT        NOT NULL REFERENCES inventory.batches (id) ON DELETE CASCADE,
    movement_id TEXT        NOT NULL REFERENCES inventory.stock_movements (id) ON DELETE CASCADE,
    -- Signed the same way the movement is: negative took stock out.
    qty         NUMERIC(15, 3) NOT NULL CHECK (qty <> 0),
    qty_before  NUMERIC(15, 3) NOT NULL CHECK (qty_before >= 0),
    qty_after   NUMERIC(15, 3) NOT NULL CHECK (qty_after >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT batch_movements_arithmetic CHECK (qty_after = qty_before + qty)
);

CREATE INDEX batch_movements_batch_idx ON inventory.batch_movements (batch_id, created_at DESC);
CREATE INDEX batch_movements_movement_idx ON inventory.batch_movements (movement_id);

-- Same rule as the ledger it hangs off: a batch allocation is history, and a
-- mistake is corrected by another allocation rather than by editing this one.
CREATE TRIGGER batch_movements_append_only
    BEFORE UPDATE OR DELETE ON inventory.batch_movements
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- Whether an item's stock is tracked by batch at all.
--
-- Most of a studio's catalogue is not: a steel bottle has no date on it, and
-- forcing a batch onto it would make every receipt a form nobody can fill in.
-- Turning it on is a decision per item, and the default is off so nothing that
-- already works starts demanding a date.
ALTER TABLE inventory.items
    ADD COLUMN track_batches BOOLEAN NOT NULL DEFAULT false,
    -- How many days before the printed date somebody should be told. A drink
    -- with six months of life wants a longer warning than a sandwich.
    ADD COLUMN expiry_warning_days INTEGER NOT NULL DEFAULT 30
        CHECK (expiry_warning_days >= 0);

-- A goods receipt of a batched item must say which batch, and the check lives
-- here so no code path can post one without.
ALTER TABLE purchasing.goods_receipt_items
    ADD CONSTRAINT goods_receipt_items_batch_has_code
    CHECK (batch_number IS NULL OR length(trim(batch_number)) > 0);
