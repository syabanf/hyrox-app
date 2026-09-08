package purchasing

import (
	"context"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// Receiving: the one place purchasing changes what is on a shelf.

// ReceiptView is a delivery with its lines named.
type ReceiptView struct {
	domain.GoodsReceipt
	SupplierName string            `json:"supplierName"`
	BranchName   string            `json:"branchName"`
	PONumber     string            `json:"poNumber"`
	Items        []ReceiptLineView `json:"items"`
}

// ReceiptLineView is one delivered line against what was ordered.
type ReceiptLineView struct {
	domain.GoodsReceiptItem
	ItemName      string          `json:"itemName"`
	QtyOrdered    domain.Quantity `json:"qtyOrdered"`
	QtyReturnable domain.Quantity `json:"qtyReturnable"`
}

func (s *Service) viewReceipt(ctx context.Context, receipt domain.GoodsReceipt) (ReceiptView, error) {
	items, err := s.repo.ReceiptItems(ctx, receipt.ID)
	if err != nil {
		return ReceiptView{}, err
	}
	order, err := s.repo.Order(ctx, receipt.OrderID, false)
	if err != nil {
		return ReceiptView{}, err
	}
	orderItems, err := s.repo.OrderItems(ctx, receipt.OrderID, false)
	if err != nil {
		return ReceiptView{}, err
	}
	ordered := map[string]domain.Quantity{}
	for _, item := range orderItems {
		ordered[item.ID] = item.QtyOrdered
	}
	supplier, err := s.repo.Supplier(ctx, receipt.SupplierID)
	if err != nil {
		return ReceiptView{}, err
	}
	branches, err := s.branchNames(ctx)
	if err != nil {
		return ReceiptView{}, err
	}
	names, err := s.stock.ItemNames(ctx)
	if err != nil {
		return ReceiptView{}, err
	}

	view := ReceiptView{
		GoodsReceipt: receipt, SupplierName: supplier.Name,
		BranchName: branches[receipt.BranchID], PONumber: order.PONumber,
	}
	for _, item := range items {
		view.Items = append(view.Items, ReceiptLineView{
			GoodsReceiptItem: item, ItemName: names[item.ItemID],
			QtyOrdered:    ordered[item.OrderItemID],
			QtyReturnable: item.QtyReturnable(),
		})
	}
	return view, nil
}

func (s *Service) Receipts(ctx context.Context, filter ReceiptFilter) ([]domain.GoodsReceipt, error) {
	return s.repo.Receipts(ctx, filter)
}

func (s *Service) Receipt(ctx context.Context, receiptID string) (ReceiptView, error) {
	receipt, err := s.repo.Receipt(ctx, receiptID, false)
	if err != nil {
		return ReceiptView{}, err
	}
	return s.viewReceipt(ctx, receipt)
}

// OpenReceipt starts a delivery note against an order.
func (s *Service) OpenReceipt(ctx context.Context, orderID string, deliveryNote *string,
	deliveryID *string, note *string, actor Actor) (ReceiptView, error) {

	order, err := s.repo.Order(ctx, orderID, false)
	if err != nil {
		return ReceiptView{}, err
	}
	// A receipt may inspect a recorded arrival, or be raised straight off the
	// order — the small delivery somebody checked as it came in is still a
	// legitimate way to work.
	if deliveryID != nil && *deliveryID != "" {
		delivery, err := s.repo.Delivery(ctx, *deliveryID, false)
		if err != nil {
			return ReceiptView{}, err
		}
		if delivery.OrderID != orderID {
			return ReceiptView{}, httpx.Invalid("That delivery arrived against a different order.")
		}
		if delivery.Status != domain.DeliveryArrived {
			return ReceiptView{}, httpx.Conflict("DELIVERY_CLOSED",
				"That delivery is %s and has already been inspected.",
				strings.ToLower(string(delivery.Status)))
		}
	}
	switch order.Status {
	case domain.POApproved, domain.POSent, domain.POPartiallyReceived:
	default:
		return ReceiptView{}, httpx.Conflict("ORDER_NOT_OPEN",
			"That order is %s; nothing can be received against it.",
			strings.ToLower(string(order.Status)))
	}

	created, err := s.repo.InsertReceipt(ctx, domain.GoodsReceipt{
		ID: s.ids.New(id.GoodsReceipt), GRNNumber: s.documentNumber("GRN", id.GoodsReceipt),
		OrderID: orderID, SupplierID: order.SupplierID, BranchID: order.BranchID,
		ReceivedOn: s.today(), ReceivedBy: &actor.ID, ReceivedByName: &actor.Name,
		DeliveryNoteNumber: deliveryNote, DeliveryID: deliveryID,
		Status: domain.GRNDraft, Note: note,
	})
	if err != nil {
		return ReceiptView{}, err
	}
	s.record(ctx, "purchasing.receipt", created.ID, "OPEN", actor, nil)
	return s.viewReceipt(ctx, created)
}

// ReceiveLineInput is one line of what turned up.
type ReceiveLineInput struct {
	OrderItemID string
	QtyAccepted domain.Quantity
	// QtyRejected failed inspection. It is recorded and never enters stock,
	// which is why the two numbers are separate.
	QtyRejected domain.Quantity
	BatchNumber *string
	ExpiresOn   *domain.Date
	Note        *string
}

// AddReceiptLine records one delivered line on a draft receipt.
func (s *Service) AddReceiptLine(ctx context.Context, receiptID string, in ReceiveLineInput, actor Actor) (ReceiptView, error) {
	var view ReceiptView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		receipt, err := s.repo.Receipt(ctx, receiptID, true)
		if err != nil {
			return err
		}
		if receipt.Status != domain.GRNDraft {
			return httpx.Conflict("NOT_DRAFT",
				"That receipt is %s and cannot be changed.", strings.ToLower(string(receipt.Status)))
		}

		order, err := s.repo.Order(ctx, receipt.OrderID, false)
		if err != nil {
			return err
		}
		line, err := s.repo.OrderItem(ctx, in.OrderItemID, false)
		if err != nil {
			return err
		}
		if line.OrderID != receipt.OrderID {
			return httpx.Invalid("That line belongs to a different order.")
		}

		// Everything already on this draft counts towards the ordered
		// quantity, so two lines for the same item cannot together overshoot.
		existing, err := s.repo.ReceiptItems(ctx, receiptID)
		if err != nil {
			return err
		}
		for _, e := range existing {
			if e.OrderItemID == in.OrderItemID {
				line.QtyReceived += e.QtyAccepted
			}
		}

		if rejection := domain.EvaluateReceiptLine(order, line, in.QtyAccepted, in.QtyRejected); rejection != "" {
			return receiptRejectionError(rejection, line)
		}

		if _, err := s.repo.InsertReceiptItem(ctx, domain.GoodsReceiptItem{
			ID: s.ids.New(id.LineItem), ReceiptID: receiptID, OrderItemID: in.OrderItemID,
			ItemID: line.ItemID, QtyAccepted: in.QtyAccepted, QtyRejected: in.QtyRejected,
			// The pack comes from the order line rather than the request: what
			// was ordered in cartons arrives in cartons, and letting the
			// receipt choose its own unit is exactly the gap a conversion bug
			// gets in through.
			Unit: line.Unit, PackFactor: line.PackFactor,
			UnitPriceIDR: line.UnitPriceIDR,
			QCStatus:     domain.DeriveQCStatus(in.QtyAccepted, in.QtyRejected),
			BatchNumber:  in.BatchNumber, ExpiresOn: in.ExpiresOn, Note: in.Note,
		}); err != nil {
			return err
		}

		view, err = s.viewReceipt(ctx, receipt)
		return err
	})
	return view, err
}

