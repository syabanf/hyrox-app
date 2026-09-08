package purchasing

import (
	"context"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Orders, deliveries and returns — the half of purchasing that touches stock.

// OrderView is an order with its lines and its supplier named.
type OrderView struct {
	domain.PurchaseOrder
	SupplierName string                `json:"supplierName"`
	BranchName   string                `json:"branchName"`
	Items        []OrderLineView       `json:"items"`
	Receipts     []domain.GoodsReceipt `json:"receipts,omitempty"`
}

// OrderLineView is one ordered line with what is still owed on it.
type OrderLineView struct {
	domain.PurchaseOrderItem
	ItemName       string          `json:"itemName"`
	QtyOutstanding domain.Quantity `json:"qtyOutstanding"`
}

func (s *Service) viewOrder(ctx context.Context, order domain.PurchaseOrder, withReceipts bool) (OrderView, error) {
	items, err := s.repo.OrderItems(ctx, order.ID, false)
	if err != nil {
		return OrderView{}, err
	}
	supplier, err := s.repo.Supplier(ctx, order.SupplierID)
	if err != nil {
		return OrderView{}, err
	}
	branches, err := s.branchNames(ctx)
	if err != nil {
		return OrderView{}, err
	}
	names, err := s.stock.ItemNames(ctx)
	if err != nil {
		return OrderView{}, err
	}

	view := OrderView{
		PurchaseOrder: order, SupplierName: supplier.Name, BranchName: branches[order.BranchID],
	}
	for _, item := range items {
		view.Items = append(view.Items, OrderLineView{
			PurchaseOrderItem: item, ItemName: names[item.ItemID],
			QtyOutstanding: item.QtyOutstanding(),
		})
	}
	if withReceipts {
		receipts, err := s.repo.Receipts(ctx, ReceiptFilter{OrderID: order.ID, Limit: 50})
		if err != nil {
			return OrderView{}, err
		}
		view.Receipts = receipts
	}
	return view, nil
}

func (s *Service) Orders(ctx context.Context, filter OrderFilter) ([]domain.PurchaseOrder, error) {
	return s.repo.Orders(ctx, filter)
}

func (s *Service) Order(ctx context.Context, orderID string) (OrderView, error) {
	order, err := s.repo.Order(ctx, orderID, false)
	if err != nil {
		return OrderView{}, err
	}
	return s.viewOrder(ctx, order, true)
}

// OrderInput opens a purchase order.
type OrderInput struct {
	SupplierID  string
	BranchID    string
	RequestID   *string
	ExpectedOn  *domain.Date
	TaxPercent  float64
	DiscountIDR float64
	Terms       *string
	ShipTo      *string
	Note        *string
}

func (s *Service) CreateOrder(ctx context.Context, in OrderInput, actor Actor) (OrderView, error) {
	supplier, err := s.repo.Supplier(ctx, in.SupplierID)
	if err != nil {
		return OrderView{}, err
	}
	// A blocked supplier is one nobody may order from again. Enforced here
	// rather than only in the dropdown.
	if !supplier.Status.CanOrderFrom() {
		return OrderView{}, httpx.Conflict("SUPPLIER_NOT_ORDERABLE",
			"%s is %s and cannot be ordered from.", supplier.Name, strings.ToLower(string(supplier.Status)))
	}

	taxPercent := in.TaxPercent
	if taxPercent == 0 {
		taxPercent = 11
	}

	created, err := s.repo.InsertOrder(ctx, domain.PurchaseOrder{
		ID: s.ids.New(id.PurchaseOrder), PONumber: s.documentNumber("PO", id.PurchaseOrder),
		SupplierID: in.SupplierID, BranchID: in.BranchID, RequestID: in.RequestID,
		Status: domain.PODraft, OrderedOn: s.today(), ExpectedOn: in.ExpectedOn,
		TaxPercent: taxPercent, DiscountIDR: in.DiscountIDR,
		Terms: in.Terms, ShipTo: in.ShipTo, Note: in.Note,
	})
	if err != nil {
		return OrderView{}, err
	}
	s.record(ctx, "purchasing.order", created.ID, "CREATE", actor, nil)
	return s.viewOrder(ctx, created, false)
}

// OrderLineInput is one line of a commitment.
type OrderLineInput struct {
	ItemID       string
	Description  string
	Qty          domain.Quantity
	Unit         string
	UnitPriceIDR float64
	DiscountIDR  float64
	Note         *string
}

// AddOrderLine adds a line and re-totals. Only a draft order can be changed:
// once it has been approved, the numbers somebody signed stay put.
func (s *Service) AddOrderLine(ctx context.Context, orderID string, in OrderLineInput, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		if order.Status != domain.PODraft {
			return httpx.Conflict("NOT_DRAFT",
				"That order is %s; its lines can no longer be changed.",
				strings.ToLower(string(order.Status)))
		}
		if in.Qty <= 0 {
			return httpx.Invalid("A line needs a positive quantity.")
		}

		// The pack the buyer is ordering in, resolved against what the item is
		// actually stocked in. An empty unit means "however we normally buy
		// it", which is the case that keeps most lines free of a decision.
		pack, err := s.stock.PackFor(ctx, in.ItemID, in.Unit)
		if err != nil {
			return err
		}
		if _, err := s.repo.InsertOrderItem(ctx, domain.PurchaseOrderItem{
			ID: s.ids.New(id.LineItem), OrderID: orderID, ItemID: in.ItemID,
			Description: strings.TrimSpace(in.Description), QtyOrdered: in.Qty,
			Unit: pack.UnitCode, PackFactor: pack.Factor,
			UnitPriceIDR: in.UnitPriceIDR, DiscountIDR: in.DiscountIDR, Note: in.Note,
		}); err != nil {
			return err
		}

		order, err = s.retotalOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, order, false)
		return err
	})
	return view, err
}

