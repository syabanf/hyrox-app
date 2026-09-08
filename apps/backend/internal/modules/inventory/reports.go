package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
)

// What the shelf is worth, and how every quantity got there.

// Valuation is the whole shelf at its weighted-average cost.
//
// Average cost, not last purchase price: it is what the stock on hand actually
// cost, which is the only figure a balance sheet can defend.
func (s *Service) Valuation(ctx context.Context, branchID string) (domain.ValuationReport, error) {
	lines, err := s.repo.ValuationLines(ctx, branchID)
	if err != nil {
		return domain.ValuationReport{}, err
	}
	return domain.Value(lines), nil
}

func (r *Repository) ValuationLines(ctx context.Context, branchID string) ([]domain.ValuationLine, error) {
	query := `
		SELECT i.id, i.sku, i.name, i.unit, sum(l.qty_on_hand), i.unit_cost_idr
		FROM inventory.stock_levels l
		JOIN inventory.items i ON i.id = l.item_id
		WHERE l.qty_on_hand > 0`
	args := []any{}
	if branchID != "" {
		args = append(args, branchID)
		query += fmt.Sprintf(" AND l.branch_id = $%d", len(args))
	}
	query += ` GROUP BY i.id, i.sku, i.name, i.unit, i.unit_cost_idr`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: valuing stock: %w", err)
	}
	defer rows.Close()

	out := []domain.ValuationLine{}
	for rows.Next() {
		var line domain.ValuationLine
		if err := rows.Scan(&line.ItemID, &line.SKU, &line.Name, &line.Unit,
			&line.QtyOnHand, &line.UnitCost); err != nil {
			return nil, fmt.Errorf("inventory: scanning valuation line: %w", err)
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

// StockCard is one item's history at one branch, with the running balance
// beside every movement.
//
// The balance comes from the movement's own qty_after rather than being summed
// here: the ledger already stores what the quantity became, and recomputing it
// would produce a second answer that could disagree with the first.
func (s *Service) StockCard(ctx context.Context, itemID, branchID string,
	from, to time.Time) ([]domain.StockCardEntry, error) {

	return s.repo.StockCard(ctx, itemID, branchID, from, to)
}

func (r *Repository) StockCard(ctx context.Context, itemID, branchID string,
	from, to time.Time) ([]domain.StockCardEntry, error) {

	args := []any{itemID, from, to}
	query := `
		SELECT id, created_at, kind, COALESCE(reference_number, COALESCE(reason, '')),
		       qty, qty_after, unit_cost_idr
		FROM inventory.stock_movements
		WHERE item_id = $1 AND created_at >= $2 AND created_at < $3`
	if branchID != "" {
		args = append(args, branchID)
		query += fmt.Sprintf(" AND branch_id = $%d", len(args))
	}
	query += ` ORDER BY created_at, id`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: reading stock card: %w", err)
	}
	defer rows.Close()

	out := []domain.StockCardEntry{}
	for rows.Next() {
		var entry domain.StockCardEntry
		var qty domain.Quantity
		if err := rows.Scan(&entry.MovementID, &entry.At, &entry.Kind, &entry.Reference,
			&qty, &entry.Balance, &entry.UnitCost); err != nil {
			return nil, fmt.Errorf("inventory: scanning stock card entry: %w", err)
		}
		// In and out are separated rather than signed, because that is how a
		// stock card is read and added up by hand when somebody disputes it.
		if qty >= 0 {
			entry.In = qty
		} else {
			entry.Out = -qty
		}
		entry.ValueIDR = float64(entry.Balance) * entry.UnitCost
		out = append(out, entry)
	}
	return out, rows.Err()
}
