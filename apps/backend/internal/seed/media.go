package seed

import (
	"context"
	"fmt"

	"github.com/syabanf/nuhabit-backend/internal/media"
)

// Pictures for the demo rows.
//
// They are assigned here, in one place, rather than threaded through every
// seed struct: an image is a property of the demo, not of the thing being
// seeded, and a studio loading its own data has no use for any of this.
//
// Every path points at an image embedded in the binary (see internal/media),
// so a demo with no network still has faces, shelves and race cards on it.

// pictures maps a demo row to its image file, per table.
type pictures struct {
	table  string
	column string
	byID   map[string]string
}

func demoPictures() []pictures {
	return []pictures{
		{"identity.members", "avatar_url", map[string]string{
			"mem_demo":    "avatar-fahmi.svg",
			"mem_natalie": "avatar-natalie.svg",
			"mem_lucas":   "avatar-lucas.svg",
			"mem_jaime":   "avatar-jaime.svg",
			"mem_doris":   "avatar-doris.svg",
			"mem_luke":    "avatar-luke.svg",
			"mem_sari":    "avatar-sari.svg",
			"mem_dimas":   "avatar-dimas.svg",
			"mem_rina":    "avatar-rina.svg",
		}},
		{"hris.employees", "photo_url", map[string]string{
			"emp_alya":  "avatar-alya.svg",
			"emp_raka":  "avatar-raka.svg",
			"emp_bima":  "avatar-bima.svg",
			"emp_nadia": "avatar-nadia.svg",
			"emp_kevin": "avatar-kevin.svg",
			"emp_sinta": "avatar-sinta.svg",
			"emp_maya":  "avatar-maya.svg",
			"emp_rizky": "avatar-rizky.svg",
			"emp_tara":  "avatar-tara.svg",
		}},
		{"training.race_events", "image_url", map[string]string{
			"rce_jakarta":      "race-jakarta.svg",
			"rce_jakarta_past": "race-jakarta.svg",
			"rce_singapore":    "race-singapore.svg",
			"rce_bangkok":      "race-bangkok.svg",
			"rce_sydney":       "race-sydney.svg",
			"rce_london":       "race-london.svg",
		}},
		{"inventory.items", "image_url", map[string]string{
			"itm_tee":    "item-tee.svg",
			"itm_tank":   "item-tank.svg",
			"itm_bottle": "item-bottle.svg",
			"itm_grips":  "item-grips.svg",
			"itm_shaker": "item-shaker.svg",
			"itm_bar":    "item-bar.svg",
			"itm_iso":    "item-iso.svg",
			"itm_whey":   "item-whey.svg",
			"itm_towel":  "item-towel.svg",
			"itm_wipes":  "item-wipes.svg",
		}},
		{"crm.rewards", "image_url", map[string]string{
			"rwd_shaker": "reward-shaker.svg",
			"rwd_bar":    "reward-bar.svg",
			"rwd_class":  "reward-class.svg",
			"rwd_tee":    "reward-tee.svg",
			"rwd_month":  "reward-month.svg",
		}},
	}
}

func (s *Seeder) seedMedia(ctx context.Context) (int, error) {
	assigned := 0
	for _, set := range demoPictures() {
		for rowID, file := range set.byID {
			// Only fills a gap: a picture somebody uploaded is theirs, and a
			// re-seed must not overwrite it.
			tag, err := s.db.Exec(ctx, fmt.Sprintf(
				`UPDATE %s SET %s = $2 WHERE id = $1 AND %s IS NULL`,
				set.table, set.column, set.column), rowID, media.URL(file))
			if err != nil {
				return 0, fmt.Errorf("seed: picture for %s %s: %w", set.table, rowID, err)
			}
			assigned += int(tag.RowsAffected())
		}
	}

	// The till sells packs of the shelf's items, so a product shows the
	// picture of the item behind it rather than needing one of its own.
	tag, err := s.db.Exec(ctx, `
		UPDATE pos.products p SET image_url = i.image_url
		FROM inventory.items i
		WHERE p.inventory_item_id = i.id AND p.image_url IS NULL AND i.image_url IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("seed: product pictures: %w", err)
	}
	assigned += int(tag.RowsAffected())

	return assigned, nil
}
