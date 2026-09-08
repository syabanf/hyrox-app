package httpx

import (
	"net/http"
	"strings"
)

// Router is a thin layer over the standard ServeMux: pattern routing comes
// from the stdlib, and this adds middleware stacks and grouping. Keeping the
// router dependency-free is what lets a module move to another service (or
// another framework) without touching its handlers.
type Router struct {
	mux         *http.ServeMux
	prefix      string
	middlewares []Middleware
}

// NewRouter creates a router with the given global middleware.
func NewRouter(middlewares ...Middleware) *Router {
	return &Router{mux: http.NewServeMux(), middlewares: middlewares}
}

// Group returns a router that prefixes paths and adds middleware, for mounting
// one module under its own path with its own auth requirements.
func (r *Router) Group(prefix string, middlewares ...Middleware) *Router {
	combined := make([]Middleware, 0, len(r.middlewares)+len(middlewares))
	combined = append(combined, r.middlewares...)
	combined = append(combined, middlewares...)
	return &Router{
		mux:         r.mux,
		prefix:      strings.TrimSuffix(r.prefix+prefix, "/"),
		middlewares: combined,
	}
}

// Use appends middleware to this router (not to groups already derived).
func (r *Router) Use(middlewares ...Middleware) {
	r.middlewares = append(r.middlewares, middlewares...)
}

// Handle registers a handler for "METHOD /path", applying the middleware stack.
func (r *Router) Handle(method, path string, handler http.HandlerFunc, middlewares ...Middleware) {
	stack := make([]Middleware, 0, len(r.middlewares)+len(middlewares))
	stack = append(stack, r.middlewares...)
	stack = append(stack, middlewares...)

	var h http.Handler = handler
	for i := len(stack) - 1; i >= 0; i-- {
		h = stack[i](h)
	}
	r.mux.Handle(method+" "+r.prefix+path, h)
}

func (r *Router) Get(path string, h http.HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodGet, path, h, mw...)
}

func (r *Router) Post(path string, h http.HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodPost, path, h, mw...)
}

func (r *Router) Patch(path string, h http.HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodPatch, path, h, mw...)
}

func (r *Router) Put(path string, h http.HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodPut, path, h, mw...)
}

func (r *Router) Delete(path string, h http.HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodDelete, path, h, mw...)
}

// ServeHTTP makes the router the server's handler. OPTIONS is answered by the
// CORS middleware, so preflight never needs a route of its own.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		for i := len(r.middlewares) - 1; i >= 0; i-- {
			h = r.middlewares[i](h)
		}
		h.ServeHTTP(w, req)
		return
	}
	r.mux.ServeHTTP(w, req)
}

// NotFoundHandler answers unmatched paths in the shared error envelope rather
// than the stdlib's plain-text 404.
func (r *Router) NotFoundHandler() {
	r.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		Fail(w, req, NotFound("route").WithMessage("No route for %s %s.", req.Method, req.URL.Path))
	})
}
