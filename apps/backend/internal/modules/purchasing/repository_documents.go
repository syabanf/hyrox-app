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

// The documents: request, order, receipt, return.

// ── Purchase requests ────────────────────────────────────────────────────────

const requestColumns = `id, pr_number, branch_id, requester_id, requester_name, department_id,
	status, priority, total_idr, required_on, note,
	approved_by_head, approved_at_head, approved_by_finance, approved_at_finance,
	approved_by_director, approved_at_director,
	rejected_by, rejected_at, rejection_reason, converted_po_id, created_at, updated_at`

func scanRequest(row pgx.Row) (domain.PurchaseRequest, error) {
	var r domain.PurchaseRequest
	var requiredOn *time.Time
	err := row.Scan(&r.ID, &r.PRNumber, &r.BranchID, &r.RequesterID, &r.RequesterName,
		&r.DepartmentID, &r.Status, &r.Priority, &r.TotalIDR, &requiredOn, &r.Note,
		&r.ApprovedByHead, &r.ApprovedAtHead, &r.ApprovedByFinance, &r.ApprovedAtFinance,
		&r.ApprovedByDirector, &r.ApprovedAtDirector,
		&r.RejectedBy, &r.RejectedAt, &r.RejectionReason, &r.ConvertedPOID,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return domain.PurchaseRequest{}, err
	}
	r.RequiredOn = optionalDate(requiredOn)
	return r, nil
}

// RequestFilter narrows the request list.
type RequestFilter struct {
	BranchID string
	Status   string
	Priority string
	Limit    int
}