// retotalOrder recomputes an order from its lines through the domain, so the
// stored figures and the arithmetic can never disagree.
func (s *Service) retotalOrder(ctx context.Context, order domain.PurchaseOrder) (domain.PurchaseOrder, error) {
	items, err := s.repo.OrderItems(ctx, order.ID, false)
	if err != nil {
		return domain.PurchaseOrder{}, err
	}
	totals := domain.ComputeOrderTotals(items, order.DiscountIDR, order.TaxPercent)
	order.SubtotalIDR = totals.SubtotalIDR
	order.DiscountIDR = totals.DiscountIDR
	order.TaxIDR = totals.TaxIDR
	order.TotalIDR = totals.TotalIDR
	return s.repo.SaveOrder(ctx, order)
}

func (s *Service) RemoveOrderLine(ctx context.Context, orderID, lineID string, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		if order.Status != domain.PODraft {
			return httpx.Conflict("NOT_DRAFT",
				"That order is %s and cannot be changed.", strings.ToLower(string(order.Status)))
		}
		if err := s.repo.DeleteOrderLine(ctx, lineID); err != nil {
			return err
		}
		order, err = s.retotalOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, order, false)
		return err
	})
	return view, err
}

// SetOrderTerms changes the discount, tax and delivery details on a draft.
type OrderTermsInput struct {
	DiscountIDR float64
	TaxPercent  float64
	ExpectedOn  *domain.Date
	Terms       *string
	ShipTo      *string
	Note        *string
}

func (s *Service) SetOrderTerms(ctx context.Context, orderID string, in OrderTermsInput, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		if order.Status != domain.PODraft {
			return httpx.Conflict("NOT_DRAFT",
				"That order is %s and cannot be repriced.", strings.ToLower(string(order.Status)))
		}
		order.DiscountIDR = in.DiscountIDR
		order.TaxPercent = in.TaxPercent
		order.ExpectedOn = in.ExpectedOn
		order.Terms = in.Terms
		order.ShipTo = in.ShipTo
		order.Note = in.Note

		order, err = s.retotalOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, order, false)
		return err
	})
	return view, err
}

// ApproveOrder commits the studio to the money.
func (s *Service) ApproveOrder(ctx context.Context, orderID string, actor Actor) (OrderView, error) {
	return s.moveOrder(ctx, orderID, domain.POApproved, "APPROVE", "", actor)
}

// SendOrder marks the order as placed with the supplier, and tells stock that
// the goods are on their way so the reorder screen stops asking for them.
func (s *Service) SendOrder(ctx context.Context, orderID string, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.PurchaseOrderTransitions, order.Status, domain.POSent)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That order is %s and cannot be sent.", strings.ToLower(string(order.Status)))
		}

		items, err := s.repo.OrderItems(ctx, orderID, false)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return httpx.Conflict("NO_LINES", "That order has nothing on it.")
		}
		for _, item := range items {
			// On-order is a stock figure, so it is in base units: ordering
			// ten cartons means 240 pieces are coming.
			outstanding := domain.PackToBase(item.QtyOutstanding(), item.PackFactor)
			if err := s.stock.ReserveOnOrder(ctx, item.ItemID, order.BranchID, outstanding); err != nil {
				return err
			}
		}

		now := s.clock.Now()
		order.Status = next
		order.SentAt = &now
		saved, err := s.repo.SaveOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, saved, false)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	s.record(ctx, "purchasing.order", orderID, "SEND", actor, nil)
	return view, nil
}

