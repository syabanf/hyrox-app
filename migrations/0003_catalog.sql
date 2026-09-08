-- Catalog: the things the studio configures once and then operates against.
-- Organization, branches, gates, coaches, class templates, credit packages,
-- the exercise library, and the business rules themselves.

CREATE SCHEMA IF NOT EXISTS catalog;

CREATE TABLE catalog.organizations (
    id         TEXT PRIMARY KEY,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE catalog.branches (
    id              TEXT PRIMARY KEY,
    organization_id TEXT        NOT NULL REFERENCES catalog.organizations (id),
    name            TEXT        NOT NULL,
    address         TEXT        NOT NULL,
    timezone        TEXT        NOT NULL DEFAULT 'Asia/Jakarta',
    operating_hours TEXT        NOT NULL DEFAULT '06:00 - 22:00',
    status          TEXT        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    manager_name    TEXT,
    -- A partial BusinessRules; NULL means the branch follows org defaults.
    rules_override  JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX branches_org_idx ON catalog.branches (organization_id);

CREATE TABLE catalog.gates (
    id         TEXT PRIMARY KEY,
    branch_id  TEXT        NOT NULL REFERENCES catalog.branches (id),
    name       TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'ONLINE' CHECK (status IN ('ONLINE', 'OFFLINE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX gates_branch_idx ON catalog.gates (branch_id);

CREATE TABLE catalog.coaches (
    id             TEXT PRIMARY KEY,
    name           TEXT        NOT NULL,
    bio            TEXT        NOT NULL DEFAULT '',
    specialization TEXT        NOT NULL DEFAULT '',
    branch_id      TEXT        NOT NULL REFERENCES catalog.branches (id),
    status         TEXT        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX coaches_branch_idx ON catalog.coaches (branch_id);

CREATE TABLE catalog.class_types (
    id                   TEXT PRIMARY KEY,
    name                 TEXT        NOT NULL,
    description          TEXT        NOT NULL DEFAULT '',
    default_duration_min INTEGER     NOT NULL CHECK (default_duration_min > 0),
    default_credit_cost  INTEGER     NOT NULL CHECK (default_credit_cost > 0),
    default_capacity     INTEGER     NOT NULL CHECK (default_capacity > 0),
    active               BOOLEAN     NOT NULL DEFAULT true,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE catalog.credit_packages (
    id                        TEXT PRIMARY KEY,
    name                      TEXT        NOT NULL,
    credits                   INTEGER     NOT NULL CHECK (credits > 0),
    price_idr                 BIGINT      NOT NULL CHECK (price_idr >= 0),
    validity_days             INTEGER     NOT NULL CHECK (validity_days > 0),
    -- NULL means the package is sold at every branch.
    branch_id                 TEXT REFERENCES catalog.branches (id),
    purchase_limit_per_member INTEGER CHECK (purchase_limit_per_member > 0),
    -- NULL means the credits book any class; a list restricts coverage.
    applicable_class_type_ids JSONB,
    -- Sold packages are ARCHIVED, never deleted, so old payments still resolve.
    status                    TEXT        NOT NULL DEFAULT 'ACTIVE'
                              CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX credit_packages_status_idx ON catalog.credit_packages (status);

-- Business rules: exactly one row, guarded by the singleton check. Branch
-- deviations live in branches.rules_override.
CREATE TABLE catalog.business_rules (
    id                            BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
    default_credit_expiry_days    INTEGER NOT NULL DEFAULT 60  CHECK (default_credit_expiry_days > 0),
    cancellation_deadline_hours   INTEGER NOT NULL DEFAULT 4   CHECK (cancellation_deadline_hours >= 0),
    late_cancellation_policy      TEXT    NOT NULL DEFAULT 'FORFEIT' CHECK (late_cancellation_policy IN ('FORFEIT', 'FREE')),
    no_show_policy                TEXT    NOT NULL DEFAULT 'FORFEIT' CHECK (no_show_policy IN ('FORFEIT', 'FREE')),
    re_entry_grace_minutes        INTEGER NOT NULL DEFAULT 15  CHECK (re_entry_grace_minutes >= 0),
    anti_passback_minutes         INTEGER NOT NULL DEFAULT 60  CHECK (anti_passback_minutes >= 0),
    qr_ttl_seconds                INTEGER NOT NULL DEFAULT 45  CHECK (qr_ttl_seconds >= 10),
    waitlist_auto_promote         BOOLEAN NOT NULL DEFAULT true,
    low_balance_threshold         INTEGER NOT NULL DEFAULT 3   CHECK (low_balance_threshold >= 0),
    expiry_reminder_days          INTEGER NOT NULL DEFAULT 7   CHECK (expiry_reminder_days >= 1),
    booking_opens_days_before     INTEGER NOT NULL DEFAULT 7   CHECK (booking_opens_days_before >= 0),
    booking_closes_minutes_before INTEGER NOT NULL DEFAULT 0   CHECK (booking_closes_minutes_before >= 0),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO catalog.business_rules (id) VALUES (true) ON CONFLICT DO NOTHING;

-- The exercise library behind the member Guides tab and the workout generator.
CREATE TABLE catalog.exercises (
    id                  TEXT PRIMARY KEY,
    name                TEXT        NOT NULL,
    category            TEXT        NOT NULL
                        CHECK (category IN ('ERG', 'SLED', 'JUMP', 'CARRY', 'LUNGE', 'THROW', 'RUN', 'CONDITIONING')),
    equipment           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    -- 1..8 when this exercise is one of the race stations, NULL otherwise.
    hyrox_station_order INTEGER CHECK (hyrox_station_order BETWEEN 1 AND 8),
    difficulty          INTEGER     NOT NULL DEFAULT 1 CHECK (difficulty BETWEEN 1 AND 3),
    default_spec        JSONB       NOT NULL DEFAULT '{"distanceM":null,"reps":null}'::jsonb,
    video_url           TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX exercises_category_idx ON catalog.exercises (category);
CREATE UNIQUE INDEX exercises_station_idx ON catalog.exercises (hyrox_station_order)
    WHERE hyrox_station_order IS NOT NULL;

-- Which exercise can stand in for which, when equipment is unavailable.
CREATE TABLE catalog.substitution_rules (
    original_exercise_id    TEXT NOT NULL REFERENCES catalog.exercises (id) ON DELETE CASCADE,
    alternative_exercise_id TEXT NOT NULL REFERENCES catalog.exercises (id) ON DELETE CASCADE,
    similarity              NUMERIC(3, 2) NOT NULL CHECK (similarity BETWEEN 0 AND 1),
    conversion_note         TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (original_exercise_id, alternative_exercise_id)
);
