package pos

import (
	"net/http"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// windowFrom reads a report's date range off the query string.
//
// It defaults to the last thirty days rather than to everything: a report with
// no window is a table scan that gets slower every month, and "since we
// opened" is almost never the question.
func (h *Handler) windowFrom(r *http.Request) (ReportRange, error) {
	studio := h.service.studio
	now := h.service.clock.Now().In(studio)

	to := now.AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -31)

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
		// The end of the range is inclusive of its day: a report "to Tuesday"
		// that stops at midnight on Tuesday morning silently loses a day.
		to = parsed.AddDate(0, 0, 1)
	}
	if err := ValidateRange(from, to); err != nil {
		return ReportRange{}, err
	}
	return ReportRange{BranchID: httpx.Query(r, "branchId"), From: from, To: to}, nil
}

func (h *Handler) reportTransactions(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	orders, err := h.service.TransactionsReport(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, orders)
}

func (h *Handler) reportProductSales(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lines, err := h.service.ProductSales(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, lines)
}

func (h *Handler) reportRevenue(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	composition, err := h.service.RevenueComposition(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, composition)
}

func (h *Handler) reportRushHour(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	report, err := h.service.RushHour(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, report)
}

func (h *Handler) reportVoids(w http.ResponseWriter, r *http.Request) {
	window, err := h.windowFrom(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	orders, err := h.service.VoidsReport(r.Context(), window)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, orders)
}

func (h *Handler) reportClosing(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.ClosingReport(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, report)
}
