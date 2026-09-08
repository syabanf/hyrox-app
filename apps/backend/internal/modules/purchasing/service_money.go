package purchasing

import (
	"context"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// The receiving bay, and the money.

// ── Deliveries ───────────────────────────────────────────────────────────────

// DeliveryView is an arrival with what was on it.
type DeliveryView struct {
	domain.Delivery
	SupplierName string                   `json:"supplierName"`
	OrderNumber  string                   `json:"orderNumber"`
	Items        []DeliveryLineView       `json:"items"`
	Inspection   domain.InspectionOutcome `json:"inspection"`
}

// DeliveryLineView is one line off the truck, and how much of it has been
// judged since.
type DeliveryLineView struct {
	domain.DeliveryItem
	ItemName    string          `json:"itemName"`
	QtyAccepted domain.Quantity `json:"qtyAccepted"`
	QtyRejected domain.Quantity `json:"qtyRejected"`
	Outstanding domain.Quantity `json:"outstanding"`
}

func (s *Service) Deliveries(ctx context.Context, filter DeliveryFilter) ([]domain.Delivery, error) {
	return s.repo.Deliveries(ctx, filter)
}

func (s *Service) Delivery(ctx context.Context, deliveryID string) (DeliveryView, error) {
	delivery, err := s.repo.Delivery(ctx, deliveryID, false)
	if err != nil {
		return DeliveryView{}, err
	}
	return s.viewDelivery(ctx, delivery)
}

func (s *Service) viewDelivery(ctx context.Context, delivery domain.Delivery) (DeliveryView, error) {
	items, err := s.repo.DeliveryItems(ctx, delivery.ID)
	if err != nil {
		return DeliveryView{}, err
	}
	names, err := s.stock.ItemNames(ctx)
	if err != nil {
		return DeliveryView{}, err
	}
	order, err := s.repo.Order(ctx, delivery.OrderID, false)
	if err != nil {
		return DeliveryView{}, err
	}
	supplier, err := s.repo.Supplier(ctx, delivery.SupplierID)
	if err != nil {
		return DeliveryView{}, err
	}

	view := DeliveryView{
		Delivery: delivery, SupplierName: supplier.Name, OrderNumber: order.PONumber,
		Items: []DeliveryLineView{},
	}
	var delivered, accepted, rejected domain.Quantity
	for _, item := range items {
		// What this delivery's own lines have since been judged as. Receipt
		// lines point at the order line rather than the delivery line, so the
		// figures come from there.
		acceptedLine, rejectedLine, err := s.repo.InspectedOnDelivery(ctx, delivery.ID, item.OrderItemID)
		if err != nil {
			return DeliveryView{}, err
		}
		outcome := domain.InspectDelivery(item.QtyDelivered, acceptedLine, rejectedLine)
		view.Items = append(view.Items, DeliveryLineView{
			DeliveryItem: item, ItemName: names[item.ItemID],
			QtyAccepted: acceptedLine, QtyRejected: rejectedLine,
			Outstanding: outcome.Outstanding,
		})
		delivered += item.QtyDelivered
		accepted += acceptedLine
		rejected += rejectedLine
	}
	view.Inspection = domain.InspectDelivery(delivered, accepted, rejected)
	return view, nil
}

// DeliveryInput opens an arrival.
type DeliveryInput struct {
	OrderID            string
	DeliveryNoteNumber *string
	DriverName         *string
	Vehicle            *string
	ArrivedOn          domain.Date
	Note               *string
}

// OpenDelivery records that a truck turned up.
//
// It moves no stock and makes no judgement: that is the whole point of it
// being a separate document from the goods receipt. Somebody at the bay counts
// what was handed over, and somebody else decides later whether it is any good.
func (s *Service) OpenDelivery(ctx context.Context, in DeliveryInput, actor Actor) (DeliveryView, error) {
	var view DeliveryView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, in.OrderID, false)
		if err != nil {
			return err
		}
		switch order.Status {
		case domain.POApproved, domain.POSent, domain.POPartiallyReceived:
		default:
			return httpx.Conflict("ORDER_NOT_OPEN",
				"That order is %s; nothing should be arriving against it.",
				strings.ToLower(string(order.Status)))
		}

		arrivedOn := in.ArrivedOn
		if arrivedOn == "" {
			arrivedOn = domain.DateOf(s.clock.Now(), s.studio)
		}
		created, err := s.repo.InsertDelivery(ctx, domain.Delivery{
			ID: s.ids.New(id.Delivery), DeliveryNumber: s.documentNumber("DO", id.Delivery),
			OrderID: order.ID, SupplierID: order.SupplierID, BranchID: order.BranchID,
			DeliveryNoteNumber: in.DeliveryNoteNumber, DriverName: in.DriverName,
			Vehicle: in.Vehicle, ArrivedOn: arrivedOn,
			ReceivedBy: &actor.ID, ReceivedByName: &actor.Name,
			Status: domain.DeliveryArrived, Note: in.Note,
		})
		if err != nil {
			return err
		}
		view, err = s.viewDelivery(ctx, created)
		return err
	})
	if err != nil {
		return DeliveryView{}, err
	}
	s.record(ctx, "purchasing.delivery", view.ID, "OPEN", actor, nil)
	return view, nil
}

