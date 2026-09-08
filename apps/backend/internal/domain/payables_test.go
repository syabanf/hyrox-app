package domain

import "testing"

func TestAnInstalmentScheduleAbsorbsItsRoundingAtTheEnd(t *testing.T) {
	// A million split three ways rounds to 333.333.33 each, which is a rupiah
	// short. A payables report that is a rupiah out is one somebody stops
	// trusting, so the last instalment carries the difference.
	third := 100.0 / 3
	terms := Rebalance([]PaymentTerm{
		{Sequence: 1, Percent: &third},
		{Sequence: 2, Percent: &third},
		{Sequence: 3, Percent: &third},
	}, 1_000_000)

	var total float64
	for _, term := range terms {
		total += term.AmountIDR
	}
	if total != 1_000_000 {
		t.Fatalf("instalments must add to the total, got %v", total)
	}
	if terms[2].AmountIDR == terms[0].AmountIDR {
		t.Fatalf("the last instalment absorbs the rounding, got %v", terms[2].AmountIDR)
	}
}

func TestACancelledInstalmentIsNotRebalancedInto(t *testing.T) {
	half := 50.0
	terms := Rebalance([]PaymentTerm{
		{Sequence: 1, Percent: &half},
		{Sequence: 2, Percent: &half, Status: TermCancelled, AmountIDR: 500_000},
		{Sequence: 3, Percent: &half},
	}, 1_000_000)

	if terms[1].AmountIDR != 500_000 {
		t.Fatalf("a cancelled instalment is left alone, got %v", terms[1].AmountIDR)
	}
	if terms[0].AmountIDR+terms[2].AmountIDR != 1_000_000 {
		t.Fatalf("the live instalments carry the whole total, got %v and %v",
			terms[0].AmountIDR, terms[2].AmountIDR)
	}
}

func TestOverpayingAnInstalmentIsReportedRatherThanAbsorbed(t *testing.T) {
	term := PaymentTerm{AmountIDR: 4_000_000, Status: TermPending}

	part := PostToTerm(term, 1_500_000)
	if part.Status != TermPartial || part.OverpaidBy != 0 {
		t.Fatalf("a part payment is partial, got %+v", part)
	}

	// Paying more than is owed is either a mistake or a payment against
	// something else. Swallowing the difference loses whichever it was.
	over := PostToTerm(term, 5_000_000)
	if over.Status != TermPaid || over.OverpaidBy != 1_000_000 {
		t.Fatalf("an overpayment is reported, got %+v", over)
	}
}

func TestACreditNoteSettlesTheDebtWithoutMovingMoney(t *testing.T) {
	scheduled := 100.0
	terms := []PaymentTerm{
		{AmountIDR: 4_000_000, PaidIDR: 1_000_000, Status: TermPartial,
			DueOn: "2026-09-01", Percent: &scheduled},
	}

	// A payables list that still shows a credited debt sends somebody to pay
	// twice.
	position := PayablesFor(4_000_000, terms, 500_000, "2026-09-08")
	if position.OutstandingIDR != 2_500_000 {
		t.Fatalf("4m less 1m paid less 500k credited is 2.5m, got %v", position.OutstandingIDR)
	}
	// The instalment's own day has passed, so what it still needs is overdue.
	if position.OverdueIDR != 3_000_000 {
		t.Fatalf("the instalment still needs 3m and its day has gone, got %v", position.OverdueIDR)
	}
	if position.NextDue == nil || *position.NextDue != "2026-09-01" {
		t.Fatalf("the soonest unpaid instalment is the next due, got %v", position.NextDue)
	}
}

