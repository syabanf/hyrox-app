package pos

import (
	"net/http"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// ── Promotions ───────────────────────────────────────────────────────────────

func (h *Handler) listPromotions(w http.ResponseWriter, r *http.Request) {
	promotions, err := h.service.Promotions(r.Context(), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, promotions)
}

type promotionBody struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Kind             string   `json:"kind"`
	Percent          *float64 `json:"percent"`
	AmountIDR        *float64 `json:"amountIdr"`
	BuyQty           *float64 `json:"buyQty"`
	FreeQty          *float64 `json:"freeQty"`
	BundlePriceIDR   *float64 `json:"bundlePriceIdr"`
	MinSpendIDR      float64  `json:"minSpendIdr"`
	MinQty           float64  `json:"minQty"`
	RequiresCode     bool     `json:"requiresCode"`
	Channels         []string `json:"channels"`
	Exclusive        bool     `json:"exclusive"`
	Priority         int      `json:"priority"`
	StartsOn         *string  `json:"startsOn"`
	EndsOn           *string  `json:"endsOn"`
	MaxUses          *int     `json:"maxUses"`
	MaxUsesPerMember *int     `json:"maxUsesPerMember"`
	Active           *bool    `json:"active"`
	Targets          []struct {
		ProductID  *string `json:"productId"`
		CategoryID *string `json:"categoryId"`
		Qty        float64 `json:"qty"`
	} `json:"targets"`
}

func (p *promotionBody) Validate() error {
	if strings.TrimSpace(p.Code) == "" || strings.TrimSpace(p.Name) == "" {
		return httpx.Invalid("An offer needs a code and a name.")
	}
	if !domain.IsValidPromotionKind(strings.ToUpper(p.Kind)) {
		return httpx.Invalid("%q is not a kind of offer.", p.Kind)
	}
	for _, raw := range []*string{p.StartsOn, p.EndsOn} {
		if raw == nil {
			continue
		}
		if _, err := domain.ParseDate(*raw); err != nil {
			return httpx.Invalid("An offer's dates are YYYY-MM-DD.")
		}
	}
	for _, target := range p.Targets {
		if (target.ProductID == nil) == (target.CategoryID == nil) {
			return httpx.Invalid("A target is either a product or a category, not both.")
		}
	}
	return nil
}

