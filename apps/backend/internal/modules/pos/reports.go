package pos

import (
	"context"
	"fmt"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// What the counter did.
//
// Every report here reads completed sales only. A voided sale is money that
// came in and went out again, and counting it as takings makes a day look
// better than it was — the voids report exists precisely so those are visible
// on their own.

// ReportRange is the window a report covers. Both ends are inclusive days in
// the studio's timezone, because "yesterday" is a calendar question.
type ReportRange struct {
	BranchID string
	From     time.Time
	To       time.Time
}

func (r *Repository) reportWindow(window ReportRange) (string, []any) {
	clause := ` AND o.status = 'COMPLETED' AND o.completed_at >= $1 AND o.completed_at < $2`
	args := []any{window.From, window.To}
	if window.BranchID != "" {
		args = append(args, window.BranchID)
		clause += fmt.Sprintf(" AND o.branch_id = $%d", len(args))
	}
	return clause, args
}

// TransactionsReport is every completed sale in the window.
func (s *Service) TransactionsReport(ctx context.Context, window ReportRange) ([]domain.POSOrder, error) {
	return s.repo.CompletedOrders(ctx, window)
}

func (r *Repository) CompletedOrders(ctx context.Context, window ReportRange) ([]domain.POSOrder, error) {
	clause, args := r.reportWindow(window)
	rows, err := r.db.Query(ctx,
		`SELECT `+orderColumns+` FROM pos.orders o WHERE 1 = 1`+clause+
			` ORDER BY o.completed_at DESC LIMIT 1000`, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing completed orders: %w", err)
	}
	defer rows.Close()

	out := []domain.POSOrder{}
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning completed order: %w", err)
		}
		out = append(out, order)
	}
	return out, rows.Err()
}

// ProductSales is what sold, ranked by what it contributed.
func (s *Service) ProductSales(ctx context.Context, window ReportRange) ([]domain.ProfitLine, error) {
	lines, err := s.repo.ProductSales(ctx, window)
	if err != nil {
		return nil, err
	}
	return domain.RankProfit(lines), nil
}

func (r *Repository) ProductSales(ctx context.Context, window ReportRange) ([]domain.ProfitLine, error) {
	clause, args := r.reportWindow(window)
	rows, err := r.db.Query(ctx, `
		SELECT i.product_id, max(i.product_name),
		       sum(i.qty), sum(i.line_total_idr),
		       -- Cost is per base unit, so a carton costs its factor times as
		       -- much. Multiplying by the sold quantity would make every
		       -- multipack look like a miracle.
		       sum(i.qty * i.pack_factor * i.unit_cost_idr)
		FROM pos.order_items i
		JOIN pos.orders o ON o.id = i.order_id
		WHERE 1 = 1`+clause+`
		GROUP BY i.product_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: summarizing product sales: %w", err)
	}
	defer rows.Close()

	out := []domain.ProfitLine{}
	for rows.Next() {
		var line domain.ProfitLine
		if err := rows.Scan(&line.ProductID, &line.ProductName, &line.Qty,
			&line.SalesIDR, &line.CostIDR); err != nil {
			return nil, fmt.Errorf("pos: scanning product sales: %w", err)
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

// RevenueComposition is where the money came from, by category and by channel.
type RevenueComposition struct {
	ByCategory []domain.SalesBucket `json:"byCategory"`
	ByChannel  []domain.SalesBucket `json:"byChannel"`
	ByMethod   []domain.SalesBucket `json:"byMethod"`
	TotalIDR   float64              `json:"totalIdr"`
}

func (s *Service) RevenueComposition(ctx context.Context, window ReportRange) (RevenueComposition, error) {
	byCategory, err := s.repo.SalesByCategory(ctx, window)
	if err != nil {
		return RevenueComposition{}, err
	}
	byChannel, err := s.repo.SalesByChannel(ctx, window)
	if err != nil {
		return RevenueComposition{}, err
	}
	byMethod, err := s.repo.SalesByMethod(ctx, window)
	if err != nil {
		return RevenueComposition{}, err
	}

	composition := RevenueComposition{
		ByCategory: domain.ShareOut(byCategory),
		ByChannel:  domain.ShareOut(byChannel),
		ByMethod:   domain.ShareOut(byMethod),
	}
	for _, bucket := range composition.ByCategory {
		composition.TotalIDR += bucket.SalesIDR
	}
	return composition, nil
}

func (r *Repository) SalesByCategory(ctx context.Context, window ReportRange) ([]domain.SalesBucket, error) {
	clause, args := r.reportWindow(window)
	return r.buckets(ctx, `
		SELECT COALESCE(c.id, 'none'), COALESCE(c.name, 'Uncategorised'),
		       count(DISTINCT o.id), sum(i.qty), sum(i.line_total_idr)
		FROM pos.order_items i
		JOIN pos.orders o ON o.id = i.order_id
		LEFT JOIN pos.products p ON p.id = i.product_id
		LEFT JOIN pos.categories c ON c.id = p.category_id
		WHERE 1 = 1`+clause+`
		GROUP BY c.id, c.name ORDER BY 5 DESC`, args)
}

func (r *Repository) SalesByChannel(ctx context.Context, window ReportRange) ([]domain.SalesBucket, error) {
	clause, args := r.reportWindow(window)
	return r.buckets(ctx, `
		SELECT o.order_type, o.order_type, count(*), 0, sum(o.total_idr)
		FROM pos.orders o WHERE 1 = 1`+clause+`
		GROUP BY o.order_type ORDER BY 5 DESC`, args)
}

func (r *Repository) SalesByMethod(ctx context.Context, window ReportRange) ([]domain.SalesBucket, error) {
	clause, args := r.reportWindow(window)
	return r.buckets(ctx, `
		SELECT p.method, p.method, count(*), 0, sum(p.amount_idr - p.change_idr)
		FROM pos.payments p
		JOIN pos.orders o ON o.id = p.order_id
		WHERE 1 = 1`+clause+`
		GROUP BY p.method ORDER BY 5 DESC`, args)
}

func (r *Repository) buckets(ctx context.Context, query string, args []any) ([]domain.SalesBucket, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: bucketing sales: %w", err)
	}
	defer rows.Close()

	out := []domain.SalesBucket{}
	for rows.Next() {
		var bucket domain.SalesBucket
		if err := rows.Scan(&bucket.Key, &bucket.Label, &bucket.Orders,
			&bucket.Items, &bucket.SalesIDR); err != nil {
			return nil, fmt.Errorf("pos: scanning sales bucket: %w", err)
		}
		out = append(out, bucket)
	}
	return out, rows.Err()
}

// RushHour says when the counter is busy, which is the number that decides
// whether a second till is worth staffing.
func (s *Service) RushHour(ctx context.Context, window ReportRange) (domain.RushHour, error) {
	hourly, err := s.repo.HourlySales(ctx, window, s.studio)
	if err != nil {
		return domain.RushHour{}, err
	}
	return domain.SummarizeRushHour(hourly), nil
}

func (r *Repository) HourlySales(ctx context.Context, window ReportRange, studio *time.Location) ([]domain.HourlySale, error) {
	clause, args := r.reportWindow(window)
	// The hour is the studio's, not UTC's. A shop in Jakarta closing at 21:00
	// should not see its evening rush reported at 14:00.
	args = append(args, studio.String())
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT EXTRACT(HOUR FROM o.completed_at AT TIME ZONE $%d)::int,
		       count(*), 0, sum(o.total_idr)
		FROM pos.orders o WHERE 1 = 1%s
		GROUP BY 1 ORDER BY 1`, len(args), clause), args...)
	if err != nil {
		return nil, fmt.Errorf("pos: summarizing hourly sales: %w", err)
	}
	defer rows.Close()

	out := []domain.HourlySale{}
	for rows.Next() {
		var sale domain.HourlySale
		if err := rows.Scan(&sale.Hour, &sale.Orders, &sale.Items, &sale.SalesIDR); err != nil {
			return nil, fmt.Errorf("pos: scanning hourly sales: %w", err)
		}
		out = append(out, sale)
	}
	return out, rows.Err()
}