func receiptRejectionError(rejection domain.ReceiptRejection, line domain.PurchaseOrderItem) error {
	switch rejection {
	case domain.ReceiptRejectOverDelivered:
		return httpx.Conflict("OVER_DELIVERED",
			"Only %v of that line is still outstanding.", line.QtyOutstanding())
	case domain.ReceiptRejectNothingArrived:
		return httpx.Invalid("A delivery line needs something on it, accepted or rejected.")
	case domain.ReceiptRejectOrderNotOpen:
		return httpx.Conflict("ORDER_NOT_OPEN", "That order is not open for delivery.")
	default:
		return httpx.Invalid("That delivery line cannot be recorded.")
	}
}

func (s *Service) RemoveReceiptLine(ctx context.Context, receiptID, lineID string, actor Actor) (ReceiptView, error) {
	var view ReceiptView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		receipt, err := s.repo.Receipt(ctx, receiptID, true)
		if err != nil {
			return err
		}
		if receipt.Status != domain.GRNDraft {
			return httpx.Conflict("NOT_DRAFT",
				"That receipt is %s and cannot be changed.", strings.ToLower(string(receipt.Status)))
		}
		if err := s.repo.DeleteReceiptItem(ctx, lineID); err != nil {
			return err
		}
		view, err = s.viewReceipt(ctx, receipt)
		return err
	})
	return view, err
}

