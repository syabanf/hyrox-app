package domain

import (
	"testing"
	"time"
)

func batch(id string, expires string, qty Quantity, cost float64, received string) Batch {
	b := Batch{ID: id, ItemID: "itm", BranchID: "brn", BatchCode: id, QtyOnHand: qty, UnitCostIDR: cost}
	if expires != "" {
		d := Date(expires)
		b.ExpiresOn = &d
	}
	if received != "" {
		b.ReceivedOn, _ = time.Parse("2006-01-02", received)
	}
	return b
}

func TestFEFOTakesTheSoonestDateFirstNotTheOldestDelivery(t *testing.T) {
	// The whole point. The second delivery arrived later but expires sooner,
	// and FIFO would leave it at the back of the shelf until it is worthless.
	batches := []Batch{
		batch("old", "2026-12-01", 30, 10_000, "2026-08-01"),
		batch("new", "2026-09-20", 20, 10_000, "2026-09-01"),
	}

	outcome := AllocateFEFO(batches, 25, "2026-09-08")
	if !outcome.Allocated() {
		t.Fatalf("25 of 50 should allocate, short by %v", outcome.Shortfall)
	}
	if len(outcome.Allocations) != 2 {
		t.Fatalf("25 spans both batches, got %d", len(outcome.Allocations))
	}
	if outcome.Allocations[0].BatchID != "new" || outcome.Allocations[0].Qty != -20 {
		t.Fatalf("the short-dated batch empties first, got %+v", outcome.Allocations[0])
	}
	if outcome.Allocations[1].BatchID != "old" || outcome.Allocations[1].Qty != -5 {
		t.Fatalf("the balance comes off the later batch, got %+v", outcome.Allocations[1])
	}
}

func TestAnUndatedBatchGoesLast(t *testing.T) {
	// A batch with no date is not urgent. Putting it first would leave dated
	// stock behind it to rot.
	batches := []Batch{
		batch("undated", "", 100, 10_000, "2026-01-01"),
		batch("dated", "2027-01-01", 10, 10_000, "2026-09-01"),
	}
	outcome := AllocateFEFO(batches, 5, "2026-09-08")
	if outcome.Allocations[0].BatchID != "dated" {
		t.Fatalf("dated stock leaves before undated, got %+v", outcome.Allocations[0])
	}
}

func TestExpiredStockIsNeverAllocated(t *testing.T) {
	// Selling stock that is past its date is the outcome this whole mechanism
	// exists to prevent. It is reported as a shortfall — not quietly included,
	// and not silently ignored either.
	batches := []Batch{
		batch("gone", "2026-09-01", 40, 10_000, "2026-06-01"),
		batch("good", "2027-01-01", 5, 10_000, "2026-08-01"),
	}

	outcome := AllocateFEFO(batches, 10, "2026-09-08")
	if outcome.Allocated() {
		t.Fatal("only 5 unexpired are available, so 10 cannot be allocated")
	}
	if outcome.Shortfall != 5 {
		t.Fatalf("short by 5, got %v", outcome.Shortfall)
	}
	if len(outcome.Expired) != 1 || outcome.Expired[0] != "gone" {
		t.Fatalf("the expired batch should be named, got %v", outcome.Expired)
	}
	// A batch expiring today is still good: the date is the last day, not the
	// first day after.
	today := AllocateFEFO([]Batch{batch("today", "2026-09-08", 5, 10_000, "2026-08-01")}, 5, "2026-09-08")
	if !today.Allocated() {
		t.Fatal("a batch expiring today may still be sold")
	}
}

func TestAllocationCostsWhatTheBatchCost(t *testing.T) {
	// Two deliveries at two prices. What left is worth what those particular
	// goods cost, not what the item averages — which is what makes a write-off
	// a number somebody can put in a ledger.
	batches := []Batch{
		batch("cheap", "2026-10-01", 10, 16_000, "2026-08-01"),
		batch("dear", "2026-11-01", 10, 20_000, "2026-08-15"),
	}
	outcome := AllocateFEFO(batches, 15, "2026-09-08")
	if outcome.CostIDR != 10*16_000+5*20_000 {
		t.Fatalf("10 cheap and 5 dear is 260.000, got %v", outcome.CostIDR)
	}
}

func TestExpiryStateUsesTheItemsOwnWarningWindow(t *testing.T) {
	b := batch("b", "2026-10-01", 10, 10_000, "2026-08-01")
	const today Date = "2026-09-08"

	// Twenty-three days out. Near for a 30-day window, fresh for a 7-day one:
	// one number for the whole catalogue would be wrong for most of it.
	if got := b.StateOn(today, 30); got != ExpiryNear {
		t.Fatalf("23 days out is near on a 30-day window, got %q", got)
	}
	if got := b.StateOn(today, 7); got != ExpiryFresh {
		t.Fatalf("23 days out is fresh on a 7-day window, got %q", got)
	}
	if got := (batch("n", "", 1, 0, "")).StateOn(today, 30); got != ExpiryNone {
		t.Fatalf("a batch with no date never expires, got %q", got)
	}
	if got := (batch("x", "2026-09-07", 1, 0, "")).StateOn(today, 30); got != ExpiryExpired {
		t.Fatalf("yesterday is expired, got %q", got)
	}
}

func TestExpirySummaryValuesWhatIsAboutToBeLost(t *testing.T) {
	batches := []Batch{
		batch("near", "2026-09-20", 10, 16_000, ""),
		batch("gone", "2026-09-01", 4, 16_000, ""),
		batch("fine", "2027-06-01", 50, 16_000, ""),
		batch("empty", "2026-09-10", 0, 16_000, ""),
	}
	summary := SummarizeExpiry(batches, "2026-09-08", map[string]int{"itm": 30})

	if summary.NearBatches != 1 || summary.NearValueIDR != 160_000 {
		t.Fatalf("one near batch worth 160.000, got %+v", summary)
	}
	if summary.ExpiredBatches != 1 || summary.ExpiredValueIDR != 64_000 {
		t.Fatalf("one expired batch worth 64.000, got %+v", summary)
	}
	// An emptied batch is kept for a recall but is worth nothing and warns
	// about nothing.
	if summary.NearQty != 10 {
		t.Fatalf("an empty batch is not a warning, got %v", summary.NearQty)
	}
}
