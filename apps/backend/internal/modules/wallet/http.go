package wallet

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Handler serves the wallet module's HTTP surface.
type Handler struct {
	service *Service
	catalog Catalog
	members Members
	guard   *auth.Guard
	// devRoutes exposes the maintenance endpoints. Off outside development.
	devRoutes bool
}

func NewHandler(service *Service, catalog Catalog, members Members, guard *auth.Guard, devRoutes bool) *Handler {
	return &Handler{service: service, catalog: catalog, members: members, guard: guard, devRoutes: devRoutes}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }

	r.Get("/api/me/wallet", h.wallet, h.guard.RequireMember)
	r.Post("/api/me/topup", h.topUp, h.guard.RequireMember)
	r.Post("/api/vouchers/validate", h.validateVoucher, h.guard.RequireMember)
	// One promo, opened from the home screen's promo strip.
	r.Get("/api/promos/{code}", h.promo, h.guard.RequireMember)
	r.Get("/api/payments/{id}", h.payment, h.guard.RequireMember)
	// Settlement is reachable by the paying member (the demo "I paid" button)
	// or by finance staff; the handler checks which.
	r.Post("/api/payments/{id}/simulate", h.simulatePayment, h.guard.Optional)

	r.Get("/api/admin/payments", h.listPayments, admin(domain.PermPaymentsView))
	r.Post("/api/admin/payments/{id}/refund", h.refundPayment, admin(domain.PermRefundsManage))

	r.Get("/api/admin/vouchers", h.listVouchers, admin(domain.PermCommercialView))
	r.Post("/api/admin/vouchers", h.createVoucher, admin(domain.PermVouchersManage))
	r.Patch("/api/admin/vouchers/{id}", h.updateVoucher, admin(domain.PermVouchersManage))
	r.Post("/api/admin/vouchers/{id}/status", h.setVoucherStatus, admin(domain.PermVouchersManage))
	r.Delete("/api/admin/vouchers/{id}", h.deleteVoucher, admin(domain.PermVouchersManage))

	r.Post("/api/admin/members/{id}/adjust", h.adjustCredits, admin(domain.PermMembersAdjustCredit))
	r.Post("/api/admin/ledger/{entryId}/reverse", h.reverseEntry, admin(domain.PermLedgerReverse))

	if h.devRoutes {
		r.Post("/api/dev/expiry-sweep", h.expirySweep)
	} else {
		r.Post("/api/dev/expiry-sweep", h.expirySweep, admin(domain.PermRulesUpdate))
	}
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

// ── Member wallet ────────────────────────────────────────────────────────────