// PostReceipt books the delivery in.
//
// This is the crossing point: accepted quantities become stock movements at
// the price on the order, the order's lines and status catch up, and the
// goods stop counting as on order. All of it in one transaction, so a delivery
// can never be half-recorded.
func (s *Service) PostReceipt(ctx context.Context, receiptID string, actor Actor) (ReceiptView, error) {
	var view ReceiptView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		receipt, err := s.repo.Receipt(ctx, receiptID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.GoodsReceiptTransitions, receipt.Status, domain.GRNPosted)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That receipt is %s and cannot be posted.", strings.ToLower(string(receipt.Status)))
		}

		lines, err := s.repo.ReceiptItems(ctx, receiptID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return httpx.Conflict("NO_LINES", "That receipt has nothing on it.")
		}

		order, err := s.repo.Order(ctx, receipt.OrderID, true)
		if err != nil {
			return err
		}
		for _, line := range lines {
			// The batch travels from the delivery note straight onto the
			// shelf. Capturing it on the receipt and then not carrying it
			// through would leave dated goods indistinguishable once they are
			// in stock, which is the entire problem batches solve.
			batchCode := ""
			if line.BatchNumber != nil {
				batchCode = *line.BatchNumber
			}
			ref := StockRef{Type: "GOODS_RECEIPT", ID: receipt.ID, Number: receipt.GRNNumber,
				PackUnit: line.Unit, PackFactor: line.PackFactor,
				BatchCode: batchCode, ExpiresOn: line.ExpiresOn}
			if line.QtyAccepted > 0 {
				// Ten cartons delivered is 240 pieces on the shelf, at the
				// carton price divided by 24. Both conversions happen here,
				// once, because a receipt that multiplies the quantity but
				// not the price is how an average cost silently inflates by
				// the size of the pack.
				if err := s.stock.Receive(ctx, line.ItemID, receipt.BranchID,
					line.BaseAccepted(), line.UnitPriceBaseIDR(), ref, s.stockActor(actor)); err != nil {
					return err
				}
				// The order line is counted in the pack it was ordered in, so
				// this one is not converted.
				if err := s.repo.AddReceivedQty(ctx, line.OrderItemID, line.QtyAccepted); err != nil {
					return err
				}
				if err := s.stock.ReleaseOnOrder(ctx, line.ItemID, receipt.BranchID, line.BaseAccepted()); err != nil {
					return err
				}
			}
			// Rejected goods are recorded and never enter stock. They are also
			// no longer expected, so they stop being on order.
			if line.QtyRejected > 0 {
				if err := s.stock.ReleaseOnOrder(ctx, line.ItemID, receipt.BranchID, line.BaseRejected()); err != nil {
					return err
				}
			}
		}

		items, err := s.repo.OrderItems(ctx, order.ID, false)
		if err != nil {
			return err
		}
		outcome := domain.ApplyReceipt(items)
		if outcome.Status != order.Status {
			moved, err := domain.Transition(domain.PurchaseOrderTransitions, order.Status, outcome.Status)
			if err == nil {
				order.Status = moved
				if _, err := s.repo.SaveOrder(ctx, order); err != nil {
					return err
				}
			}
		}

		now := s.clock.Now()
		receipt.Status = next
		receipt.PostedAt = &now
		saved, err := s.repo.SaveReceipt(ctx, receipt)
		if err != nil {
			return err
		}
		view, err = s.viewReceipt(ctx, saved)
		return err
	})
	if err != nil {
		return ReceiptView{}, err
	}
	s.record(ctx, "purchasing.receipt", receiptID, "POST", actor, nil)
	return view, nil
}