// DeliveryLineInput is one line off the truck.
type DeliveryLineInput struct {
	OrderItemID string
	Qty         domain.Quantity
	BatchNumber *string
	ExpiresOn   *domain.Date
	Note        *string
}

// AddDeliveryLine counts one line at the bay.
func (s *Service) AddDeliveryLine(ctx context.Context, deliveryID string,
	in DeliveryLineInput, actor Actor) (DeliveryView, error) {

	var view DeliveryView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		delivery, err := s.repo.Delivery(ctx, deliveryID, true)
		if err != nil {
			return err
		}
		if delivery.Status != domain.DeliveryArrived {
			return httpx.Conflict("DELIVERY_CLOSED",
				"That delivery is %s and cannot be changed.",
				strings.ToLower(string(delivery.Status)))
		}

		order, err := s.repo.Order(ctx, delivery.OrderID, false)
		if err != nil {
			return err
		}
		line, err := s.repo.OrderItem(ctx, in.OrderItemID, false)
		if err != nil {
			return err
		}
		if line.OrderID != delivery.OrderID {
			return httpx.Invalid("That line belongs to a different order.")
		}

		delivered, err := s.repo.DeliveredOnOrderLine(ctx, in.OrderItemID)
		if err != nil {
			return err
		}
		if rejection := domain.EvaluateDeliveryLine(order, line, delivered, in.Qty); rejection != "" {
			return deliveryRejectionError(rejection, line, delivered)
		}

		if _, err := s.repo.InsertDeliveryItem(ctx, domain.DeliveryItem{
			ID: s.ids.New(id.LineItem), DeliveryID: deliveryID, OrderItemID: in.OrderItemID,
			ItemID: line.ItemID, QtyDelivered: in.Qty,
			// The pack comes off the order line: what was ordered in cartons
			// arrives in cartons, and letting the bay pick its own unit is
			// the gap a conversion bug gets in through.
			Unit: line.Unit, PackFactor: line.PackFactor,
			BatchNumber: in.BatchNumber, ExpiresOn: in.ExpiresOn, Note: in.Note,
		}); err != nil {
			return err
		}

		view, err = s.viewDelivery(ctx, delivery)
		return err
	})
	return view, err
}

func deliveryRejectionError(rejection domain.DeliveryRejection,
	line domain.PurchaseOrderItem, delivered domain.Quantity) error {

	switch rejection {
	case domain.DeliveryRejectOverOrdered:
		return httpx.Conflict("OVER_DELIVERED",
			"%v %s were ordered and %v have already arrived; only %v more can.",
			line.QtyOrdered, strings.ToLower(line.Unit), delivered,
			domain.RoundQuantity(line.QtyOrdered-delivered))
	case domain.DeliveryRejectClosed:
		return httpx.Conflict("ORDER_NOT_OPEN", "Nothing should be arriving against that order.")
	case domain.DeliveryRejectZero:
		return httpx.Invalid("A delivery line needs a positive quantity.")
	}
	return httpx.Invalid("That delivery line cannot be recorded.")
}

