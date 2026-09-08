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

	// A product is one pack of one item at one price, which is why the drink
	// appears twice: a single off the shelf and a carton for somebody who came
	// for a carton. Same item, same ledger, two barcodes and two prices.
	products := []struct {
		id, sku, name, category, item string
		price, cost                   float64
		bonusXP                       int
		barcode, packUnit             string
		packFactor                    float64
	}{
		{"prd_tee", "P-TEE", "NuHabit Training Tee", "pctg_apparel", "itm_tee", 189_000, 85_000, 20, "8991234500042", "PCS", 1},
		{"prd_tank", "P-TANK", "NuHabit Tank Top", "pctg_apparel", "itm_tank", 169_000, 78_000, 20, "8991234500059", "PCS", 1},
		{"prd_bottle", "P-BTL", "Steel Bottle 750ml", "pctg_equipment", "itm_bottle", 139_000, 62_000, 15, "8991234500066", "PCS", 1},
		{"prd_grips", "P-GRIP", "Training Grips", "pctg_equipment", "itm_grips", 199_000, 95_000, 15, "8991234500073", "PAIR", 1},
		{"prd_shaker", "P-SHKR", "Protein Shaker", "pctg_equipment", "itm_shaker", 79_000, 35_000, 10, "8991234500080", "PCS", 1},
		{"prd_bar", "P-BAR", "Protein Bar", "pctg_nutrition", "itm_bar", 35_000, 18_000, 0, "8991234500011", "PCS", 1},
		{"prd_bar_ctn", "P-BAR-CTN", "Protein Bar (carton of 24)", "pctg_nutrition", "itm_bar", 780_000, 18_000, 0, "18991234500018", "CTN", 24},
		{"prd_iso", "P-ISO", "Isotonic Drink", "pctg_nutrition", "itm_iso", 25_000, 12_000, 0, "8991234500028", "PCS", 1},
		{"prd_iso_ctn", "P-ISO-CTN", "Isotonic Drink (carton of 24)", "pctg_nutrition", "itm_iso", 552_000, 12_000, 0, "18991234500025", "CTN", 24},
		{"prd_whey", "P-WHEY", "Whey Protein 1kg", "pctg_nutrition", "itm_whey", 749_000, 480_000, 50, "8991234500035", "PCS", 1},
	}
	for _, p := range products {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.products (id, sku, name, category_id, inventory_item_id,
				price_idr, cost_idr, bonus_xp, barcode, pack_unit, pack_factor)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT (id) DO NOTHING`,
			p.id, p.sku, p.name, p.category, p.item, p.price, p.cost, p.bonusXP,
			p.barcode, p.packUnit, p.packFactor); err != nil {
			return 0, fmt.Errorf("seed: pos product %s: %w", p.id, err)
		}

		// Barcodes and packs arrived after the first demo databases were
		// seeded, and DO NOTHING leaves those rows without them — a shelf full
		// of goods the scanner cannot find. Backfilling only where there is
		// nothing yet fills them in without overwriting anything real.
		if _, err := s.db.Exec(ctx, `
			UPDATE pos.products SET barcode = $2, pack_unit = $3, pack_factor = $4
			WHERE id = $1 AND barcode IS NULL`,
			p.id, p.barcode, p.packUnit, p.packFactor); err != nil {
			return 0, fmt.Errorf("seed: backfilling product %s: %w", p.id, err)
		}
	}

	// Price breaks. Buying a dozen bars gets the case rate without having to
	// buy the case, and a reseller pays less again — the two mechanics that
	// make a counter fast-moving rather than boutique.
	prices := []struct {
		id, product, channel string
		minQty, price        float64
	}{
		{"ppr_bar_12", "prd_bar", "RETAIL", 12, 31_000},
		{"ppr_bar_ws", "prd_bar", "WHOLESALE", 1, 27_500},
		{"ppr_iso_12", "prd_iso", "RETAIL", 12, 22_000},
		{"ppr_iso_ws", "prd_iso", "WHOLESALE", 1, 19_500},
		{"ppr_bar_ctn_ws", "prd_bar_ctn", "WHOLESALE", 1, 660_000},
		{"ppr_iso_ctn_ws", "prd_iso_ctn", "WHOLESALE", 1, 468_000},
		{"ppr_tee_staff", "prd_tee", "STAFF", 1, 120_000},
	}
	for _, p := range prices {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO pos.product_prices (id, product_id, channel, min_qty, price_idr)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`,
			p.id, p.product, p.channel, p.minQty, p.price); err != nil {
			return 0, fmt.Errorf("seed: product price %s: %w", p.id, err)
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
