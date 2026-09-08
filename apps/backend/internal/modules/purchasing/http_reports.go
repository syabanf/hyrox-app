package purchasing

import (
	"net/http"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// windowFrom reads a report's date range, defaulting to the last quarter.
//
// Buying moves more slowly than selling — a purchase order raised in January
// can still be arriving in March — so the default window is wider than the
// till's.
func (h *Handler) windowFrom(r *http.Request) (ReportRange, error) {
	studio := h.service.studio
	now := h.service.clock.Now().In(studio)

	to := now.AddDate(0, 0, 1)
	from := to.AddDate(0, -3, 0)

	if raw := httpx.Query(r, "from"); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, studio)
		if err != nil {
			return ReportRange{}, httpx.Invalid("from must be a date as YYYY-MM-DD.")
		}
		from = parsed
	}
	if raw := httpx.Query(r, "to"); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, studio)
		if err != nil {
			return ReportRange{}, httpx.Invalid("to must be a date as YYYY-MM-DD.")
		}
		to = parsed.AddDate(0, 0, 1)
	}
	if to.Before(from) {
		return ReportRange{}, httpx.Invalid("That date range ends before it starts.")
	}
	return ReportRange{
		BranchID:   httpx.Query(r, "branchId"),
		SupplierID: httpx.Query(r, "supplierId"),
		From:       from, To: to,
	}, nil
}

func (h *Handler) reportOrders(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	summary, err := h.service.OrderSummary(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summary)
}

func (h *Handler) reportSuppliers(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	performance, err := h.service.SupplierPerformance(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, performance)
}

func (h *Handler) reportPriceHistory(w http.ResponseWriter, r *http.Request) {
	history, err := h.service.PriceHistory(r.Context(),
		httpx.Query(r, "itemId"), httpx.QueryInt(r, "limit", 50))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, history)
}
