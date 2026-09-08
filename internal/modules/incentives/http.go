package incentives

import (
	"net/http"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the payroll surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }

	r.Get("/api/admin/incentives/schemes", h.listSchemes, admin(domain.PermIncentivesView))
	r.Post("/api/admin/incentives/schemes", h.createScheme, admin(domain.PermIncentivesManage))
	r.Put("/api/admin/incentives/schemes/{id}", h.updateScheme, admin(domain.PermIncentivesManage))

	r.Get("/api/admin/incentives/statements", h.statements, admin(domain.PermIncentivesView))
	r.Get("/api/admin/incentives/payouts", h.listPayouts, admin(domain.PermIncentivesView))
	r.Post("/api/admin/incentives/payouts", h.createPayout, admin(domain.PermIncentivesManage))
	r.Post("/api/admin/incentives/payouts/{id}/{action}", h.actPayout, admin(domain.PermIncentivesManage))
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

func (h *Handler) listSchemes(w http.ResponseWriter, r *http.Request) {
	schemes, err := h.service.Schemes(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, schemes)
}

type schemeRequest struct {
	CoachID                   *string `json:"coachId"`
	SessionFeeIDR             int64   `json:"sessionFeeIdr"`
	PerAttendeeIDR            int64   `json:"perAttendeeIdr"`
	FullClassBonusIDR         int64   `json:"fullClassBonusIdr"`
	FullClassThresholdPercent int     `json:"fullClassThresholdPercent"`
	NoShowPenaltyIDR          int64   `json:"noShowPenaltyIdr"`
	Active                    *bool   `json:"active"`
}

func (s *schemeRequest) Validate() error {
	if s.SessionFeeIDR < 0 || s.PerAttendeeIDR < 0 || s.FullClassBonusIDR < 0 || s.NoShowPenaltyIDR < 0 {
		return httpx.Invalid("Incentive amounts cannot be negative.")
	}
	if s.FullClassThresholdPercent < 0 || s.FullClassThresholdPercent > 100 {
		return httpx.Invalid("The full-class threshold must be between 0 and 100 percent.")
	}
	return nil
}

func (s schemeRequest) toInput() SchemeInput {
	return SchemeInput{
		CoachID:                   s.CoachID,
		SessionFeeIDR:             s.SessionFeeIDR,
		PerAttendeeIDR:            s.PerAttendeeIDR,
		FullClassBonusIDR:         s.FullClassBonusIDR,
		FullClassThresholdPercent: s.FullClassThresholdPercent,
		NoShowPenaltyIDR:          s.NoShowPenaltyIDR,
		Active:                    s.Active == nil || *s.Active,
	}
}

func (h *Handler) createScheme(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[schemeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	view, err := h.service.CreateScheme(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, view)
}

func (h *Handler) updateScheme(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[schemeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	view, err := h.service.UpdateScheme(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

func (h *Handler) statements(w http.ResponseWriter, r *http.Request) {
	period := httpx.Query(r, "period")
	if period == "" {
		httpx.Fail(w, r, httpx.Invalid("A period (YYYY-MM) is required."))
		return
	}
	statements, err := h.service.Statements(r.Context(), period, httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, statements)
}

func (h *Handler) listPayouts(w http.ResponseWriter, r *http.Request) {
	payouts, err := h.service.Payouts(r.Context(),
		httpx.Query(r, "period"), httpx.Query(r, "coachId"), httpx.Query(r, "status"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, payouts)
}

type createPayoutRequest struct {
	CoachID     string `json:"coachId"`
	PeriodMonth string `json:"periodMonth"`
}

func (c *createPayoutRequest) Validate() error {
	if c.CoachID == "" {
		return httpx.Invalid("A coach is required.")
	}
	if len(c.PeriodMonth) != 7 || c.PeriodMonth[4] != '-' {
		return httpx.Invalid("Period must be in YYYY-MM form.")
	}
	return nil
}

func (h *Handler) createPayout(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[createPayoutRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	view, err := h.service.CreatePayout(r.Context(), body.CoachID, body.PeriodMonth, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, view)
}

type payoutActionRequest struct {
	PaymentReference *string `json:"paymentReference"`
	Note             *string `json:"note"`
}

func (h *Handler) actPayout(w http.ResponseWriter, r *http.Request) {
	// The body is optional: approving needs nothing, paying needs a reference,
	// voiding needs a note.
	body := payoutActionRequest{}
	if r.ContentLength > 0 {
		decoded, err := httpx.Decode[payoutActionRequest](r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		body = decoded
	}

	view, err := h.service.ActPayout(r.Context(), httpx.Param(r, "id"),
		domain.PayoutAction(httpx.Param(r, "action")), body.PaymentReference, body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}
