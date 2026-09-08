package domain

import (
	"testing"
	"time"
)

// fixedTime is any instant: these tests care that a signature exists, never
// when it was made.
func fixedTime() time.Time {
	return time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
}

func TestApprovalChainGrowsWithTheAmount(t *testing.T) {
	thresholds := DefaultApprovalThresholds()

	// Every request needs a head's signature, however small: somebody other
	// than the person asking has to have seen it.
	small := RequiredApprovals(250_000, thresholds)
	if len(small) != 1 || small[0] != ApprovalHead {
		t.Fatalf("a small request needs a head only, got %v", small)
	}

	medium := RequiredApprovals(9_000_000, thresholds)
	if len(medium) != 2 || medium[1] != ApprovalFinance {
		t.Fatalf("past the finance threshold needs finance, got %v", medium)
	}

	large := RequiredApprovals(40_000_000, thresholds)
	if len(large) != 3 || large[2] != ApprovalDirector {
		t.Fatalf("a large request needs a director, got %v", large)
	}

	// Exactly on a threshold is below it: the rule is "more than".
	onTheLine := RequiredApprovals(thresholds.FinanceAbove, thresholds)
	if len(onTheLine) != 1 {
		t.Fatalf("exactly at the threshold stays with the head, got %v", onTheLine)
	}
}

func TestSeniorityCanSignForJuniorityButNotTheReverse(t *testing.T) {
	// A director approving what a branch manager could have is normal, and
	// refusing it only creates deadlock when somebody is on leave.
	if !CanApproveAt(RoleSuperAdmin, ApprovalHead) {
		t.Fatal("a super admin must be able to sign at head level")
	}
	if !CanApproveAt(RoleHQAdmin, ApprovalFinance) {
		t.Fatal("an HQ admin must be able to sign at finance level")
	}
	// The reverse never holds.
	if CanApproveAt(RoleBranchManager, ApprovalFinance) {
		t.Fatal("a branch manager must not sign the finance level")
	}
	if CanApproveAt(RoleFinance, ApprovalDirector) {
		t.Fatal("finance must not sign the director level")
	}
	if CanApproveAt(RoleFrontDesk, ApprovalHead) || CanApproveAt(RoleCoach, ApprovalHead) {
		t.Fatal("the counter and the gym floor sign nothing")
	}
}

func TestNextApprovalWalksTheChainInOrder(t *testing.T) {
	thresholds := DefaultApprovalThresholds()
	now := fixedTime()
	request := PurchaseRequest{TotalIDR: 40_000_000}

	level, waiting := NextApproval(request, thresholds)
	if !waiting || level != ApprovalHead {
		t.Fatalf("want to be waiting on the head, got %v/%v", level, waiting)
	}

	request.ApprovedAtHead = &now
	level, waiting = NextApproval(request, thresholds)
	if !waiting || level != ApprovalFinance {
		t.Fatalf("want to be waiting on finance, got %v/%v", level, waiting)
	}

	request.ApprovedAtFinance = &now
	level, waiting = NextApproval(request, thresholds)
	if !waiting || level != ApprovalDirector {
		t.Fatalf("want to be waiting on a director, got %v/%v", level, waiting)
	}

	request.ApprovedAtDirector = &now
	if _, waiting = NextApproval(request, thresholds); waiting {
		t.Fatal("a fully signed request waits on nobody")
	}
}

func TestOrderTotalsTaxWhatIsActuallyPayable(t *testing.T) {
	items := []PurchaseOrderItem{
		{QtyOrdered: 10, UnitPriceIDR: 100_000},                     // 1.000.000
		{QtyOrdered: 4, UnitPriceIDR: 250_000, DiscountIDR: 50_000}, // 950.000
	}

	totals := ComputeOrderTotals(items, 0, 11)
	if totals.SubtotalIDR != 1_950_000 {
		t.Fatalf("want a subtotal of 1.950.000, got %v", totals.SubtotalIDR)
	}
	if totals.TaxIDR != 214_500 {
		t.Fatalf("want 11%% of 1.950.000, got %v", totals.TaxIDR)
	}
	if totals.TotalIDR != 2_164_500 {
		t.Fatalf("want 2.164.500, got %v", totals.TotalIDR)
	}

	// An order-level discount reduces the tax as well as the total, which is
	// both the rule and the only thing that makes a discount worth having.
	discounted := ComputeOrderTotals(items, 950_000, 11)
	if discounted.TaxIDR != 110_000 {
		t.Fatalf("want tax on the discounted 1.000.000, got %v", discounted.TaxIDR)
	}
	if discounted.TotalIDR != 1_110_000 {
		t.Fatalf("want 1.110.000, got %v", discounted.TotalIDR)
	}

	// A discount larger than the order cannot make the total negative.
	silly := ComputeOrderTotals(items, 99_000_000, 11)
	if silly.TotalIDR != 0 {
		t.Fatalf("want 0, got %v", silly.TotalIDR)
	}
}

