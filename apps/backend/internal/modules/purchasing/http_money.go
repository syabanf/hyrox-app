package purchasing

import (
	"net/http"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// ── Deliveries ───────────────────────────────────────────────────────────────

func (h *Handler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	deliveries, err := h.service.Deliveries(r.Context(), DeliveryFilter{
		OrderID:    httpx.Query(r, "orderId"),
		SupplierID: httpx.Query(r, "supplierId"),
		BranchID:   httpx.Query(r, "branchId"),
		Status:     strings.ToUpper(httpx.Query(r, "status")),
		Limit:      httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, deliveries)
}

func (h *Handler) getDelivery(w http.ResponseWriter, r *http.Request) {
	delivery, err := h.service.Delivery(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, delivery)
}

type deliveryBody struct {
	OrderID            string  `json:"orderId"`
	DeliveryNoteNumber *string `json:"deliveryNoteNumber"`
	DriverName         *string `json:"driverName"`
	Vehicle            *string `json:"vehicle"`
	ArrivedOn          *string `json:"arrivedOn"`
	Note               *string `json:"note"`
}

func (d *deliveryBody) Validate() error {
	if strings.TrimSpace(d.OrderID) == "" {
		return httpx.Invalid("A delivery arrives against an order.")
	}
	if d.ArrivedOn != nil {
		if _, err := domain.ParseDate(*d.ArrivedOn); err != nil {
			return httpx.Invalid("arrivedOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) openDelivery(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[deliveryBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var arrivedOn domain.Date
	if parsed := optionalDateInput(body.ArrivedOn); parsed != nil {
		arrivedOn = *parsed
	}
	delivery, err := h.service.OpenDelivery(r.Context(), DeliveryInput{
		OrderID: body.OrderID, DeliveryNoteNumber: body.DeliveryNoteNumber,
		DriverName: body.DriverName, Vehicle: body.Vehicle,
		ArrivedOn: arrivedOn, Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, delivery)
}

type deliveryLineBody struct {
	OrderItemID string  `json:"orderItemId"`
	Qty         float64 `json:"qty"`
	BatchNumber *string `json:"batchNumber"`
	ExpiresOn   *string `json:"expiresOn"`
	Note        *string `json:"note"`
}

func (d *deliveryLineBody) Validate() error {
	if strings.TrimSpace(d.OrderItemID) == "" {
		return httpx.Invalid("Which order line arrived?")
	}
	if d.Qty <= 0 {
		return httpx.Invalid("A delivery line needs a positive quantity.")
	}
	if d.ExpiresOn != nil {
		if _, err := domain.ParseDate(*d.ExpiresOn); err != nil {
			return httpx.Invalid("expiresOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) addDeliveryLine(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[deliveryLineBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	delivery, err := h.service.AddDeliveryLine(r.Context(), httpx.Param(r, "id"), DeliveryLineInput{
		OrderItemID: body.OrderItemID, Qty: domain.Quantity(body.Qty),
		BatchNumber: body.BatchNumber, ExpiresOn: optionalDateInput(body.ExpiresOn),
		Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, delivery)
}

func (h *Handler) closeDelivery(w http.ResponseWriter, r *http.Request) {
	delivery, err := h.service.CloseDelivery(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, delivery)
}

// ── Instalments and payments ─────────────────────────────────────────────────

func (h *Handler) getPayables(w http.ResponseWriter, r *http.Request) {
	payables, err := h.service.Payables(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, payables)
}

func (h *Handler) scheduleTerms(w http.ResponseWriter, r *http.Request) {
	payables, err := h.service.ScheduleFromTerms(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, payables)
}

type termsBody struct {
	Terms []struct {
		Sequence  int      `json:"sequence"`
		Label     string   `json:"label"`
		DueOn     string   `json:"dueOn"`
		Percent   *float64 `json:"percent"`
		AmountIDR float64  `json:"amountIdr"`
		Note      *string  `json:"note"`
	} `json:"terms"`
}

func (t *termsBody) Validate() error {
	if len(t.Terms) == 0 {
		return httpx.Invalid("An order needs at least one instalment.")
	}
	var percent float64
	for _, term := range t.Terms {
		if _, err := domain.ParseDate(term.DueOn); err != nil {
			return httpx.Invalid("Every instalment needs a due date as YYYY-MM-DD.")
		}
		if term.Percent != nil {
			if *term.Percent <= 0 || *term.Percent > 100 {
				return httpx.Invalid("An instalment's share runs from just above 0 to 100 percent.")
			}
			percent += *term.Percent
		}
	}
	// A schedule that does not add up to the whole order leaves money nobody
	// has agreed to pay, which is worse than no schedule at all.
	if percent > 0 && (percent < 99.9 || percent > 100.1) {
		return httpx.Invalid("The instalments add up to %.1f%% of the order, not 100%%.", percent)
	}
	return nil
}

func (h *Handler) saveTerms(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[termsBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	terms := make([]TermInput, 0, len(body.Terms))
	for _, term := range body.Terms {
		due, _ := domain.ParseDate(term.DueOn)
		terms = append(terms, TermInput{
			Sequence: term.Sequence, Label: term.Label, DueOn: due,
			Percent: term.Percent, AmountIDR: term.AmountIDR, Note: term.Note,
		})
	}
	payables, err := h.service.SaveTerms(r.Context(), httpx.Param(r, "id"), terms, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, payables)
}

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	payments, err := h.service.VendorPayments(r.Context(), PaymentFilter{
		SupplierID: httpx.Query(r, "supplierId"),
		OrderID:    httpx.Query(r, "orderId"),
		Status:     strings.ToUpper(httpx.Query(r, "status")),
		Limit:      httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, payments)
}

type paymentBody struct {
	SupplierID string  `json:"supplierId"`
	OrderID    *string `json:"orderId"`
	TermID     *string `json:"termId"`
	PaidOn     *string `json:"paidOn"`
	AmountIDR  float64 `json:"amountIdr"`
	Method     string  `json:"method"`
	Reference  *string `json:"reference"`
	Note       *string `json:"note"`
	UseCredits bool    `json:"useCredits"`
}

func (p *paymentBody) Validate() error {
	if strings.TrimSpace(p.SupplierID) == "" {
		return httpx.Invalid("Who is being paid?")
	}
	if p.AmountIDR <= 0 {
		return httpx.Invalid("A payment of nothing is not a payment.")
	}
	if p.Method != "" {
		switch strings.ToUpper(p.Method) {
		case "TRANSFER", "CASH", "CHEQUE", "CARD", "CREDIT_NOTE":
		default:
			return httpx.Invalid("%q is not a way of paying a supplier.", p.Method)
		}
	}
	if p.PaidOn != nil {
		if _, err := domain.ParseDate(*p.PaidOn); err != nil {
			return httpx.Invalid("paidOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) recordPayment(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[paymentBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var paidOn domain.Date
	if parsed := optionalDateInput(body.PaidOn); parsed != nil {
		paidOn = *parsed
	}
	payment, err := h.service.RecordPayment(r.Context(), PaymentInput{
		SupplierID: body.SupplierID, OrderID: body.OrderID, TermID: body.TermID,
		PaidOn: paidOn, AmountIDR: body.AmountIDR, Method: body.Method,
		Reference: body.Reference, Note: body.Note, UseCredits: body.UseCredits,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, payment)
}

func (h *Handler) postPayment(w http.ResponseWriter, r *http.Request) {
	payment, err := h.service.PostPayment(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, payment)
}

type voidPaymentBody struct {
	Reason string `json:"reason"`
}

func (v *voidPaymentBody) Validate() error {
	if strings.TrimSpace(v.Reason) == "" {
		return httpx.Invalid("Voiding a payment needs a reason.")
	}
	return nil
}

func (h *Handler) voidPayment(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[voidPaymentBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	payment, err := h.service.VoidPayment(r.Context(), httpx.Param(r, "id"), body.Reason, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, payment)
}

// ── Credit notes ─────────────────────────────────────────────────────────────

func (h *Handler) listCredits(w http.ResponseWriter, r *http.Request) {
	credits, err := h.service.VendorCredits(r.Context(), CreditFilter{
		SupplierID: httpx.Query(r, "supplierId"),
		Status:     strings.ToUpper(httpx.Query(r, "status")),
		OpenOnly:   httpx.QueryBool(r, "openOnly", false),
		Limit:      httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, credits)
}

type creditBody struct {
	SupplierID string  `json:"supplierId"`
	ReturnID   *string `json:"returnId"`
	AmountIDR  float64 `json:"amountIdr"`
	Reason     *string `json:"reason"`
	ExpiresOn  *string `json:"expiresOn"`
}

func (c *creditBody) Validate() error {
	if strings.TrimSpace(c.SupplierID) == "" {
		return httpx.Invalid("Which supplier owes this?")
	}
	if c.AmountIDR <= 0 {
		return httpx.Invalid("A credit note of nothing is not a credit note.")
	}
	if c.ExpiresOn != nil {
		if _, err := domain.ParseDate(*c.ExpiresOn); err != nil {
			return httpx.Invalid("expiresOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) raiseCredit(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[creditBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	credit, err := h.service.RaiseCredit(r.Context(), CreditInput{
		SupplierID: body.SupplierID, ReturnID: body.ReturnID, AmountIDR: body.AmountIDR,
		Reason: body.Reason, ExpiresOn: optionalDateInput(body.ExpiresOn),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, credit)
}

// ── Return decisions ─────────────────────────────────────────────────────────

type decisionBody struct {
	Note *string `json:"note"`
}

func (h *Handler) submitReturn(w http.ResponseWriter, r *http.Request) {
	ret, err := h.service.SubmitReturn(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, ret)
}

func (h *Handler) reviseReturn(w http.ResponseWriter, r *http.Request) {
	ret, err := h.service.ReviseReturn(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, ret)
}

func (h *Handler) approveReturn(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[decisionBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ret, err := h.service.ApproveReturn(r.Context(), httpx.Param(r, "id"), body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, ret)
}

func (h *Handler) rejectReturn(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[decisionBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ret, err := h.service.RejectReturn(r.Context(), httpx.Param(r, "id"), body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, ret)
}
