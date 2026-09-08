package pos

import (
	"context"
	"fmt"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Offers, cards, tenders, receipts and the manager standing there.

// ── Promotions ───────────────────────────────────────────────────────────────

// PromotionView is an offer with what it applies to.
type PromotionView struct {
	domain.Promotion
	Targets []domain.PromotionTarget `json:"targets"`
}

func (s *Service) Promotions(ctx context.Context, activeOnly bool) ([]PromotionView, error) {
	promotions, err := s.repo.Promotions(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	targets, err := s.repo.PromotionTargets(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]PromotionView, 0, len(promotions))
	for _, promotion := range promotions {
		list := targets[promotion.ID]
		if list == nil {
			list = []domain.PromotionTarget{}
		}
		views = append(views, PromotionView{Promotion: promotion, Targets: list})
	}
	return views, nil
}

// PromotionInput defines an offer.
type PromotionInput struct {
	domain.Promotion
	Targets []domain.PromotionTarget
}

// SavePromotion writes an offer and what it targets.
func (s *Service) SavePromotion(ctx context.Context, in PromotionInput, actor Actor) (PromotionView, error) {
	if rejection := validatePromotion(in.Promotion); rejection != "" {
		return PromotionView{}, httpx.Invalid("%s", rejection)
	}

	var saved domain.Promotion
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		promotion := in.Promotion
		promotion.ID = s.ids.New(id.Promotion)
		promotion.Code = strings.ToUpper(strings.TrimSpace(promotion.Code))
		var err error
		saved, err = s.repo.UpsertPromotion(ctx, promotion)
		if err != nil {
			return err
		}

		targets := make([]domain.PromotionTarget, 0, len(in.Targets))
		for _, target := range in.Targets {
			target.ID = s.ids.New(id.PromoTarget)
			if target.Qty <= 0 {
				target.Qty = 1
			}
			targets = append(targets, target)
		}
		return s.repo.ReplaceTargets(ctx, saved.ID, targets)
	})
	if err != nil {
		return PromotionView{}, err
	}
	s.record(ctx, "pos.promotion", saved.ID, "SAVE", actor, nil)

	targets, err := s.repo.PromotionTargets(ctx)
	if err != nil {
		return PromotionView{}, err
	}
	list := targets[saved.ID]
	if list == nil {
		list = []domain.PromotionTarget{}
	}
	return PromotionView{Promotion: saved, Targets: list}, nil
}

// validatePromotion checks that an offer carries the numbers its kind needs.
//
// A percent offer with no percentage is a row that silently discounts nothing,
// which is worse than a refusal — nobody finds out until a customer asks why
// the poster in the window is lying.
func validatePromotion(p domain.Promotion) string {
	if strings.TrimSpace(p.Code) == "" || strings.TrimSpace(p.Name) == "" {
		return "An offer needs a code and a name."
	}
	if !domain.IsValidPromotionKind(string(p.Kind)) {
		return fmt.Sprintf("%q is not a kind of offer.", p.Kind)
	}
	switch p.Kind {
	case domain.PromoPercent:
		if p.Percent == nil || *p.Percent <= 0 || *p.Percent > 100 {
			return "A percentage offer needs a percentage between 0 and 100."
		}
	case domain.PromoAmount:
		if p.AmountIDR == nil || *p.AmountIDR <= 0 {
			return "An amount offer needs an amount."
		}
	case domain.PromoBuyXGetY:
		if p.BuyQty == nil || p.FreeQty == nil || *p.BuyQty <= 0 || *p.FreeQty <= 0 {
			return "A buy-x-get-y offer needs both quantities."
		}
	case domain.PromoBundle:
		if p.BundlePriceIDR == nil {
			return "A bundle needs a price for the set."
		}
	}
	for _, channel := range p.Channels {
		if !domain.IsValidChannel(channel) {
			return fmt.Sprintf("%q is not a sales channel.", channel)
		}
	}
	return ""
}

