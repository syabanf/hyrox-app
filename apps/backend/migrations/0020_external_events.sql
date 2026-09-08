-- Things that happened somewhere else.
--
-- A studio does not live alone: a race timing system knows somebody finished,
-- a physio's booking system knows they turned up, a partner gym knows they
-- trained there. Each of those is a fact about a member that this system did
-- not observe, and the only honest way to hold it is as a record of what
-- somebody else said — with who said it, when, and the payload as it arrived.
--
-- Deliberately not folded into the outbox. The outbox carries things *this*
-- system decided; this table carries claims from outside, which have to be
-- matched to a member, may be about somebody we have never heard of, and are
-- redelivered by senders we do not control.

CREATE TABLE crm.integration_partners (
    id          TEXT PRIMARY KEY,
    code        TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    kind        TEXT        NOT NULL DEFAULT 'OTHER'
                CHECK (kind IN ('RACE', 'GYM', 'HEALTH', 'RETAIL', 'PAYMENT', 'OTHER')),
    -- The shared secret their calls are signed with. A partner without one
    -- cannot post anything, which is the safe state for a public endpoint.
    secret      TEXT,
    contact_name  TEXT,
    contact_email TEXT,
    -- Whether events from them may award points. A partner we merely record
    -- is a different thing from a partner we pay out on.
    awards_xp   BOOLEAN     NOT NULL DEFAULT false,
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX integration_partners_active_idx ON crm.integration_partners (active);

CREATE TABLE crm.external_events (
    id          TEXT PRIMARY KEY,
    partner_id  TEXT        NOT NULL REFERENCES crm.integration_partners (id),
    -- The sender's own id for this event. The whole of the idempotency story:
    -- a partner retrying is the same event, not a second one.
    external_id TEXT        NOT NULL,
    event_type  TEXT        NOT NULL,
    -- Who it is about, as the partner identified them, and who we matched
    -- that to. Both are kept: a match that turns out to be wrong should be
    -- correctable without losing what the partner actually sent.
    subject     TEXT        NOT NULL DEFAULT '',
    member_id   TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status      TEXT        NOT NULL DEFAULT 'RECEIVED'
                CHECK (status IN ('RECEIVED', 'MATCHED', 'PROCESSED', 'UNMATCHED', 'IGNORED', 'FAILED')),
    -- What we did about it, if anything.
    xp_awarded  INTEGER     NOT NULL DEFAULT 0 CHECK (xp_awarded >= 0),
    error       TEXT,
    processed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- One row per event per partner, forever. This index is the idempotency.
    UNIQUE (partner_id, external_id)
);

CREATE INDEX external_events_member_idx ON crm.external_events (member_id, occurred_at DESC);
CREATE INDEX external_events_partner_idx ON crm.external_events (partner_id, occurred_at DESC);
-- The queue somebody works through: events we could not match to anybody.
CREATE INDEX external_events_unmatched_idx ON crm.external_events (created_at)
    WHERE status = 'UNMATCHED';

-- What a partner sent is what they sent. A correction is a new event, and a
-- rematch changes only our side of it.
CREATE TRIGGER external_events_payload_immutable
    BEFORE DELETE ON crm.external_events
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();
