-- FMCG: the same goods bought by the carton and sold by the piece.
--
-- This is the shape a fast-moving consumer goods counter actually has, and it
-- replaces the food-service assumptions the POS was ported with. A restaurant
-- sells a dish it made; an FMCG counter sells a packaged unit somebody else
-- packed, and the whole difference falls out of one number: how many sellable
-- units are inside the thing that arrives on the pallet.
--
-- Stock is always counted in the item's base unit. A pack is a way of handing
-- that unit over — a six-pack, a carton of twenty-four — with a factor saying
-- how many base units it contains and, because this is retail, its own
-- barcode. Buying is in packs, selling is in packs, and the ledger only ever
-- sees base units. Getting that wrong is the classic FMCG data bug: an order
-- for 10 cartons receipted as 10 pieces, and a stock report nobody trusts
-- again.

-- ── Units ────────────────────────────────────────────────────────────────────

-- The unit master. Small because a unit is a word: the arithmetic lives on the
-- pack, where it belongs, and not here where it would be true of every item.
CREATE TABLE inventory.units (
    id         TEXT PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    -- COUNT units are whole things (piece, carton). MEASURE units are
    -- continuous (kilogram, litre) and are what makes qty NUMERIC(15,3)
    -- rather than an integer.
    kind       TEXT        NOT NULL DEFAULT 'COUNT'
               CHECK (kind IN ('COUNT', 'MEASURE')),
    active     BOOLEAN     NOT NULL DEFAULT true,
    sort_order INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO inventory.units (id, code, name, kind, sort_order) VALUES
    ('unt_pcs',  'PCS',  'Piece',     'COUNT',   1),
    ('unt_pair', 'PAIR', 'Pair',      'COUNT',   2),
    ('unt_pack', 'PACK', 'Pack',      'COUNT',   3),
    ('unt_box',  'BOX',  'Box',       'COUNT',   4),
    ('unt_ctn',  'CTN',  'Carton',    'COUNT',   5),
    ('unt_dus',  'DUS',  'Dus',       'COUNT',   6),
    ('unt_tub',  'TUB',  'Tub',       'COUNT',   7),
    ('unt_kg',   'KG',   'Kilogram',  'MEASURE', 8),
    ('unt_g',    'G',    'Gram',      'MEASURE', 9),
    ('unt_l',    'L',    'Litre',     'MEASURE', 10),
    ('unt_ml',   'ML',   'Millilitre','MEASURE', 11);

-- ── Packs ────────────────────────────────────────────────────────────────────

-- How one item may be handed over. Exactly one pack per item is the base, with
-- a factor of 1, and every other pack is defined as a multiple of it.
--
-- The barcode lives here rather than on the item because that is the point: a
-- single and a carton of the same drink carry different barcodes, and scanning
-- one at the till has to mean twenty-four and not one.
CREATE TABLE inventory.item_packs (
    id         TEXT PRIMARY KEY,
    item_id    TEXT        NOT NULL REFERENCES inventory.items (id) ON DELETE CASCADE,
    unit_code  TEXT        NOT NULL REFERENCES inventory.units (code),
    -- Base units inside one of these. 24 for a carton of 24.
    factor     NUMERIC(15, 3) NOT NULL CHECK (factor > 0),
    -- Its own barcode, unique across the catalogue so a scan resolves to one
    -- pack of one item and never to two.
    barcode    TEXT UNIQUE,
    is_base    BOOLEAN     NOT NULL DEFAULT false,
    -- What a purchase order and a sale reach for first.
    purchase_default BOOLEAN NOT NULL DEFAULT false,
    sale_default     BOOLEAN NOT NULL DEFAULT false,
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (item_id, unit_code),
    -- The base unit is the unit the ledger counts in, so its factor is not
    -- negotiable.
    CONSTRAINT item_packs_base_is_one CHECK (NOT is_base OR factor = 1)
);

-- One base per item, one default each way. Two bases is a conversion table
-- that contradicts itself.
CREATE UNIQUE INDEX item_packs_one_base_idx
    ON inventory.item_packs (item_id) WHERE is_base;
CREATE UNIQUE INDEX item_packs_one_purchase_default_idx
    ON inventory.item_packs (item_id) WHERE purchase_default;
CREATE UNIQUE INDEX item_packs_one_sale_default_idx
    ON inventory.item_packs (item_id) WHERE sale_default;
CREATE INDEX item_packs_item_idx ON inventory.item_packs (item_id);
CREATE INDEX item_packs_barcode_idx ON inventory.item_packs (barcode) WHERE barcode IS NOT NULL;

-- Every existing item gets its own unit as its base pack, so nothing that
-- already works starts needing a pack it does not have.
INSERT INTO inventory.item_packs (id, item_id, unit_code, factor, barcode, is_base,
                                  purchase_default, sale_default)
SELECT 'ipk_base_' || i.id, i.id, i.unit, 1, i.barcode, true, true, true
FROM inventory.items i
WHERE EXISTS (SELECT 1 FROM inventory.units u WHERE u.code = i.unit);

-- The item's own unit is now the name of its base pack. Keeping the column is
-- deliberate: a stock report should not have to join to say "PCS".
COMMENT ON COLUMN inventory.items.unit IS
    'Base unit. The item_packs row with is_base carries the same code.';

-- ── Movements remember the pack ──────────────────────────────────────────────

-- The ledger stays in base units — that is what makes it addable — but it also
-- records what was physically handled, so "we received 10" can be read back as
-- ten cartons rather than as ten mystery units.
ALTER TABLE inventory.stock_movements
    ADD COLUMN pack_unit   TEXT,
    ADD COLUMN pack_qty    NUMERIC(15, 3),
    ADD COLUMN pack_factor NUMERIC(15, 3) CHECK (pack_factor IS NULL OR pack_factor > 0);

-- A pack quantity that does not multiply out to the base quantity is a lie
-- about the same movement, so the row refuses to hold both.
ALTER TABLE inventory.stock_movements
    ADD CONSTRAINT stock_movements_pack_arithmetic
    CHECK (
        pack_qty IS NULL OR pack_factor IS NULL
        OR round(abs(qty)::numeric, 3) = round((pack_qty * pack_factor)::numeric, 3)
    );

-- ── Buying by the carton ─────────────────────────────────────────────────────

-- Quantities on a purchase document are in the pack the buyer ordered, and the
-- base quantity is derived rather than typed. A generated column is the point:
-- there is no code path that can write a base quantity disagreeing with the
-- pack it came from.
ALTER TABLE purchasing.purchase_request_items
    ADD COLUMN pack_factor NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0),
    ADD COLUMN qty_base NUMERIC(15, 3) GENERATED ALWAYS AS (qty * pack_factor) STORED;