// promotionsFor evaluates every live offer against an order's lines.
//
// It runs on every retotal rather than once at completion, so the customer
// sees what they will pay while the basket is still being built.
func (s *Service) promotionsFor(ctx context.Context, order domain.POSOrder,
	items []domain.POSOrderItem) (domain.PromotionOutcome, error) {

	if len(items) == 0 {
		return domain.PromotionOutcome{Applied: []domain.AppliedPromotion{}}, nil
	}
	promotions, err := s.repo.Promotions(ctx, true)
	if err != nil {
		return domain.PromotionOutcome{}, err
	}
	if len(promotions) == 0 {
		return domain.PromotionOutcome{Applied: []domain.AppliedPromotion{}}, nil
	}
	targets, err := s.repo.PromotionTargets(ctx)
	if err != nil {
		return domain.PromotionOutcome{}, err
	}

	// The category comes off the product, so a category-wide offer does not
	// need the till to have loaded the catalogue.
	basket := make([]domain.BasketLine, 0, len(items))
	for _, item := range items {
		product, err := s.repo.Product(ctx, item.ProductID)
		if err != nil {
			return domain.PromotionOutcome{}, err
		}
		basket = append(basket, domain.BasketLine{
			LineID: item.ID, ProductID: item.ProductID, CategoryID: product.CategoryID,
			Qty: item.Qty, UnitPriceIDR: item.UnitPriceIDR, LineTotalIDR: item.LineTotalIDR,
		})
	}

	uses := map[string]int{}
	if order.MemberID != nil {
		uses, err = s.repo.MemberPromotionUses(ctx, *order.MemberID)
		if err != nil {
			return domain.PromotionOutcome{}, err
		}
	}

	code := ""
	if order.PromoCode != nil {
		code = *order.PromoCode
	}
	return domain.EvaluatePromotions(basket, promotions, targets, domain.PromotionContext{
		Channel: order.Channel, Today: domain.DateOf(s.clock.Now(), s.studio),
		Now: s.clock.Now(), Code: code, MemberUses: uses,
	}), nil
}

