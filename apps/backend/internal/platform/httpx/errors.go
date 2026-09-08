package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is the single error type crossing the transport boundary. Services
// return it with a stable machine code; the client shows the message and
// branches on the code, which is why codes are part of the API contract.
type Error struct {
	Status  int
	Code    string
	Message string
	// Err is the underlying cause. It is logged, never serialized.
	Err error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// Wrap attaches an underlying cause for the logs.
func (e *Error) Wrap(err error) *Error {
	clone := *e
	clone.Err = err
	return &clone
}

// WithMessage overrides the human-readable half, keeping status and code.
func (e *Error) WithMessage(format string, args ...any) *Error {
	clone := *e
	clone.Message = fmt.Sprintf(format, args...)
	return &clone
}

func newError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Transport-level errors shared by every module. Domain-specific codes
// (SLOT_TAKEN, INSUFFICIENT_CREDITS, ...) are declared by their own module.
var (
	ErrBadRequest   = newError(http.StatusBadRequest, "BAD_REQUEST", "The request could not be understood.")
	ErrValidation   = newError(http.StatusUnprocessableEntity, "VALIDATION_FAILED", "The request failed validation.")
	ErrUnauthorized = newError(http.StatusUnauthorized, "UNAUTHORIZED", "Sign in to continue.")
	ErrForbidden    = newError(http.StatusForbidden, "FORBIDDEN", "You do not have permission to do that.")
	ErrNotFound     = newError(http.StatusNotFound, "NOT_FOUND", "Not found.")
	ErrConflict     = newError(http.StatusConflict, "CONFLICT", "That action conflicts with the current state.")
	ErrInUse        = newError(http.StatusConflict, "IN_USE", "This record is referenced elsewhere and cannot be deleted.")
	ErrRateLimited  = newError(http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests, try again shortly.")
	ErrInternal     = newError(http.StatusInternalServerError, "INTERNAL", "Something went wrong on our side.")
)

// NotFound builds a 404 naming the entity, e.g. NotFound("member").
func NotFound(entity string) *Error {
	return &Error{
		Status:  http.StatusNotFound,
		Code:    "NOT_FOUND",
		Message: fmt.Sprintf("%s not found.", capitalize(entity)),
	}
}

// Invalid builds a 422 for a specific field problem.
func Invalid(format string, args ...any) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "VALIDATION_FAILED", Message: fmt.Sprintf(format, args...)}
}

// Conflict builds a 409 with a domain-specific code, which is how business
// rules (booking closed, slot taken, already reversed) reach the client.
func Conflict(code, format string, args ...any) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: fmt.Sprintf(format, args...)}
}

// AsError maps any error onto the transport shape, defaulting to 500 so an
// unexpected failure never leaks internals to the caller.
func AsError(err error) *Error {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return ErrInternal.Wrap(err)
}

// IsNotFound reports whether an error is (or wraps) a 404. Callers use it to
// turn "no such row" into something other than a 404 — a sign-in, for
// instance, must not answer differently for an unknown account.
func IsNotFound(err error) bool {
	var appErr *Error
	return errors.As(err, &appErr) && appErr.Status == http.StatusNotFound
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}
