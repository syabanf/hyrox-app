package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Middleware decorates a handler. Stacks are applied outermost-first.
type Middleware func(http.Handler) http.Handler

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestIDFrom returns the id assigned to this request, for logs and support.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// RequestID assigns (or adopts) a correlation id. Once modules are split into
// services, the inbound header is what stitches their logs back together.
func RequestID(gen id.Generator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get("X-Request-Id")
			if rid == "" {
				rid = gen.New("req")
			}
			w.Header().Set("X-Request-Id", rid)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, rid)))
		})
	}
}

// Recoverer turns a panic into a 500 so one bad request cannot take the
// process down with it.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				writeError(w, ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures what was actually written for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Logger emits one structured line per request.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		} else if rec.status >= 400 {
			level = slog.LevelWarn
		}
		slog.LogAttrs(r.Context(), level, "http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("took", time.Since(start).Round(time.Millisecond)),
			slog.String("request_id", RequestIDFrom(r.Context())),
		)
	})
}

// CORS allows the member PWA and admin panel, which are served from their own
// origins, to call the API directly from the browser.
func CORS(allowed []string) Middleware {
	allowAny := false
	for _, o := range allowed {
		if o == "*" {
			allowAny = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAny || matchesOrigin(allowed, origin)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id, Idempotency-Key")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func matchesOrigin(allowed []string, origin string) bool {
	for _, a := range allowed {
		if strings.EqualFold(a, origin) {
			return true
		}
	}
	return false
}

// Timeout bounds handler execution, so a stuck dependency sheds load instead
// of pinning connections.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