// ApplyCode puts a promotion code on an open sale.
//
// A code that saves nothing is refused with the reason, because the cashier is
// about to be asked why and "nothing happened" is not an answer.
func (s *Service) ApplyCode(ctx context.Context, orderID, code string, actor Actor) (OrderView, error) {
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

		trimmed := strings.ToUpper(strings.TrimSpace(code))
		if trimmed == "" {
			order.PromoCode = nil
		} else {
			order.PromoCode = &trimmed
		}

		items, err := s.repo.OrderItems(ctx, orderID)
		if err != nil {
			return err
		}
		outcome, err := s.promotionsFor(ctx, order, items)
		if err != nil {
			return err
		}
		if trimmed != "" && outcome.Rejected != "" {
			return codeRejectionError(outcome.Rejected, trimmed)
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

func codeRejectionError(rejection domain.PromotionRejection, code string) error {
	switch rejection {
	case domain.PromoRejectUnknown:
		return httpx.NotFound("promotion code")
	case domain.PromoRejectWindow:
		return httpx.Conflict("OUTSIDE_WINDOW", "%s is not running today.", code)
	case domain.PromoRejectChannel:
		return httpx.Conflict("WRONG_CHANNEL", "%s does not apply to this kind of sale.", code)
	case domain.PromoRejectMinSpend:
		return httpx.Conflict("BELOW_MINIMUM", "This basket is under what %s needs.", code)
	case domain.PromoRejectExhausted:
		return httpx.Conflict("FULLY_USED", "%s has been used up.", code)
	case domain.PromoRejectPerMember:
		return httpx.Conflict("ALREADY_USED", "This member has already had %s.", code)
	case domain.PromoRejectNoQualify:
		return httpx.Conflict("NOTHING_QUALIFIES", "Nothing on this sale qualifies for %s.", code)
	}
	return httpx.Conflict("NOT_APPLICABLE", "%s does not apply to this sale.", code)
}

// ── Gift cards ───────────────────────────────────────────────────────────────

// GiftCardView is a card with what has happened to it.
type GiftCardView struct {
	domain.GiftCard
	MemberName string          `json:"memberName"`
	Entries    []GiftCardEntry `json:"entries"`
}

func (s *Service) GiftCards(ctx context.Context, filter GiftCardFilter) ([]domain.GiftCard, error) {
	return s.repo.GiftCards(ctx, filter)
}

func (s *Service) GiftCard(ctx context.Context, code string) (GiftCardView, error) {
	card, err := s.repo.GiftCardByCode(ctx, strings.TrimSpace(code), false)
	if err != nil {
		return GiftCardView{}, err
	}
	entries, err := s.repo.GiftCardEntries(ctx, card.ID)
	if err != nil {
		return GiftCardView{}, err
	}

	view := GiftCardView{GiftCard: card, Entries: entries}
	if card.MemberID != nil {
		if member, err := s.members.Member(ctx, *card.MemberID); err == nil {
			view.MemberName = member.FullName
		}
	}
	return view, nil
}

// GiftCardInput issues a card.
type GiftCardInput struct {
	Code      string
	Barcode   *string
	MemberID  *string
	AmountIDR float64
	ExpiresOn *domain.Date
	Note      *string
}

// IssueGiftCard puts money on a new card.
//
// The opening balance is written as a movement rather than only as a number,
// so the card's ledger explains its balance from the first rupiah.
func (s *Service) IssueGiftCard(ctx context.Context, in GiftCardInput, actor Actor) (GiftCardView, error) {
	if in.AmountIDR <= 0 {
		return GiftCardView{}, httpx.Invalid("A gift card needs money on it.")
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		code = strings.ToUpper(shortCode(s.ids.New(id.GiftCard)))
	}

	var card domain.GiftCard
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		if in.MemberID != nil {
			if _, err := s.members.Member(ctx, *in.MemberID); err != nil {
				return err
			}
		}
		var err error
		card, err = s.repo.InsertGiftCard(ctx, domain.GiftCard{
			ID: s.ids.New(id.GiftCard), Code: code, Barcode: in.Barcode,
			MemberID: in.MemberID, IssuedOn: domain.DateOf(s.clock.Now(), s.studio),
			ExpiresOn: in.ExpiresOn, InitialIDR: in.AmountIDR, BalanceIDR: in.AmountIDR,
			Status: domain.GiftCardActive, IssuedBy: &actor.ID, Note: in.Note,
		})
		if err != nil {
			return err
		}
		return s.repo.PostGiftCard(ctx, card.ID, domain.PostedGiftCard{
			AmountIDR: in.AmountIDR, BalanceBefore: 0, BalanceAfter: in.AmountIDR,
		}, "ISSUE", nil, in.Note, actor.ID, s.ids.New(id.GiftCardEntry))
	})
	if err != nil {
		return GiftCardView{}, err
	}
	s.record(ctx, "pos.giftcard", card.ID, "ISSUE", actor, nil)
	return s.GiftCard(ctx, card.Code)
}

// TopUpGiftCard adds money to an existing card.
func (s *Service) TopUpGiftCard(ctx context.Context, code string, amountIDR float64, actor Actor) (GiftCardView, error) {
	var card domain.GiftCard
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		existing, err := s.repo.GiftCardByCode(ctx, code, true)
		if err != nil {
			return err
		}
		posted := domain.TopUpGiftCard(existing, amountIDR)
		if !posted.Allowed() {
			return giftCardRejectionError(posted.Rejection, existing)
		}
		card = existing
		return s.repo.PostGiftCard(ctx, existing.ID, posted, "TOP_UP", nil, nil,
			actor.ID, s.ids.New(id.GiftCardEntry))
	})
	if err != nil {
		return GiftCardView{}, err
	}
	s.record(ctx, "pos.giftcard", card.ID, "TOP_UP", actor, nil)
	return s.GiftCard(ctx, card.Code)
}

// SetGiftCardStatus freezes, unfreezes or cancels a card.
func (s *Service) SetGiftCardStatus(ctx context.Context, code, status string, actor Actor) (GiftCardView, error) {
	card, err := s.repo.GiftCardByCode(ctx, code, false)
	if err != nil {
		return GiftCardView{}, err
	}
	next := strings.ToUpper(strings.TrimSpace(status))
	switch domain.GiftCardStatus(next) {
	case domain.GiftCardActive, domain.GiftCardFrozen, domain.GiftCardExpired, domain.GiftCardCancelled:
	default:
		return GiftCardView{}, httpx.Invalid("%q is not a state a card can be in.", status)
	}

	if _, err := s.repo.SetGiftCardStatus(ctx, card.ID, next); err != nil {
		return GiftCardView{}, err
	}
	s.record(ctx, "pos.giftcard", card.ID, "STATUS", actor, &next)
	return s.GiftCard(ctx, card.Code)
}

