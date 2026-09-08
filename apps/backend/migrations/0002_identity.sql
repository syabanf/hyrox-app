-- Identity: who is who. Members, staff accounts, and the OTP challenges that
-- get a member signed in.

CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE identity.members (
    id                  TEXT PRIMARY KEY,
    full_name           TEXT        NOT NULL,
    email               TEXT        NOT NULL,
    phone               TEXT        NOT NULL,
    date_of_birth       TIMESTAMPTZ,
    gender              TEXT CHECK (gender IN ('MALE', 'FEMALE', 'OTHER')),
    -- Embedded value object: never queried on its own, so it stays JSONB
    -- rather than becoming a table nobody joins to.
    emergency_contact   JSONB,
    -- Branch lives in the catalog module, so this is a plain id with no FK.
    preferred_branch_id TEXT,
    avatar_url          TEXT,
    status              TEXT        NOT NULL DEFAULT 'ACTIVE'
                        CHECK (status IN ('ACTIVE', 'SUSPENDED', 'INACTIVE', 'ARCHIVED')),
    -- The waiver version is recorded so a later revision does not silently
    -- appear to have been accepted.
    waiver_version      TEXT,
    waiver_accepted_at  TIMESTAMPTZ,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Email and phone identify a member at sign-in, so both must be unique.
-- Archived members keep their rows but release their identifiers, so a
-- returning member can register again with the same contact details.
CREATE UNIQUE INDEX members_email_idx ON identity.members (lower(email))
    WHERE status <> 'ARCHIVED';
CREATE UNIQUE INDEX members_phone_idx ON identity.members (phone)
    WHERE status <> 'ARCHIVED';
CREATE INDEX members_status_idx ON identity.members (status);
CREATE INDEX members_branch_idx ON identity.members (preferred_branch_id);
-- Admin member search hits full name constantly.
CREATE INDEX members_name_idx ON identity.members (lower(full_name));

CREATE TABLE identity.admin_users (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL,
    email     TEXT NOT NULL,
    role      TEXT NOT NULL
              CHECK (role IN ('SUPER_ADMIN', 'HQ_ADMIN', 'BRANCH_MANAGER', 'FRONT_DESK', 'COACH', 'FINANCE')),
    -- NULL means the user is not scoped to one branch (HQ level).
    branch_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX admin_users_email_idx ON identity.admin_users (lower(email));
CREATE INDEX admin_users_role_idx ON identity.admin_users (role);

-- One-time passcodes. The code is stored hashed: a leaked database must not
-- hand over live sign-in codes.
CREATE TABLE identity.otp_challenges (
    id          TEXT PRIMARY KEY,
    identifier  TEXT        NOT NULL,
    code_hash   TEXT        NOT NULL,
    member_id   TEXT,
    attempts    INTEGER     NOT NULL DEFAULT 0,
    consumed_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX otp_identifier_idx ON identity.otp_challenges (identifier, created_at DESC);
-- Expired challenges are swept periodically; this index makes that cheap.
CREATE INDEX otp_expiry_idx ON identity.otp_challenges (expires_at)
    WHERE consumed_at IS NULL;