func TestCreditsAreSpentOldestFirstAndNeverAfterTheyExpire(t *testing.T) {
	credits := []VendorCredit{
		{ID: "new", CreditNumber: "CN-2", IssuedOn: "2026-08-01", AmountIDR: 1_000_000, Status: CreditOpen},
		{ID: "old", CreditNumber: "CN-1", IssuedOn: "2026-06-01", AmountIDR: 400_000, Status: CreditOpen},
		{ID: "lapsed", CreditNumber: "CN-0", IssuedOn: "2026-01-01", AmountIDR: 900_000,
			Status: CreditOpen, ExpiresOn: dateOf("2026-08-31")},
	}

	allocations, shortfall := AllocateCredits(credits, 900_000, "2026-09-08")
	if shortfall != 0 {
		t.Fatalf("1.4m of usable credit covers 900k, short by %v", shortfall)
	}
	// Oldest usable first: spending the newest leaves the old to lapse, which
	// is the same as throwing the money away.
	if allocations[0].CreditID != "old" || allocations[0].AmountIDR != 400_000 {
		t.Fatalf("the oldest usable credit goes first, got %+v", allocations[0])
	}
	if allocations[1].CreditID != "new" || allocations[1].AmountIDR != 500_000 {
		t.Fatalf("the balance comes off the next one, got %+v", allocations[1])
	}
	for _, allocation := range allocations {
		if allocation.CreditID == "lapsed" {
			t.Fatal("an expired credit cannot be spent")
		}
	}

	// More than there is: the shortfall is what still has to be paid in money.
	_, short := AllocateCredits(credits, 2_000_000, "2026-09-08")
	if short != 600_000 {
		t.Fatalf("1.4m of credit against 2m leaves 600k to pay, got %v", short)
	}
}

func TestACreditCannotBeSpentPastItsFace(t *testing.T) {
	credit := VendorCredit{AmountIDR: 400_000, AppliedIDR: 400_000, Status: CreditApplied}
	if credit.RemainingIDR() != 0 {
		t.Fatalf("a fully applied credit has nothing left, got %v", credit.RemainingIDR())
	}
	allocations, shortfall := AllocateCredits([]VendorCredit{credit}, 100_000, "2026-09-08")
	if len(allocations) != 0 || shortfall != 100_000 {
		t.Fatalf("a spent credit pays for nothing, got %v and %v", allocations, shortfall)
	}
}

func TestADeliveryIsCountedAgainstTheOrderNotAgainstWhatWasAccepted(t *testing.T) {
	order := PurchaseOrder{Status: POSent}
	line := PurchaseOrderItem{QtyOrdered: 100}

	if got := EvaluateDeliveryLine(order, line, 0, 60); got != "" {
		t.Fatalf("sixty of a hundred is fine, got %q", got)
	}
	// A supplier who delivered ten and had nine accepted still delivered ten.
	// The tenth is a rejection, not room for another delivery.
	if got := EvaluateDeliveryLine(order, line, 60, 50); got != DeliveryRejectOverOrdered {
		t.Fatalf("110 of 100 must be refused, got %q", got)
	}
	if got := EvaluateDeliveryLine(PurchaseOrder{Status: PODraft}, line, 0, 10); got != DeliveryRejectClosed {
		t.Fatalf("nothing arrives against a draft order, got %q", got)
	}
}

func TestInspectionCountsRejectionsAsDecided(t *testing.T) {
	// Goods that failed inspection have been dealt with. Only goods nobody has
	// looked at are still outstanding, and treating a rejection as unfinished
	// leaves a delivery that can never close.
	outcome := InspectDelivery(100, 70, 30)
	if !outcome.Complete || outcome.Outstanding != 0 {
		t.Fatalf("70 accepted and 30 rejected is all 100 judged, got %+v", outcome)
	}

	partial := InspectDelivery(100, 40, 10)
	if partial.Complete || partial.Outstanding != 50 {
		t.Fatalf("fifty are still unlooked-at, got %+v", partial)
	}
}

func TestARejectedReturnGoesBackToDraft(t *testing.T) {
	// The usual outcome of "no, not that one" is a corrected return rather
	// than an abandoned one.
	if _, err := Transition(PurchaseReturnTransitions, ReturnRejected, ReturnDraft); err != nil {
		t.Fatalf("a rejected return can be corrected: %v", err)
	}
	// And nothing leaves the building without a signature.
	if _, err := Transition(PurchaseReturnTransitions, ReturnDraft, ReturnPosted); err == nil {
		t.Fatal("a draft return must not post without approval")
	}
	if _, err := Transition(PurchaseReturnTransitions, ReturnApproved, ReturnPosted); err != nil {
		t.Fatalf("an approved return posts: %v", err)
	}
}

func dateOf(v string) *Date {
	d := Date(v)
	return &d
}