ALTER TABLE purchasing.purchase_order_items
    ADD COLUMN pack_factor NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0),
    ADD COLUMN qty_ordered_base NUMERIC(15, 3) GENERATED ALWAYS AS (qty_ordered * pack_factor) STORED,
    ADD COLUMN qty_received_base NUMERIC(15, 3) GENERATED ALWAYS AS (qty_received * pack_factor) STORED;

ALTER TABLE purchasing.goods_receipt_items
    ADD COLUMN unit TEXT NOT NULL DEFAULT 'PCS',
    ADD COLUMN pack_factor NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0),
    ADD COLUMN qty_accepted_base NUMERIC(15, 3) GENERATED ALWAYS AS (qty_accepted * pack_factor) STORED,
    ADD COLUMN qty_rejected_base NUMERIC(15, 3) GENERATED ALWAYS AS (qty_rejected * pack_factor) STORED;

ALTER TABLE purchasing.purchase_return_items
    ADD COLUMN unit TEXT NOT NULL DEFAULT 'PCS',
    ADD COLUMN pack_factor NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0),
    ADD COLUMN qty_base NUMERIC(15, 3) GENERATED ALWAYS AS (qty * pack_factor) STORED;

-- Supplier prices are quoted per pack too — a carton price, not a piece price.
ALTER TABLE purchasing.supplier_prices
    ADD COLUMN unit TEXT NOT NULL DEFAULT 'PCS',
    ADD COLUMN pack_factor NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0);

-- ── Selling by the piece ─────────────────────────────────────────────────────

-- A product is one pack of one item at one price. The same drink sold as a
-- single and as a six-pack is two products pointing at one item, which is why
-- the factor lives here and not on the item.
ALTER TABLE pos.products
    ADD COLUMN barcode     TEXT UNIQUE,
    ADD COLUMN pack_unit   TEXT NOT NULL DEFAULT 'PCS',
    ADD COLUMN pack_factor NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0);

CREATE INDEX products_barcode_idx ON pos.products (barcode) WHERE barcode IS NOT NULL;

-- A sold line records the pack it was sold in, for the same reason a movement
-- does: a receipt reading "2" has to still say two six-packs next year.
ALTER TABLE pos.order_items
    ADD COLUMN pack_unit   TEXT NOT NULL DEFAULT 'PCS',
    ADD COLUMN pack_factor NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (pack_factor > 0),
    ADD COLUMN qty_base    NUMERIC(15, 3) GENERATED ALWAYS AS (qty * pack_factor) STORED;

-- ── Channels, not table service ──────────────────────────────────────────────

-- The service charge goes. It is a food-service construct: a shop charges for
-- goods and the tax on them, and a percentage added for being served is not a
-- thing an FMCG counter does.
ALTER TABLE pos.orders DROP COLUMN service_charge_idr;

-- Order type stops describing where the customer sits and starts describing
-- what they are buying as, which is the distinction that actually changes the
-- price.
ALTER TABLE pos.orders DROP CONSTRAINT orders_order_type_check;
UPDATE pos.orders SET order_type = 'RETAIL' WHERE order_type IN ('COUNTER', 'TAKEAWAY', 'DINE_IN');
ALTER TABLE pos.orders ALTER COLUMN order_type SET DEFAULT 'RETAIL';
ALTER TABLE pos.orders ADD CONSTRAINT orders_order_type_check
    CHECK (order_type IN ('RETAIL', 'WHOLESALE', 'STAFF'));

-- A price break: buy ten or more and it is cheaper, and a reseller pays a
-- different price again. Nothing else in the system knows about channels, so
-- this table is the whole of what "wholesale" means.
CREATE TABLE pos.product_prices (
    id         TEXT PRIMARY KEY,
    product_id TEXT        NOT NULL REFERENCES pos.products (id) ON DELETE CASCADE,
    channel    TEXT        NOT NULL DEFAULT 'RETAIL'
               CHECK (channel IN ('RETAIL', 'WHOLESALE', 'STAFF')),
    -- The quantity at which this price starts applying, in the product's own
    -- pack. A row with 1 is the plain price for that channel.
    min_qty    NUMERIC(15, 3) NOT NULL DEFAULT 1 CHECK (min_qty > 0),
    price_idr  NUMERIC(15, 2) NOT NULL CHECK (price_idr >= 0),
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- One price per break per channel. Two rows at the same break is an
    -- argument the till cannot settle.
    UNIQUE (product_id, channel, min_qty)
);

CREATE INDEX product_prices_lookup_idx ON pos.product_prices (product_id, channel, min_qty DESC);