func giftCardRejectionError(rejection domain.GiftCardRejection, card domain.GiftCard) error {
	switch rejection {
	case domain.GiftRejectNotActive:
		return httpx.Conflict("CARD_NOT_ACTIVE",
			"That card is %s.", strings.ToLower(string(card.Status)))
	case domain.GiftRejectExpired:
		return httpx.Conflict("CARD_EXPIRED", "That card expired on %s.", *card.ExpiresOn)
	case domain.GiftRejectInsufficient:
		return httpx.Conflict("INSUFFICIENT_BALANCE",
			"That card has %.0f on it.", card.BalanceIDR)
	case domain.GiftRejectWrongMember:
		return httpx.Conflict("WRONG_MEMBER",
			"That card belongs to somebody else. Name them on the sale to use it.")
	case domain.GiftRejectZero:
		return httpx.Invalid("A card movement of nothing is not a movement.")
	}
	return httpx.Invalid("That card cannot be used.")
}

// ── Payment methods ──────────────────────────────────────────────────────────

func (s *Service) PaymentMethods(ctx context.Context, activeOnly bool) ([]domain.PaymentMethod, error) {
	return s.repo.PaymentMethods(ctx, activeOnly)
}

func (s *Service) SaveMethod(ctx context.Context, m domain.PaymentMethod, actor Actor) (domain.PaymentMethod, error) {
	m.Code = strings.ToUpper(strings.TrimSpace(m.Code))
	if m.Code == "" || strings.TrimSpace(m.Name) == "" {
		return domain.PaymentMethod{}, httpx.Invalid("A payment method needs a code and a name.")
	}
	if !domain.IsValidMethodKind(string(m.Kind)) {
		return domain.PaymentMethod{}, httpx.Invalid("%q is not a kind of payment.", m.Kind)
	}
	m.ID = s.ids.New(id.PaymentMethod)

	saved, err := s.repo.UpsertMethod(ctx, m)
	if err != nil {
		return domain.PaymentMethod{}, err
	}
	s.record(ctx, "pos.method", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// ── Receipts ─────────────────────────────────────────────────────────────────

func (s *Service) ReceiptSettings(ctx context.Context, branchID string) (ReceiptSettings, error) {
	return s.repo.Settings(ctx, branchID)
}

func (s *Service) SaveReceiptSettings(ctx context.Context, settings ReceiptSettings, actor Actor) (ReceiptSettings, error) {
	if strings.TrimSpace(settings.BranchID) == "" {
		return ReceiptSettings{}, httpx.Invalid("Receipt settings belong to a branch.")
	}
	if settings.PaperWidth != 58 && settings.PaperWidth != 80 {
		settings.PaperWidth = 58
	}
	saved, err := s.repo.SaveSettings(ctx, settings)
	if err != nil {
		return ReceiptSettings{}, err
	}
	s.record(ctx, "pos.receipt", settings.BranchID, "SAVE", actor, nil)
	return saved, nil
}

// RenderReceipt turns a sale into the text a thermal printer takes.
//
// Rendered on the server and stored with the job, so a reprint next week is
// the same paper as the original even after the product has been renamed and
// the prices have moved.
func (s *Service) RenderReceipt(ctx context.Context, orderID string) (string, error) {
	view, err := s.Order(ctx, orderID)
	if err != nil {
		return "", err
	}
	settings, err := s.repo.Settings(ctx, view.BranchID)
	if err != nil {
		return "", err
	}
	promotions, err := s.repo.OrderPromotions(ctx, orderID)
	if err != nil {
		return "", err
	}
	return renderReceipt(view, settings, promotions, s.studio), nil
}

// QueueReceipt puts a rendered receipt in front of a printer.
func (s *Service) QueueReceipt(ctx context.Context, orderID string, actor Actor) (PrintJob, error) {
	view, err := s.Order(ctx, orderID)
	if err != nil {
		return PrintJob{}, err
	}
	payload, err := s.RenderReceipt(ctx, orderID)
	if err != nil {
		return PrintJob{}, err
	}

	job, err := s.repo.InsertPrintJob(ctx, PrintJob{
		ID: s.ids.New(id.PrintJob), BranchID: view.BranchID, Kind: "RECEIPT",
		OrderID: &orderID, Payload: payload, Status: "QUEUED",
	})
	if err != nil {
		return PrintJob{}, err
	}
	s.record(ctx, "pos.print", job.ID, "QUEUE", actor, nil)
	return job, nil
}

func (s *Service) ClaimPrintJobs(ctx context.Context, branchID string, limit int) ([]PrintJob, error) {
	if branchID == "" {
		return nil, httpx.Invalid("A printer belongs to a branch.")
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	return s.repo.ClaimPrintJobs(ctx, branchID, limit)
}

func (s *Service) FinishPrintJob(ctx context.Context, jobID, status string, failure *string) (PrintJob, error) {
	next := strings.ToUpper(strings.TrimSpace(status))
	switch next {
	case "PRINTED", "FAILED", "CANCELLED":
	default:
		return PrintJob{}, httpx.Invalid("A print job ends printed, failed or cancelled.")
	}
	return s.repo.FinishPrintJob(ctx, jobID, next, failure)
}

func (s *Service) PrintJobs(ctx context.Context, branchID, status string, limit int) ([]PrintJob, error) {
	return s.repo.PrintJobs(ctx, branchID, strings.ToUpper(status), limit)
}

// SendReceipt queues a receipt to a phone.
//
// A job rather than a call, for the same reason printing is: a message that
// was never sent should be visible as one rather than disappearing into a
// provider nobody can query.
func (s *Service) SendReceipt(ctx context.Context, orderID, channel, destination string, actor Actor) (ReceiptSend, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return ReceiptSend{}, httpx.Invalid("Where should the receipt go?")
	}
	next := strings.ToUpper(strings.TrimSpace(channel))
	if next == "" {
		next = "WHATSAPP"
	}
	switch next {
	case "WHATSAPP", "EMAIL", "SMS":
	default:
		return ReceiptSend{}, httpx.Invalid("%q is not a way of sending a receipt.", channel)
	}

	body, err := s.RenderReceipt(ctx, orderID)
	if err != nil {
		return ReceiptSend{}, err
	}
	send, err := s.repo.InsertReceiptSend(ctx, ReceiptSend{
		ID: s.ids.New(id.ReceiptSend), OrderID: orderID, Channel: next,
		Destination: destination, Body: body, Status: "QUEUED",
	})
	if err != nil {
		return ReceiptSend{}, err
	}
	s.record(ctx, "pos.receipt", orderID, "SEND", actor, &next)
	return send, nil
}

// spendGiftCard takes money off a card as part of a sale.
//
// The card's reference is its number: a cashier holding one should not have to
// look it up, and the tender's reference field is where they type it.
func (s *Service) spendGiftCard(ctx context.Context, order domain.POSOrder,
	in TenderInput, actor Actor) (string, error) {

	if in.Reference == nil || strings.TrimSpace(*in.Reference) == "" {
		return "", httpx.Invalid("A gift card tender needs the card number.")
	}
	card, err := s.repo.GiftCardByCode(ctx, strings.TrimSpace(*in.Reference), true)
	if err != nil {
		return "", err
	}

	posted := domain.SpendGiftCard(card, in.AmountIDR,
		domain.DateOf(s.clock.Now(), s.studio), order.MemberID)
	if !posted.Allowed() {
		return "", giftCardRejectionError(posted.Rejection, card)
	}
	if err := s.repo.PostGiftCard(ctx, card.ID, posted, "SPEND", &order.ID, nil,
		actor.ID, s.ids.New(id.GiftCardEntry)); err != nil {
		return "", err
	}
	return card.ID, nil
}
