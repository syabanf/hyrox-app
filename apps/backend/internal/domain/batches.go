package domain

import (
	"math"
	"sort"
	"time"
)

// Batches: stock that goes off, and the order it has to leave in.
//
// The rule is FEFO — first expired, first out — and not FIFO. Goods that
// arrived later can easily expire sooner, so issuing in arrival order leaves
// the short-dated stock at the back of the shelf until it is worthless. This
// is the single decision that separates a shop that marks stock down from one
// that throws it away.

// Batch is one delivery of one item into one branch, with a date on it.
type Batch struct {
	ID            string    `json:"id"`
	ItemID        string    `json:"itemId"`
	BranchID      string    `json:"branchId"`
	BatchCode     string    `json:"batchCode"`
	ExpiresOn     *Date     `json:"expiresOn"`
	QtyOnHand     Quantity  `json:"qtyOnHand"`
	UnitCostIDR   float64   `json:"unitCostIdr"`
	ReceivedOn    time.Time `json:"receivedOn"`
	ReceiptID     *string   `json:"receiptId"`
	ReceiptNumber *string   `json:"receiptNumber"`
	Note          *string   `json:"note"`
}

// ExpiryState is how close a batch is to being worthless.
type ExpiryState string

const (
	// ExpiryNone is a batch with no date: it never expires.
	ExpiryNone ExpiryState = "NONE"
	// ExpiryFresh is comfortably inside its life.
	ExpiryFresh ExpiryState = "FRESH"
	// ExpiryNear is inside the warning window: sell it, discount it, move it.
	ExpiryNear ExpiryState = "NEAR"
	// ExpiryExpired is past its date and may not be sold at all.
	ExpiryExpired ExpiryState = "EXPIRED"
)

// StateOn reports where a batch stands on a given day.
//
// The date is passed in rather than read from the clock so the answer is
// testable and so a report can ask "what will be near expiry on Monday".
func (b Batch) StateOn(today Date, warningDays int) ExpiryState {
	if b.ExpiresOn == nil {
		return ExpiryNone
	}
	days := DaysBetween(today, *b.ExpiresOn)
	switch {
	case days < 0:
		return ExpiryExpired
	case days <= warningDays:
		return ExpiryNear
	default:
		return ExpiryFresh
	}
}

// DaysUntilExpiry is negative once a batch is past its date.
func (b Batch) DaysUntilExpiry(today Date) *int {
	if b.ExpiresOn == nil {
		return nil
	}
	days := DaysBetween(today, *b.ExpiresOn)
	return &days
}

// Allocation is one batch's share of a movement.
type Allocation struct {
	BatchID   string   `json:"batchId"`
	BatchCode string   `json:"batchCode"`
	ExpiresOn *Date    `json:"expiresOn"`
	Qty       Quantity `json:"qty"`
	QtyBefore Quantity `json:"qtyBefore"`
	QtyAfter  Quantity `json:"qtyAfter"`
	// UnitCostIDR is what this batch cost, which is what makes a write-off
	// worth a number rather than a count.
	UnitCostIDR float64 `json:"unitCostIdr"`
}

// AllocationOutcome is how a requested quantity was spread across batches.
type AllocationOutcome struct {
	Allocations []Allocation
	// Shortfall is what could not be found in any batch. Non-zero means the
	// batches and the stock level disagree, which is a bug rather than an
	// ordinary refusal — the level is checked first.
	Shortfall Quantity
	// CostIDR is what the allocated goods actually cost, batch by batch.
	CostIDR float64
	// Expired lists batches that were skipped because they are past their
	// date. They are reported rather than silently consumed.
	Expired []string
}

// Allocated reports whether the whole quantity was found.
func (o AllocationOutcome) Allocated() bool { return o.Shortfall == 0 }

