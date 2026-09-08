package purchasing

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Deliveries, instalments, payments and credit notes.

// ── Deliveries ───────────────────────────────────────────────────────────────

const deliveryColumns = `id, delivery_number, order_id, supplier_id, branch_id,
	delivery_note_number, driver_name, vehicle, arrived_on, arrived_at,
	received_by, received_by_name, status, note, created_at, updated_at`

func scanDelivery(row pgx.Row) (domain.Delivery, error) {
	var d domain.Delivery
	var arrivedOn time.Time
	err := row.Scan(&d.ID, &d.DeliveryNumber, &d.OrderID, &d.SupplierID, &d.BranchID,
		&d.DeliveryNoteNumber, &d.DriverName, &d.Vehicle, &arrivedOn, &d.ArrivedAt,
		&d.ReceivedBy, &d.ReceivedByName, &d.Status, &d.Note, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return domain.Delivery{}, err
	}
	d.ArrivedOn = scanDate(&arrivedOn)
	return d, nil
}

// DeliveryFilter narrows the receiving bay.
type DeliveryFilter struct {
	OrderID    string
	SupplierID string
	BranchID   string
	Status     string
	Limit      int
}

func (r *Repository) Deliveries(ctx context.Context, filter DeliveryFilter) ([]domain.Delivery, error) {
	query := `SELECT ` + deliveryColumns + ` FROM purchasing.deliveries WHERE 1 = 1`
	args := []any{}

	for _, clause := range []struct {
		column string
		value  string
	}{
		{"order_id", filter.OrderID},
		{"supplier_id", filter.SupplierID},
		{"branch_id", filter.BranchID},
		{"status", filter.Status},
	} {
		if clause.value == "" {
			continue
		}
		args = append(args, clause.value)
		query += fmt.Sprintf(" AND %s = $%d", clause.column, len(args))
	}
	query += ` ORDER BY arrived_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing deliveries: %w", err)
	}
	defer rows.Close()

	out := []domain.Delivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repository) Delivery(ctx context.Context, id string, forUpdate bool) (domain.Delivery, error) {
	query := `SELECT ` + deliveryColumns + ` FROM purchasing.deliveries WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	d, err := scanDelivery(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.Delivery{}, httpx.NotFound("delivery")
	}
	if err != nil {
		return domain.Delivery{}, fmt.Errorf("purchasing: reading delivery: %w", err)
	}
	return d, nil
}

