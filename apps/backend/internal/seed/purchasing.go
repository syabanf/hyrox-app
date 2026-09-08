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
	// A supplier quotes the pack they ship, not the piece: a carton price for
	// a carton, and the minimum order in cartons too. Dividing that down to a
	// unit cost is the receipt's job, not the price list's.
	prices := []struct {
		supplier, item, unit string
		packFactor           float64
		price                float64
		minQty               float64
		leadDays             int
	}{
		{"sup_apparel", "itm_tee", "CTN", 12, 936_000, 1, 7},
		{"sup_apparel", "itm_tank", "PCS", 1, 72_000, 12, 7},
		{"sup_nutrition", "itm_bar", "CTN", 24, 396_000, 2, 3},
		{"sup_nutrition", "itm_iso", "CTN", 24, 252_000, 2, 3},
		{"sup_nutrition", "itm_whey", "BOX", 6, 2_670_000, 1, 10},
		{"sup_equipment", "itm_bottle", "CTN", 12, 684_000, 1, 14},
		{"sup_equipment", "itm_grips", "PAIR", 1, 88_000, 6, 14},
		{"sup_equipment", "itm_shaker", "CTN", 24, 744_000, 1, 14},
		{"sup_supplies", "itm_towel", "CTN", 20, 390_000, 1, 2},
		{"sup_supplies", "itm_wipes", "TUB", 1, 132_000, 6, 2},
	}
	for i, p := range prices {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.supplier_prices (id, supplier_id, item_id, unit_price_idr,
				unit, pack_factor, min_order_qty, lead_time_days, effective_from)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_DATE) ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("lin_price_%02d", i), p.supplier, p.item, p.price, p.unit,
			p.packFactor, p.minQty, p.leadDays); err != nil {
			return 0, fmt.Errorf("seed: supplier price %s/%s: %w", p.supplier, p.item, err)
		}
	}
	return len(suppliers), nil
}
