-- Purchasing: how stock gets onto the shelves in the first place.
--
-- The spine is request → order → receipt, and each step is a different pair of
-- hands on purpose. Raising a purchase, approving it and signing for what
-- arrives are three separate grants, because one person doing all three is how
-- invoices get paid for goods that never came.
--
-- Receiving is the one place this schema reaches outside itself: a goods
-- receipt line posts a movement into inventory. That happens through the
-- inventory service, in the same transaction, never by writing its tables.

CREATE SCHEMA IF NOT EXISTS purchasing;

CREATE TABLE purchasing.suppliers (
    id            TEXT PRIMARY KEY,
    code          TEXT        NOT NULL UNIQUE,
    name          TEXT        NOT NULL,
    contact_name  TEXT,
    contact_phone TEXT,
    email         TEXT,
    address       TEXT,
    city          TEXT,
    -- Indonesian tax number, quoted on every invoice.
    tax_number    TEXT,
    -- How long after delivery the invoice falls due.
    payment_terms TEXT        NOT NULL DEFAULT 'NET30'
                  CHECK (payment_terms IN ('COD', 'NET7', 'NET14', 'NET30', 'NET45', 'NET60')),
    bank_name     TEXT,
    bank_account  TEXT,
    bank_holder   TEXT,
    category      TEXT,
    -- PROBATION is a supplier being trialled; BLOCKED is one nobody may order
    -- from again, which is different from merely inactive.
    status        TEXT        NOT NULL DEFAULT 'ACTIVE'
                  CHECK (status IN ('DRAFT', 'ACTIVE', 'PROBATION', 'INACTIVE', 'BLOCKED')),
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX suppliers_status_idx ON purchasing.suppliers (status);
CREATE INDEX suppliers_name_idx ON purchasing.suppliers (lower(name));

-- What one supplier charges for one item, so an order can be priced without
-- somebody remembering.
CREATE TABLE purchasing.supplier_prices (
    id            TEXT PRIMARY KEY,
    supplier_id   TEXT        NOT NULL REFERENCES purchasing.suppliers (id) ON DELETE CASCADE,
    -- Plain id: items live in the inventory schema.
    item_id       TEXT        NOT NULL,
    unit_price_idr NUMERIC(15, 2) NOT NULL CHECK (unit_price_idr >= 0),
    min_order_qty NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (min_order_qty >= 0),
    lead_time_days INTEGER    NOT NULL DEFAULT 0 CHECK (lead_time_days >= 0),
    effective_from DATE       NOT NULL DEFAULT CURRENT_DATE,
    active        BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT supplier_prices_unique UNIQUE (supplier_id, item_id, effective_from)
);

CREATE INDEX supplier_prices_item_idx ON purchasing.supplier_prices (item_id, active);

-- A purchase request is somebody asking. It carries no supplier and no price
-- anybody has committed to — that is what an order is for.
CREATE TABLE purchasing.purchase_requests (
    id           TEXT PRIMARY KEY,
    pr_number    TEXT        NOT NULL UNIQUE,
    branch_id    TEXT        NOT NULL,
    -- The employee who asked, and the department it is charged to.
    requester_id TEXT,
    requester_name TEXT      NOT NULL,
    department_id TEXT,
    status       TEXT        NOT NULL DEFAULT 'DRAFT'
                 CHECK (status IN ('DRAFT', 'PENDING_HEAD', 'PENDING_FINANCE',
                                   'PENDING_DIRECTOR', 'APPROVED', 'REJECTED', 'CONVERTED')),
    priority     TEXT        NOT NULL DEFAULT 'NORMAL'
                 CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'URGENT')),
    total_idr    NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (total_idr >= 0),
    required_on  DATE,
    note         TEXT,
    -- Each signature is stored on its own, so who signed what is never a
    -- matter of reading the status backwards.
    approved_by_head      TEXT,
    approved_at_head      TIMESTAMPTZ,
    approved_by_finance   TEXT,
    approved_at_finance   TIMESTAMPTZ,
    approved_by_director  TEXT,
    approved_at_director  TIMESTAMPTZ,
    rejected_by      TEXT,
    rejected_at      TIMESTAMPTZ,
    rejection_reason TEXT,
    converted_po_id  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT purchase_requests_rejected_has_reason
        CHECK (status <> 'REJECTED' OR rejection_reason IS NOT NULL)
);

