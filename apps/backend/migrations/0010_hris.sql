-- HRIS: the people who run the studio, as opposed to the members who train in
-- it. Employees, the org chart they sit in, the shifts they work, the
-- attendance that results, and the leave they take.
--
-- Ported from the arkiv-os ERP. The rules are kept faithful — lateness counted
-- from the shift start, collective leave deducting where a public holiday does
-- not — while the storage follows this codebase's conventions.

CREATE SCHEMA IF NOT EXISTS hris;

CREATE TABLE hris.departments (
    id                   TEXT PRIMARY KEY,
    name                 TEXT        NOT NULL,
    code                 TEXT        NOT NULL,
    description          TEXT        NOT NULL DEFAULT '',
    -- Departments nest, so "Operations > Coaching" needs no second table.
    parent_department_id TEXT REFERENCES hris.departments (id),
    cost_center          TEXT,
    active               BOOLEAN     NOT NULL DEFAULT true,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX departments_code_idx ON hris.departments (upper(code));
CREATE INDEX departments_parent_idx ON hris.departments (parent_department_id);

CREATE TABLE hris.positions (
    id         TEXT PRIMARY KEY,
    title      TEXT        NOT NULL,
    level      TEXT        NOT NULL DEFAULT 'Staff',
    active     BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Employment status is a table, not an enum: studios invent their own
-- (probation, permanent, contract, freelance coach) and must not need a
-- migration to add one.
CREATE TABLE hris.employment_statuses (
    id          TEXT PRIMARY KEY,
    code        TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX employment_statuses_code_idx ON hris.employment_statuses (upper(code));

CREATE TABLE hris.employees (
    id              TEXT PRIMARY KEY,
    full_name       TEXT        NOT NULL,
    -- The studio's own identifier, quoted on payslips and contracts.
    employee_number TEXT        NOT NULL,
    email           TEXT        NOT NULL,
    phone           TEXT        NOT NULL,
    birth_date      DATE,
    gender          TEXT CHECK (gender IN ('MALE', 'FEMALE', 'OTHER')),
    marital_status  TEXT CHECK (marital_status IN ('SINGLE', 'MARRIED', 'DIVORCED', 'WIDOWED')),
    address         TEXT,
    join_date       DATE        NOT NULL,
    end_date        DATE,
    employment_status_code TEXT NOT NULL DEFAULT 'PROBATION',
    active          BOOLEAN     NOT NULL DEFAULT true,
    department_id   TEXT REFERENCES hris.departments (id),
    position_id     TEXT REFERENCES hris.positions (id),
    -- Branch, coach and staff login live in other modules, so these are plain
    -- ids with no foreign key: identity and catalog may one day be elsewhere.
    branch_id       TEXT,
    coach_id        TEXT,
    admin_user_id   TEXT,
    reporting_to    TEXT REFERENCES hris.employees (id),
    bank_name       TEXT,
    bank_account    TEXT,
    emergency_contact_name     TEXT,
    emergency_contact_phone    TEXT,
    emergency_contact_relation TEXT,
    photo_url       TEXT,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT employees_employment_period CHECK (end_date IS NULL OR end_date >= join_date)
);

CREATE UNIQUE INDEX employees_number_idx ON hris.employees (upper(employee_number));
-- Email identifies a person while they are on the payroll. A leaver releases
-- it, so a returning employee can be re-hired with the same address.
CREATE UNIQUE INDEX employees_email_idx ON hris.employees (lower(email)) WHERE active;
CREATE INDEX employees_department_idx ON hris.employees (department_id);
CREATE INDEX employees_active_idx ON hris.employees (active);
CREATE INDEX employees_reporting_idx ON hris.employees (reporting_to);
-- One employee record per coach, so payroll cannot double-count a class.
CREATE UNIQUE INDEX employees_coach_idx ON hris.employees (coach_id) WHERE coach_id IS NOT NULL;
CREATE INDEX employees_name_idx ON hris.employees (lower(full_name));

CREATE TABLE hris.shifts (
    id                     TEXT PRIMARY KEY,
    name                   TEXT        NOT NULL,
    start_time             TIME        NOT NULL,
    end_time               TIME        NOT NULL,
    break_minutes          INTEGER     NOT NULL DEFAULT 60 CHECK (break_minutes >= 0),
    -- Grace before an arrival counts as late. The lateness itself is still
    -- measured from start_time; this only decides whether it is recorded.
    late_tolerance_minutes INTEGER     NOT NULL DEFAULT 10 CHECK (late_tolerance_minutes >= 0),
    -- Explicit rather than inferred from end < start, so a shift may legally
    -- run 22:00 to 06:00 or 09:00 to 08:00 the next day without ambiguity.
    is_overnight           BOOLEAN     NOT NULL DEFAULT false,
    active                 BOOLEAN     NOT NULL DEFAULT true,
    sort_order             INTEGER     NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A weekly pattern. Re-rostering someone is a new row with a later
-- effective_from, never an edit, so past attendance can always be explained
-- against the schedule that was actually in force.
CREATE TABLE hris.employee_shifts (
    id             TEXT PRIMARY KEY,
    employee_id    TEXT        NOT NULL REFERENCES hris.employees (id) ON DELETE CASCADE,
    day_of_week    INTEGER     NOT NULL CHECK (day_of_week BETWEEN 1 AND 7),
    -- NULL is a scheduled rest day, which is different from having no pattern.
    shift_id       TEXT REFERENCES hris.shifts (id),
    effective_from DATE        NOT NULL,
    effective_to   DATE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT employee_shifts_range CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX employee_shifts_lookup_idx
    ON hris.employee_shifts (employee_id, day_of_week, effective_from DESC);

CREATE TABLE hris.attendance (
    id           TEXT PRIMARY KEY,
    employee_id  TEXT        NOT NULL REFERENCES hris.employees (id) ON DELETE CASCADE,
    date         DATE        NOT NULL,
    clock_in     TIMESTAMPTZ,
    clock_out    TIMESTAMPTZ,
    shift_id     TEXT REFERENCES hris.shifts (id),
    -- Frozen at clock-in. Editing a shift later must not rewrite what somebody
    -- was expected to do last Tuesday.
    scheduled_start TIMESTAMPTZ,
    scheduled_end   TIMESTAMPTZ,
    work_hours   NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (work_hours >= 0),
    break_minutes INTEGER    NOT NULL DEFAULT 60 CHECK (break_minutes >= 0),
    status       TEXT        NOT NULL DEFAULT 'PRESENT'
                 CHECK (status IN ('PRESENT', 'LATE', 'ABSENT', 'HALF_DAY', 'REMOTE', 'ON_LEAVE', 'HOLIDAY', 'REST_DAY')),
    is_late      BOOLEAN     NOT NULL DEFAULT false,
    late_minutes INTEGER     NOT NULL DEFAULT 0 CHECK (late_minutes >= 0),
    overtime_hours NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (overtime_hours >= 0),
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- One row per person per day: clocking in twice is an update, not a
    -- second attendance record.
    CONSTRAINT attendance_one_per_day UNIQUE (employee_id, date),
    CONSTRAINT attendance_clock_ordered CHECK (clock_out IS NULL OR clock_in IS NULL OR clock_out >= clock_in)
);

CREATE INDEX attendance_employee_date_idx ON hris.attendance (employee_id, date DESC);
CREATE INDEX attendance_date_idx ON hris.attendance (date DESC);
CREATE INDEX attendance_status_idx ON hris.attendance (status, date DESC);

-- Public holidays are data, not code. The Indonesian government publishes them
-- by joint decree and moves the movable ones, so HR corrects the calendar
-- without waiting for a deploy.
CREATE TABLE hris.public_holidays (
    id            TEXT PRIMARY KEY,
    holiday_date  DATE        NOT NULL,
    name          TEXT        NOT NULL,
    type          TEXT        NOT NULL DEFAULT 'NATIONAL'
                  CHECK (type IN ('NATIONAL', 'COLLECTIVE', 'COMPANY')),
    -- The reason this column exists: collective leave ("cuti bersama") is
    -- drawn from the employee's annual allowance; a national holiday is not.
    deducts_leave BOOLEAN     NOT NULL DEFAULT false,
    -- Draft rows are imported but unapproved, and are ignored by every
    -- calculation until HR publishes them.
    status        TEXT        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('DRAFT', 'ACTIVE')),
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX public_holidays_date_idx ON hris.public_holidays (holiday_date);
-- The same date may carry a national holiday and a collective leave day, but
-- not the same entry twice.
CREATE UNIQUE INDEX public_holidays_unique_idx ON hris.public_holidays (holiday_date, lower(name));

CREATE TABLE hris.leaves (
    id               TEXT PRIMARY KEY,
    employee_id      TEXT        NOT NULL REFERENCES hris.employees (id) ON DELETE CASCADE,
    type             TEXT        NOT NULL
                     CHECK (type IN ('ANNUAL', 'SICK', 'MATERNITY', 'PATERNITY',
                                     'UNPAID', 'EMERGENCY', 'PILGRIMAGE', 'MENSTRUAL')),
    start_date       DATE        NOT NULL,
    end_date         DATE        NOT NULL,
    -- Working days the request actually costs: weekends and free public
    -- holidays are already excluded by the time this is written.
    total_days       NUMERIC(5, 2) NOT NULL CHECK (total_days > 0),
    reason           TEXT        NOT NULL,
    attachment_url   TEXT,
    status           TEXT        NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED')),
    approved_by      TEXT,
    approved_at      TIMESTAMPTZ,
    rejection_reason TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT leaves_dates_ordered CHECK (end_date >= start_date),
    CONSTRAINT leaves_rejected_has_reason
        CHECK (status <> 'REJECTED' OR rejection_reason IS NOT NULL)
);

CREATE INDEX leaves_employee_idx ON hris.leaves (employee_id, start_date DESC);
CREATE INDEX leaves_status_idx ON hris.leaves (status, start_date DESC);
CREATE INDEX leaves_dates_idx ON hris.leaves (start_date, end_date);

-- One allowance row per employee per year. Only annual leave is rationed; the
-- other counters exist so HR can see usage, not to refuse a request.
CREATE TABLE hris.leave_balances (
    employee_id      TEXT        NOT NULL REFERENCES hris.employees (id) ON DELETE CASCADE,
    year             INTEGER     NOT NULL CHECK (year >= 2020),
    annual_total     NUMERIC(5, 2) NOT NULL DEFAULT 12 CHECK (annual_total >= 0),
    annual_used      NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (annual_used >= 0),
    sick_used        NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (sick_used >= 0),
    unpaid_used      NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (unpaid_used >= 0),
    maternity_used   NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (maternity_used >= 0),
    paternity_used   NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (paternity_used >= 0),
    emergency_used   NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (emergency_used >= 0),
    pilgrimage_used  NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (pilgrimage_used >= 0),
    menstrual_used   NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (menstrual_used >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (employee_id, year),
    -- The allowance cannot be overspent. The application refuses first; this
    -- is what makes the refusal true even under a race.
    CONSTRAINT leave_balance_not_overspent CHECK (annual_used <= annual_total)
);

CREATE TABLE hris.overtime_requests (
    id               TEXT PRIMARY KEY,
    employee_id      TEXT        NOT NULL REFERENCES hris.employees (id) ON DELETE CASCADE,
    date             DATE        NOT NULL,
    start_time       TIME        NOT NULL,
    end_time         TIME        NOT NULL,
    hours            NUMERIC(5, 2) NOT NULL CHECK (hours > 0 AND hours <= 24),
    -- Who asked matters: company-directed overtime is payable on different
    -- terms from hours an employee volunteered.
    source           TEXT        NOT NULL DEFAULT 'EMPLOYEE'
                     CHECK (source IN ('EMPLOYEE', 'COMPANY')),
    status           TEXT        NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED')),
    reason           TEXT,
    decided_by       TEXT,
    decided_at       TIMESTAMPTZ,
    rejection_reason TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX overtime_employee_date_idx ON hris.overtime_requests (employee_id, date DESC);
CREATE INDEX overtime_status_idx ON hris.overtime_requests (status, date DESC);
