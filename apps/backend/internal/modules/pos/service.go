// Package pos is the till: selling merchandise, drinks and supplements at the
// counter.
//
// A sale is where three modules meet. Completing one takes stock out of
// inventory, earns the member loyalty points, and is the only place in the
// system where somebody hands over cash. All three happen in one transaction,
// so a sale can never be paid for without the stock moving.
package pos

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/audit"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// StockRef and StockActor are what a stock movement points back at.
type StockRef struct {
	Type, ID, Number string
	// The pack the document was written in, so the ledger row can say "10
	// CTN" beside the 240 pieces it actually moved.
	PackUnit   string
	PackFactor float64
	// What this movement undoes, when it undoes something. Goods coming back
	// from a voided sale belong in the batches that sale took them from, and
	// this is how inventory finds them.
	RestoresType string
	RestoresID   string
}
type StockActor struct{ ID, Name string }

// Stock is the port the till needs from inventory: take stock out when
// something is sold, put it back when the sale is voided, and know what it
// cost so the margin can be frozen onto the line.
type Stock interface {
	Issue(ctx context.Context, itemID, branchID string, qty domain.Quantity,
		ref StockRef, actor StockActor) error
	Restock(ctx context.Context, itemID, branchID string, qty domain.Quantity,
		unitCostIDR float64, ref StockRef, actor StockActor) error
	UnitCost(ctx context.Context, itemID string) (float64, error)
	OnHand(ctx context.Context, itemID, branchID string) (domain.Quantity, error)
	// PackFor resolves the unit a product is sold in to how many base units it
	// takes off the shelf. The till never invents a factor: a six-pack that
	// inventory has no six-pack for is a catalogue mistake, not a sale.
	PackFor(ctx context.Context, itemID, unitCode string) (domain.ItemPack, error)
}

// Supervisors is the port the till needs for an override.
//
// A permission answers "may this person do it"; a PIN answers "is a manager
// standing here right now". Voiding a paid sale is the second question, and
// handing a cashier a manager's login to answer it is how a manager's login
// ends up on a sticky note under the till.
type Supervisors interface {
	VerifyPIN(ctx context.Context, pin string, permission domain.Permission) (SupervisorRef, bool, error)
}

// Loyalty is the port the till needs from CRM: what a member's tier takes off
// the price, and what the sale earns them.
type Loyalty interface {
	// TierDiscountPercent is what the member's standing is worth at the till.
	TierDiscountPercent(ctx context.Context, memberID string) (float64, error)
	// AwardSale posts the points for a completed sale, once per sale.
	AwardSale(ctx context.Context, memberID, branchID, orderID string,
		amountIDR float64, items int, bonusXP int, idempotencyKey string) (int, error)
}

// Members is the port for naming the customer on a receipt.
type Members interface {
	Member(ctx context.Context, id string) (domain.Member, error)
}

// Service implements the counter's use cases.
type Service struct {
	db      *database.DB
	repo    *Repository
	stock   Stock
	loyalty Loyalty
	members Members
	supers  Supervisors
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
	studio  *time.Location
}

func NewService(db *database.DB, repo *Repository, stock Stock, loyalty Loyalty,
	members Members, supers Supervisors, ids id.Generator, c clock.Clock,
	auditor audit.Recorder, studio *time.Location) *Service {
	if studio == nil {
		studio = time.UTC
	}
	return &Service{db: db, repo: repo, stock: stock, loyalty: loyalty, members: members,
		supers: supers, ids: ids, clock: c, auditor: auditor, studio: studio}
}

// Actor is the cashier.
type Actor struct {
	ID   string
	Name string
	// Role decides what they may do without a manager standing there.
	Role domain.AdminRole
}

func (s *Service) record(ctx context.Context, entity, entityID, action string, actor Actor, reason *string) {
	_ = s.auditor.Record(ctx, audit.Event{
		EntityType: entity, EntityID: entityID, Action: action,
		ActorID: actor.ID, ActorName: actor.Name, Reason: reason,
	})
}

func shortCode(generated string) string {
	if _, tail, found := strings.Cut(generated, "_"); found && len(tail) >= 6 {
		return tail[len(tail)-6:]
	}
	return generated
}

