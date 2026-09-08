package pos

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Repository persists the till. It reads and writes the `pos` schema only;
// stock, members and loyalty are reached through ports.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── Categories ───────────────────────────────────────────────────────────────

const categoryColumns = `id, name, sort_order, active, created_at, updated_at`

func scanCategory(row pgx.Row) (domain.POSCategory, error) {
	var c domain.POSCategory
	err := row.Scan(&c.ID, &c.Name, &c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *Repository) Categories(ctx context.Context) ([]domain.POSCategory, error) {
	rows, err := r.db.Query(ctx, `SELECT `+categoryColumns+` FROM pos.categories ORDER BY sort_order, name`)
	if err != nil {
		return nil, fmt.Errorf("pos: listing categories: %w", err)
	}
	defer rows.Close()

	out := []domain.POSCategory{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertCategory(ctx context.Context, c domain.POSCategory) (domain.POSCategory, error) {
	saved, err := scanCategory(r.db.QueryRow(ctx, `
		INSERT INTO pos.categories (id, name, sort_order, active)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name,
			sort_order = EXCLUDED.sort_order, active = EXCLUDED.active, updated_at = now()
		RETURNING `+categoryColumns,
		c.ID, c.Name, c.SortOrder, c.Active))
	if err != nil {
		return domain.POSCategory{}, fmt.Errorf("pos: saving category: %w", err)
	}
	return saved, nil
}

// ── Products ─────────────────────────────────────────────────────────────────

const productColumns = `id, sku, name, description, category_id, inventory_item_id, price_idr,
	cost_idr, tax_percent, barcode, pack_unit, pack_factor, bonus_xp, image_url, active,
	available, created_at, updated_at`

func scanProduct(row pgx.Row) (domain.POSProduct, error) {
	var p domain.POSProduct
	err := row.Scan(&p.ID, &p.SKU, &p.Name, &p.Description, &p.CategoryID, &p.InventoryItemID,
		&p.PriceIDR, &p.CostIDR, &p.TaxPercent, &p.Barcode, &p.PackUnit, &p.PackFactor,
		&p.BonusXP, &p.ImageURL, &p.Active, &p.Available, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// ProductFilter narrows the catalogue.
type ProductFilter struct {
	Query        string
	CategoryID   string
	SellableOnly bool
	Limit        int
}

func (r *Repository) Products(ctx context.Context, filter ProductFilter) ([]domain.POSProduct, error) {
	query := `SELECT ` + productColumns + ` FROM pos.products WHERE 1 = 1`
	args := []any{}

	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		query += fmt.Sprintf(` AND (lower(name) LIKE $%d OR lower(sku) LIKE $%d)`, len(args), len(args))
	}
	if filter.CategoryID != "" {
		args = append(args, filter.CategoryID)
		query += fmt.Sprintf(` AND category_id = $%d`, len(args))
	}
	if filter.SellableOnly {
		query += ` AND active AND available`
	}
	query += ` ORDER BY name`
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	args = append(args, limit)
	query += fmt.Sprintf(` LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing products: %w", err)
	}
	defer rows.Close()

	out := []domain.POSProduct{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning product: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) Product(ctx context.Context, id string) (domain.POSProduct, error) {
	p, err := scanProduct(r.db.QueryRow(ctx, `SELECT `+productColumns+` FROM pos.products WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.POSProduct{}, httpx.NotFound("product")
	}
	if err != nil {
		return domain.POSProduct{}, fmt.Errorf("pos: reading product: %w", err)
	}
	return p, nil
}

func (r *Repository) UpsertProduct(ctx context.Context, p domain.POSProduct) (domain.POSProduct, error) {
	saved, err := scanProduct(r.db.QueryRow(ctx, `
		INSERT INTO pos.products (id, sku, name, description, category_id, inventory_item_id,
			price_idr, cost_idr, tax_percent, barcode, pack_unit, pack_factor, bonus_xp,
			image_url, active, available)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (sku) DO UPDATE SET name = EXCLUDED.name,
			description = EXCLUDED.description, category_id = EXCLUDED.category_id,
			inventory_item_id = EXCLUDED.inventory_item_id, price_idr = EXCLUDED.price_idr,
			tax_percent = EXCLUDED.tax_percent, barcode = EXCLUDED.barcode,
			pack_unit = EXCLUDED.pack_unit, pack_factor = EXCLUDED.pack_factor,
			bonus_xp = EXCLUDED.bonus_xp,
			image_url = EXCLUDED.image_url, active = EXCLUDED.active,
			available = EXCLUDED.available, updated_at = now()
		RETURNING `+productColumns,
		p.ID, p.SKU, p.Name, p.Description, p.CategoryID, p.InventoryItemID, p.PriceIDR,
		p.CostIDR, p.TaxPercent, p.Barcode, p.PackUnit, p.PackFactor, p.BonusXP,
		p.ImageURL, p.Active, p.Available))
	if database.IsUniqueViolation(err) {
		return domain.POSProduct{}, httpx.Conflict("DUPLICATE_BARCODE",
			"That barcode already belongs to another product.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.POSProduct{}, httpx.NotFound("category")
	}
	if err != nil {
		return domain.POSProduct{}, fmt.Errorf("pos: saving product: %w", err)
	}
	return saved, nil
}

// SetProductCost writes the cost the till should freeze onto a sold line. It
// follows the inventory item's weighted average rather than being typed.
func (r *Repository) SetProductCost(ctx context.Context, productID string, cost float64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE pos.products SET cost_idr = $2, updated_at = now() WHERE id = $1`, productID, cost)
	if err != nil {
		return fmt.Errorf("pos: setting product cost: %w", err)
	}
	return nil
}

// ── Shifts ───────────────────────────────────────────────────────────────────

const shiftColumns = `id, shift_number, cashier_id, cashier_name, branch_id, status, opened_at,
	closed_at, opening_cash_idr, closing_cash_idr, expected_cash_idr, variance_idr, note,
	created_at, updated_at`

func scanShift(row pgx.Row) (domain.CashierShift, error) {
	var s domain.CashierShift
	err := row.Scan(&s.ID, &s.ShiftNumber, &s.CashierID, &s.CashierName, &s.BranchID, &s.Status,
		&s.OpenedAt, &s.ClosedAt, &s.OpeningCashIDR, &s.ClosingCashIDR, &s.ExpectedCashIDR,
		&s.VarianceIDR, &s.Note, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *Repository) Shifts(ctx context.Context, branchID, cashierID, status string, limit int) ([]domain.CashierShift, error) {
	query := `SELECT ` + shiftColumns + ` FROM pos.shifts WHERE 1 = 1`
	args := []any{}
	if branchID != "" {
		args = append(args, branchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if cashierID != "" {
		args = append(args, cashierID)
		query += fmt.Sprintf(` AND cashier_id = $%d`, len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY opened_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing shifts: %w", err)
	}
	defer rows.Close()

	out := []domain.CashierShift{}
	for rows.Next() {
		s, err := scanShift(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning shift: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Shift(ctx context.Context, id string, forUpdate bool) (domain.CashierShift, error) {
	query := `SELECT ` + shiftColumns + ` FROM pos.shifts WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	s, err := scanShift(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.CashierShift{}, httpx.NotFound("shift")
	}
	if err != nil {
		return domain.CashierShift{}, fmt.Errorf("pos: reading shift: %w", err)
	}
	return s, nil
}

// OpenShiftFor finds a cashier's open till, if they have one.
func (r *Repository) OpenShiftFor(ctx context.Context, cashierID, branchID string) (domain.CashierShift, bool, error) {
	s, err := scanShift(r.db.QueryRow(ctx,
		`SELECT `+shiftColumns+` FROM pos.shifts
		 WHERE cashier_id = $1 AND branch_id = $2 AND status = 'OPEN'`, cashierID, branchID))
	if database.IsNoRows(err) {
		return domain.CashierShift{}, false, nil
	}
	if err != nil {
		return domain.CashierShift{}, false, fmt.Errorf("pos: reading open shift: %w", err)
	}
	return s, true, nil
}

func (r *Repository) InsertShift(ctx context.Context, s domain.CashierShift) (domain.CashierShift, error) {
	created, err := scanShift(r.db.QueryRow(ctx, `
		INSERT INTO pos.shifts (id, shift_number, cashier_id, cashier_name, branch_id, status,
			opening_cash_idr, expected_cash_idr, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8) RETURNING `+shiftColumns,
		s.ID, s.ShiftNumber, s.CashierID, s.CashierName, s.BranchID, s.Status,
		s.OpeningCashIDR, s.Note))
	if database.IsUniqueViolation(err, "shifts_one_open_idx") {
		return domain.CashierShift{}, httpx.Conflict("SHIFT_ALREADY_OPEN",
			"That cashier already has a till open at this branch.")
	}
	if database.IsUniqueViolation(err) {
		return domain.CashierShift{}, httpx.Conflict("DUPLICATE", "That shift number is taken.")
	}
	if err != nil {
		return domain.CashierShift{}, fmt.Errorf("pos: inserting shift: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveShift(ctx context.Context, s domain.CashierShift) (domain.CashierShift, error) {
	saved, err := scanShift(r.db.QueryRow(ctx, `
		UPDATE pos.shifts SET status = $2, closed_at = $3, closing_cash_idr = $4,
			expected_cash_idr = $5, note = $6, updated_at = now()
		WHERE id = $1 RETURNING `+shiftColumns,
		s.ID, s.Status, s.ClosedAt, s.ClosingCashIDR, s.ExpectedCashIDR, s.Note))
	if database.IsNoRows(err) {
		return domain.CashierShift{}, httpx.NotFound("shift")
	}
	if database.IsCheckViolation(err) {
		return domain.CashierShift{}, httpx.Invalid("Closing a till needs a counted amount.")
	}
	if err != nil {
		return domain.CashierShift{}, fmt.Errorf("pos: saving shift: %w", err)
	}
	return saved, nil
}

// ── Orders ───────────────────────────────────────────────────────────────────

const orderColumns = `id, order_number, branch_id, shift_id, cashier_id, cashier_name,
	member_id, order_type, status, payment_status, subtotal_idr, discount_idr,
	tier_discount_idr, discount_reason, promo_discount_idr, promo_code,
	tax_idr, total_idr, paid_idr, change_idr, cost_idr, gross_profit_idr,
	xp_earned, note, opened_at, completed_at, cancelled_at, voided_at, voided_by, void_reason,
	authorised_by, authorised_by_name, created_at, updated_at`

func scanOrder(row pgx.Row) (domain.POSOrder, error) {
	var o domain.POSOrder
	err := row.Scan(&o.ID, &o.OrderNumber, &o.BranchID, &o.ShiftID, &o.CashierID, &o.CashierName,
		&o.MemberID, &o.Channel, &o.Status, &o.PaymentStatus, &o.SubtotalIDR, &o.DiscountIDR,
		&o.TierDiscountIDR, &o.DiscountReason, &o.PromoDiscountIDR, &o.PromoCode,
		&o.TaxIDR, &o.TotalIDR, &o.PaidIDR, &o.ChangeIDR,
		&o.CostIDR, &o.GrossProfitIDR, &o.XPEarned, &o.Note, &o.OpenedAt, &o.CompletedAt,
		&o.CancelledAt, &o.VoidedAt, &o.VoidedBy, &o.VoidReason,
		&o.AuthorisedBy, &o.AuthorisedByName, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

// OrderFilter narrows the sales list.
type OrderFilter struct {
	BranchID string
	ShiftID  string
	MemberID string
	Status   string
	From     *time.Time
	To       *time.Time
	Limit    int
}

func (r *Repository) Orders(ctx context.Context, filter OrderFilter) ([]domain.POSOrder, error) {
	query := `SELECT ` + orderColumns + ` FROM pos.orders WHERE 1 = 1`
	args := []any{}

	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.ShiftID != "" {
		args = append(args, filter.ShiftID)
		query += fmt.Sprintf(` AND shift_id = $%d`, len(args))
	}
	if filter.MemberID != "" {
		args = append(args, filter.MemberID)
		query += fmt.Sprintf(` AND member_id = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		query += fmt.Sprintf(` AND opened_at >= $%d`, len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		query += fmt.Sprintf(` AND opened_at <= $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY opened_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing orders: %w", err)
	}
	defer rows.Close()

	out := []domain.POSOrder{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning order: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repository) Order(ctx context.Context, id string, forUpdate bool) (domain.POSOrder, error) {
	query := `SELECT ` + orderColumns + ` FROM pos.orders WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	o, err := scanOrder(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.POSOrder{}, httpx.NotFound("order")
	}
	if err != nil {
		return domain.POSOrder{}, fmt.Errorf("pos: reading order: %w", err)
	}
	return o, nil
}

func (r *Repository) InsertOrder(ctx context.Context, o domain.POSOrder) (domain.POSOrder, error) {
	created, err := scanOrder(r.db.QueryRow(ctx, `
		INSERT INTO pos.orders (id, order_number, branch_id, shift_id, cashier_id, cashier_name,
			member_id, order_type, status, payment_status, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING `+orderColumns,
		o.ID, o.OrderNumber, o.BranchID, o.ShiftID, o.CashierID, o.CashierName, o.MemberID,
		o.Channel, o.Status, o.PaymentStatus, o.Note))
	if database.IsUniqueViolation(err) {
		return domain.POSOrder{}, httpx.Conflict("DUPLICATE", "That order number is taken.")
	}
	if err != nil {
		return domain.POSOrder{}, fmt.Errorf("pos: inserting order: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveOrder(ctx context.Context, o domain.POSOrder) (domain.POSOrder, error) {
	saved, err := scanOrder(r.db.QueryRow(ctx, `
		UPDATE pos.orders SET member_id = $2, status = $3, payment_status = $4,
			subtotal_idr = $5, discount_idr = $6, tier_discount_idr = $7, discount_reason = $8,
			tax_idr = $9, total_idr = $10, paid_idr = $11,
			change_idr = $12, cost_idr = $13, gross_profit_idr = $14, xp_earned = $15,
			note = $16, completed_at = $17, cancelled_at = $18, voided_at = $19, voided_by = $20,
			void_reason = $21, promo_discount_idr = $22, promo_code = $23,
			authorised_by = $24, authorised_by_name = $25, updated_at = now()
		WHERE id = $1 RETURNING `+orderColumns,
		o.ID, o.MemberID, o.Status, o.PaymentStatus, o.SubtotalIDR, o.DiscountIDR,
		o.TierDiscountIDR, o.DiscountReason, o.TaxIDR, o.TotalIDR,
		o.PaidIDR, o.ChangeIDR, o.CostIDR, o.GrossProfitIDR, o.XPEarned, o.Note,
		o.CompletedAt, o.CancelledAt, o.VoidedAt, o.VoidedBy, o.VoidReason,
		o.PromoDiscountIDR, o.PromoCode, o.AuthorisedBy, o.AuthorisedByName))
	if database.IsNoRows(err) {
		return domain.POSOrder{}, httpx.NotFound("order")
	}
	if database.IsCheckViolation(err) {
		return domain.POSOrder{}, httpx.Invalid("Voiding a sale needs a reason.")
	}
	if err != nil {
		return domain.POSOrder{}, fmt.Errorf("pos: saving order: %w", err)
	}
	return saved, nil
}

const orderItemColumns = `id, order_id, product_id, product_name, product_sku,
	inventory_item_id, qty, pack_unit, pack_factor, qty_base, unit_price_idr, discount_idr,
	tax_percent, line_total_idr, unit_cost_idr, note, created_at`

func scanOrderItem(row pgx.Row) (domain.POSOrderItem, error) {
	var i domain.POSOrderItem
	err := row.Scan(&i.ID, &i.OrderID, &i.ProductID, &i.ProductName, &i.ProductSKU,
		&i.InventoryItemID, &i.Qty, &i.PackUnit, &i.PackFactor, &i.QtyBase, &i.UnitPriceIDR,
		&i.DiscountIDR, &i.TaxPercent, &i.LineTotalIDR, &i.UnitCostIDR, &i.Note, &i.CreatedAt)
	return i, err
}

func (r *Repository) OrderItems(ctx context.Context, orderID string) ([]domain.POSOrderItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+orderItemColumns+` FROM pos.order_items WHERE order_id = $1 ORDER BY created_at, id`,
		orderID)
	if err != nil {
		return nil, fmt.Errorf("pos: listing order items: %w", err)
	}
	defer rows.Close()

	out := []domain.POSOrderItem{}
	for rows.Next() {
		i, err := scanOrderItem(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning order item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *Repository) InsertOrderItem(ctx context.Context, i domain.POSOrderItem) (domain.POSOrderItem, error) {
	created, err := scanOrderItem(r.db.QueryRow(ctx, `
		INSERT INTO pos.order_items (id, order_id, product_id, product_name, product_sku,
			inventory_item_id, qty, pack_unit, pack_factor, unit_price_idr, discount_idr,
			tax_percent, unit_cost_idr, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING `+orderItemColumns,
		i.ID, i.OrderID, i.ProductID, i.ProductName, i.ProductSKU, i.InventoryItemID, i.Qty,
		i.PackUnit, i.PackFactor, i.UnitPriceIDR, i.DiscountIDR, i.TaxPercent, i.UnitCostIDR, i.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.POSOrderItem{}, httpx.NotFound("order")
	}
	if err != nil {
		return domain.POSOrderItem{}, fmt.Errorf("pos: inserting order item: %w", err)
	}
	return created, nil
}

func (r *Repository) DeleteOrderItem(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM pos.order_items WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("pos: deleting order item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("order line")
	}
	return nil
}

// ── Payments ─────────────────────────────────────────────────────────────────

const paymentColumns = `id, order_id, method, amount_idr, change_idr, reference,
	gift_card_id, cashier_id, taken_at`

func scanPayment(row pgx.Row) (domain.POSPayment, error) {
	var p domain.POSPayment
	err := row.Scan(&p.ID, &p.OrderID, &p.Method, &p.AmountIDR, &p.ChangeIDR, &p.Reference,
		&p.GiftCardID, &p.CashierID, &p.TakenAt)
	return p, err
}

func (r *Repository) Payments(ctx context.Context, orderID string) ([]domain.POSPayment, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+paymentColumns+` FROM pos.payments WHERE order_id = $1 ORDER BY taken_at, id`,
		orderID)
	if err != nil {
		return nil, fmt.Errorf("pos: listing payments: %w", err)
	}
	defer rows.Close()

	out := []domain.POSPayment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning payment: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PaymentsForShift is every tender taken during one session, which is what the
// drawer count is worked out from.
func (r *Repository) PaymentsForShift(ctx context.Context, shiftID string) ([]domain.POSPayment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+prefixed(paymentColumns, "p")+`
		FROM pos.payments p
		JOIN pos.orders o ON o.id = p.order_id
		WHERE o.shift_id = $1 AND o.status = 'COMPLETED'
		ORDER BY p.taken_at`, shiftID)
	if err != nil {
		return nil, fmt.Errorf("pos: listing shift payments: %w", err)
	}
	defer rows.Close()

	out := []domain.POSPayment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning shift payment: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func prefixed(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for i, part := range parts {
		parts[i] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func (r *Repository) InsertPayment(ctx context.Context, p domain.POSPayment) (domain.POSPayment, error) {
	created, err := scanPayment(r.db.QueryRow(ctx, `
		INSERT INTO pos.payments (id, order_id, method, amount_idr, change_idr, reference, cashier_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+paymentColumns,
		p.ID, p.OrderID, p.Method, p.AmountIDR, p.ChangeIDR, p.Reference, p.CashierID))
	if database.IsCheckViolation(err) {
		return domain.POSPayment{}, httpx.Invalid("Only a cash tender gives change.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.POSPayment{}, httpx.NotFound("order")
	}
	if err != nil {
		return domain.POSPayment{}, fmt.Errorf("pos: inserting payment: %w", err)
	}
	return created, nil
}

// SetChange records the change given on a cash tender once the order settles.
func (r *Repository) SetChange(ctx context.Context, paymentID string, change float64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE pos.payments SET change_idr = $2 WHERE id = $1`, paymentID, change)
	if err != nil {
		return fmt.Errorf("pos: setting change: %w", err)
	}
	return nil
}

// ── Reporting ────────────────────────────────────────────────────────────────

// SalesSummary is what the counter took.
type SalesSummary struct {
	Orders         int     `json:"orders"`
	SalesIDR       float64 `json:"salesIdr"`
	CostIDR        float64 `json:"costIdr"`
	GrossProfitIDR float64 `json:"grossProfitIdr"`
	DiscountIDR    float64 `json:"discountIdr"`
	VoidedOrders   int     `json:"voidedOrders"`
	OpenOrders     int     `json:"openOrders"`
	XPAwarded      int     `json:"xpAwarded"`
}

func (r *Repository) SalesSummary(ctx context.Context, branchID string, from time.Time) (SalesSummary, error) {
	query := `
		SELECT count(*) FILTER (WHERE status = 'COMPLETED'),
			COALESCE(SUM(total_idr) FILTER (WHERE status = 'COMPLETED'), 0),
			COALESCE(SUM(cost_idr) FILTER (WHERE status = 'COMPLETED'), 0),
			COALESCE(SUM(gross_profit_idr) FILTER (WHERE status = 'COMPLETED'), 0),
			COALESCE(SUM(discount_idr) FILTER (WHERE status = 'COMPLETED'), 0),
			count(*) FILTER (WHERE status = 'VOIDED'),
			count(*) FILTER (WHERE status = 'OPEN'),
			COALESCE(SUM(xp_earned) FILTER (WHERE status = 'COMPLETED'), 0)
		FROM pos.orders WHERE opened_at >= $1`
	args := []any{from}
	if branchID != "" {
		args = append(args, branchID)
		query += ` AND branch_id = $2`
	}

	var s SalesSummary
	if err := r.db.QueryRow(ctx, query, args...).Scan(&s.Orders, &s.SalesIDR, &s.CostIDR,
		&s.GrossProfitIDR, &s.DiscountIDR, &s.VoidedOrders, &s.OpenOrders, &s.XPAwarded); err != nil {
		return SalesSummary{}, fmt.Errorf("pos: summarizing sales: %w", err)
	}
	return s, nil
}

// TopProducts is what actually sells.
type TopProduct struct {
	ProductID   string          `json:"productId"`
	ProductName string          `json:"productName"`
	Qty         domain.Quantity `json:"qty"`
	SalesIDR    float64         `json:"salesIdr"`
}

func (r *Repository) TopProducts(ctx context.Context, branchID string, from time.Time, limit int) ([]TopProduct, error) {
	query := `
		SELECT i.product_id, i.product_name, SUM(i.qty), SUM(i.line_total_idr)
		FROM pos.order_items i
		JOIN pos.orders o ON o.id = i.order_id
		WHERE o.status = 'COMPLETED' AND o.opened_at >= $1`
	args := []any{from}
	if branchID != "" {
		args = append(args, branchID)
		query += ` AND o.branch_id = $2`
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	args = append(args, limit)
	query += fmt.Sprintf(` GROUP BY i.product_id, i.product_name
		ORDER BY SUM(i.line_total_idr) DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing top products: %w", err)
	}
	defer rows.Close()

	out := []TopProduct{}
	for rows.Next() {
		var p TopProduct
		if err := rows.Scan(&p.ProductID, &p.ProductName, &p.Qty, &p.SalesIDR); err != nil {
			return nil, fmt.Errorf("pos: scanning top product: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── Barcode and channel pricing ──────────────────────────────────────────────

// ProductByBarcode resolves a scan.
//
// The barcode is on the product rather than the item because that is what
// distinguishes a single from a six-pack of the same drink, and a till that
// resolved a scan to the item would have to ask which — at which point it is
// not a scan any more.
func (r *Repository) ProductByBarcode(ctx context.Context, barcode string) (domain.POSProduct, error) {
	p, err := scanProduct(r.db.QueryRow(ctx,
		`SELECT `+productColumns+` FROM pos.products WHERE barcode = $1`, barcode))
	if database.IsNoRows(err) {
		return domain.POSProduct{}, httpx.NotFound("barcode")
	}
	if err != nil {
		return domain.POSProduct{}, fmt.Errorf("pos: resolving barcode: %w", err)
	}
	return p, nil
}

const productPriceColumns = `id, product_id, channel, min_qty, price_idr, active`

func scanProductPrice(row pgx.Row) (domain.ProductPrice, error) {
	var p domain.ProductPrice
	err := row.Scan(&p.ID, &p.ProductID, &p.Channel, &p.MinQty, &p.PriceIDR, &p.Active)
	return p, err
}

// ProductPrices is every break defined for one product, in every channel. The
// domain picks between them; this only fetches.
func (r *Repository) ProductPrices(ctx context.Context, productID string) ([]domain.ProductPrice, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+productPriceColumns+` FROM pos.product_prices
		 WHERE product_id = $1 ORDER BY channel, min_qty`, productID)
	if err != nil {
		return nil, fmt.Errorf("pos: listing product prices: %w", err)
	}
	defer rows.Close()

	out := []domain.ProductPrice{}
	for rows.Next() {
		p, err := scanProductPrice(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning product price: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertProductPrice(ctx context.Context, p domain.ProductPrice) (domain.ProductPrice, error) {
	saved, err := scanProductPrice(r.db.QueryRow(ctx, `
		INSERT INTO pos.product_prices (id, product_id, channel, min_qty, price_idr, active)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (product_id, channel, min_qty)
		DO UPDATE SET price_idr = EXCLUDED.price_idr, active = EXCLUDED.active, updated_at = now()
		RETURNING `+productPriceColumns,
		p.ID, p.ProductID, p.Channel, p.MinQty, p.PriceIDR, p.Active))
	if database.IsForeignKeyViolation(err) {
		return domain.ProductPrice{}, httpx.NotFound("product")
	}
	if err != nil {
		return domain.ProductPrice{}, fmt.Errorf("pos: saving product price: %w", err)
	}
	return saved, nil
}

func (r *Repository) DeleteProductPrice(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM pos.product_prices WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("pos: deleting product price: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("price")
	}
	return nil
}
