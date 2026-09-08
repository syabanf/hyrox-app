-- Engagement: reaching members. Campaigns, in-app notifications, and the
-- community challenges shown in the training tab.

CREATE SCHEMA IF NOT EXISTS engagement;

CREATE TABLE engagement.campaigns (
    id            TEXT PRIMARY KEY,
    name          TEXT        NOT NULL,
    segment       TEXT        NOT NULL
                  CHECK (segment IN ('ALL_ACTIVE', 'LOW_BALANCE', 'EXPIRING_CREDITS',
                                     'NEW_MEMBERS', 'NO_VISIT_14D', 'CUSTOM')),
    -- Extra predicates for a CUSTOM audience.
    custom_filter JSONB,
    message       TEXT        NOT NULL,
    deep_link     TEXT,
    image_url     TEXT,
    scheduled_at  TIMESTAMPTZ,
    status        TEXT        NOT NULL DEFAULT 'DRAFT'
                  CHECK (status IN ('DRAFT', 'SCHEDULED', 'PROCESSING', 'SENT', 'FAILED', 'CANCELLED')),
    -- Populated when the send finishes, so a partial send is visible.
    sent_count    INTEGER,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX campaigns_status_idx ON engagement.campaigns (status, created_at DESC);
-- The scheduler polls for campaigns whose time has arrived.
CREATE INDEX campaigns_due_idx ON engagement.campaigns (scheduled_at)
    WHERE status = 'SCHEDULED';

CREATE TABLE engagement.member_notifications (
    id         TEXT PRIMARY KEY,
    member_id  TEXT        NOT NULL,
    type       TEXT        NOT NULL
               CHECK (type IN ('BOOKING_CONFIRMED', 'BOOKING_REMINDER', 'WAITLIST_PROMOTED',
                               'LOW_BALANCE', 'CREDIT_EXPIRY', 'VISIT_LOGGED',
                               'SESSION_CHANGED', 'ANNOUNCEMENT')),
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    -- Set when this notification came from a campaign, for reporting reach.
    campaign_id TEXT REFERENCES engagement.campaigns (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at    TIMESTAMPTZ
);

CREATE INDEX notifications_member_idx ON engagement.member_notifications (member_id, created_at DESC);
-- The unread badge is read on every app open.
CREATE INDEX notifications_unread_idx ON engagement.member_notifications (member_id)
    WHERE read_at IS NULL;

-- Dedupe key for automatic notifications (booking reminders, expiry warnings)
-- so a member is never told the same thing twice.
CREATE TABLE engagement.notification_marks (
    mark_key   TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE engagement.challenges (
    id          TEXT PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    -- ANY counts every activity type toward the target.
    type        TEXT        NOT NULL DEFAULT 'ANY'
                CHECK (type IN ('ANY', 'RUN', 'RIDE', 'WALK', 'WORKOUT')),
    target_km   NUMERIC(8, 2) NOT NULL CHECK (target_km > 0),
    starts_at   TIMESTAMPTZ NOT NULL,
    ends_at     TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT challenges_window_ordered CHECK (ends_at > starts_at)
);

CREATE INDEX challenges_window_idx ON engagement.challenges (starts_at, ends_at);

CREATE TABLE engagement.challenge_joins (
    challenge_id TEXT        NOT NULL REFERENCES engagement.challenges (id) ON DELETE CASCADE,
    member_id    TEXT        NOT NULL,
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (challenge_id, member_id)
);

CREATE INDEX challenge_joins_member_idx ON engagement.challenge_joins (member_id);
