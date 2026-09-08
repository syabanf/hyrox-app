package inventory

import (
	"net/http"

	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// The catalogue as a spreadsheet.

const maxUploadBytes = 8 << 20

func (h *Handler) importItems(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Body(r, maxUploadBytes)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := httpx.ReadCSV(body, []string{"sku", "name"})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// A dry run unless somebody explicitly asks to apply it. Three hundred
	// rows of mistake is not something to discover afterwards.
	result, err := h.service.ImportItems(r.Context(), rows,
		httpx.QueryBool(r, "apply", false), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, result)
}

func (h *Handler) exportItems(w http.ResponseWriter, r *http.Request) {
	header, rows, err := h.service.ExportItems(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteCSV(w, "items", header, rows)
}

func (h *Handler) exportStock(w http.ResponseWriter, r *http.Request) {
	header, rows, err := h.service.ExportStock(r.Context(), httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.WriteCSV(w, "stock", header, rows)
}