// CancelReceipt abandons a draft delivery note. A posted one cannot be
// cancelled: it has written append-only stock movements, and the way to undo
// it is a purchase return.
func (s *Service) CancelReceipt(ctx context.Context, receiptID string, actor Actor) (ReceiptView, error) {
	receipt, err := s.repo.Receipt(ctx, receiptID, false)
	if err != nil {
		return ReceiptView{}, err
	}
	next, err := domain.Transition(domain.GoodsReceiptTransitions, receipt.Status, domain.GRNCancelled)
	if err != nil {
		return ReceiptView{}, httpx.Conflict("INVALID_TRANSITION",
			"That receipt is %s; a posted one is undone with a return, not a cancellation.",
			strings.ToLower(string(receipt.Status)))
	}
	receipt.Status = next
	saved, err := s.repo.SaveReceipt(ctx, receipt)
	if err != nil {
		return ReceiptView{}, err
	}
	s.record(ctx, "purchasing.receipt", receiptID, "CANCEL", actor, nil)
	return s.viewReceipt(ctx, saved)
}

// ── Returns ──────────────────────────────────────────────────────────────────

// ReturnView is goods going back, with its lines named.
type ReturnView struct {
	domain.PurchaseReturn
	SupplierName string           `json:"supplierName"`
	GRNNumber    string           `json:"grnNumber"`
	Items        []ReturnLineView `json:"items"`
}

// ReturnLineView is one line going back.
type ReturnLineView struct {
	domain.PurchaseReturnItem
	ItemName string `json:"itemName"`
}

func (s *Service) viewReturn(ctx context.Context, ret domain.PurchaseReturn) (ReturnView, error) {
	items, err := s.repo.ReturnItems(ctx, ret.ID)
	if err != nil {
		return ReturnView{}, err
	}
	receipt, err := s.repo.Receipt(ctx, ret.ReceiptID, false)
	if err != nil {
		return ReturnView{}, err
	}
	supplier, err := s.repo.Supplier(ctx, ret.SupplierID)
	if err != nil {
		return ReturnView{}, err
	}
	names, err := s.stock.ItemNames(ctx)
	if err != nil {
		return ReturnView{}, err
	}

	view := ReturnView{
		PurchaseReturn: ret, SupplierName: supplier.Name, GRNNumber: receipt.GRNNumber,
	}
	for _, item := range items {
		view.Items = append(view.Items, ReturnLineView{
			PurchaseReturnItem: item, ItemName: names[item.ItemID],
		})
	}
	return view, nil
}

func (s *Service) Returns(ctx context.Context, receiptID, status string, limit int) ([]domain.PurchaseReturn, error) {
	return s.repo.Returns(ctx, receiptID, status, limit)
}

func (s *Service) Return(ctx context.Context, returnID string) (ReturnView, error) {
	ret, err := s.repo.Return(ctx, returnID, false)
	if err != nil {
		return ReturnView{}, err
	}
	return s.viewReturn(ctx, ret)
}

// OpenReturn starts a return against a posted receipt.
func (s *Service) OpenReturn(ctx context.Context, receiptID string, reason domain.ReturnReason, note *string, actor Actor) (ReturnView, error) {
	receipt, err := s.repo.Receipt(ctx, receiptID, false)
	if err != nil {
		return ReturnView{}, err
	}
	// Only goods actually taken into stock can go back out of it.
	if receipt.Status != domain.GRNPosted {
		return ReturnView{}, httpx.Conflict("RECEIPT_NOT_POSTED",
			"That delivery is %s; nothing has been taken into stock to return.",
			strings.ToLower(string(receipt.Status)))
	}

	created, err := s.repo.InsertReturn(ctx, domain.PurchaseReturn{
		ID: s.ids.New(id.PurchaseReturn), ReturnNumber: s.documentNumber("RTN", id.PurchaseReturn),
		ReceiptID: receiptID, SupplierID: receipt.SupplierID, BranchID: receipt.BranchID,
		ReturnedOn: s.today(), ReasonType: reason, ReasonNote: note, Status: domain.ReturnDraft,
	})
	if err != nil {
		return ReturnView{}, err
	}
	s.record(ctx, "purchasing.return", created.ID, "OPEN", actor, note)
	return s.viewReturn(ctx, created)
}

// ReturnLineInput is one line going back.
type ReturnLineInput struct {
	ReceiptItemID string
	Qty           domain.Quantity
	Note          *string
}

