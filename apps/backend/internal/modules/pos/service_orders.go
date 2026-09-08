package pos

import (
	"context"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// The sale.

// OrderView is a sale with its lines, tenders and customer named.
type OrderView struct {
	domain.POSOrder
	MemberName *string               `json:"memberName"`
	Items      []domain.POSOrderItem `json:"items"`
	Payments   []domain.POSPayment   `json:"payments"`
	// DueIDR is what is still owed, which is what the till shows next.
	DueIDR float64 `json:"dueIdr"`
}

func (s *Service) viewOrder(ctx context.Context, order domain.POSOrder) (OrderView, error) {
	items, err := s.repo.OrderItems(ctx, order.ID)
	if err != nil {
		return OrderView{}, err
	}
	payments, err := s.repo.Payments(ctx, order.ID)
	if err != nil {
		return OrderView{}, err
	}

	view := OrderView{POSOrder: order, Items: items, Payments: payments}
	view.DueIDR = order.TotalIDR - order.PaidIDR
	if view.DueIDR < 0 {
		view.DueIDR = 0
	}
	if order.MemberID != nil {
		if member, err := s.members.Member(ctx, *order.MemberID); err == nil {
			name := member.FullName
			view.MemberName = &name
		}
	}
	return view, nil
}

func (s *Service) Orders(ctx context.Context, filter OrderFilter) ([]domain.POSOrder, error) {
	return s.repo.Orders(ctx, filter)
}

func (s *Service) Order(ctx context.Context, orderID string) (OrderView, error) {
	order, err := s.repo.Order(ctx, orderID, false)
	if err != nil {
		return OrderView{}, err
	}
	return s.viewOrder(ctx, order)
}

// OpenOrder starts a sale on the cashier's open till.
func (s *Service) OpenOrder(ctx context.Context, branchID string, memberID *string,
	channelCode string, note *string, actor Actor) (OrderView, error) {

	shift, hasShift, err := s.repo.OpenShiftFor(ctx, actor.ID, branchID)
	if err != nil {
		return OrderView{}, err
	}
	// A sale with no till behind it has nowhere to be counted at the end of
	// the day, so it is refused rather than orphaned.
	if !hasShift {
		return OrderView{}, httpx.Conflict("NO_OPEN_SHIFT",
			"Open a till before selling: this sale would have nowhere to be counted.")
	}
	if memberID != nil {
		if _, err := s.members.Member(ctx, *memberID); err != nil {
			return OrderView{}, err
		}
	}

	channel := domain.SalesChannel(strings.ToUpper(strings.TrimSpace(channelCode)))
	if channel == "" {
		channel = domain.ChannelRetail
	}
	if !domain.IsValidChannel(string(channel)) {
		return OrderView{}, httpx.Invalid("%q is not a sales channel.", channelCode)
	}
	created, err := s.repo.InsertOrder(ctx, domain.POSOrder{
		ID: s.ids.New(id.POSOrder), OrderNumber: s.documentNumber("SAL", id.POSOrder),
		BranchID: branchID, ShiftID: &shift.ID, CashierID: actor.ID, CashierName: actor.Name,
		MemberID: memberID, Channel: channel,
		Status: domain.POSOpen, PaymentStatus: domain.POSUnpaid, Note: note,
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.viewOrder(ctx, created)
}

// LineInput is one thing being sold.
type LineInput struct {
	ProductID   string
	Qty         domain.Quantity
	DiscountIDR float64
	Note        *string
}

// AddLine puts a product on a sale, freezing its name, price and cost onto the
// line — a receipt has to still be true a year later, when the product has
// been renamed and repriced and the average cost has moved twice.
func (s *Service) AddLine(ctx context.Context, orderID string, in LineInput, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		if order.Status != domain.POSOpen {
			return httpx.Conflict("ORDER_NOT_OPEN",
				"That sale is %s and cannot be added to.", strings.ToLower(string(order.Status)))
		}
		if in.Qty <= 0 {
			return httpx.Invalid("A line needs a positive quantity.")
		}

		product, err := s.repo.Product(ctx, in.ProductID)
		if err != nil {
			return err
		}
		if !product.Sellable() {
			return httpx.Conflict("PRODUCT_NOT_SELLABLE",
				"%s is not available right now.", product.Name)
		}

		// The cost is read at the moment of sale, so the margin is the truth
		// about this sale rather than about whenever the product was set up.
		// It is a cost per base unit, which is why the line multiplies it by
		// the pack factor and not by the quantity the cashier typed.
		cost := product.CostIDR
		if product.InventoryItemID != nil {
			if current, err := s.stock.UnitCost(ctx, *product.InventoryItemID); err == nil {
				cost = current
			}
		}

		// What this customer pays: the deepest price break their channel and
		// quantity reach, falling back to the shelf price.
		prices, err := s.repo.ProductPrices(ctx, product.ID)
		if err != nil {
			return err
		}
		unitPrice := domain.ResolvePrice(prices, product.PriceIDR, order.Channel, in.Qty)

		if _, err := s.repo.InsertOrderItem(ctx, domain.POSOrderItem{
			ID: s.ids.New(id.LineItem), OrderID: orderID, ProductID: product.ID,
			ProductName: product.Name, ProductSKU: product.SKU,
			InventoryItemID: product.InventoryItemID, Qty: in.Qty,
			PackUnit: product.PackUnit, PackFactor: product.PackFactor,
			UnitPriceIDR: unitPrice, DiscountIDR: in.DiscountIDR,
			TaxPercent: product.TaxPercent, UnitCostIDR: cost, Note: in.Note,
		}); err != nil {
			return err
		}

		order, err = s.retotal(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, order)
		return err
	})
	return view, err
}

func (s *Service) RemoveLine(ctx context.Context, orderID, lineID string, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		if order.Status != domain.POSOpen {
			return httpx.Conflict("ORDER_NOT_OPEN",
				"That sale is %s and cannot be changed.", strings.ToLower(string(order.Status)))
		}
		if err := s.repo.DeleteOrderItem(ctx, lineID); err != nil {
			return err
		}
		order, err = s.retotal(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, order)
		return err
	})
	return view, err
}

