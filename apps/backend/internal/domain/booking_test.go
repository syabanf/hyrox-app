package domain

import (
	"testing"
	"time"
)

func testSession(mods ...func(*ClassSession)) ClassSession {
	s := ClassSession{
		ID:              "ses_1",
		ClassTypeID:     "clt_1",
		BranchID:        "brn_1",
		CoachID:         "coa_1",
		StartsAt:        testNow.Add(24 * time.Hour),
		EndsAt:          testNow.Add(25 * time.Hour),
		Capacity:        10,
		CreditCost:      1,
		BookingOpensAt:  testNow.Add(-24 * time.Hour),
		BookingClosesAt: testNow.Add(24 * time.Hour),
		Status:          SessionPublished,
	}
	for _, m := range mods {
		m(&s)
	}
	return s
}

func testMember(status MemberStatus) Member {
	return Member{ID: "mem_1", FullName: "Demo Member", Status: status}
}

func TestEvaluateBookingDecisionMatrix(t *testing.T) {
	tests := []struct {
		name     string
		request  BookingRequest
		wantKind BookingDecisionKind
		wantWhy  BookingDenialReason
		wantPos  int
	}{
		{
			name: "confirms when everything checks out",
			request: BookingRequest{
				Member: testMember(MemberActive), Session: testSession(),
				Balance: 5, ConfirmedCount: 3, Now: testNow,
			},
			wantKind: DecisionConfirm,
		},
		{
			name: "denies a suspended member",
			request: BookingRequest{
				Member: testMember(MemberSuspended), Session: testSession(),
				Balance: 5, Now: testNow,
			},
			wantKind: DecisionDeny, wantWhy: DenyMemberNotActive,
		},
		{
			name: "denies a draft session",
			request: BookingRequest{
				Member:  testMember(MemberActive),
				Session: testSession(func(s *ClassSession) { s.Status = SessionDraft }),
				Balance: 5, Now: testNow,
			},
			wantKind: DecisionDeny, wantWhy: DenySessionNotBookable,
		},
		{
			name: "denies before the booking window opens",
			request: BookingRequest{
				Member:  testMember(MemberActive),
				Session: testSession(func(s *ClassSession) { s.BookingOpensAt = testNow.Add(time.Hour) }),
				Balance: 5, Now: testNow,
			},
			wantKind: DecisionDeny, wantWhy: DenyBookingNotOpenYet,
		},
		{
			name: "denies after the booking window closes",
			request: BookingRequest{
				Member:  testMember(MemberActive),
				Session: testSession(func(s *ClassSession) { s.BookingClosesAt = testNow.Add(-time.Minute) }),
				Balance: 5, Now: testNow,
			},
			wantKind: DecisionDeny, wantWhy: DenyBookingWindowClosed,
		},
		{
			name: "denies a second active booking for the same session",
			request: BookingRequest{
				Member: testMember(MemberActive), Session: testSession(),
				Balance: 5, HasExistingActiveBooking: true, Now: testNow,
			},
			wantKind: DecisionDeny, wantWhy: DenyAlreadyBooked,
		},
		{
			name: "denies when the balance cannot cover the class",
			request: BookingRequest{
				Member:  testMember(MemberActive),
				Session: testSession(func(s *ClassSession) { s.CreditCost = 3 }),
				Balance: 2, Now: testNow,
			},
			wantKind: DecisionDeny, wantWhy: DenyInsufficientCredits,
		},
		{
			name: "waitlists when the class is at capacity",
			request: BookingRequest{
				Member: testMember(MemberActive), Session: testSession(),
				Balance: 5, ConfirmedCount: 10, WaitlistCount: 2, Now: testNow,
			},
			wantKind: DecisionWaitlist, wantPos: 3,
		},
		{
			name: "waitlists on a FULL session too",
			request: BookingRequest{
				Member:  testMember(MemberActive),
				Session: testSession(func(s *ClassSession) { s.Status = SessionFull }),
				Balance: 5, ConfirmedCount: 10, WaitlistCount: 0, Now: testNow,
			},
			wantKind: DecisionWaitlist, wantPos: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateBooking(tc.request)
			if got.Kind != tc.wantKind {
				t.Fatalf("kind = %s, want %s", got.Kind, tc.wantKind)
			}
			if tc.wantWhy != "" && got.Reason != tc.wantWhy {
				t.Fatalf("reason = %s, want %s", got.Reason, tc.wantWhy)
			}
			if tc.wantPos != 0 && got.Position != tc.wantPos {
				t.Fatalf("position = %d, want %d", got.Position, tc.wantPos)
			}
		})
	}
}

func TestBookingChecksMembershipBeforeAnythingElse(t *testing.T) {
	// A suspended member on an unbookable session should hear about the
	// membership first: the pipeline order is part of the contract.
	got := EvaluateBooking(BookingRequest{
		Member:  testMember(MemberSuspended),
		Session: testSession(func(s *ClassSession) { s.Status = SessionCancelled }),
		Balance: 0, Now: testNow,
	})
	if got.Reason != DenyMemberNotActive {
		t.Fatalf("reason = %s, want MEMBER_NOT_ACTIVE", got.Reason)
	}
}

