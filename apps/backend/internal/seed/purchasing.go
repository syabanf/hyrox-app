package seed

import (
	"context"
	"fmt"
)

// seedPurchasing loads the suppliers the studio buys from, and what they
// charge, so a purchase order can be priced without anybody remembering.
func (s *Seeder) seedPurchasing(ctx context.Context) (int, error) {
	suppliers := []struct {
		id, code, name, contact, phone, city, terms, category string
	}{
		{"sup_apparel", "SUP-APR", "Garmen Nusantara", "Dewi Lestari", "+622150001111",
			"Bandung", "NET30", "Apparel"},
		{"sup_nutrition", "SUP-NUT", "Prima Nutrisi Indonesia", "Andre Wijaya", "+622150002222",
			"Jakarta", "NET14", "Nutrition"},
		{"sup_equipment", "SUP-EQP", "Fitline Equipment", "Hendra Santoso", "+622150003333",
			"Tangerang", "NET45", "Equipment"},
		{"sup_supplies", "SUP-SUP", "Bersih Jaya Supplies", "Ratna Sari", "+622150004444",
			"Jakarta", "COD", "Studio Supplies"},
	}
	for _, sup := range suppliers {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.suppliers (id, code, name, contact_name, contact_phone, city,
				payment_terms, category, status, bank_name, bank_account, bank_holder)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'ACTIVE', 'Bank Mandiri', $9, $3)
			ON CONFLICT (id) DO NOTHING`,
			sup.id, sup.code, sup.name, sup.contact, sup.phone, sup.city, sup.terms,
			sup.category, "137000"+sup.code[4:]); err != nil {
			return 0, fmt.Errorf("seed: supplier %s: %w", sup.id, err)
		}
	}

	// What each one charges. The purchase price is below the stock cost the
	// items carry, so a receipt visibly moves the weighted average.
	prices := []struct {
		supplier, item string
		price          float64
		minQty         float64
		leadDays       int
	}{
		{"sup_apparel", "itm_tee", 78_000, 12, 7},
		{"sup_apparel", "itm_tank", 72_000, 12, 7},
		{"sup_nutrition", "itm_bar", 16_500, 48, 3},
		{"sup_nutrition", "itm_iso", 10_500, 48, 3},
		{"sup_nutrition", "itm_whey", 445_000, 6, 10},
		{"sup_equipment", "itm_bottle", 57_000, 12, 14},
		{"sup_equipment", "itm_grips", 88_000, 6, 14},
		{"sup_equipment", "itm_shaker", 31_000, 24, 14},
		{"sup_supplies", "itm_towel", 19_500, 24, 2},
		{"sup_supplies", "itm_wipes", 132_000, 6, 2},
	}
	for i, p := range prices {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.supplier_prices (id, supplier_id, item_id, unit_price_idr,
				min_order_qty, lead_time_days, effective_from)
			VALUES ($1, $2, $3, $4, $5, $6, CURRENT_DATE) ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("lin_price_%02d", i), p.supplier, p.item, p.price, p.minQty, p.leadDays); err != nil {
			return 0, fmt.Errorf("seed: supplier price %s/%s: %w", p.supplier, p.item, err)
		}
	}
	return len(suppliers), nil
}
