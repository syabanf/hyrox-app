package domain

import (
	"testing"
	"time"
)

func validToken() QrToken {
	return QrToken{
		Token:     "qr_abc",
		MemberID:  "mem_1",
		IssuedAt:  testNow,
		ExpiresAt: testNow.Add(45 * time.Second),
	}
}

func gateInput(mods ...func(*GateScanInput)) GateScanInput {
	member := testMember(MemberActive)
	in := GateScanInput{
		TokenProblem:     QrOK,
		Token:            validToken(),
		Member:           &member,
		Balance:          5,
		CandidateBooking: &CandidateBooking{ID: "bkg_1", CreditCost: 1},
		Rules:            DefaultBusinessRules(),
		Now:              testNow,
	}
	for _, m := range mods {
		m(&in)
	}
	return in
}

func effectKinds(effects []GateEffect) []GateEffectKind {
	kinds := make([]GateEffectKind, 0, len(effects))
	for _, e := range effects {
		kinds = append(kinds, e.Kind)
	}
	return kinds
}

func TestGateScanAllowsABookedMemberAndEmitsAllEffects(t *testing.T) {
	got := EvaluateGateScan(gateInput())

	if got.Decision != AccessAllowed {
		t.Fatalf("decision = %s, want ALLOWED", got.Decision)
	}
	if got.EntryKind == nil || *got.EntryKind != EntryBooking {
		t.Fatalf("entry kind = %v, want BOOKING", got.EntryKind)
	}
	kinds := effectKinds(got.Effects)
	if len(kinds) != 3 {
		t.Fatalf("effects = %v, want token consume + deduction + check-in", kinds)
	}
	if kinds[0] != EffectConsumeToken || kinds[1] != EffectDeductCredits || kinds[2] != EffectCheckInBooking {
		t.Fatalf("effects = %v, wrong order or kinds", kinds)
	}
	if got.Effects[1].Amount != 1 {
		t.Fatalf("deduction = %d, want the session credit cost (1)", got.Effects[1].Amount)
	}
}

func TestGateScanTokenProblemsMapToDenialReasons(t *testing.T) {
	tests := []struct {
		problem QrTokenProblem
		want    GateDenialReason
	}{
		{QrNotFound, GateTokenInvalid},
		{QrExpired, GateTokenExpired},
		{QrConsumed, GateTokenConsumed},
	}
	for _, tc := range tests {
		t.Run(string(tc.problem), func(t *testing.T) {
			got := EvaluateGateScan(gateInput(func(in *GateScanInput) { in.TokenProblem = tc.problem }))
			if got.Decision != AccessDenied {
				t.Fatalf("decision = %s, want DENIED", got.Decision)
			}
			if got.Reason == nil || *got.Reason != tc.want {
				t.Fatalf("reason = %v, want %s", got.Reason, tc.want)
			}
			// An unresolvable token is not consumed: there is nothing to burn.
			if len(got.Effects) != 0 {
				t.Fatalf("effects = %v, want none", effectKinds(got.Effects))
			}
		})
	}
}

func TestGateScanBurnsTheTokenEvenWhenItRefusesEntry(t *testing.T) {
	got := EvaluateGateScan(gateInput(func(in *GateScanInput) {
		suspended := testMember(MemberSuspended)
		in.Member = &suspended
	}))

	if got.Decision != AccessDenied || got.Reason == nil || *got.Reason != GateMemberNotActive {
		t.Fatalf("got %+v, want DENIED/MEMBER_NOT_ACTIVE", got)
	}
	// The presented code must not survive a refusal, or it could be retried
	// at another gate.
	if kinds := effectKinds(got.Effects); len(kinds) != 1 || kinds[0] != EffectConsumeToken {
		t.Fatalf("effects = %v, want a single token consume", kinds)
	}
}