CREATE INDEX purchase_requests_status_idx ON purchasing.purchase_requests (status, created_at DESC);
CREATE INDEX purchase_requests_branch_idx ON purchasing.purchase_requests (branch_id, created_at DESC);

CREATE TABLE purchasing.purchase_request_items (
    id            TEXT PRIMARY KEY,
    request_id    TEXT        NOT NULL REFERENCES purchasing.purchase_requests (id) ON DELETE CASCADE,
    item_id       TEXT,
    -- Free text so somebody can ask for a thing that is not in the catalogue
    -- yet, which is most of why purchase requests exist.
    description   TEXT        NOT NULL,
    qty           NUMERIC(15, 3) NOT NULL CHECK (qty > 0),
    unit          TEXT        NOT NULL DEFAULT 'PCS',
    estimated_price_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (estimated_price_idr >= 0),
    total_idr     NUMERIC(15, 2) GENERATED ALWAYS AS (qty * estimated_price_idr) STORED,
    note          TEXT
);

CREATE INDEX purchase_request_items_request_idx ON purchasing.purchase_request_items (request_id);

-- A purchase order is a commitment to a supplier at a price.
CREATE TABLE purchasing.purchase_orders (
    id          TEXT PRIMARY KEY,
    po_number   TEXT        NOT NULL UNIQUE,
    supplier_id TEXT        NOT NULL REFERENCES purchasing.suppliers (id),
    branch_id   TEXT        NOT NULL,
    request_id  TEXT REFERENCES purchasing.purchase_requests (id),
    status      TEXT        NOT NULL DEFAULT 'DRAFT'
                CHECK (status IN ('DRAFT', 'APPROVED', 'SENT', 'PARTIALLY_RECEIVED',
                                  'RECEIVED', 'CANCELLED')),
    ordered_on  DATE        NOT NULL DEFAULT CURRENT_DATE,
    expected_on DATE,
    subtotal_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (subtotal_idr >= 0),
    discount_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (discount_idr >= 0),
    -- Indonesian VAT. A rate rather than a constant because it has moved twice
    -- in living memory and will move again.
    tax_percent NUMERIC(5, 2) NOT NULL DEFAULT 11 CHECK (tax_percent >= 0 AND tax_percent <= 100),
    tax_idr     NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (tax_idr >= 0),
    total_idr   NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (total_idr >= 0),
    terms       TEXT,
    ship_to     TEXT,
    note        TEXT,
    approved_by TEXT,
    approved_at TIMESTAMPTZ,
    sent_at     TIMESTAMPTZ,
    cancelled_by TEXT,
    cancelled_at TIMESTAMPTZ,
    cancellation_reason TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT purchase_orders_cancelled_has_reason
        CHECK (status <> 'CANCELLED' OR cancellation_reason IS NOT NULL)
);

CREATE INDEX purchase_orders_status_idx ON purchasing.purchase_orders (status, ordered_on DESC);
CREATE INDEX purchase_orders_supplier_idx ON purchasing.purchase_orders (supplier_id, ordered_on DESC);
CREATE INDEX purchase_orders_branch_idx ON purchasing.purchase_orders (branch_id, ordered_on DESC);

CREATE TABLE purchasing.purchase_order_items (
    id            TEXT PRIMARY KEY,
    order_id      TEXT        NOT NULL REFERENCES purchasing.purchase_orders (id) ON DELETE CASCADE,
    item_id       TEXT        NOT NULL,
    description   TEXT        NOT NULL,
    qty_ordered   NUMERIC(15, 3) NOT NULL CHECK (qty_ordered > 0),
    qty_received  NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_received >= 0),
    unit          TEXT        NOT NULL DEFAULT 'PCS',
    unit_price_idr NUMERIC(15, 2) NOT NULL CHECK (unit_price_idr >= 0),
    discount_idr  NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (discount_idr >= 0),
    subtotal_idr  NUMERIC(15, 2) GENERATED ALWAYS AS
                  ((qty_ordered * unit_price_idr) - discount_idr) STORED,
    note          TEXT,
    -- More cannot arrive than was ordered. Over-delivery is a conversation
    -- with the supplier, not a quantity the system invents.
    CONSTRAINT purchase_order_items_not_over_received CHECK (qty_received <= qty_ordered)
);

CREATE INDEX purchase_order_items_order_idx ON purchasing.purchase_order_items (order_id);
CREATE INDEX purchase_order_items_item_idx ON purchasing.purchase_order_items (item_id);

