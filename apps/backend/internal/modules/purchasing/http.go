package purchasing

import (
	"net/http"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Handler serves the buying surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }
	view := admin(domain.PermPurchasingView)
	manage := admin(domain.PermPurchasingManage)
	approve := admin(domain.PermPurchasingApprove)
	receive := admin(domain.PermPurchasingReceive)
	pay := admin(domain.PermPurchasingPay)

	r.Get("/api/admin/purchasing/overview", h.overview, view)

	r.Get("/api/admin/purchasing/suppliers", h.listSuppliers, view)
	r.Post("/api/admin/purchasing/suppliers", h.createSupplier, manage)
	r.Get("/api/admin/purchasing/suppliers/{id}", h.getSupplier, view)
	r.Put("/api/admin/purchasing/suppliers/{id}", h.updateSupplier, manage)
	r.Delete("/api/admin/purchasing/suppliers/{id}", h.deleteSupplier, manage)
	r.Get("/api/admin/purchasing/suppliers/{id}/prices", h.listPrices, view)
	r.Post("/api/admin/purchasing/suppliers/{id}/prices", h.setPrice, manage)

	r.Get("/api/admin/purchasing/requests", h.listRequests, view)
	r.Post("/api/admin/purchasing/requests", h.createRequest, manage)
	r.Get("/api/admin/purchasing/requests/{id}", h.getRequest, view)
	r.Post("/api/admin/purchasing/requests/{id}/lines", h.addRequestLine, manage)
	r.Delete("/api/admin/purchasing/requests/{id}/lines/{lineId}", h.removeRequestLine, manage)
	r.Post("/api/admin/purchasing/requests/{id}/submit", h.submitRequest, manage)
	// Approving is its own grant, and the level signed depends on the role.
	r.Post("/api/admin/purchasing/requests/{id}/approve", h.approveRequest, approve)
	r.Post("/api/admin/purchasing/requests/{id}/reject", h.rejectRequest, approve)
	r.Post("/api/admin/purchasing/requests/{id}/convert", h.convertRequest, manage)

	r.Get("/api/admin/purchasing/orders", h.listOrders, view)
	r.Post("/api/admin/purchasing/orders", h.createOrder, manage)
	r.Get("/api/admin/purchasing/orders/{id}", h.getOrder, view)
	r.Put("/api/admin/purchasing/orders/{id}/terms", h.setOrderTerms, manage)
	r.Post("/api/admin/purchasing/orders/{id}/lines", h.addOrderLine, manage)
	r.Delete("/api/admin/purchasing/orders/{id}/lines/{lineId}", h.removeOrderLine, manage)
	r.Post("/api/admin/purchasing/orders/{id}/approve", h.approveOrder, approve)
	r.Post("/api/admin/purchasing/orders/{id}/send", h.sendOrder, manage)
	r.Post("/api/admin/purchasing/orders/{id}/cancel", h.cancelOrder, manage)

	// Signing for goods is a third grant again: whoever is at the back door.
	r.Get("/api/admin/purchasing/receipts", h.listReceipts, view)
	r.Post("/api/admin/purchasing/receipts", h.openReceipt, receive)
	r.Get("/api/admin/purchasing/receipts/{id}", h.getReceipt, view)
	r.Post("/api/admin/purchasing/receipts/{id}/lines", h.addReceiptLine, receive)
	r.Delete("/api/admin/purchasing/receipts/{id}/lines/{lineId}", h.removeReceiptLine, receive)
	r.Post("/api/admin/purchasing/receipts/{id}/post", h.postReceipt, receive)
	r.Post("/api/admin/purchasing/receipts/{id}/cancel", h.cancelReceipt, receive)

	r.Get("/api/admin/purchasing/returns", h.listReturns, view)
	r.Post("/api/admin/purchasing/returns", h.openReturn, receive)
	r.Get("/api/admin/purchasing/returns/{id}", h.getReturn, view)
	r.Post("/api/admin/purchasing/returns/{id}/lines", h.addReturnLine, receive)
	// Sending goods back is somebody's signature, not the receiving bay's
	// decision, so submitting and approving are separate grants.
	r.Post("/api/admin/purchasing/returns/{id}/submit", h.submitReturn, receive)
	r.Post("/api/admin/purchasing/returns/{id}/approve", h.approveReturn, approve)
	r.Post("/api/admin/purchasing/returns/{id}/reject", h.rejectReturn, approve)
	// A rejected return goes back to whoever raised it to be corrected.
	r.Post("/api/admin/purchasing/returns/{id}/revise", h.reviseReturn, receive)
	r.Post("/api/admin/purchasing/returns/{id}/post", h.postReturn, receive)

	// The receiving bay: what came off the truck, before anybody judged it.
	r.Get("/api/admin/purchasing/deliveries", h.listDeliveries, view)
	r.Post("/api/admin/purchasing/deliveries", h.openDelivery, receive)
	r.Get("/api/admin/purchasing/deliveries/{id}", h.getDelivery, view)
	r.Post("/api/admin/purchasing/deliveries/{id}/lines", h.addDeliveryLine, receive)
	r.Post("/api/admin/purchasing/deliveries/{id}/close", h.closeDelivery, receive)

	// The money. Paying a supplier is a finance grant, not a buying one.
	r.Get("/api/admin/purchasing/orders/{id}/payables", h.getPayables, view)
	r.Post("/api/admin/purchasing/orders/{id}/payables/schedule", h.scheduleTerms, pay)
	r.Put("/api/admin/purchasing/orders/{id}/payables/terms", h.saveTerms, pay)
	r.Get("/api/admin/purchasing/payments", h.listPayments, view)
	r.Post("/api/admin/purchasing/payments", h.recordPayment, pay)
	r.Post("/api/admin/purchasing/payments/{id}/post", h.postPayment, pay)
	r.Post("/api/admin/purchasing/payments/{id}/void", h.voidPayment, pay)
	r.Get("/api/admin/purchasing/credits", h.listCredits, view)
	r.Post("/api/admin/purchasing/credits", h.raiseCredit, pay)

	// Spreadsheets in and out.
	r.Post("/api/admin/purchasing/suppliers/import", h.importSuppliers, manage)
	r.Get("/api/admin/purchasing/suppliers/export", h.exportSuppliers, view)
	r.Post("/api/admin/purchasing/suppliers/{id}/prices/import", h.importPrices, manage)
	r.Get("/api/admin/purchasing/orders/export", h.exportOrders, view)

	// What buying cost, and how suppliers actually behaved.
	r.Get("/api/admin/purchasing/reports/orders", h.reportOrders, view)
	r.Get("/api/admin/purchasing/reports/suppliers", h.reportSuppliers, view)
	r.Get("/api/admin/purchasing/reports/price-history", h.reportPriceHistory, view)
}

