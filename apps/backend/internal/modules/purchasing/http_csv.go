package purchasing

import (
	"net/http"

	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

const maxUploadBytes = 8 << 20

func (h *Handler) importSuppliers(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Body(r, maxUploadBytes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := httpx.ReadCSV(body, []string{"code", "name"})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.service.ImportSuppliers(r.Context(), rows,
		httpx.QueryBool(r, "apply", false), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, result)
}

func (h *Handler) importPrices(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Body(r, maxUploadBytes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := httpx.ReadCSV(body, []string{"sku", "price"})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.service.ImportPrices(r.Context(), httpx.Param(r, "id"), rows,
		httpx.QueryBool(r, "apply", false), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, result)
}

func (h *Handler) exportSuppliers(w http.ResponseWriter, r *http.Request) {
	header, rows, err := h.service.ExportSuppliers(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteCSV(w, "suppliers", header, rows)
}

func (h *Handler) exportOrders(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	header, rows, err := h.service.ExportOrders(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteCSV(w, "purchase-orders", header, rows)
}