// VoidsReport is every unwound sale, with who unwound it and why.
//
// It is a report rather than a filter on the sales list because a void is the
// one action that makes money disappear, and the point of the report is that
// somebody looks at them together.
func (s *Service) VoidsReport(ctx context.Context, window ReportRange) ([]domain.POSOrder, error) {
	return s.repo.VoidedOrders(ctx, window)
}

func (r *Repository) VoidedOrders(ctx context.Context, window ReportRange) ([]domain.POSOrder, error) {
	args := []any{window.From, window.To}
	clause := ` AND o.status = 'VOIDED' AND o.voided_at >= $1 AND o.voided_at < $2`
	if window.BranchID != "" {
		args = append(args, window.BranchID)
		clause += fmt.Sprintf(" AND o.branch_id = $%d", len(args))
	}

	rows, err := r.db.Query(ctx,
		`SELECT `+orderColumns+` FROM pos.orders o WHERE 1 = 1`+clause+
			` ORDER BY o.voided_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing voided orders: %w", err)
	}
	defer rows.Close()

	out := []domain.POSOrder{}
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning voided order: %w", err)
		}
		out = append(out, order)
	}
	return out, rows.Err()
}

// ClosingReport is the cash-up: what the drawer should hold, what it held, and
// the difference.
func (s *Service) ClosingReport(ctx context.Context, shiftID string) (domain.ClosingReport, error) {
	// The totals come from the same place the shift screen reads them, so a
	// cash-up and the screen behind it can never disagree.
	view, err := s.Shift(ctx, shiftID)
	if err != nil {
		return domain.ClosingReport{}, err
	}
	shift, totals := view.CashierShift, view.Totals

	byMethod, err := s.repo.ShiftByMethod(ctx, shiftID)
	if err != nil {
		return domain.ClosingReport{}, err
	}

	report := domain.ClosingReport{
		ShiftID: shift.ID, ShiftNumber: shift.ShiftNumber, CashierName: shift.CashierName,
		OpenedAt: shift.OpenedAt, ClosedAt: shift.ClosedAt, Totals: totals,
		ByMethod:        domain.ShareOut(byMethod),
		CountedCashIDR:  shift.ClosingCashIDR,
		ExpectedCashIDR: shift.ExpectedCashIDR,
		VarianceIDR:     shift.VarianceIDR,
		// Over and short are different conversations, so the report names
		// which one this is rather than leaving a signed number to be read.
		Short: shift.ClosingCashIDR != nil && shift.VarianceIDR < 0,
	}
	return report, nil
}

func (r *Repository) ShiftByMethod(ctx context.Context, shiftID string) ([]domain.SalesBucket, error) {
	return r.buckets(ctx, `
		SELECT p.method, p.method, count(*), 0, sum(p.amount_idr - p.change_idr)
		FROM pos.payments p
		JOIN pos.orders o ON o.id = p.order_id
		WHERE o.shift_id = $1 AND o.status = 'COMPLETED'
		GROUP BY p.method ORDER BY 5 DESC`, []any{shiftID})
}

// ValidateRange refuses a window that runs backwards, which otherwise returns
// an empty report that looks like a quiet week.
func ValidateRange(from, to time.Time) error {
	if to.Before(from) {
		return httpx.Invalid("That date range ends before it starts.")
	}
	return nil
}