// CloseDelivery marks an arrival as fully inspected.
func (s *Service) CloseDelivery(ctx context.Context, deliveryID string, actor Actor) (DeliveryView, error) {
	var view DeliveryView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		delivery, err := s.repo.Delivery(ctx, deliveryID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.DeliveryTransitions, delivery.Status, domain.DeliveryInspected)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That delivery is %s.", strings.ToLower(string(delivery.Status)))
		}

		current, err := s.viewDelivery(ctx, delivery)
		if err != nil {
			return err
		}
		if !current.Inspection.Complete {
			return httpx.Conflict("NOT_INSPECTED",
				"%v units on that delivery have not been accepted or rejected yet.",
				current.Inspection.Outstanding)
		}

		delivery.Status = next
		saved, err := s.repo.SaveDelivery(ctx, delivery)
		if err != nil {
			return err
		}
		view, err = s.viewDelivery(ctx, saved)
		return err
	})
	if err != nil {
		return DeliveryView{}, err
	}
	s.record(ctx, "purchasing.delivery", deliveryID, "CLOSE", actor, nil)
	return view, nil
}

// ── Instalments ──────────────────────────────────────────────────────────────

// PayablesView is an order's money: what was agreed, what has moved, what is
// left.
type PayablesView struct {
	OrderID  string                  `json:"orderId"`
	Position domain.PayablesPosition `json:"position"`
	Terms    []domain.PaymentTerm    `json:"terms"`
	Payments []VendorPayment         `json:"payments"`
}

func (s *Service) Payables(ctx context.Context, orderID string) (PayablesView, error) {
	order, err := s.repo.Order(ctx, orderID, false)
	if err != nil {
		return PayablesView{}, err
	}
	terms, err := s.repo.PaymentTerms(ctx, orderID)
	if err != nil {
		return PayablesView{}, err
	}
	payments, err := s.repo.VendorPayments(ctx, PaymentFilter{OrderID: orderID, Limit: 200})
	if err != nil {
		return PayablesView{}, err
	}
	_, credited, err := s.repo.PaidOnOrder(ctx, orderID)
	if err != nil {
		return PayablesView{}, err
	}

	return PayablesView{
		OrderID: orderID, Terms: terms, Payments: payments,
		Position: domain.PayablesFor(order.TotalIDR, terms, credited,
			domain.DateOf(s.clock.Now(), s.studio)),
	}, nil
}

// ScheduleFromTerms builds an order's instalments from its supplier's terms.
//
// Called when an order is approved, and re-callable afterwards: it clears only
// the instalments nothing has been paid against, because money that has moved
// is a fact a rebuild must not erase.
func (s *Service) ScheduleFromTerms(ctx context.Context, orderID string, actor Actor) (PayablesView, error) {
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, false)
		if err != nil {
			return err
		}
		supplier, err := s.repo.Supplier(ctx, order.SupplierID)
		if err != nil {
			return err
		}
		if err := s.repo.DeleteTerms(ctx, orderID); err != nil {
			return err
		}

		orderedOn := order.OrderedOn
		if orderedOn == "" {
			orderedOn = domain.DateOf(s.clock.Now(), s.studio)
		}
		for _, term := range domain.BuildSchedule(order.TotalIDR, supplier.PaymentTerms, orderedOn) {
			term.ID = s.ids.New(id.PaymentTerm)
			term.OrderID = orderID
			if _, err := s.repo.UpsertTerm(ctx, term); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PayablesView{}, err
	}
	s.record(ctx, "purchasing.order", orderID, "SCHEDULE", actor, nil)
	return s.Payables(ctx, orderID)
}

// TermInput is one instalment, as somebody edits it.
type TermInput struct {
	Sequence  int
	Label     string
	DueOn     domain.Date
	Percent   *float64
	AmountIDR float64
	Note      *string
}

