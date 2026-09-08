-- Access: the door. Short-lived QR credentials and the log of every scan.

CREATE SCHEMA IF NOT EXISTS access;

-- A QR token is a single-use credential, not a member id. The token itself is
-- the primary key: it is generated from a cryptographic source and never
-- reused.
CREATE TABLE access.qr_tokens (
    token       TEXT PRIMARY KEY,
    member_id   TEXT        NOT NULL,
    issued_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    -- Which gate burned it, for tracing a disputed scan.
    consumed_by_gate_id TEXT
);

CREATE INDEX qr_member_idx ON access.qr_tokens (member_id, issued_at DESC);
-- Expired tokens are swept in bulk; unconsumed ones are all the sweep needs.
CREATE INDEX qr_expiry_idx ON access.qr_tokens (expires_at) WHERE consumed_at IS NULL;

-- Every scan attempt is recorded, including refusals: this table is the record
-- of what happened at the door, not just of who got in.
CREATE TABLE access.access_logs (
    id           TEXT PRIMARY KEY,
    -- NULL when the presented token resolved to nobody.
    member_id    TEXT,
    gate_id      TEXT        NOT NULL,
    branch_id    TEXT        NOT NULL,
    result       TEXT        NOT NULL
                 CHECK (result IN ('REQUESTED', 'ALLOWED', 'DENIED', 'OFFLINE_ALLOWED', 'SYNCED', 'CONFLICT')),
    reason_code  TEXT CHECK (reason_code IN ('TOKEN_INVALID', 'TOKEN_EXPIRED', 'TOKEN_CONSUMED',
                                             'MEMBER_NOT_ACTIVE', 'ANTI_PASSBACK', 'NO_BOOKING',
                                             'INSUFFICIENT_CREDITS')),
    -- Signed credit change this scan caused: 0 for refusals and re-entries.
    credit_delta INTEGER     NOT NULL DEFAULT 0,
    mode         TEXT        NOT NULL DEFAULT 'ONLINE' CHECK (mode IN ('ONLINE', 'OFFLINE')),
    booking_id   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Anti-passback asks for this member's last allowed entry at this branch, and
-- it asks on every single scan.
CREATE INDEX access_logs_member_branch_idx ON access.access_logs (member_id, branch_id, created_at DESC)
    WHERE result IN ('ALLOWED', 'OFFLINE_ALLOWED', 'SYNCED');
CREATE INDEX access_logs_recent_idx ON access.access_logs (created_at DESC);
CREATE INDEX access_logs_gate_idx ON access.access_logs (gate_id, created_at DESC);
CREATE INDEX access_logs_branch_idx ON access.access_logs (branch_id, created_at DESC);
CREATE INDEX access_logs_result_idx ON access.access_logs (result, created_at DESC);
-- Offline scans waiting to be reconciled.
CREATE INDEX access_logs_conflict_idx ON access.access_logs (created_at DESC)
    WHERE result = 'CONFLICT';

CREATE TRIGGER access_logs_append_only
    BEFORE DELETE ON access.access_logs
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();
