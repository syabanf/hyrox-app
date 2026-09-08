package engagement

import (
	"net/http"

	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the member-facing engagement surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	r.Get("/api/me/notifications", h.list, h.guard.RequireMember)
	r.Post("/api/me/notifications/read-all", h.readAll, h.guard.RequireMember)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.service.Notifications(r.Context(),
		auth.MemberID(r.Context()), httpx.QueryInt(r, "limit", 50))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, notifications)
}

func (h *Handler) readAll(w http.ResponseWriter, r *http.Request) {
	marked, err := h.service.MarkAllRead(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]any{"ok": true, "marked": marked})
}
