package purchasing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Repository persists suppliers, requests, orders, receipts and returns. It
// reads and writes the `purchasing` schema only: items and stock belong to
// inventory and are reached through a port.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// dateValue converts a calendar date for the driver, mapping the empty date to
// SQL NULL.
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

func requiredDate(d domain.Date) any { return dateValue(&d) }

func scanDate(t *time.Time) domain.Date {
	if t == nil {
		return ""
	}
	return domain.Date(t.Format("2006-01-02"))
}

func optionalDate(t *time.Time) *domain.Date {
	if t == nil {
		return nil
	}
	d := scanDate(t)
	return &d
}

// ── Suppliers ────────────────────────────────────────────────────────────────

const supplierColumns = `id, code, name, contact_name, contact_phone, email, address, city,
	tax_number, payment_terms, bank_name, bank_account, bank_holder, category, status, note,
	created_at, updated_at`

func scanSupplier(row pgx.Row) (domain.Supplier, error) {
	var s domain.Supplier
	err := row.Scan(&s.ID, &s.Code, &s.Name, &s.ContactName, &s.ContactPhone, &s.Email,
		&s.Address, &s.City, &s.TaxNumber, &s.PaymentTerms, &s.BankName, &s.BankAccount,
		&s.BankHolder, &s.Category, &s.Status, &s.Note, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// SupplierFilter narrows the supplier list.
type SupplierFilter struct {
	Query  string
	Status string
	Limit  int
}

func (r *Repository) Suppliers(ctx context.Context, filter SupplierFilter) ([]domain.Supplier, error) {
	query := `SELECT ` + supplierColumns + ` FROM purchasing.suppliers WHERE 1 = 1`
	args := []any{}

	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		query += fmt.Sprintf(
			` AND (lower(name) LIKE $%d OR lower(code) LIKE $%d OR lower(coalesce(contact_name, '')) LIKE $%d)`,
			len(args), len(args), len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	query += ` ORDER BY name`
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing suppliers: %w", err)
	}
	defer rows.Close()

	out := []domain.Supplier{}
	for rows.Next() {
		s, err := scanSupplier(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning supplier: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Supplier(ctx context.Context, id string) (domain.Supplier, error) {
	s, err := scanSupplier(r.db.QueryRow(ctx, `SELECT `+supplierColumns+` FROM purchasing.suppliers WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Supplier{}, httpx.NotFound("supplier")
	}
	if err != nil {
		return domain.Supplier{}, fmt.Errorf("purchasing: reading supplier: %w", err)
	}
	return s, nil
}

func (r *Repository) InsertSupplier(ctx context.Context, s domain.Supplier) (domain.Supplier, error) {
	created, err := scanSupplier(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.suppliers (id, code, name, contact_name, contact_phone, email,
			address, city, tax_number, payment_terms, bank_name, bank_account, bank_holder,
			category, status, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING `+supplierColumns,
		s.ID, s.Code, s.Name, s.ContactName, s.ContactPhone, s.Email, s.Address, s.City,
		s.TaxNumber, s.PaymentTerms, s.BankName, s.BankAccount, s.BankHolder, s.Category,
		s.Status, s.Note))
	if database.IsUniqueViolation(err) {
		return domain.Supplier{}, httpx.Conflict("DUPLICATE", "That supplier code is taken.")
	}
	if err != nil {
		return domain.Supplier{}, fmt.Errorf("purchasing: inserting supplier: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateSupplier(ctx context.Context, s domain.Supplier) (domain.Supplier, error) {
	updated, err := scanSupplier(r.db.QueryRow(ctx, `
		UPDATE purchasing.suppliers SET code = $2, name = $3, contact_name = $4,
			contact_phone = $5, email = $6, address = $7, city = $8, tax_number = $9,
			payment_terms = $10, bank_name = $11, bank_account = $12, bank_holder = $13,
			category = $14, status = $15, note = $16, updated_at = now()
		WHERE id = $1 RETURNING `+supplierColumns,
		s.ID, s.Code, s.Name, s.ContactName, s.ContactPhone, s.Email, s.Address, s.City,
		s.TaxNumber, s.PaymentTerms, s.BankName, s.BankAccount, s.BankHolder, s.Category,
		s.Status, s.Note))
	if database.IsNoRows(err) {
		return domain.Supplier{}, httpx.NotFound("supplier")
	}
	if database.IsUniqueViolation(err) {
		return domain.Supplier{}, httpx.Conflict("DUPLICATE", "Another supplier uses that code.")
	}
	if err != nil {
		return domain.Supplier{}, fmt.Errorf("purchasing: updating supplier: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteSupplier(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM purchasing.suppliers WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.Conflict("IN_USE", "That supplier has orders against it. Block it instead.")
	}
	if err != nil {
		return fmt.Errorf("purchasing: deleting supplier: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("supplier")
	}
	return nil
}

// ── Supplier prices ──────────────────────────────────────────────────────────

const priceColumns = `id, supplier_id, item_id, unit_price_idr, min_order_qty, lead_time_days,
	effective_from, active, created_at`

func scanPrice(row pgx.Row) (domain.SupplierPrice, error) {
	var p domain.SupplierPrice
	var from time.Time
	err := row.Scan(&p.ID, &p.SupplierID, &p.ItemID, &p.UnitPriceIDR, &p.MinOrderQty,
		&p.LeadTimeDays, &from, &p.Active, &p.CreatedAt)
	if err != nil {
		return domain.SupplierPrice{}, err
	}
	p.EffectiveFrom = scanDate(&from)
	return p, nil
}

func (r *Repository) SupplierPrices(ctx context.Context, supplierID, itemID string) ([]domain.SupplierPrice, error) {
	query := `SELECT ` + priceColumns + ` FROM purchasing.supplier_prices WHERE active`
	args := []any{}
	if supplierID != "" {
		args = append(args, supplierID)
		query += fmt.Sprintf(` AND supplier_id = $%d`, len(args))
	}
	if itemID != "" {
		args = append(args, itemID)
		query += fmt.Sprintf(` AND item_id = $%d`, len(args))
	}
	query += ` ORDER BY effective_from DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("purchasing: listing supplier prices: %w", err)
	}
	defer rows.Close()

	out := []domain.SupplierPrice{}
	for rows.Next() {
		p, err := scanPrice(rows)
		if err != nil {
			return nil, fmt.Errorf("purchasing: scanning supplier price: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertSupplierPrice(ctx context.Context, p domain.SupplierPrice) (domain.SupplierPrice, error) {
	saved, err := scanPrice(r.db.QueryRow(ctx, `
		INSERT INTO purchasing.supplier_prices (id, supplier_id, item_id, unit_price_idr,
			min_order_qty, lead_time_days, effective_from, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (supplier_id, item_id, effective_from) DO UPDATE
			SET unit_price_idr = EXCLUDED.unit_price_idr,
				min_order_qty = EXCLUDED.min_order_qty,
				lead_time_days = EXCLUDED.lead_time_days,
				active = EXCLUDED.active
		RETURNING `+priceColumns,
		p.ID, p.SupplierID, p.ItemID, p.UnitPriceIDR, p.MinOrderQty, p.LeadTimeDays,
		requiredDate(p.EffectiveFrom), p.Active))
	if database.IsForeignKeyViolation(err) {
		return domain.SupplierPrice{}, httpx.NotFound("supplier")
	}
	if err != nil {
		return domain.SupplierPrice{}, fmt.Errorf("purchasing: saving supplier price: %w", err)
	}
	return saved, nil
}
