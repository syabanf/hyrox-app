package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// DefaultPasswordIterations follows OWASP's guidance for PBKDF2-HMAC-SHA256.
//
// It is deliberately slow: roughly a tenth of a second per attempt, which is
// unnoticeable at a login form and ruinous for anyone working through a
// password list against a stolen table.
const DefaultPasswordIterations = 210_000

const (
	// The scheme is written into every hash, so raising the iteration count or
	// changing the KDF later does not invalidate the passwords already stored.
	passwordScheme  = "pbkdf2-sha256"
	passwordSaltLen = 16
	passwordKeyLen  = 32
)

// Passwords hashes and checks staff passwords.
//
// Supervisor PINs use a keyed digest a few lines below; passwords use a slow
// KDF with a per-password salt. The difference is deliberate: a PIN has four
// digits of entropy and is only checked against an account that already holds
// the permission, whereas a password is the whole of the credential.
type Passwords struct{ iterations int }

// NewPasswords builds a hasher. Iterations below one fall back to the default,
// so a missing configuration value is safe rather than instant.
func NewPasswords(iterations int) *Passwords {
	if iterations < 1 {
		iterations = DefaultPasswordIterations
	}
	return &Passwords{iterations: iterations}
}

// Hash returns "pbkdf2-sha256$<iterations>$<salt>$<key>", base64 raw-URL
// throughout so the value survives a JSON round trip and a URL unharmed.
func (p *Passwords) Hash(plain string) (string, error) {
	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generating salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, plain, salt, p.iterations, passwordKeyLen)
	if err != nil {
		return "", fmt.Errorf("auth: hashing password: %w", err)
	}
	enc := base64.RawURLEncoding
	return fmt.Sprintf("%s$%d$%s$%s",
		passwordScheme, p.iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Matches reports whether plain produced encoded.
//
// A malformed or empty hash returns false rather than an error: an account
// with no password set must fail the check like any wrong password, not take a
// different path a caller might forget to handle.
func (p *Passwords) Matches(plain, encoded string) bool {
	scheme, iterations, salt, key, ok := parsePasswordHash(encoded)
	if !ok || scheme != passwordScheme {
		return false
	}
	candidate, err := pbkdf2.Key(sha256.New, plain, salt, iterations, len(key))
	if err != nil {
		return false
	}
	// Constant time: a fast reject would leak how much of the hash matched.
	return subtle.ConstantTimeCompare(candidate, key) == 1
}

// NeedsRehash reports whether a stored hash is weaker than what is issued now,
// which is the signal to re-hash the password on the next successful sign-in.
func (p *Passwords) NeedsRehash(encoded string) bool {
	scheme, iterations, _, key, ok := parsePasswordHash(encoded)
	if !ok {
		return true
	}
	return scheme != passwordScheme || iterations < p.iterations || len(key) < passwordKeyLen
}

func parsePasswordHash(encoded string) (scheme string, iterations int, salt, key []byte, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return "", 0, nil, nil, false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return "", 0, nil, nil, false
	}
	enc := base64.RawURLEncoding
	salt, err = enc.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return "", 0, nil, nil, false
	}
	key, err = enc.DecodeString(parts[3])
	if err != nil || len(key) == 0 {
		return "", 0, nil, nil, false
	}
	return parts[0], iterations, salt, key, true
}
