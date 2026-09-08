package purchasing

import (
	"context"
	"fmt"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
)

// What buying actually cost, and how suppliers actually behaved.

// ReportRange is the window a report covers.
type ReportRange struct {
	BranchID   string
	SupplierID string
	From       time.Time
	To         time.Time
}

// OrderSummary is spend by supplier and by status over a window.
type OrderSummary struct {
	Orders      int     `json:"orders"`
	TotalIDR    float64 `json:"totalIdr"`
	ReceivedIDR float64 `json:"receivedIdr"`
	// Outstanding is what has been committed to but has not arrived, which is
	// the number a cash-flow conversation is actually about.
	OutstandingIDR float64              `json:"outstandingIdr"`
	BySupplier     []domain.SalesBucket `json:"bySupplier"`
	ByStatus       []domain.SalesBucket `json:"byStatus"`
}

func (s *Service) OrderSummary(ctx context.Context, window ReportRange) (OrderSummary, error) {
	summary, err := s.repo.OrderSummary(ctx, window)
	if err != nil {
		return OrderSummary{}, err
	}
	summary.BySupplier = domain.ShareOut(summary.BySupplier)
	summary.ByStatus = domain.ShareOut(summary.ByStatus)
	return summary, nil
}

func (r *Repository) OrderSummary(ctx context.Context, window ReportRange) (OrderSummary, error) {
	args := []any{window.From, window.To}
	clause := ` AND o.ordered_on >= $1 AND o.ordered_on < $2 AND o.status <> 'CANCELLED'`
	if window.BranchID != "" {
		args = append(args, window.BranchID)
		clause += fmt.Sprintf(" AND o.branch_id = $%d", len(args))
	}
	if window.SupplierID != "" {
		args = append(args, window.SupplierID)
		clause += fmt.Sprintf(" AND o.supplier_id = $%d", len(args))
	}

	var summary OrderSummary
	// Received value is computed from the lines rather than from the order's
	// total: a partly received order has arrived in part, and calling the
	// whole total "received" is how outstanding commitments disappear.
	if err := r.db.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(o.total_idr), 0),
		       COALESCE(sum((SELECT COALESCE(sum(i.qty_received * i.unit_price_idr), 0)
		                     FROM purchasing.purchase_order_items i
		                     WHERE i.order_id = o.id)), 0)
		FROM purchasing.purchase_orders o WHERE 1 = 1`+clause, args...).
		Scan(&summary.Orders, &summary.TotalIDR, &summary.ReceivedIDR); err != nil {
		return OrderSummary{}, fmt.Errorf("purchasing: summarizing orders: %w", err)
	}
	summary.OutstandingIDR = summary.TotalIDR - summary.ReceivedIDR

	bySupplier, err := r.orderBuckets(ctx, `
		SELECT s.id, s.name, count(*), 0, sum(o.total_idr)
		FROM purchasing.purchase_orders o
		JOIN purchasing.suppliers s ON s.id = o.supplier_id
		WHERE 1 = 1`+clause+` GROUP BY s.id, s.name ORDER BY 5 DESC`, args)
	if err != nil {
		return OrderSummary{}, err
	}
	byStatus, err := r.orderBuckets(ctx, `
		SELECT o.status, o.status, count(*), 0, sum(o.total_idr)
		FROM purchasing.purchase_orders o
		WHERE 1 = 1`+clause+` GROUP BY o.status ORDER BY 5 DESC`, args)
	if err != nil {
		return OrderSummary{}, err
	}

	summary.BySupplier, summary.ByStatus = bySupplier, byStatus
	return summary, nil
}

func (r *Repository) orderBuckets(ctx context.Context, query string, args []any) ([]domain.SalesBucket, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: bucketing orders: %w", err)
	}
	defer rows.Close()

	out := []domain.SalesBucket{}
	for rows.Next() {
		var bucket domain.SalesBucket
		if err := rows.Scan(&bucket.Key, &bucket.Label, &bucket.Orders,
			&bucket.Items, &bucket.SalesIDR); err != nil {
			return nil, fmt.Errorf("purchasing: scanning order bucket: %w", err)
		}
		out = append(out, bucket)
	}
	return out, rows.Err()
}

// SupplierPerformance is how suppliers actually behave, as opposed to what
// their price lists say.
func (s *Service) SupplierPerformance(ctx context.Context, window ReportRange) ([]domain.SupplierPerformance, error) {
	deliveries, err := s.repo.DeliveryOutcomes(ctx, window)
	if err != nil {
		return nil, err
	}
	return domain.RankSuppliers(deliveries), nil
}

// DeliveryOutcomes is one row per order: what was ordered against what turned
// up. Named apart from Deliveries, which is the receiving bay's own document.
func (r *Repository) DeliveryOutcomes(ctx context.Context, window ReportRange) ([]domain.SupplierDelivery, error) {
	args := []any{window.From, window.To}
	clause := ` AND o.ordered_on >= $1 AND o.ordered_on < $2 AND o.status <> 'CANCELLED'`
	if window.SupplierID != "" {
		args = append(args, window.SupplierID)
		clause += fmt.Sprintf(" AND o.supplier_id = $%d", len(args))
	}

	// One row per order: the quantities are in base units so a supplier who
	// ships cartons and one who ships pieces are compared on the same scale.
	rows, err := r.db.Query(ctx, `
		SELECT o.supplier_id, s.name, o.total_idr,
		       COALESCE(sum(i.qty_ordered * i.pack_factor), 0),
		       COALESCE(sum(g.accepted), 0), COALESCE(sum(g.rejected), 0),
		       COALESCE(max(o.expected_on - o.ordered_on), 0),
		       -- On time is judged against the date promised, and an order
		       -- with no promised date cannot be late.
		       bool_or(o.expected_on IS NULL OR r.received_on <= o.expected_on)
		FROM purchasing.purchase_orders o
		JOIN purchasing.suppliers s ON s.id = o.supplier_id
		LEFT JOIN purchasing.purchase_order_items i ON i.order_id = o.id
		LEFT JOIN LATERAL (
			SELECT sum(gi.qty_accepted * gi.pack_factor) AS accepted,
			       sum(gi.qty_rejected * gi.pack_factor) AS rejected
			FROM purchasing.goods_receipt_items gi
			WHERE gi.order_item_id = i.id
		) g ON true
		LEFT JOIN purchasing.goods_receipts r ON r.order_id = o.id AND r.status = 'POSTED'
		WHERE 1 = 1`+clause+`
		GROUP BY o.id, o.supplier_id, s.name, o.total_idr`, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: reading deliveries: %w", err)
	}
	defer rows.Close()

	out := []domain.SupplierDelivery{}
	for rows.Next() {
		var delivery domain.SupplierDelivery
		var onTime *bool
		if err := rows.Scan(&delivery.SupplierID, &delivery.SupplierName, &delivery.TotalIDR,
			&delivery.QtyOrdered, &delivery.QtyAccepted, &delivery.QtyRejected,
			&delivery.LeadDays, &onTime); err != nil {
			return nil, fmt.Errorf("purchasing: scanning delivery: %w", err)
		}
		delivery.OnTime = onTime != nil && *onTime
		out = append(out, delivery)
	}
	return out, rows.Err()
}

// PriceHistory is what one item has actually cost over time, per supplier.
//
// It reads receipts rather than price lists: a quoted price is a promise and a
// received price is a fact, and the gap between them is the conversation.
type PriceHistoryEntry struct {
	ReceivedOn   domain.Date `json:"receivedOn"`
	SupplierID   string      `json:"supplierId"`
	SupplierName string      `json:"supplierName"`
	GRNNumber    string      `json:"grnNumber"`
	Unit         string      `json:"unit"`
	PackFactor   float64     `json:"packFactor"`
	// Both prices, because a carton price that fell while the pack size fell
	// further is a price rise.
	PackPriceIDR float64         `json:"packPriceIdr"`
	UnitPriceIDR float64         `json:"unitPriceIdr"`
	Qty          domain.Quantity `json:"qty"`
}

func (s *Service) PriceHistory(ctx context.Context, itemID string, limit int) ([]PriceHistoryEntry, error) {
	return s.repo.PriceHistory(ctx, itemID, limit)
}

func (r *Repository) PriceHistory(ctx context.Context, itemID string, limit int) ([]PriceHistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT g.received_on, s.id, s.name, g.grn_number, gi.unit, gi.pack_factor,
		       gi.unit_price_idr, gi.qty_accepted
		FROM purchasing.goods_receipt_items gi
		JOIN purchasing.goods_receipts g ON g.id = gi.receipt_id
		JOIN purchasing.suppliers s ON s.id = g.supplier_id
		WHERE gi.item_id = $1 AND g.status = 'POSTED' AND gi.qty_accepted > 0
		ORDER BY g.received_on DESC LIMIT $2`, itemID, limit)
	if err != nil {
		return nil, fmt.Errorf("purchasing: reading price history: %w", err)
	}
	defer rows.Close()

	out := []PriceHistoryEntry{}
	for rows.Next() {
		var entry PriceHistoryEntry
		var receivedOn time.Time
		if err := rows.Scan(&receivedOn, &entry.SupplierID, &entry.SupplierName,
			&entry.GRNNumber, &entry.Unit, &entry.PackFactor,
			&entry.PackPriceIDR, &entry.Qty); err != nil {
			return nil, fmt.Errorf("purchasing: scanning price history: %w", err)
		}
		entry.ReceivedOn = scanDate(&receivedOn)
		entry.UnitPriceIDR = domain.PackPriceToBase(entry.PackPriceIDR, entry.PackFactor)
		out = append(out, entry)
	}
	return out, rows.Err()
}