// SaveTerms replaces an order's schedule with the one somebody typed.
//
// "Half on order, half on delivery" is a real thing a supplier asks for and no
// code on a supplier record can express it, so the generated schedule is a
// starting point rather than the last word.
func (s *Service) SaveTerms(ctx context.Context, orderID string, in []TermInput, actor Actor) (PayablesView, error) {
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		order, err := s.repo.Order(ctx, orderID, false)
		if err != nil {
			return err
		}
		if len(in) == 0 {
			return httpx.Invalid("An order needs at least one instalment.")
		}
		if err := s.repo.DeleteTerms(ctx, orderID); err != nil {
			return err
		}

		terms := make([]domain.PaymentTerm, 0, len(in))
		for index, entry := range in {
			sequence := entry.Sequence
			if sequence <= 0 {
				sequence = index + 1
			}
			if entry.DueOn == "" {
				return httpx.Invalid("Every instalment needs a due date.")
			}
			terms = append(terms, domain.PaymentTerm{
				ID: s.ids.New(id.PaymentTerm), OrderID: orderID, Sequence: sequence,
				Label: entry.Label, DueOn: entry.DueOn, Percent: entry.Percent,
				AmountIDR: entry.AmountIDR, Status: domain.TermPending, Note: entry.Note,
			})
		}
		// Percentages win over typed amounts where both are given, and the
		// last instalment absorbs the rounding.
		terms = domain.Rebalance(terms, order.TotalIDR)

		for _, term := range terms {
			if _, err := s.repo.UpsertTerm(ctx, term); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PayablesView{}, err
	}
	s.record(ctx, "purchasing.order", orderID, "TERMS", actor, nil)
	return s.Payables(ctx, orderID)
}

// ── Payments ─────────────────────────────────────────────────────────────────

func (s *Service) VendorPayments(ctx context.Context, filter PaymentFilter) ([]VendorPayment, error) {
	return s.repo.VendorPayments(ctx, filter)
}

// PaymentInput is money about to leave.
type PaymentInput struct {
	SupplierID string
	OrderID    *string
	TermID     *string
	PaidOn     domain.Date
	AmountIDR  float64
	Method     string
	Reference  *string
	Note       *string
	// UseCredits settles as much as possible with open credit notes before
	// any cash moves, which is the whole reason a credit note exists.
	UseCredits bool
}

// RecordPayment raises a payment. It is a draft until somebody posts it.
func (s *Service) RecordPayment(ctx context.Context, in PaymentInput, actor Actor) (VendorPayment, error) {
	if in.AmountIDR <= 0 {
		return VendorPayment{}, httpx.Invalid("A payment of nothing is not a payment.")
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = "TRANSFER"
	}

	var payment VendorPayment
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		supplier, err := s.repo.Supplier(ctx, in.SupplierID)
		if err != nil {
			return err
		}
		paidOn := in.PaidOn
		if paidOn == "" {
			paidOn = domain.DateOf(s.clock.Now(), s.studio)
		}

		payment, err = s.repo.InsertVendorPayment(ctx, VendorPayment{
			ID: s.ids.New(id.VendorPayment), PaymentNumber: s.documentNumber("PAY", id.VendorPayment),
			SupplierID: supplier.ID, OrderID: in.OrderID, TermID: in.TermID,
			PaidOn: paidOn, AmountIDR: in.AmountIDR, Method: method,
			Reference: in.Reference, Status: "DRAFT", Note: in.Note,
		})
		if err != nil {
			return err
		}

		if !in.UseCredits {
			return nil
		}
		// Spend credit notes first, oldest first. Paying cash while a credit
		// note sits unspent until it lapses is the same as throwing the money
		// away.
		credits, err := s.repo.CreditsForSpending(ctx, supplier.ID)
		if err != nil {
			return err
		}
		allocations, _ := domain.AllocateCredits(credits, in.AmountIDR,
			domain.DateOf(s.clock.Now(), s.studio))

		var applied float64
		for _, allocation := range allocations {
			var credit domain.VendorCredit
			for _, candidate := range credits {
				if candidate.ID == allocation.CreditID {
					credit = candidate
				}
			}
			if err := s.repo.ApplyCredit(ctx, allocation, payment.ID,
				s.ids.New(id.CreditApply), domain.Spent(credit, allocation.AmountIDR)); err != nil {
				return err
			}
			applied += allocation.AmountIDR
		}
		if applied > 0 {
			payment.CreditIDR = applied
			payment, err = s.repo.SaveVendorPayment(ctx, payment)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return VendorPayment{}, err
	}
	s.record(ctx, "purchasing.payment", payment.ID, "RECORD", actor, nil)
	return payment, nil
}

