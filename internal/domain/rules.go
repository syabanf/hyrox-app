package domain

// BusinessRules are the policies the studio configures rather than the code
// hard-coding: expiry, cancellation, gate timing, booking windows. Branches
// may override any subset of the organization defaults.
//
// JSON tags match the published API contract exactly, because these structs
// are what the member app and admin panel already consume.
type BusinessRules struct {
	DefaultCreditExpiryDays    int    `json:"defaultCreditExpiryDays"`
	CancellationDeadlineHours  int    `json:"cancellationDeadlineHours"`
	LateCancellationPolicy     Policy `json:"lateCancellationPolicy"`
	NoShowPolicy               Policy `json:"noShowPolicy"`
	ReEntryGraceMinutes        int    `json:"reEntryGraceMinutes"`
	AntiPassbackMinutes        int    `json:"antiPassbackMinutes"`
	QRTTLSeconds               int    `json:"qrTtlSeconds"`
	WaitlistAutoPromote        bool   `json:"waitlistAutoPromote"`
	LowBalanceThreshold        int    `json:"lowBalanceThreshold"`
	ExpiryReminderDays         int    `json:"expiryReminderDays"`
	BookingOpensDaysBefore     int    `json:"bookingOpensDaysBefore"`
	BookingClosesMinutesBefore int    `json:"bookingClosesMinutesBefore"`
}

// Policy decides whether a credit is kept or lost when a member cancels late
// or fails to show up.
type Policy string

const (
	PolicyForfeit Policy = "FORFEIT"
	PolicyFree    Policy = "FREE"
)

// DefaultBusinessRules are the values a fresh organization starts with.
func DefaultBusinessRules() BusinessRules {
	return BusinessRules{
		DefaultCreditExpiryDays:    60,
		CancellationDeadlineHours:  4,
		LateCancellationPolicy:     PolicyForfeit,
		NoShowPolicy:               PolicyForfeit,
		ReEntryGraceMinutes:        15,
		AntiPassbackMinutes:        60,
		QRTTLSeconds:               45,
		WaitlistAutoPromote:        true,
		LowBalanceThreshold:        3,
		ExpiryReminderDays:         7,
		BookingOpensDaysBefore:     7,
		BookingClosesMinutesBefore: 0,
	}
}

// RulesOverride is a partial BusinessRules: every field is optional, so a
// branch overrides only what it actually differs on. Stored as JSONB.
type RulesOverride struct {
	DefaultCreditExpiryDays    *int    `json:"defaultCreditExpiryDays,omitempty"`
	CancellationDeadlineHours  *int    `json:"cancellationDeadlineHours,omitempty"`
	LateCancellationPolicy     *Policy `json:"lateCancellationPolicy,omitempty"`
	NoShowPolicy               *Policy `json:"noShowPolicy,omitempty"`
	ReEntryGraceMinutes        *int    `json:"reEntryGraceMinutes,omitempty"`
	AntiPassbackMinutes        *int    `json:"antiPassbackMinutes,omitempty"`
	QRTTLSeconds               *int    `json:"qrTtlSeconds,omitempty"`
	WaitlistAutoPromote        *bool   `json:"waitlistAutoPromote,omitempty"`
	LowBalanceThreshold        *int    `json:"lowBalanceThreshold,omitempty"`
	ExpiryReminderDays         *int    `json:"expiryReminderDays,omitempty"`
	BookingOpensDaysBefore     *int    `json:"bookingOpensDaysBefore,omitempty"`
	BookingClosesMinutesBefore *int    `json:"bookingClosesMinutesBefore,omitempty"`
}

// IsEmpty reports whether the override sets nothing.
func (o RulesOverride) IsEmpty() bool {
	return o == RulesOverride{}
}

// ResolveRules merges a branch override over the organization defaults. Any
// rule the branch does not set keeps the organization value.
func ResolveRules(base BusinessRules, override *RulesOverride) BusinessRules {
	if override == nil {
		return base
	}
	merged := base
	setIf(&merged.DefaultCreditExpiryDays, override.DefaultCreditExpiryDays)
	setIf(&merged.CancellationDeadlineHours, override.CancellationDeadlineHours)
	setIf(&merged.LateCancellationPolicy, override.LateCancellationPolicy)
	setIf(&merged.NoShowPolicy, override.NoShowPolicy)
	setIf(&merged.ReEntryGraceMinutes, override.ReEntryGraceMinutes)
	setIf(&merged.AntiPassbackMinutes, override.AntiPassbackMinutes)
	setIf(&merged.QRTTLSeconds, override.QRTTLSeconds)
	setIf(&merged.WaitlistAutoPromote, override.WaitlistAutoPromote)
	setIf(&merged.LowBalanceThreshold, override.LowBalanceThreshold)
	setIf(&merged.ExpiryReminderDays, override.ExpiryReminderDays)
	setIf(&merged.BookingOpensDaysBefore, override.BookingOpensDaysBefore)
	setIf(&merged.BookingClosesMinutesBefore, override.BookingClosesMinutesBefore)
	return merged
}

func setIf[T any](target *T, value *T) {
	if value != nil {
		*target = *value
	}
}
