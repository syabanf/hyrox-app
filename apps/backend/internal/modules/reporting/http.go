package reporting

import (
	"net/http"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the cross-module read models.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }

	// The member app's own composite reads.
	r.Get("/api/me", h.me, h.guard.RequireMember)
	r.Get("/api/home", h.home, h.guard.RequireMember)

	r.Get("/api/admin/reports/dashboard", h.dashboard, admin(domain.PermDashboardView))
	r.Get("/api/admin/reports/sales", h.sales, admin(domain.PermReportsView))
	r.Get("/api/admin/reports/visits", h.visits, admin(domain.PermReportsView))
	r.Get("/api/admin/reports/credits", h.credits, admin(domain.PermReportsView))
	r.Get("/api/admin/reports/classes", h.classes, admin(domain.PermReportsView))

	r.Get("/api/admin/members", h.memberList, admin(domain.PermMembersView))
	r.Get("/api/admin/members/{id}", h.memberDetail, admin(domain.PermMembersView))

	r.Get("/api/admin/audit", h.auditTrail, admin(domain.PermConfigView))
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.Me(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.Home(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.Dashboard(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

func (h *Handler) sales(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.Sales(r.Context(), httpx.QueryInt(r, "days", 30))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, report)
}

func (h *Handler) visits(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.Visits(r.Context(), httpx.QueryInt(r, "days", 30))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, report)
}

func (h *Handler) credits(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.Credits(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, report)
}

func (h *Handler) classes(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.Classes(r.Context(), httpx.QueryInt(r, "days", 90))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, report)
}

func (h *Handler) memberList(w http.ResponseWriter, r *http.Request) {
	members, err := h.service.MemberList(r.Context(), MemberQuery{
		Query:  httpx.Query(r, "query"),
		Status: httpx.Query(r, "status"),
		Limit:  httpx.QueryInt(r, "limit", 200),
		Offset: httpx.QueryInt(r, "offset", 0),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, members)
}

func (h *Handler) memberDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.MemberDetail(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, detail)
}

func (h *Handler) auditTrail(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.Audit(r.Context(), httpx.QueryInt(r, "limit", 100))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, events)
}
