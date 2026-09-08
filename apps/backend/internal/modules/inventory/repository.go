package inventory

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

// Repository persists stock. It reads and writes the `inventory` schema only;
// branches are referenced by plain id.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── Categories ───────────────────────────────────────────────────────────────

const categoryColumns = `id, name, code, active, sort_order, created_at, updated_at`

func scanCategory(row pgx.Row) (domain.InventoryCategory, error) {
	var c domain.InventoryCategory
	err := row.Scan(&c.ID, &c.Name, &c.Code, &c.Active, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *Repository) Categories(ctx context.Context) ([]domain.InventoryCategory, error) {
	rows, err := r.db.Query(ctx, `SELECT `+categoryColumns+` FROM inventory.categories ORDER BY sort_order, name`)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing categories: %w", err)
	}
	defer rows.Close()

	out := []domain.InventoryCategory{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) InsertCategory(ctx context.Context, c domain.InventoryCategory) (domain.InventoryCategory, error) {
	created, err := scanCategory(r.db.QueryRow(ctx, `
		INSERT INTO inventory.categories (id, name, code, active, sort_order)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+categoryColumns,
		c.ID, c.Name, c.Code, c.Active, c.SortOrder))
	if database.IsUniqueViolation(err) {
		return domain.InventoryCategory{}, httpx.Conflict("DUPLICATE", "That category code is taken.")
	}
	if err != nil {
		return domain.InventoryCategory{}, fmt.Errorf("inventory: inserting category: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateCategory(ctx context.Context, c domain.InventoryCategory) (domain.InventoryCategory, error) {
	updated, err := scanCategory(r.db.QueryRow(ctx, `
		UPDATE inventory.categories SET name = $2, code = $3, active = $4, sort_order = $5,
			updated_at = now()
		WHERE id = $1 RETURNING `+categoryColumns,
		c.ID, c.Name, c.Code, c.Active, c.SortOrder))
	if database.IsNoRows(err) {
		return domain.InventoryCategory{}, httpx.NotFound("category")
	}
	if err != nil {
		return domain.InventoryCategory{}, fmt.Errorf("inventory: updating category: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM inventory.categories WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.Conflict("IN_USE", "Items still belong to that category.")
	}
	if err != nil {
		return fmt.Errorf("inventory: deleting category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("category")
	}
	return nil
}

// ── Items ────────────────────────────────────────────────────────────────────

const itemColumns = `id, sku, name, description, category_id, unit, kind, unit_cost_idr,
	track_stock, track_batches, expiry_warning_days, barcode, image_url, active,
	created_at, updated_at`

func scanItem(row pgx.Row) (domain.InventoryItem, error) {
	var i domain.InventoryItem
	err := row.Scan(&i.ID, &i.SKU, &i.Name, &i.Description, &i.CategoryID, &i.Unit, &i.Kind,
		&i.UnitCostIDR, &i.TrackStock, &i.TrackBatches, &i.ExpiryWarningDays, &i.Barcode,
		&i.ImageURL, &i.Active, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

// ItemFilter narrows the catalogue.
type ItemFilter struct {
	Query      string
	CategoryID string
	Kind       string
	ActiveOnly bool
	Limit      int
}

func (r *Repository) Items(ctx context.Context, filter ItemFilter) ([]domain.InventoryItem, error) {
	query := `SELECT ` + itemColumns + ` FROM inventory.items WHERE 1 = 1`
	args := []any{}

	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		query += fmt.Sprintf(
			` AND (lower(name) LIKE $%d OR lower(sku) LIKE $%d OR lower(coalesce(barcode, '')) LIKE $%d)`,
			len(args), len(args), len(args))
	}
	if filter.CategoryID != "" {
		args = append(args, filter.CategoryID)
		query += fmt.Sprintf(` AND category_id = $%d`, len(args))
	}
	if filter.Kind != "" {
		args = append(args, filter.Kind)
		query += fmt.Sprintf(` AND kind = $%d`, len(args))
	}
	if filter.ActiveOnly {
		query += ` AND active`
	}
	query += ` ORDER BY name`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing items: %w", err)
	}
	defer rows.Close()

	out := []domain.InventoryItem{}
	for rows.Next() {
		i, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Item reads one, optionally locking it because a receipt changes its average
// cost and two receipts must not interleave.
func (r *Repository) Item(ctx context.Context, id string, forUpdate bool) (domain.InventoryItem, error) {
	query := `SELECT ` + itemColumns + ` FROM inventory.items WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	item, err := scanItem(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.InventoryItem{}, httpx.NotFound("item")
	}
	if err != nil {
		return domain.InventoryItem{}, fmt.Errorf("inventory: reading item: %w", err)
	}
	return item, nil
}

func (r *Repository) InsertItem(ctx context.Context, i domain.InventoryItem) (domain.InventoryItem, error) {
	created, err := scanItem(r.db.QueryRow(ctx, `
		INSERT INTO inventory.items (id, sku, name, description, category_id, unit, kind,
			unit_cost_idr, track_stock, track_batches, expiry_warning_days, barcode,
			image_url, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING `+itemColumns,
		i.ID, i.SKU, i.Name, i.Description, i.CategoryID, i.Unit, i.Kind, i.UnitCostIDR,
		i.TrackStock, i.TrackBatches, i.ExpiryWarningDays, i.Barcode, i.ImageURL, i.Active))
	if database.IsUniqueViolation(err) {
		return domain.InventoryItem{}, httpx.Conflict("DUPLICATE", "That SKU is already in the catalogue.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.InventoryItem{}, httpx.NotFound("category")
	}
	if err != nil {
		return domain.InventoryItem{}, fmt.Errorf("inventory: inserting item: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateItem(ctx context.Context, i domain.InventoryItem) (domain.InventoryItem, error) {
	updated, err := scanItem(r.db.QueryRow(ctx, `
		UPDATE inventory.items SET sku = $2, name = $3, description = $4, category_id = $5,
			unit = $6, kind = $7, track_stock = $8, track_batches = $9,
			expiry_warning_days = $10, barcode = $11, image_url = $12,
			active = $13, updated_at = now()
		WHERE id = $1 RETURNING `+itemColumns,
		i.ID, i.SKU, i.Name, i.Description, i.CategoryID, i.Unit, i.Kind,
		i.TrackStock, i.TrackBatches, i.ExpiryWarningDays, i.Barcode, i.ImageURL, i.Active))
	if database.IsNoRows(err) {
		return domain.InventoryItem{}, httpx.NotFound("item")
	}
	if database.IsUniqueViolation(err) {
		return domain.InventoryItem{}, httpx.Conflict("DUPLICATE", "Another item already uses that SKU.")
	}
	if err != nil {
		return domain.InventoryItem{}, fmt.Errorf("inventory: updating item: %w", err)
	}
	return updated, nil
}

// SetUnitCost writes the weighted average back. Deliberately not part of
// UpdateItem: the cost is derived from receipts, never typed in.
func (r *Repository) SetUnitCost(ctx context.Context, itemID string, cost float64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE inventory.items SET unit_cost_idr = $2, updated_at = now() WHERE id = $1`,
		itemID, cost)
	if err != nil {
		return fmt.Errorf("inventory: setting unit cost: %w", err)
	}
	return nil
}

// ── Stock levels ─────────────────────────────────────────────────────────────

const levelColumns = `item_id, branch_id, qty_on_hand, qty_on_order, qty_minimum, qty_maximum,
	bin_location, last_movement_at, note`

func scanLevel(row pgx.Row) (domain.StockLevel, error) {
	var l domain.StockLevel
	err := row.Scan(&l.ItemID, &l.BranchID, &l.QtyOnHand, &l.QtyOnOrder, &l.QtyMinimum,
		&l.QtyMaximum, &l.BinLocation, &l.LastMovementAt, &l.Note)
	return l, err
}

// Level reads one item at one branch, creating the row on first touch so a new
// item is countable everywhere without seeding.
//
// forUpdate is what stops two tills selling the same last unit.
func (r *Repository) Level(ctx context.Context, itemID, branchID string, forUpdate bool) (domain.StockLevel, error) {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO inventory.stock_levels (item_id, branch_id) VALUES ($1, $2)
		ON CONFLICT (item_id, branch_id) DO NOTHING`, itemID, branchID); err != nil {
		if database.IsForeignKeyViolation(err) {
			return domain.StockLevel{}, httpx.NotFound("item")
		}
		return domain.StockLevel{}, fmt.Errorf("inventory: ensuring stock level: %w", err)
	}

	query := `SELECT ` + levelColumns + ` FROM inventory.stock_levels WHERE item_id = $1 AND branch_id = $2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	level, err := scanLevel(r.db.QueryRow(ctx, query, itemID, branchID))
	if err != nil {
		return domain.StockLevel{}, fmt.Errorf("inventory: reading stock level: %w", err)
	}
	return level, nil
}

// LevelFilter narrows the stock list.
type LevelFilter struct {
	BranchID   string
	CategoryID string
	Query      string
	LowOnly    bool
	Limit      int
}

// LevelRow is a level with its item, which is how every screen wants it.
type LevelRow struct {
	Item  domain.InventoryItem
	Level domain.StockLevel
}

func (r *Repository) Levels(ctx context.Context, filter LevelFilter) ([]LevelRow, error) {
	query := `
		SELECT ` + prefixed(itemColumns, "i") + `, ` + prefixed(levelColumns, "l") + `
		FROM inventory.stock_levels l
		JOIN inventory.items i ON i.id = l.item_id
		WHERE 1 = 1`
	args := []any{}

	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND l.branch_id = $%d`, len(args))
	}
	if filter.CategoryID != "" {
		args = append(args, filter.CategoryID)
		query += fmt.Sprintf(` AND i.category_id = $%d`, len(args))
	}
	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		args = append(args, "%"+strings.ToLower(trimmed)+"%")
		query += fmt.Sprintf(` AND (lower(i.name) LIKE $%d OR lower(i.sku) LIKE $%d)`, len(args), len(args))
	}
	if filter.LowOnly {
		// The same rule as domain.IsLowStock, in SQL, so the list can be paged
		// by the database rather than filtered in Go after loading everything.
		query += ` AND l.qty_on_hand + l.qty_on_order <= l.qty_minimum`
	}
	query += ` ORDER BY i.name, l.branch_id`
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	args = append(args, limit)
	query += fmt.Sprintf(` LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing stock: %w", err)
	}
	defer rows.Close()

	out := []LevelRow{}
	for rows.Next() {
		var row LevelRow
		if err := rows.Scan(
			&row.Item.ID, &row.Item.SKU, &row.Item.Name, &row.Item.Description, &row.Item.CategoryID,
			&row.Item.Unit, &row.Item.Kind, &row.Item.UnitCostIDR, &row.Item.TrackStock,
			&row.Item.TrackBatches, &row.Item.ExpiryWarningDays,
			&row.Item.Barcode, &row.Item.ImageURL, &row.Item.Active, &row.Item.CreatedAt, &row.Item.UpdatedAt,
			&row.Level.ItemID, &row.Level.BranchID, &row.Level.QtyOnHand, &row.Level.QtyOnOrder,
			&row.Level.QtyMinimum, &row.Level.QtyMaximum, &row.Level.BinLocation,
			&row.Level.LastMovementAt, &row.Level.Note,
		); err != nil {
			return nil, fmt.Errorf("inventory: scanning stock row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// prefixed qualifies a column list with a table alias.
func prefixed(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for i, part := range parts {
		parts[i] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

// SaveLevel writes a level back. Callers hold the row lock from Level.
func (r *Repository) SaveLevel(ctx context.Context, l domain.StockLevel, touchMovement bool) (domain.StockLevel, error) {
	movement := `last_movement_at`
	if touchMovement {
		movement = `now()`
	}
	saved, err := scanLevel(r.db.QueryRow(ctx, `
		UPDATE inventory.stock_levels SET qty_on_hand = $3, qty_on_order = $4, qty_minimum = $5,
			qty_maximum = $6, bin_location = $7, note = $8, last_movement_at = `+movement+`,
			updated_at = now()
		WHERE item_id = $1 AND branch_id = $2 RETURNING `+levelColumns,
		l.ItemID, l.BranchID, l.QtyOnHand, l.QtyOnOrder, l.QtyMinimum, l.QtyMaximum,
		l.BinLocation, l.Note))
	if database.IsNoRows(err) {
		return domain.StockLevel{}, httpx.NotFound("stock level")
	}
	if database.IsCheckViolation(err) {
		return domain.StockLevel{}, httpx.Conflict("INSUFFICIENT_STOCK",
			"That would take the quantity below zero.")
	}
	if err != nil {
		return domain.StockLevel{}, fmt.Errorf("inventory: saving stock level: %w", err)
	}
	return saved, nil
}

// AdjustOnOrder moves the on-order figure, which purchasing drives.
func (r *Repository) AdjustOnOrder(ctx context.Context, itemID, branchID string, delta domain.Quantity) error {
	if _, err := r.Level(ctx, itemID, branchID, false); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `
		UPDATE inventory.stock_levels
		SET qty_on_order = GREATEST(0, qty_on_order + $3), updated_at = now()
		WHERE item_id = $1 AND branch_id = $2`, itemID, branchID, delta)
	if err != nil {
		return fmt.Errorf("inventory: adjusting on-order: %w", err)
	}
	return nil
}

// ── Movements ────────────────────────────────────────────────────────────────

const movementColumns = `id, item_id, branch_id, kind, qty, qty_before, qty_after,
	unit_cost_idr, total_cost_idr, pack_unit, pack_qty, pack_factor,
	reference_type, reference_id, reference_number,
	reason, note, actor_id, actor_name, created_at`

func scanMovement(row pgx.Row) (domain.StockMovement, error) {
	var m domain.StockMovement
	err := row.Scan(&m.ID, &m.ItemID, &m.BranchID, &m.Kind, &m.Qty, &m.QtyBefore, &m.QtyAfter,
		&m.UnitCostIDR, &m.TotalCostIDR, &m.PackUnit, &m.PackQty, &m.PackFactor,
		&m.ReferenceType, &m.ReferenceID, &m.ReferenceNumber,
		&m.Reason, &m.Note, &m.ActorID, &m.ActorName, &m.CreatedAt)
	return m, err
}

func (r *Repository) InsertMovement(ctx context.Context, m domain.StockMovement) (domain.StockMovement, error) {
	created, err := scanMovement(r.db.QueryRow(ctx, `
		INSERT INTO inventory.stock_movements (id, item_id, branch_id, kind, qty, qty_before,
			qty_after, unit_cost_idr, total_cost_idr, pack_unit, pack_qty, pack_factor,
			reference_type, reference_id, reference_number, reason, note, actor_id, actor_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING `+movementColumns,
		m.ID, m.ItemID, m.BranchID, m.Kind, m.Qty, m.QtyBefore, m.QtyAfter, m.UnitCostIDR,
		m.TotalCostIDR, m.PackUnit, m.PackQty, m.PackFactor,
		m.ReferenceType, m.ReferenceID, m.ReferenceNumber, m.Reason, m.Note,
		m.ActorID, m.ActorName))
	if database.IsCheckViolation(err) {
		return domain.StockMovement{}, httpx.Invalid(
			"That movement's pack quantity does not multiply out to its base quantity.")
	}
	if err != nil {
		return domain.StockMovement{}, fmt.Errorf("inventory: inserting movement: %w", err)
	}
	return created, nil
}

// MovementFilter narrows the ledger.
type MovementFilter struct {
	ItemID        string
	BranchID      string
	Kind          string
	ReferenceType string
	ReferenceID   string
	Limit         int
}

func (r *Repository) Movements(ctx context.Context, filter MovementFilter) ([]domain.StockMovement, error) {
	query := `SELECT ` + movementColumns + ` FROM inventory.stock_movements WHERE 1 = 1`
	args := []any{}

	if filter.ItemID != "" {
		args = append(args, filter.ItemID)
		query += fmt.Sprintf(` AND item_id = $%d`, len(args))
	}
	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if filter.Kind != "" {
		args = append(args, filter.Kind)
		query += fmt.Sprintf(` AND kind = $%d`, len(args))
	}
	if filter.ReferenceType != "" {
		args = append(args, filter.ReferenceType)
		query += fmt.Sprintf(` AND reference_type = $%d`, len(args))
	}
	if filter.ReferenceID != "" {
		args = append(args, filter.ReferenceID)
		query += fmt.Sprintf(` AND reference_id = $%d`, len(args))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing movements: %w", err)
	}
	defer rows.Close()

	out := []domain.StockMovement{}
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning movement: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SumMovements is what the ledger says a level should be. The integration
// suite uses it to prove the cached quantity never drifts from its history.
func (r *Repository) SumMovements(ctx context.Context, itemID, branchID string) (domain.Quantity, error) {
	var total domain.Quantity
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(qty), 0) FROM inventory.stock_movements
		WHERE item_id = $1 AND branch_id = $2`, itemID, branchID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("inventory: summing movements: %w", err)
	}
	return total, nil
}

// ── Valuation ────────────────────────────────────────────────────────────────

// Valuation is what the shelves are worth.
type Valuation struct {
	Items      int     `json:"items"`
	Units      float64 `json:"units"`
	ValueIDR   float64 `json:"valueIdr"`
	LowStock   int     `json:"lowStock"`
	OutOfStock int     `json:"outOfStock"`
}

func (r *Repository) Valuation(ctx context.Context, branchID string) (Valuation, error) {
	query := `
		SELECT count(*), COALESCE(SUM(l.qty_on_hand), 0),
			COALESCE(SUM(l.qty_on_hand * i.unit_cost_idr), 0),
			count(*) FILTER (WHERE l.qty_on_hand + l.qty_on_order <= l.qty_minimum),
			count(*) FILTER (WHERE l.qty_on_hand = 0)
		FROM inventory.stock_levels l
		JOIN inventory.items i ON i.id = l.item_id
		WHERE i.active AND i.track_stock`
	args := []any{}
	if branchID != "" {
		args = append(args, branchID)
		query += ` AND l.branch_id = $1`
	}

	var v Valuation
	if err := r.db.QueryRow(ctx, query, args...).
		Scan(&v.Items, &v.Units, &v.ValueIDR, &v.LowStock, &v.OutOfStock); err != nil {
		return Valuation{}, fmt.Errorf("inventory: valuing stock: %w", err)
	}
	return v, nil
}

// ── Stock takes ──────────────────────────────────────────────────────────────

const takeColumns = `id, take_number, branch_id, status, counted_on, note,
	applied_by, applied_at, created_at, updated_at`

func scanTake(row pgx.Row) (domain.StockTake, error) {
	var t domain.StockTake
	var countedOn time.Time
	err := row.Scan(&t.ID, &t.TakeNumber, &t.BranchID, &t.Status, &countedOn, &t.Note,
		&t.AppliedBy, &t.AppliedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return domain.StockTake{}, err
	}
	t.CountedOn = domain.Date(countedOn.Format("2006-01-02"))
	return t, nil
}

func (r *Repository) StockTakes(ctx context.Context, branchID, status string, limit int) ([]domain.StockTake, error) {
	query := `SELECT ` + takeColumns + ` FROM inventory.stock_takes WHERE 1 = 1`
	args := []any{}
	if branchID != "" {
		args = append(args, branchID)
		query += fmt.Sprintf(` AND branch_id = $%d`, len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY counted_on DESC, created_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing stock takes: %w", err)
	}
	defer rows.Close()

	out := []domain.StockTake{}
	for rows.Next() {
		t, err := scanTake(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning stock take: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) StockTake(ctx context.Context, id string, forUpdate bool) (domain.StockTake, error) {
	query := `SELECT ` + takeColumns + ` FROM inventory.stock_takes WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	t, err := scanTake(r.db.QueryRow(ctx, query, id))
	if database.IsNoRows(err) {
		return domain.StockTake{}, httpx.NotFound("stock take")
	}
	if err != nil {
		return domain.StockTake{}, fmt.Errorf("inventory: reading stock take: %w", err)
	}
	return t, nil
}

func (r *Repository) InsertStockTake(ctx context.Context, t domain.StockTake) (domain.StockTake, error) {
	created, err := scanTake(r.db.QueryRow(ctx, `
		INSERT INTO inventory.stock_takes (id, take_number, branch_id, status, counted_on, note)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+takeColumns,
		t.ID, t.TakeNumber, t.BranchID, t.Status, string(t.CountedOn), t.Note))
	if database.IsUniqueViolation(err) {
		return domain.StockTake{}, httpx.Conflict("DUPLICATE", "That stock take number is taken.")
	}
	if err != nil {
		return domain.StockTake{}, fmt.Errorf("inventory: inserting stock take: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateStockTakeStatus(ctx context.Context, t domain.StockTake) (domain.StockTake, error) {
	updated, err := scanTake(r.db.QueryRow(ctx, `
		UPDATE inventory.stock_takes SET status = $2, applied_by = $3, applied_at = $4,
			note = $5, updated_at = now()
		WHERE id = $1 RETURNING `+takeColumns,
		t.ID, t.Status, t.AppliedBy, t.AppliedAt, t.Note))
	if database.IsNoRows(err) {
		return domain.StockTake{}, httpx.NotFound("stock take")
	}
	if err != nil {
		return domain.StockTake{}, fmt.Errorf("inventory: updating stock take: %w", err)
	}
	return updated, nil
}

const takeLineColumns = `id, stock_take_id, item_id, qty_expected, qty_counted, note`

func scanTakeLine(row pgx.Row) (domain.StockTakeLine, error) {
	var l domain.StockTakeLine
	err := row.Scan(&l.ID, &l.StockTakeID, &l.ItemID, &l.QtyExpected, &l.QtyCounted, &l.Note)
	return l, err
}

func (r *Repository) StockTakeLines(ctx context.Context, takeID string) ([]domain.StockTakeLine, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+takeLineColumns+` FROM inventory.stock_take_lines WHERE stock_take_id = $1 ORDER BY id`,
		takeID)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing stock take lines: %w", err)
	}
	defer rows.Close()

	out := []domain.StockTakeLine{}
	for rows.Next() {
		l, err := scanTakeLine(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning stock take line: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpsertStockTakeLine records a count. Counting the same item twice corrects
// the line rather than adding a second one.
func (r *Repository) UpsertStockTakeLine(ctx context.Context, l domain.StockTakeLine) (domain.StockTakeLine, error) {
	saved, err := scanTakeLine(r.db.QueryRow(ctx, `
		INSERT INTO inventory.stock_take_lines (id, stock_take_id, item_id, qty_expected, qty_counted, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (stock_take_id, item_id) DO UPDATE
			SET qty_counted = EXCLUDED.qty_counted, note = EXCLUDED.note
		RETURNING `+takeLineColumns,
		l.ID, l.StockTakeID, l.ItemID, l.QtyExpected, l.QtyCounted, l.Note))
	if database.IsForeignKeyViolation(err) {
		return domain.StockTakeLine{}, httpx.NotFound("stock take or item")
	}
	if err != nil {
		return domain.StockTakeLine{}, fmt.Errorf("inventory: saving stock take line: %w", err)
	}
	return saved, nil
}

func (r *Repository) DeleteStockTakeLine(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM inventory.stock_take_lines WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("inventory: deleting stock take line: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("stock take line")
	}
	return nil
}

// ── Transfers ────────────────────────────────────────────────────────────────

const transferColumns = `id, transfer_number, item_id, from_branch_id, to_branch_id, qty,
	note, actor_id, actor_name, created_at`

func scanTransfer(row pgx.Row) (domain.StockTransfer, error) {
	var t domain.StockTransfer
	err := row.Scan(&t.ID, &t.TransferNumber, &t.ItemID, &t.FromBranchID, &t.ToBranchID,
		&t.Qty, &t.Note, &t.ActorID, &t.ActorName, &t.CreatedAt)
	return t, err
}

func (r *Repository) InsertTransfer(ctx context.Context, t domain.StockTransfer) (domain.StockTransfer, error) {
	created, err := scanTransfer(r.db.QueryRow(ctx, `
		INSERT INTO inventory.stock_transfers (id, transfer_number, item_id, from_branch_id,
			to_branch_id, qty, note, actor_id, actor_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+transferColumns,
		t.ID, t.TransferNumber, t.ItemID, t.FromBranchID, t.ToBranchID, t.Qty, t.Note,
		t.ActorID, t.ActorName))
	if database.IsCheckViolation(err) {
		return domain.StockTransfer{}, httpx.Invalid("A transfer needs two different branches.")
	}
	if err != nil {
		return domain.StockTransfer{}, fmt.Errorf("inventory: inserting transfer: %w", err)
	}
	return created, nil
}

func (r *Repository) Transfers(ctx context.Context, itemID string, limit int) ([]domain.StockTransfer, error) {
	query := `SELECT ` + transferColumns + ` FROM inventory.stock_transfers WHERE 1 = 1`
	args := []any{}
	if itemID != "" {
		args = append(args, itemID)
		query += fmt.Sprintf(` AND item_id = $%d`, len(args))
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inventory: listing transfers: %w", err)
	}
	defer rows.Close()

	out := []domain.StockTransfer{}
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: scanning transfer: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