// retotal recomputes a sale through the domain, applying the member's tier
// discount on top of whatever the cashier gave.
func (s *Service) retotal(ctx context.Context, order domain.POSOrder) (domain.POSOrder, error) {
	items, err := s.repo.OrderItems(ctx, order.ID)
	if err != nil {
		return domain.POSOrder{}, err
	}

	// The tier discount is recomputed from scratch every time, never added to
	// what is already stored: folding the two into one figure makes the tier's
	// share compound with every line the cashier scans.
	order.TierDiscountIDR = 0
	if order.MemberID != nil {
		percent, err := s.loyalty.TierDiscountPercent(ctx, *order.MemberID)
		if err != nil {
			return domain.POSOrder{}, err
		}
		var subtotal float64
		for _, item := range items {
			subtotal += item.LineTotalIDR
		}
		order.TierDiscountIDR = domain.TierDiscount(subtotal, percent)
	}

	// Offers are evaluated on every retotal rather than once at completion, so
	// the price on the screen is the price that will be charged.
	outcome, err := s.promotionsFor(ctx, order, items)
	if err != nil {
		return domain.POSOrder{}, err
	}
	order.PromoDiscountIDR = outcome.DiscountIDR

	ids := make([]string, len(outcome.Applied))
	for i := range outcome.Applied {
		ids[i] = s.ids.New(id.OrderPromotion)
	}
	if err := s.repo.ReplaceOrderPromotions(ctx, order.ID, outcome.Applied, ids); err != nil {
		return domain.POSOrder{}, err
	}

	totals := domain.ComputeOrderPOSTotals(items,
		order.DiscountIDR+order.TierDiscountIDR+order.PromoDiscountIDR)
	order.SubtotalIDR = totals.SubtotalIDR
	order.TaxIDR = totals.TaxIDR
	order.TotalIDR = totals.TotalIDR
	order.CostIDR = totals.CostIDR
	order.GrossProfitIDR = totals.GrossProfitIDR
	return s.repo.SaveOrder(ctx, order)
}

