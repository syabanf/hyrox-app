package domain

import (
	"math"
	"sort"
	"time"
)

// What arrived, what we owe, and who signed.

// ── The truck arrived ────────────────────────────────────────────────────────

// DeliveryStatus is where an arrival has got to.
type DeliveryStatus string

const (
	// DeliveryArrived is goods on the bay, counted, not yet inspected.
	DeliveryArrived DeliveryStatus = "ARRIVED"
	// DeliveryInspected is a goods receipt having been raised against it.
	DeliveryInspected DeliveryStatus = "INSPECTED"
	DeliveryCancelled DeliveryStatus = "CANCELLED"
)

// DeliveryTransitions. An inspected delivery is final: a goods receipt points
// at it and stock has moved, so unwinding it is a credit note rather than an
// edit.
var DeliveryTransitions = TransitionMap[DeliveryStatus]{
	DeliveryArrived:   {DeliveryInspected, DeliveryCancelled},
	DeliveryInspected: {},
	DeliveryCancelled: {},
}

// Delivery is what came off the truck.
type Delivery struct {
	ID                 string         `json:"id"`
	DeliveryNumber     string         `json:"deliveryNumber"`
	OrderID            string         `json:"orderId"`
	SupplierID         string         `json:"supplierId"`
	BranchID           string         `json:"branchId"`
	DeliveryNoteNumber *string        `json:"deliveryNoteNumber"`
	DriverName         *string        `json:"driverName"`
	Vehicle            *string        `json:"vehicle"`
	ArrivedOn          Date           `json:"arrivedOn"`
	ArrivedAt          time.Time      `json:"arrivedAt"`
	ReceivedBy         *string        `json:"receivedBy"`
	ReceivedByName     *string        `json:"receivedByName"`
	Status             DeliveryStatus `json:"status"`
	Note               *string        `json:"note"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
}

// DeliveryItem is one line off the truck, in the pack it was ordered in.
//
// There is one quantity here and not two, which is the point of separating a
// delivery from a receipt: this is what was handed over, before anybody has
// decided whether it is any good.
type DeliveryItem struct {
	ID           string   `json:"id"`
	DeliveryID   string   `json:"deliveryId"`
	OrderItemID  string   `json:"orderItemId"`
	ItemID       string   `json:"itemId"`
	QtyDelivered Quantity `json:"qtyDelivered"`
	Unit         string   `json:"unit"`
	PackFactor   float64  `json:"packFactor"`
	BatchNumber  *string  `json:"batchNumber"`
	ExpiresOn    *Date    `json:"expiresOn"`
	Note         *string  `json:"note"`
}

// BaseDelivered is the line in the unit stock is counted in.
func (i DeliveryItem) BaseDelivered() Quantity {
	return PackToBase(i.QtyDelivered, i.PackFactor)
}

// DeliveryRejection explains why an arrival cannot be recorded.
type DeliveryRejection string

const (
	DeliveryRejectOverOrdered DeliveryRejection = "OVER_DELIVERED"
	DeliveryRejectClosed      DeliveryRejection = "ORDER_NOT_OPEN"
	DeliveryRejectZero        DeliveryRejection = "ZERO_QUANTITY"
)

// EvaluateDeliveryLine checks one arrival against what is still owed.
//
// The comparison is against the order, not against what has already been
// receipted: a supplier who delivers ten and has nine accepted still delivered
// ten, and the tenth is a rejection rather than a second delivery.
func EvaluateDeliveryLine(order PurchaseOrder, line PurchaseOrderItem,
	alreadyDelivered, qty Quantity) DeliveryRejection {

	if qty <= 0 {
		return DeliveryRejectZero
	}
	switch order.Status {
	case POApproved, POSent, POPartiallyReceived:
	default:
		return DeliveryRejectClosed
	}
	if RoundQuantity(alreadyDelivered+qty) > line.QtyOrdered {
		return DeliveryRejectOverOrdered
	}
	return ""
}

// InspectionOutcome is how much of a delivery a receipt may still account for.
type InspectionOutcome struct {
	Delivered   Quantity `json:"delivered"`
	Inspected   Quantity `json:"inspected"`
	Outstanding Quantity `json:"outstanding"`
	// Complete reports whether every delivered unit has now been either
	// accepted or rejected, which is what lets the delivery close.
	Complete bool `json:"complete"`
}

// InspectDelivery works out what is left to judge on an arrival.
//
// Accepted and rejected are added together because both are decisions: goods
// that failed inspection have been dealt with, and only goods nobody has
// looked at are still outstanding.
func InspectDelivery(delivered Quantity, accepted, rejected Quantity) InspectionOutcome {
	inspected := RoundQuantity(accepted + rejected)
	outstanding := RoundQuantity(delivered - inspected)
	if outstanding < 0 {
		outstanding = 0
	}
	return InspectionOutcome{
		Delivered: delivered, Inspected: inspected,
		Outstanding: outstanding, Complete: inspected >= delivered,
	}
}

// ── What we owe ──────────────────────────────────────────────────────────────

// PaymentTermStatus is how far an instalment has been settled.
type PaymentTermStatus string

const (
	TermPending   PaymentTermStatus = "PENDING"
	TermPartial   PaymentTermStatus = "PARTIAL"
	TermPaid      PaymentTermStatus = "PAID"
	TermCancelled PaymentTermStatus = "CANCELLED"
)

// PaymentTerm is one instalment on an order.
type PaymentTerm struct {
	ID        string            `json:"id"`
	OrderID   string            `json:"orderId"`
	Sequence  int               `json:"sequence"`
	Label     string            `json:"label"`
	DueOn     Date              `json:"dueOn"`
	Percent   *float64          `json:"percent"`
	AmountIDR float64           `json:"amountIdr"`
	Status    PaymentTermStatus `json:"status"`
	PaidIDR   float64           `json:"paidIdr"`
	Note      *string           `json:"note"`
}

// OutstandingIDR is what this instalment still needs.
func (t PaymentTerm) OutstandingIDR() float64 {
	if t.Status == TermCancelled {
		return 0
	}
	return round2(math.Max(0, t.AmountIDR-t.PaidIDR))
}

// Overdue reports whether an instalment's day has passed with money still owed.
func (t PaymentTerm) Overdue(today Date) bool {
	return t.OutstandingIDR() > 0 && t.DueOn.Before(today)
}

// TermSchedule is how a supplier's payment terms turn into dates.
//
// The codes are the ones a supplier record carries. COD is due the day the
// order is placed; the NET codes are that many days after.
func TermSchedule(code PaymentTerms) (days int, ok bool) {
	switch code {
	case TermsCOD:
		return 0, true
	case TermsNet7:
		return 7, true
	case TermsNet14:
		return 14, true
	case TermsNet30:
		return 30, true
	case TermsNet45:
		return 45, true
	case TermsNet60:
		return 60, true
	}
	return 0, false
}

// BuildSchedule turns an order total and a term code into instalments.
//
// One instalment, because that is what a term code can express. Splitting it
// — half on order, half on delivery — is an edit somebody makes afterwards,
// since no code on a supplier record can say that.
func BuildSchedule(totalIDR float64, code PaymentTerms, orderedOn Date) []PaymentTerm {
	days, ok := TermSchedule(code)
	if !ok {
		days = 30
	}
	full := 100.0
	return []PaymentTerm{{
		Sequence: 1, Label: string(code), DueOn: orderedOn.AddDays(days),
		Percent: &full, AmountIDR: round2(totalIDR), Status: TermPending,
	}}
}

// Rebalance spreads a total across instalments by their agreed percentages.
//
// The last instalment absorbs the rounding. Splitting 1.000.000 three ways and
// letting each round independently leaves a rupiah unaccounted for, and a
// payables report that is one rupiah out is a payables report somebody stops
// trusting.
func Rebalance(terms []PaymentTerm, totalIDR float64) []PaymentTerm {
	out := append([]PaymentTerm(nil), terms...)
	live := []int{}
	for i, term := range out {
		if term.Status != TermCancelled && term.Percent != nil {
			live = append(live, i)
		}
	}
	if len(live) == 0 {
		return out
	}

	var assigned float64
	for _, i := range live[:len(live)-1] {
		amount := round2(totalIDR * *out[i].Percent / 100)
		out[i].AmountIDR = amount
		assigned += amount
	}
	last := live[len(live)-1]
	out[last].AmountIDR = round2(totalIDR - assigned)
	return out
}

// PaymentPosting is what a payment does to an instalment.
type PaymentPosting struct {
	PaidIDR    float64
	Status     PaymentTermStatus
	OverpaidBy float64
}

// PostToTerm applies money to one instalment.
//
// Overpayment is reported rather than absorbed: paying 5.000.000 against a
// 4.000.000 instalment is either a mistake or a payment against something
// else, and quietly swallowing the difference loses whichever it was.
func PostToTerm(term PaymentTerm, amountIDR float64) PaymentPosting {
	paid := round2(term.PaidIDR + amountIDR)
	posting := PaymentPosting{PaidIDR: paid}
	if paid > term.AmountIDR {
		posting.OverpaidBy = round2(paid - term.AmountIDR)
	}
	switch {
	case paid <= 0:
		posting.Status = TermPending
	case paid < term.AmountIDR:
		posting.Status = TermPartial
	default:
		posting.Status = TermPaid
	}
	return posting
}

// PayablesPosition is where an order stands on money.
type PayablesPosition struct {
	TotalIDR     float64 `json:"totalIdr"`
	ScheduledIDR float64 `json:"scheduledIdr"`
	PaidIDR      float64 `json:"paidIdr"`
	CreditedIDR  float64 `json:"creditedIdr"`
	// OutstandingIDR counts credit notes as settled: the money will not move,
	// but the debt is gone, and a payables list that still shows it sends
	// somebody to pay twice.
	OutstandingIDR float64 `json:"outstandingIdr"`
	OverdueIDR     float64 `json:"overdueIdr"`
	NextDue        *Date   `json:"nextDue"`
}

// PayablesFor adds up an order's money.
func PayablesFor(totalIDR float64, terms []PaymentTerm, creditedIDR float64, today Date) PayablesPosition {
	position := PayablesPosition{TotalIDR: round2(totalIDR), CreditedIDR: round2(creditedIDR)}
	for _, term := range terms {
		if term.Status == TermCancelled {
			continue
		}
		position.ScheduledIDR += term.AmountIDR
		position.PaidIDR += term.PaidIDR
		if term.Overdue(today) {
			position.OverdueIDR += term.OutstandingIDR()
		}
		if term.OutstandingIDR() > 0 {
			if position.NextDue == nil || term.DueOn.Before(*position.NextDue) {
				due := term.DueOn
				position.NextDue = &due
			}
		}
	}
	position.ScheduledIDR = round2(position.ScheduledIDR)
	position.PaidIDR = round2(position.PaidIDR)
	position.OverdueIDR = round2(position.OverdueIDR)
	position.OutstandingIDR = round2(math.Max(0,
		position.TotalIDR-position.PaidIDR-position.CreditedIDR))
	return position
}

// ── Credit notes ─────────────────────────────────────────────────────────────

// CreditStatus is how much of a credit note has been spent.
type CreditStatus string

const (
	CreditOpen      CreditStatus = "OPEN"
	CreditPartial   CreditStatus = "PARTIALLY_APPLIED"
	CreditApplied   CreditStatus = "APPLIED"
	CreditCancelled CreditStatus = "CANCELLED"
)

// VendorCredit is what a supplier owes back, usually from a return.
//
// It is not a refund: the money does not move, the next invoice is smaller.
type VendorCredit struct {
	ID           string       `json:"id"`
	CreditNumber string       `json:"creditNumber"`
	SupplierID   string       `json:"supplierId"`
	ReturnID     *string      `json:"returnId"`
	IssuedOn     Date         `json:"issuedOn"`
	AmountIDR    float64      `json:"amountIdr"`
	AppliedIDR   float64      `json:"appliedIdr"`
	Status       CreditStatus `json:"status"`
	Reason       *string      `json:"reason"`
	ExpiresOn    *Date        `json:"expiresOn"`
}

// RemainingIDR is what is left on a credit note.
func (c VendorCredit) RemainingIDR() float64 {
	if c.Status == CreditCancelled {
		return 0
	}
	return round2(math.Max(0, c.AmountIDR-c.AppliedIDR))
}

// Spent reports where a credit stands once some of it is used.
func Spent(credit VendorCredit, amountIDR float64) CreditStatus {
	applied := round2(credit.AppliedIDR + amountIDR)
	switch {
	case applied <= 0:
		return CreditOpen
	case applied < credit.AmountIDR:
		return CreditPartial
	default:
		return CreditApplied
	}
}

// CreditAllocation is one credit note's share of a payment.
type CreditAllocation struct {
	CreditID     string  `json:"creditId"`
	CreditNumber string  `json:"creditNumber"`
	AmountIDR    float64 `json:"amountIdr"`
}

// AllocateCredits decides which credit notes settle a payment.
//
// Oldest first, and expired ones not at all. Spending the newest would leave
// the old ones to lapse, which is the same as throwing the money away — the
// whole reason a credit note carries an expiry is that somebody has to use it
// before then.
func AllocateCredits(credits []VendorCredit, amountIDR float64, today Date) ([]CreditAllocation, float64) {
	if amountIDR <= 0 {
		return nil, 0
	}

	usable := make([]VendorCredit, 0, len(credits))
	for _, credit := range credits {
		if credit.RemainingIDR() <= 0 {
			continue
		}
		if credit.ExpiresOn != nil && credit.ExpiresOn.Before(today) {
			continue
		}
		usable = append(usable, credit)
	}
	sort.SliceStable(usable, func(a, b int) bool {
		return usable[a].IssuedOn.Before(usable[b].IssuedOn)
	})

	allocations := []CreditAllocation{}
	remaining := round2(amountIDR)
	for _, credit := range usable {
		if remaining <= 0 {
			break
		}
		take := math.Min(credit.RemainingIDR(), remaining)
		take = round2(take)
		if take <= 0 {
			continue
		}
		allocations = append(allocations, CreditAllocation{
			CreditID: credit.ID, CreditNumber: credit.CreditNumber, AmountIDR: take,
		})
		remaining = round2(remaining - take)
	}
	return allocations, remaining
}
