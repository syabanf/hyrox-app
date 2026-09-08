package domain

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)

func entry(id string, kind LedgerEntryType, amount int, opts ...func(*CreditLedgerEntry)) CreditLedgerEntry {
	e := CreditLedgerEntry{ID: id, MemberID: "mem_1", Type: kind, Amount: amount, CreatedAt: testNow}
	for _, o := range opts {
		o(&e)
	}
	return e
}

func withSource(sourceType LedgerSourceType, sourceID string) func(*CreditLedgerEntry) {
	return func(e *CreditLedgerEntry) {
		e.SourceType = &sourceType
		e.SourceID = &sourceID
	}
}

func lot(id string, credits int, expiresIn time.Duration) TopUpLot {
	return TopUpLot{
		ID:        id,
		MemberID:  "mem_1",
		Credits:   credits,
		ExpiresAt: testNow.Add(expiresIn),
		CreatedAt: testNow,
	}
}

func TestComputeBalanceSumsSignedAmounts(t *testing.T) {
	entries := []CreditLedgerEntry{
		entry("led_1", LedgerTopUp, 10),
		entry("led_2", LedgerVisitDeduction, -1),
		entry("led_3", LedgerVisitDeduction, -1),
		entry("led_4", LedgerBonus, 2),
	}
	if got := ComputeBalance(entries); got != 10 {
		t.Fatalf("balance = %d, want 10", got)
	}
	if got := ComputeBalance(nil); got != 0 {
		t.Fatalf("empty balance = %d, want 0", got)
	}
}

func TestComputeLotRemaindersConsumesEarliestExpiryFirst(t *testing.T) {
	lots := []TopUpLot{
		lot("lot_late", 5, 30*24*time.Hour),
		lot("lot_soon", 5, 2*24*time.Hour),
	}
	entries := []CreditLedgerEntry{
		entry("led_1", LedgerTopUp, 5),
		entry("led_2", LedgerTopUp, 5),
		entry("led_3", LedgerVisitDeduction, -3),
	}

	remainders := ComputeLotRemainders(lots, entries)
	if len(remainders) != 2 {
		t.Fatalf("got %d remainders, want 2", len(remainders))
	}
	// FIFO by expiry: the lot expiring sooner is spent first.
	if remainders[0].Lot.ID != "lot_soon" {
		t.Fatalf("first remainder is %s, want lot_soon", remainders[0].Lot.ID)
	}
	if remainders[0].Remaining != 2 {
		t.Fatalf("lot_soon remaining = %d, want 2", remainders[0].Remaining)
	}
	if remainders[1].Remaining != 5 {
		t.Fatalf("lot_late remaining = %d, want 5", remainders[1].Remaining)
	}
}

func TestComputeLotRemaindersGivesCreditsBackOnRefundAndReversal(t *testing.T) {
	lots := []TopUpLot{lot("lot_1", 10, 30*24*time.Hour)}
	entries := []CreditLedgerEntry{
		entry("led_1", LedgerTopUp, 10),
		entry("led_2", LedgerVisitDeduction, -4),
		// A refund and a positive reversal both hand credits back, so the
		// consumption pool shrinks and the lot refills.
		entry("led_3", LedgerRefund, 1),
		entry("led_4", LedgerReversal, 2),
	}

	remainders := ComputeLotRemainders(lots, entries)
	if remainders[0].Remaining != 9 {
		t.Fatalf("remaining = %d, want 9 (10 - 4 + 1 + 2)", remainders[0].Remaining)
	}
}

func TestComputeLotRemaindersPinsExpirationToItsOwnLot(t *testing.T) {
	lots := []TopUpLot{
		lot("lot_expired", 4, -24*time.Hour),
		lot("lot_live", 6, 30*24*time.Hour),
	}
	entries := []CreditLedgerEntry{
		entry("led_1", LedgerTopUp, 4),
		entry("led_2", LedgerTopUp, 6),
		// This EXPIRATION belongs to lot_expired only; it must not be spread
		// across the live lot.
		entry("led_3", LedgerExpiration, -4, withSource(SourceSystem, "lot_expired")),
	}

	remainders := ComputeLotRemainders(lots, entries)
	byID := map[string]int{}
	for _, r := range remainders {
		byID[r.Lot.ID] = r.Remaining
	}
	if byID["lot_expired"] != 0 {
		t.Fatalf("expired lot remaining = %d, want 0", byID["lot_expired"])
	}
	if byID["lot_live"] != 6 {
		t.Fatalf("live lot remaining = %d, want 6", byID["lot_live"])
	}
}

