package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

const batchColumns = `id, item_id, branch_id, batch_code, expires_on, qty_on_hand,
	unit_cost_idr, received_on, receipt_id, receipt_number, note`

func scanBatch(row pgx.Row) (domain.Batch, error) {
	var b domain.Batch
	var expiresOn *time.Time
	err := row.Scan(&b.ID, &b.ItemID, &b.BranchID, &b.BatchCode, &expiresOn, &b.QtyOnHand,
		&b.UnitCostIDR, &b.ReceivedOn, &b.ReceiptID, &b.ReceiptNumber, &b.Note)
	if err != nil {
		return domain.Batch{}, err
	}
	b.ExpiresOn = optionalDate(expiresOn)
	return b, nil
}

// BatchFilter narrows the batch list.
type BatchFilter struct {
	ItemID   string
	BranchID string
	// InStockOnly hides emptied batches, which are kept forever so a recall
	// can still be answered after the goods have gone.
	InStockOnly bool
	// ExpiringBefore limits to batches dated on or before a day.
	ExpiringBefore domain.Date
	Limit          int
}

func (r *Repository) Batches(ctx context.Context, filter BatchFilter) ([]domain.Batch, error) {
	query := `SELECT ` + batchColumns + ` FROM inventory.batches WHERE 1 = 1`
	args := []any{}

	if filter.ItemID != "" {
		args = append(args, filter.ItemID)
		query += fmt.Sprintf(" AND item_id = $%d", len(args))
	}
	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(" AND branch_id = $%d", len(args))
	}
	if filter.InStockOnly {
		query += " AND qty_on_hand > 0"
	}
	if filter.ExpiringBefore != "" {
		args = append(args, string(filter.ExpiringBefore))
		query += fmt.Sprintf(" AND expires_on IS NOT NULL AND expires_on <= $%d", len(args))
	}
	// FEFO order, so a caller that only wants the front of the shelf gets it
	// without sorting again.
	query += " ORDER BY expires_on NULLS LAST, received_on"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing batches: %w", err)
	}
	defer rows.Close()

	out := []domain.Batch{}
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning batch: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BatchesForIssue locks every batch of one item at one branch that still holds
// stock, in FEFO order.
//
// The lock is the point: two tills selling the last case at the same moment
// must not both be told they may have it.
func (r *Repository) BatchesForIssue(ctx context.Context, itemID, branchID string) ([]domain.Batch, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+batchColumns+` FROM inventory.batches
		 WHERE item_id = $1 AND branch_id = $2 AND qty_on_hand > 0
		 ORDER BY expires_on NULLS LAST, received_on
		 FOR UPDATE`, itemID, branchID)
	if err != nil {
		return nil, fmt.Errorf("inventory: locking batches: %w", err)
	}
	defer rows.Close()

	out := []domain.Batch{}
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning batch: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpsertBatch books goods into a batch, adding to it when the same code
// arrives twice. The cost is a weighted average within the batch, for the same
// reason it is at item level: two prices for one physical pile is not a thing.
func (r *Repository) UpsertBatch(ctx context.Context, b domain.Batch) (domain.Batch, error) {
	saved, err := scanBatch(r.db.QueryRow(ctx, `
		INSERT INTO inventory.batches (id, item_id, branch_id, batch_code, expires_on,
			qty_on_hand, unit_cost_idr, receipt_id, receipt_number, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (item_id, branch_id, batch_code) DO UPDATE SET
			qty_on_hand = inventory.batches.qty_on_hand + EXCLUDED.qty_on_hand,
			unit_cost_idr = CASE
				WHEN inventory.batches.qty_on_hand + EXCLUDED.qty_on_hand > 0
				THEN round((inventory.batches.qty_on_hand * inventory.batches.unit_cost_idr
				          + EXCLUDED.qty_on_hand * EXCLUDED.unit_cost_idr)
				         / (inventory.batches.qty_on_hand + EXCLUDED.qty_on_hand), 2)
				ELSE EXCLUDED.unit_cost_idr END,
			expires_on = COALESCE(EXCLUDED.expires_on, inventory.batches.expires_on),
			updated_at = now()
		RETURNING `+batchColumns,
		b.ID, b.ItemID, b.BranchID, b.BatchCode, dateValue(b.ExpiresOn), b.QtyOnHand,
		b.UnitCostIDR, b.ReceiptID, b.ReceiptNumber, b.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.Batch{}, httpx.NotFound("item")
	}
	if err != nil {
		return domain.Batch{}, fmt.Errorf("inventory: saving batch: %w", err)
	}
	return saved, nil
}

// DrawFromBatch takes stock out of one batch and records that it did.
//
// The two writes are one act: a batch quantity that moved without a
// batch_movements row explaining it is exactly the drift this table exists to
// make impossible.
func (r *Repository) DrawFromBatch(ctx context.Context, movementID string, a domain.Allocation, id string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE inventory.batches SET qty_on_hand = $2, updated_at = now() WHERE id = $1`,
		a.BatchID, a.QtyAfter)
	if err != nil {
		return fmt.Errorf("inventory: drawing from batch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("batch")
	}

	if _, err := r.db.Exec(ctx, `
		INSERT INTO inventory.batch_movements (id, batch_id, movement_id, qty, qty_before, qty_after)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		id, a.BatchID, movementID, a.Qty, a.QtyBefore, a.QtyAfter); err != nil {
		return fmt.Errorf("inventory: recording batch movement: %w", err)
	}
	return nil
}

