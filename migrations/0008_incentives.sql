-- Incentives: coach payroll in IDR. Entirely separate from the member credit
-- ledger — coaches are paid money, members spend credits.

CREATE SCHEMA IF NOT EXISTS incentives;

CREATE TABLE incentives.schemes (
    id                           TEXT PRIMARY KEY,
    -- NULL is the organization default; a coach id overrides it for that coach.
    coach_id                     TEXT,
    session_fee_idr              BIGINT  NOT NULL DEFAULT 0 CHECK (session_fee_idr >= 0),
    per_attendee_idr             BIGINT  NOT NULL DEFAULT 0 CHECK (per_attendee_idr >= 0),
    full_class_bonus_idr         BIGINT  NOT NULL DEFAULT 0 CHECK (full_class_bonus_idr >= 0),
    full_class_threshold_percent INTEGER NOT NULL DEFAULT 80
                                 CHECK (full_class_threshold_percent BETWEEN 0 AND 100),
    no_show_penalty_idr          BIGINT  NOT NULL DEFAULT 0 CHECK (no_show_penalty_idr >= 0),
    active                       BOOLEAN NOT NULL DEFAULT true,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One scheme per coach, and exactly one organization default.
CREATE UNIQUE INDEX schemes_coach_idx ON incentives.schemes (coach_id) WHERE coach_id IS NOT NULL;
CREATE UNIQUE INDEX schemes_default_idx ON incentives.schemes ((coach_id IS NULL)) WHERE coach_id IS NULL;

-- A payout freezes one coach's statement for one calendar month.
--
-- The frozen copy is the point: a later attendance correction must never
-- silently change what was already approved or paid. Fixing a mistake means
-- voiding the payout and creating a new one.
CREATE TABLE incentives.payouts (
    id                TEXT PRIMARY KEY,
    coach_id          TEXT        NOT NULL,
    branch_id         TEXT        NOT NULL,
    period_start      TIMESTAMPTZ NOT NULL,
    -- Exclusive month boundary.
    period_end        TIMESTAMPTZ NOT NULL,
    statement         JSONB       NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'DRAFT'
                      CHECK (status IN ('DRAFT', 'APPROVED', 'PAID', 'VOID')),
    created_by        TEXT        NOT NULL,
    approved_by       TEXT,
    approved_at       TIMESTAMPTZ,
    paid_at           TIMESTAMPTZ,
    payment_reference TEXT,
    note              TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payouts_period_ordered CHECK (period_end > period_start),
    -- Paying requires a reference; voiding requires a reason.
    CONSTRAINT payouts_paid_has_reference
        CHECK (status <> 'PAID' OR payment_reference IS NOT NULL),
    CONSTRAINT payouts_void_has_note
        CHECK (status <> 'VOID' OR note IS NOT NULL)
);

-- A coach has at most one live payout per period; voided ones do not block a
-- replacement.
CREATE UNIQUE INDEX payouts_period_idx ON incentives.payouts (coach_id, period_start)
    WHERE status <> 'VOID';
CREATE INDEX payouts_status_idx ON incentives.payouts (status, created_at DESC);
CREATE INDEX payouts_branch_idx ON incentives.payouts (branch_id, period_start DESC);
CREATE INDEX payouts_coach_idx ON incentives.payouts (coach_id, period_start DESC);
