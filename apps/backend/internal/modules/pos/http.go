package pos

import (
	"net/http"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the counter.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }
	view := admin(domain.PermPOSView)
	sell := admin(domain.PermPOSSell)
	manage := admin(domain.PermPOSManage)
	void := admin(domain.PermPOSVoid)

	r.Get("/api/admin/pos/overview", h.overview, view)

	r.Get("/api/admin/pos/categories", h.listCategories, view)
	r.Put("/api/admin/pos/categories", h.saveCategory, manage)
	r.Get("/api/admin/pos/products", h.listProducts, view)
	r.Put("/api/admin/pos/products", h.saveProduct, manage)

	r.Get("/api/admin/pos/shifts", h.listShifts, view)
	r.Get("/api/admin/pos/shifts/{id}", h.getShift, view)
	r.Post("/api/admin/pos/shifts", h.openShift, sell)
	r.Post("/api/admin/pos/shifts/{id}/close", h.closeShift, sell)

	r.Get("/api/admin/pos/orders", h.listOrders, view)
	r.Post("/api/admin/pos/orders", h.openOrder, sell)
	r.Get("/api/admin/pos/orders/{id}", h.getOrder, view)
	r.Put("/api/admin/pos/orders/{id}", h.setOrderDetails, sell)
	r.Post("/api/admin/pos/orders/{id}/lines", h.addLine, sell)
	r.Delete("/api/admin/pos/orders/{id}/lines/{lineId}", h.removeLine, sell)
	r.Post("/api/admin/pos/orders/{id}/tender", h.tender, sell)
	r.Post("/api/admin/pos/orders/{id}/complete", h.complete, sell)
	r.Post("/api/admin/pos/orders/{id}/cancel", h.cancel, sell)
	// Unwinding a paid sale is the one action that makes money disappear, so
	// it needs a manager rather than whoever is at the till.
	r.Post("/api/admin/pos/orders/{id}/void", h.void, void)
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.service.Overview(r.Context(), httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, overview)
}

// ── Menu ─────────────────────────────────────────────────────────────────────

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.Categories(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, categories)
}

type categoryBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	Active    *bool  `json:"active"`
}

func (c *categoryBody) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return httpx.Invalid("A category needs a name.")
	}
	return nil
}

