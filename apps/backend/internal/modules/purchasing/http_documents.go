package purchasing

import (
	"net/http"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Requests, orders, receipts and returns over HTTP.

// reasonRequest is the shared body for anything that needs an explanation.
type reasonRequest struct {
	Reason string `json:"reason"`
}

// ── Requests ─────────────────────────────────────────────────────────────────

func (h *Handler) listRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := h.service.Requests(r.Context(), RequestFilter{
		BranchID: httpx.Query(r, "branchId"),
		Status:   strings.ToUpper(httpx.Query(r, "status")),
		Priority: strings.ToUpper(httpx.Query(r, "priority")),
		Limit:    httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, requests)
}

func (h *Handler) getRequest(w http.ResponseWriter, r *http.Request) {
	request, err := h.service.Request(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, request)
}

type createRequestBody struct {
	BranchID      string  `json:"branchId"`
	RequesterID   *string `json:"requesterId"`
	RequesterName string  `json:"requesterName"`
	DepartmentID  *string `json:"departmentId"`
	Priority      string  `json:"priority"`
	RequiredOn    *string `json:"requiredOn"`
	Note          *string `json:"note"`
}

func (c *createRequestBody) Validate() error {
	if strings.TrimSpace(c.BranchID) == "" {
		return httpx.Invalid("A branch is required.")
	}
	if c.RequiredOn != nil && *c.RequiredOn != "" {
		if _, err := domain.ParseDate(*c.RequiredOn); err != nil {
			return httpx.Invalid("requiredOn must be a date as YYYY-MM-DD.")
		}
	}
	switch strings.ToUpper(c.Priority) {
	case "", "LOW", "NORMAL", "HIGH", "URGENT":
	default:
		return httpx.Invalid("%q is not a priority.", c.Priority)
	}
	return nil
}

// optionalDateInput turns an absent or blank string into no date at all.
func optionalDateInput(raw *string) *domain.Date {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	parsed, err := domain.ParseDate(*raw)
	if err != nil {
		return nil
	}
	return &parsed
}

func (h *Handler) createRequest(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[createRequestBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	request, err := h.service.CreateRequest(r.Context(), RequestInput{
		BranchID: body.BranchID, RequesterID: body.RequesterID,
		RequesterName: body.RequesterName, DepartmentID: body.DepartmentID,
		Priority: body.Priority, RequiredOn: optionalDateInput(body.RequiredOn), Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, request)
}

type requestLineBody struct {
	ItemID            *string `json:"itemId"`
	Description       string  `json:"description"`
	Qty               float64 `json:"qty"`
	Unit              string  `json:"unit"`
	EstimatedPriceIDR float64 `json:"estimatedPriceIdr"`
	Note              *string `json:"note"`
}

func (l *requestLineBody) Validate() error {
	if strings.TrimSpace(l.Description) == "" {
		return httpx.Invalid("A line needs a description.")
	}
	if l.Qty <= 0 {
		return httpx.Invalid("A line needs a positive quantity.")
	}
	if l.EstimatedPriceIDR < 0 {
		return httpx.Invalid("A price cannot be negative.")
	}
	return nil
}

func (h *Handler) addRequestLine(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[requestLineBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	request, err := h.service.AddRequestLine(r.Context(), httpx.Param(r, "id"), RequestLineInput{
		ItemID: body.ItemID, Description: body.Description, Qty: domain.Quantity(body.Qty),
		Unit: body.Unit, EstimatedPriceIDR: body.EstimatedPriceIDR, Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, request)
}

func (h *Handler) removeRequestLine(w http.ResponseWriter, r *http.Request) {
	request, err := h.service.RemoveRequestLine(r.Context(),
		httpx.Param(r, "id"), httpx.Param(r, "lineId"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, request)
}

func (h *Handler) submitRequest(w http.ResponseWriter, r *http.Request) {
	request, err := h.service.SubmitRequest(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, request)
}

func (h *Handler) approveRequest(w http.ResponseWriter, r *http.Request) {
	request, err := h.service.ApproveRequest(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, request)
}

func (h *Handler) rejectRequest(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[reasonRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	request, err := h.service.RejectRequest(r.Context(), httpx.Param(r, "id"), body.Reason, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, request)
}

type convertBody struct {
	SupplierID string `json:"supplierId"`
}

func (c *convertBody) Validate() error {
	if strings.TrimSpace(c.SupplierID) == "" {
		return httpx.Invalid("A supplier is required.")
	}
	return nil
}

func (h *Handler) convertRequest(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[convertBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.ConvertRequest(r.Context(), httpx.Param(r, "id"), body.SupplierID, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, order)
}

// ── Orders ───────────────────────────────────────────────────────────────────

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.service.Orders(r.Context(), OrderFilter{
		BranchID:   httpx.Query(r, "branchId"),
		SupplierID: httpx.Query(r, "supplierId"),
		Status:     strings.ToUpper(httpx.Query(r, "status")),
		Limit:      httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, orders)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.Order(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

type createOrderBody struct {
	SupplierID  string  `json:"supplierId"`
	BranchID    string  `json:"branchId"`
	RequestID   *string `json:"requestId"`
	ExpectedOn  *string `json:"expectedOn"`
	TaxPercent  float64 `json:"taxPercent"`
	DiscountIDR float64 `json:"discountIdr"`
	Terms       *string `json:"terms"`
	ShipTo      *string `json:"shipTo"`
	Note        *string `json:"note"`
}

func (c *createOrderBody) Validate() error {
	if strings.TrimSpace(c.SupplierID) == "" {
		return httpx.Invalid("A supplier is required.")
	}
	if strings.TrimSpace(c.BranchID) == "" {
		return httpx.Invalid("A branch is required.")
	}
	if c.TaxPercent < 0 || c.TaxPercent > 100 {
		return httpx.Invalid("A tax rate runs from 0 to 100 percent.")
	}
	if c.DiscountIDR < 0 {
		return httpx.Invalid("A discount cannot be negative.")
	}
	return nil
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[createOrderBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.CreateOrder(r.Context(), OrderInput{
		SupplierID: body.SupplierID, BranchID: body.BranchID, RequestID: body.RequestID,
		ExpectedOn: optionalDateInput(body.ExpectedOn), TaxPercent: body.TaxPercent,
		DiscountIDR: body.DiscountIDR, Terms: body.Terms, ShipTo: body.ShipTo, Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, order)
}

type orderLineBody struct {
	ItemID       string  `json:"itemId"`
	Description  string  `json:"description"`
	Qty          float64 `json:"qty"`
	Unit         string  `json:"unit"`
	UnitPriceIDR float64 `json:"unitPriceIdr"`
	DiscountIDR  float64 `json:"discountIdr"`
	Note         *string `json:"note"`
}

func (l *orderLineBody) Validate() error {
	if strings.TrimSpace(l.ItemID) == "" {
		return httpx.Invalid("An item is required.")
	}
	if l.Qty <= 0 {
		return httpx.Invalid("A line needs a positive quantity.")
	}
	if l.UnitPriceIDR < 0 || l.DiscountIDR < 0 {
		return httpx.Invalid("Prices and discounts cannot be negative.")
	}
	return nil
}

func (h *Handler) addOrderLine(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[orderLineBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	description := body.Description
	if strings.TrimSpace(description) == "" {
		description = body.ItemID
	}
	order, err := h.service.AddOrderLine(r.Context(), httpx.Param(r, "id"), OrderLineInput{
		ItemID: body.ItemID, Description: description, Qty: domain.Quantity(body.Qty),
		Unit: body.Unit, UnitPriceIDR: body.UnitPriceIDR, DiscountIDR: body.DiscountIDR,
		Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, order)
}

func (h *Handler) removeOrderLine(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.RemoveOrderLine(r.Context(),
		httpx.Param(r, "id"), httpx.Param(r, "lineId"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

type orderTermsBody struct {
	DiscountIDR float64 `json:"discountIdr"`
	TaxPercent  float64 `json:"taxPercent"`
	ExpectedOn  *string `json:"expectedOn"`
	Terms       *string `json:"terms"`
	ShipTo      *string `json:"shipTo"`
	Note        *string `json:"note"`
}

func (t *orderTermsBody) Validate() error {
	if t.TaxPercent < 0 || t.TaxPercent > 100 {
		return httpx.Invalid("A tax rate runs from 0 to 100 percent.")
	}
	if t.DiscountIDR < 0 {
		return httpx.Invalid("A discount cannot be negative.")
	}
	return nil
}

func (h *Handler) setOrderTerms(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[orderTermsBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.SetOrderTerms(r.Context(), httpx.Param(r, "id"), OrderTermsInput{
		DiscountIDR: body.DiscountIDR, TaxPercent: body.TaxPercent,
		ExpectedOn: optionalDateInput(body.ExpectedOn), Terms: body.Terms,
		ShipTo: body.ShipTo, Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

func (h *Handler) approveOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.ApproveOrder(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

func (h *Handler) sendOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.SendOrder(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[reasonRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.CancelOrder(r.Context(), httpx.Param(r, "id"), body.Reason, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

// ── Receipts ─────────────────────────────────────────────────────────────────

func (h *Handler) listReceipts(w http.ResponseWriter, r *http.Request) {
	receipts, err := h.service.Receipts(r.Context(), ReceiptFilter{
		OrderID:  httpx.Query(r, "orderId"),
		BranchID: httpx.Query(r, "branchId"),
		Status:   strings.ToUpper(httpx.Query(r, "status")),
		Limit:    httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, receipts)
}

func (h *Handler) getReceipt(w http.ResponseWriter, r *http.Request) {
	receipt, err := h.service.Receipt(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, receipt)
}

type openReceiptBody struct {
	OrderID            string  `json:"orderId"`
	DeliveryNoteNumber *string `json:"deliveryNoteNumber"`
	// The arrival this receipt is inspecting, when there was one.
	DeliveryID *string `json:"deliveryId"`
	Note       *string `json:"note"`
}

func (o *openReceiptBody) Validate() error {
	if strings.TrimSpace(o.OrderID) == "" {
		return httpx.Invalid("An order is required.")
	}
	return nil
}

func (h *Handler) openReceipt(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[openReceiptBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	receipt, err := h.service.OpenReceipt(r.Context(), body.OrderID,
		body.DeliveryNoteNumber, body.DeliveryID, body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, receipt)
}

type receiptLineBody struct {
	OrderItemID string  `json:"orderItemId"`
	QtyAccepted float64 `json:"qtyAccepted"`
	QtyRejected float64 `json:"qtyRejected"`
	BatchNumber *string `json:"batchNumber"`
	ExpiresOn   *string `json:"expiresOn"`
	Note        *string `json:"note"`
}

func (l *receiptLineBody) Validate() error {
	if strings.TrimSpace(l.OrderItemID) == "" {
		return httpx.Invalid("An order line is required.")
	}
	if l.QtyAccepted < 0 || l.QtyRejected < 0 {
		return httpx.Invalid("Quantities cannot be negative.")
	}
	if l.QtyAccepted == 0 && l.QtyRejected == 0 {
		return httpx.Invalid("A delivery line needs something on it, accepted or rejected.")
	}
	if l.ExpiresOn != nil && *l.ExpiresOn != "" {
		if _, err := domain.ParseDate(*l.ExpiresOn); err != nil {
			return httpx.Invalid("expiresOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) addReceiptLine(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[receiptLineBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	receipt, err := h.service.AddReceiptLine(r.Context(), httpx.Param(r, "id"), ReceiveLineInput{
		OrderItemID: body.OrderItemID,
		QtyAccepted: domain.Quantity(body.QtyAccepted),
		QtyRejected: domain.Quantity(body.QtyRejected),
		BatchNumber: body.BatchNumber, ExpiresOn: optionalDateInput(body.ExpiresOn), Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, receipt)
}

func (h *Handler) removeReceiptLine(w http.ResponseWriter, r *http.Request) {
	receipt, err := h.service.RemoveReceiptLine(r.Context(),
		httpx.Param(r, "id"), httpx.Param(r, "lineId"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, receipt)
}

func (h *Handler) postReceipt(w http.ResponseWriter, r *http.Request) {
	receipt, err := h.service.PostReceipt(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, receipt)
}

func (h *Handler) cancelReceipt(w http.ResponseWriter, r *http.Request) {
	receipt, err := h.service.CancelReceipt(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, receipt)
}

// ── Returns ──────────────────────────────────────────────────────────────────

func (h *Handler) listReturns(w http.ResponseWriter, r *http.Request) {
	returns, err := h.service.Returns(r.Context(),
		httpx.Query(r, "receiptId"), strings.ToUpper(httpx.Query(r, "status")),
		httpx.QueryInt(r, "limit", 200))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, returns)
}

func (h *Handler) getReturn(w http.ResponseWriter, r *http.Request) {
	ret, err := h.service.Return(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, ret)
}

type openReturnBody struct {
	ReceiptID  string  `json:"receiptId"`
	ReasonType string  `json:"reasonType"`
	ReasonNote *string `json:"reasonNote"`
}

func (o *openReturnBody) Validate() error {
	if strings.TrimSpace(o.ReceiptID) == "" {
		return httpx.Invalid("A delivery is required.")
	}
	if !domain.IsValidReturnReason(strings.ToUpper(o.ReasonType)) {
		return httpx.Invalid("%q is not a return reason.", o.ReasonType)
	}
	return nil
}

func (h *Handler) openReturn(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[openReturnBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ret, err := h.service.OpenReturn(r.Context(), body.ReceiptID,
		domain.ReturnReason(strings.ToUpper(body.ReasonType)), body.ReasonNote, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, ret)
}

type returnLineBody struct {
	ReceiptItemID string  `json:"receiptItemId"`
	Qty           float64 `json:"qty"`
	Note          *string `json:"note"`
}

func (l *returnLineBody) Validate() error {
	if strings.TrimSpace(l.ReceiptItemID) == "" {
		return httpx.Invalid("A delivery line is required.")
	}
	if l.Qty <= 0 {
		return httpx.Invalid("A return line needs a positive quantity.")
	}
	return nil
}

func (h *Handler) addReturnLine(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[returnLineBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ret, err := h.service.AddReturnLine(r.Context(), httpx.Param(r, "id"), ReturnLineInput{
		ReceiptItemID: body.ReceiptItemID, Qty: domain.Quantity(body.Qty), Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, ret)
}

func (h *Handler) postReturn(w http.ResponseWriter, r *http.Request) {
	ret, err := h.service.PostReturn(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, ret)
}