func (s *Service) documentNumber(prefix, idPrefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix,
		s.clock.Now().In(s.studio).Format("20060102"), shortCode(s.ids.New(idPrefix)))
}

// ── Catalogue ────────────────────────────────────────────────────────────────

func (s *Service) Categories(ctx context.Context) ([]domain.POSCategory, error) {
	return s.repo.Categories(ctx)
}

// CategoryInput creates or replaces a category.
type CategoryInput struct {
	ID        string
	Name      string
	SortOrder int
	Active    bool
}

func (s *Service) SaveCategory(ctx context.Context, in CategoryInput, actor Actor) (domain.POSCategory, error) {
	identifier := in.ID
	if identifier == "" {
		identifier = s.ids.New(id.POSCategory)
	}
	saved, err := s.repo.UpsertCategory(ctx, domain.POSCategory{
		ID: identifier, Name: in.Name, SortOrder: in.SortOrder, Active: in.Active,
	})
	if err != nil {
		return domain.POSCategory{}, err
	}
	s.record(ctx, "pos.category", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// ProductView is a product with its stock, so the counter can see whether it
// can actually sell what it is showing.
type ProductView struct {
	domain.POSProduct
	CategoryName *string          `json:"categoryName"`
	OnHand       *domain.Quantity `json:"onHand"`
}

func (s *Service) Products(ctx context.Context, filter ProductFilter, branchID string) ([]ProductView, error) {
	products, err := s.repo.Products(ctx, filter)
	if err != nil {
		return nil, err
	}
	categories, err := s.repo.Categories(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, c := range categories {
		names[c.ID] = c.Name
	}

	views := make([]ProductView, 0, len(products))
	for _, product := range products {
		view := ProductView{POSProduct: product}
		if product.CategoryID != nil {
			if name, ok := names[*product.CategoryID]; ok {
				view.CategoryName = &name
			}
		}
		// A product with no inventory item is a service, and has no stock to
		// report — which is different from having none left.
		if product.InventoryItemID != nil && branchID != "" {
			onHand, err := s.stock.OnHand(ctx, *product.InventoryItemID, branchID)
			if err == nil {
				held := onHand
				view.OnHand = &held
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// ProductInput creates or replaces something for sale.
type ProductInput struct {
	SKU             string
	Name            string
	Description     string
	CategoryID      *string
	InventoryItemID *string
	PriceIDR        float64
	TaxPercent      float64
	Barcode         *string
	// PackUnit and PackFactor say how much stock one sold unit takes off the
	// shelf. A carton of 24 is a factor of 24, and selling one moves 24.
	PackUnit   string
	PackFactor float64
	BonusXP    int
	ImageURL   *string
	Active     bool
	Available  bool
}

func (s *Service) SaveProduct(ctx context.Context, in ProductInput, actor Actor) (domain.POSProduct, error) {
	factor := in.PackFactor
	if factor <= 0 {
		factor = 1
	}
	unit := strings.ToUpper(strings.TrimSpace(in.PackUnit))
	if unit == "" {
		unit = "PCS"
	}
	// A product that draws on stock is sold in one of that item's defined
	// packs, and the factor comes from there rather than from the form: two
	// places holding the same conversion is two places for it to drift.
	if in.InventoryItemID != nil {
		pack, err := s.stock.PackFor(ctx, *in.InventoryItemID, in.PackUnit)
		if err != nil {
			return domain.POSProduct{}, err
		}
		unit, factor = pack.UnitCode, pack.Factor
	}

	// The cost follows the linked item's weighted average rather than being
	// typed: a margin somebody typed is a margin nobody can trust. It is a
	// cost per base unit, so a carton product carries the piece cost and the
	// factor does the rest.
	var cost float64
	if in.InventoryItemID != nil {
		itemCost, err := s.stock.UnitCost(ctx, *in.InventoryItemID)
		if err != nil {
			return domain.POSProduct{}, err
		}
		cost = itemCost
	}

	saved, err := s.repo.UpsertProduct(ctx, domain.POSProduct{
		ID: s.ids.New(id.POSProduct), SKU: strings.ToUpper(strings.TrimSpace(in.SKU)),
		Name: strings.TrimSpace(in.Name), Description: in.Description,
		CategoryID: in.CategoryID, InventoryItemID: in.InventoryItemID,
		PriceIDR: in.PriceIDR, CostIDR: cost, TaxPercent: in.TaxPercent,
		Barcode: in.Barcode, PackUnit: unit, PackFactor: factor,
		BonusXP: in.BonusXP, ImageURL: in.ImageURL,
		Active: in.Active, Available: in.Available,
	})
	if err != nil {
		return domain.POSProduct{}, err
	}
	s.record(ctx, "pos.product", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// ── Shifts ───────────────────────────────────────────────────────────────────

// ShiftView is a till session with what it took.
type ShiftView struct {
	domain.CashierShift
	Totals domain.ShiftTotals `json:"totals"`
}

func (s *Service) Shifts(ctx context.Context, branchID, cashierID, status string, limit int) ([]domain.CashierShift, error) {
	return s.repo.Shifts(ctx, branchID, cashierID, status, limit)
}

func (s *Service) Shift(ctx context.Context, shiftID string) (ShiftView, error) {
	shift, err := s.repo.Shift(ctx, shiftID, false)
	if err != nil {
		return ShiftView{}, err
	}
	return s.viewShift(ctx, shift)
}

func (s *Service) viewShift(ctx context.Context, shift domain.CashierShift) (ShiftView, error) {
	payments, err := s.repo.PaymentsForShift(ctx, shift.ID)
	if err != nil {
		return ShiftView{}, err
	}
	orders, err := s.repo.Orders(ctx, OrderFilter{ShiftID: shift.ID, Limit: 500})
	if err != nil {
		return ShiftView{}, err
	}

	totals := domain.ShiftTotals{}
	for _, order := range orders {
		switch order.Status {
		case domain.POSCompleted:
			totals.Orders++
			totals.SalesIDR += order.TotalIDR
		case domain.POSVoided:
			totals.VoidedIDR += order.TotalIDR
		}
	}
	for _, payment := range payments {
		net := payment.AmountIDR - payment.ChangeIDR
		switch payment.Method {
		case domain.PayCash:
			totals.CashIDR += net
		case domain.PayQRIS:
			totals.QRISIDR += net
		case domain.PayDebit, domain.PayCredit:
			totals.CardIDR += net
		case domain.PayTransfer:
			totals.TransferIDR += net
		case domain.PayMemberCredit:
			totals.CreditIDR += net
		}
	}
	totals.ExpectedCash = domain.ExpectedCash(shift.OpeningCashIDR, payments)
	return ShiftView{CashierShift: shift, Totals: totals}, nil
}

// OpenShift starts a cashier's session. One open till per cashier per branch:
// two under one name is how cash goes missing without anybody being
// accountable for it.
func (s *Service) OpenShift(ctx context.Context, branchID string, openingCashIDR float64, note *string, actor Actor) (ShiftView, error) {
	if openingCashIDR < 0 {
		return ShiftView{}, httpx.Invalid("An opening float cannot be negative.")
	}
	created, err := s.repo.InsertShift(ctx, domain.CashierShift{
		ID: s.ids.New(id.POSShift), ShiftNumber: s.documentNumber("SHF", id.POSShift),
		CashierID: actor.ID, CashierName: actor.Name, BranchID: branchID,
		Status: domain.ShiftOpen, OpeningCashIDR: openingCashIDR, Note: note,
	})
	if err != nil {
		return ShiftView{}, err
	}
	s.record(ctx, "pos.shift", created.ID, "OPEN", actor, nil)
	return s.viewShift(ctx, created)
}

// CloseShift counts the drawer. The variance between what should be there and
// what is is the number the whole table exists to produce.
func (s *Service) CloseShift(ctx context.Context, shiftID string, countedCashIDR float64, note *string, actor Actor) (ShiftView, error) {
	if countedCashIDR < 0 {
		return ShiftView{}, httpx.Invalid("A counted drawer cannot be negative.")
	}

	var closed domain.CashierShift
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		shift, err := s.repo.Shift(ctx, shiftID, true)
		if err != nil {
			return err
		}
		if shift.Status != domain.ShiftOpen {
			return httpx.Conflict("SHIFT_CLOSED", "That till is already closed.")
		}
		open, err := s.repo.Orders(ctx, OrderFilter{
			ShiftID: shiftID, Status: string(domain.POSOpen), Limit: 10,
		})
		if err != nil {
			return err
		}
		// An unfinished sale would be stranded: the shift it belongs to would
		// be closed and its takings counted without it.
		if len(open) > 0 {
			return httpx.Conflict("ORDERS_STILL_OPEN",
				"There are %d unfinished sales on that till.", len(open))
		}

		payments, err := s.repo.PaymentsForShift(ctx, shiftID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		shift.Status = domain.ShiftClosed
		shift.ClosedAt = &now
		shift.ClosingCashIDR = &countedCashIDR
		shift.ExpectedCashIDR = domain.ExpectedCash(shift.OpeningCashIDR, payments)
		if note != nil {
			shift.Note = note
		}

		closed, err = s.repo.SaveShift(ctx, shift)
		return err
	})
	if err != nil {
		return ShiftView{}, err
	}
	s.record(ctx, "pos.shift", shiftID, "CLOSE", actor, note)
	return s.viewShift(ctx, closed)
}

// ── Scanning and price breaks ────────────────────────────────────────────────

// ScanView is what a barcode resolves to: the product, what it costs in each
// channel, and whether there is any left.
type ScanView struct {
	ProductView
	Prices []domain.ProductPrice `json:"prices"`
}

// Scan resolves a barcode to one product.
//
// A miss is a 404 rather than an empty list, because the cashier's next move
// depends on knowing the scan found nothing rather than found nothing yet.
func (s *Service) Scan(ctx context.Context, barcode, branchID string) (ScanView, error) {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return ScanView{}, httpx.Invalid("A scan needs a barcode.")
	}

	product, err := s.repo.ProductByBarcode(ctx, barcode)
	if err != nil {
		return ScanView{}, err
	}

	view := ScanView{ProductView: ProductView{POSProduct: product}}
	if product.CategoryID != nil {
		categories, err := s.repo.Categories(ctx)
		if err != nil {
			return ScanView{}, err
		}
		for _, c := range categories {
			if c.ID == *product.CategoryID {
				name := c.Name
				view.CategoryName = &name
			}
		}
	}
	// Stock is held in base units, and the till thinks in packs: five bottles
	// on hand is not one six-pack, and saying so is the point of dividing.
	if product.InventoryItemID != nil && branchID != "" {
		if onHand, err := s.stock.OnHand(ctx, *product.InventoryItemID, branchID); err == nil {
			held := domain.BaseToPack(onHand, product.PackFactor)
			view.OnHand = &held
		}
	}
	prices, err := s.repo.ProductPrices(ctx, product.ID)
	if err != nil {
		return ScanView{}, err
	}
	view.Prices = prices
	return view, nil
}

func (s *Service) ProductPrices(ctx context.Context, productID string) ([]domain.ProductPrice, error) {
	return s.repo.ProductPrices(ctx, productID)
}

// ProductPriceInput is one price break.
type ProductPriceInput struct {
	ProductID string
	Channel   domain.SalesChannel
	MinQty    domain.Quantity
	PriceIDR  float64
	Active    bool
}

func (s *Service) SaveProductPrice(ctx context.Context, in ProductPriceInput, actor Actor) (domain.ProductPrice, error) {
	saved, err := s.repo.UpsertProductPrice(ctx, domain.ProductPrice{
		ID: s.ids.New(id.ProductPrice), ProductID: in.ProductID, Channel: in.Channel,
		MinQty: in.MinQty, PriceIDR: in.PriceIDR, Active: in.Active,
	})
	if err != nil {
		return domain.ProductPrice{}, err
	}
	s.record(ctx, "pos.price", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

func (s *Service) DeleteProductPrice(ctx context.Context, priceID string, actor Actor) error {
	if err := s.repo.DeleteProductPrice(ctx, priceID); err != nil {
		return err
	}
	s.record(ctx, "pos.price", priceID, "DELETE", actor, nil)
	return nil
}

// SupervisorRef is who a PIN turned out to belong to.
type SupervisorRef struct {
	ID   string
	Name string
}
