package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// maxBodyBytes caps request bodies; activity tracks with GPS points are the
// largest legitimate payload, so the limit is generous rather than tight.
const maxBodyBytes = 8 << 20

// errorEnvelope is the wire shape the web clients already parse:
//
//	{"error": {"code": "...", "message": "..."}}
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON writes a successful response. Views are serialized unwrapped, matching
// the existing client contract.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if payload == nil || status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("response encoding failed", "error", err)
		writeError(w, ErrInternal)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// OK is the common 200 case.
func OK(w http.ResponseWriter, payload any) { JSON(w, http.StatusOK, payload) }

// Created is the common 201 case.
func Created(w http.ResponseWriter, payload any) { JSON(w, http.StatusCreated, payload) }

// Fail writes any error using the shared envelope, logging the cause when the
// failure is ours rather than the caller's.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	appErr := AsError(err)
	if appErr.Status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed",
			"code", appErr.Code,
			"method", r.Method,
			"path", r.URL.Path,
			"error", appErr.Error(),
		)
	}
	writeError(w, appErr)
}

func writeError(w http.ResponseWriter, appErr *Error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(appErr.Status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: errorBody{Code: appErr.Code, Message: appErr.Message}})
}

// Decode reads and validates a JSON request body. Unknown fields are rejected
// so a typo in a client payload fails loudly instead of being ignored.
func Decode[T any](r *http.Request) (T, error) {
	var target T
	if r.Body == nil {
		return target, ErrBadRequest.WithMessage("A request body is required.")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&target); err != nil {
		return target, decodeError(err)
	}
	if v, ok := any(&target).(Validator); ok {
		if err := v.Validate(); err != nil {
			return target, err
		}
	}
	return target, nil
}

// Validator lets a request DTO carry its own field rules.
type Validator interface {
	Validate() error
}

func decodeError(err error) error {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return ErrBadRequest.WithMessage("The request body is not valid JSON.").Wrap(err)
	case errors.As(err, &typeErr):
		return Invalid("Field %q expects a %s.", typeErr.Field, typeErr.Type.String()).Wrap(err)
	case errors.Is(err, io.EOF):
		return ErrBadRequest.WithMessage("A request body is required.").Wrap(err)
	case strings.Contains(err.Error(), "unknown field"):
		field := strings.TrimSuffix(strings.TrimPrefix(err.Error(), `json: unknown field "`), `"`)
		return Invalid("Unknown field %q.", field).Wrap(err)
	default:
		return ErrBadRequest.Wrap(err)
	}
}

// Query reads a string query parameter.
func Query(r *http.Request, key string) string { return strings.TrimSpace(r.URL.Query().Get(key)) }

// QueryInt reads an integer query parameter, falling back when absent or invalid.
func QueryInt(r *http.Request, key string, fallback int) int {
	raw := Query(r, key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

// QueryBool reads a boolean query parameter.
func QueryBool(r *http.Request, key string, fallback bool) bool {
	raw := Query(r, key)
	if raw == "" {
		return fallback
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return b
}

// Param reads a path parameter declared in a ServeMux pattern.
func Param(r *http.Request, key string) string { return r.PathValue(key) }