func (r *Repository) InsertDelivery(ctx context.Context, d domain.Delivery) (domain.Delivery, error) {
	created, err := scanDelivery(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.deliveries (id, delivery_number, order_id, supplier_id, branch_id,
			delivery_note_number, driver_name, vehicle, arrived_on, received_by,
			received_by_name, status, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+deliveryColumns,
		d.ID, d.DeliveryNumber, d.OrderID, d.SupplierID, d.BranchID, d.DeliveryNoteNumber,
		d.DriverName, d.Vehicle, requiredDate(d.ArrivedOn), d.ReceivedBy, d.ReceivedByName,
		d.Status, d.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.Delivery{}, httpx.NotFound("purchase order or supplier")
	}
	if err != nil {
		return domain.Delivery{}, fmt.Errorf("purchasing: inserting delivery: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveDelivery(ctx context.Context, d domain.Delivery) (domain.Delivery, error) {
	saved, err := scanDelivery(r.db.QueryRow(ctx, `
		UPDATE purchasing.deliveries SET delivery_note_number = $2, driver_name = $3,
			vehicle = $4, status = $5, note = $6, updated_at = now()
		WHERE id = $1 RETURNING `+deliveryColumns,
		d.ID, d.DeliveryNoteNumber, d.DriverName, d.Vehicle, d.Status, d.Note))
	if database.IsNoRows(err) {
		return domain.Delivery{}, httpx.NotFound("delivery")
	}
	if err != nil {
		return domain.Delivery{}, fmt.Errorf("purchasing: saving delivery: %w", err)
	}
	return saved, nil
}

const deliveryItemColumns = `id, delivery_id, order_item_id, item_id, qty_delivered, unit,
	pack_factor, batch_number, expires_on, note`

func scanDeliveryItem(row pgx.Row) (domain.DeliveryItem, error) {
	var i domain.DeliveryItem
	var expiresOn *time.Time
	err := row.Scan(&i.ID, &i.DeliveryID, &i.OrderItemID, &i.ItemID, &i.QtyDelivered,
		&i.Unit, &i.PackFactor, &i.BatchNumber, &expiresOn, &i.Note)
	if err != nil {
		return domain.DeliveryItem{}, err
	}
	i.ExpiresOn = optionalDate(expiresOn)
	return i, nil
}

func (r *Repository) DeliveryItems(ctx context.Context, deliveryID string) ([]domain.DeliveryItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+deliveryItemColumns+` FROM purchasing.delivery_items
		 WHERE delivery_id = $1 ORDER BY id`, deliveryID)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing delivery items: %w", err)
	}
	defer rows.Close()

	out := []domain.DeliveryItem{}
	for rows.Next() {
		i, err := scanDeliveryItem(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning delivery item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *Repository) InsertDeliveryItem(ctx context.Context, i domain.DeliveryItem) (domain.DeliveryItem, error) {
	created, err := scanDeliveryItem(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.delivery_items (id, delivery_id, order_item_id, item_id,
			qty_delivered, unit, pack_factor, batch_number, expires_on, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+deliveryItemColumns,
		i.ID, i.DeliveryID, i.OrderItemID, i.ItemID, i.QtyDelivered, i.Unit, i.PackFactor,
		i.BatchNumber, dateValue(i.ExpiresOn), i.Note))
	if database.IsUniqueViolation(err) {
		return domain.DeliveryItem{}, httpx.Conflict("DUPLICATE_LINE",
			"That order line is already on this delivery. Change its quantity instead.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.DeliveryItem{}, httpx.NotFound("delivery or order line")
	}
	if err != nil {
		return domain.DeliveryItem{}, fmt.Errorf("purchasing: inserting delivery item: %w", err)
	}
	return created, nil
}

// DeliveredOnOrderLine is how much of one order line has already come off a
// truck, across every delivery against it.
func (r *Repository) DeliveredOnOrderLine(ctx context.Context, orderItemID string) (domain.Quantity, error) {
	var qty domain.Quantity
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(sum(i.qty_delivered), 0)
		FROM purchasing.delivery_items i
		JOIN purchasing.deliveries d ON d.id = i.delivery_id
		WHERE i.order_item_id = $1 AND d.status <> 'CANCELLED'`, orderItemID).Scan(&qty)
	if err != nil {
		return 0, fmt.Errorf("purchasing: summing deliveries on a line: %w", err)
	}
	return qty, nil
}

// ── Instalments ──────────────────────────────────────────────────────────────

const termColumns = `id, order_id, sequence, label, due_on, percent, amount_idr,
	status, paid_idr, note`

func scanTerm(row pgx.Row) (domain.PaymentTerm, error) {
	var t domain.PaymentTerm
	var dueOn time.Time
	err := row.Scan(&t.ID, &t.OrderID, &t.Sequence, &t.Label, &dueOn, &t.Percent,
		&t.AmountIDR, &t.Status, &t.PaidIDR, &t.Note)
	if err != nil {
		return domain.PaymentTerm{}, err
	}
	t.DueOn = scanDate(&dueOn)
	return t, nil
}

func (r *Repository) PaymentTerms(ctx context.Context, orderID string) ([]domain.PaymentTerm, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+termColumns+` FROM purchasing.payment_terms
		 WHERE order_id = $1 ORDER BY sequence`, orderID)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing payment terms: %w", err)
	}
	defer rows.Close()

	out := []domain.PaymentTerm{}
	for rows.Next() {
		t, err := scanTerm(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning payment term: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) Term(ctx context.Context, id string, forUpdate bool) (domain.PaymentTerm, error) {
	query := `SELECT ` + termColumns + ` FROM purchasing.payment_terms WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	t, err := scanTerm(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.PaymentTerm{}, httpx.NotFound("instalment")
	}
	if err != nil {
		return domain.PaymentTerm{}, fmt.Errorf("purchasing: reading payment term: %w", err)
	}
	return t, nil
}

func (r *Repository) UpsertTerm(ctx context.Context, t domain.PaymentTerm) (domain.PaymentTerm, error) {
	saved, err := scanTerm(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.payment_terms (id, order_id, sequence, label, due_on,
			percent, amount_idr, status, paid_idr, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (order_id, sequence) DO UPDATE SET label = EXCLUDED.label,
			due_on = EXCLUDED.due_on, percent = EXCLUDED.percent,
			amount_idr = EXCLUDED.amount_idr, status = EXCLUDED.status,
			paid_idr = EXCLUDED.paid_idr, note = EXCLUDED.note, updated_at = now()
		RETURNING `+termColumns,
		t.ID, t.OrderID, t.Sequence, t.Label, requiredDate(t.DueOn), t.Percent,
		t.AmountIDR, t.Status, t.PaidIDR, t.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.PaymentTerm{}, httpx.NotFound("purchase order")
	}
	if err != nil {
		return domain.PaymentTerm{}, fmt.Errorf("purchasing: saving payment term: %w", err)
	}
	return saved, nil
}

func (r *Repository) DeleteTerms(ctx context.Context, orderID string) error {
	// Only unpaid instalments go: money that has moved against one is a fact,
	// and rebuilding a schedule must not erase it.
	_, err := r.db.Exec(ctx,
		`DELETE FROM purchasing.payment_terms WHERE order_id = $1 AND paid_idr = 0`, orderID)
	if err != nil {
		return fmt.Errorf("purchasing: clearing payment terms: %w", err)
	}
	return nil
}

// ── Payments ─────────────────────────────────────────────────────────────────

const vendorPaymentColumns = `id, payment_number, supplier_id, order_id, term_id, paid_on,
	amount_idr, credit_idr, method, reference, status, note, posted_at, posted_by,
	voided_at, void_reason, created_at, updated_at`

// VendorPayment is money that actually left.
type VendorPayment struct {
	ID            string      `json:"id"`
	PaymentNumber string      `json:"paymentNumber"`
	SupplierID    string      `json:"supplierId"`
	OrderID       *string     `json:"orderId"`
	TermID        *string     `json:"termId"`
	PaidOn        domain.Date `json:"paidOn"`
	AmountIDR     float64     `json:"amountIdr"`
	// CreditIDR is the part settled with a credit note rather than cash.
	CreditIDR  float64    `json:"creditIdr"`
	Method     string     `json:"method"`
	Reference  *string    `json:"reference"`
	Status     string     `json:"status"`
	Note       *string    `json:"note"`
	PostedAt   *time.Time `json:"postedAt"`
	PostedBy   *string    `json:"postedBy"`
	VoidedAt   *time.Time `json:"voidedAt"`
	VoidReason *string    `json:"voidReason"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// CashIDR is what actually has to leave the bank.
func (p VendorPayment) CashIDR() float64 { return p.AmountIDR - p.CreditIDR }

func scanVendorPayment(row pgx.Row) (VendorPayment, error) {
	var p VendorPayment
	var paidOn time.Time
	err := row.Scan(&p.ID, &p.PaymentNumber, &p.SupplierID, &p.OrderID, &p.TermID, &paidOn,
		&p.AmountIDR, &p.CreditIDR, &p.Method, &p.Reference, &p.Status, &p.Note,
		&p.PostedAt, &p.PostedBy, &p.VoidedAt, &p.VoidReason, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return VendorPayment{}, err
	}
	p.PaidOn = scanDate(&paidOn)
	return p, nil
}

// PaymentFilter narrows the payment run.
type PaymentFilter struct {
	SupplierID string
	OrderID    string
	Status     string
	Limit      int
}

func (r *Repository) VendorPayments(ctx context.Context, filter PaymentFilter) ([]VendorPayment, error) {
	query := `SELECT ` + vendorPaymentColumns + ` FROM purchasing.vendor_payments WHERE 1 = 1`
	args := []any{}
	for _, clause := range []struct{ column, value string }{
		{"supplier_id", filter.SupplierID},
		{"order_id", filter.OrderID},
		{"status", filter.Status},
	} {
		if clause.value == "" {
			continue
		}
		args = append(args, clause.value)
		query += fmt.Sprintf(" AND %s = $%d", clause.column, len(args))
	}
	query += ` ORDER BY paid_on DESC, created_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing vendor payments: %w", err)
	}
	defer rows.Close()

	out := []VendorPayment{}
	for rows.Next() {
		p, err := scanVendorPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning vendor payment: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) VendorPayment(ctx context.Context, id string, forUpdate bool) (VendorPayment, error) {
	query := `SELECT ` + vendorPaymentColumns + ` FROM purchasing.vendor_payments WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	p, err := scanVendorPayment(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return VendorPayment{}, httpx.NotFound("payment")
	}
	if err != nil {
		return VendorPayment{}, fmt.Errorf("purchasing: reading vendor payment: %w", err)
	}
	return p, nil
}

func (r *Repository) InsertVendorPayment(ctx context.Context, p VendorPayment) (VendorPayment, error) {
	created, err := scanVendorPayment(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.vendor_payments (id, payment_number, supplier_id, order_id,
			term_id, paid_on, amount_idr, credit_idr, method, reference, status, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+vendorPaymentColumns,
		p.ID, p.PaymentNumber, p.SupplierID, p.OrderID, p.TermID, requiredDate(p.PaidOn),
		p.AmountIDR, p.CreditIDR, p.Method, p.Reference, p.Status, p.Note))
	if database.IsForeignKeyViolation(err) {
		return VendorPayment{}, httpx.NotFound("supplier, order or instalment")
	}
	if database.IsCheckViolation(err) {
		return VendorPayment{}, httpx.Invalid("A payment cannot be settled by more credit than it is worth.")
	}
	if err != nil {
		return VendorPayment{}, fmt.Errorf("purchasing: inserting vendor payment: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveVendorPayment(ctx context.Context, p VendorPayment) (VendorPayment, error) {
	saved, err := scanVendorPayment(r.db.QueryRow(ctx, `
		UPDATE purchasing.vendor_payments SET amount_idr = $2, credit_idr = $3, method = $4,
			reference = $5, status = $6, note = $7, posted_at = $8, posted_by = $9,
			voided_at = $10, void_reason = $11, updated_at = now()
		WHERE id = $1 RETURNING `+vendorPaymentColumns,
		p.ID, p.AmountIDR, p.CreditIDR, p.Method, p.Reference, p.Status, p.Note,
		p.PostedAt, p.PostedBy, p.VoidedAt, p.VoidReason))
	if database.IsNoRows(err) {
		return VendorPayment{}, httpx.NotFound("payment")
	}
	if database.IsCheckViolation(err) {
		return VendorPayment{}, httpx.Invalid("Voiding a payment needs a reason.")
	}
	if err != nil {
		return VendorPayment{}, fmt.Errorf("purchasing: saving vendor payment: %w", err)
	}
	return saved, nil
}

// PaidOnOrder is what has actually been paid against an order, ignoring drafts
// and voided payments.
func (r *Repository) PaidOnOrder(ctx context.Context, orderID string) (float64, float64, error) {
	var paid, credited float64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_idr), 0), COALESCE(sum(credit_idr), 0)
		FROM purchasing.vendor_payments
		WHERE order_id = $1 AND status = 'POSTED'`, orderID).Scan(&paid, &credited)
	if err != nil {
		return 0, 0, fmt.Errorf("purchasing: summing payments on an order: %w", err)
	}
	return paid, credited, nil
}

// ── Credit notes ─────────────────────────────────────────────────────────────

const creditColumns = `id, credit_number, supplier_id, return_id, issued_on, amount_idr,
	applied_idr, status, reason, expires_on`

func scanCredit(row pgx.Row) (domain.VendorCredit, error) {
	var c domain.VendorCredit
	var issuedOn time.Time
	var expiresOn *time.Time
	err := row.Scan(&c.ID, &c.CreditNumber, &c.SupplierID, &c.ReturnID, &issuedOn,
		&c.AmountIDR, &c.AppliedIDR, &c.Status, &c.Reason, &expiresOn)
	if err != nil {
		return domain.VendorCredit{}, err
	}
	c.IssuedOn = scanDate(&issuedOn)
	c.ExpiresOn = optionalDate(expiresOn)
	return c, nil
}

// CreditFilter narrows the credit notes.
type CreditFilter struct {
	SupplierID string
	Status     string
	// OpenOnly is the list that can actually pay for something.
	OpenOnly bool
	Limit    int
}

func (r *Repository) VendorCredits(ctx context.Context, filter CreditFilter) ([]domain.VendorCredit, error) {
	query := `SELECT ` + creditColumns + ` FROM purchasing.vendor_credits WHERE 1 = 1`
	args := []any{}
	if filter.SupplierID != "" {
		args = append(args, filter.SupplierID)
		query += fmt.Sprintf(" AND supplier_id = $%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if filter.OpenOnly {
		query += " AND status IN ('OPEN', 'PARTIALLY_APPLIED')"
	}
	query += ` ORDER BY issued_on, created_at`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing credit notes: %w", err)
	}
	defer rows.Close()

	out := []domain.VendorCredit{}
	for rows.Next() {
		c, err := scanCredit(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning credit note: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreditsForSpending locks a supplier's usable credit notes, oldest first.
func (r *Repository) CreditsForSpending(ctx context.Context, supplierID string) ([]domain.VendorCredit, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+creditColumns+` FROM purchasing.vendor_credits
		 WHERE supplier_id = $1 AND status IN ('OPEN', 'PARTIALLY_APPLIED')
		 ORDER BY issued_on, created_at FOR UPDATE`, supplierID)
	if err != nil {
		return nil, fmt.Errorf("purchasing: locking credit notes: %w", err)
	}
	defer rows.Close()

	out := []domain.VendorCredit{}
	for rows.Next() {
		c, err := scanCredit(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning credit note: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) InsertCredit(ctx context.Context, c domain.VendorCredit) (domain.VendorCredit, error) {
	created, err := scanCredit(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.vendor_credits (id, credit_number, supplier_id, return_id,
			issued_on, amount_idr, applied_idr, status, reason, expires_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+creditColumns,
		c.ID, c.CreditNumber, c.SupplierID, c.ReturnID, requiredDate(c.IssuedOn),
		c.AmountIDR, c.AppliedIDR, c.Status, c.Reason, dateValue(c.ExpiresOn)))
	if database.IsForeignKeyViolation(err) {
		return domain.VendorCredit{}, httpx.NotFound("supplier or return")
	}
	if err != nil {
		return domain.VendorCredit{}, fmt.Errorf("purchasing: inserting credit note: %w", err)
	}
	return created, nil
}

// ApplyCredit spends part of a credit note and records where it went.
func (r *Repository) ApplyCredit(ctx context.Context, allocation domain.CreditAllocation,
	paymentID, id string, status domain.CreditStatus) error {

	tag, err := r.db.Exec(ctx, `
		UPDATE purchasing.vendor_credits
		SET applied_idr = applied_idr + $2, status = $3, updated_at = now()
		WHERE id = $1`, allocation.CreditID, allocation.AmountIDR, status)
	if database.IsCheckViolation(err) {
		return httpx.Conflict("CREDIT_EXHAUSTED", "That credit note does not have that much left on it.")
	}
	if err != nil {
		return fmt.Errorf("purchasing: applying credit note: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("credit note")
	}

	if _, err := r.db.Exec(ctx, `
		INSERT INTO purchasing.credit_applications (id, credit_id, payment_id, amount_idr)
		VALUES ($1, $2, $3, $4)`,
		id, allocation.CreditID, paymentID, allocation.AmountIDR); err != nil {
		return fmt.Errorf("purchasing: recording credit application: %w", err)
	}
	return nil
}

// SetReturnCredit points a return at the credit note it produced.
func (r *Repository) SetReturnCredit(ctx context.Context, returnID, creditID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE purchasing.purchase_returns SET credit_id = $2, updated_at = now() WHERE id = $1`,
		returnID, creditID)
	if err != nil {
		return fmt.Errorf("purchasing: linking return to credit note: %w", err)
	}
	return nil
}

// InspectedOnDelivery is how much of one delivery's line has since been
// accepted or rejected on a posted goods receipt.
//
// Receipt lines point at the order line rather than the delivery line, so the
// join goes through the receipt's own delivery reference — which is why a
// receipt raised straight off an order counts against no delivery at all.
func (r *Repository) InspectedOnDelivery(ctx context.Context, deliveryID, orderItemID string) (domain.Quantity, domain.Quantity, error) {
	var accepted, rejected domain.Quantity
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(sum(gi.qty_accepted), 0), COALESCE(sum(gi.qty_rejected), 0)
		FROM purchasing.goods_receipt_items gi
		JOIN purchasing.goods_receipts g ON g.id = gi.receipt_id
		WHERE g.delivery_id = $1 AND gi.order_item_id = $2 AND g.status <> 'CANCELLED'`,
		deliveryID, orderItemID).Scan(&accepted, &rejected)
	if err != nil {
		return 0, 0, fmt.Errorf("purchasing: summing inspection on a delivery: %w", err)
	}
	return accepted, rejected, nil
}
