package seed

import (
	"context"
	"fmt"
)

// seedPOS puts the shelf on the till: the same items inventory counts, priced
// for the counter, so selling one visibly moves stock.
func (s *Seeder) seedPOS(ctx context.Context) (int, error) {
	categories := [][2]string{
		{"pctg_apparel", "Apparel"},
		{"pctg_nutrition", "Nutrition"},
		{"pctg_equipment", "Equipment"},
		{"pctg_service", "Services"},
	}
	for i, c := range categories {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.categories (id, name, sort_order) VALUES ($1, $2, $3)
			ON CONFLICT (id) DO NOTHING`, c[0], c[1], i+1); err != nil {
			return 0, fmt.Errorf("seed: pos category %s: %w", c[0], err)
		}
	}

	products := []struct {
		id, sku, name, category, item string
		price, cost                   float64
		bonusXP                       int
	}{
		{"prd_tee", "P-TEE", "NuHabit Training Tee", "pctg_apparel", "itm_tee", 189_000, 85_000, 20},
		{"prd_tank", "P-TANK", "NuHabit Tank Top", "pctg_apparel", "itm_tank", 169_000, 78_000, 20},
		{"prd_bottle", "P-BTL", "Steel Bottle 750ml", "pctg_equipment", "itm_bottle", 139_000, 62_000, 15},
		{"prd_grips", "P-GRIP", "Training Grips", "pctg_equipment", "itm_grips", 199_000, 95_000, 15},
		{"prd_shaker", "P-SHKR", "Protein Shaker", "pctg_equipment", "itm_shaker", 79_000, 35_000, 10},
		{"prd_bar", "P-BAR", "Protein Bar", "pctg_nutrition", "itm_bar", 35_000, 18_000, 0},
		{"prd_iso", "P-ISO", "Isotonic Drink", "pctg_nutrition", "itm_iso", 25_000, 12_000, 0},
		{"prd_whey", "P-WHEY", "Whey Protein 1kg", "pctg_nutrition", "itm_whey", 749_000, 480_000, 50},
	}
	for _, p := range products {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.products (id, sku, name, category_id, inventory_item_id,
				price_idr, cost_idr, bonus_xp)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO NOTHING`,
			p.id, p.sku, p.name, p.category, p.item, p.price, p.cost, p.bonusXP); err != nil {
			return 0, fmt.Errorf("seed: pos product %s: %w", p.id, err)
		}
	}

	// A service: it has a price and earns points, but moves no stock. Its
	// inventory_item_id is deliberately null.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO pos.products (id, sku, name, category_id, price_idr, cost_idr, bonus_xp)
		VALUES ('prd_towel_hire', 'P-TWLH', 'Towel hire', 'pctg_service', 15000, 0, 0)
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return 0, fmt.Errorf("seed: towel hire: %w", err)
	}

	return len(products) + 1, nil
}
