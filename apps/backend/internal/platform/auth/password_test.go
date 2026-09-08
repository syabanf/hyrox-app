package auth_test

import (
	"strings"
	"testing"

	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
)

func TestPasswordRoundTrip(t *testing.T) {
	p := auth.NewPasswords(1000)
	hash, err := p.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if !p.Matches("correct horse battery staple", hash) {
		t.Fatal("the right password did not match its own hash")
	}
	if p.Matches("correct horse battery stapl", hash) {
		t.Fatal("a wrong password matched")
	}
}

func TestPasswordHashesAreSalted(t *testing.T) {
	p := auth.NewPasswords(1000)
	first, _ := p.Hash("same password")
	second, _ := p.Hash("same password")
	if first == second {
		t.Fatal("two hashes of the same password are identical, so the salt is not random")
	}
	// Both must still verify: the salt travels with the hash.
	if !p.Matches("same password", first) || !p.Matches("same password", second) {
		t.Fatal("a salted hash did not verify")
	}
}

func TestPasswordRejectsRubbish(t *testing.T) {
	p := auth.NewPasswords(1000)
	for _, hash := range []string{
		"",
		"not-a-hash",
		"pbkdf2-sha256$notanumber$c2FsdA$a2V5",
		"pbkdf2-sha256$0$c2FsdA$a2V5",
		"bcrypt$1000$c2FsdA$a2V5",
		"pbkdf2-sha256$1000$$a2V5",
	} {
		if p.Matches("anything", hash) {
			t.Fatalf("%q was accepted as a password hash", hash)
		}
	}
}

// An account with no password must refuse an empty password, not welcome it.
func TestEmptyPasswordDoesNotMatchEmptyHash(t *testing.T) {
	p := auth.NewPasswords(1000)
	if p.Matches("", "") {
		t.Fatal("an unset password accepted an empty attempt")
	}
}

func TestNeedsRehashOnWeakerHash(t *testing.T) {
	weak := auth.NewPasswords(1000)
	strong := auth.NewPasswords(50_000)

	hash, _ := weak.Hash("password")
	if !strong.NeedsRehash(hash) {
		t.Fatal("a hash below the current iteration count was not flagged for rehashing")
	}
	if weak.NeedsRehash(hash) {
		t.Fatal("a hash at the current iteration count was flagged for rehashing")
	}
	if !strong.NeedsRehash("nonsense") {
		t.Fatal("an unreadable hash was not flagged for rehashing")
	}
}

func TestHashCarriesItsParameters(t *testing.T) {
	hash, err := auth.NewPasswords(1234).Hash("password")
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$1234$") {
		t.Fatalf("the hash does not name its own scheme and cost: %s", hash)
	}
	if strings.Contains(hash, "password") {
		t.Fatal("the hash contains the password")
	}
}
