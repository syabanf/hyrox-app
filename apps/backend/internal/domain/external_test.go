package domain

import "testing"

func TestASubjectIsMatchedOnEmailOrPhoneAndNeverOnName(t *testing.T) {
	candidates := []MatchCandidate{
		{MemberID: "a", Email: "budi@example.com", Phone: "+628123456789", FullName: "Budi Santoso"},
		{MemberID: "b", Email: "other@example.com", Phone: "+628999888777", FullName: "Budi Santoso"},
	}

	if got, ok := MatchSubject("BUDI@example.com", candidates); !ok || got != "a" {
		t.Fatalf("email matches regardless of case, got %q", got)
	}
	// +6281... and 081... are the same phone, and a country code somebody
	// typed differently should not lose a member their points.
	if got, ok := MatchSubject("081 234 56789", candidates); !ok || got != "a" {
		t.Fatalf("the phone matches on its last digits, got %q", got)
	}

	// Two people called Budi Santoso is the normal case in any member list.
	// Awarding one of them the other's race result is worse than leaving it
	// for somebody to look at.
	if _, ok := MatchSubject("Budi Santoso", candidates); ok {
		t.Fatal("a name must never be enough to match on")
	}
	if _, ok := MatchSubject("", candidates); ok {
		t.Fatal("an empty subject matches nobody")
	}
}

func TestRecordingAPartnersWordIsNotPayingOutOnIt(t *testing.T) {
	trusted := IntegrationPartner{Active: true, AwardsXP: true}
	watched := IntegrationPartner{Active: true, AwardsXP: false}

	if got := DecideEvent(trusted, true); got.Status != EventProcessed || !got.AwardXP {
		t.Fatalf("a trusted partner's matched event pays out, got %+v", got)
	}
	if got := DecideEvent(watched, true); got.Status != EventProcessed || got.AwardXP {
		t.Fatalf("an untrusted partner's event is recorded and not paid, got %+v", got)
	}

	// Unmatched is a queue for a person, not an error: the member may join
	// next week and the event should still be there.
	if got := DecideEvent(trusted, false); got.Status != EventUnmatched || got.AwardXP {
		t.Fatalf("an unmatched event waits, got %+v", got)
	}
	// A switched-off partner is ignored rather than acted on.
	off := IntegrationPartner{Active: false, AwardsXP: true}
	if got := DecideEvent(off, true); got.Status != EventIgnored || got.AwardXP {
		t.Fatalf("a disabled partner's event is ignored, got %+v", got)
	}
}