func TestGateScanReEntryGraceLetsAMemberBackInForFree(t *testing.T) {
	last := testNow.Add(-10 * time.Minute) // inside the 15-minute grace
	got := EvaluateGateScan(gateInput(func(in *GateScanInput) { in.LastAllowedEntryAt = &last }))

	if got.Decision != AccessAllowed {
		t.Fatalf("decision = %s, want ALLOWED", got.Decision)
	}
	if got.EntryKind == nil || *got.EntryKind != EntryReEntry {
		t.Fatalf("entry kind = %v, want RE_ENTRY", got.EntryKind)
	}
	// Free means free: no second deduction for stepping outside.
	if kinds := effectKinds(got.Effects); len(kinds) != 1 || kinds[0] != EffectConsumeToken {
		t.Fatalf("effects = %v, want no deduction", kinds)
	}
}

func TestGateScanAntiPassbackBlocksAQuickSecondEntry(t *testing.T) {
	last := testNow.Add(-30 * time.Minute) // past grace, inside anti-passback
	got := EvaluateGateScan(gateInput(func(in *GateScanInput) { in.LastAllowedEntryAt = &last }))

	if got.Reason == nil || *got.Reason != GateAntiPassback {
		t.Fatalf("reason = %v, want ANTI_PASSBACK", got.Reason)
	}
}

func TestGateScanAllowsEntryOnceAntiPassbackHasElapsed(t *testing.T) {
	last := testNow.Add(-90 * time.Minute) // past the 60-minute window
	got := EvaluateGateScan(gateInput(func(in *GateScanInput) { in.LastAllowedEntryAt = &last }))

	if got.Decision != AccessAllowed {
		t.Fatalf("decision = %s, want ALLOWED", got.Decision)
	}
	if got.EntryKind == nil || *got.EntryKind != EntryBooking {
		t.Fatalf("entry kind = %v, want BOOKING", got.EntryKind)
	}
}

func TestGateScanRequiresABookedClass(t *testing.T) {
	got := EvaluateGateScan(gateInput(func(in *GateScanInput) { in.CandidateBooking = nil }))

	if got.Reason == nil || *got.Reason != GateNoBooking {
		t.Fatalf("reason = %v, want NO_BOOKING", got.Reason)
	}
}

func TestGateScanRefusesWhenCreditsCannotCoverTheClass(t *testing.T) {
	got := EvaluateGateScan(gateInput(func(in *GateScanInput) {
		in.Balance = 1
		in.CandidateBooking = &CandidateBooking{ID: "bkg_1", CreditCost: 2}
	}))

	if got.Reason == nil || *got.Reason != GateInsufficientCredits {
		t.Fatalf("reason = %v, want INSUFFICIENT_CREDITS", got.Reason)
	}
	if kinds := effectKinds(got.Effects); len(kinds) != 1 {
		t.Fatalf("effects = %v, want token consume only (no deduction)", kinds)
	}
}

func TestCheckQrTokenLifecycle(t *testing.T) {
	token := validToken()
	if got := CheckQrToken(&token, testNow); got != QrOK {
		t.Fatalf("fresh token = %s, want ok", got)
	}
	if got := CheckQrToken(nil, testNow); got != QrNotFound {
		t.Fatalf("missing token = %s, want NOT_FOUND", got)
	}
	if got := CheckQrToken(&token, testNow.Add(time.Minute)); got != QrExpired {
		t.Fatalf("stale token = %s, want EXPIRED", got)
	}

	consumed := validToken()
	at := testNow
	consumed.ConsumedAt = &at
	// A replayed token reads as CONSUMED even after it would have expired.
	if got := CheckQrToken(&consumed, testNow.Add(time.Hour)); got != QrConsumed {
		t.Fatalf("replayed token = %s, want CONSUMED", got)
	}
}

func TestIssueQrTokenSetsTTLAndCountdown(t *testing.T) {
	token := IssueQrToken("mem_1", "nonce123", testNow, 45)

	if token.Token != "qr_nonce123" {
		t.Fatalf("token = %s, want qr_nonce123", token.Token)
	}
	if !token.ExpiresAt.Equal(testNow.Add(45 * time.Second)) {
		t.Fatalf("expiry = %s, want 45s after issue", token.ExpiresAt)
	}
	if got := QrSecondsRemaining(token, testNow.Add(20*time.Second)); got != 25 {
		t.Fatalf("countdown = %d, want 25", got)
	}
	if got := QrSecondsRemaining(token, testNow.Add(time.Hour)); got != 0 {
		t.Fatalf("expired countdown = %d, want 0", got)
	}
}
