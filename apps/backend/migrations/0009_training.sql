-- Training: the athlete side of the member app. Activities with GPS tracks,
-- the social graph around them, segments, clubs, gear, generated HYROX
-- workouts, and race calendars.
--
-- The schema lands with the rest of the system so the module has somewhere to
-- write from day one; its HTTP surface is built out after the studio core.

CREATE SCHEMA IF NOT EXISTS training;

CREATE TABLE training.activities (
    id                   TEXT PRIMARY KEY,
    member_id            TEXT        NOT NULL,
    type                 TEXT        NOT NULL CHECK (type IN ('RUN', 'RIDE', 'WALK', 'WORKOUT')),
    title                TEXT        NOT NULL,
    description          TEXT        NOT NULL DEFAULT '',
    started_at           TIMESTAMPTZ NOT NULL,
    elapsed_sec          INTEGER     NOT NULL DEFAULT 0 CHECK (elapsed_sec >= 0),
    moving_sec           INTEGER     NOT NULL DEFAULT 0 CHECK (moving_sec >= 0),
    distance_m           NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (distance_m >= 0),
    avg_pace_sec_per_km  INTEGER,
    elevation_gain_m     NUMERIC(8, 2) NOT NULL DEFAULT 0,
    -- The raw track. Stored whole because it is always read whole, and never
    -- queried by its contents.
    points               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    photos               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    visibility           TEXT        NOT NULL DEFAULT 'EVERYONE'
                         CHECK (visibility IN ('EVERYONE', 'FOLLOWERS', 'PRIVATE')),
    gear_id              TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX activities_member_idx ON training.activities (member_id, started_at DESC);
-- The feed is "recent activities I am allowed to see".
CREATE INDEX activities_feed_idx ON training.activities (started_at DESC)
    WHERE visibility <> 'PRIVATE';
CREATE INDEX activities_type_idx ON training.activities (type, started_at DESC);

CREATE TABLE training.follows (
    follower_id TEXT        NOT NULL,
    followee_id TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CONSTRAINT follows_no_self CHECK (follower_id <> followee_id)
);

CREATE INDEX follows_followee_idx ON training.follows (followee_id);

CREATE TABLE training.kudos (
    activity_id TEXT        NOT NULL REFERENCES training.activities (id) ON DELETE CASCADE,
    member_id   TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (activity_id, member_id)
);

CREATE TABLE training.activity_comments (
    id          TEXT PRIMARY KEY,
    activity_id TEXT        NOT NULL REFERENCES training.activities (id) ON DELETE CASCADE,
    member_id   TEXT        NOT NULL,
    text        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX comments_activity_idx ON training.activity_comments (activity_id, created_at);

CREATE TABLE training.segments (
    id         TEXT PRIMARY KEY,
    name       TEXT        NOT NULL,
    type       TEXT        NOT NULL CHECK (type IN ('RUN', 'RIDE', 'WALK', 'WORKOUT')),
    distance_m NUMERIC(10, 2) NOT NULL CHECK (distance_m > 0),
    location   TEXT        NOT NULL DEFAULT '',
    path       JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE training.segment_efforts (
    id          TEXT PRIMARY KEY,
    segment_id  TEXT        NOT NULL REFERENCES training.segments (id) ON DELETE CASCADE,
    activity_id TEXT        NOT NULL REFERENCES training.activities (id) ON DELETE CASCADE,
    member_id   TEXT        NOT NULL,
    elapsed_sec INTEGER     NOT NULL CHECK (elapsed_sec > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Leaderboards read a segment ordered by time.
CREATE INDEX efforts_segment_idx ON training.segment_efforts (segment_id, elapsed_sec);
CREATE INDEX efforts_member_idx ON training.segment_efforts (member_id, segment_id, elapsed_sec);
CREATE UNIQUE INDEX efforts_activity_idx ON training.segment_efforts (activity_id, segment_id);

CREATE TABLE training.clubs (
    id          TEXT PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    location    TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE training.club_members (
    club_id   TEXT        NOT NULL REFERENCES training.clubs (id) ON DELETE CASCADE,
    member_id TEXT        NOT NULL,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (club_id, member_id)
);

CREATE INDEX club_members_member_idx ON training.club_members (member_id);

CREATE TABLE training.gear (
    id         TEXT PRIMARY KEY,
    member_id  TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    kind       TEXT        NOT NULL CHECK (kind IN ('SHOES', 'BIKE')),
    -- Accumulated from the activities logged against it.
    distance_m NUMERIC(12, 2) NOT NULL DEFAULT 0,
    retired    BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX gear_member_idx ON training.gear (member_id) WHERE NOT retired;

CREATE TABLE training.routes (
    id         TEXT PRIMARY KEY,
    member_id  TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    points     JSONB       NOT NULL,
    distance_m NUMERIC(10, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX routes_member_idx ON training.routes (member_id, created_at DESC);

CREATE TABLE training.athlete_settings (
    member_id         TEXT PRIMARY KEY,
    units             TEXT    NOT NULL DEFAULT 'METRIC' CHECK (units IN ('METRIC', 'IMPERIAL')),
    booking_reminders BOOLEAN NOT NULL DEFAULT true,
    weekly_goal_km    NUMERIC(6, 2) CHECK (weekly_goal_km > 0),
    language          TEXT    NOT NULL DEFAULT 'EN' CHECK (language IN ('EN', 'ID')),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A generated HYROX workout. Blocks are embedded: a workout is authored once
-- and then read as a unit.
CREATE TABLE training.workouts (
    id                    TEXT PRIMARY KEY,
    member_id             TEXT        NOT NULL,
    type                  TEXT        NOT NULL
                          CHECK (type IN ('FULL_SIMULATION', 'COVERAGE', 'QUICK', 'PRACTICE')),
    division              TEXT        NOT NULL
                          CHECK (division IN ('MEN_OPEN', 'MEN_PRO', 'WOMEN_OPEN', 'WOMEN_PRO')),
    blocks                JSONB       NOT NULL,
    excluded_exercise_ids JSONB       NOT NULL DEFAULT '[]'::jsonb,
    total_target_sec      INTEGER     NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX workouts_member_idx ON training.workouts (member_id, created_at DESC);

CREATE TABLE training.workout_sessions (
    id              TEXT PRIMARY KEY,
    workout_id      TEXT        NOT NULL REFERENCES training.workouts (id) ON DELETE CASCADE,
    member_id       TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'READY'
                    CHECK (status IN ('READY', 'STARTED', 'PAUSED', 'COMPLETED', 'PARTIAL')),
    current_block   INTEGER     NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    block_results   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    pause_count     INTEGER     NOT NULL DEFAULT 0,
    total_pause_sec INTEGER     NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX workout_sessions_member_idx ON training.workout_sessions (member_id, created_at DESC);

CREATE TABLE training.race_events (
    id               TEXT PRIMARY KEY,
    name             TEXT        NOT NULL,
    country          TEXT        NOT NULL,
    region           TEXT        NOT NULL CHECK (region IN ('ASIA', 'EUROPE', 'AMERICAS', 'OCEANIA')),
    city             TEXT        NOT NULL,
    venue            TEXT        NOT NULL DEFAULT '',
    starts_at        TIMESTAMPTZ NOT NULL,
    ends_at          TIMESTAMPTZ NOT NULL,
    registration_url TEXT        NOT NULL DEFAULT '',
    image_url        TEXT,
    status           TEXT        NOT NULL DEFAULT 'ANNOUNCED'
                     CHECK (status IN ('ANNOUNCED', 'REGISTRATION_OPEN', 'SOLD_OUT', 'UPCOMING',
                                       'ONGOING', 'COMPLETED', 'CANCELLED')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX race_events_time_idx ON training.race_events (starts_at);
CREATE INDEX race_events_region_idx ON training.race_events (region, starts_at);

CREATE TABLE training.user_races (
    id            TEXT PRIMARY KEY,
    member_id     TEXT        NOT NULL,
    race_event_id TEXT        NOT NULL REFERENCES training.race_events (id) ON DELETE CASCADE,
    division      TEXT        NOT NULL
                  CHECK (division IN ('MEN_OPEN', 'MEN_PRO', 'WOMEN_OPEN', 'WOMEN_PRO')),
    goal_sec      INTEGER CHECK (goal_sec > 0),
    status        TEXT        NOT NULL DEFAULT 'TRAINING'
                  CHECK (status IN ('TRAINING', 'RACED', 'CANCELLED')),
    result_sec    INTEGER CHECK (result_sec > 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A member joins a race once; cancelling frees the slot for a re-entry.
CREATE UNIQUE INDEX user_races_member_event_idx ON training.user_races (member_id, race_event_id)
    WHERE status <> 'CANCELLED';
CREATE INDEX user_races_member_idx ON training.user_races (member_id, created_at DESC);
CREATE INDEX user_races_event_idx ON training.user_races (race_event_id);
