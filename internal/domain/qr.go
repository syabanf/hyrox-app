package domain

import "time"

// QrToken is a short-lived, single-use access credential.
//
// It is deliberately NOT a member id: a screenshot of a member's code is
// useless within a minute, and each token opens the gate at most once.
type QrToken struct {
	Token      string     `json:"token"`
	MemberID   string     `json:"memberId"`
	IssuedAt   time.Time  `json:"issuedAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	ConsumedAt *time.Time `json:"consumedAt"`
}

// IssueQrToken mints a credential valid for ttlSeconds. The nonce must be
// unguessable; the caller supplies it from a cryptographic source.
func IssueQrToken(memberID, nonce string, now time.Time, ttlSeconds int) QrToken {
	return QrToken{
		Token:     "qr_" + nonce,
		MemberID:  memberID,
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Duration(ttlSeconds) * time.Second),
	}
}

// QrTokenProblem is why a token was rejected; empty means the token is good.
type QrTokenProblem string

const (
	QrOK       QrTokenProblem = ""
	QrNotFound QrTokenProblem = "NOT_FOUND"
	QrExpired  QrTokenProblem = "EXPIRED"
	QrConsumed QrTokenProblem = "CONSUMED"
)

// CheckQrToken validates a presented token. Consumption is checked before
// expiry so a replayed token always reads as CONSUMED, which is the more
// actionable signal at the door.
func CheckQrToken(token *QrToken, now time.Time) QrTokenProblem {
	switch {
	case token == nil:
		return QrNotFound
	case token.ConsumedAt != nil:
		return QrConsumed
	case now.After(token.ExpiresAt):
		return QrExpired
	default:
		return QrOK
	}
}

// QrSecondsRemaining is the countdown the member app renders around the code.
func QrSecondsRemaining(token QrToken, now time.Time) int {
	remaining := int(token.ExpiresAt.Sub(now).Seconds())
	if remaining < 0 {
		return 0
	}
	return remaining
}
