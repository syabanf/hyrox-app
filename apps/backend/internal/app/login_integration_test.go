package app_test

import (
	"net/http"
	"testing"

	"github.com/syabanf/nuhabit-backend/internal/seed"
)

// The ordinary way in: an email address and the password that belongs to it.
func TestAdminSignsInWithAPassword(t *testing.T) {
	h := newHarness(t)

	status, session := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email":    "alya@nuhabit.id",
		"password": seed.DemoPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("signing in returned %d: %v", status, session)
	}
	if session["token"] == "" || session["token"] == nil {
		t.Fatal("a successful sign-in returned no token")
	}
	user := session["user"].(map[string]any)
	if user["email"] != "alya@nuhabit.id" || user["role"] != "SUPER_ADMIN" {
		t.Fatalf("signed in as the wrong person: %v", user)
	}
	if session["mustChangePassword"] != false {
		t.Fatalf("a seeded account should not be forced to change its password: %v", session["mustChangePassword"])
	}

	// The token works on a real endpoint, not just as a string.
	status, _ = h.request(http.MethodGet, "/api/admin/members", session["token"].(string), nil)
	if status != http.StatusOK {
		t.Fatalf("the issued token was refused by the members endpoint: %d", status)
	}
}

// Email case is not part of the credential; a password is.
func TestSignInIsCaseInsensitiveOnEmailOnly(t *testing.T) {
	h := newHarness(t)

	status, _ := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email":    "  ALYA@NuHabit.ID  ",
		"password": seed.DemoPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("a differently-cased email was refused: %d", status)
	}

	status, _ = h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email":    "alya@nuhabit.id",
		"password": "NUHABIT-DEMO-2026",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("a differently-cased password was accepted: %d", status)
	}
}

// A wrong password and an email nobody has must be indistinguishable, or the
// login form becomes a way to find out who works here.
func TestUnknownAccountAndWrongPasswordLookTheSame(t *testing.T) {
	h := newHarness(t)

	wrongPassword, wrongBody := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email":    "alya@nuhabit.id",
		"password": "not-the-password",
	})
	unknownEmail, unknownBody := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email":    "nobody@nuhabit.id",
		"password": "not-the-password",
	})

	if wrongPassword != http.StatusUnauthorized || unknownEmail != http.StatusUnauthorized {
		t.Fatalf("expected 401 for both, got %d and %d", wrongPassword, unknownEmail)
	}
	if wrongBody["message"] != unknownBody["message"] || wrongBody["code"] != unknownBody["code"] {
		t.Fatalf("the two failures are distinguishable:\n  wrong password: %v\n  unknown email:  %v",
			wrongBody, unknownBody)
	}
}

// Ten wrong guesses shut the door, and the right password does not reopen it.
func TestRepeatedWrongPasswordsLockTheAccount(t *testing.T) {
	h := newHarness(t)

	for attempt := 1; attempt <= 10; attempt++ {
		status, body := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
			"email":    "nadia@nuhabit.id",
			"password": "wrong",
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d, want 401: %v", attempt, status, body)
		}
	}

	status, body := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email":    "nadia@nuhabit.id",
		"password": seed.DemoPassword,
	})
	if status != http.StatusTooManyRequests {
		t.Fatalf("the right password got in past a locked account: %d %v", status, body)
	}

	// Somebody else's account is untouched: the lock is per-account, not global.
	status, _ = h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email":    "alya@nuhabit.id",
		"password": seed.DemoPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("one locked account locked out everybody else: %d", status)
	}
}

// A reset clears the lock, and the new password has to be replaced at once.
func TestPasswordResetUnlocksAndForcesAChange(t *testing.T) {
	h := newHarness(t)
	super := h.adminToken("adm_super")

	for range 10 {
		h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
			"email": "nadia@nuhabit.id", "password": "wrong",
		})
	}

	status, body := h.request(http.MethodPut, "/api/admin/users/adm_desk/password", super,
		map[string]string{"password": "handed-over-once"})
	if status != http.StatusOK {
		t.Fatalf("setting a password returned %d: %v", status, body)
	}

	status, session := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email": "nadia@nuhabit.id", "password": "handed-over-once",
	})
	if status != http.StatusOK {
		t.Fatalf("the new password was refused after a reset: %d %v", status, session)
	}
	if session["mustChangePassword"] != true {
		t.Fatal("a password somebody else chose was not flagged for replacement")
	}

	// Replacing it clears the flag. The current password is not required,
	// because the account never had one it chose.
	token := session["token"].(string)
	status, body = h.request(http.MethodPost, "/api/admin/auth/password", token, map[string]string{
		"currentPassword": "handed-over-once",
		"newPassword":     "chosen-by-nadia",
	})
	if status != http.StatusOK {
		t.Fatalf("changing the password returned %d: %v", status, body)
	}

	status, session = h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email": "nadia@nuhabit.id", "password": "chosen-by-nadia",
	})
	if status != http.StatusOK || session["mustChangePassword"] != false {
		t.Fatalf("the change did not take: %d %v", status, session)
	}
}

// Changing a password needs the old one: a borrowed session is not consent.
func TestChangingAPasswordNeedsTheCurrentOne(t *testing.T) {
	h := newHarness(t)

	status, session := h.request(http.MethodPost, "/api/admin/auth/login", "", map[string]string{
		"email": "raka@nuhabit.id", "password": seed.DemoPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("signing in returned %d: %v", status, session)
	}
	token := session["token"].(string)

	status, body := h.request(http.MethodPost, "/api/admin/auth/password", token, map[string]string{
		"currentPassword": "guessing",
		"newPassword":     "a-perfectly-good-password",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("a wrong current password was accepted: %d %v", status, body)
	}

	// And the replacement has to clear the length floor.
	status, body = h.request(http.MethodPost, "/api/admin/auth/password", token, map[string]string{
		"currentPassword": seed.DemoPassword,
		"newPassword":     "short",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a five-character password was accepted: %d %v", status, body)
	}

	// Nor may it be the one already in use.
	status, body = h.request(http.MethodPost, "/api/admin/auth/password", token, map[string]string{
		"currentPassword": seed.DemoPassword,
		"newPassword":     seed.DemoPassword,
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("the same password was accepted as a change: %d %v", status, body)
	}
}

// Only somebody who manages logins may hand out a password.
func TestSettingSomebodyElsesPasswordNeedsThePermission(t *testing.T) {
	h := newHarness(t)
	desk := h.adminToken("adm_desk")

	status, body := h.request(http.MethodPut, "/api/admin/users/adm_super/password", desk,
		map[string]string{"password": "front-desk-takeover"})
	if status != http.StatusForbidden {
		t.Fatalf("the front desk reset a Super Admin's password: %d %v", status, body)
	}
}

// The login screen asks what it is allowed to offer before drawing itself.
func TestAuthModeIsPublic(t *testing.T) {
	h := newHarness(t)

	status, mode := h.request(http.MethodGet, "/api/admin/auth/mode", "", nil)
	if status != http.StatusOK {
		t.Fatalf("reading the auth mode returned %d: %v", status, mode)
	}
	if mode["demoRoster"] != true {
		t.Fatalf("the test harness runs with demo mode on, but the mode says %v", mode["demoRoster"])
	}
	if mode["accountsWithoutPassword"] != float64(0) {
		t.Fatalf("the seeded roster left %v accounts unable to sign in", mode["accountsWithoutPassword"])
	}
	if mode["minPasswordLength"] != float64(10) {
		t.Fatalf("the form and the server disagree on the length floor: %v", mode["minPasswordLength"])
	}
}
