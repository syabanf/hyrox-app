-- Members sign in with a password.
--
-- Until now a member typed an email address and waited for a one-time code.
-- That is a fine way to prove you own an inbox and a poor way to open an app
-- you use four times a week: it costs a round trip through a mail provider
-- every single time, and it fails at exactly the moment somebody is standing
-- at the door with a queue behind them.
--
-- The columns mirror identity.admin_users, for the same reasons and with the
-- same hash format ("pbkdf2-sha256$210000$salt$key") — the scheme and cost
-- travel with the hash, so raising the cost later re-hashes people as they
-- sign in rather than locking out everyone whose password predates it.

ALTER TABLE identity.members
    ADD COLUMN password_hash TEXT,
    ADD COLUMN password_set_at TIMESTAMPTZ,
    ADD COLUMN last_login_at TIMESTAMPTZ,
    ADD COLUMN failed_logins INTEGER NOT NULL DEFAULT 0
               CHECK (failed_logins >= 0),
    ADD COLUMN locked_until TIMESTAMPTZ;

COMMENT ON COLUMN identity.members.password_hash IS
    'PBKDF2-HMAC-SHA256, salted per password. NULL means this member predates passwords and must use a one-time code until they set one.';
COMMENT ON COLUMN identity.members.locked_until IS
    'Set when failed_logins crosses the threshold; cleared by a successful sign-in or a password reset.';

-- Sign-in looks a member up by email or by phone, and does it before it knows
-- which one it was handed. Both need to be quick, and neither may be
-- ambiguous: two accounts sharing a phone number would make "who is signing
-- in" a question with two answers.
CREATE UNIQUE INDEX members_phone_unique ON identity.members (phone)
    WHERE phone <> '' AND status <> 'ARCHIVED';