func (r *Repository) Requests(ctx context.Context, filter RequestFilter) ([]domain.PurchaseRequest, error) {
	query := `SELECT ` + requestColumns + ` FROM purchasing.purchase_requests WHERE 1 = 1`
	args := []any{}

	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.Priority != "" {
		args = append(args, filter.Priority)
		query += fmt.Sprintf(` AND priority = $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing requests: %w", err)
	}
	defer rows.Close()

	out := []domain.PurchaseRequest{}
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning request: %w", err)
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (r *Repository) Request(ctx context.Context, id string, forUpdate bool) (domain.PurchaseRequest, error) {
	query := `SELECT ` + requestColumns + ` FROM purchasing.purchase_requests WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	req, err := scanRequest(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.PurchaseRequest{}, httpx.NotFound("purchase request")
	}
	if err != nil {
		return domain.PurchaseRequest{}, fmt.Errorf("purchasing: reading request: %w", err)
	}
	return req, nil
}

func (r *Repository) InsertRequest(ctx context.Context, req domain.PurchaseRequest) (domain.PurchaseRequest, error) {
	created, err := scanRequest(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.purchase_requests (id, pr_number, branch_id, requester_id,
			requester_name, department_id, status, priority, total_idr, required_on, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING `+requestColumns,
		req.ID, req.PRNumber, req.BranchID, req.RequesterID, req.RequesterName,
		req.DepartmentID, req.Status, req.Priority, req.TotalIDR,
		dateValue(req.RequiredOn), req.Note))
	if database.IsUniqueViolation(err) {
		return domain.PurchaseRequest{}, httpx.Conflict("DUPLICATE", "That request number is taken.")
	}
	if err != nil {
		return domain.PurchaseRequest{}, fmt.Errorf("purchasing: inserting request: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveRequest(ctx context.Context, req domain.PurchaseRequest) (domain.PurchaseRequest, error) {
	saved, err := scanRequest(r.db.QueryRow(ctx, `
		UPDATE purchasing.purchase_requests SET status = $2, priority = $3, total_idr = $4,
			required_on = $5, note = $6,
			approved_by_head = $7, approved_at_head = $8,
			approved_by_finance = $9, approved_at_finance = $10,
			approved_by_director = $11, approved_at_director = $12,
			rejected_by = $13, rejected_at = $14, rejection_reason = $15,
			converted_po_id = $16, updated_at = now()
		WHERE id = $1 RETURNING `+requestColumns,
		req.ID, req.Status, req.Priority, req.TotalIDR, dateValue(req.RequiredOn), req.Note,
		req.ApprovedByHead, req.ApprovedAtHead, req.ApprovedByFinance, req.ApprovedAtFinance,
		req.ApprovedByDirector, req.ApprovedAtDirector,
		req.RejectedBy, req.RejectedAt, req.RejectionReason, req.ConvertedPOID))
	if database.IsNoRows(err) {
		return domain.PurchaseRequest{}, httpx.NotFound("purchase request")
	}
	if database.IsCheckViolation(err) {
		return domain.PurchaseRequest{}, httpx.Invalid("A rejection needs a reason.")
	}
	if err != nil {
		return domain.PurchaseRequest{}, fmt.Errorf("purchasing: saving request: %w", err)
	}
	return saved, nil
}

const requestItemColumns = `id, request_id, item_id, description, qty, unit,
	estimated_price_idr, total_idr, note`

func scanRequestItem(row pgx.Row) (domain.PurchaseRequestItem, error) {
	var i domain.PurchaseRequestItem
	err := row.Scan(&i.ID, &i.RequestID, &i.ItemID, &i.Description, &i.Qty, &i.Unit,
		&i.EstimatedPriceIDR, &i.TotalIDR, &i.Note)
	return i, err
}

func (r *Repository) RequestItems(ctx context.Context, requestID string) ([]domain.PurchaseRequestItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+requestItemColumns+` FROM purchasing.purchase_request_items WHERE request_id = $1 ORDER BY id`,
		requestID)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing request items: %w", err)
	}
	defer rows.Close()

	out := []domain.PurchaseRequestItem{}
	for rows.Next() {
		i, err := scanRequestItem(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning request item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *Repository) InsertRequestItem(ctx context.Context, i domain.PurchaseRequestItem) (domain.PurchaseRequestItem, error) {
	created, err := scanRequestItem(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.purchase_request_items (id, request_id, item_id, description,
			qty, unit, estimated_price_idr, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+requestItemColumns,
		i.ID, i.RequestID, i.ItemID, i.Description, i.Qty, i.Unit, i.EstimatedPriceIDR, i.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.PurchaseRequestItem{}, httpx.NotFound("purchase request")
	}
	if err != nil {
		return domain.PurchaseRequestItem{}, fmt.Errorf("purchasing: inserting request item: %w", err)
	}
	return created, nil
}

func (r *Repository) DeleteRequestItem(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM purchasing.purchase_request_items WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("purchasing: deleting request item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("request line")
	}
	return nil
}

// ── Purchase orders ──────────────────────────────────────────────────────────

const orderColumns = `id, po_number, supplier_id, branch_id, request_id, status, ordered_on,
	expected_on, subtotal_idr, discount_idr, tax_percent, tax_idr, total_idr, terms, ship_to,
	note, approved_by, approved_at, sent_at, cancelled_by, cancelled_at, cancellation_reason,
	created_at, updated_at`

func scanOrder(row pgx.Row) (domain.PurchaseOrder, error) {
	var o domain.PurchaseOrder
	var orderedOn time.Time
	var expectedOn *time.Time
	err := row.Scan(&o.ID, &o.PONumber, &o.SupplierID, &o.BranchID, &o.RequestID, &o.Status,
		&orderedOn, &expectedOn, &o.SubtotalIDR, &o.DiscountIDR, &o.TaxPercent, &o.TaxIDR,
		&o.TotalIDR, &o.Terms, &o.ShipTo, &o.Note, &o.ApprovedBy, &o.ApprovedAt, &o.SentAt,
		&o.CancelledBy, &o.CancelledAt, &o.CancellationReason, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return domain.PurchaseOrder{}, err
	}
	o.OrderedOn = scanDate(&orderedOn)
	o.ExpectedOn = optionalDate(expectedOn)
	return o, nil
}

// OrderFilter narrows the order list.
type OrderFilter struct {
	BranchID   string
	SupplierID string
	Status     string
	Limit      int
}

func (r *Repository) Orders(ctx context.Context, filter OrderFilter) ([]domain.PurchaseOrder, error) {
	query := `SELECT ` + orderColumns + ` FROM purchasing.purchase_orders WHERE 1 = 1`
	args := []any{}

	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.SupplierID != "" {
		args = append(args, filter.SupplierID)
		query += fmt.Sprintf(` AND supplier_id = $%d`, len(args))
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
	query += fmt.Sprintf(` ORDER BY ordered_on DESC, created_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing orders: %w", err)
	}
	defer rows.Close()

	out := []domain.PurchaseOrder{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning order: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repository) Order(ctx context.Context, id string, forUpdate bool) (domain.PurchaseOrder, error) {
	query := `SELECT ` + orderColumns + ` FROM purchasing.purchase_orders WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	o, err := scanOrder(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.PurchaseOrder{}, httpx.NotFound("purchase order")
	}
	if err != nil {
		return domain.PurchaseOrder{}, fmt.Errorf("purchasing: reading order: %w", err)
	}
	return o, nil
}

func (r *Repository) InsertOrder(ctx context.Context, o domain.PurchaseOrder) (domain.PurchaseOrder, error) {
	created, err := scanOrder(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.purchase_orders (id, po_number, supplier_id, branch_id, request_id,
			status, ordered_on, expected_on, subtotal_idr, discount_idr, tax_percent, tax_idr,
			total_idr, terms, ship_to, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING `+orderColumns,
		o.ID, o.PONumber, o.SupplierID, o.BranchID, o.RequestID, o.Status,
		requiredDate(o.OrderedOn), dateValue(o.ExpectedOn), o.SubtotalIDR, o.DiscountIDR,
		o.TaxPercent, o.TaxIDR, o.TotalIDR, o.Terms, o.ShipTo, o.Note))
	if database.IsUniqueViolation(err) {
		return domain.PurchaseOrder{}, httpx.Conflict("DUPLICATE", "That order number is taken.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.PurchaseOrder{}, httpx.NotFound("supplier")
	}
	if err != nil {
		return domain.PurchaseOrder{}, fmt.Errorf("purchasing: inserting order: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveOrder(ctx context.Context, o domain.PurchaseOrder) (domain.PurchaseOrder, error) {
	saved, err := scanOrder(r.db.QueryRow(ctx, `
		UPDATE purchasing.purchase_orders SET status = $2, expected_on = $3, subtotal_idr = $4,
			discount_idr = $5, tax_percent = $6, tax_idr = $7, total_idr = $8, terms = $9,
			ship_to = $10, note = $11, approved_by = $12, approved_at = $13, sent_at = $14,
			cancelled_by = $15, cancelled_at = $16, cancellation_reason = $17, updated_at = now()
		WHERE id = $1 RETURNING `+orderColumns,
		o.ID, o.Status, dateValue(o.ExpectedOn), o.SubtotalIDR, o.DiscountIDR, o.TaxPercent,
		o.TaxIDR, o.TotalIDR, o.Terms, o.ShipTo, o.Note, o.ApprovedBy, o.ApprovedAt, o.SentAt,
		o.CancelledBy, o.CancelledAt, o.CancellationReason))
	if database.IsNoRows(err) {
		return domain.PurchaseOrder{}, httpx.NotFound("purchase order")
	}
	if database.IsCheckViolation(err) {
		return domain.PurchaseOrder{}, httpx.Invalid("A cancellation needs a reason.")
	}
	if err != nil {
		return domain.PurchaseOrder{}, fmt.Errorf("purchasing: saving order: %w", err)
	}
	return saved, nil
}

const orderItemColumns = `id, order_id, item_id, description, qty_ordered, qty_received, unit,
	pack_factor, unit_price_idr, discount_idr, subtotal_idr, note`

func scanOrderItem(row pgx.Row) (domain.PurchaseOrderItem, error) {
	var i domain.PurchaseOrderItem
	err := row.Scan(&i.ID, &i.OrderID, &i.ItemID, &i.Description, &i.QtyOrdered, &i.QtyReceived,
		&i.Unit, &i.PackFactor, &i.UnitPriceIDR, &i.DiscountIDR, &i.SubtotalIDR, &i.Note)
	return i, err
}

func (r *Repository) OrderItems(ctx context.Context, orderID string, forUpdate bool) ([]domain.PurchaseOrderItem, error) {
	query := `SELECT ` + orderItemColumns + ` FROM purchasing.purchase_order_items WHERE order_id = $1 ORDER BY id`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	rows, err := r.db.Query(ctx, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing order items: %w", err)
	}
	defer rows.Close()

	out := []domain.PurchaseOrderItem{}
	for rows.Next() {
		i, err := scanOrderItem(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning order item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *Repository) OrderItem(ctx context.Context, id string, forUpdate bool) (domain.PurchaseOrderItem, error) {
	query := `SELECT ` + orderItemColumns + ` FROM purchasing.purchase_order_items WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	i, err := scanOrderItem(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.PurchaseOrderItem{}, httpx.NotFound("order line")
	}
	if err != nil {
		return domain.PurchaseOrderItem{}, fmt.Errorf("purchasing: reading order item: %w", err)
	}
	return i, nil
}

func (r *Repository) InsertOrderItem(ctx context.Context, i domain.PurchaseOrderItem) (domain.PurchaseOrderItem, error) {
	created, err := scanOrderItem(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.purchase_order_items (id, order_id, item_id, description,
			qty_ordered, unit, pack_factor, unit_price_idr, discount_idr, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+orderItemColumns,
		i.ID, i.OrderID, i.ItemID, i.Description, i.QtyOrdered, i.Unit, i.PackFactor,
		i.UnitPriceIDR, i.DiscountIDR, i.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.PurchaseOrderItem{}, httpx.NotFound("purchase order")
	}
	if err != nil {
		return domain.PurchaseOrderItem{}, fmt.Errorf("purchasing: inserting order item: %w", err)
	}
	return created, nil
}

func (r *Repository) DeleteOrderLine(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM purchasing.purchase_order_items WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.Conflict("IN_USE", "That line has already been received against.")
	}
	if err != nil {
		return fmt.Errorf("purchasing: deleting order item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("order line")
	}
	return nil
}

// AddReceivedQty books delivered stock against an order line. The CHECK on the
// column is what makes over-delivery impossible even under a race.
func (r *Repository) AddReceivedQty(ctx context.Context, orderItemID string, qty domain.Quantity) error {
	_, err := r.db.Exec(ctx,
		`UPDATE purchasing.purchase_order_items SET qty_received = qty_received + $2 WHERE id = $1`,
		orderItemID, qty)
	if database.IsCheckViolation(err) {
		return httpx.Conflict("OVER_DELIVERED", "That is more than was ordered on the line.")
	}
	if err != nil {
		return fmt.Errorf("purchasing: booking received quantity: %w", err)
	}
	return nil
}
