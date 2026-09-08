package domain

import "testing"

func stockItem() InventoryItem {
	return InventoryItem{
		ID: "itm_1", SKU: "SKU-1", Name: "Protein bar", Unit: "PCS",
		Kind: ItemRetail, UnitCostIDR: 10_000, TrackStock: true, Active: true,
	}
}

func levelWith(onHand, onOrder, minimum Quantity) StockLevel {
	return StockLevel{
		ItemID: "itm_1", BranchID: "brn_senopati",
		QtyOnHand: onHand, QtyOnOrder: onOrder, QtyMinimum: minimum,
	}
}

func TestStockCannotGoNegative(t *testing.T) {
	item := stockItem()
	level := levelWith(5, 0, 0)

	// Taking out more than is there is refused rather than clamped: a till
	// that thinks it sold six of five has a problem the count cannot hide.
	posted := PostMovement(item, level, MovementOut, -6, 0)
	if posted.Allowed() {
		t.Fatal("selling more than is on hand must be refused")
	}
	if posted.Rejection != StockRejectInsufficient {
		t.Fatalf("want INSUFFICIENT_STOCK, got %q", posted.Rejection)
	}

	// Taking out exactly what is there is fine, and lands on zero.
	posted = PostMovement(item, level, MovementOut, -5, 0)
	if !posted.Allowed() {
		t.Fatalf("selling the last unit must be allowed, got %q", posted.Rejection)
	}
	if posted.QtyAfter != 0 {
		t.Fatalf("want 0 left, got %v", posted.QtyAfter)
	}
}

func TestMovementArithmeticAlwaysHolds(t *testing.T) {
	item := stockItem()
	level := levelWith(12, 0, 0)

	for _, qty := range []Quantity{1, -1, 7.5, -7.5, 0.001} {
		posted := PostMovement(item, level, MovementAdjustment, qty, 0)
		if !posted.Allowed() {
			t.Fatalf("qty %v: %q", qty, posted.Rejection)
		}
		// The same invariant the database CHECK enforces on the row.
		if posted.QtyAfter != RoundQuantity(posted.QtyBefore+posted.Qty) {
			t.Fatalf("qty %v: after (%v) != before (%v) + qty (%v)",
				qty, posted.QtyAfter, posted.QtyBefore, posted.Qty)
		}
	}
}

func TestAZeroMovementIsNotAMovement(t *testing.T) {
	posted := PostMovement(stockItem(), levelWith(3, 0, 0), MovementAdjustment, 0, 0)
	if posted.Allowed() || posted.Rejection != StockRejectZeroQty {
		t.Fatalf("want ZERO_QUANTITY, got %q", posted.Rejection)
	}
}

func TestUntrackedAndInactiveItemsHoldNoStock(t *testing.T) {
	untracked := stockItem()
	untracked.TrackStock = false
	if posted := PostMovement(untracked, levelWith(0, 0, 0), MovementIn, 5, 0); posted.Rejection != StockRejectNotTracked {
		t.Fatalf("want NOT_TRACKED, got %q", posted.Rejection)
	}

	retired := stockItem()
	retired.Active = false
	if posted := PostMovement(retired, levelWith(0, 0, 0), MovementIn, 5, 0); posted.Rejection != StockRejectInactive {
		t.Fatalf("want ITEM_INACTIVE, got %q", posted.Rejection)
	}
}

func TestReceivingMovesTheAverageCostAndIssuingDoesNot(t *testing.T) {
	item := stockItem() // 10 units' worth at Rp10.000 each
	level := levelWith(10, 0, 0)

	// Ten at 10.000 plus ten at 20.000 is twenty at 15.000.
	posted := PostMovement(item, level, MovementIn, 10, 20_000)
	if !posted.Allowed() {
		t.Fatalf("receipt refused: %q", posted.Rejection)
	}
	if posted.UnitCostAfter != 15_000 {
		t.Fatalf("want an average of 15.000, got %v", posted.UnitCostAfter)
	}
	if posted.TotalCostIDR != 200_000 {
		t.Fatalf("want the receipt valued at 200.000, got %v", posted.TotalCostIDR)
	}

	// Selling values the stock at what it cost, and leaves the average alone.
	posted = PostMovement(item, level, MovementOut, -4, 0)
	if posted.UnitCostAfter != 10_000 {
		t.Fatalf("issuing must not move the average, got %v", posted.UnitCostAfter)
	}
	if posted.TotalCostIDR != 40_000 {
		t.Fatalf("want 4 x 10.000 = 40.000, got %v", posted.TotalCostIDR)
	}
}

