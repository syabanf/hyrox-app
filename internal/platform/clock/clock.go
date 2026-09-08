// Package clock makes "now" an injected dependency so time-sensitive rules
// (QR expiry, booking windows, anti-passback, credit expiry) are testable
// without sleeping.
package clock

import "time"

type Clock interface {
	Now() time.Time
}

// Real reads the system clock.
type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

// Fixed returns a time the test controls.
type Fixed struct{ T time.Time }

func NewFixed(t time.Time) *Fixed { return &Fixed{T: t.UTC()} }

func (f *Fixed) Now() time.Time { return f.T }

// Advance moves a fixed clock forward, for multi-step scenarios.
func (f *Fixed) Advance(d time.Duration) { f.T = f.T.Add(d) }
