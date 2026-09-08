package domain

import (
	"math"
	"time"
)

// Buying: who may ask, who must sign, and what happens when the boxes arrive.

// SupplierStatus is how much business may be done with a supplier.
type SupplierStatus string

const (
	SupplierDraft  SupplierStatus = "DRAFT"
	SupplierActive SupplierStatus = "ACTIVE"
	// SupplierProbation is one being trialled: orders are allowed and somebody
	// is watching.
	SupplierProbation SupplierStatus = "PROBATION"
	SupplierInactive  SupplierStatus = "INACTIVE"
	// SupplierBlocked is one nobody may order from again, which is a different
	// statement from merely inactive.
	SupplierBlocked SupplierStatus = "BLOCKED"
)

// CanOrderFrom reports whether a purchase order may name this supplier.
func (s SupplierStatus) CanOrderFrom() bool {
	return s == SupplierActive || s == SupplierProbation
}

// PaymentTerms is how long after delivery an invoice falls due.
type PaymentTerms string

const (
	TermsCOD   PaymentTerms = "COD"
	TermsNet7  PaymentTerms = "NET7"
	TermsNet14 PaymentTerms = "NET14"
	TermsNet30 PaymentTerms = "NET30"
	TermsNet45 PaymentTerms = "NET45"
	TermsNet60 PaymentTerms = "NET60"
)

// IsValidPaymentTerms validates terms arriving from a request.
func IsValidPaymentTerms(value string) bool {
	switch PaymentTerms(value) {
	case TermsCOD, TermsNet7, TermsNet14, TermsNet30, TermsNet45, TermsNet60:
		return true
	}
	return false
}

// DueDate is when an invoice delivered on a date must be paid.
func (t PaymentTerms) DueDate(delivered Date) Date {
	switch t {
	case TermsCOD:
		return delivered
	case TermsNet7:
		return delivered.AddDays(7)
	case TermsNet14:
		return delivered.AddDays(14)
	case TermsNet30:
		return delivered.AddDays(30)
	case TermsNet45:
		return delivered.AddDays(45)
	case TermsNet60:
		return delivered.AddDays(60)
	default:
		return delivered
	}
}

