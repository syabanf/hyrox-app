package domain

import "testing"

func soldLine(qty Quantity, price, cost, taxPercent float64) POSOrderItem {
	return POSOrderItem{
		Qty: qty, UnitPriceIDR: price, UnitCostIDR: cost, TaxPercent: taxPercent,
		LineTotalIDR: float64(qty) * price,
	}
}

func TestASaleAddsUpAndKnowsWhatItMade(t *testing.T) {
	items := []POSOrderItem{
		soldLine(2, 85_000, 62_000, 0), // two tees
		soldLine(3, 18_000, 12_000, 0), // three bars
	}

	totals := ComputeOrderPOSTotals(items, 0, 0)
	if totals.SubtotalIDR != 224_000 {
		t.Fatalf("want a subtotal of 224.000, got %v", totals.SubtotalIDR)
	}
	// Cost is frozen from the line, so the margin is knowable at the till.
	if totals.CostIDR != 160_000 {
		t.Fatalf("want a cost of 160.000, got %v", totals.CostIDR)
	}
	if totals.GrossProfitIDR != 64_000 {
		t.Fatalf("want 64.000 gross, got %v", totals.GrossProfitIDR)
	}
}

func TestTaxIsChargedOnWhatIsActuallyPaid(t *testing.T) {
	// One taxed line and one untaxed, with a discount across both.
	items := []POSOrderItem{
		soldLine(1, 100_000, 0, 11),
		soldLine(1, 100_000, 0, 0),
	}

	// No discount: tax is 11% of the taxed line only.
	plain := ComputeOrderPOSTotals(items, 0, 0)
	if plain.TaxIDR != 11_000 {
		t.Fatalf("want 11.000 tax, got %v", plain.TaxIDR)
	}

	// A 50.000 discount is spread in proportion, so the taxed line's share is
	// 25.000 and it is taxed on 75.000. Doing it the other way round
	// overcharges the customer and the tax return in one stroke.
	discounted := ComputeOrderPOSTotals(items, 50_000, 0)
	if discounted.TaxIDR != 8_250 {
		t.Fatalf("want 8.250 tax on the discounted line, got %v", discounted.TaxIDR)
	}
	if discounted.TotalIDR != 158_250 {
		t.Fatalf("want 158.250, got %v", discounted.TotalIDR)
	}
}

func TestADiscountCannotMakeASaleNegative(t *testing.T) {
	items := []POSOrderItem{soldLine(1, 50_000, 20_000, 0)}
	totals := ComputeOrderPOSTotals(items, 999_000, 0)
	if totals.TotalIDR != 0 || totals.DiscountIDR != 50_000 {
		t.Fatalf("want a zero total and a capped discount, got %v/%v",
			totals.TotalIDR, totals.DiscountIDR)
	}
}

func TestChangeComesOutOfCashAndNothingElse(t *testing.T) {
	total := 100_000.0

	// Cash overpaid gives change.
	cash := Settle(total, []POSPayment{{Method: PayCash, AmountIDR: 150_000}})
	if !cash.Settled || cash.ChangeIDR != 50_000 {
		t.Fatalf("want 50.000 change, got %v (settled %v)", cash.ChangeIDR, cash.Settled)
	}

	// A split where the card covers most and cash overpays the rest: change
	// can only come from the cash actually handed over.
	split := Settle(total, []POSPayment{
		{Method: PayQRIS, AmountIDR: 90_000},
		{Method: PayCash, AmountIDR: 20_000},
	})
	if !split.Settled || split.ChangeIDR != 10_000 {
		t.Fatalf("want 10.000 change, got %v", split.ChangeIDR)
	}

	// A card alone that somehow overpaid gives no change: there is nothing in
	// the drawer to give back.
	card := Settle(total, []POSPayment{{Method: PayQRIS, AmountIDR: 120_000}})
	if card.ChangeIDR != 0 {
		t.Fatalf("a card gives no change, got %v", card.ChangeIDR)
	}
}