// SetOrderDetails changes the customer, the manual discount or the note on an
// open sale.
type OrderDetailsInput struct {
	MemberID       *string
	SetMember      bool
	DiscountIDR    float64
	DiscountReason *string
	Note           *string
}

func (s *Service) SetOrderDetails(ctx context.Context, orderID string, in OrderDetailsInput, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		if order.Status != domain.POSOpen {
			return httpx.Conflict("ORDER_NOT_OPEN",
				"That sale is %s and cannot be changed.", strings.ToLower(string(order.Status)))
		}
		if in.SetMember {
			if in.MemberID != nil {
				if _, err := s.members.Member(ctx, *in.MemberID); err != nil {
					return err
				}
			}
			order.MemberID = in.MemberID
		}
		if in.DiscountIDR < 0 {
			return httpx.Invalid("A discount cannot be negative.")
		}
		order.DiscountIDR = in.DiscountIDR
		order.DiscountReason = in.DiscountReason
		if in.Note != nil {
			order.Note = in.Note
		}

		order, err = s.retotal(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, order)
		return err
	})
	return view, err
}

// TenderInput is money handed over.
type TenderInput struct {
	Method    domain.POSPaymentMethod
	AmountIDR float64
	Reference *string
}

// Tender takes a payment. Several tenders is a split payment, which is why
// this is a list rather than two fields on the sale.
func (s *Service) Tender(ctx context.Context, orderID string, in TenderInput, actor Actor) (OrderView, error) {
	if in.AmountIDR <= 0 {
		return OrderView{}, httpx.Invalid("A tender needs a positive amount.")
	}

	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		payments, err := s.repo.Payments(ctx, orderID)
		if err != nil {
			return err
		}
		var alreadyPaid float64
		for _, payment := range payments {
			alreadyPaid += payment.AmountIDR
		}

		methods, err := s.repo.MethodsByCode(ctx)
		if err != nil {
			return err
		}
		method, known := methods[string(in.Method)]
		if !known || !method.Active {
			return httpx.Invalid("%q is not a payment this counter takes.", in.Method)
		}
		if method.NeedsReference && method.Kind != domain.MethodGiftCard &&
			(in.Reference == nil || strings.TrimSpace(*in.Reference) == "") {
			return httpx.Invalid("%s needs a reference — the approval code or transfer id.", method.Name)
		}

		if rejection := domain.EvaluateTenderWith(order, method, in.AmountIDR, alreadyPaid); rejection != "" {
			return tenderRejectionError(rejection, order, alreadyPaid)
		}

		// A gift card is money already paid for, so taking it is a movement on
		// the card's own ledger rather than a note on the receipt.
		var cardID *string
		if method.Kind == domain.MethodGiftCard {
			card, err := s.spendGiftCard(ctx, order, in, actor)
			if err != nil {
				return err
			}
			cardID = &card
		}

		saved, err := s.repo.InsertPayment(ctx, domain.POSPayment{
			ID: s.ids.New(id.POSPayment), OrderID: orderID, Method: in.Method,
			AmountIDR: in.AmountIDR, Reference: in.Reference, CashierID: actor.ID,
			GiftCardID: cardID,
		})
		if err != nil {
			return err
		}

		payments = append(payments, saved)
		outcome := domain.SettleWith(order.TotalIDR, payments, methods)
		order.PaidIDR = outcome.PaidIDR
		order.ChangeIDR = outcome.ChangeIDR
		order.PaymentStatus = outcome.PaymentStatus
		// Change is recorded against the tender that overpaid, so the drawer
		// count can subtract it from what was actually taken.
		if outcome.ChangeIDR > 0 && method.GivesChange {
			if err := s.repo.SetChange(ctx, saved.ID, outcome.ChangeIDR); err != nil {
				return err
			}
		}

		if _, err := s.repo.SaveOrder(ctx, order); err != nil {
			return err
		}
		updated, err := s.repo.Order(ctx, orderID, false)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, updated)
		return err
	})
	return view, err
}

