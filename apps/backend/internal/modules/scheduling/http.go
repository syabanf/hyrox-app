package scheduling

import (
	"net/http"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the scheduling module's HTTP surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }

	// The public schedule takes an optional member token: with one, each class
	// also reports whether the caller already has a place.
	r.Get("/api/sessions", h.listSessions, h.guard.Optional)
	r.Get("/api/sessions/{id}", h.session, h.guard.Optional)

	r.Post("/api/sessions/{id}/book", h.book, h.guard.RequireMember)
	r.Get("/api/me/bookings", h.myBookings, h.guard.RequireMember)
	r.Post("/api/bookings/{id}/confirm-spot", h.confirmSpot, h.guard.RequireMember)
	// Cancellation is reachable by the member who booked or by the front desk.
	r.Post("/api/bookings/{id}/cancel", h.cancel, h.guard.Optional)

	r.Get("/api/admin/sessions", h.listAdminSessions, admin(domain.PermOperationsView))
	r.Get("/api/admin/sessions/{id}", h.adminSession, admin(domain.PermOperationsView))
	r.Post("/api/admin/sessions", h.createSession, admin(domain.PermSessionsManage))
	r.Patch("/api/admin/sessions/{id}", h.updateSession, admin(domain.PermSessionsManage))
	r.Delete("/api/admin/sessions/{id}", h.deleteSession, admin(domain.PermSessionsManage))
	r.Post("/api/admin/sessions/{id}/publish", h.publishSession, admin(domain.PermSessionsManage))
	r.Post("/api/admin/sessions/{id}/cancel", h.cancelSession, admin(domain.PermSessionsManage))
	r.Post("/api/admin/sessions/{id}/complete", h.completeSession, admin(domain.PermSessionsManage))

	r.Post("/api/admin/bookings", h.adminBook, admin(domain.PermBookingsManage))
	r.Post("/api/admin/bookings/{id}/check-in", h.checkIn, admin(domain.PermAttendanceManage))
	r.Post("/api/admin/bookings/{id}/no-show", h.noShow, admin(domain.PermAttendanceManage))
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

// ── Schedule ─────────────────────────────────────────────────────────────────

// parseWindow reads the from/to query parameters that bound a schedule query.
func parseWindow(r *http.Request) (*time.Time, *time.Time, error) {
	var from, to *time.Time
	if raw := httpx.Query(r, "from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, nil, httpx.Invalid("The 'from' parameter must be an RFC3339 timestamp.")
		}
		from = &parsed
	}
	if raw := httpx.Query(r, "to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, nil, httpx.Invalid("The 'to' parameter must be an RFC3339 timestamp.")
		}
		to = &parsed
	}
	return from, to, nil
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to, err := parseWindow(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sessions, err := h.service.Sessions(ctx, SessionFilter{
		BranchID: httpx.Query(r, "branchId"),
		From:     from,
		To:       to,
		// Drafts are staff-only: a member must never see a class that has not
		// been published.
		Statuses: []domain.SessionStatus{
			domain.SessionPublished, domain.SessionFull,
			domain.SessionCompleted, domain.SessionCancelled,
		},
		Limit: httpx.QueryInt(r, "limit", 500),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	summaries, err := h.service.Summaries(ctx, sessions, auth.MemberID(ctx))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summaries)
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, err := h.service.Session(ctx, httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if session.Status == domain.SessionDraft {
		if _, isAdmin := auth.Admin(ctx); !isAdmin {
			httpx.Fail(w, r, httpx.NotFound("session"))
			return
		}
	}
	summaries, err := h.service.Summaries(ctx, []domain.ClassSession{session}, auth.MemberID(ctx))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summaries[0])
}

func (h *Handler) listAdminSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to, err := parseWindow(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var statuses []domain.SessionStatus
	if status := httpx.Query(r, "status"); status != "" {
		statuses = []domain.SessionStatus{domain.SessionStatus(status)}
	}
	sessions, err := h.service.Sessions(ctx, SessionFilter{
		BranchID: httpx.Query(r, "branchId"),
		CoachID:  httpx.Query(r, "coachId"),
		From:     from,
		To:       to,
		Statuses: statuses,
		Limit:    httpx.QueryInt(r, "limit", 500),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	summaries, err := h.service.Summaries(ctx, sessions, "")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summaries)
}

// sessionDetailView is the roster screen: the class plus who is coming.
type sessionDetailView struct {
	SessionSummary
	Roster []RosterEntry `json:"roster"`
}

func (h *Handler) adminSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := httpx.Param(r, "id")
	session, err := h.service.Session(ctx, sessionID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	summaries, err := h.service.Summaries(ctx, []domain.ClassSession{session}, "")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	roster, err := h.service.Roster(ctx, sessionID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, sessionDetailView{SessionSummary: summaries[0], Roster: roster})
}

type createSessionRequest struct {
	ClassTypeID string    `json:"classTypeId"`
	BranchID    string    `json:"branchId"`
	CoachID     string    `json:"coachId"`
	StartsAt    time.Time `json:"startsAt"`
	DurationMin int       `json:"durationMin"`
	Capacity    int       `json:"capacity"`
	CreditCost  int       `json:"creditCost"`
	Area        *string   `json:"area"`
	Publish     *bool     `json:"publish"`
}

func (c *createSessionRequest) Validate() error {
	if c.ClassTypeID == "" || c.BranchID == "" || c.CoachID == "" {
		return httpx.Invalid("A class type, branch and coach are all required.")
	}
	if c.StartsAt.IsZero() {
		return httpx.Invalid("A start time is required.")
	}
	return nil
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[createSessionRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	publish := body.Publish == nil || *body.Publish
	session, err := h.service.CreateSession(r.Context(), SessionInput{
		ClassTypeID: body.ClassTypeID,
		BranchID:    body.BranchID,
		CoachID:     body.CoachID,
		StartsAt:    body.StartsAt,
		DurationMin: body.DurationMin,
		Capacity:    body.Capacity,
		CreditCost:  body.CreditCost,
		Area:        body.Area,
		Publish:     publish,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, session)
}

type sessionPatchRequest struct {
	CoachID     *string    `json:"coachId"`
	Capacity    *int       `json:"capacity"`
	StartsAt    *time.Time `json:"startsAt"`
	DurationMin *int       `json:"durationMin"`
	Area        **string   `json:"area"`
}

func (h *Handler) updateSession(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[sessionPatchRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	patch := SessionPatch{
		CoachID:     body.CoachID,
		Capacity:    body.Capacity,
		StartsAt:    body.StartsAt,
		DurationMin: body.DurationMin,
	}
	if body.Area != nil {
		patch.SetArea = true
		patch.Area = *body.Area
	}
	session, err := h.service.UpdateSession(r.Context(), httpx.Param(r, "id"), patch, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, session)
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteSession(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, target domain.SessionStatus) {
	session, err := h.service.TransitionSession(r.Context(), httpx.Param(r, "id"), target, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, session)
}

func (h *Handler) publishSession(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, domain.SessionPublished)
}

func (h *Handler) cancelSession(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, domain.SessionCancelled)
}

func (h *Handler) completeSession(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, domain.SessionCompleted)
}

// ── Bookings ─────────────────────────────────────────────────────────────────

func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Book(r.Context(), auth.MemberID(r.Context()),
		httpx.Param(r, "id"), domain.BookingSourceMember)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, result)
}

type adminBookRequest struct {
	MemberID  string `json:"memberId"`
	SessionID string `json:"sessionId"`
}

func (a *adminBookRequest) Validate() error {
	if a.MemberID == "" || a.SessionID == "" {
		return httpx.Invalid("A member and a session are both required.")
	}
	return nil
}

func (h *Handler) adminBook(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[adminBookRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.service.Book(r.Context(), body.MemberID, body.SessionID, domain.BookingSourceAdmin)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, result)
}

func (h *Handler) myBookings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookings, err := h.service.MemberBookings(ctx, auth.MemberID(ctx), httpx.QueryInt(r, "limit", 100))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	summaries, err := h.service.BookingSummaries(ctx, bookings)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summaries)
}

func (h *Handler) confirmSpot(w http.ResponseWriter, r *http.Request) {
	booking, err := h.service.ConfirmOfferedSpot(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, booking)
}

// cancel is reachable by the member who booked, or by staff with the bookings
// permission acting on their behalf.
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principal, signedIn := auth.FromContext(ctx)
	if !signedIn {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	bookingID := httpx.Param(r, "id")

	if principal.IsMember() {
		booking, err := h.service.Booking(ctx, bookingID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if booking.MemberID != principal.ID {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("That booking is not yours."))
			return
		}
	} else if !domain.HasPermission(domain.AdminRole(principal.Role), domain.PermBookingsManage) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Your role cannot cancel bookings."))
		return
	}

	result, err := h.service.Cancel(ctx, bookingID, Actor{ID: principal.ID, Name: principal.Role})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, result)
}

func (h *Handler) checkIn(w http.ResponseWriter, r *http.Request) {
	booking, err := h.service.CheckInManually(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, booking)
}

func (h *Handler) noShow(w http.ResponseWriter, r *http.Request) {
	booking, err := h.service.MarkNoShow(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, booking)
}