-- A goods receipt is what actually turned up, which is frequently not what was
-- ordered. It is the document stock movements point back at.
CREATE TABLE purchasing.goods_receipts (
    id           TEXT PRIMARY KEY,
    grn_number   TEXT        NOT NULL UNIQUE,
    order_id     TEXT        NOT NULL REFERENCES purchasing.purchase_orders (id),
    supplier_id  TEXT        NOT NULL REFERENCES purchasing.suppliers (id),
    branch_id    TEXT        NOT NULL,
    received_on  DATE        NOT NULL DEFAULT CURRENT_DATE,
    received_by  TEXT,
    received_by_name TEXT,
    delivery_note_number TEXT,
    status       TEXT        NOT NULL DEFAULT 'DRAFT'
                 CHECK (status IN ('DRAFT', 'POSTED', 'CANCELLED')),
    note         TEXT,
    posted_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX goods_receipts_order_idx ON purchasing.goods_receipts (order_id);
CREATE INDEX goods_receipts_status_idx ON purchasing.goods_receipts (status, received_on DESC);

CREATE TABLE purchasing.goods_receipt_items (
    id            TEXT PRIMARY KEY,
    receipt_id    TEXT        NOT NULL REFERENCES purchasing.goods_receipts (id) ON DELETE CASCADE,
    order_item_id TEXT        NOT NULL REFERENCES purchasing.purchase_order_items (id),
    item_id       TEXT        NOT NULL,
    qty_accepted  NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_accepted >= 0),
    -- Delivered but failed inspection. It is recorded and never enters stock,
    -- which is the whole reason the two numbers are separate.
    qty_rejected  NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_rejected >= 0),
    qty_returned  NUMERIC(15, 3) NOT NULL DEFAULT 0 CHECK (qty_returned >= 0),
    unit_price_idr NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (unit_price_idr >= 0),
    qc_status     TEXT        NOT NULL DEFAULT 'ACCEPTED'
                  CHECK (qc_status IN ('ACCEPTED', 'PARTIALLY_REJECTED', 'REJECTED')),
    batch_number  TEXT,
    expires_on    DATE,
    note          TEXT,
    CONSTRAINT goods_receipt_items_something_arrived
        CHECK (qty_accepted > 0 OR qty_rejected > 0),
    CONSTRAINT goods_receipt_items_returns_within_accepted
        CHECK (qty_returned <= qty_accepted)
);

CREATE INDEX goods_receipt_items_receipt_idx ON purchasing.goods_receipt_items (receipt_id);
CREATE INDEX goods_receipt_items_order_item_idx ON purchasing.goods_receipt_items (order_item_id);

-- Sending goods back. Only against a receipt, and only up to what was accepted
-- on it — you cannot return what you never took in.
CREATE TABLE purchasing.purchase_returns (
    id            TEXT PRIMARY KEY,
    return_number TEXT        NOT NULL UNIQUE,
    receipt_id    TEXT        NOT NULL REFERENCES purchasing.goods_receipts (id),
    supplier_id   TEXT        NOT NULL REFERENCES purchasing.suppliers (id),
    branch_id     TEXT        NOT NULL,
    returned_on   DATE        NOT NULL DEFAULT CURRENT_DATE,
    reason_type   TEXT        NOT NULL
                  CHECK (reason_type IN ('DAMAGED', 'WRONG_ITEM', 'EXPIRED',
                                         'OVERSTOCK', 'SPEC_MISMATCH', 'OTHER')),
    reason_note   TEXT,
    status        TEXT        NOT NULL DEFAULT 'DRAFT'
                  CHECK (status IN ('DRAFT', 'POSTED', 'CANCELLED')),
    total_idr     NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (total_idr >= 0),
    posted_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX purchase_returns_receipt_idx ON purchasing.purchase_returns (receipt_id);
CREATE INDEX purchase_returns_status_idx ON purchasing.purchase_returns (status, returned_on DESC);

CREATE TABLE purchasing.purchase_return_items (
    id              TEXT PRIMARY KEY,
    return_id       TEXT        NOT NULL REFERENCES purchasing.purchase_returns (id) ON DELETE CASCADE,
    receipt_item_id TEXT        NOT NULL REFERENCES purchasing.goods_receipt_items (id),
    item_id         TEXT        NOT NULL,
    qty             NUMERIC(15, 3) NOT NULL CHECK (qty > 0),
    unit_price_idr  NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (unit_price_idr >= 0),
    note            TEXT
);

CREATE INDEX purchase_return_items_return_idx ON purchasing.purchase_return_items (return_id);
