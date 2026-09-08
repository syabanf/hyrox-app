package inventory

import (
	"context"
	"fmt"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// The catalogue as a spreadsheet.
//
// This is how a catalogue actually arrives: three hundred rows in a file
// somebody exported from whatever they used before. Typing them in is not a
// migration plan.

// ImportItems reads a catalogue upload.
//
// It runs as a dry run unless told otherwise, because a three-hundred-row
// mistake is not something anybody wants to find afterwards. A bad row is
// reported and skipped rather than aborting: a file with one broken line
// should not lose the other two hundred and ninety-nine.
func (s *Service) ImportItems(ctx context.Context, rows []httpx.CSVRow, apply bool, actor Actor) (httpx.ImportResult, error) {
	result := httpx.ImportResult{DryRun: !apply, Rows: len(rows), Failures: []httpx.ImportError{}}

	categories, err := s.repo.Categories(ctx)
	if err != nil {
		return result, err
	}
	byCode := map[string]string{}
	for _, category := range categories {
		byCode[strings.ToUpper(category.Code)] = category.ID
		byCode[strings.ToUpper(category.Name)] = category.ID
	}

	existing, err := s.repo.Items(ctx, ItemFilter{})
	if err != nil {
		return result, err
	}
	bySKU := map[string]domain.InventoryItem{}
	for _, item := range existing {
		bySKU[strings.ToUpper(item.SKU)] = item
	}

	for _, row := range rows {
		sku := strings.ToUpper(row.Get("sku"))
		name := row.Get("name")
		if sku == "" || name == "" {
			result.Fail(row.Line, sku, "a row needs a SKU and a name")
			continue
		}

		cost, err := row.Number("cost")
		if err != nil {
			result.Fail(row.Line, sku, fmt.Sprintf("cost: %v", err))
			continue
		}

		var categoryID *string
		if code := strings.ToUpper(row.Get("category")); code != "" {
			found, ok := byCode[code]
			if !ok {
				// Naming a category that does not exist is a typo far more
				// often than it is a new category, so it is reported rather
				// than quietly created.
				result.Fail(row.Line, sku, fmt.Sprintf("no category %q", row.Get("category")))
				continue
			}
			categoryID = &found
		}

		unit := strings.ToUpper(row.Get("unit"))
		if unit == "" {
			unit = "PCS"
		}
		kind := domain.ItemKind(strings.ToUpper(row.Get("kind")))
		if kind == "" {
			kind = domain.ItemRetail
		}

		current, updating := bySKU[sku]
		if !apply {
			if updating {
				result.Updated++
			} else {
				result.Created++
			}
			continue
		}

		item := domain.InventoryItem{
			ID: current.ID, SKU: sku, Name: name, Description: row.Get("description"),
			CategoryID: categoryID, Unit: unit, Kind: kind,
			TrackStock:        !row.Has("trackstock") || row.Bool("trackstock"),
			TrackBatches:      row.Bool("trackbatches"),
			ExpiryWarningDays: 30, Active: !row.Has("active") || row.Bool("active"),
		}
		if barcode := row.Get("barcode"); barcode != "" {
			item.Barcode = &barcode
		}

		if updating {
			// The cost is not imported over: it is the weighted average of what
			// the stock on hand actually cost, and a spreadsheet cannot know
			// that. A new item may start somewhere, an existing one may not.
			item.UnitCostIDR = current.UnitCostIDR
			if _, err := s.repo.UpdateItem(ctx, item); err != nil {
				result.Fail(row.Line, sku, err.Error())
				continue
			}
			result.Updated++
			continue
		}

		item.ID = s.ids.New(id.InventoryItem)
		item.UnitCostIDR = cost
		if err := s.db.InTx(ctx, func(ctx context.Context) error {
			created, err := s.repo.InsertItem(ctx, item)
			if err != nil {
				return err
			}
			// Every item needs a base pack, or its quantities mean nothing.
			_, err = s.repo.UpsertPack(ctx, domain.ItemPack{
				ID: s.ids.New(id.ItemPack), ItemID: created.ID, UnitCode: created.Unit,
				Factor: 1, Barcode: created.Barcode, IsBase: true,
				PurchaseDefault: true, SaleDefault: true, Active: true,
			})
			return err
		}); err != nil {
			result.Fail(row.Line, sku, err.Error())
			continue
		}
		result.Created++
	}

	if apply {
		s.record(ctx, "inventory.item", "import", "IMPORT", actor, nil)
	}
	return result, nil
}

// ExportItems writes the catalogue back out, in the shape the importer reads.
//
// The two match on purpose: exporting, editing in a spreadsheet and importing
// again is the way people actually do bulk edits, and a format that only goes
// one way turns that into a manual re-typing exercise.
func (s *Service) ExportItems(ctx context.Context) ([]string, [][]string, error) {
	items, err := s.repo.Items(ctx, ItemFilter{})
	if err != nil {
		return nil, nil, err
	}
	categories, err := s.repo.Categories(ctx)
	if err != nil {
		return nil, nil, err
	}
	codes := map[string]string{}
	for _, category := range categories {
		codes[category.ID] = category.Code
	}

	header := []string{"sku", "name", "description", "category", "unit", "kind",
		"cost", "barcode", "trackStock", "trackBatches", "active"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		category := ""
		if item.CategoryID != nil {
			category = codes[*item.CategoryID]
		}
		barcode := ""
		if item.Barcode != nil {
			barcode = *item.Barcode
		}
		rows = append(rows, []string{
			item.SKU, item.Name, item.Description, category, item.Unit, string(item.Kind),
			fmt.Sprintf("%.2f", item.UnitCostIDR), barcode,
			boolText(item.TrackStock), boolText(item.TrackBatches), boolText(item.Active),
		})
	}
	return header, rows, nil
}

// ExportStock writes what is on every shelf, valued.
func (s *Service) ExportStock(ctx context.Context, branchID string) ([]string, [][]string, error) {
	rows, err := s.Stock(ctx, LevelFilter{BranchID: branchID, Limit: 5000})
	if err != nil {
		return nil, nil, err
	}
	branches, err := s.branchNames(ctx)
	if err != nil {
		return nil, nil, err
	}

	header := []string{"sku", "name", "branch", "unit", "onHand", "onOrder",
		"minimum", "unitCost", "value"}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, []string{
			row.Item.SKU, row.Item.Name, branches[row.Level.BranchID], row.Item.Unit,
			fmt.Sprintf("%v", row.Level.QtyOnHand), fmt.Sprintf("%v", row.Level.QtyOnOrder),
			fmt.Sprintf("%v", row.Level.QtyMinimum),
			fmt.Sprintf("%.2f", row.Item.UnitCostIDR),
			fmt.Sprintf("%.2f", float64(row.Level.QtyOnHand)*row.Item.UnitCostIDR),
		})
	}
	return header, out, nil
}

func boolText(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
