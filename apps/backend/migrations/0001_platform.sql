-- Platform: cross-cutting tables that belong to no single bounded context.
--
-- Schema-per-module is the rule in this database. A module reads and writes
-- only its own schema; anything it needs from another module it asks for
-- through a Go interface, never a cross-schema join. That is what lets a
-- module move to its own service (and its own database) without a rewrite.

CREATE SCHEMA IF NOT EXISTS platform;

-- Audit trail. Append-only: an audit log that can be edited is not evidence.
CREATE TABLE platform.audit_events (
    id             TEXT PRIMARY KEY,
    entity_type    TEXT        NOT NULL,
    entity_id      TEXT        NOT NULL,
    action         TEXT        NOT NULL,
    previous_value TEXT,
    new_value      TEXT,
    actor_id       TEXT        NOT NULL,
    actor_name     TEXT        NOT NULL,
    reason         TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_entity_idx ON platform.audit_events (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_events_created_idx ON platform.audit_events (created_at DESC);
CREATE INDEX audit_events_actor_idx ON platform.audit_events (actor_id, created_at DESC);

-- Transactional outbox. Cross-module effects (notify a member, recount a
-- session) are written here inside the same transaction as the state change,
-- then dispatched separately. Once modules are separate services this is the
-- seam that keeps them eventually consistent without distributed transactions.
CREATE TABLE platform.outbox_messages (
    id            TEXT PRIMARY KEY,
    topic         TEXT        NOT NULL,
    payload       JSONB       NOT NULL,
    -- Deduplication key for consumers that must act at most once.
    idempotency_key TEXT,
    status        TEXT        NOT NULL DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING', 'PROCESSING', 'DONE', 'FAILED')),
    attempts      INTEGER     NOT NULL DEFAULT 0,
    last_error    TEXT,
    available_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at  TIMESTAMPTZ
);

-- The dispatcher polls this: pending messages whose time has come, oldest first.
CREATE INDEX outbox_pending_idx ON platform.outbox_messages (available_at)
    WHERE status IN ('PENDING', 'FAILED');
CREATE UNIQUE INDEX outbox_idempotency_idx ON platform.outbox_messages (topic, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Refuse UPDATE and DELETE on append-only tables. Correction happens by
-- writing a compensating row, never by rewriting history.
CREATE OR REPLACE FUNCTION platform.reject_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'table %.% is append-only; write a compensating row instead',
        TG_TABLE_SCHEMA, TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER audit_events_append_only
    BEFORE UPDATE OR DELETE ON platform.audit_events
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();