func TestDeriveExpirationEntriesOnlyForPastLotsWithCreditsLeft(t *testing.T) {
	lots := []TopUpLot{
		lot("lot_past", 3, -time.Hour),
		lot("lot_future", 5, 48*time.Hour),
	}
	entries := []CreditLedgerEntry{
		entry("led_1", LedgerTopUp, 3),
		entry("led_2", LedgerTopUp, 5),
	}

	drafts := DeriveExpirationEntries(lots, entries, testNow)
	if len(drafts) != 1 {
		t.Fatalf("got %d drafts, want 1", len(drafts))
	}
	if drafts[0].LotID != "lot_past" || drafts[0].Amount != -3 {
		t.Fatalf("draft = %+v, want lot_past with amount -3", drafts[0])
	}
}

func TestDeriveExpirationEntriesIsIdempotent(t *testing.T) {
	lots := []TopUpLot{lot("lot_past", 3, -time.Hour)}
	entries := []CreditLedgerEntry{entry("led_1", LedgerTopUp, 3)}

	first := DeriveExpirationEntries(lots, entries, testNow)
	if len(first) != 1 {
		t.Fatalf("first sweep produced %d drafts, want 1", len(first))
	}
	// Apply the first sweep, then run again: nothing is left to expire.
	entries = append(entries, entry("led_2", LedgerExpiration, first[0].Amount, withSource(SourceSystem, "lot_past")))
	if second := DeriveExpirationEntries(lots, entries, testNow); len(second) != 0 {
		t.Fatalf("second sweep produced %d drafts, want 0", len(second))
	}
}

func TestComputeExpiringCreditsExcludesAlreadyExpiredAndDistantLots(t *testing.T) {
	lots := []TopUpLot{
		lot("lot_gone", 2, -time.Hour),       // already expired: not "expiring"
		lot("lot_soon", 4, 3*24*time.Hour),   // inside the 7-day horizon
		lot("lot_later", 6, 30*24*time.Hour), // beyond the horizon
	}
	entries := []CreditLedgerEntry{
		entry("led_1", LedgerTopUp, 2),
		entry("led_2", LedgerTopUp, 4),
		entry("led_3", LedgerTopUp, 6),
		entry("led_4", LedgerExpiration, -2, withSource(SourceSystem, "lot_gone")),
	}

	if got := ComputeExpiringCredits(lots, entries, testNow, 7); got != 4 {
		t.Fatalf("expiring = %d, want 4", got)
	}
}

func TestBuildReversalProducesCompensatingEntry(t *testing.T) {
	original := entry("led_1", LedgerTopUp, 10, withSource(SourcePayment, "pay_1"))

	draft, err := BuildReversal(original, "adm_1", "duplicate charge", false)
	if err != nil {
		t.Fatalf("BuildReversal returned %v", err)
	}
	if draft.Amount != -10 {
		t.Fatalf("amount = %d, want -10", draft.Amount)
	}
	if draft.ReversesEntryID != "led_1" {
		t.Fatalf("reverses = %s, want led_1", draft.ReversesEntryID)
	}
	if draft.SourceID == nil || *draft.SourceID != "pay_1" {
		t.Fatalf("source id not carried over: %+v", draft.SourceID)
	}
}

func TestBuildReversalRejectsReversingAReversalOrDoubleReversal(t *testing.T) {
	reversal := entry("led_2", LedgerReversal, -10)
	if _, err := BuildReversal(reversal, "adm_1", "oops", false); err != ErrCannotReverseReversal {
		t.Fatalf("err = %v, want ErrCannotReverseReversal", err)
	}

	original := entry("led_1", LedgerTopUp, 10)
	if _, err := BuildReversal(original, "adm_1", "oops", true); err != ErrAlreadyReversed {
		t.Fatalf("err = %v, want ErrAlreadyReversed", err)
	}
}
