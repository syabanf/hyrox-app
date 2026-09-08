package domain

import (
	"math"
	"time"
)

// The till: what is for sale, what was sold, and how it was paid for.

// POSCategory groups what is on the counter.
type POSCategory struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sortOrder"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// POSProduct is something for sale.
//
// It is not an inventory item: the same shirt can be sold at two prices, and a
// towel hire has a price but no stock. InventoryItemID is the link where one
// exists, and its absence is what makes a service sellable.
type POSProduct struct {
	ID              string  `json:"id"`
	SKU             string  `json:"sku"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	CategoryID      *string `json:"categoryId"`
	InventoryItemID *string `json:"inventoryItemId"`
	PriceIDR        float64 `json:"priceIdr"`
	CostIDR         float64 `json:"costIdr"`
	TaxPercent      float64 `json:"taxPercent"`
	// BonusXP is worth extra points on top of whatever the spend earns.
	BonusXP   int       `json:"bonusXp"`
	ImageURL  *string   `json:"imageUrl"`
	Active    bool      `json:"active"`
	Available bool      `json:"available"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Sellable reports whether a product may be put on an order right now.
func (p POSProduct) Sellable() bool { return p.Active && p.Available }

// ── Orders ───────────────────────────────────────────────────────────────────

// POSOrderStatus is where a sale has got to.
type POSOrderStatus string

const (
	POSOpen      POSOrderStatus = "OPEN"
	POSCompleted POSOrderStatus = "COMPLETED"
	// POSCancelled is an order abandoned before it was paid for.
	POSCancelled POSOrderStatus = "CANCELLED"
	// POSVoided is a completed sale unwound afterwards, which is a different
	// act needing a different permission and a reason.
	POSVoided POSOrderStatus = "VOIDED"
)

// POSOrderTransitions. An open order is cancelled; a completed one is voided.
// Conflating the two would let the counter erase a paid sale by calling it a
// cancellation.
var POSOrderTransitions = TransitionMap[POSOrderStatus]{
	POSOpen:      {POSCompleted, POSCancelled},
	POSCompleted: {POSVoided},
	POSCancelled: {},
	POSVoided:    {},
}

// POSPaymentStatus is how much of an order has been settled.
type POSPaymentStatus string

const (
	POSUnpaid   POSPaymentStatus = "UNPAID"
	POSPartial  POSPaymentStatus = "PARTIAL"
	POSPaid     POSPaymentStatus = "PAID"
	POSRefunded POSPaymentStatus = "REFUNDED"
)

// POSPaymentMethod is how the money arrived.
type POSPaymentMethod string

const (
	PayCash     POSPaymentMethod = "CASH"
	PayQRIS     POSPaymentMethod = "QRIS"
	PayDebit    POSPaymentMethod = "DEBIT"
	PayCredit   POSPaymentMethod = "CREDIT"
	PayTransfer POSPaymentMethod = "TRANSFER"
	// PayMemberCredit spends class credits at the counter. It is the one
	// method that touches the wallet.
	PayMemberCredit POSPaymentMethod = "MEMBER_CREDIT"
)

// IsValidPaymentMethod validates a method arriving from a request.
func IsValidPaymentMethod(value string) bool {
	switch POSPaymentMethod(value) {
	case PayCash, PayQRIS, PayDebit, PayCredit, PayTransfer, PayMemberCredit:
		return true
	}
	return false
}

// POSOrder is one sale.
type POSOrder struct {
	ID          string  `json:"id"`
	OrderNumber string  `json:"orderNumber"`
	BranchID    string  `json:"branchId"`
	ShiftID     *string `json:"shiftId"`
	CashierID   string  `json:"cashierId"`
	CashierName string  `json:"cashierName"`
	// MemberID is what earns points and applies a tier discount.
	MemberID      *string          `json:"memberId"`
	OrderType     string           `json:"orderType"`
	Status        POSOrderStatus   `json:"status"`
	PaymentStatus POSPaymentStatus `json:"paymentStatus"`
	SubtotalIDR   float64          `json:"subtotalIdr"`
	// DiscountIDR is what the cashier took off by hand; TierDiscountIDR is
	// what the member's standing took off. They are separate because the
	// second is recomputed on every change and the first is not — folding them
	// into one figure makes the tier's share compound with every line scanned.
	DiscountIDR      float64    `json:"discountIdr"`
	TierDiscountIDR  float64    `json:"tierDiscountIdr"`
	DiscountReason   *string    `json:"discountReason"`
	TaxIDR           float64    `json:"taxIdr"`
	ServiceChargeIDR float64    `json:"serviceChargeIdr"`
	TotalIDR         float64    `json:"totalIdr"`
	PaidIDR          float64    `json:"paidIdr"`
	ChangeIDR        float64    `json:"changeIdr"`
	CostIDR          float64    `json:"costIdr"`
	GrossProfitIDR   float64    `json:"grossProfitIdr"`
	XPEarned         int        `json:"xpEarned"`
	Note             *string    `json:"note"`
	OpenedAt         time.Time  `json:"openedAt"`
	CompletedAt      *time.Time `json:"completedAt"`
	CancelledAt      *time.Time `json:"cancelledAt"`
	VoidedAt         *time.Time `json:"voidedAt"`
	VoidedBy         *string    `json:"voidedBy"`
	VoidReason       *string    `json:"voidReason"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// POSOrderItem is one sold line.
//
// The product's name, SKU, price and cost are copied onto it because a receipt
// has to still be true a year later, when the product has been renamed and
// repriced and the average cost has moved twice.
type POSOrderItem struct {
	ID              string    `json:"id"`
	OrderID         string    `json:"orderId"`
	ProductID       string    `json:"productId"`
	ProductName     string    `json:"productName"`
	ProductSKU      string    `json:"productSku"`
	InventoryItemID *string   `json:"inventoryItemId"`
	Qty             Quantity  `json:"qty"`
	UnitPriceIDR    float64   `json:"unitPriceIdr"`
	DiscountIDR     float64   `json:"discountIdr"`
	TaxPercent      float64   `json:"taxPercent"`
	LineTotalIDR    float64   `json:"lineTotalIdr"`
	UnitCostIDR     float64   `json:"unitCostIdr"`
	Note            *string   `json:"note"`
	CreatedAt       time.Time `json:"createdAt"`
}

// POSPayment is one tender against an order. Several rows is a split payment.
type POSPayment struct {
	ID        string           `json:"id"`
	OrderID   string           `json:"orderId"`
	Method    POSPaymentMethod `json:"method"`
	AmountIDR float64          `json:"amountIdr"`
	ChangeIDR float64          `json:"changeIdr"`
	Reference *string          `json:"reference"`
	CashierID string           `json:"cashierId"`
	TakenAt   time.Time        `json:"takenAt"`
}

// POSTotals is the arithmetic of a sale.
type POSTotals struct {
	SubtotalIDR      float64 `json:"subtotalIdr"`
	DiscountIDR      float64 `json:"discountIdr"`
	TaxIDR           float64 `json:"taxIdr"`
	ServiceChargeIDR float64 `json:"serviceChargeIdr"`
	TotalIDR         float64 `json:"totalIdr"`
	CostIDR          float64 `json:"costIdr"`
	GrossProfitIDR   float64 `json:"grossProfitIdr"`
}

// ComputeOrderPOSTotals adds up a sale.
//
// The order-level discount is spread across the lines in proportion to their
// value before tax is worked out, because tax is charged on what is actually
// paid. Doing it the other way round overcharges the customer and the studio's
// tax return in the same stroke.
func ComputeOrderPOSTotals(items []POSOrderItem, discountIDR, serviceChargePercent float64) POSTotals {
	var subtotal, cost float64
	for _, item := range items {
		subtotal += item.LineTotalIDR
		cost += float64(item.Qty) * item.UnitCostIDR
	}
	subtotal = round2(subtotal)
	cost = round2(cost)

	if discountIDR > subtotal {
		discountIDR = subtotal
	}
	discountIDR = round2(discountIDR)

	var tax float64
	if subtotal > 0 {
		for _, item := range items {
			share := item.LineTotalIDR / subtotal
			taxable := item.LineTotalIDR - discountIDR*share
			tax += taxable * item.TaxPercent / 100
		}
	}
	tax = round2(tax)

	taxable := subtotal - discountIDR
	service := round2(taxable * serviceChargePercent / 100)
	total := round2(taxable + tax + service)

	return POSTotals{
		SubtotalIDR: subtotal, DiscountIDR: discountIDR, TaxIDR: tax,
		ServiceChargeIDR: service, TotalIDR: total,
		CostIDR: cost, GrossProfitIDR: round2(taxable - cost),
	}
}

// TierDiscount is what a member's standing takes off a sale.
func TierDiscount(subtotalIDR, discountPercent float64) float64 {
	if discountPercent <= 0 {
		return 0
	}
	return round2(subtotalIDR * discountPercent / 100)
}

// SettlementOutcome is what a set of tenders does to an order.
type SettlementOutcome struct {
	PaidIDR       float64
	ChangeIDR     float64
	PaymentStatus POSPaymentStatus
	// Settled reports whether the order is now fully paid.
	Settled bool
}

// Settle works out where an order stands after its tenders.
//
// Overpayment is only change when it was cash. A card is charged what it is
// charged, so an overpaid card tender is a mistake to refuse rather than a
// float to hand back.
func Settle(total float64, payments []POSPayment) SettlementOutcome {
	var paid, cash float64
	for _, payment := range payments {
		paid += payment.AmountIDR
		if payment.Method == PayCash {
			cash += payment.AmountIDR
		}
	}
	paid = round2(paid)

	outcome := SettlementOutcome{PaidIDR: paid}
	switch {
	case paid <= 0:
		outcome.PaymentStatus = POSUnpaid
	case paid < total:
		outcome.PaymentStatus = POSPartial
	default:
		outcome.PaymentStatus = POSPaid
		outcome.Settled = true
		over := round2(paid - total)
		if over > 0 {
			// Change comes out of the cash tendered, never out of a card.
			outcome.ChangeIDR = math.Min(over, cash)
		}
	}
	return outcome
}

// POSRejection explains why a line or a tender is refused.
type POSRejection string

const (
	POSRejectNotSellable  POSRejection = "PRODUCT_NOT_SELLABLE"
	POSRejectOrderClosed  POSRejection = "ORDER_NOT_OPEN"
	POSRejectNothingToPay POSRejection = "NOTHING_TO_PAY"
	POSRejectOverTendered POSRejection = "OVER_TENDERED"
	POSRejectEmptyOrder   POSRejection = "EMPTY_ORDER"
	POSRejectNotSettled   POSRejection = "NOT_SETTLED"
)

// EvaluateTender checks one tender against what is still owed.
//
// A card cannot overpay: there is no change to give, and the difference would
// simply be lost.
func EvaluateTender(order POSOrder, method POSPaymentMethod, amount, alreadyPaid float64) POSRejection {
	if order.Status != POSOpen {
		return POSRejectOrderClosed
	}
	if order.TotalIDR <= 0 {
		return POSRejectNothingToPay
	}
	if method != PayCash && round2(alreadyPaid+amount) > order.TotalIDR {
		return POSRejectOverTendered
	}
	return ""
}

// ── Shifts ───────────────────────────────────────────────────────────────────

// ShiftStatus is whether a till is open.
type ShiftStatus string

const (
	ShiftOpen   ShiftStatus = "OPEN"
	ShiftClosed ShiftStatus = "CLOSED"
)

// CashierShift is one cashier's session at the till.
type CashierShift struct {
	ID              string      `json:"id"`
	ShiftNumber     string      `json:"shiftNumber"`
	CashierID       string      `json:"cashierId"`
	CashierName     string      `json:"cashierName"`
	BranchID        string      `json:"branchId"`
	Status          ShiftStatus `json:"status"`
	OpenedAt        time.Time   `json:"openedAt"`
	ClosedAt        *time.Time  `json:"closedAt"`
	OpeningCashIDR  float64     `json:"openingCashIdr"`
	ClosingCashIDR  *float64    `json:"closingCashIdr"`
	ExpectedCashIDR float64     `json:"expectedCashIdr"`
	VarianceIDR     float64     `json:"varianceIdr"`
	Note            *string     `json:"note"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

// ShiftTotals is what a session took, by method.
type ShiftTotals struct {
	Orders       int     `json:"orders"`
	SalesIDR     float64 `json:"salesIdr"`
	CashIDR      float64 `json:"cashIdr"`
	QRISIDR      float64 `json:"qrisIdr"`
	CardIDR      float64 `json:"cardIdr"`
	TransferIDR  float64 `json:"transferIdr"`
	CreditIDR    float64 `json:"memberCreditIdr"`
	VoidedIDR    float64 `json:"voidedIdr"`
	ExpectedCash float64 `json:"expectedCashIdr"`
}

// ExpectedCash is what the drawer should hold at the end of a shift: what was
// in it at the start, plus every cash tender, minus the change handed back.
//
// Only cash counts. A card sale never touches the drawer, and counting it
// would make every till look short by exactly the card takings.
func ExpectedCash(openingCashIDR float64, payments []POSPayment) float64 {
	total := openingCashIDR
	for _, payment := range payments {
		if payment.Method != PayCash {
			continue
		}
		total += payment.AmountIDR - payment.ChangeIDR
	}
	return round2(total)
}