func tenderRejectionError(rejection domain.POSRejection, order domain.POSOrder, paid float64) error {
	switch rejection {
	case domain.POSRejectOrderClosed:
		return httpx.Conflict("ORDER_NOT_OPEN",
			"That sale is %s and takes no more money.", strings.ToLower(string(order.Status)))
	case domain.POSRejectNothingToPay:
		return httpx.Conflict("NOTHING_TO_PAY", "That sale has nothing on it yet.")
	case domain.POSRejectOverTendered:
		return httpx.Conflict("OVER_TENDERED",
			"Only %.0f is still owed, and a card gives no change.", order.TotalIDR-paid)
	default:
		return httpx.Invalid("That payment cannot be taken.")
	}
}

// Complete finishes a sale.
//
// This is where the three modules meet: the stock goes out, the points go on,
// and the sale is closed — in one transaction, so a paid sale can never exist
// without its stock having moved.
func (s *Service) Complete(ctx context.Context, orderID string, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.POSOrderTransitions, order.Status, domain.POSCompleted)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That sale is %s and cannot be completed.", strings.ToLower(string(order.Status)))
		}

		items, err := s.repo.OrderItems(ctx, orderID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return httpx.Conflict("EMPTY_ORDER", "That sale has nothing on it.")
		}
		if order.PaymentStatus != domain.POSPaid {
			return httpx.Conflict("NOT_SETTLED",
				"That sale still owes %.0f.", order.TotalIDR-order.PaidIDR)
		}

		soldItems := 0
		for _, item := range items {
			ref := StockRef{Type: "POS_SALE", ID: order.ID, Number: order.OrderNumber,
				PackUnit: item.PackUnit, PackFactor: item.PackFactor}
			soldItems += int(item.BaseQty())
			// A product with no inventory item is a service, and moves no
			// stock. Everything else comes off the shelf here or the sale
			// does not happen at all.
			if item.InventoryItemID == nil {
				continue
			}
			// Base units, never sold units: selling two six-packs takes
			// twelve bottles off the shelf.
			if err := s.stock.Issue(ctx, *item.InventoryItemID, order.BranchID,
				item.BaseQty(), ref, StockActor{ID: actor.ID, Name: actor.Name}); err != nil {
				return err
			}
		}

		if order.MemberID != nil {
			var bonus int
			for _, item := range items {
				if product, err := s.repo.Product(ctx, item.ProductID); err == nil {
					bonus += product.BonusXP * int(item.Qty)
				}
			}
			// Keyed on the sale, so completing it twice could never award the
			// points twice even if the transition somehow allowed it.
			earned, err := s.loyalty.AwardSale(ctx, *order.MemberID, order.BranchID, order.ID,
				order.TotalIDR, soldItems, bonus, "pos:"+order.ID)
			if err != nil {
				return err
			}
			order.XPEarned = earned
		}

		now := s.clock.Now()
		order.Status = next
		order.CompletedAt = &now
		saved, err := s.repo.SaveOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, saved)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	s.record(ctx, "pos.order", orderID, "COMPLETE", actor, nil)
	return view, nil
}

// Cancel abandons a sale before it is paid for. Nothing has moved, so nothing
// is unwound.
func (s *Service) Cancel(ctx context.Context, orderID string, actor Actor) (OrderView, error) {
	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.POSOrderTransitions, order.Status, domain.POSCancelled)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That sale is %s; a completed one is voided, not cancelled.",
				strings.ToLower(string(order.Status)))
		}
		if order.PaidIDR > 0 {
			return httpx.Conflict("ALREADY_PAID",
				"Money has been taken for that sale. Void it instead, with a reason.")
		}

		now := s.clock.Now()
		order.Status = next
		order.CancelledAt = &now
		saved, err := s.repo.SaveOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, saved)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	s.record(ctx, "pos.order", orderID, "CANCEL", actor, nil)
	return view, nil
}