// AllocateFEFO decides which batches a quantity comes out of.
//
// Soonest expiry first, undated last — a batch with no date is not urgent, and
// putting it first would leave dated stock to rot behind it. Expired batches
// are skipped entirely: selling stock that is past its date is the outcome
// this whole mechanism exists to prevent, and quietly including it would make
// the shortfall look like a stock error instead of a shelf full of goods
// somebody has to throw away.
func AllocateFEFO(batches []Batch, qty Quantity, today Date) AllocationOutcome {
	qty = RoundQuantity(qty)
	if qty <= 0 {
		return AllocationOutcome{}
	}

	usable := make([]Batch, 0, len(batches))
	outcome := AllocationOutcome{}
	for _, batch := range batches {
		if batch.QtyOnHand <= 0 {
			continue
		}
		if batch.StateOn(today, 0) == ExpiryExpired {
			outcome.Expired = append(outcome.Expired, batch.ID)
			continue
		}
		usable = append(usable, batch)
	}
	sortFEFO(usable)

	remaining := qty
	for _, batch := range usable {
		if remaining <= 0 {
			break
		}
		take := batch.QtyOnHand
		if take > remaining {
			take = remaining
		}
		take = RoundQuantity(take)
		outcome.Allocations = append(outcome.Allocations, Allocation{
			BatchID: batch.ID, BatchCode: batch.BatchCode, ExpiresOn: batch.ExpiresOn,
			Qty: -take, QtyBefore: batch.QtyOnHand,
			QtyAfter:    RoundQuantity(batch.QtyOnHand - take),
			UnitCostIDR: batch.UnitCostIDR,
		})
		outcome.CostIDR += float64(take) * batch.UnitCostIDR
		remaining = RoundQuantity(remaining - take)
	}

	outcome.Shortfall = remaining
	outcome.CostIDR = math.Round(outcome.CostIDR*100) / 100
	return outcome
}

// sortFEFO orders batches the way they should leave the shelf: soonest date
// first, undated last, and arrival order as the tie-break so two batches
// sharing a date still come out in a stable order.
func sortFEFO(batches []Batch) {
	sort.SliceStable(batches, func(a, b int) bool {
		x, y := batches[a], batches[b]
		switch {
		case x.ExpiresOn == nil && y.ExpiresOn == nil:
			return x.ReceivedOn.Before(y.ReceivedOn)
		case x.ExpiresOn == nil:
			return false
		case y.ExpiresOn == nil:
			return true
		case *x.ExpiresOn != *y.ExpiresOn:
			return *x.ExpiresOn < *y.ExpiresOn
		default:
			return x.ReceivedOn.Before(y.ReceivedOn)
		}
	})
}

// ExpirySummary is what a branch is about to lose.
type ExpirySummary struct {
	NearBatches  int      `json:"nearBatches"`
	NearQty      Quantity `json:"nearQty"`
	NearValueIDR float64  `json:"nearValueIdr"`
	// Expired is stock that may no longer be sold and is waiting to be
	// written off. It is counted separately because it is already a loss
	// rather than a warning.
	ExpiredBatches  int      `json:"expiredBatches"`
	ExpiredQty      Quantity `json:"expiredQty"`
	ExpiredValueIDR float64  `json:"expiredValueIdr"`
}

// SummarizeExpiry values what is near its date and what is past it.
//
// Each batch is judged against its own item's warning window: a drink with six
// months of life wants a longer warning than something with a fortnight, and
// one number for the whole catalogue would be wrong for most of it.
func SummarizeExpiry(batches []Batch, today Date, warningDaysOf map[string]int) ExpirySummary {
	summary := ExpirySummary{}
	for _, batch := range batches {
		if batch.QtyOnHand <= 0 {
			continue
		}
		warning, ok := warningDaysOf[batch.ItemID]
		if !ok {
			warning = 30
		}
		value := float64(batch.QtyOnHand) * batch.UnitCostIDR
		switch batch.StateOn(today, warning) {
		case ExpiryNear:
			summary.NearBatches++
			summary.NearQty += batch.QtyOnHand
			summary.NearValueIDR += value
		case ExpiryExpired:
			summary.ExpiredBatches++
			summary.ExpiredQty += batch.QtyOnHand
			summary.ExpiredValueIDR += value
		}
	}
	summary.NearQty = RoundQuantity(summary.NearQty)
	summary.ExpiredQty = RoundQuantity(summary.ExpiredQty)
	summary.NearValueIDR = math.Round(summary.NearValueIDR*100) / 100
	summary.ExpiredValueIDR = math.Round(summary.ExpiredValueIDR*100) / 100
	return summary
}
