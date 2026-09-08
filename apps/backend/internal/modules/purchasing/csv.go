package purchasing

import (
	"context"
	"fmt"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// Suppliers and price lists, as spreadsheets.
//
// A supplier's price list arrives as a file about once a quarter, and retyping
// forty lines of it is how a stale price ends up on a purchase order.

// ImportSuppliers reads a supplier upload.
func (s *Service) ImportSuppliers(ctx context.Context, rows []httpx.CSVRow, apply bool, actor Actor) (httpx.ImportResult, error) {
	result := httpx.ImportResult{DryRun: !apply, Rows: len(rows), Failures: []httpx.ImportError{}}

	existing, err := s.repo.Suppliers(ctx, SupplierFilter{Limit: 5000})
	if err != nil {
		return result, err
	}
	byCode := map[string]domain.Supplier{}
	for _, supplier := range existing {
		byCode[strings.ToUpper(supplier.Code)] = supplier
	}

	for _, row := range rows {
		code := strings.ToUpper(row.Get("code"))
		name := row.Get("name")
		if code == "" || name == "" {
			result.Fail(row.Line, code, "a row needs a code and a name")
			continue
		}

		terms := domain.PaymentTerms(strings.ToUpper(row.Get("terms")))
		if terms == "" {
			terms = domain.TermsNet30
		}
		if !domain.IsValidPaymentTerms(string(terms)) {
			result.Fail(row.Line, code, fmt.Sprintf("%q is not a payment term", row.Get("terms")))
			continue
		}

		current, updating := byCode[code]
		if !apply {
			if updating {
				result.Updated++
			} else {
				result.Created++
			}
			continue
		}

		supplier := domain.Supplier{
			ID: current.ID, Code: code, Name: name,
			City: optionalText(row.Get("city")), PaymentTerms: terms,
			Category: optionalText(row.Get("category")),
			Status:   domain.SupplierActive,
		}
		if supplier.ID == "" {
			supplier.ID = s.ids.New(id.Supplier)
		}
		if contact := row.Get("contact"); contact != "" {
			supplier.ContactName = &contact
		}
		if phone := row.Get("phone"); phone != "" {
			supplier.ContactPhone = &phone
		}
		if email := row.Get("email"); email != "" {
			supplier.Email = &email
		}
		if updating {
			// A blocked supplier stays blocked: somebody blocked them for a
			// reason, and a spreadsheet is not that reason being reversed.
			supplier.Status = current.Status
		}

		if updating {
			if _, err := s.repo.UpdateSupplier(ctx, supplier); err != nil {
				result.Fail(row.Line, code, err.Error())
				continue
			}
			result.Updated++
			continue
		}
		if _, err := s.repo.InsertSupplier(ctx, supplier); err != nil {
			result.Fail(row.Line, code, err.Error())
			continue
		}
		result.Created++
	}

	if apply {
		s.record(ctx, "purchasing.supplier", "import", "IMPORT", actor, nil)
	}
	return result, nil
}

// ImportPrices reads a supplier's price list.
//
// Prices are per pack, because that is how a supplier quotes: a carton price
// on a carton line. The unit has to be one the item is actually stocked in —
// a price list naming a pack nobody has defined would put a number against a
// quantity that means nothing.
func (s *Service) ImportPrices(ctx context.Context, supplierID string,
	rows []httpx.CSVRow, apply bool, actor Actor) (httpx.ImportResult, error) {

	result := httpx.ImportResult{DryRun: !apply, Rows: len(rows), Failures: []httpx.ImportError{}}

	supplier, err := s.repo.Supplier(ctx, supplierID)
	if err != nil {
		return result, err
	}
	names, err := s.stock.ItemNames(ctx)
	if err != nil {
		return result, err
	}

	for _, row := range rows {
		sku := strings.ToUpper(row.Get("sku"))
		if sku == "" {
			result.Fail(row.Line, "", "a row needs a SKU")
			continue
		}
		itemID, err := s.stock.ItemIDBySKU(ctx, sku)
		if err != nil {
			result.Fail(row.Line, sku, "no item with that SKU")
			continue
		}

		price, err := row.Number("price")
		if err != nil || price <= 0 {
			result.Fail(row.Line, sku, "price must be a positive number")
			continue
		}
		minQty, _ := row.Number("minOrder")
		leadDays, _ := row.Number("leadDays")

		pack, err := s.stock.PackFor(ctx, itemID, row.Get("unit"))
		if err != nil {
			result.Fail(row.Line, sku, err.Error())
			continue
		}

		if !apply {
			result.Created++
			continue
		}
		if _, err := s.repo.UpsertSupplierPrice(ctx, domain.SupplierPrice{
			ID: s.ids.New(id.LineItem), SupplierID: supplier.ID, ItemID: itemID,
			UnitPriceIDR: price, Unit: pack.UnitCode, PackFactor: pack.Factor,
			MinOrderQty: domain.Quantity(minQty), LeadTimeDays: int(leadDays),
			EffectiveFrom: domain.DateOf(s.clock.Now(), s.studio), Active: true,
		}); err != nil {
			result.Fail(row.Line, sku, err.Error())
			continue
		}
		_ = names
		result.Created++
	}

	if apply {
		s.record(ctx, "purchasing.supplier", supplierID, "IMPORT_PRICES", actor, nil)
	}
	return result, nil
}

// ExportSuppliers writes suppliers in the shape the importer reads.
func (s *Service) ExportSuppliers(ctx context.Context) ([]string, [][]string, error) {
	suppliers, err := s.repo.Suppliers(ctx, SupplierFilter{Limit: 5000})
	if err != nil {
		return nil, nil, err
	}

	header := []string{"code", "name", "contact", "phone", "email", "city",
		"terms", "category", "status"}
	rows := make([][]string, 0, len(suppliers))
	for _, supplier := range suppliers {
		rows = append(rows, []string{
			supplier.Code, supplier.Name, text(supplier.ContactName), text(supplier.ContactPhone),
			text(supplier.Email), text(supplier.City), string(supplier.PaymentTerms),
			text(supplier.Category), string(supplier.Status),
		})
	}
	return header, rows, nil
}

// ExportOrders writes purchase orders for a window, one row per line, because
// that is the shape a spreadsheet can actually pivot.
func (s *Service) ExportOrders(ctx context.Context, window ReportRange) ([]string, [][]string, error) {
	orders, err := s.repo.Orders(ctx, OrderFilter{
		BranchID: window.BranchID, SupplierID: window.SupplierID, Limit: 2000,
	})
	if err != nil {
		return nil, nil, err
	}
	names, err := s.stock.ItemNames(ctx)
	if err != nil {
		return nil, nil, err
	}

	header := []string{"poNumber", "orderedOn", "supplier", "status", "item",
		"qtyOrdered", "unit", "qtyReceived", "unitPrice", "lineTotal"}
	rows := [][]string{}
	for _, order := range orders {
		items, err := s.repo.OrderItems(ctx, order.ID, false)
		if err != nil {
			return nil, nil, err
		}
		supplier, err := s.repo.Supplier(ctx, order.SupplierID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range items {
			rows = append(rows, []string{
				order.PONumber, string(order.OrderedOn), supplier.Name, string(order.Status),
				names[item.ItemID], fmt.Sprintf("%v", item.QtyOrdered), item.Unit,
				fmt.Sprintf("%v", item.QtyReceived),
				fmt.Sprintf("%.2f", item.UnitPriceIDR),
				fmt.Sprintf("%.2f", item.SubtotalIDR),
			})
		}
	}
	return header, rows, nil
}

func text(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func optionalText(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}
