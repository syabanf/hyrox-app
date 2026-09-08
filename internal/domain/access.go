package domain

import "time"

// AccessLogState is the outcome recorded for every scan attempt. Denied scans
// are logged too: the log is the record of what happened at the door, not just
// of who got in.
type AccessLogState string

const (
	AccessRequested      AccessLogState = "REQUESTED"
	AccessAllowed        AccessLogState = "ALLOWED"
	AccessDenied         AccessLogState = "DENIED"
	AccessOfflineAllowed AccessLogState = "OFFLINE_ALLOWED"
	AccessSynced         AccessLogState = "SYNCED"
	AccessConflict       AccessLogState = "CONFLICT"
)

// GateDenialReason is why the gate stayed shut. These reach the scanner UI and
// the admin monitor as codes, so the wording can change without breaking them.
type GateDenialReason string

const (
	GateTokenInvalid        GateDenialReason = "TOKEN_INVALID"
	GateTokenExpired        GateDenialReason = "TOKEN_EXPIRED"
	GateTokenConsumed       GateDenialReason = "TOKEN_CONSUMED"
	GateMemberNotActive     GateDenialReason = "MEMBER_NOT_ACTIVE"
	GateAntiPassback        GateDenialReason = "ANTI_PASSBACK"
	GateNoBooking           GateDenialReason = "NO_BOOKING"
	GateInsufficientCredits GateDenialReason = "INSUFFICIENT_CREDITS"
)

// AccessMode distinguishes a scan validated by the server from one the gate
// admitted on its own while offline.
type AccessMode string

const (
	ModeOnline  AccessMode = "ONLINE"
	ModeOffline AccessMode = "OFFLINE"
)

// AccessLog is one scan attempt at one gate.
type AccessLog struct {
	ID string `json:"id"`
	// MemberID is nil when the token could not be resolved to anyone.
	MemberID   *string           `json:"memberId"`
	GateID     string            `json:"gateId"`
	BranchID   string            `json:"branchId"`
	Result     AccessLogState    `json:"result"`
	ReasonCode *GateDenialReason `json:"reasonCode"`
	// CreditDelta is the signed credit change this scan caused; 0 for denied
	// entries and for re-entries inside the grace window.
	CreditDelta int        `json:"creditDelta"`
	Mode        AccessMode `json:"mode"`
	BookingID   *string    `json:"bookingId"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// GateEntryKind separates a paid entry from a free pass-through.
type GateEntryKind string

const (
	EntryBooking GateEntryKind = "BOOKING"
	EntryReEntry GateEntryKind = "RE_ENTRY"
)

// GateEffectKind names a side effect the gate decision implies.
type GateEffectKind string

const (
	EffectConsumeToken   GateEffectKind = "CONSUME_TOKEN"
	EffectDeductCredits  GateEffectKind = "DEDUCT_CREDITS"
	EffectCheckInBooking GateEffectKind = "CHECK_IN_BOOKING"
)

// GateEffect is one state change the caller must apply.
type GateEffect struct {
	Kind        GateEffectKind
	Token       string
	Amount      int
	Description string
	BookingID   string
}

// CandidateBooking is the member's booking that justifies this entry.
type CandidateBooking struct {
	ID         string
	CreditCost int
}

// GateScanInput is everything the door needs to decide. The caller resolves
// the token, member, balance and booking; this function only judges.
type GateScanInput struct {
	// TokenProblem is empty when the presented token is valid.
	TokenProblem QrTokenProblem
	Token        QrToken
	Member       *Member
	Balance      int
	// LastAllowedEntryAt is when this member last got in at this branch.
	LastAllowedEntryAt *time.Time
	// CandidateBooking is the member's confirmed booking around now. Entry
	// requires one: the studio has no open-gym access.
	CandidateBooking *CandidateBooking
	Rules            BusinessRules
	Now              time.Time
}

// GateScanEvaluation is the decision plus the effects that must accompany it.
type GateScanEvaluation struct {
	Decision  AccessLogState
	Reason    *GateDenialReason
	EntryKind *GateEntryKind
	Effects   []GateEffect
}

// EvaluateGateScan runs the door pipeline: token, membership, anti-passback,
// booking, credit.
//
// It returns effects rather than applying them so the caller can commit the
// deduction, the check-in and the token consumption in ONE transaction. That
// is the whole point: the gate must never open without its deduction, and a
// deduction must never happen without the gate opening.
func EvaluateGateScan(in GateScanInput) GateScanEvaluation {
	denied := func(reason GateDenialReason, effects []GateEffect) GateScanEvaluation {
		r := reason
		return GateScanEvaluation{Decision: AccessDenied, Reason: &r, Effects: effects}
	}

	// 1. Is the credential itself valid?
	if in.TokenProblem != QrOK {
		return denied(tokenDenial(in.TokenProblem), nil)
	}
	// A presented token is burned even when the scan is refused, so a rejected
	// code cannot be retried at another gate.
	consume := GateEffect{Kind: EffectConsumeToken, Token: in.Token.Token}

	// 2. Is the membership in good standing?
	if in.Member == nil || !in.Member.IsActive() {
		return denied(GateMemberNotActive, []GateEffect{consume})
	}

	// 3. Re-entry grace, then anti-passback. Order matters: the grace window
	// sits inside the anti-passback window, so it must be tested first.
	if in.LastAllowedEntryAt != nil {
		elapsed := in.Now.Sub(*in.LastAllowedEntryAt).Minutes()
		if elapsed >= 0 && elapsed <= float64(in.Rules.ReEntryGraceMinutes) {
			kind := EntryReEntry
			return GateScanEvaluation{
				Decision:  AccessAllowed,
				EntryKind: &kind,
				Effects:   []GateEffect{consume},
			}
		}
		if elapsed >= 0 && elapsed <= float64(in.Rules.AntiPassbackMinutes) {
			return denied(GateAntiPassback, []GateEffect{consume})
		}
	}

	// 4. Entry is tied to a booked class.
	if in.CandidateBooking == nil {
		return denied(GateNoBooking, []GateEffect{consume})
	}

	// 5. The booked session's cost must be covered right now.
	if in.Balance < in.CandidateBooking.CreditCost {
		return denied(GateInsufficientCredits, []GateEffect{consume})
	}

	kind := EntryBooking
	return GateScanEvaluation{
		Decision:  AccessAllowed,
		EntryKind: &kind,
		Effects: []GateEffect{
			consume,
			{
				Kind:        EffectDeductCredits,
				Amount:      in.CandidateBooking.CreditCost,
				Description: "Class check-in",
			},
			{Kind: EffectCheckInBooking, BookingID: in.CandidateBooking.ID},
		},
	}
}

func tokenDenial(problem QrTokenProblem) GateDenialReason {
	switch problem {
	case QrExpired:
		return GateTokenExpired
	case QrConsumed:
		return GateTokenConsumed
	default:
		return GateTokenInvalid
	}
}