func (h *Handler) savePromotion(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[promotionBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	promotion := domain.Promotion{
		Code: body.Code, Name: body.Name, Description: body.Description,
		Kind:    domain.PromotionKind(strings.ToUpper(body.Kind)),
		Percent: body.Percent, AmountIDR: body.AmountIDR,
		BundlePriceIDR: body.BundlePriceIDR,
		MinSpendIDR:    body.MinSpendIDR, MinQty: domain.Quantity(body.MinQty),
		RequiresCode: body.RequiresCode, Channels: body.Channels,
		Exclusive: body.Exclusive, Priority: body.Priority,
		StartsOn: parsePromoDate(body.StartsOn), EndsOn: parsePromoDate(body.EndsOn),
		MaxUses: body.MaxUses, MaxUsesPerMember: body.MaxUsesPerMember,
		Active: body.Active == nil || *body.Active,
	}
	if body.BuyQty != nil {
		buy := domain.Quantity(*body.BuyQty)
		promotion.BuyQty = &buy
	}
	if body.FreeQty != nil {
		free := domain.Quantity(*body.FreeQty)
		promotion.FreeQty = &free
	}
	if promotion.Channels == nil {
		promotion.Channels = []string{}
	}

	targets := make([]domain.PromotionTarget, 0, len(body.Targets))
	for _, target := range body.Targets {
		targets = append(targets, domain.PromotionTarget{
			ProductID: target.ProductID, CategoryID: target.CategoryID,
			Qty: domain.Quantity(target.Qty),
		})
	}

	saved, err := h.service.SavePromotion(r.Context(),
		PromotionInput{Promotion: promotion, Targets: targets}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, saved)
}

func parsePromoDate(raw *string) *domain.Date {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	parsed, err := domain.ParseDate(strings.TrimSpace(*raw))
	if err != nil {
		return nil
	}
	return &parsed
}

type codeBody struct {
	Code string `json:"code"`
}

func (h *Handler) applyCode(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[codeBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	order, err := h.service.ApplyCode(r.Context(), httpx.Param(r, "id"), body.Code, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, order)
}

// ── Gift cards ───────────────────────────────────────────────────────────────

func (h *Handler) listGiftCards(w http.ResponseWriter, r *http.Request) {
	cards, err := h.service.GiftCards(r.Context(), GiftCardFilter{
		MemberID: httpx.Query(r, "memberId"),
		Status:   strings.ToUpper(httpx.Query(r, "status")),
		Query:    httpx.Query(r, "query"),
		Limit:    httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, cards)
}

func (h *Handler) getGiftCard(w http.ResponseWriter, r *http.Request) {
	card, err := h.service.GiftCard(r.Context(), httpx.Param(r, "code"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, card)
}

type giftCardBody struct {
	Code      string  `json:"code"`
	Barcode   *string `json:"barcode"`
	MemberID  *string `json:"memberId"`
	AmountIDR float64 `json:"amountIdr"`
	ExpiresOn *string `json:"expiresOn"`
	Note      *string `json:"note"`
}

func (g *giftCardBody) Validate() error {
	if g.AmountIDR <= 0 {
		return httpx.Invalid("A gift card needs money on it.")
	}
	if g.ExpiresOn != nil {
		if _, err := domain.ParseDate(*g.ExpiresOn); err != nil {
			return httpx.Invalid("expiresOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) issueGiftCard(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[giftCardBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	card, err := h.service.IssueGiftCard(r.Context(), GiftCardInput{
		Code: body.Code, Barcode: body.Barcode, MemberID: body.MemberID,
		AmountIDR: body.AmountIDR, ExpiresOn: parsePromoDate(body.ExpiresOn), Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, card)
}

type topUpBody struct {
	AmountIDR float64 `json:"amountIdr"`
}

func (t *topUpBody) Validate() error {
	if t.AmountIDR <= 0 {
		return httpx.Invalid("A top-up of nothing is not a top-up.")
	}
	return nil
}

func (h *Handler) topUpGiftCard(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[topUpBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	card, err := h.service.TopUpGiftCard(r.Context(), httpx.Param(r, "code"),
		body.AmountIDR, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, card)
}

type cardStatusBody struct {
	Status string `json:"status"`
}

func (h *Handler) setGiftCardStatus(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[cardStatusBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	card, err := h.service.SetGiftCardStatus(r.Context(), httpx.Param(r, "code"),
		body.Status, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, card)
}

// ── Payment methods ──────────────────────────────────────────────────────────

func (h *Handler) listMethods(w http.ResponseWriter, r *http.Request) {
	methods, err := h.service.PaymentMethods(r.Context(), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, methods)
}

type methodBody struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	GivesChange    bool   `json:"givesChange"`
	NeedsReference bool   `json:"needsReference"`
	CountsInDrawer bool   `json:"countsInDrawer"`
	SortOrder      int    `json:"sortOrder"`
	Active         *bool  `json:"active"`
}

func (m *methodBody) Validate() error {
	if strings.TrimSpace(m.Code) == "" || strings.TrimSpace(m.Name) == "" {
		return httpx.Invalid("A payment method needs a code and a name.")
	}
	if !domain.IsValidMethodKind(strings.ToUpper(m.Kind)) {
		return httpx.Invalid("%q is not a kind of payment.", m.Kind)
	}
	if m.GivesChange && !m.CountsInDrawer {
		return httpx.Invalid(
			"Change comes out of the drawer, so a method that gives it has to be counted in it.")
	}
	return nil
}

func (h *Handler) saveMethod(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[methodBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	method, err := h.service.SaveMethod(r.Context(), domain.PaymentMethod{
		Code: body.Code, Name: body.Name,
		Kind:           domain.PaymentMethodKind(strings.ToUpper(body.Kind)),
		GivesChange:    body.GivesChange,
		NeedsReference: body.NeedsReference, CountsInDrawer: body.CountsInDrawer,
		SortOrder: body.SortOrder, Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, method)
}

// ── Receipts ─────────────────────────────────────────────────────────────────

func (h *Handler) getReceiptSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.ReceiptSettings(r.Context(), httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, settings)
}

type receiptSettingsBody struct {
	BranchID     string  `json:"branchId"`
	Header       string  `json:"header"`
	Footer       string  `json:"footer"`
	BusinessName string  `json:"businessName"`
	Address      string  `json:"address"`
	Phone        *string `json:"phone"`
	TaxNumber    *string `json:"taxNumber"`
	PaperWidth   int     `json:"paperWidth"`
	ShowLogo     *bool   `json:"showLogo"`
	ShowCashier  *bool   `json:"showCashier"`
	AutoPrint    *bool   `json:"autoPrint"`
}

func (b *receiptSettingsBody) Validate() error {
	if strings.TrimSpace(b.BranchID) == "" {
		return httpx.Invalid("Receipt settings belong to a branch.")
	}
	if b.PaperWidth != 0 && b.PaperWidth != 58 && b.PaperWidth != 80 {
		return httpx.Invalid("Thermal paper is 58mm or 80mm.")
	}
	return nil
}

func (h *Handler) saveReceiptSettings(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[receiptSettingsBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	width := body.PaperWidth
	if width == 0 {
		width = 58
	}
	settings, err := h.service.SaveReceiptSettings(r.Context(), ReceiptSettings{
		BranchID: body.BranchID, Header: body.Header, Footer: body.Footer,
		BusinessName: body.BusinessName, Address: body.Address, Phone: body.Phone,
		TaxNumber: body.TaxNumber, PaperWidth: width,
		ShowLogo:    body.ShowLogo == nil || *body.ShowLogo,
		ShowCashier: body.ShowCashier == nil || *body.ShowCashier,
		AutoPrint:   body.AutoPrint == nil || *body.AutoPrint,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, settings)
}

func (h *Handler) previewReceipt(w http.ResponseWriter, r *http.Request) {
	body, err := h.service.RenderReceipt(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]string{"body": body})
}

func (h *Handler) printReceipt(w http.ResponseWriter, r *http.Request) {
	job, err := h.service.QueueReceipt(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, job)
}

type sendReceiptBody struct {
	Channel     string `json:"channel"`
	Destination string `json:"destination"`
}

func (s *sendReceiptBody) Validate() error {
	if strings.TrimSpace(s.Destination) == "" {
		return httpx.Invalid("Where should the receipt go?")
	}
	return nil
}

func (h *Handler) sendReceipt(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[sendReceiptBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	send, err := h.service.SendReceipt(r.Context(), httpx.Param(r, "id"),
		body.Channel, body.Destination, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, send)
}

func (h *Handler) listPrintJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.service.PrintJobs(r.Context(), httpx.Query(r, "branchId"),
		httpx.Query(r, "status"), httpx.QueryInt(r, "limit", 50))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, jobs)
}

// claimPrintJobs is the printer agent's endpoint: it takes the next few jobs
// and marks them as taken, so two agents on one counter do not print the same
// receipt twice.
func (h *Handler) claimPrintJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.service.ClaimPrintJobs(r.Context(), httpx.Query(r, "branchId"),
		httpx.QueryInt(r, "limit", 5))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, jobs)
}

type printResultBody struct {
	Status string  `json:"status"`
	Error  *string `json:"error"`
}

func (h *Handler) finishPrintJob(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[printResultBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	job, err := h.service.FinishPrintJob(r.Context(), httpx.Param(r, "id"), body.Status, body.Error)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, job)
}
