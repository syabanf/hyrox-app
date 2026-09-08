package seed

import (
	"context"
	"fmt"

	"github.com/syabanf/nuhabit-backend/internal/domain"
)

// seedInventory stocks the shelves: what a studio actually sells at its
// counter, held at both branches, with a movement history behind every
// quantity so the ledger is never empty on a fresh install.
func (s *Seeder) seedInventory(ctx context.Context) (int, error) {
	categories := [][3]string{
		{"ictg_apparel", "Apparel", "APR"},
		{"ictg_nutrition", "Nutrition", "NUT"},
		{"ictg_equipment", "Equipment", "EQP"},
		{"ictg_supplies", "Studio Supplies", "SUP"},
	}
	for i, c := range categories {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO inventory.categories (id, name, code, sort_order)
			VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`,
			c[0], c[1], c[2], i+1); err != nil {
			return 0, fmt.Errorf("seed: inventory category %s: %w", c[0], err)
		}
	}

	items := []struct {
		id, sku, name, category, unit string
		kind                          domain.ItemKind
		cost                          float64
		// opening is what each branch starts with, and minimum is when the
		// reorder screen should start shouting.
		opening, minimum, maximum float64
	}{
		{"itm_tee", "APR-TEE-BLK", "NuHabit Training Tee", "ictg_apparel", "PCS", domain.ItemRetail, 85_000, 40, 12, 80},
		{"itm_tank", "APR-TNK-LIM", "NuHabit Tank Top", "ictg_apparel", "PCS", domain.ItemRetail, 78_000, 24, 10, 60},
		{"itm_bottle", "EQP-BTL-750", "Steel Bottle 750ml", "ictg_equipment", "PCS", domain.ItemRetail, 62_000, 30, 10, 60},
		{"itm_grips", "EQP-GRP-STD", "Training Grips", "ictg_equipment", "PAIR", domain.ItemRetail, 95_000, 18, 8, 40},
		{"itm_shaker", "EQP-SHK-600", "Protein Shaker", "ictg_equipment", "PCS", domain.ItemRetail, 35_000, 45, 15, 90},
		{"itm_bar", "NUT-BAR-CHC", "Protein Bar (Chocolate)", "ictg_nutrition", "PCS", domain.ItemRetail, 18_000, 120, 48, 240},
		{"itm_iso", "NUT-DRK-ISO", "Isotonic Drink 500ml", "ictg_nutrition", "PCS", domain.ItemRetail, 12_000, 96, 48, 240},
		{"itm_whey", "NUT-WHE-1KG", "Whey Protein 1kg", "ictg_nutrition", "PCS", domain.ItemRetail, 480_000, 12, 4, 24},
		// Consumed by the studio, never sold: it still has to be counted and
		// reordered, which is exactly what SUPPLY is for.
		{"itm_towel", "SUP-TWL-GYM", "Gym Towel", "ictg_supplies", "PCS", domain.ItemSupply, 22_000, 80, 40, 160},
		{"itm_wipes", "SUP-WPE-500", "Equipment Wipes (500)", "ictg_supplies", "TUB", domain.ItemSupply, 145_000, 10, 6, 24},
	}

	// How each item is handed over. The base pack is the unit the ledger
	// counts in; the rest are the cartons it is bought by. Barcodes differ per
	// pack, which is the whole reason a pack is a row and not a column.
	// Every item has exactly one base pack — the unit its stock is counted in
	// — and the fast-moving ones also have the carton they are bought by. The
	// carton's barcode is the piece's with a leading digit, which is how real
	// GTIN-14 outer cases are numbered.
	packs := []struct {
		id, item, unit       string
		factor               float64
		barcode              string
		base, purchase, sale bool
	}{
		{"ipk_bar_pcs", "itm_bar", "PCS", 1, "8991234500011", true, false, true},
		{"ipk_bar_ctn", "itm_bar", "CTN", 24, "18991234500018", false, true, false},
		{"ipk_iso_pcs", "itm_iso", "PCS", 1, "8991234500028", true, false, true},
		{"ipk_iso_ctn", "itm_iso", "CTN", 24, "18991234500025", false, true, false},
		{"ipk_whey_pcs", "itm_whey", "PCS", 1, "8991234500035", true, false, true},
		{"ipk_whey_box", "itm_whey", "BOX", 6, "18991234500032", false, true, false},
		{"ipk_tee_pcs", "itm_tee", "PCS", 1, "8991234500042", true, false, true},
		{"ipk_tee_ctn", "itm_tee", "CTN", 12, "18991234500049", false, true, false},
		{"ipk_tank_pcs", "itm_tank", "PCS", 1, "8991234500059", true, true, true},
		{"ipk_bottle_pcs", "itm_bottle", "PCS", 1, "8991234500066", true, false, true},
		{"ipk_bottle_ctn", "itm_bottle", "CTN", 12, "18991234500063", false, true, false},
		{"ipk_grips_pair", "itm_grips", "PAIR", 1, "8991234500073", true, true, true},
		{"ipk_shaker_pcs", "itm_shaker", "PCS", 1, "8991234500080", true, false, true},
		{"ipk_shaker_ctn", "itm_shaker", "CTN", 24, "18991234500087", false, true, false},
		{"ipk_towel_pcs", "itm_towel", "PCS", 1, "8991234500097", true, false, true},
		{"ipk_towel_ctn", "itm_towel", "CTN", 20, "18991234500094", false, true, false},
		{"ipk_wipes_tub", "itm_wipes", "TUB", 1, "8991234500103", true, true, true},
	}

	branches := []string{"brn_senopati", "brn_pik"}
	for _, item := range items {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO inventory.items (id, sku, name, category_id, unit, kind, unit_cost_idr)
			VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (id) DO NOTHING`,
			item.id, item.sku, item.name, item.category, item.unit, item.kind, item.cost); err != nil {
			return 0, fmt.Errorf("seed: inventory item %s: %w", item.id, err)
		}

		for index, branch := range branches {
			// PIK is the smaller branch, so it carries less of everything.
			opening := item.opening
			if index == 1 {
				opening = opening * 0.6
			}
			// One item is deliberately below its reorder point at PIK, so the
			// low-stock screen has something real to show on a fresh install.
			if item.id == "itm_whey" && index == 1 {
				opening = 2
			}

			if _, err := s.db.Exec(ctx, `
				INSERT INTO inventory.stock_levels (item_id, branch_id, qty_on_hand, qty_minimum, qty_maximum, last_movement_at)
				VALUES ($1, $2, $3, $4, $5, now()) ON CONFLICT (item_id, branch_id) DO NOTHING`,
				item.id, branch, opening, item.minimum, item.maximum); err != nil {
				return 0, fmt.Errorf("seed: stock level %s/%s: %w", item.id, branch, err)
			}

			// Every quantity is explained by a movement. An opening balance
			// that appeared from nowhere is the thing this system exists to
			// make impossible.
			movementID := fmt.Sprintf("stm_open_%s_%s", item.id[4:], branch[4:])
			if _, err := s.db.Exec(ctx, `
				INSERT INTO inventory.stock_movements (id, item_id, branch_id, kind, qty,
					qty_before, qty_after, unit_cost_idr, total_cost_idr, reference_type,
					reason, actor_name)
				VALUES ($1, $2, $3, 'IN', $4, 0, $4, $5, $6, 'OPENING_BALANCE',
					'Opening balance', 'Seed')
				ON CONFLICT (id) DO NOTHING`,
				movementID, item.id, branch, opening, item.cost, opening*item.cost); err != nil {
				return 0, fmt.Errorf("seed: opening movement %s: %w", movementID, err)
			}
		}
	}
	for _, pack := range packs {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO inventory.item_packs (id, item_id, unit_code, factor, barcode, is_base,
				purchase_default, sale_default)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (item_id, unit_code) DO UPDATE SET factor = EXCLUDED.factor,
				barcode = EXCLUDED.barcode, purchase_default = EXCLUDED.purchase_default,
				sale_default = EXCLUDED.sale_default`,
			pack.id, pack.item, pack.unit, pack.factor, pack.barcode,
			pack.base, pack.purchase, pack.sale); err != nil {
			return 0, fmt.Errorf("seed: pack %s: %w", pack.id, err)
		}
	}

	return len(items), nil
}
