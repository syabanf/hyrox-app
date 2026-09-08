package inventory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Units and packs: the conversion between what arrives on a pallet and what
// leaves in a hand.

const unitColumns = `id, code, name, kind, active, sort_order`

func scanUnit(row pgx.Row) (domain.Unit, error) {
	var u domain.Unit
	err := row.Scan(&u.ID, &u.Code, &u.Name, &u.Kind, &u.Active, &u.SortOrder)
	return u, err
}

func (r *Repository) Units(ctx context.Context, activeOnly bool) ([]domain.Unit, error) {
	query := `SELECT ` + unitColumns + ` FROM inventory.units`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY sort_order, code`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing units: %w", err)
	}
	defer rows.Close()

	out := []domain.Unit{}
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning unit: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) Unit(ctx context.Context, code string) (domain.Unit, error) {
	u, err := scanUnit(r.db.QueryRow(ctx,
		`SELECT `+unitColumns+` FROM inventory.units WHERE code = $1`, code))
	if database.IsNoRows(err) {
		return domain.Unit{}, httpx.NotFound("unit")
	}
	if err != nil {
		return domain.Unit{}, fmt.Errorf("inventory: reading unit: %w", err)
	}
	return u, nil
}

func (r *Repository) UpsertUnit(ctx context.Context, u domain.Unit) (domain.Unit, error) {
	saved, err := scanUnit(r.db.QueryRow(ctx, `
		INSERT INTO inventory.units (id, code, name, kind, active, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind,
			active = EXCLUDED.active, sort_order = EXCLUDED.sort_order, updated_at = now()
		RETURNING `+unitColumns,
		u.ID, u.Code, u.Name, u.Kind, u.Active, u.SortOrder))
	if err != nil {
		return domain.Unit{}, fmt.Errorf("inventory: saving unit: %w", err)
	}
	return saved, nil
}

const packColumns = `id, item_id, unit_code, factor, barcode, is_base,
	purchase_default, sale_default, active`

func scanPack(row pgx.Row) (domain.ItemPack, error) {
	var p domain.ItemPack
	err := row.Scan(&p.ID, &p.ItemID, &p.UnitCode, &p.Factor, &p.Barcode, &p.IsBase,
		&p.PurchaseDefault, &p.SaleDefault, &p.Active)
	return p, err
}

// ItemPacks is every way one item may be handed over, base first so a caller
// reading only the head gets the unit the ledger counts in.
func (r *Repository) ItemPacks(ctx context.Context, itemID string) ([]domain.ItemPack, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+packColumns+` FROM inventory.item_packs WHERE item_id = $1
		 ORDER BY is_base DESC, factor`, itemID)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing packs: %w", err)
	}
	defer rows.Close()

	out := []domain.ItemPack{}
	for rows.Next() {
		p, err := scanPack(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning pack: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PackByBarcode resolves a scan to one pack of one item.
func (r *Repository) PackByBarcode(ctx context.Context, barcode string) (domain.ItemPack, error) {
	p, err := scanPack(r.db.QueryRow(ctx,
		`SELECT `+packColumns+` FROM inventory.item_packs WHERE barcode = $1`, barcode))
	if database.IsNoRows(err) {
		return domain.ItemPack{}, httpx.NotFound("barcode")
	}
	if err != nil {
		return domain.ItemPack{}, fmt.Errorf("inventory: resolving barcode: %w", err)
	}
	return p, nil
}

func (r *Repository) UpsertPack(ctx context.Context, p domain.ItemPack) (domain.ItemPack, error) {
	saved, err := scanPack(r.db.QueryRow(ctx, `
		INSERT INTO inventory.item_packs (id, item_id, unit_code, factor, barcode, is_base,
			purchase_default, sale_default, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (item_id, unit_code) DO UPDATE SET factor = EXCLUDED.factor,
			barcode = EXCLUDED.barcode, purchase_default = EXCLUDED.purchase_default,
			sale_default = EXCLUDED.sale_default, active = EXCLUDED.active, updated_at = now()
		RETURNING `+packColumns,
		p.ID, p.ItemID, p.UnitCode, p.Factor, p.Barcode, p.IsBase,
		p.PurchaseDefault, p.SaleDefault, p.Active))
	if database.IsUniqueViolation(err) {
		return domain.ItemPack{}, httpx.Conflict("DUPLICATE",
			"That barcode or default already belongs to another pack.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.ItemPack{}, httpx.NotFound("item or unit")
	}
	if database.IsCheckViolation(err) {
		return domain.ItemPack{}, httpx.Invalid("The base pack holds exactly one base unit.")
	}
	if err != nil {
		return domain.ItemPack{}, fmt.Errorf("inventory: saving pack: %w", err)
	}
	return saved, nil
}

// ClearPurchaseDefault and ClearSaleDefault make room before a new default is
// set: the partial unique indexes allow one each, so the old one has to go
// first rather than the write failing.
func (r *Repository) ClearPurchaseDefault(ctx context.Context, itemID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE inventory.item_packs SET purchase_default = false, updated_at = now()
		 WHERE item_id = $1 AND purchase_default`, itemID)
	if err != nil {
		return fmt.Errorf("inventory: clearing purchase default: %w", err)
	}
	return nil
}

func (r *Repository) ClearSaleDefault(ctx context.Context, itemID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE inventory.item_packs SET sale_default = false, updated_at = now()
		 WHERE item_id = $1 AND sale_default`, itemID)
	if err != nil {
		return fmt.Errorf("inventory: clearing sale default: %w", err)
	}
	return nil
}

// DeletePack removes a way of handing an item over. The base pack is not one
// of the things that can be removed: it is the unit the ledger counts in.
func (r *Repository) DeletePack(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM inventory.item_packs WHERE id = $1 AND NOT is_base`, id)
	if err != nil {
		return fmt.Errorf("inventory: deleting pack: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("BASE_PACK",
			"That is the unit stock is counted in, and cannot be removed.")
	}
	return nil
}

// ItemIDBySKU resolves a SKU from somebody else's spreadsheet to an item here.
func (r *Repository) ItemIDBySKU(ctx context.Context, sku string) (string, error) {
	var itemID string
	err := r.db.QueryRow(ctx,
		`SELECT id FROM inventory.items WHERE upper(sku) = upper($1)`, sku).Scan(&itemID)
	if database.IsNoRows(err) {
		return "", httpx.NotFound("item")
	}
	if err != nil {
		return "", fmt.Errorf("inventory: resolving SKU: %w", err)
	}
	return itemID, nil
}
