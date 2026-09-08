-- Staff sign in with a password.
--
-- Until now the panel signed people in by picking a card off a list, which is
-- fine for a demo and is not a login. The columns here are what a login needs
-- that a role picker does not: something only the user knows, a count of the
-- guesses that missed, and a door that closes for a while when too many do.
--
-- The hash carries its own scheme and cost ("pbkdf2-sha256$210000$salt$key"),
-- so raising the cost later re-hashes people as they sign in rather than
-- locking out everyone whose password predates the change.

ALTER TABLE identity.admin_users
    ADD COLUMN password_hash TEXT,
    ADD COLUMN password_set_at TIMESTAMPTZ,
    -- An administrator setting somebody else's password knows it. That is only
    -- acceptable if the password dies at first use, so a reset sets this flag
    -- and the next sign-in has to clear it.
    ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN last_login_at TIMESTAMPTZ,
    ADD COLUMN failed_logins INTEGER NOT NULL DEFAULT 0
               CHECK (failed_logins >= 0),
    ADD COLUMN locked_until TIMESTAMPTZ;

COMMENT ON COLUMN identity.admin_users.password_hash IS
    'PBKDF2-HMAC-SHA256, salted per password. NULL means the account cannot sign in with a password yet.';
COMMENT ON COLUMN identity.admin_users.locked_until IS
    'Set when failed_logins crosses the threshold; cleared by a successful sign-in or a password reset.';
