package access

import (
	"net/http"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Handler serves the access module's HTTP surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
	// gateKey authenticates the scanner hardware. Scanners are not people, so
	// they carry a shared secret rather than a member or staff token.
	gateKey string
}

func NewHandler(service *Service, guard *auth.Guard, gateKey string) *Handler {
	return &Handler{service: service, guard: guard, gateKey: gateKey}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }

	r.Post("/api/me/qr", h.issueQR, h.guard.RequireMember)
	r.Get("/api/me/visits", h.myVisits, h.guard.RequireMember)

	// The scan endpoint serves three callers: gate hardware with the shared
	// key, a member simulating their own scan, and staff running the monitor.
	r.Post("/api/gates/{gateId}/scan", h.scan, h.guard.Optional)

	r.Get("/api/admin/access-logs", h.listLogs, admin(domain.PermAccessView))
	r.Post("/api/admin/access-logs/{id}/resolve", h.resolveConflict, admin(domain.PermAccessSimulate))
}

func (h *Handler) issueQR(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.IssueQR(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

func (h *Handler) myVisits(w http.ResponseWriter, r *http.Request) {
	visits, err := h.service.MemberVisits(r.Context(), auth.MemberID(r.Context()), httpx.QueryInt(r, "limit", 50))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, visits)
}

type scanRequest struct {
	QRToken  string `json:"qrToken"`
	MemberID string `json:"memberId"`
	Offline  bool   `json:"offline"`
}

func (s *scanRequest) Validate() error {
	if strings.TrimSpace(s.QRToken) == "" && strings.TrimSpace(s.MemberID) == "" {
		return httpx.Invalid("A QR token or a member id is required.")
	}
	return nil
}

// scan authorizes the caller, then runs the gate pipeline.
//
// Presenting a QR token is self-authorizing: the token IS the credential, and
// that is what lets scanner hardware call this endpoint. Naming a member id
// instead is a simulation, and only the member themselves or staff with the
// simulate permission may do that.
func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[scanRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()

	if body.QRToken == "" {
		principal, signedIn := auth.FromContext(ctx)
		gateAuthorized := h.gateKey != "" && r.Header.Get("X-Gate-Key") == h.gateKey
		switch {
		case gateAuthorized:
		case signedIn && principal.IsMember() && principal.ID == body.MemberID:
		case signedIn && principal.IsAdmin() &&
			domain.HasPermission(domain.AdminRole(principal.Role), domain.PermAccessSimulate):
		default:
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You cannot simulate a scan for that member."))
			return
		}
	}

	result, err := h.service.Scan(ctx, ScanRequest{
		GateID:   httpx.Param(r, "gateId"),
		QRToken:  body.QRToken,
		MemberID: body.MemberID,
		Offline:  body.Offline,
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// A refusal is a valid answer to a valid question, so the decision travels
	// in a 200 body; only a broken request is an HTTP error.
	httpx.OK(w, result)
}

func (h *Handler) listLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := h.service.Logs(r.Context(), LogFilter{
		MemberID: httpx.Query(r, "memberId"),
		BranchID: httpx.Query(r, "branchId"),
		GateID:   httpx.Query(r, "gateId"),
		Result:   httpx.Query(r, "result"),
		Mode:     httpx.Query(r, "mode"),
		Limit:    httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, logs)
}

type resolveRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func (rq *resolveRequest) Validate() error {
	action := strings.ToUpper(strings.TrimSpace(rq.Action))
	if action != "APPROVE" && action != "REJECT" {
		return httpx.Invalid("Action must be APPROVE or REJECT.")
	}
	if len(strings.TrimSpace(rq.Reason)) < 3 {
		return httpx.Invalid("A reason is required.")
	}
	return nil
}

func (h *Handler) resolveConflict(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[resolveRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	principal, _ := auth.Admin(r.Context())
	view, err := h.service.ResolveConflict(r.Context(), httpx.Param(r, "id"),
		body.Action, body.Reason, Actor{ID: principal.ID, Name: principal.Role})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}