func (s *Service) AddReturnLine(ctx context.Context, returnID string, in ReturnLineInput, actor Actor) (ReturnView, error) {
	var view ReturnView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		ret, err := s.repo.Return(ctx, returnID, true)
		if err != nil {
			return err
		}
		if ret.Status != domain.ReturnDraft {
			return httpx.Conflict("NOT_DRAFT",
				"That return is %s and cannot be changed.", strings.ToLower(string(ret.Status)))
		}
		if in.Qty <= 0 {
			return httpx.Invalid("A return line needs a positive quantity.")
		}

		line, err := s.repo.ReceiptItem(ctx, in.ReceiptItemID, false)
		if err != nil {
			return err
		}
		if line.ReceiptID != ret.ReceiptID {
			return httpx.Invalid("That line belongs to a different delivery.")
		}
		// Rejected goods never entered stock, so they were never taken in and
		// cannot be sent back.
		if in.Qty > line.QtyReturnable() {
			return httpx.Conflict("OVER_RETURNED",
				"Only %v of that line was accepted and not yet returned.", line.QtyReturnable())
		}

		if _, err := s.repo.InsertReturnItem(ctx, domain.PurchaseReturnItem{
			ID: s.ids.New(id.LineItem), ReturnID: returnID, ReceiptItemID: in.ReceiptItemID,
			ItemID: line.ItemID, Qty: in.Qty, UnitPriceIDR: line.UnitPriceIDR, Note: in.Note,
		}); err != nil {
			return err
		}

		ret, err = s.retotalReturn(ctx, ret)
		if err != nil {
			return err
		}
		view, err = s.viewReturn(ctx, ret)
		return err
	})
	return view, err
}

func (s *Service) retotalReturn(ctx context.Context, ret domain.PurchaseReturn) (domain.PurchaseReturn, error) {
	items, err := s.repo.ReturnItems(ctx, ret.ID)
	if err != nil {
		return domain.PurchaseReturn{}, err
	}
	var total float64
	for _, item := range items {
		total += float64(item.Qty) * item.UnitPriceIDR
	}
	ret.TotalIDR = total
	return s.repo.SaveReturn(ctx, ret)
}

// PostReturn sends the goods back: stock leaves, and the receipt lines record
// how much of what they took in has gone again.
func (s *Service) PostReturn(ctx context.Context, returnID string, actor Actor) (ReturnView, error) {
	var view ReturnView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		ret, err := s.repo.Return(ctx, returnID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.PurchaseReturnTransitions, ret.Status, domain.ReturnPosted)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That return is %s and cannot be posted.", strings.ToLower(string(ret.Status)))
		}

		lines, err := s.repo.ReturnItems(ctx, returnID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return httpx.Conflict("NO_LINES", "That return has nothing on it.")
		}

		ref := StockRef{Type: "PURCHASE_RETURN", ID: ret.ID, Number: ret.ReturnNumber}
		var value float64
		for _, line := range lines {
			if err := s.stock.Return(ctx, line.ItemID, ret.BranchID, line.Qty, ref, s.stockActor(actor)); err != nil {
				return err
			}
			if err := s.repo.AddReturnedQty(ctx, line.ReceiptItemID, line.Qty); err != nil {
				return err
			}
			value += float64(line.Qty) * line.UnitPriceIDR
		}

		now := s.clock.Now()
		ret.Status = next
		ret.PostedAt = &now
		saved, err := s.repo.SaveReturn(ctx, ret)
		if err != nil {
			return err
		}

		// Goods going back are money coming back. A credit note is raised with
		// them so the debt is reduced at the moment the stock leaves, rather
		// than whenever somebody remembers — which is how a supplier ends up
		// paid in full for goods they took back.
		if value > 0 {
			reason := "Return " + ret.ReturnNumber
			credit, err := s.repo.InsertCredit(ctx, domain.VendorCredit{
				ID: s.ids.New(id.VendorCredit), CreditNumber: s.documentNumber("CN", id.VendorCredit),
				SupplierID: ret.SupplierID, ReturnID: &ret.ID,
				IssuedOn:  domain.DateOf(now, s.studio),
				AmountIDR: value, Status: domain.CreditOpen, Reason: &reason,
			})
			if err != nil {
				return err
			}
			if err := s.repo.SetReturnCredit(ctx, ret.ID, credit.ID); err != nil {
				return err
			}
			saved.CreditID = &credit.ID
		}
		view, err = s.viewReturn(ctx, saved)
		return err
	})
	if err != nil {
		return ReturnView{}, err
	}
	s.record(ctx, "purchasing.return", returnID, "POST", actor, nil)
	return view, nil
}

// Overview is the purchasing dashboard.
func (s *Service) Overview(ctx context.Context, branchID string) (Summary, error) {
	today := s.today()
	monthStart := domain.Date(string(today)[:8] + "01")
	return s.repo.Summary(ctx, branchID, monthStart)
}