// PostPayment settles a payment against its instalment.
func (s *Service) PostPayment(ctx context.Context, paymentID string, actor Actor) (VendorPayment, error) {
	var payment VendorPayment
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		existing, err := s.repo.VendorPayment(ctx, paymentID, true)
		if err != nil {
			return err
		}
		if existing.Status != "DRAFT" {
			return httpx.Conflict("NOT_DRAFT",
				"That payment is %s and cannot be posted again.", strings.ToLower(existing.Status))
		}

		if existing.TermID != nil {
			term, err := s.repo.Term(ctx, *existing.TermID, true)
			if err != nil {
				return err
			}
			posting := domain.PostToTerm(term, existing.AmountIDR)
			if posting.OverpaidBy > 0 {
				// Either a mistake or a payment against something else.
				// Swallowing the difference loses whichever it was.
				return httpx.Conflict("OVERPAID",
					"That instalment needs %.0f and this pays %.0f — %.0f too much.",
					term.OutstandingIDR(), existing.AmountIDR, posting.OverpaidBy)
			}
			term.PaidIDR = posting.PaidIDR
			term.Status = posting.Status
			if _, err := s.repo.UpsertTerm(ctx, term); err != nil {
				return err
			}
		}

		now := s.clock.Now()
		existing.Status = "POSTED"
		existing.PostedAt = &now
		existing.PostedBy = &actor.ID
		payment, err = s.repo.SaveVendorPayment(ctx, existing)
		return err
	})
	if err != nil {
		return VendorPayment{}, err
	}
	s.record(ctx, "purchasing.payment", paymentID, "POST", actor, nil)
	return payment, nil
}

// VoidPayment unwinds a posted payment, with a reason.
func (s *Service) VoidPayment(ctx context.Context, paymentID, reason string, actor Actor) (VendorPayment, error) {
	if strings.TrimSpace(reason) == "" {
		return VendorPayment{}, httpx.Invalid("Voiding a payment needs a reason.")
	}

	var payment VendorPayment
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		existing, err := s.repo.VendorPayment(ctx, paymentID, true)
		if err != nil {
			return err
		}
		if existing.Status == "VOIDED" {
			return httpx.Conflict("ALREADY_VOIDED", "That payment is already voided.")
		}

		// The instalment gets its money back, so the payables list shows the
		// debt again rather than quietly staying settled.
		if existing.Status == "POSTED" && existing.TermID != nil {
			term, err := s.repo.Term(ctx, *existing.TermID, true)
			if err != nil {
				return err
			}
			posting := domain.PostToTerm(term, -existing.AmountIDR)
			term.PaidIDR = posting.PaidIDR
			term.Status = posting.Status
			if _, err := s.repo.UpsertTerm(ctx, term); err != nil {
				return err
			}
		}

		now := s.clock.Now()
		existing.Status = "VOIDED"
		existing.VoidedAt = &now
		existing.VoidReason = &reason
		payment, err = s.repo.SaveVendorPayment(ctx, existing)
		return err
	})
	if err != nil {
		return VendorPayment{}, err
	}
	s.record(ctx, "purchasing.payment", paymentID, "VOID", actor, &reason)
	return payment, nil
}

// ── Credit notes ─────────────────────────────────────────────────────────────

func (s *Service) VendorCredits(ctx context.Context, filter CreditFilter) ([]domain.VendorCredit, error) {
	return s.repo.VendorCredits(ctx, filter)
}

// CreditInput raises a credit note by hand, for the cases a return does not
// cover — a price correction, a rebate, a goodwill gesture.
type CreditInput struct {
	SupplierID string
	ReturnID   *string
	AmountIDR  float64
	Reason     *string
	ExpiresOn  *domain.Date
}