// Supplier is somebody the studio buys from.
type Supplier struct {
	ID           string         `json:"id"`
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	ContactName  *string        `json:"contactName"`
	ContactPhone *string        `json:"contactPhone"`
	Email        *string        `json:"email"`
	Address      *string        `json:"address"`
	City         *string        `json:"city"`
	TaxNumber    *string        `json:"taxNumber"`
	PaymentTerms PaymentTerms   `json:"paymentTerms"`
	BankName     *string        `json:"bankName"`
	BankAccount  *string        `json:"bankAccount"`
	BankHolder   *string        `json:"bankHolder"`
	Category     *string        `json:"category"`
	Status       SupplierStatus `json:"status"`
	Note         *string        `json:"note"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

// SupplierPrice is what one supplier charges for one item.
type SupplierPrice struct {
	ID            string    `json:"id"`
	SupplierID    string    `json:"supplierId"`
	ItemID        string    `json:"itemId"`
	UnitPriceIDR  float64   `json:"unitPriceIdr"`
	MinOrderQty   Quantity  `json:"minOrderQty"`
	LeadTimeDays  int       `json:"leadTimeDays"`
	EffectiveFrom Date      `json:"effectiveFrom"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"createdAt"`
}

// ── The approval chain ───────────────────────────────────────────────────────

// ApprovalLevel is one signature on a purchase request.
type ApprovalLevel string

const (
	ApprovalHead     ApprovalLevel = "HEAD"
	ApprovalFinance  ApprovalLevel = "FINANCE"
	ApprovalDirector ApprovalLevel = "DIRECTOR"
)

// ApprovalThresholds decides how many signatures a request needs.
//
// Money is the only thing that should decide this. A rule based on who asked,
// or on what was asked for, is a rule somebody will route around.
type ApprovalThresholds struct {
	// FinanceAbove is the total past which finance must also sign.
	FinanceAbove float64
	// DirectorAbove is the total past which a director must sign as well.
	DirectorAbove float64
}

// DefaultApprovalThresholds are the studio's, in rupiah.
func DefaultApprovalThresholds() ApprovalThresholds {
	return ApprovalThresholds{FinanceAbove: 5_000_000, DirectorAbove: 25_000_000}
}

// RequiredApprovals is the chain a request of this size must climb, in order.
//
// Every request needs a head's signature however small — somebody other than
// the person asking has to have seen it.
func RequiredApprovals(totalIDR float64, t ApprovalThresholds) []ApprovalLevel {
	levels := []ApprovalLevel{ApprovalHead}
	if totalIDR > t.FinanceAbove {
		levels = append(levels, ApprovalFinance)
	}
	if totalIDR > t.DirectorAbove {
		levels = append(levels, ApprovalDirector)
	}
	return levels
}

// CanApproveAt reports whether a role may sign at a level.
//
// A higher office may sign for a lower one — a director approving what a
// branch manager could have approved is normal, and refusing it only creates
// deadlock when somebody is on leave. The reverse is never true.
func CanApproveAt(role AdminRole, level ApprovalLevel) bool {
	switch level {
	case ApprovalHead:
		return role == RoleBranchManager || role == RoleHQAdmin || role == RoleSuperAdmin
	case ApprovalFinance:
		return role == RoleFinance || role == RoleHQAdmin || role == RoleSuperAdmin
	case ApprovalDirector:
		return role == RoleHQAdmin || role == RoleSuperAdmin
	default:
		return false
	}
}

// PurchaseRequestStatus is where a request has got to.
type PurchaseRequestStatus string

const (
	PRDraft           PurchaseRequestStatus = "DRAFT"
	PRPendingHead     PurchaseRequestStatus = "PENDING_HEAD"
	PRPendingFinance  PurchaseRequestStatus = "PENDING_FINANCE"
	PRPendingDirector PurchaseRequestStatus = "PENDING_DIRECTOR"
	PRApproved        PurchaseRequestStatus = "APPROVED"
	PRRejected        PurchaseRequestStatus = "REJECTED"
	PRConverted       PurchaseRequestStatus = "CONVERTED"
)

// PurchaseRequestTransitions. A request can be turned down at any pending
// stage, and a rejected or converted one is finished.
var PurchaseRequestTransitions = TransitionMap[PurchaseRequestStatus]{
	PRDraft:           {PRPendingHead, PRRejected},
	PRPendingHead:     {PRPendingFinance, PRPendingDirector, PRApproved, PRRejected},
	PRPendingFinance:  {PRPendingDirector, PRApproved, PRRejected},
	PRPendingDirector: {PRApproved, PRRejected},
	PRApproved:        {PRConverted, PRRejected},
	PRRejected:        {},
	PRConverted:       {},
}

// PendingStatusFor is the status a request sits at while it waits for a level.
func PendingStatusFor(level ApprovalLevel) PurchaseRequestStatus {
	switch level {
	case ApprovalHead:
		return PRPendingHead
	case ApprovalFinance:
		return PRPendingFinance
	case ApprovalDirector:
		return PRPendingDirector
	default:
		return PRPendingHead
	}
}

// PurchaseRequest is somebody asking for something to be bought.
type PurchaseRequest struct {
	ID            string                `json:"id"`
	PRNumber      string                `json:"prNumber"`
	BranchID      string                `json:"branchId"`
	RequesterID   *string               `json:"requesterId"`
	RequesterName string                `json:"requesterName"`
	DepartmentID  *string               `json:"departmentId"`
	Status        PurchaseRequestStatus `json:"status"`
	Priority      string                `json:"priority"`
	TotalIDR      float64               `json:"totalIdr"`
	RequiredOn    *Date                 `json:"requiredOn"`
	Note          *string               `json:"note"`

	ApprovedByHead     *string    `json:"approvedByHead"`
	ApprovedAtHead     *time.Time `json:"approvedAtHead"`
	ApprovedByFinance  *string    `json:"approvedByFinance"`
	ApprovedAtFinance  *time.Time `json:"approvedAtFinance"`
	ApprovedByDirector *string    `json:"approvedByDirector"`
	ApprovedAtDirector *time.Time `json:"approvedAtDirector"`

	RejectedBy      *string    `json:"rejectedBy"`
	RejectedAt      *time.Time `json:"rejectedAt"`
	RejectionReason *string    `json:"rejectionReason"`
	ConvertedPOID   *string    `json:"convertedPoId"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// SignedAt reports whether a level has already signed.
func (r PurchaseRequest) SignedAt(level ApprovalLevel) bool {
	switch level {
	case ApprovalHead:
		return r.ApprovedAtHead != nil
	case ApprovalFinance:
		return r.ApprovedAtFinance != nil
	case ApprovalDirector:
		return r.ApprovedAtDirector != nil
	default:
		return false
	}
}

// NextApproval is the level a request is currently waiting on, and whether it
// is waiting on anything at all.
func NextApproval(r PurchaseRequest, t ApprovalThresholds) (ApprovalLevel, bool) {
	for _, level := range RequiredApprovals(r.TotalIDR, t) {
		if !r.SignedAt(level) {
			return level, true
		}
	}
	return "", false
}

// PurchaseRequestItem is one line of an ask. The description is free text so
// somebody can request a thing that is not in the catalogue yet, which is most
// of the reason purchase requests exist.
type PurchaseRequestItem struct {
	ID                string   `json:"id"`
	RequestID         string   `json:"requestId"`
	ItemID            *string  `json:"itemId"`
	Description       string   `json:"description"`
	Qty               Quantity `json:"qty"`
	Unit              string   `json:"unit"`
	EstimatedPriceIDR float64  `json:"estimatedPriceIdr"`
	TotalIDR          float64  `json:"totalIdr"`
	Note              *string  `json:"note"`
}

// ── Orders ───────────────────────────────────────────────────────────────────

// PurchaseOrderStatus is where an order has got to.
type PurchaseOrderStatus string

const (
	PODraft             PurchaseOrderStatus = "DRAFT"
	POApproved          PurchaseOrderStatus = "APPROVED"
	POSent              PurchaseOrderStatus = "SENT"
	POPartiallyReceived PurchaseOrderStatus = "PARTIALLY_RECEIVED"
	POReceived          PurchaseOrderStatus = "RECEIVED"
	POCancelled         PurchaseOrderStatus = "CANCELLED"
)

// PurchaseOrderTransitions. Once anything has been received the order cannot
// be cancelled: stock has moved, and a cancellation would leave the ledger
// pointing at a document that claims nothing was ever ordered.
var PurchaseOrderTransitions = TransitionMap[PurchaseOrderStatus]{
	PODraft:             {POApproved, POCancelled},
	POApproved:          {POSent, POCancelled},
	POSent:              {POPartiallyReceived, POReceived, POCancelled},
	POPartiallyReceived: {POReceived},
	POReceived:          {},
	POCancelled:         {},
}

// PurchaseOrder is a commitment to a supplier at a price.
type PurchaseOrder struct {
	ID          string              `json:"id"`
	PONumber    string              `json:"poNumber"`
	SupplierID  string              `json:"supplierId"`
	BranchID    string              `json:"branchId"`
	RequestID   *string             `json:"requestId"`
	Status      PurchaseOrderStatus `json:"status"`
	OrderedOn   Date                `json:"orderedOn"`
	ExpectedOn  *Date               `json:"expectedOn"`
	SubtotalIDR float64             `json:"subtotalIdr"`
	DiscountIDR float64             `json:"discountIdr"`
	TaxPercent  float64             `json:"taxPercent"`
	TaxIDR      float64             `json:"taxIdr"`
	TotalIDR    float64             `json:"totalIdr"`
	Terms       *string             `json:"terms"`
	ShipTo      *string             `json:"shipTo"`
	Note        *string             `json:"note"`

	ApprovedBy         *string    `json:"approvedBy"`
	ApprovedAt         *time.Time `json:"approvedAt"`
	SentAt             *time.Time `json:"sentAt"`
	CancelledBy        *string    `json:"cancelledBy"`
	CancelledAt        *time.Time `json:"cancelledAt"`
	CancellationReason *string    `json:"cancellationReason"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// PurchaseOrderItem is one line of a commitment.
type PurchaseOrderItem struct {
	ID           string   `json:"id"`
	OrderID      string   `json:"orderId"`
	ItemID       string   `json:"itemId"`
	Description  string   `json:"description"`
	QtyOrdered   Quantity `json:"qtyOrdered"`
	QtyReceived  Quantity `json:"qtyReceived"`
	Unit         string   `json:"unit"`
	UnitPriceIDR float64  `json:"unitPriceIdr"`
	DiscountIDR  float64  `json:"discountIdr"`
	SubtotalIDR  float64  `json:"subtotalIdr"`
	Note         *string  `json:"note"`
}

// QtyOutstanding is what is still owed on a line.
func (i PurchaseOrderItem) QtyOutstanding() Quantity {
	return RoundQuantity(i.QtyOrdered - i.QtyReceived)
}

// OrderTotals is the arithmetic of a purchase order.
type OrderTotals struct {
	SubtotalIDR float64 `json:"subtotalIdr"`
	DiscountIDR float64 `json:"discountIdr"`
	TaxIDR      float64 `json:"taxIdr"`
	TotalIDR    float64 `json:"totalIdr"`
}

// ComputeOrderTotals adds up an order.
//
// Tax is charged on what is actually payable — the subtotal after the
// order-level discount — which is both the Indonesian rule and the only one
// that makes a discount worth negotiating.
func ComputeOrderTotals(items []PurchaseOrderItem, discountIDR, taxPercent float64) OrderTotals {
	var subtotal float64
	for _, item := range items {
		subtotal += float64(item.QtyOrdered)*item.UnitPriceIDR - item.DiscountIDR
	}
	subtotal = round2(subtotal)

	if discountIDR > subtotal {
		discountIDR = subtotal
	}
	taxable := subtotal - discountIDR
	tax := round2(taxable * taxPercent / 100)

	return OrderTotals{
		SubtotalIDR: subtotal,
		DiscountIDR: round2(discountIDR),
		TaxIDR:      tax,
		TotalIDR:    round2(taxable + tax),
	}
}

func round2(value float64) float64 { return math.Round(value*100) / 100 }

// ReceiptOutcome is what receiving a delivery does to the order it is against.
type ReceiptOutcome struct {
	// Status is what the order becomes once these quantities are counted.
	Status PurchaseOrderStatus
	// Complete reports whether every line is now fully received.
	Complete bool
}

// ApplyReceipt decides an order's status after a delivery.
//
// Partially received is the normal case and has to be a first-class state:
// suppliers short-ship constantly, and an order that is 90% delivered is
// neither open nor done.
func ApplyReceipt(items []PurchaseOrderItem) ReceiptOutcome {
	anyReceived, allReceived := false, true
	for _, item := range items {
		if item.QtyReceived > 0 {
			anyReceived = true
		}
		if item.QtyReceived < item.QtyOrdered {
			allReceived = false
		}
	}
	switch {
	case allReceived && anyReceived:
		return ReceiptOutcome{Status: POReceived, Complete: true}
	case anyReceived:
		return ReceiptOutcome{Status: POPartiallyReceived}
	default:
		return ReceiptOutcome{Status: POSent}
	}
}

// ReceiptRejection explains why a delivery line cannot be booked in.
type ReceiptRejection string

const (
	ReceiptRejectNothingArrived ReceiptRejection = "NOTHING_ARRIVED"
	ReceiptRejectOverDelivered  ReceiptRejection = "OVER_DELIVERED"
	ReceiptRejectOrderNotOpen   ReceiptRejection = "ORDER_NOT_OPEN"
)

// EvaluateReceiptLine checks one delivered line against what was ordered.
//
// More cannot arrive than was ordered. That is a conversation with the
// supplier and a new order, not a quantity the system quietly invents.
func EvaluateReceiptLine(order PurchaseOrder, line PurchaseOrderItem, accepted, rejected Quantity) ReceiptRejection {
	if order.Status != POSent && order.Status != POApproved && order.Status != POPartiallyReceived {
		return ReceiptRejectOrderNotOpen
	}
	if accepted <= 0 && rejected <= 0 {
		return ReceiptRejectNothingArrived
	}
	if accepted+line.QtyReceived > line.QtyOrdered {
		return ReceiptRejectOverDelivered
	}
	return ""
}

// GoodsReceiptStatus is whether a delivery has been booked into stock.
type GoodsReceiptStatus string

const (
	GRNDraft     GoodsReceiptStatus = "DRAFT"
	GRNPosted    GoodsReceiptStatus = "POSTED"
	GRNCancelled GoodsReceiptStatus = "CANCELLED"
)

// GoodsReceiptTransitions. Posting writes append-only stock movements, so a
// posted receipt is final; a mistake is corrected with a purchase return.
var GoodsReceiptTransitions = TransitionMap[GoodsReceiptStatus]{
	GRNDraft:     {GRNPosted, GRNCancelled},
	GRNPosted:    {},
	GRNCancelled: {},
}

// GoodsReceipt is what actually turned up.
type GoodsReceipt struct {
	ID                 string             `json:"id"`
	GRNNumber          string             `json:"grnNumber"`
	OrderID            string             `json:"orderId"`
	SupplierID         string             `json:"supplierId"`
	BranchID           string             `json:"branchId"`
	ReceivedOn         Date               `json:"receivedOn"`
	ReceivedBy         *string            `json:"receivedBy"`
	ReceivedByName     *string            `json:"receivedByName"`
	DeliveryNoteNumber *string            `json:"deliveryNoteNumber"`
	Status             GoodsReceiptStatus `json:"status"`
	Note               *string            `json:"note"`
	PostedAt           *time.Time         `json:"postedAt"`
	CreatedAt          time.Time          `json:"createdAt"`
	UpdatedAt          time.Time          `json:"updatedAt"`
}

// QCStatus is what inspection made of a delivered line.
type QCStatus string

const (
	QCAccepted          QCStatus = "ACCEPTED"
	QCPartiallyRejected QCStatus = "PARTIALLY_REJECTED"
	QCRejected          QCStatus = "REJECTED"
)

// DeriveQCStatus reads the inspection outcome off the two quantities, so the
// label can never disagree with the numbers beside it.
func DeriveQCStatus(accepted, rejected Quantity) QCStatus {
	switch {
	case rejected == 0:
		return QCAccepted
	case accepted == 0:
		return QCRejected
	default:
		return QCPartiallyRejected
	}
}

// GoodsReceiptItem is one delivered line. Accepted stock enters inventory;
// rejected stock is recorded and never does, which is why they are two numbers
// rather than one.
type GoodsReceiptItem struct {
	ID           string   `json:"id"`
	ReceiptID    string   `json:"receiptId"`
	OrderItemID  string   `json:"orderItemId"`
	ItemID       string   `json:"itemId"`
	QtyAccepted  Quantity `json:"qtyAccepted"`
	QtyRejected  Quantity `json:"qtyRejected"`
	QtyReturned  Quantity `json:"qtyReturned"`
	UnitPriceIDR float64  `json:"unitPriceIdr"`
	QCStatus     QCStatus `json:"qcStatus"`
	BatchNumber  *string  `json:"batchNumber"`
	ExpiresOn    *Date    `json:"expiresOn"`
	Note         *string  `json:"note"`
}

// QtyReturnable is how much of an accepted line can still go back.
func (i GoodsReceiptItem) QtyReturnable() Quantity {
	return RoundQuantity(i.QtyAccepted - i.QtyReturned)
}

// ── Returns ──────────────────────────────────────────────────────────────────

// ReturnReason is why goods went back.
type ReturnReason string

const (
	ReturnDamaged      ReturnReason = "DAMAGED"
	ReturnWrongItem    ReturnReason = "WRONG_ITEM"
	ReturnExpired      ReturnReason = "EXPIRED"
	ReturnOverstock    ReturnReason = "OVERSTOCK"
	ReturnSpecMismatch ReturnReason = "SPEC_MISMATCH"
	ReturnOther        ReturnReason = "OTHER"
)

// IsValidReturnReason validates a reason arriving from a request.
func IsValidReturnReason(value string) bool {
	switch ReturnReason(value) {
	case ReturnDamaged, ReturnWrongItem, ReturnExpired,
		ReturnOverstock, ReturnSpecMismatch, ReturnOther:
		return true
	}
	return false
}

// PurchaseReturnStatus mirrors the receipt's: posting moves stock, so it is
// final.
type PurchaseReturnStatus string

const (
	ReturnDraft     PurchaseReturnStatus = "DRAFT"
	ReturnPosted    PurchaseReturnStatus = "POSTED"
	ReturnCancelled PurchaseReturnStatus = "CANCELLED"
)

var PurchaseReturnTransitions = TransitionMap[PurchaseReturnStatus]{
	ReturnDraft:     {ReturnPosted, ReturnCancelled},
	ReturnPosted:    {},
	ReturnCancelled: {},
}

// PurchaseReturn is goods going back to a supplier.
type PurchaseReturn struct {
	ID           string               `json:"id"`
	ReturnNumber string               `json:"returnNumber"`
	ReceiptID    string               `json:"receiptId"`
	SupplierID   string               `json:"supplierId"`
	BranchID     string               `json:"branchId"`
	ReturnedOn   Date                 `json:"returnedOn"`
	ReasonType   ReturnReason         `json:"reasonType"`
	ReasonNote   *string              `json:"reasonNote"`
	Status       PurchaseReturnStatus `json:"status"`
	TotalIDR     float64              `json:"totalIdr"`
	PostedAt     *time.Time           `json:"postedAt"`
	CreatedAt    time.Time            `json:"createdAt"`
	UpdatedAt    time.Time            `json:"updatedAt"`
}

// PurchaseReturnItem is one line going back.
type PurchaseReturnItem struct {
	ID            string   `json:"id"`
	ReturnID      string   `json:"returnId"`
	ReceiptItemID string   `json:"receiptItemId"`
	ItemID        string   `json:"itemId"`
	Qty           Quantity `json:"qty"`
	UnitPriceIDR  float64  `json:"unitPriceIdr"`
	Note          *string  `json:"note"`
}