// actorFrom carries the role as well as the identity: the approval chain is
// decided by what office somebody holds, not only by whether they got past the
// permission check.
func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.ID, Role: domain.AdminRole(principal.Role)}
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.Overview(r.Context(), httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summary)
}

// ── Suppliers ────────────────────────────────────────────────────────────────

func (h *Handler) listSuppliers(w http.ResponseWriter, r *http.Request) {
	suppliers, err := h.service.Suppliers(r.Context(), SupplierFilter{
		Query:  httpx.Query(r, "query"),
		Status: strings.ToUpper(httpx.Query(r, "status")),
		Limit:  httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, suppliers)
}

func (h *Handler) getSupplier(w http.ResponseWriter, r *http.Request) {
	supplier, err := h.service.Supplier(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, supplier)
}

type supplierRequest struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	ContactName  *string `json:"contactName"`
	ContactPhone *string `json:"contactPhone"`
	Email        *string `json:"email"`
	Address      *string `json:"address"`
	City         *string `json:"city"`
	TaxNumber    *string `json:"taxNumber"`
	PaymentTerms string  `json:"paymentTerms"`
	BankName     *string `json:"bankName"`
	BankAccount  *string `json:"bankAccount"`
	BankHolder   *string `json:"bankHolder"`
	Category     *string `json:"category"`
	Status       string  `json:"status"`
	Note         *string `json:"note"`
}

func (s *supplierRequest) Validate() error {
	if strings.TrimSpace(s.Code) == "" {
		return httpx.Invalid("A supplier needs a code.")
	}
	if strings.TrimSpace(s.Name) == "" {
		return httpx.Invalid("A supplier needs a name.")
	}
	if s.PaymentTerms != "" && !domain.IsValidPaymentTerms(strings.ToUpper(s.PaymentTerms)) {
		return httpx.Invalid("%q is not a set of payment terms.", s.PaymentTerms)
	}
	return nil
}

func (s supplierRequest) toInput() SupplierInput {
	terms := domain.PaymentTerms(strings.ToUpper(s.PaymentTerms))
	if terms == "" {
		terms = domain.TermsNet30
	}
	status := domain.SupplierStatus(strings.ToUpper(s.Status))
	if status == "" {
		status = domain.SupplierActive
	}
	return SupplierInput{
		Code: s.Code, Name: s.Name, ContactName: s.ContactName, ContactPhone: s.ContactPhone,
		Email: s.Email, Address: s.Address, City: s.City, TaxNumber: s.TaxNumber,
		PaymentTerms: terms, BankName: s.BankName, BankAccount: s.BankAccount,
		BankHolder: s.BankHolder, Category: s.Category, Status: status, Note: s.Note,
	}
}

func (h *Handler) createSupplier(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[supplierRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := h.service.CreateSupplier(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) updateSupplier(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[supplierRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := h.service.UpdateSupplier(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, updated)
}

func (h *Handler) deleteSupplier(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteSupplier(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

func (h *Handler) listPrices(w http.ResponseWriter, r *http.Request) {
	prices, err := h.service.SupplierPrices(r.Context(), httpx.Param(r, "id"), httpx.Query(r, "itemId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, prices)
}

type priceRequest struct {
	ItemID        string  `json:"itemId"`
	UnitPriceIDR  float64 `json:"unitPriceIdr"`
	MinOrderQty   float64 `json:"minOrderQty"`
	LeadTimeDays  int     `json:"leadTimeDays"`
	EffectiveFrom string  `json:"effectiveFrom"`
}

func (p *priceRequest) Validate() error {
	if strings.TrimSpace(p.ItemID) == "" {
		return httpx.Invalid("An item is required.")
	}
	if p.UnitPriceIDR < 0 {
		return httpx.Invalid("A price cannot be negative.")
	}
	if p.EffectiveFrom != "" {
		if _, err := domain.ParseDate(p.EffectiveFrom); err != nil {
			return httpx.Invalid("effectiveFrom must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) setPrice(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[priceRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	from, _ := domain.ParseDate(body.EffectiveFrom)
	price, err := h.service.SetSupplierPrice(r.Context(), httpx.Param(r, "id"), PriceInput{
		ItemID: body.ItemID, UnitPriceIDR: body.UnitPriceIDR,
		MinOrderQty: domain.Quantity(body.MinOrderQty), LeadTimeDays: body.LeadTimeDays,
		EffectiveFrom: from,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, price)
}
