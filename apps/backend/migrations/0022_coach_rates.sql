-- What a coach is paid for a particular class.
--
-- A scheme sets one session fee for everything a coach delivers, which is a
-- reasonable default and a poor description of a studio: a race simulation is
-- two hours of setup and coaching, a mobility class is forty-five minutes on a
-- mat, and paying the same for both is either overpaying for one or losing the
-- coach who runs the other.
--
-- A rate overrides the scheme for one class type. Anything without a rate
-- falls back to the scheme, so a studio that does not care about this never
-- has to look at it.

CREATE TABLE incentives.scheme_rates (
    id               TEXT PRIMARY KEY,
    scheme_id        TEXT   NOT NULL REFERENCES incentives.schemes (id) ON DELETE CASCADE,
    -- A plain id: class types belong to the catalog module, which owns its own
    -- schema, so this is a reference by value rather than a foreign key.
    class_type_id    TEXT   NOT NULL,
    session_fee_idr  BIGINT NOT NULL DEFAULT 0 CHECK (session_fee_idr >= 0),
    per_attendee_idr BIGINT NOT NULL DEFAULT 0 CHECK (per_attendee_idr >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One rate per class type per scheme: two answers to "what does this pay" is
-- a payroll dispute waiting to happen.
CREATE UNIQUE INDEX scheme_rates_unique_idx
    ON incentives.scheme_rates (scheme_id, class_type_id);

COMMENT ON TABLE incentives.scheme_rates IS
    'Per-class-type overrides of a scheme''s session fee and per-attendee rate.';