func (h *Handler) wallet(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.Wallet(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Coverage names are resolved here rather than in the service: they are
	// presentation, and the service already returned the ids.
	if err := h.fillCoverageNames(r.Context(), view.MyPackages); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

// fillCoverageNames turns coverage ids into class names for display. Packages
// with no restriction keep a nil list, which the app reads as "any class".
func (h *Handler) fillCoverageNames(ctx context.Context, packages []MyPackage) error {
	needsNames := false
	for _, p := range packages {
		if p.CoverageIDs != nil {
			needsNames = true
			break
		}
	}
	if !needsNames {
		return nil
	}
	classTypes, err := h.catalog.ClassTypes(ctx, false)
	if err != nil {
		return err
	}
	names := make(map[string]string, len(classTypes))
	for _, t := range classTypes {
		names[t.ID] = t.Name
	}
	for i := range packages {
		if packages[i].CoverageIDs == nil {
			continue
		}
		resolved := make([]string, 0, len(packages[i].CoverageIDs))
		for _, id := range packages[i].CoverageIDs {
			if name, ok := names[id]; ok {
				resolved = append(resolved, name)
			}
		}
		packages[i].CoverageNames = resolved
	}
	return nil
}

type topUpRequest struct {
	PackageID   string  `json:"packageId"`
	VoucherCode *string `json:"voucherCode"`
	Channel     string  `json:"channel"`
}

func (t *topUpRequest) Validate() error {
	if t.PackageID == "" {
		return httpx.Invalid("Choose a package.")
	}
	if !domain.IsValidPaymentChannel(t.Channel) {
		return httpx.Invalid("Choose a payment method: QRIS, EWALLET, VIRTUAL_ACCOUNT or CARD.")
	}
	return nil
}

func (h *Handler) topUp(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[topUpRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.service.TopUp(r.Context(), auth.MemberID(r.Context()),
		body.PackageID, body.VoucherCode, domain.PaymentChannel(body.Channel))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, result)
}

type validateVoucherRequest struct {
	Code      string `json:"code"`
	PackageID string `json:"packageId"`
}

func (v *validateVoucherRequest) Validate() error {
	if strings.TrimSpace(v.Code) == "" {
		return httpx.Invalid("Enter a voucher code.")
	}
	if v.PackageID == "" {
		return httpx.Invalid("Choose a package first.")
	}
	return nil
}

func (h *Handler) validateVoucher(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[validateVoucherRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	quote, err := h.service.QuoteVoucher(r.Context(), auth.MemberID(r.Context()), body.Code, body.PackageID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, quote)
}

// promo is one voucher as the member app advertises it: the code, what it is
// worth in words, and when it stops working.
//
// Only a live voucher is shown. A promo strip that offers something already
// expired is worse than an empty strip.
func (h *Handler) promo(w http.ResponseWriter, r *http.Request) {
	voucher, err := h.service.VoucherByCode(r.Context(), httpx.Param(r, "code"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if voucher.Status != domain.VoucherActive {
		httpx.Fail(w, r, httpx.NotFound("promo"))
		return
	}

	// nil applicable packages means the code works on everything, which the
	// app renders differently from a code limited to a named list.
	var packageNames []string
	for _, packageID := range voucher.ApplicablePackageIDs {
		pkg, err := h.catalog.Package(r.Context(), packageID)
		if err != nil {
			continue
		}
		packageNames = append(packageNames, pkg.Name)
	}

	httpx.OK(w, map[string]any{
		"code":           voucher.Code,
		"label":          voucherLabel(voucher),
		"type":           voucher.Type,
		"value":          voucher.Value,
		"endsAt":         voucher.EndsAt,
		"usageLimit":     voucher.UsageLimit,
		"newMembersOnly": voucher.EligibleSegment == domain.SegmentNewMembers,
		"packageNames":   packageNames,
	})
}

// voucherLabel says what a code is worth in the words a member reads on the
// promo strip: "10% OFF", or "Rp100.000 OFF".
func voucherLabel(v domain.Voucher) string {
	if v.Type == domain.VoucherPercent {
		return fmt.Sprintf("%d%% OFF", v.Value)
	}
	return "Rp" + formatThousands(v.Value) + " OFF"
}

// formatThousands groups with full stops, which is how Indonesian money is
// written and how every other price in the app is shown.
func formatThousands(value int64) string {
	digits := strconv.FormatInt(value, 10)
	var out strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteRune(r)
	}
	return out.String()
}

// paymentDetailView is one payment with the names a receipt needs.
type paymentDetailView struct {
	Payment     domain.Payment `json:"payment"`
	PackageName string         `json:"packageName"`
}

func (h *Handler) payment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	payment, err := h.service.Payment(ctx, httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// A member may only read their own payment, and the refusal is a 404 so
	// the endpoint cannot be used to probe for other members' payment ids.
	if payment.MemberID != auth.MemberID(ctx) {
		httpx.Fail(w, r, httpx.NotFound("payment"))
		return
	}
	pkg, err := h.catalog.Package(ctx, payment.PackageID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, paymentDetailView{Payment: payment, PackageName: pkg.Name})
}

// simulatePayment stands in for the provider's webhook while the studio runs
// on the mock gateway.
func (h *Handler) simulatePayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principal, signedIn := auth.FromContext(ctx)
	if !signedIn {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}

	payment, err := h.service.Payment(ctx, httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	switch {
	case principal.IsMember() && principal.ID == payment.MemberID:
		// The paying member confirming their own top-up.
	case principal.IsAdmin() && domain.HasPermission(domain.AdminRole(principal.Role), domain.PermPaymentsSimulate):
		// Finance settling on the member's behalf.
	default:
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You cannot settle this payment."))
		return
	}

	result, err := h.service.SettlePayment(ctx, payment.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, result)
}

// ── Admin payments ───────────────────────────────────────────────────────────

// paymentView is the admin list row.
type paymentView struct {
	Payment     domain.Payment `json:"payment"`
	MemberName  string         `json:"memberName"`
	PackageName string         `json:"packageName"`
}

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	payments, err := h.service.Payments(ctx, PaymentFilter{
		MemberID: httpx.Query(r, "memberId"),
		Status:   httpx.Query(r, "status"),
		Limit:    httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	packages, err := h.catalog.Packages(ctx, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	packageNames := map[string]string{}
	for _, p := range packages {
		packageNames[p.ID] = p.Name
	}

	views := make([]paymentView, 0, len(payments))
	for _, p := range payments {
		name := ""
		if member, err := h.members.Member(ctx, p.MemberID); err == nil {
			name = member.FullName
		}
		views = append(views, paymentView{
			Payment: p, MemberName: name, PackageName: packageNames[p.PackageID],
		})
	}
	httpx.OK(w, views)
}

type refundRequest struct {
	Reason string `json:"reason"`
}

func (rq *refundRequest) Validate() error {
	if len(strings.TrimSpace(rq.Reason)) < 3 {
		return httpx.Invalid("A refund reason is required.")
	}
	return nil
}

func (h *Handler) refundPayment(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[refundRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.service.RefundPayment(r.Context(), httpx.Param(r, "id"), body.Reason, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, result)
}

// ── Vouchers ─────────────────────────────────────────────────────────────────

type voucherView struct {
	Voucher         domain.Voucher `json:"voucher"`
	RedemptionCount int            `json:"redemptionCount"`
}

func (h *Handler) listVouchers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vouchers, err := h.service.Vouchers(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	totals, err := h.service.RedemptionTotals(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	views := make([]voucherView, 0, len(vouchers))
	for _, v := range vouchers {
		views = append(views, voucherView{Voucher: v, RedemptionCount: totals[v.ID]})
	}
	httpx.OK(w, views)
}

type voucherRequest struct {
	Code                 string    `json:"code"`
	Type                 string    `json:"type"`
	Value                int64     `json:"value"`
	StartsAt             time.Time `json:"startsAt"`
	EndsAt               time.Time `json:"endsAt"`
	UsageLimit           *int      `json:"usageLimit"`
	PerMemberLimit       *int      `json:"perMemberLimit"`
	EligibleSegment      string    `json:"eligibleSegment"`
	ApplicablePackageIDs []string  `json:"applicablePackageIds"`
}

func (v *voucherRequest) Validate() error {
	if len(strings.TrimSpace(v.Code)) < 3 {
		return httpx.Invalid("A voucher code of at least 3 characters is required.")
	}
	if v.Type != string(domain.VoucherFixedIDR) && v.Type != string(domain.VoucherPercent) {
		return httpx.Invalid("Type must be FIXED_IDR or PERCENT.")
	}
	if v.Value <= 0 {
		return httpx.Invalid("Value must be greater than zero.")
	}
	if v.Type == string(domain.VoucherPercent) && v.Value > 100 {
		return httpx.Invalid("A percentage discount cannot exceed 100.")
	}
	if v.StartsAt.IsZero() || v.EndsAt.IsZero() {
		return httpx.Invalid("A start and end date are required.")
	}
	if !v.EndsAt.After(v.StartsAt) {
		return httpx.Invalid("The end date must be after the start date.")
	}
	if v.EligibleSegment != "" && v.EligibleSegment != string(domain.SegmentAll) && v.EligibleSegment != string(domain.SegmentNewMembers) {
		return httpx.Invalid("Segment must be ALL or NEW_MEMBERS.")
	}
	return nil
}

func (h *Handler) createVoucher(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[voucherRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	voucher, err := h.service.CreateVoucher(r.Context(), VoucherInput{
		Code: body.Code, Type: domain.VoucherType(body.Type), Value: body.Value,
		StartsAt: body.StartsAt, EndsAt: body.EndsAt,
		UsageLimit: body.UsageLimit, PerMemberLimit: body.PerMemberLimit,
		EligibleSegment:      domain.VoucherSegment(body.EligibleSegment),
		ApplicablePackageIDs: body.ApplicablePackageIDs,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, voucherView{Voucher: voucher})
}

type voucherPatch struct {
	Code                 *string    `json:"code"`
	Type                 *string    `json:"type"`
	Value                *int64     `json:"value"`
	StartsAt             *time.Time `json:"startsAt"`
	EndsAt               *time.Time `json:"endsAt"`
	UsageLimit           *int       `json:"usageLimit"`
	PerMemberLimit       *int       `json:"perMemberLimit"`
	EligibleSegment      *string    `json:"eligibleSegment"`
	ApplicablePackageIDs *[]string  `json:"applicablePackageIds"`
}

func (h *Handler) updateVoucher(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[voucherPatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := VoucherInput{UsageLimit: body.UsageLimit, PerMemberLimit: body.PerMemberLimit}
	if body.Code != nil {
		in.Code = *body.Code
	}
	if body.Type != nil {
		in.Type = domain.VoucherType(*body.Type)
	}
	if body.Value != nil {
		in.Value = *body.Value
	}
	if body.StartsAt != nil {
		in.StartsAt = *body.StartsAt
	}
	if body.EndsAt != nil {
		in.EndsAt = *body.EndsAt
	}
	if body.EligibleSegment != nil {
		in.EligibleSegment = domain.VoucherSegment(*body.EligibleSegment)
	}
	setPackages := body.ApplicablePackageIDs != nil
	if setPackages {
		in.ApplicablePackageIDs = *body.ApplicablePackageIDs
	}

	voucher, err := h.service.UpdateVoucher(r.Context(), httpx.Param(r, "id"), in, setPackages, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, voucherView{Voucher: voucher})
}

type voucherStatusRequest struct {
	Status string `json:"status"`
}

func (v *voucherStatusRequest) Validate() error {
	switch domain.VoucherStatus(v.Status) {
	case domain.VoucherDraft, domain.VoucherScheduled, domain.VoucherActive,
		domain.VoucherExpired, domain.VoucherDisabled:
		return nil
	}
	return httpx.Invalid("That voucher status is not recognized.")
}

func (h *Handler) setVoucherStatus(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[voucherStatusRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	voucher, err := h.service.SetVoucherStatus(r.Context(), httpx.Param(r, "id"),
		domain.VoucherStatus(body.Status), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, voucherView{Voucher: voucher})
}

func (h *Handler) deleteVoucher(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteVoucher(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

// ── Credit corrections ───────────────────────────────────────────────────────

type adjustRequest struct {
	Amount int    `json:"amount"`
	Reason string `json:"reason"`
}

func (a *adjustRequest) Validate() error {
	if a.Amount == 0 {
		return httpx.Invalid("An adjustment cannot be zero.")
	}
	if len(strings.TrimSpace(a.Reason)) < 3 {
		return httpx.Invalid("A reason is required for a credit adjustment.")
	}
	return nil
}

func (h *Handler) adjustCredits(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[adjustRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entry, err := h.service.AdjustCredits(r.Context(), httpx.Param(r, "id"),
		body.Amount, body.Reason, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, entry)
}

type reverseRequest struct {
	Reason string `json:"reason"`
}

func (rq *reverseRequest) Validate() error {
	if len(strings.TrimSpace(rq.Reason)) < 3 {
		return httpx.Invalid("A reason is required to reverse an entry.")
	}
	return nil
}

func (h *Handler) reverseEntry(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[reverseRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entry, err := h.service.ReverseEntry(r.Context(), httpx.Param(r, "entryId"), body.Reason, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, entry)
}

func (h *Handler) expirySweep(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.SweepAll(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, result)
}