func (h *Handler) saveCategory(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[categoryBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	category, err := h.service.SaveCategory(r.Context(), CategoryInput{
		ID: body.ID, Name: body.Name, SortOrder: body.SortOrder,
		Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, category)
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.service.Products(r.Context(), ProductFilter{
		Query:        httpx.Query(r, "query"),
		CategoryID:   httpx.Query(r, "categoryId"),
		SellableOnly: httpx.QueryBool(r, "sellableOnly", false),
		Limit:        httpx.QueryInt(r, "limit", 300),
	}, httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, products)
}

type productBody struct {
	SKU             string  `json:"sku"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	CategoryID      *string `json:"categoryId"`
	InventoryItemID *string `json:"inventoryItemId"`
	PriceIDR        float64 `json:"priceIdr"`
	TaxPercent      float64 `json:"taxPercent"`
	BonusXP         int     `json:"bonusXp"`
	ImageURL        *string `json:"imageUrl"`
	Active          *bool   `json:"active"`
	Available       *bool   `json:"available"`
}

func (p *productBody) Validate() error {
	if strings.TrimSpace(p.SKU) == "" || strings.TrimSpace(p.Name) == "" {
		return httpx.Invalid("A product needs a SKU and a name.")
	}
	if p.PriceIDR < 0 {
		return httpx.Invalid("A price cannot be negative.")
	}
	if p.TaxPercent < 0 || p.TaxPercent > 100 {
		return httpx.Invalid("A tax rate runs from 0 to 100 percent.")
	}
	if p.BonusXP < 0 {
		return httpx.Invalid("Bonus points cannot be negative.")
	}
	return nil
}

func (h *Handler) saveProduct(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[productBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	product, err := h.service.SaveProduct(r.Context(), ProductInput{
		SKU: body.SKU, Name: body.Name, Description: body.Description,
		CategoryID: body.CategoryID, InventoryItemID: body.InventoryItemID,
		PriceIDR: body.PriceIDR, TaxPercent: body.TaxPercent, BonusXP: body.BonusXP,
		ImageURL:  body.ImageURL,
		Active:    body.Active == nil || *body.Active,
		Available: body.Available == nil || *body.Available,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, product)
}

// ── Shifts ───────────────────────────────────────────────────────────────────

func (h *Handler) listShifts(w http.ResponseWriter, r *http.Request) {
	shifts, err := h.service.Shifts(r.Context(),
		httpx.Query(r, "branchId"), httpx.Query(r, "cashierId"),
		strings.ToUpper(httpx.Query(r, "status")), httpx.QueryInt(r, "limit", 100))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, shifts)
}

func (h *Handler) getShift(w http.ResponseWriter, r *http.Request) {
	shift, err := h.service.Shift(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, shift)
}

type openShiftBody struct {
	BranchID       string  `json:"branchId"`
	OpeningCashIDR float64 `json:"openingCashIdr"`
	Note           *string `json:"note"`
}

func (o *openShiftBody) Validate() error {
	if strings.TrimSpace(o.BranchID) == "" {
		return httpx.Invalid("A branch is required.")
	}
	if o.OpeningCashIDR < 0 {
		return httpx.Invalid("An opening float cannot be negative.")
	}
	return nil
}

func (h *Handler) openShift(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[openShiftBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	shift, err := h.service.OpenShift(r.Context(), body.BranchID, body.OpeningCashIDR,
		body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, shift)
}

type closeShiftBody struct {
	CountedCashIDR float64 `json:"countedCashIdr"`
	Note           *string `json:"note"`
}

func (c *closeShiftBody) Validate() error {
	if c.CountedCashIDR < 0 {
		return httpx.Invalid("A counted drawer cannot be negative.")
	}
	return nil
}

func (h *Handler) closeShift(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[closeShiftBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	shift, err := h.service.CloseShift(r.Context(), httpx.Param(r, "id"),
		body.CountedCashIDR, body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, shift)
}

// ── Orders ───────────────────────────────────────────────────────────────────

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.service.Orders(r.Context(), OrderFilter{
		BranchID: httpx.Query(r, "branchId"),
		ShiftID:  httpx.Query(r, "shiftId"),
		MemberID: httpx.Query(r, "memberId"),
		Status:   strings.ToUpper(httpx.Query(r, "status")),
		Limit:    httpx.QueryInt(r, "limit", 200),
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

type openOrderBody struct {
	BranchID  string  `json:"branchId"`
	MemberID  *string `json:"memberId"`
	OrderType string  `json:"orderType"`
	Note      *string `json:"note"`
}

func (o *openOrderBody) Validate() error {
	if strings.TrimSpace(o.BranchID) == "" {
		return httpx.Invalid("A branch is required.")
	}
	switch strings.ToUpper(o.OrderType) {
	case "", "COUNTER", "TAKEAWAY", "DINE_IN":
	default:
		return httpx.Invalid("%q is not an order type.", o.OrderType)
	}
	return nil
}

func (h *Handler) openOrder(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[openOrderBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.OpenOrder(r.Context(), body.BranchID, body.MemberID,
		body.OrderType, body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, order)
}

type lineBody struct {
	ProductID   string  `json:"productId"`
	Qty         float64 `json:"qty"`
	DiscountIDR float64 `json:"discountIdr"`
	Note        *string `json:"note"`
}

func (l *lineBody) Validate() error {
	if strings.TrimSpace(l.ProductID) == "" {
		return httpx.Invalid("A product is required.")
	}
	if l.Qty <= 0 {
		return httpx.Invalid("A line needs a positive quantity.")
	}
	if l.DiscountIDR < 0 {
		return httpx.Invalid("A discount cannot be negative.")
	}
	return nil
}

func (h *Handler) addLine(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[lineBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.AddLine(r.Context(), httpx.Param(r, "id"), LineInput{
		ProductID: body.ProductID, Qty: domain.Quantity(body.Qty),
		DiscountIDR: body.DiscountIDR, Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, order)
}

func (h *Handler) removeLine(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.RemoveLine(r.Context(),
		httpx.Param(r, "id"), httpx.Param(r, "lineId"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

type orderDetailsBody struct {
	MemberID       *string `json:"memberId"`
	SetMember      bool    `json:"setMember"`
	DiscountIDR    float64 `json:"discountIdr"`
	DiscountReason *string `json:"discountReason"`
	Note           *string `json:"note"`
}

func (o *orderDetailsBody) Validate() error {
	if o.DiscountIDR < 0 {
		return httpx.Invalid("A discount cannot be negative.")
	}
	return nil
}

func (h *Handler) setOrderDetails(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[orderDetailsBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.SetOrderDetails(r.Context(), httpx.Param(r, "id"), OrderDetailsInput{
		MemberID: body.MemberID, SetMember: body.SetMember,
		DiscountIDR: body.DiscountIDR, DiscountReason: body.DiscountReason, Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

type tenderBody struct {
	Method    string  `json:"method"`
	AmountIDR float64 `json:"amountIdr"`
	Reference *string `json:"reference"`
}

func (t *tenderBody) Validate() error {
	if !domain.IsValidPaymentMethod(strings.ToUpper(t.Method)) {
		return httpx.Invalid("%q is not a payment method.", t.Method)
	}
	if t.AmountIDR <= 0 {
		return httpx.Invalid("A tender needs a positive amount.")
	}
	return nil
}

func (h *Handler) tender(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[tenderBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.Tender(r.Context(), httpx.Param(r, "id"), TenderInput{
		Method:    domain.POSPaymentMethod(strings.ToUpper(body.Method)),
		AmountIDR: body.AmountIDR, Reference: body.Reference,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, order)
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.Complete(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.Cancel(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

type voidBody struct {
	Reason string `json:"reason"`
}

func (v *voidBody) Validate() error {
	if strings.TrimSpace(v.Reason) == "" {
		return httpx.Invalid("Voiding a sale needs a reason.")
	}
	return nil
}

func (h *Handler) void(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[voidBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.Void(r.Context(), httpx.Param(r, "id"), body.Reason, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}
