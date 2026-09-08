// Package auth issues and verifies the bearer tokens every module trusts.
//
// Tokens are self-contained HS256 JWTs signed with a shared secret, so a
// service split out of the monolith verifies callers on its own without a
// round trip to an auth service or a shared session table. The cost of that
// choice is revocation: tokens stay valid until they expire, so TTLs are
// short-lived by configuration and sensitive actions re-check the live row.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind separates the two audiences: studio members and staff.
type Kind string

const (
	KindMember Kind = "member"
	KindAdmin  Kind = "admin"
)

// Principal is the authenticated caller as the rest of the code sees it.
type Principal struct {
	ID   string
	Kind Kind
	// Role and BranchID are set for staff only. An empty BranchID means the
	// user is not scoped to one branch (HQ level).
	Role     string
	BranchID string
}

func (p Principal) IsMember() bool { return p.Kind == KindMember }
func (p Principal) IsAdmin() bool  { return p.Kind == KindAdmin }

// claims is the JWT payload. Field names follow the registered claim names so
// the tokens stay readable in any JWT debugger.
type claims struct {
	Subject  string `json:"sub"`
	Kind     string `json:"knd"`
	Role     string `json:"rol,omitempty"`
	BranchID string `json:"brn,omitempty"`
	Issuer   string `json:"iss"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
	TokenID  string `json:"jti"`
}

var (
	ErrTokenMalformed = errors.New("auth: token is malformed")
	ErrTokenSignature = errors.New("auth: token signature does not verify")
	ErrTokenExpired   = errors.New("auth: token has expired")
	ErrTokenIssuer    = errors.New("auth: token issuer is not recognized")
)

// Issuer mints and verifies tokens.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	name   string
	nonce  func() string
}

// NewIssuer builds an issuer. nonce supplies the jti, which makes each token
// unique even when the same principal signs in twice in one second.
func NewIssuer(secret []byte, ttl time.Duration, name string, nonce func() string) *Issuer {
	return &Issuer{secret: secret, ttl: ttl, name: name, nonce: nonce}
}

// Issue returns a signed token for the principal plus its expiry.
func (i *Issuer) Issue(p Principal, now time.Time) (string, time.Time, error) {
	expires := now.Add(i.ttl)
	payload := claims{
		Subject:  p.ID,
		Kind:     string(p.Kind),
		Role:     p.Role,
		BranchID: p.BranchID,
		Issuer:   i.name,
		IssuedAt: now.Unix(),
		Expires:  expires.Unix(),
		TokenID:  i.nonce(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: encoding claims: %w", err)
	}
	// The header is constant, so it is precomputed rather than marshalled.
	const header = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(body)
	signature := i.sign(signingInput)
	return signingInput + "." + signature, expires, nil
}

// Parse verifies signature, issuer and expiry, returning the principal.
func (i *Issuer) Parse(token string, now time.Time) (Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, ErrTokenMalformed
	}
	expected := i.sign(parts[0] + "." + parts[1])
	// Constant-time compare: a timing side channel here would leak the secret.
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Principal{}, ErrTokenSignature
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, ErrTokenMalformed
	}
	var payload claims
	if err := json.Unmarshal(body, &payload); err != nil {
		return Principal{}, ErrTokenMalformed
	}
	if payload.Issuer != i.name {
		return Principal{}, ErrTokenIssuer
	}
	if now.Unix() >= payload.Expires {
		return Principal{}, ErrTokenExpired
	}
	kind := Kind(payload.Kind)
	if kind != KindMember && kind != KindAdmin {
		return Principal{}, ErrTokenMalformed
	}
	return Principal{
		ID:       payload.Subject,
		Kind:     kind,
		Role:     payload.Role,
		BranchID: payload.BranchID,
	}, nil
}

func (i *Issuer) sign(input string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
