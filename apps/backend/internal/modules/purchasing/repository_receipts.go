package purchasing

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// What arrived, and what went back.

// ── Goods receipts ───────────────────────────────────────────────────────────

const receiptColumns = `id, grn_number, order_id, supplier_id, branch_id, received_on,
	received_by, received_by_name, delivery_note_number, delivery_id, status, note, posted_at,
	created_at, updated_at`

func scanReceipt(row pgx.Row) (domain.GoodsReceipt, error) {
	var g domain.GoodsReceipt
	var receivedOn time.Time
	err := row.Scan(&g.ID, &g.GRNNumber, &g.OrderID, &g.SupplierID, &g.BranchID, &receivedOn,
		&g.ReceivedBy, &g.ReceivedByName, &g.DeliveryNoteNumber, &g.DeliveryID, &g.Status, &g.Note,
		&g.PostedAt, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return domain.GoodsReceipt{}, err
	}
	g.ReceivedOn = scanDate(&receivedOn)
	return g, nil
}

// ReceiptFilter narrows the receipt list.
type ReceiptFilter struct {
	OrderID  string
	BranchID string
	Status   string
	Limit    int
}

func (r *Repository) Receipts(ctx context.Context, filter ReceiptFilter) ([]domain.GoodsReceipt, error) {
	query := `SELECT ` + receiptColumns + ` FROM purchasing.goods_receipts WHERE 1 = 1`
	args := []any{}

	if filter.OrderID != "" {
		args = append(args, filter.OrderID)
		query += fmt.Sprintf(` AND order_id = $%d`, len(args))
	}
	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY received_on DESC, created_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing receipts: %w", err)
	}
	defer rows.Close()

	out := []domain.GoodsReceipt{}
	for rows.Next() {
		g, err := scanReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning receipt: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *Repository) Receipt(ctx context.Context, id string, forUpdate bool) (domain.GoodsReceipt, error) {
	query := `SELECT ` + receiptColumns + ` FROM purchasing.goods_receipts WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	g, err := scanReceipt(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.GoodsReceipt{}, httpx.NotFound("goods receipt")
	}
	if err != nil {
		return domain.GoodsReceipt{}, fmt.Errorf("purchasing: reading receipt: %w", err)
	}
	return g, nil
}

func (r *Repository) InsertReceipt(ctx context.Context, g domain.GoodsReceipt) (domain.GoodsReceipt, error) {
	created, err := scanReceipt(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.goods_receipts (id, grn_number, order_id, supplier_id, branch_id,
			received_on, received_by, received_by_name, delivery_note_number, delivery_id,
			status, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING `+receiptColumns,
		g.ID, g.GRNNumber, g.OrderID, g.SupplierID, g.BranchID, requiredDate(g.ReceivedOn),
		g.ReceivedBy, g.ReceivedByName, g.DeliveryNoteNumber, g.DeliveryID, g.Status, g.Note))
	if database.IsUniqueViolation(err) {
		return domain.GoodsReceipt{}, httpx.Conflict("DUPLICATE", "That receipt number is taken.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.GoodsReceipt{}, httpx.NotFound("purchase order")
	}
	if err != nil {
		return domain.GoodsReceipt{}, fmt.Errorf("purchasing: inserting receipt: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveReceipt(ctx context.Context, g domain.GoodsReceipt) (domain.GoodsReceipt, error) {
	saved, err := scanReceipt(r.db.QueryRow(ctx, `
		UPDATE purchasing.goods_receipts SET status = $2, note = $3, delivery_note_number = $4,
			posted_at = $5, updated_at = now()
		WHERE id = $1 RETURNING `+receiptColumns,
		g.ID, g.Status, g.Note, g.DeliveryNoteNumber, g.PostedAt))
	if database.IsNoRows(err) {
		return domain.GoodsReceipt{}, httpx.NotFound("goods receipt")
	}
	if err != nil {
		return domain.GoodsReceipt{}, fmt.Errorf("purchasing: saving receipt: %w", err)
	}
	return saved, nil
}

const receiptItemColumns = `id, receipt_id, order_item_id, item_id, qty_accepted, qty_rejected,
	qty_returned, unit, pack_factor, unit_price_idr, qc_status, batch_number, expires_on, note`

func scanReceiptItem(row pgx.Row) (domain.GoodsReceiptItem, error) {
	var i domain.GoodsReceiptItem
	var expiresOn *time.Time
	err := row.Scan(&i.ID, &i.ReceiptID, &i.OrderItemID, &i.ItemID, &i.QtyAccepted,
		&i.QtyRejected, &i.QtyReturned, &i.Unit, &i.PackFactor, &i.UnitPriceIDR,
		&i.QCStatus, &i.BatchNumber, &expiresOn, &i.Note)
	if err != nil {
		return domain.GoodsReceiptItem{}, err
	}
	i.ExpiresOn = optionalDate(expiresOn)
	return i, nil
}

func (r *Repository) ReceiptItems(ctx context.Context, receiptID string) ([]domain.GoodsReceiptItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+receiptItemColumns+` FROM purchasing.goods_receipt_items WHERE receipt_id = $1 ORDER BY id`,
		receiptID)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing receipt items: %w", err)
	}
	defer rows.Close()

	out := []domain.GoodsReceiptItem{}
	for rows.Next() {
		i, err := scanReceiptItem(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning receipt item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *Repository) ReceiptItem(ctx context.Context, id string, forUpdate bool) (domain.GoodsReceiptItem, error) {
	query := `SELECT ` + receiptItemColumns + ` FROM purchasing.goods_receipt_items WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	i, err := scanReceiptItem(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.GoodsReceiptItem{}, httpx.NotFound("receipt line")
	}
	if err != nil {
		return domain.GoodsReceiptItem{}, fmt.Errorf("purchasing: reading receipt item: %w", err)
	}
	return i, nil
}

func (r *Repository) InsertReceiptItem(ctx context.Context, i domain.GoodsReceiptItem) (domain.GoodsReceiptItem, error) {
	created, err := scanReceiptItem(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.goods_receipt_items (id, receipt_id, order_item_id, item_id,
			qty_accepted, qty_rejected, unit, pack_factor, unit_price_idr, qc_status,
			batch_number, expires_on, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING `+receiptItemColumns,
		i.ID, i.ReceiptID, i.OrderItemID, i.ItemID, i.QtyAccepted, i.QtyRejected,
		i.Unit, i.PackFactor, i.UnitPriceIDR, i.QCStatus, i.BatchNumber,
		dateValue(i.ExpiresOn), i.Note))
	if database.IsCheckViolation(err) {
		return domain.GoodsReceiptItem{}, httpx.Invalid("A delivery line needs something on it.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.GoodsReceiptItem{}, httpx.NotFound("goods receipt or order line")
	}
	if err != nil {
		return domain.GoodsReceiptItem{}, fmt.Errorf("purchasing: inserting receipt item: %w", err)
	}
	return created, nil
}

func (r *Repository) DeleteReceiptItem(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM purchasing.goods_receipt_items WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.Conflict("IN_USE", "That line has a return against it.")
	}
	if err != nil {
		return fmt.Errorf("purchasing: deleting receipt item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("receipt line")
	}
	return nil
}

// AddReturnedQty books goods going back against a receipt line. The CHECK is
// what stops more going back than was ever taken in.
func (r *Repository) AddReturnedQty(ctx context.Context, receiptItemID string, qty domain.Quantity) error {
	_, err := r.db.Exec(ctx,
		`UPDATE purchasing.goods_receipt_items SET qty_returned = qty_returned + $2 WHERE id = $1`,
		receiptItemID, qty)
	if database.IsCheckViolation(err) {
		return httpx.Conflict("OVER_RETURNED", "That is more than was accepted on the line.")
	}
	if err != nil {
		return fmt.Errorf("purchasing: booking returned quantity: %w", err)
	}
	return nil
}

// ── Returns ──────────────────────────────────────────────────────────────────

const returnColumns = `id, return_number, receipt_id, supplier_id, branch_id, returned_on,
	reason_type, reason_note, status, total_idr, posted_at, submitted_at, approved_by,
	approved_at, rejected_by, rejected_at, decision_note, credit_id, created_at, updated_at`

func scanReturn(row pgx.Row) (domain.PurchaseReturn, error) {
	var p domain.PurchaseReturn
	var returnedOn time.Time
	err := row.Scan(&p.ID, &p.ReturnNumber, &p.ReceiptID, &p.SupplierID, &p.BranchID,
		&returnedOn, &p.ReasonType, &p.ReasonNote, &p.Status, &p.TotalIDR, &p.PostedAt,
		&p.SubmittedAt, &p.ApprovedBy, &p.ApprovedAt, &p.RejectedBy, &p.RejectedAt,
		&p.DecisionNote, &p.CreditID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return domain.PurchaseReturn{}, err
	}
	p.ReturnedOn = scanDate(&returnedOn)
	return p, nil
}

func (r *Repository) Returns(ctx context.Context, receiptID, status string, limit int) ([]domain.PurchaseReturn, error) {
	query := `SELECT ` + returnColumns + ` FROM purchasing.purchase_returns WHERE 1 = 1`
	args := []any{}
	if receiptID != "" {
		args = append(args, receiptID)
		query += fmt.Sprintf(` AND receipt_id = $%d`, len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY returned_on DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing returns: %w", err)
	}
	defer rows.Close()

	out := []domain.PurchaseReturn{}
	for rows.Next() {
		p, err := scanReturn(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning return: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) Return(ctx context.Context, id string, forUpdate bool) (domain.PurchaseReturn, error) {
	query := `SELECT ` + returnColumns + ` FROM purchasing.purchase_returns WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	p, err := scanReturn(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.PurchaseReturn{}, httpx.NotFound("purchase return")
	}
	if err != nil {
		return domain.PurchaseReturn{}, fmt.Errorf("purchasing: reading return: %w", err)
	}
	return p, nil
}

func (r *Repository) InsertReturn(ctx context.Context, p domain.PurchaseReturn) (domain.PurchaseReturn, error) {
	created, err := scanReturn(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.purchase_returns (id, return_number, receipt_id, supplier_id,
			branch_id, returned_on, reason_type, reason_note, status, total_idr)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+returnColumns,
		p.ID, p.ReturnNumber, p.ReceiptID, p.SupplierID, p.BranchID, requiredDate(p.ReturnedOn),
		p.ReasonType, p.ReasonNote, p.Status, p.TotalIDR))
	if database.IsUniqueViolation(err) {
		return domain.PurchaseReturn{}, httpx.Conflict("DUPLICATE", "That return number is taken.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.PurchaseReturn{}, httpx.NotFound("goods receipt")
	}
	if err != nil {
		return domain.PurchaseReturn{}, fmt.Errorf("purchasing: inserting return: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveReturn(ctx context.Context, p domain.PurchaseReturn) (domain.PurchaseReturn, error) {
	saved, err := scanReturn(r.db.QueryRow(ctx, `
		UPDATE purchasing.purchase_returns SET status = $2, reason_note = $3, total_idr = $4,
			posted_at = $5, submitted_at = $6, approved_by = $7, approved_at = $8,
			rejected_by = $9, rejected_at = $10, decision_note = $11, updated_at = now()
		WHERE id = $1 RETURNING `+returnColumns,
		p.ID, p.Status, p.ReasonNote, p.TotalIDR, p.PostedAt, p.SubmittedAt,
		p.ApprovedBy, p.ApprovedAt, p.RejectedBy, p.RejectedAt, p.DecisionNote))
	if database.IsNoRows(err) {
		return domain.PurchaseReturn{}, httpx.NotFound("purchase return")
	}
	if database.IsCheckViolation(err) {
		return domain.PurchaseReturn{}, httpx.Invalid("Rejecting a return needs a reason.")
	}
	if err != nil {
		return domain.PurchaseReturn{}, fmt.Errorf("purchasing: saving return: %w", err)
	}
	return saved, nil
}

const returnItemColumns = `id, return_id, receipt_item_id, item_id, qty, unit_price_idr, note`

func scanReturnItem(row pgx.Row) (domain.PurchaseReturnItem, error) {
	var i domain.PurchaseReturnItem
	err := row.Scan(&i.ID, &i.ReturnID, &i.ReceiptItemID, &i.ItemID, &i.Qty, &i.UnitPriceIDR, &i.Note)
	return i, err
}

func (r *Repository) ReturnItems(ctx context.Context, returnID string) ([]domain.PurchaseReturnItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+returnItemColumns+` FROM purchasing.purchase_return_items WHERE return_id = $1 ORDER BY id`,
		returnID)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing return items: %w", err)
	}
	defer rows.Close()

	out := []domain.PurchaseReturnItem{}
	for rows.Next() {
		i, err := scanReturnItem(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning return item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *Repository) InsertReturnItem(ctx context.Context, i domain.PurchaseReturnItem) (domain.PurchaseReturnItem, error) {
	created, err := scanReturnItem(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.purchase_return_items (id, return_id, receipt_item_id, item_id,
			qty, unit_price_idr, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+returnItemColumns,
		i.ID, i.ReturnID, i.ReceiptItemID, i.ItemID, i.Qty, i.UnitPriceIDR, i.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.PurchaseReturnItem{}, httpx.NotFound("purchase return or receipt line")
	}
	if err != nil {
		return domain.PurchaseReturnItem{}, fmt.Errorf("purchasing: inserting return item: %w", err)
	}
	return created, nil
}

// ── Summaries ────────────────────────────────────────────────────────────────

// Summary backs the purchasing dashboard.
type Summary struct {
	PendingRequests  int     `json:"pendingRequests"`
	OpenOrders       int     `json:"openOrders"`
	AwaitingDelivery int     `json:"awaitingDelivery"`
	Suppliers        int     `json:"suppliers"`
	CommittedIDR     float64 `json:"committedIdr"`
	ReceivedMonthIDR float64 `json:"receivedMonthIdr"`
}

func (r *Repository) Summary(ctx context.Context, branchID string, monthStart domain.Date) (Summary, error) {
	var s Summary
	branchFilter := ``
	args := []any{requiredDate(monthStart)}
	if branchID != "" {
		args = append(args, branchID)
		branchFilter = ` AND branch_id = $2`
	}

	err := r.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM purchasing.purchase_requests
			  WHERE status IN ('PENDING_HEAD', 'PENDING_FINANCE', 'PENDING_DIRECTOR')`+branchFilter+`),
			(SELECT count(*) FROM purchasing.purchase_orders
			  WHERE status IN ('DRAFT', 'APPROVED', 'SENT', 'PARTIALLY_RECEIVED')`+branchFilter+`),
			(SELECT count(*) FROM purchasing.purchase_orders
			  WHERE status IN ('SENT', 'PARTIALLY_RECEIVED')`+branchFilter+`),
			(SELECT count(*) FROM purchasing.suppliers WHERE status = 'ACTIVE'),
			(SELECT COALESCE(SUM(total_idr), 0) FROM purchasing.purchase_orders
			  WHERE status IN ('APPROVED', 'SENT', 'PARTIALLY_RECEIVED')`+branchFilter+`),
			(SELECT COALESCE(SUM(gi.qty_accepted * gi.unit_price_idr), 0)
			   FROM purchasing.goods_receipt_items gi
			   JOIN purchasing.goods_receipts g ON g.id = gi.receipt_id
			  WHERE g.status = 'POSTED' AND g.received_on >= $1`+
		func() string {
			if branchID != "" {
				return ` AND g.branch_id = $2`
			}
			return ``
		}()+`)`,
		args...).Scan(&s.PendingRequests, &s.OpenOrders, &s.AwaitingDelivery, &s.Suppliers,
		&s.CommittedIDR, &s.ReceivedMonthIDR)
	if err != nil {
		return Summary{}, fmt.Errorf("purchasing: summarizing: %w", err)
	}
	return s, nil
}