func TestPartialDeliveryIsAFirstClassState(t *testing.T) {
	// Suppliers short-ship constantly. An order that is 90% delivered is
	// neither open nor done, and pretending otherwise loses the difference.
	partial := ApplyReceipt([]PurchaseOrderItem{
		{QtyOrdered: 10, QtyReceived: 10},
		{QtyOrdered: 5, QtyReceived: 2},
	})
	if partial.Status != POPartiallyReceived || partial.Complete {
		t.Fatalf("want PARTIALLY_RECEIVED, got %v", partial.Status)
	}

	full := ApplyReceipt([]PurchaseOrderItem{
		{QtyOrdered: 10, QtyReceived: 10},
		{QtyOrdered: 5, QtyReceived: 5},
	})
	if full.Status != POReceived || !full.Complete {
		t.Fatalf("want RECEIVED, got %v", full.Status)
	}

	none := ApplyReceipt([]PurchaseOrderItem{{QtyOrdered: 10}})
	if none.Status != POSent {
		t.Fatalf("nothing received leaves the order sent, got %v", none.Status)
	}
}

func TestMoreCannotArriveThanWasOrdered(t *testing.T) {
	order := PurchaseOrder{Status: POSent}
	line := PurchaseOrderItem{QtyOrdered: 10, QtyReceived: 8}

	// Two more is exactly the rest.
	if rejection := EvaluateReceiptLine(order, line, 2, 0); rejection != "" {
		t.Fatalf("the last two must be receivable, got %q", rejection)
	}
	// Three is one more than was ever ordered.
	if rejection := EvaluateReceiptLine(order, line, 3, 0); rejection != ReceiptRejectOverDelivered {
		t.Fatalf("want OVER_DELIVERED, got %q", rejection)
	}
	// A delivery of nothing is not a delivery.
	if rejection := EvaluateReceiptLine(order, line, 0, 0); rejection != ReceiptRejectNothingArrived {
		t.Fatalf("want NOTHING_ARRIVED, got %q", rejection)
	}
	// Rejected goods still count as something having arrived.
	if rejection := EvaluateReceiptLine(order, line, 0, 2); rejection != "" {
		t.Fatalf("a wholly rejected delivery is still a delivery, got %q", rejection)
	}
	// A draft order has not been placed with anybody yet.
	if rejection := EvaluateReceiptLine(PurchaseOrder{Status: PODraft}, line, 1, 0); rejection != ReceiptRejectOrderNotOpen {
		t.Fatalf("want ORDER_NOT_OPEN, got %q", rejection)
	}
}

func TestQCStatusIsReadOffTheQuantities(t *testing.T) {
	// The label can never disagree with the numbers beside it.
	if got := DeriveQCStatus(10, 0); got != QCAccepted {
		t.Fatalf("want ACCEPTED, got %q", got)
	}
	if got := DeriveQCStatus(0, 10); got != QCRejected {
		t.Fatalf("want REJECTED, got %q", got)
	}
	if got := DeriveQCStatus(7, 3); got != QCPartiallyRejected {
		t.Fatalf("want PARTIALLY_REJECTED, got %q", got)
	}
}

func TestAReceivedOrderCannotBeCancelled(t *testing.T) {
	// Stock has moved. Cancelling would leave the stock ledger pointing at a
	// document claiming nothing was ever ordered.
	if _, err := Transition(PurchaseOrderTransitions, POPartiallyReceived, POCancelled); err == nil {
		t.Fatal("a partly received order cannot be cancelled")
	}
	if _, err := Transition(PurchaseOrderTransitions, POReceived, POCancelled); err == nil {
		t.Fatal("a received order cannot be cancelled")
	}
	// Before anything arrives it can be.
	if _, err := Transition(PurchaseOrderTransitions, POSent, POCancelled); err != nil {
		t.Fatalf("a sent order with no deliveries can be cancelled: %v", err)
	}
}

func TestBlockedSuppliersCannotBeOrderedFrom(t *testing.T) {
	if !SupplierActive.CanOrderFrom() || !SupplierProbation.CanOrderFrom() {
		t.Fatal("active and probation suppliers accept orders")
	}
	for _, status := range []SupplierStatus{SupplierBlocked, SupplierInactive, SupplierDraft} {
		if status.CanOrderFrom() {
			t.Fatalf("%s must not accept orders", status)
		}
	}
}

func TestPaymentTermsDecideTheDueDate(t *testing.T) {
	delivered := Date("2026-03-02")
	if got := TermsNet30.DueDate(delivered); got != "2026-04-01" {
		t.Fatalf("want 2026-04-01, got %v", got)
	}
	if got := TermsCOD.DueDate(delivered); got != delivered {
		t.Fatalf("cash on delivery is due on delivery, got %v", got)
	}
}

func TestReturnableIsWhatWasAcceptedAndNotYetSentBack(t *testing.T) {
	line := GoodsReceiptItem{QtyAccepted: 10, QtyRejected: 2, QtyReturned: 4}
	// Rejected stock never entered inventory, so it is not returnable — it was
	// never taken in to begin with.
	if got := line.QtyReturnable(); got != 6 {
		t.Fatalf("want 6 returnable, got %v", got)
	}
}