// CancelOrder abandons an order that has not been delivered against. Once
// anything has arrived it cannot be cancelled: stock has moved, and the ledger
// would be left pointing at a document claiming nothing was ordered.
func (s *Service) CancelOrder(ctx context.Context, orderID, reason string, actor Actor) (OrderView, error) {
	if strings.TrimSpace(reason) == "" {
		return OrderView{}, httpx.Invalid("A cancellation needs a reason.")
	}
	return s.moveOrder(ctx, orderID, domain.POCancelled, "CANCEL", reason, actor)
}

func (s *Service) moveOrder(ctx context.Context, orderID string, target domain.PurchaseOrderStatus,
	action, reason string, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		wasSent := order.Status == domain.POSent

		next, err := domain.Transition(domain.PurchaseOrderTransitions, order.Status, target)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That order is %s and cannot become %s.",
				strings.ToLower(string(order.Status)), strings.ToLower(string(target)))
		}

		now := s.clock.Now()
		order.Status = next
		switch target {
		case domain.POApproved:
			order.ApprovedBy, order.ApprovedAt = &actor.ID, &now
		case domain.POCancelled:
			order.CancelledBy, order.CancelledAt = &actor.ID, &now
			order.CancellationReason = &reason
			// Goods that will now never arrive stop being on order.
			if wasSent {
				items, err := s.repo.OrderItems(ctx, orderID, false)
				if err != nil {
					return err
				}
				for _, item := range items {
					if err := s.stock.ReleaseOnOrder(ctx, item.ItemID, order.BranchID, item.QtyOutstanding()); err != nil {
						return err
					}
				}
			}
		}

		saved, err := s.repo.SaveOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, saved, false)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	var note *string
	if reason != "" {
		note = &reason
	}
	s.record(ctx, "purchasing.order", orderID, action, actor, note)
	return view, nil
}

// ConvertRequest turns an approved request into a draft order against a
// supplier, carrying every line across at its estimated price.
func (s *Service) ConvertRequest(ctx context.Context, requestID, supplierID string, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.Request(ctx, requestID, true)
		if err != nil {
			return err
		}
		if request.Status != domain.PRApproved {
			return httpx.Conflict("NOT_APPROVED",
				"That request is %s; only an approved one becomes an order.",
				strings.ToLower(string(request.Status)))
		}
		items, err := s.repo.RequestItems(ctx, requestID)
		if err != nil {
			return err
		}

		order, err := s.CreateOrder(ctx, OrderInput{
			SupplierID: supplierID, BranchID: request.BranchID, RequestID: &request.ID,
			ExpectedOn: request.RequiredOn, TaxPercent: 11,
		}, actor)
		if err != nil {
			return err
		}

		for _, item := range items {
			// A line asking for something not in the catalogue cannot be
			// ordered as stock; it needs an item first.
			if item.ItemID == nil {
				return httpx.Conflict("ITEM_NOT_IN_CATALOGUE",
					"%q is not a catalogue item yet, so it cannot be ordered.", item.Description)
			}
			if _, err := s.repo.InsertOrderItem(ctx, domain.PurchaseOrderItem{
				ID: s.ids.New(id.LineItem), OrderID: order.ID, ItemID: *item.ItemID,
				Description: item.Description, QtyOrdered: item.Qty, Unit: item.Unit,
				UnitPriceIDR: item.EstimatedPriceIDR,
			}); err != nil {
				return err
			}
		}

		placed, err := s.repo.Order(ctx, order.ID, true)
		if err != nil {
			return err
		}
		placed, err = s.retotalOrder(ctx, placed)
		if err != nil {
			return err
		}

		next, err := domain.Transition(domain.PurchaseRequestTransitions, request.Status, domain.PRConverted)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "That request cannot be converted.")
		}
		request.Status = next
		request.ConvertedPOID = &placed.ID
		if _, err := s.repo.SaveRequest(ctx, request); err != nil {
			return err
		}

		view, err = s.viewOrder(ctx, placed, false)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	s.record(ctx, "purchasing.request", requestID, "CONVERT", actor, nil)
	return view, nil
}