func TestEvaluateCancellationAppliesThePolicyAtTheDeadline(t *testing.T) {
	rules := DefaultBusinessRules() // 4 hours, FORFEIT
	session := testSession(func(s *ClassSession) { s.StartsAt = testNow.Add(4 * time.Hour); s.CreditCost = 2 })

	// Exactly on the deadline is still free.
	onDeadline := EvaluateCancellation(session, rules, testNow)
	if onDeadline.Kind != CancelReleased {
		t.Fatalf("on deadline = %s, want RELEASED", onDeadline.Kind)
	}

	late := EvaluateCancellation(session, rules, testNow.Add(time.Minute))
	if late.Kind != CancelLate {
		t.Fatalf("after deadline = %s, want LATE", late.Kind)
	}
	if late.PenaltyCredits != 2 {
		t.Fatalf("penalty = %d, want 2", late.PenaltyCredits)
	}

	rules.LateCancellationPolicy = PolicyFree
	free := EvaluateCancellation(session, rules, testNow.Add(time.Minute))
	if free.Kind != CancelLate || free.PenaltyCredits != 0 {
		t.Fatalf("FREE policy = %+v, want LATE with no penalty", free)
	}
}

func TestNoShowPenaltyFollowsPolicy(t *testing.T) {
	session := testSession(func(s *ClassSession) { s.CreditCost = 3 })
	rules := DefaultBusinessRules()
	if got := NoShowPenalty(session, rules); got != 3 {
		t.Fatalf("forfeit penalty = %d, want 3", got)
	}
	rules.NoShowPolicy = PolicyFree
	if got := NoShowPenalty(session, rules); got != 0 {
		t.Fatalf("free penalty = %d, want 0", got)
	}
}

func TestWaitlistPromotionIsFIFOByPositionThenTime(t *testing.T) {
	pos := func(n int) *int { return &n }
	waitlist := []Booking{
		{ID: "bkg_c", Status: BookingWaitlist, WaitlistPosition: pos(2), CreatedAt: testNow},
		{ID: "bkg_a", Status: BookingWaitlist, WaitlistPosition: pos(1), CreatedAt: testNow.Add(time.Minute)},
		{ID: "bkg_x", Status: BookingCancelled, WaitlistPosition: pos(0), CreatedAt: testNow},
	}

	winner := PickWaitlistPromotion(waitlist)
	if winner == nil || winner.ID != "bkg_a" {
		t.Fatalf("winner = %+v, want bkg_a (lowest position)", winner)
	}

	// Same position: the earlier join wins.
	tie := []Booking{
		{ID: "bkg_late", Status: BookingWaitlist, WaitlistPosition: pos(1), CreatedAt: testNow.Add(time.Hour)},
		{ID: "bkg_early", Status: BookingWaitlist, WaitlistPosition: pos(1), CreatedAt: testNow},
	}
	if winner := PickWaitlistPromotion(tie); winner == nil || winner.ID != "bkg_early" {
		t.Fatalf("tie winner = %+v, want bkg_early", winner)
	}

	if PickWaitlistPromotion([]Booking{{ID: "bkg_x", Status: BookingCancelled}}) != nil {
		t.Fatal("cancelled-only waitlist should promote nobody")
	}
}

func TestNextWaitlistPositionCountsOnlyWaitingBookings(t *testing.T) {
	pos := func(n int) *int { return &n }
	waitlist := []Booking{
		{Status: BookingWaitlist, WaitlistPosition: pos(1)},
		{Status: BookingWaitlist, WaitlistPosition: pos(3)},
		{Status: BookingCancelled, WaitlistPosition: pos(9)},
	}
	if got := NextWaitlistPosition(waitlist); got != 4 {
		t.Fatalf("next position = %d, want 4", got)
	}
	if got := NextWaitlistPosition(nil); got != 1 {
		t.Fatalf("first position = %d, want 1", got)
	}
}

func TestBookingTransitionsGuardTheLifecycle(t *testing.T) {
	if _, err := Transition(BookingTransitions, BookingConfirmed, BookingCheckedIn); err != nil {
		t.Fatalf("CONFIRMED -> CHECKED_IN should be legal: %v", err)
	}
	if _, err := Transition(BookingTransitions, BookingCancelled, BookingConfirmed); err == nil {
		t.Fatal("CANCELLED -> CONFIRMED must be rejected")
	}
	if _, err := Transition(BookingTransitions, BookingWaitlist, BookingCheckedIn); err == nil {
		t.Fatal("a waitlisted member must be confirmed before checking in")
	}
}

func TestDeriveBookingWindowUsesRules(t *testing.T) {
	rules := DefaultBusinessRules()
	rules.BookingOpensDaysBefore = 3
	rules.BookingClosesMinutesBefore = 30
	start := testNow.Add(72 * time.Hour)

	opens, closes := DeriveBookingWindow(start, rules)
	if !opens.Equal(start.AddDate(0, 0, -3)) {
		t.Fatalf("opens = %s, want 3 days before start", opens)
	}
	if !closes.Equal(start.Add(-30 * time.Minute)) {
		t.Fatalf("closes = %s, want 30 minutes before start", closes)
	}
}