// BatchAllocations is which batches one movement drew on, for a receipt that
// has to answer "which date did we sell them".
func (r *Repository) BatchAllocations(ctx context.Context, movementID string) ([]domain.Allocation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.batch_code, b.expires_on, m.qty, m.qty_before, m.qty_after, b.unit_cost_idr
		FROM inventory.batch_movements m
		JOIN inventory.batches b ON b.id = m.batch_id
		WHERE m.movement_id = $1 ORDER BY m.created_at`, movementID)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing batch allocations: %w", err)
	}
	defer rows.Close()

	out := []domain.Allocation{}
	for rows.Next() {
		var a domain.Allocation
		var expiresOn *time.Time
		if err := rows.Scan(&a.BatchID, &a.BatchCode, &expiresOn, &a.Qty,
			&a.QtyBefore, &a.QtyAfter, &a.UnitCostIDR); err != nil {
			return nil, fmt.Errorf("inventory: scanning batch allocation: %w", err)
		}
		a.ExpiresOn = optionalDate(expiresOn)
		out = append(out, a)
	}
	return out, rows.Err()
}

// WarningDaysOf is each item's expiry warning window, for a report that judges
// every batch against its own item rather than one number for the catalogue.
func (r *Repository) WarningDaysOf(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT id, expiry_warning_days FROM inventory.items`)
	if err != nil {
		return nil, fmt.Errorf("inventory: reading warning windows: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var id string
		var days int
		if err := rows.Scan(&id, &days); err != nil {
			return nil, fmt.Errorf("inventory: scanning warning window: %w", err)
		}
		out[id] = days
	}
	return out, rows.Err()
}

// dateValue and optionalDate move calendar dates across the driver boundary.
// A batch's expiry is a date rather than an instant: "expires on 3 December"
// is true whatever time it is, and keeping it out of time.Time is what stops
// it drifting a day on its way through a timezone.
func dateValue(d *domain.Date) any {
	if d == nil || *d == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", string(*d))
	if err != nil {
		return nil
	}
	return parsed
}

func optionalDate(t *time.Time) *domain.Date {
	if t == nil {
		return nil
	}
	d := domain.Date(t.Format("2006-01-02"))
	return &d
}

// RecordBatchArrival notes that a batch grew, without moving it again: the
// upsert has already applied the quantity, and this is the row that explains
// where it came from.
func (r *Repository) RecordBatchArrival(ctx context.Context, movementID string,
	batch domain.Batch, qty domain.Quantity, id string) error {

	before := domain.RoundQuantity(batch.QtyOnHand - qty)
	if _, err := r.db.Exec(ctx, `
		INSERT INTO inventory.batch_movements (id, batch_id, movement_id, qty, qty_before, qty_after)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		id, batch.ID, movementID, qty, before, batch.QtyOnHand); err != nil {
		return fmt.Errorf("inventory: recording batch arrival: %w", err)
	}
	return nil
}

// OutwardAllocations is which batches a document took stock out of, so a
// reversal can put it back where it came from. Most recent first, because the
// last thing taken is the first thing given back.
func (r *Repository) OutwardAllocations(ctx context.Context, itemID, branchID,
	referenceType, referenceID string) ([]domain.Allocation, error) {

	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.batch_code, b.expires_on, a.qty, a.qty_before, a.qty_after, b.unit_cost_idr
		FROM inventory.batch_movements a
		JOIN inventory.batches b ON b.id = a.batch_id
		JOIN inventory.stock_movements m ON m.id = a.movement_id
		WHERE b.item_id = $1 AND b.branch_id = $2
		  AND m.reference_type = $3 AND m.reference_id = $4 AND a.qty < 0
		ORDER BY a.created_at DESC`, itemID, branchID, referenceType, referenceID)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing outward allocations: %w", err)
	}
	defer rows.Close()

	out := []domain.Allocation{}
	for rows.Next() {
		var a domain.Allocation
		var expiresOn *time.Time
		if err := rows.Scan(&a.BatchID, &a.BatchCode, &expiresOn, &a.Qty,
			&a.QtyBefore, &a.QtyAfter, &a.UnitCostIDR); err != nil {
			return nil, fmt.Errorf("inventory: scanning outward allocation: %w", err)
		}
		a.ExpiresOn = optionalDate(expiresOn)
		out = append(out, a)
	}
	return out, rows.Err()
}

// BatchQty reads and locks one batch's quantity, for a return that has to add
// to whatever is there now rather than to whatever was there then.
func (r *Repository) BatchQty(ctx context.Context, batchID string) (domain.Quantity, error) {
	var qty domain.Quantity
	err := r.db.QueryRow(ctx,
		`SELECT qty_on_hand FROM inventory.batches WHERE id = $1 FOR UPDATE`, batchID).Scan(&qty)
	if database.IsNoRows(err) {
		return 0, httpx.NotFound("batch")
	}
	if err != nil {
		return 0, fmt.Errorf("inventory: reading batch quantity: %w", err)
	}
	return qty, nil
}
