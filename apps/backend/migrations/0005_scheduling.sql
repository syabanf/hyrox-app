-- Scheduling: class sessions and the bookings against them.

CREATE SCHEMA IF NOT EXISTS scheduling;

-- A session copies capacity and credit cost from its class type at creation.
-- Editing the template later must not change what an already-booked member
-- agreed to, which is why these are columns and not a join.
CREATE TABLE scheduling.class_sessions (
    id                TEXT PRIMARY KEY,
    class_type_id     TEXT        NOT NULL,
    branch_id         TEXT        NOT NULL,
    coach_id          TEXT        NOT NULL,
    starts_at         TIMESTAMPTZ NOT NULL,
    ends_at           TIMESTAMPTZ NOT NULL,
    capacity          INTEGER     NOT NULL CHECK (capacity > 0),
    credit_cost       INTEGER     NOT NULL CHECK (credit_cost > 0),
    booking_opens_at  TIMESTAMPTZ NOT NULL,
    booking_closes_at TIMESTAMPTZ NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'DRAFT'
                      CHECK (status IN ('DRAFT', 'PUBLISHED', 'FULL', 'COMPLETED', 'CANCELLED')),
    area              TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT sessions_time_ordered CHECK (ends_at > starts_at)
);

-- The schedule is always read as "this branch, this date range".
CREATE INDEX sessions_branch_time_idx ON scheduling.class_sessions (branch_id, starts_at);
CREATE INDEX sessions_time_idx ON scheduling.class_sessions (starts_at);
CREATE INDEX sessions_coach_idx ON scheduling.class_sessions (coach_id, starts_at);
CREATE INDEX sessions_class_type_idx ON scheduling.class_sessions (class_type_id);
CREATE INDEX sessions_status_idx ON scheduling.class_sessions (status, starts_at);

CREATE TABLE scheduling.bookings (
    id                   TEXT PRIMARY KEY,
    member_id            TEXT        NOT NULL,
    session_id           TEXT        NOT NULL REFERENCES scheduling.class_sessions (id),
    status               TEXT        NOT NULL
                         CHECK (status IN ('PENDING', 'CONFIRMED', 'WAITLIST', 'CANCELLED',
                                           'CHECKED_IN', 'COMPLETED', 'NO_SHOW')),
    waitlist_position    INTEGER CHECK (waitlist_position > 0),
    source               TEXT        NOT NULL DEFAULT 'MEMBER' CHECK (source IN ('MEMBER', 'ADMIN')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    cancelled_at         TIMESTAMPTZ,
    checked_in_at        TIMESTAMPTZ,
    -- Set when a freed slot was offered to this waitlisted member.
    promotion_offered_at TIMESTAMPTZ,
    -- A waitlisted booking is the only kind that carries a position.
    CONSTRAINT bookings_position_only_on_waitlist
        CHECK (waitlist_position IS NULL OR status = 'WAITLIST')
);

-- ALREADY_BOOKED enforced by the database, not just by the eligibility check:
-- two concurrent requests cannot both create an active booking for the same
-- member and session.
CREATE UNIQUE INDEX bookings_one_active_per_member_session
    ON scheduling.bookings (member_id, session_id)
    WHERE status IN ('PENDING', 'CONFIRMED', 'WAITLIST', 'CHECKED_IN');

CREATE INDEX bookings_session_idx ON scheduling.bookings (session_id, status);
CREATE INDEX bookings_member_idx ON scheduling.bookings (member_id, created_at DESC);
-- Picking the next member off a waitlist is ordered by position, then time.
CREATE INDEX bookings_waitlist_idx ON scheduling.bookings (session_id, waitlist_position, created_at)
    WHERE status = 'WAITLIST';
-- The gate looks up "does this member have a confirmed class around now".
CREATE INDEX bookings_checkin_lookup_idx ON scheduling.bookings (member_id, status)
    WHERE status IN ('CONFIRMED', 'CHECKED_IN');