func TestWeightedAverageOnAnEmptyShelfIsWhatArrived(t *testing.T) {
	// Nothing on hand means the incoming price is the only information there
	// is; averaging against a cost of zero would halve it for no reason.
	if got := WeightedAverageCost(0, 0, 5, 12_500); got != 12_500 {
		t.Fatalf("want 12.500, got %v", got)
	}
}

func TestLowStockCountsWhatIsAlreadyOnItsWay(t *testing.T) {
	// Three left with a minimum of five is low.
	if !IsLowStock(levelWith(3, 0, 5)) {
		t.Fatal("three against a minimum of five is low")
	}
	// Three left but ten already ordered is not: raising a second purchase
	// order is exactly the mistake this prevents.
	if IsLowStock(levelWith(3, 10, 5)) {
		t.Fatal("stock already on order must count")
	}
}

func TestReorderQuantityFillsToTheMaximum(t *testing.T) {
	max := Quantity(50)
	level := levelWith(4, 6, 10)
	level.QtyMaximum = &max

	// 4 on hand + 6 on order = 10; filling to 50 needs 40.
	if got := ReorderQuantity(level); got != 40 {
		t.Fatalf("want 40, got %v", got)
	}

	// With no maximum the target is the minimum.
	level.QtyMaximum = nil
	if got := ReorderQuantity(level); got != 0 {
		t.Fatalf("already at the minimum, want 0, got %v", got)
	}

	// Never negative, however overstocked.
	if got := ReorderQuantity(levelWith(100, 0, 10)); got != 0 {
		t.Fatalf("want 0, got %v", got)
	}
}

func TestStockTakeVarianceIsValuedInMoney(t *testing.T) {
	lines := []StockTakeLine{
		{ItemID: "itm_1", QtyExpected: 10, QtyCounted: 8}, // two short
		{ItemID: "itm_2", QtyExpected: 5, QtyCounted: 7},  // two over
		{ItemID: "itm_3", QtyExpected: 3, QtyCounted: 3},  // exact
	}
	costs := map[string]float64{"itm_1": 10_000, "itm_2": 2_500, "itm_3": 999}

	summary := SummarizeStockTake(lines, costs)
	if summary.Lines != 3 || summary.LinesVaried != 2 {
		t.Fatalf("want 3 lines with 2 varied, got %d/%d", summary.Lines, summary.LinesVaried)
	}
	if summary.QtyShort != 2 || summary.QtyOver != 2 {
		t.Fatalf("want 2 short and 2 over, got %v/%v", summary.QtyShort, summary.QtyOver)
	}
	// Two short at 10.000 is -20.000; two over at 2.500 is +5.000.
	if summary.ValueVariance != -15_000 {
		t.Fatalf("want a variance of -15.000, got %v", summary.ValueVariance)
	}
}

func TestAnAppliedStockTakeIsFinal(t *testing.T) {
	if _, err := Transition(StockTakeTransitions, StockTakeDraft, StockTakeApplied); err != nil {
		t.Fatalf("a draft count must be appliable: %v", err)
	}
	// It has already written adjustment movements, and those are append-only.
	if _, err := Transition(StockTakeTransitions, StockTakeApplied, StockTakeCancelled); err == nil {
		t.Fatal("an applied count cannot be cancelled")
	}
}

func TestQuantitiesRoundToWhatTheColumnStores(t *testing.T) {
	// NUMERIC(15,3): Go and PostgreSQL must agree on the same three decimals.
	if got := RoundQuantity(0.1 + 0.2); got != 0.3 {
		t.Fatalf("want 0.3, got %v", got)
	}
	if got := RoundQuantity(1.23456); got != 1.235 {
		t.Fatalf("want 1.235, got %v", got)
	}
}