// Void unwinds a completed sale: the stock goes back on the shelf and the
// points come off. It needs a reason and a permission the counter does not
// have, because it is the one action that makes money disappear.
func (s *Service) Void(ctx context.Context, orderID, reason, supervisorPIN string, actor Actor) (OrderView, error) {
	if strings.TrimSpace(reason) == "" {
		return OrderView{}, httpx.Invalid("Voiding a sale needs a reason.")
	}

	// A permission answers "may this person do it"; a PIN answers "is a
	// manager standing here right now". A cashier who cannot void may still
	// void one with a manager beside them, and the manager is recorded.
	var supervisor *SupervisorRef
	if !domain.HasPermission(actor.Role, domain.PermPOSVoid) {
		found, ok, err := s.supers.VerifyPIN(ctx, supervisorPIN, domain.PermPOSVoid)
		if err != nil {
			return OrderView{}, err
		}
		if !ok {
			return OrderView{}, httpx.ErrForbidden.WithMessage(
				"Voiding a paid sale needs a supervisor. Ask a manager for their PIN.")
		}
		supervisor = &found
	}

	var view OrderView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, true)
		if err != nil {
			return err
		}
		if supervisor != nil {
			order.AuthorisedBy, order.AuthorisedByName = &supervisor.ID, &supervisor.Name
		}
		next, err := domain.Transition(domain.POSOrderTransitions, order.Status, domain.POSVoided)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That sale is %s and cannot be voided.", strings.ToLower(string(order.Status)))
		}

		items, err := s.repo.OrderItems(ctx, orderID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.InventoryItemID == nil {
				continue
			}
			ref := StockRef{Type: "POS_VOID", ID: order.ID, Number: order.OrderNumber,
				PackUnit: item.PackUnit, PackFactor: item.PackFactor,
				RestoresType: "POS_SALE", RestoresID: order.ID}
			// Back at what it cost when it was sold, not at today's average:
			// the goods returning are the same goods that left.
			if err := s.stock.Restock(ctx, *item.InventoryItemID, order.BranchID,
				item.BaseQty(), item.UnitCostIDR, ref, StockActor{ID: actor.ID, Name: actor.Name}); err != nil {
				return err
			}
		}

		now := s.clock.Now()
		order.Status = next
		order.VoidedAt = &now
		order.VoidedBy = &actor.ID
		order.VoidReason = &reason
		order.PaymentStatus = domain.POSRefunded
		saved, err := s.repo.SaveOrder(ctx, order)
		if err != nil {
			return err
		}
		view, err = s.viewOrder(ctx, saved)
		return err
	})
	if err != nil {
		return OrderView{}, err
	}
	s.record(ctx, "pos.order", orderID, "VOID", actor, &reason)
	return view, nil
}

// ── Reporting ────────────────────────────────────────────────────────────────

// Overview is the counter's dashboard.
type Overview struct {
	Today  SalesSummary          `json:"today"`
	Month  SalesSummary          `json:"month"`
	Top    []TopProduct          `json:"topProducts"`
	Shifts []domain.CashierShift `json:"openShifts"`
}

func (s *Service) Overview(ctx context.Context, branchID string) (Overview, error) {
	now := s.clock.Now().In(s.studio)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.studio)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.studio)

	today, err := s.repo.SalesSummary(ctx, branchID, dayStart)
	if err != nil {
		return Overview{}, err
	}
	month, err := s.repo.SalesSummary(ctx, branchID, monthStart)
	if err != nil {
		return Overview{}, err
	}
	top, err := s.repo.TopProducts(ctx, branchID, monthStart, 10)
	if err != nil {
		return Overview{}, err
	}
	open, err := s.repo.Shifts(ctx, branchID, "", string(domain.ShiftOpen), 20)
	if err != nil {
		return Overview{}, err
	}
	return Overview{Today: today, Month: month, Top: top, Shifts: open}, nil
}