func TestPartialPaymentIsItsOwnState(t *testing.T) {
	outcome := Settle(100_000, []POSPayment{{Method: PayCash, AmountIDR: 40_000}})
	if outcome.PaymentStatus != POSPartial || outcome.Settled {
		t.Fatalf("want PARTIAL and unsettled, got %v/%v", outcome.PaymentStatus, outcome.Settled)
	}
	if none := Settle(100_000, nil); none.PaymentStatus != POSUnpaid {
		t.Fatalf("want UNPAID, got %v", none.PaymentStatus)
	}
}

func TestACardCannotOverpay(t *testing.T) {
	order := POSOrder{Status: POSOpen, TotalIDR: 100_000}

	// There is no change to give, so the difference would simply be lost.
	if got := EvaluateTender(order, PayQRIS, 120_000, 0); got != POSRejectOverTendered {
		t.Fatalf("want OVER_TENDERED, got %q", got)
	}
	// Cash may, because the difference comes back as change.
	if got := EvaluateTender(order, PayCash, 120_000, 0); got != "" {
		t.Fatalf("cash may overpay, got %q", got)
	}
	// And a second card tender cannot take the total past what is owed.
	if got := EvaluateTender(order, PayDebit, 60_000, 50_000); got != POSRejectOverTendered {
		t.Fatalf("want OVER_TENDERED, got %q", got)
	}
	if got := EvaluateTender(order, PayDebit, 50_000, 50_000); got != "" {
		t.Fatalf("exactly the remainder is fine, got %q", got)
	}
}

func TestAClosedOrderTakesNoMoreMoney(t *testing.T) {
	for _, status := range []POSOrderStatus{POSCompleted, POSCancelled, POSVoided} {
		order := POSOrder{Status: status, TotalIDR: 50_000}
		if got := EvaluateTender(order, PayCash, 50_000, 0); got != POSRejectOrderClosed {
			t.Fatalf("%s: want ORDER_NOT_OPEN, got %q", status, got)
		}
	}
}

func TestCancellingAndVoidingAreDifferentActs(t *testing.T) {
	// An open order is cancelled; a completed one is voided. Conflating them
	// would let the counter erase a paid sale by calling it a cancellation.
	if _, err := Transition(POSOrderTransitions, POSOpen, POSCancelled); err != nil {
		t.Fatalf("an open order can be cancelled: %v", err)
	}
	if _, err := Transition(POSOrderTransitions, POSOpen, POSVoided); err == nil {
		t.Fatal("an unpaid order is cancelled, not voided")
	}
	if _, err := Transition(POSOrderTransitions, POSCompleted, POSCancelled); err == nil {
		t.Fatal("a completed sale cannot be cancelled")
	}
	if _, err := Transition(POSOrderTransitions, POSCompleted, POSVoided); err != nil {
		t.Fatalf("a completed sale can be voided: %v", err)
	}
	if _, err := Transition(POSOrderTransitions, POSVoided, POSCompleted); err == nil {
		t.Fatal("a voided sale is final")
	}
}

func TestTheDrawerOnlyCountsCash(t *testing.T) {
	payments := []POSPayment{
		{Method: PayCash, AmountIDR: 200_000, ChangeIDR: 50_000},
		{Method: PayQRIS, AmountIDR: 300_000},
		{Method: PayCash, AmountIDR: 100_000},
	}
	// 500.000 opening + (200.000 - 50.000) + 100.000. The card sale never
	// touched the drawer, and counting it would make every till look short by
	// exactly the card takings.
	if got := ExpectedCash(500_000, payments); got != 750_000 {
		t.Fatalf("want 750.000 in the drawer, got %v", got)
	}
}

func TestATierDiscountIsAPercentageOfTheSubtotal(t *testing.T) {
	if got := TierDiscount(240_000, 10); got != 24_000 {
		t.Fatalf("want 24.000, got %v", got)
	}
	if got := TierDiscount(240_000, 0); got != 0 {
		t.Fatalf("no tier discount is nothing, got %v", got)
	}
}

func TestOnlyAvailableProductsAreSellable(t *testing.T) {
	product := POSProduct{Active: true, Available: true}
	if !product.Sellable() {
		t.Fatal("an active, available product sells")
	}
	// Off the menu today without being retired.
	product.Available = false
	if product.Sellable() {
		t.Fatal("an unavailable product does not sell")
	}
	product.Available, product.Active = true, false
	if product.Sellable() {
		t.Fatal("a retired product does not sell")
	}
}
