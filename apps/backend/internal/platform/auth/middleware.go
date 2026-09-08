package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

type ctxKey string

const principalKey ctxKey = "principal"

// Permits answers whether a staff role holds a permission. It is injected so
// this package does not depend on the domain's RBAC table.
type Permits func(role, permission string) bool

// Guard turns tokens into authenticated, authorized requests.
type Guard struct {
	issuer  *Issuer
	clock   clock.Clock
	permits Permits
}

func NewGuard(issuer *Issuer, c clock.Clock, permits Permits) *Guard {
	return &Guard{issuer: issuer, clock: c, permits: permits}
}

// Optional attaches the principal when a valid token is present and otherwise
// lets the request through unauthenticated. Used by endpoints that serve both
// anonymous and signed-in callers.
func (g *Guard) Optional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, err := g.principal(r); err == nil {
			r = r.WithContext(WithPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}

// RequireMember admits signed-in members only.
func (g *Guard) RequireMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := g.principal(r)
		if err != nil {
			httpx.Fail(w, r, authError(err))
			return
		}
		if !p.IsMember() {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This endpoint is for member accounts."))
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

// RequireAdmin admits staff holding the permission. Permissions are checked
// here as well as in the UI, so hiding a button is never the only control.
func (g *Guard) RequireAdmin(permission string) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := g.principal(r)
			if err != nil {
				httpx.Fail(w, r, authError(err))
				return
			}
			if !p.IsAdmin() {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This endpoint is for staff accounts."))
				return
			}
			if permission != "" && !g.permits(p.Role, permission) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Your role does not include %q.", permission))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// RequireAnyAdmin admits any authenticated staff member.
func (g *Guard) RequireAnyAdmin(next http.Handler) http.Handler {
	return g.RequireAdmin("")(next)
}

func (g *Guard) principal(r *http.Request) (Principal, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return Principal{}, ErrTokenMalformed
	}
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		token, ok = strings.CutPrefix(header, "bearer ")
	}
	if !ok || strings.TrimSpace(token) == "" {
		return Principal{}, ErrTokenMalformed
	}
	return g.issuer.Parse(strings.TrimSpace(token), g.clock.Now())
}

func authError(err error) error {
	switch {
	case errors.Is(err, ErrTokenExpired):
		return httpx.ErrUnauthorized.WithMessage("Your session has expired, sign in again.").Wrap(err)
	default:
		return httpx.ErrUnauthorized.Wrap(err)
	}
}

// WithPrincipal stores a principal on the context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// FromContext returns the authenticated caller, if any.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// MemberID returns the calling member's id. Handlers behind RequireMember can
// rely on it being non-empty.
func MemberID(ctx context.Context) string {
	if p, ok := FromContext(ctx); ok && p.IsMember() {
		return p.ID
	}
	return ""
}

// Admin returns the calling staff principal.
func Admin(ctx context.Context) (Principal, bool) {
	p, ok := FromContext(ctx)
	if !ok || !p.IsAdmin() {
		return Principal{}, false
	}
	return p, true
}

// TokenTTL exposes the configured lifetime for login responses.
func (g *Guard) Issue(p Principal) (string, time.Time, error) {
	return g.issuer.Issue(p, g.clock.Now())
}
