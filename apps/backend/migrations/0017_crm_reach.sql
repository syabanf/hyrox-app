-- The rest of CRM: badges, consent, conversations and what members say back.
--
-- The loyalty half of CRM already exists — tiers, XP, rewards, redemptions.
-- What was missing is everything to do with *reaching* a member and hearing
-- from them: what they have earned that is not points, whether they want to
-- be contacted at all, the conversations they start, and the reviews they
-- leave.
--
-- Consent runs through all of it. An opt-out is not a preference to be
-- weighed against a campaign's audience: it is a hard exclusion, enforced
-- where the audience is built rather than remembered by whoever builds it.

-- ── Badges ───────────────────────────────────────────────────────────────────

-- What a member has done, as opposed to what they have spent.
--
-- Deliberately separate from tiers. A tier is a standing that moves both ways
-- with activity; a badge is a fact about the past that never becomes untrue —
-- somebody who ran a hundred sessions ran a hundred sessions, whatever they do
-- next.
CREATE TABLE crm.badges (
    id          TEXT PRIMARY KEY,
    code        TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    -- What earns it, and how much of it.
    metric      TEXT        NOT NULL
                CHECK (metric IN ('VISITS', 'BOOKINGS', 'SPEND_IDR', 'XP_EARNED',
                                  'STREAK_DAYS', 'REFERRALS', 'MANUAL')),
    threshold   NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (threshold >= 0),
    -- Points awarded the first time it is earned, on top of the achievement.
    bonus_xp    INTEGER     NOT NULL DEFAULT 0 CHECK (bonus_xp >= 0),
    icon        TEXT,
    sort_order  INTEGER     NOT NULL DEFAULT 0,
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX badges_metric_idx ON crm.badges (metric) WHERE active;

CREATE TABLE crm.member_badges (
    id         TEXT PRIMARY KEY,
    member_id  TEXT        NOT NULL,
    badge_id   TEXT        NOT NULL REFERENCES crm.badges (id) ON DELETE CASCADE,
    -- What the metric stood at when it was earned, frozen: the badge is a
    -- statement about that moment, and recomputing it later would let a
    -- correction elsewhere silently take an achievement away.
    earned_value NUMERIC(15, 2) NOT NULL DEFAULT 0,
    earned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    awarded_by TEXT,
    note       TEXT,
    -- Earned once. A badge that could be earned twice is a counter.
    UNIQUE (member_id, badge_id)
);

CREATE INDEX member_badges_member_idx ON crm.member_badges (member_id, earned_at DESC);

-- Achievements are not taken back. A badge awarded in error is deleted by
-- somebody who says so; it does not quietly disappear because a number moved.
CREATE TRIGGER member_badges_append_only
    BEFORE UPDATE ON crm.member_badges
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- ── Consent ──────────────────────────────────────────────────────────────────

-- Whether a member wants to hear from us, per channel.
--
-- A row means they have said something. No row means they have not, and the
-- default lives in the application rather than here, because "we never asked"
-- and "they said yes" are different facts and a boolean column cannot hold
-- both.
CREATE TABLE crm.contact_preferences (
    member_id  TEXT        NOT NULL,
    channel    TEXT        NOT NULL
               CHECK (channel IN ('PUSH', 'EMAIL', 'WHATSAPP', 'SMS')),
    opted_in   BOOLEAN     NOT NULL,
    -- Why, in their words or ours. An opt-out with a reason is a product
    -- signal; one without is just a smaller list.
    reason     TEXT,
    -- Marketing is refusable; a booking confirmation is not. This says which
    -- kind of message the preference governs.
    scope      TEXT        NOT NULL DEFAULT 'MARKETING'
               CHECK (scope IN ('MARKETING', 'ALL')),
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by TEXT,
    PRIMARY KEY (member_id, channel)
);

CREATE INDEX contact_preferences_opted_out_idx ON crm.contact_preferences (channel)
    WHERE NOT opted_in;

-- ── Campaign delivery ────────────────────────────────────────────────────────

-- One row per member per campaign: who it was meant for, what happened to it.
--
-- Without this a campaign can only report "sent 400", which answers none of
-- the questions worth asking — who was skipped, who opened it, whether the
-- deep link did anything.
CREATE TABLE engagement.campaign_recipients (
    id          TEXT PRIMARY KEY,
    campaign_id TEXT        NOT NULL REFERENCES engagement.campaigns (id) ON DELETE CASCADE,
    member_id   TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'PENDING'
                CHECK (status IN ('PENDING', 'SENT', 'SKIPPED', 'FAILED', 'OPENED', 'CLICKED')),
    -- Why it was skipped: an opt-out, no contact details, already sent today.
    skip_reason TEXT,
    notification_id TEXT,
    sent_at     TIMESTAMPTZ,
    opened_at   TIMESTAMPTZ,
    clicked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (campaign_id, member_id)
);

CREATE INDEX campaign_recipients_campaign_idx
    ON engagement.campaign_recipients (campaign_id, status);
CREATE INDEX campaign_recipients_member_idx
    ON engagement.campaign_recipients (member_id, created_at DESC);

-- A campaign is written once and sent many times, so the words live apart
-- from the send.
CREATE TABLE engagement.message_templates (
    id         TEXT PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    channel    TEXT        NOT NULL DEFAULT 'PUSH'
               CHECK (channel IN ('PUSH', 'EMAIL', 'WHATSAPP', 'SMS', 'INBOX')),
    subject    TEXT,
    body       TEXT        NOT NULL,
    -- Placeholders the body uses, so a preview can be rendered and a missing
    -- one caught before four hundred people read "Hi {{name}}".
    variables  TEXT[]      NOT NULL DEFAULT '{}',
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Conversations ────────────────────────────────────────────────────────────

-- The inbox: a member says something, somebody answers.
CREATE TABLE crm.conversations (
    id          TEXT PRIMARY KEY,
    member_id   TEXT,
    -- A conversation can start before we know who it is — a walk-in question,
    -- a message from a number nobody recognises — so the name stands alone.
    contact_name  TEXT      NOT NULL DEFAULT '',
    contact_handle TEXT,
    channel     TEXT        NOT NULL DEFAULT 'INBOX'
                CHECK (channel IN ('INBOX', 'WHATSAPP', 'EMAIL', 'INSTAGRAM', 'WALK_IN')),
    subject     TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'OPEN'
                CHECK (status IN ('OPEN', 'PENDING', 'RESOLVED', 'CLOSED')),
    priority    TEXT        NOT NULL DEFAULT 'NORMAL'
                CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'URGENT')),
    assigned_to TEXT,
    assigned_name TEXT,
    branch_id   TEXT,
    tags        TEXT[]      NOT NULL DEFAULT '{}',
    -- When the member last said something, and when we last answered. The
    -- gap between them is the only service metric that means anything.
    last_member_at TIMESTAMPTZ,
    last_staff_at  TIMESTAMPTZ,
    first_response_seconds INTEGER,
    resolved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX conversations_status_idx ON crm.conversations (status, updated_at DESC);
CREATE INDEX conversations_member_idx ON crm.conversations (member_id, created_at DESC);
CREATE INDEX conversations_assigned_idx ON crm.conversations (assigned_to, status);
-- The queue: unanswered, oldest first.
CREATE INDEX conversations_waiting_idx ON crm.conversations (last_member_at)
    WHERE status IN ('OPEN', 'PENDING');

CREATE TABLE crm.conversation_messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT    NOT NULL REFERENCES crm.conversations (id) ON DELETE CASCADE,
    direction       TEXT    NOT NULL CHECK (direction IN ('INBOUND', 'OUTBOUND')),
    body            TEXT    NOT NULL,
    -- Who wrote it. Null author on an inbound message is the member.
    author_id       TEXT,
    author_name     TEXT,
    template_id     TEXT REFERENCES engagement.message_templates (id),
    -- The provider's own id, so a webhook redelivering the same message does
    -- not post it twice.
    external_id     TEXT,
    attachments     JSONB   NOT NULL DEFAULT '[]'::jsonb,
    internal        BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX conversation_messages_thread_idx
    ON crm.conversation_messages (conversation_id, created_at);
-- One row per provider message. This is the whole of the idempotency story
-- for an inbound webhook: the second delivery hits this index and stops.
CREATE UNIQUE INDEX conversation_messages_external_idx
    ON crm.conversation_messages (external_id) WHERE external_id IS NOT NULL;

-- What was said is what was said. A message is corrected by sending another.
CREATE TRIGGER conversation_messages_append_only
    BEFORE UPDATE OR DELETE ON crm.conversation_messages
    FOR EACH ROW EXECUTE FUNCTION platform.reject_mutation();

-- ── Reviews ──────────────────────────────────────────────────────────────────

-- What members say about a class, a coach, or the place.
CREATE TABLE crm.reviews (
    id         TEXT PRIMARY KEY,
    member_id  TEXT        NOT NULL,
    -- What is being reviewed. Plain ids: a session lives in scheduling and a
    -- coach in catalog, and neither should own a foreign key from here.
    subject_type TEXT      NOT NULL
                 CHECK (subject_type IN ('SESSION', 'COACH', 'BRANCH', 'PRODUCT')),
    subject_id TEXT        NOT NULL,
    rating     INTEGER     NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment    TEXT,
    -- A review of a class the member did not attend is not a review. Set when
    -- the visit behind it can be found.
    verified   BOOLEAN     NOT NULL DEFAULT false,
    visit_id   TEXT,
    status     TEXT        NOT NULL DEFAULT 'PUBLISHED'
               CHECK (status IN ('PUBLISHED', 'HIDDEN', 'FLAGGED')),
    -- The reply, kept on the review so a member sees it in one place.
    reply      TEXT,
    replied_by TEXT,
    replied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- One review per member per thing. A second opinion is an edit.
    UNIQUE (member_id, subject_type, subject_id)
);

CREATE INDEX reviews_subject_idx ON crm.reviews (subject_type, subject_id, created_at DESC);
CREATE INDEX reviews_rating_idx ON crm.reviews (rating) WHERE status = 'PUBLISHED';
CREATE INDEX reviews_member_idx ON crm.reviews (member_id, created_at DESC);