func (s *Service) RaiseCredit(ctx context.Context, in CreditInput, actor Actor) (domain.VendorCredit, error) {
	if in.AmountIDR <= 0 {
		return domain.VendorCredit{}, httpx.Invalid("A credit note of nothing is not a credit note.")
	}

	credit, err := s.repo.InsertCredit(ctx, domain.VendorCredit{
		ID: s.ids.New(id.VendorCredit), CreditNumber: s.documentNumber("CN", id.VendorCredit),
		SupplierID: in.SupplierID, ReturnID: in.ReturnID,
		IssuedOn:  domain.DateOf(s.clock.Now(), s.studio),
		AmountIDR: in.AmountIDR, Status: domain.CreditOpen,
		Reason: in.Reason, ExpiresOn: in.ExpiresOn,
	})
	if err != nil {
		return domain.VendorCredit{}, err
	}
	s.record(ctx, "purchasing.credit", credit.ID, "RAISE", actor, in.Reason)
	return credit, nil
}

// ── Somebody says yes ────────────────────────────────────────────────────────

// SubmitReturn sends a return for approval.
func (s *Service) SubmitReturn(ctx context.Context, returnID string, actor Actor) (ReturnView, error) {
	return s.decideReturn(ctx, returnID, domain.ReturnPending, nil, actor, "SUBMIT")
}

// ApproveReturn signs a return off. Only then may the goods leave.
func (s *Service) ApproveReturn(ctx context.Context, returnID string, note *string, actor Actor) (ReturnView, error) {
	return s.decideReturn(ctx, returnID, domain.ReturnApproved, note, actor, "APPROVE")
}

// ReviseReturn takes a rejected return back to draft so it can be corrected.
//
// This is the step that makes rejection useful: without it "no, not that one"
// is a dead end, and whoever raised it opens a second return rather than
// fixing the first.
func (s *Service) ReviseReturn(ctx context.Context, returnID string, actor Actor) (ReturnView, error) {
	return s.decideReturn(ctx, returnID, domain.ReturnDraft, nil, actor, "REVISE")
}

// RejectReturn refuses one, with a reason.
//
// A rejected return goes back to draft rather than dying: the usual outcome of
// "no, not that one" is a corrected return, not an abandoned one.
func (s *Service) RejectReturn(ctx context.Context, returnID string, note *string, actor Actor) (ReturnView, error) {
	if note == nil || strings.TrimSpace(*note) == "" {
		return ReturnView{}, httpx.Invalid("Rejecting a return needs a reason.")
	}
	return s.decideReturn(ctx, returnID, domain.ReturnRejected, note, actor, "REJECT")
}

func (s *Service) decideReturn(ctx context.Context, returnID string,
	target domain.PurchaseReturnStatus, note *string, actor Actor, action string) (ReturnView, error) {

	var view ReturnView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		ret, err := s.repo.Return(ctx, returnID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.PurchaseReturnTransitions, ret.Status, target)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That return is %s and cannot become %s.",
				strings.ToLower(string(ret.Status)), strings.ToLower(string(target)))
		}

		if target == domain.ReturnPending {
			lines, err := s.repo.ReturnItems(ctx, returnID)
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				return httpx.Conflict("NO_LINES",
					"That return has nothing on it to approve.")
			}
		}

		now := s.clock.Now()
		ret.Status = next
		switch target {
		case domain.ReturnPending:
			ret.SubmittedAt = &now
		case domain.ReturnApproved:
			ret.ApprovedBy, ret.ApprovedAt, ret.DecisionNote = &actor.ID, &now, note
		case domain.ReturnRejected:
			ret.RejectedBy, ret.RejectedAt, ret.DecisionNote = &actor.ID, &now, note
		}

		saved, err := s.repo.SaveReturn(ctx, ret)
		if err != nil {
			return err
		}
		view, err = s.viewReturn(ctx, saved)
		return err
	})
	if err != nil {
		return ReturnView{}, err
	}
	s.record(ctx, "purchasing.return", returnID, action, actor, note)
	return view, nil
}
