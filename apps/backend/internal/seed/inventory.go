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
	return len(items), nil
}
