// Package inventory is what the studio has on its shelves.
//
// It owns one thing that matters: the only code path that changes a stock
// quantity. Purchasing receiving goods, the till selling a shirt, a manager
// correcting a miscount — all of them come through Post, so "you cannot take
// out what is not there" is implemented once, the movement ledger is always
// written, and the average cost is always maintained.
package inventory

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

// Catalog is the port for the branches stock is held at. Inventory names
// them; it never writes them.
type Catalog interface {
	Branches(ctx context.Context) ([]domain.Branch, error)
}

// Service implements the stock use cases.
type Service struct {
	db      *database.DB
	repo    *Repository
	catalog Catalog
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
	studio  *time.Location
}

func NewService(db *database.DB, repo *Repository, catalog Catalog,
	ids id.Generator, c clock.Clock, auditor audit.Recorder, studio *time.Location) *Service {
	if studio == nil {
		studio = time.UTC
	}
	return &Service{db: db, repo: repo, catalog: catalog, ids: ids, clock: c,
		auditor: auditor, studio: studio}
}

// shortCode is the tail of a generated id, used to make a human-quotable
// document number. The prefix is dropped because "TRF-20260908-strf_06G8" says
// the same thing twice.
func shortCode(generated string) string {
	if _, tail, found := strings.Cut(generated, "_"); found && len(tail) >= 6 {
		return tail[len(tail)-6:]
	}
	return generated
}

// Actor identifies who performed an administrative action.
type Actor struct {
	ID   string
	Name string
}

func (s *Service) record(ctx context.Context, entity, entityID, action string, actor Actor, reason *string) {
	_ = s.auditor.Record(ctx, audit.Event{
		EntityType: entity, EntityID: entityID, Action: action,
		ActorID: actor.ID, ActorName: actor.Name, Reason: reason,
	})
}

// ── The one place stock changes ──────────────────────────────────────────────

// Movement is a request to change a quantity.
//
// Qty is signed, and the caller decides the sign, because a RETURN is stock
// leaving when it goes back to a supplier and stock arriving when a member
// brings a shirt back. The kind only explains why.
type Movement struct {
	ItemID   string
	BranchID string
	Kind     domain.MovementKind
	Qty      domain.Quantity
	// UnitCostIDR is what arriving stock cost. Ignored on anything leaving,
	// which is valued at the average it already carries.
	UnitCostIDR     float64
	ReferenceType   *string
	ReferenceID     *string
	ReferenceNumber *string
	// PackUnit and PackFactor record what was handled, when the caller dealt
	// in something other than base units.
	PackUnit   *string
	PackFactor *float64
	// BatchCode and ExpiresOn name the batch stock is arriving into. They are
	// required for an item that tracks batches and ignored for one that does
	// not.
	BatchCode string
	ExpiresOn *domain.Date
	// BatchID names a specific batch for stock leaving, which ordinarily FEFO
	// chooses. It exists for the one case where the batch *is* the reason —
	// writing off a batch that has expired, which FEFO deliberately refuses
	// to touch.
	BatchID string
	// RestoresReferenceType and RestoresReferenceID identify a movement being
	// undone. Goods coming back from a voided sale go into the batches that
	// sale took them out of — asking the cashier to type a batch code off a
	// receipt would be asking them to guess, and inventing a new batch would
	// quietly give returned stock a fresh expiry date.
	RestoresReferenceType string
	RestoresReferenceID   string
	Reason                *string
	Note                  *string
}

// Post applies one movement: it locks the level, asks the domain whether the
// change is legal, writes the ledger row, saves the new quantity and updates
// the item's average cost.
//
// It must be called inside a transaction — every caller has something else to
// commit alongside it, and a stock change that outlives its reason is worse
// than no stock change at all.
func (s *Service) Post(ctx context.Context, in Movement, actor Actor) (domain.StockMovement, error) {
	item, err := s.repo.Item(ctx, in.ItemID, true)
	if err != nil {
		return domain.StockMovement{}, err
	}
	level, err := s.repo.Level(ctx, in.ItemID, in.BranchID, true)
	if err != nil {
		return domain.StockMovement{}, err
	}

	posted := domain.PostMovement(item, level, in.Kind, in.Qty, in.UnitCostIDR)
	if !posted.Allowed() {
		return domain.StockMovement{}, movementRejectionError(posted.Rejection, item, level, in.Qty)
	}

	movement := domain.StockMovement{
		ID: s.ids.New(id.StockMovement), ItemID: in.ItemID, BranchID: in.BranchID,
		Kind: posted.Kind, Qty: posted.Qty, QtyBefore: posted.QtyBefore, QtyAfter: posted.QtyAfter,
		UnitCostIDR: in.UnitCostIDR, TotalCostIDR: posted.TotalCostIDR,
		ReferenceType: in.ReferenceType, ReferenceID: in.ReferenceID,
		ReferenceNumber: in.ReferenceNumber, Reason: in.Reason, Note: in.Note,
	}
	// The pack quantity is derived from the base one rather than passed in, so
	// the two can never contradict each other on the way to a constraint that
	// would reject them both.
	if in.PackUnit != nil && in.PackFactor != nil && *in.PackFactor > 0 {
		packQty := domain.BaseToPack(posted.Qty, *in.PackFactor)
		if packQty < 0 {
			packQty = -packQty
		}
		movement.PackUnit = in.PackUnit
		movement.PackQty = &packQty
		movement.PackFactor = in.PackFactor
	}
	if actor.ID != "" {
		movement.ActorID = &actor.ID
		movement.ActorName = &actor.Name
	}
	if posted.Qty < 0 {
		movement.UnitCostIDR = item.UnitCostIDR
	}

	saved, err := s.repo.InsertMovement(ctx, movement)
	if err != nil {
		return domain.StockMovement{}, err
	}

	// Dated goods are also placed on, or taken off, a specific batch. The
	// allocation is written inside the same transaction as the movement it
	// explains, so batch quantities and the ledger can never disagree about
	// what happened.
	if item.TrackBatches {
		if err := s.applyBatches(ctx, item, in, posted, saved); err != nil {
			return domain.StockMovement{}, err
		}
	}

	level.QtyOnHand = posted.QtyAfter
	if _, err := s.repo.SaveLevel(ctx, level, true); err != nil {
		return domain.StockMovement{}, err
	}
	if posted.UnitCostAfter != item.UnitCostIDR {
		if err := s.repo.SetUnitCost(ctx, item.ID, posted.UnitCostAfter); err != nil {
			return domain.StockMovement{}, err
		}
	}
	return saved, nil
}

// applyBatches puts arriving stock into a named batch, and takes leaving stock
// out of whichever batches expire soonest.
//
// FEFO, not FIFO: goods that arrived later can easily expire sooner, and
// issuing in arrival order leaves short-dated stock at the back of the shelf
// until it is worthless.
func (s *Service) applyBatches(ctx context.Context, item domain.InventoryItem,
	in Movement, posted domain.PostedMovement, movement domain.StockMovement) error {

	if posted.Qty > 0 {
		// Goods coming back from something that already happened go where
		// they came from.
		if in.RestoresReferenceID != "" {
			restored, err := s.restoreToOriginBatches(ctx, item, in, posted, movement)
			if err != nil || restored {
				return err
			}
		}

		code := strings.TrimSpace(in.BatchCode)
		if code == "" {
			return httpx.Invalid(
				"%s is tracked by batch, so goods arriving have to say which batch.", item.Name)
		}
		cost := in.UnitCostIDR
		if cost <= 0 {
			cost = item.UnitCostIDR
		}
		batch, err := s.repo.UpsertBatch(ctx, domain.Batch{
			ID: s.ids.New(id.Batch), ItemID: item.ID, BranchID: in.BranchID,
			BatchCode: code, ExpiresOn: in.ExpiresOn, QtyOnHand: posted.Qty,
			UnitCostIDR: cost, ReceiptID: in.ReferenceID, ReceiptNumber: in.ReferenceNumber,
		})
		if err != nil {
			return err
		}
		// The batch row is already updated by the upsert, so the allocation
		// records what it did rather than doing it again.
		return s.repo.RecordBatchArrival(ctx, movement.ID, batch, posted.Qty, s.ids.New(id.BatchMovement))
	}

	batches, err := s.repo.BatchesForIssue(ctx, item.ID, in.BranchID)
	if err != nil {
		return err
	}

	// A named batch bypasses FEFO. This is the write-off path: the goods being
	// removed are expired, and FEFO refuses expired batches by design.
	if in.BatchID != "" {
		for _, batch := range batches {
			if batch.ID != in.BatchID {
				continue
			}
			take := -posted.Qty
			if take > batch.QtyOnHand {
				return httpx.Conflict("BATCH_SHORTFALL",
					"Batch %s holds %v, which is less than the %v being taken from it.",
					batch.BatchCode, batch.QtyOnHand, take)
			}
			return s.repo.DrawFromBatch(ctx, movement.ID, domain.Allocation{
				BatchID: batch.ID, BatchCode: batch.BatchCode, ExpiresOn: batch.ExpiresOn,
				Qty: posted.Qty, QtyBefore: batch.QtyOnHand,
				QtyAfter:    domain.RoundQuantity(batch.QtyOnHand - take),
				UnitCostIDR: batch.UnitCostIDR,
			}, s.ids.New(id.BatchMovement))
		}
		return httpx.NotFound("batch")
	}

	today := domain.DateOf(s.clock.Now(), s.studio)
	outcome := domain.AllocateFEFO(batches, -posted.Qty, today)

	if !outcome.Allocated() {
		// The level said there was enough and the batches say there is not.
		// When the difference is expired stock, that is the honest answer —
		// it is on the shelf and may not be sold.
		if len(outcome.Expired) > 0 {
			return httpx.Conflict("EXPIRED_STOCK",
				"%s has %v short of unexpired stock: %d batch(es) are past their date and cannot be sold.",
				item.Name, outcome.Shortfall, len(outcome.Expired))
		}
		return httpx.Conflict("BATCH_SHORTFALL",
			"%s is %v short across its batches, which disagrees with its stock level.",
			item.Name, outcome.Shortfall)
	}

	for _, allocation := range outcome.Allocations {
		if err := s.repo.DrawFromBatch(ctx, movement.ID, allocation, s.ids.New(id.BatchMovement)); err != nil {
			return err
		}
	}
	return nil
}

// restoreToOriginBatches puts returned goods back into the batches they were
// taken from, newest allocation first.
//
// It reports whether it found an origin at all: a return against a movement
// that predates batch tracking has nowhere to go back to, and falls through to
// the ordinary path rather than failing.
func (s *Service) restoreToOriginBatches(ctx context.Context, item domain.InventoryItem,
	in Movement, posted domain.PostedMovement, movement domain.StockMovement) (bool, error) {

	origins, err := s.repo.OutwardAllocations(ctx, item.ID, in.BranchID,
		in.RestoresReferenceType, in.RestoresReferenceID)
	if err != nil {
		return false, err
	}
	if len(origins) == 0 {
		return false, nil
	}

	remaining := posted.Qty
	for _, origin := range origins {
		if remaining <= 0 {
			break
		}
		// Never put back more than that batch gave: a sale of 3 from one
		// batch and 2 from another, voided, is 3 and 2 — not 5 into the first.
		give := domain.RoundQuantity(-origin.Qty)
		if give > remaining {
			give = remaining
		}
		if give <= 0 {
			continue
		}
		before, err := s.repo.BatchQty(ctx, origin.BatchID)
		if err != nil {
			return false, err
		}
		if err := s.repo.DrawFromBatch(ctx, movement.ID, domain.Allocation{
			BatchID: origin.BatchID, BatchCode: origin.BatchCode, ExpiresOn: origin.ExpiresOn,
			Qty: give, QtyBefore: before, QtyAfter: domain.RoundQuantity(before + give),
			UnitCostIDR: origin.UnitCostIDR,
		}, s.ids.New(id.BatchMovement)); err != nil {
			return false, err
		}
		remaining = domain.RoundQuantity(remaining - give)
	}

	if remaining > 0 {
		return false, httpx.Conflict("MORE_THAN_LEFT",
			"%v of %s is being returned against a movement that only took out %v.",
			posted.Qty, item.Name, posted.Qty-remaining)
	}
	return true, nil
}

// movementRejectionError turns a domain refusal into the HTTP answer, keeping
// the codes stable for the UI and the domain free of transport concerns.
func movementRejectionError(rejection domain.StockRejection, item domain.InventoryItem,
	level domain.StockLevel, qty domain.Quantity) error {
	switch rejection {
	case domain.StockRejectInsufficient:
		return httpx.Conflict("INSUFFICIENT_STOCK",
			"%s has %v %s left; that movement needs %v.",
			item.Name, level.QtyOnHand, strings.ToLower(item.Unit), -qty)
	case domain.StockRejectZeroQty:
		return httpx.Invalid("A movement of nothing is not a movement.")
	case domain.StockRejectNotTracked:
		return httpx.Conflict("NOT_TRACKED", "%s is not stock-tracked.", item.Name)
	case domain.StockRejectInactive:
		return httpx.Conflict("ITEM_INACTIVE", "%s is no longer in the catalogue.", item.Name)
	default:
		return httpx.Invalid("That movement cannot be posted.")
	}
}

// Receive is the entry point purchasing uses when goods arrive.
func (s *Service) Receive(ctx context.Context, itemID, branchID string, qty domain.Quantity,
	unitCostIDR float64, ref Reference, actor Actor) (domain.StockMovement, error) {
	movement := Movement{
		ItemID: itemID, BranchID: branchID, Kind: domain.MovementIn, Qty: qty,
		UnitCostIDR: unitCostIDR, ReferenceType: &ref.Type, ReferenceID: &ref.ID,
		ReferenceNumber: &ref.Number,
		BatchCode:       ref.BatchCode, ExpiresOn: ref.ExpiresOn,
	}
	ref.applyPack(&movement)
	return s.Post(ctx, movement, actor)
}

// Restore is the entry point for goods coming back from something that already
// happened: a voided sale, a reversed issue.
//
// It differs from Receive in one way that matters for dated stock — it puts
// the goods back into the batches they came out of, rather than asking for a
// batch code the person unwinding a receipt has no way to know.
func (s *Service) Restore(ctx context.Context, itemID, branchID string, qty domain.Quantity,
	unitCostIDR float64, ref Reference, actor Actor) (domain.StockMovement, error) {
	movement := Movement{
		ItemID: itemID, BranchID: branchID, Kind: domain.MovementIn, Qty: qty,
		UnitCostIDR: unitCostIDR, ReferenceType: &ref.Type, ReferenceID: &ref.ID,
		ReferenceNumber: &ref.Number,
		BatchCode:       ref.BatchCode, ExpiresOn: ref.ExpiresOn,
		RestoresReferenceType: ref.RestoresType, RestoresReferenceID: ref.RestoresID,
	}
	ref.applyPack(&movement)
	return s.Post(ctx, movement, actor)
}

// Issue is the entry point the till uses when something is sold, and the one
// purchasing uses when goods go back to a supplier.
func (s *Service) Issue(ctx context.Context, itemID, branchID string, qty domain.Quantity,
	kind domain.MovementKind, ref Reference, actor Actor) (domain.StockMovement, error) {
	if qty < 0 {
		qty = -qty
	}
	movement := Movement{
		ItemID: itemID, BranchID: branchID, Kind: kind, Qty: -qty,
		ReferenceType: &ref.Type, ReferenceID: &ref.ID, ReferenceNumber: &ref.Number,
	}
	ref.applyPack(&movement)
	return s.Post(ctx, movement, actor)
}

// Reference is what caused a movement, so the ledger row can be traced back.
//
// PackUnit and PackFactor say what was physically handled. The ledger stays in
// base units — that is what makes it addable — but a receipt line reading
// "240" is unreadable next to a delivery note saying ten cartons, so the row
// carries both and a CHECK constraint refuses them if they disagree.
type Reference struct {
	Type       string
	ID         string
	Number     string
	PackUnit   string
	PackFactor float64
	// BatchCode and ExpiresOn come off the delivery note, for goods with a
	// date on them. Ignored by an item that does not track batches.
	BatchCode string
	ExpiresOn *domain.Date
	// RestoresType and RestoresID identify a movement being undone, so
	// returned goods go back into the batches they left from.
	RestoresType string
	RestoresID   string
}

// applyPack copies the transacted pack onto a movement, when there is one.
func (r Reference) applyPack(m *Movement) {
	if r.PackUnit == "" || r.PackFactor <= 0 {
		return
	}
	unit, factor := r.PackUnit, r.PackFactor
	m.PackUnit, m.PackFactor = &unit, &factor
}

// ReserveOnOrder and ReleaseOnOrder are how purchasing tells stock that goods
// are coming, so the reorder screen does not raise a second order for them.
func (s *Service) ReserveOnOrder(ctx context.Context, itemID, branchID string, qty domain.Quantity) error {
	return s.repo.AdjustOnOrder(ctx, itemID, branchID, qty)
}

func (s *Service) ReleaseOnOrder(ctx context.Context, itemID, branchID string, qty domain.Quantity) error {
	return s.repo.AdjustOnOrder(ctx, itemID, branchID, -qty)
}

// ── Catalogue ────────────────────────────────────────────────────────────────

func (s *Service) Categories(ctx context.Context) ([]domain.InventoryCategory, error) {
	return s.repo.Categories(ctx)
}

// CategoryInput creates or replaces a category.
type CategoryInput struct {
	Name      string
	Code      string
	Active    bool
	SortOrder int
}

func (s *Service) CreateCategory(ctx context.Context, in CategoryInput, actor Actor) (domain.InventoryCategory, error) {
	created, err := s.repo.InsertCategory(ctx, domain.InventoryCategory{
		ID: s.ids.New(id.InventoryCategory), Name: in.Name,
		Code: strings.ToUpper(in.Code), Active: in.Active, SortOrder: in.SortOrder,
	})
	if err != nil {
		return domain.InventoryCategory{}, err
	}
	s.record(ctx, "inventory.category", created.ID, "CREATE", actor, nil)
	return created, nil
}

func (s *Service) UpdateCategory(ctx context.Context, categoryID string, in CategoryInput, actor Actor) (domain.InventoryCategory, error) {
	updated, err := s.repo.UpdateCategory(ctx, domain.InventoryCategory{
		ID: categoryID, Name: in.Name, Code: strings.ToUpper(in.Code),
		Active: in.Active, SortOrder: in.SortOrder,
	})
	if err != nil {
		return domain.InventoryCategory{}, err
	}
	s.record(ctx, "inventory.category", categoryID, "UPDATE", actor, nil)
	return updated, nil
}

func (s *Service) DeleteCategory(ctx context.Context, categoryID string, actor Actor) error {
	if err := s.repo.DeleteCategory(ctx, categoryID); err != nil {
		return err
	}
	s.record(ctx, "inventory.category", categoryID, "DELETE", actor, nil)
	return nil
}

// ItemView is an item with its category named and its stock totalled across
// branches, which is what a catalogue row wants to show.
type ItemView struct {
	domain.InventoryItem
	CategoryName *string         `json:"categoryName"`
	TotalOnHand  domain.Quantity `json:"totalOnHand"`
	Levels       []LevelView     `json:"levels,omitempty"`
}

// LevelView is one branch's shelf.
type LevelView struct {
	domain.StockLevel
	BranchName string          `json:"branchName"`
	LowStock   bool            `json:"lowStock"`
	ReorderQty domain.Quantity `json:"reorderQty"`
	ValueIDR   float64         `json:"valueIdr"`
}

func (s *Service) branchNames(ctx context.Context) (map[string]string, error) {
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(branches))
	for _, b := range branches {
		names[b.ID] = b.Name
	}
	return names, nil
}

func (s *Service) Items(ctx context.Context, filter ItemFilter) ([]ItemView, error) {
	items, err := s.repo.Items(ctx, filter)
	if err != nil {
		return nil, err
	}
	categories, err := s.repo.Categories(ctx)
	if err != nil {
		return nil, err
	}
	categoryNames := map[string]string{}
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	// One query for every level, rather than one per item.
	levels, err := s.repo.Levels(ctx, LevelFilter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	onHand := map[string]domain.Quantity{}
	for _, row := range levels {
		onHand[row.Level.ItemID] += row.Level.QtyOnHand
	}

	views := make([]ItemView, 0, len(items))
	for _, item := range items {
		view := ItemView{InventoryItem: item, TotalOnHand: domain.RoundQuantity(onHand[item.ID])}
		if item.CategoryID != nil {
			if name, ok := categoryNames[*item.CategoryID]; ok {
				view.CategoryName = &name
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// ItemDetail is one item's page: what it is, where it is, and its history.
type ItemDetail struct {
	Item      ItemView               `json:"item"`
	Levels    []LevelView            `json:"levels"`
	Movements []domain.StockMovement `json:"movements"`
}

func (s *Service) ItemDetail(ctx context.Context, itemID string) (ItemDetail, error) {
	item, err := s.repo.Item(ctx, itemID, false)
	if err != nil {
		return ItemDetail{}, err
	}
	categories, err := s.repo.Categories(ctx)
	if err != nil {
		return ItemDetail{}, err
	}
	view := ItemView{InventoryItem: item}
	for _, c := range categories {
		if item.CategoryID != nil && c.ID == *item.CategoryID {
			name := c.Name
			view.CategoryName = &name
		}
	}

	branches, err := s.branchNames(ctx)
	if err != nil {
		return ItemDetail{}, err
	}
	rows, err := s.repo.Levels(ctx, LevelFilter{Limit: 1000})
	if err != nil {
		return ItemDetail{}, err
	}
	levels := []LevelView{}
	for _, row := range rows {
		if row.Level.ItemID != itemID {
			continue
		}
		view.TotalOnHand += row.Level.QtyOnHand
		levels = append(levels, LevelView{
			StockLevel: row.Level, BranchName: branches[row.Level.BranchID],
			LowStock:   domain.IsLowStock(row.Level),
			ReorderQty: domain.ReorderQuantity(row.Level),
			ValueIDR:   domain.StockValue(item, row.Level),
		})
	}
	view.TotalOnHand = domain.RoundQuantity(view.TotalOnHand)

	movements, err := s.repo.Movements(ctx, MovementFilter{ItemID: itemID, Limit: 50})
	if err != nil {
		return ItemDetail{}, err
	}
	return ItemDetail{Item: view, Levels: levels, Movements: movements}, nil
}

// ItemInput is a whole catalogue entry. The average cost is not in it: that is
// derived from receipts, never typed.
type ItemInput struct {
	SKU               string
	Name              string
	Description       string
	CategoryID        *string
	Unit              string
	Kind              domain.ItemKind
	TrackStock        bool
	TrackBatches      bool
	ExpiryWarningDays int
	Barcode           *string
	ImageURL          *string
	Active            bool
}

func (in ItemInput) apply(item domain.InventoryItem) domain.InventoryItem {
	item.SKU = strings.ToUpper(strings.TrimSpace(in.SKU))
	item.Name = strings.TrimSpace(in.Name)
	item.Description = in.Description
	item.CategoryID = in.CategoryID
	item.Unit = strings.ToUpper(strings.TrimSpace(in.Unit))
	item.Kind = in.Kind
	item.TrackStock = in.TrackStock
	item.TrackBatches = in.TrackBatches
	item.ExpiryWarningDays = in.ExpiryWarningDays
	if item.ExpiryWarningDays <= 0 {
		item.ExpiryWarningDays = 30
	}
	item.Barcode = in.Barcode
	item.ImageURL = in.ImageURL
	item.Active = in.Active
	return item
}

func (s *Service) CreateItem(ctx context.Context, in ItemInput, actor Actor) (ItemView, error) {
	var created domain.InventoryItem
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		var err error
		created, err = s.repo.InsertItem(ctx, in.apply(domain.InventoryItem{ID: s.ids.New(id.InventoryItem)}))
		if err != nil {
			return err
		}
		// An item without a base pack has quantities that mean nothing, so it
		// gets one in the same transaction it is created in. Every other pack
		// is a multiple of this one.
		_, err = s.repo.UpsertPack(ctx, domain.ItemPack{
			ID: s.ids.New(id.ItemPack), ItemID: created.ID, UnitCode: created.Unit,
			Factor: 1, Barcode: created.Barcode, IsBase: true,
			PurchaseDefault: true, SaleDefault: true, Active: true,
		})
		return err
	})
	if err != nil {
		return ItemView{}, err
	}
	s.record(ctx, "inventory.item", created.ID, "CREATE", actor, nil)
	return ItemView{InventoryItem: created}, nil
}

func (s *Service) UpdateItem(ctx context.Context, itemID string, in ItemInput, actor Actor) (ItemView, error) {
	existing, err := s.repo.Item(ctx, itemID, false)
	if err != nil {
		return ItemView{}, err
	}
	updated, err := s.repo.UpdateItem(ctx, in.apply(existing))
	if err != nil {
		return ItemView{}, err
	}
	s.record(ctx, "inventory.item", itemID, "UPDATE", actor, nil)
	return ItemView{InventoryItem: updated}, nil
}

// ── Stock ────────────────────────────────────────────────────────────────────

// StockRow is one shelf, ready to render.
type StockRow struct {
	Item  domain.InventoryItem `json:"item"`
	Level LevelView            `json:"level"`
}

func (s *Service) Stock(ctx context.Context, filter LevelFilter) ([]StockRow, error) {
	rows, err := s.repo.Levels(ctx, filter)
	if err != nil {
		return nil, err
	}
	branches, err := s.branchNames(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]StockRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, StockRow{
			Item: row.Item,
			Level: LevelView{
				StockLevel: row.Level, BranchName: branches[row.Level.BranchID],
				LowStock:   domain.IsLowStock(row.Level),
				ReorderQty: domain.ReorderQuantity(row.Level),
				ValueIDR:   domain.StockValue(row.Item, row.Level),
			},
		})
	}
	return out, nil
}

// ReorderInput sets the levels at which an item is considered low.
type ReorderInput struct {
	QtyMinimum  domain.Quantity
	QtyMaximum  *domain.Quantity
	BinLocation *string
	Note        *string
}

func (s *Service) SetReorderPoint(ctx context.Context, itemID, branchID string, in ReorderInput, actor Actor) (LevelView, error) {
	if in.QtyMaximum != nil && *in.QtyMaximum < in.QtyMinimum {
		return LevelView{}, httpx.Invalid("The maximum cannot be below the minimum.")
	}

	var saved domain.StockLevel
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		level, err := s.repo.Level(ctx, itemID, branchID, true)
		if err != nil {
			return err
		}
		level.QtyMinimum = in.QtyMinimum
		level.QtyMaximum = in.QtyMaximum
		level.BinLocation = in.BinLocation
		level.Note = in.Note
		saved, err = s.repo.SaveLevel(ctx, level, false)
		return err
	})
	if err != nil {
		return LevelView{}, err
	}
	s.record(ctx, "inventory.item", itemID, "REORDER_POINT", actor, nil)

	branches, err := s.branchNames(ctx)
	if err != nil {
		return LevelView{}, err
	}
	item, err := s.repo.Item(ctx, itemID, false)
	if err != nil {
		return LevelView{}, err
	}
	return LevelView{
		StockLevel: saved, BranchName: branches[saved.BranchID],
		LowStock:   domain.IsLowStock(saved),
		ReorderQty: domain.ReorderQuantity(saved),
		ValueIDR:   domain.StockValue(item, saved),
	}, nil
}

// AdjustInput is a manual correction. A reason is required, because an
// unexplained change to a quantity is indistinguishable from theft.
type AdjustInput struct {
	ItemID   string
	BranchID string
	Qty      domain.Quantity
	Reason   string
	// BatchCode and ExpiresOn are needed when dated stock is being added by
	// hand — a surplus found at stocktake is a physical pile with a date on
	// it, and the system has no way to guess which. Stock being removed does
	// not name a batch: FEFO decides, same as a sale.
	BatchCode string
	ExpiresOn *domain.Date
	Note      *string
}

func (s *Service) Adjust(ctx context.Context, in AdjustInput, actor Actor) (domain.StockMovement, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return domain.StockMovement{}, httpx.Invalid("An adjustment needs a reason.")
	}

	var movement domain.StockMovement
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		var err error
		movement, err = s.Post(ctx, Movement{
			ItemID: in.ItemID, BranchID: in.BranchID, Kind: domain.MovementAdjustment,
			Qty: in.Qty, BatchCode: in.BatchCode, ExpiresOn: in.ExpiresOn,
			Reason: &in.Reason, Note: in.Note,
		}, actor)
		return err
	})
	if err != nil {
		return domain.StockMovement{}, err
	}
	s.record(ctx, "inventory.item", in.ItemID, "ADJUST", actor, &in.Reason)
	return movement, nil
}

func (s *Service) Movements(ctx context.Context, filter MovementFilter) ([]domain.StockMovement, error) {
	return s.repo.Movements(ctx, filter)
}

// ── Transfers ────────────────────────────────────────────────────────────────

// TransferInput moves stock between branches.
type TransferInput struct {
	ItemID       string
	FromBranchID string
	ToBranchID   string
	Qty          domain.Quantity
	Note         *string
}

// Transfer is two movements that must both happen or neither. The transfer row
// is what makes them one act rather than two hopeful ones.
func (s *Service) Transfer(ctx context.Context, in TransferInput, actor Actor) (domain.StockTransfer, error) {
	if in.Qty <= 0 {
		return domain.StockTransfer{}, httpx.Invalid("A transfer needs a positive quantity.")
	}
	if in.FromBranchID == in.ToBranchID {
		return domain.StockTransfer{}, httpx.Invalid("A transfer needs two different branches.")
	}

	var transfer domain.StockTransfer
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		number := fmt.Sprintf("TRF-%s-%s",
			s.clock.Now().In(s.studio).Format("20060102"), shortCode(s.ids.New(id.StockTransfer)))

		var err error
		transfer, err = s.repo.InsertTransfer(ctx, domain.StockTransfer{
			ID: s.ids.New(id.StockTransfer), TransferNumber: number, ItemID: in.ItemID,
			FromBranchID: in.FromBranchID, ToBranchID: in.ToBranchID, Qty: in.Qty,
			Note: in.Note, ActorID: &actor.ID, ActorName: &actor.Name,
		})
		if err != nil {
			return err
		}

		ref := Reference{Type: "STOCK_TRANSFER", ID: transfer.ID, Number: number}
		// Out of the source first: if there is not enough, nothing else has
		// happened yet and the whole transaction unwinds.
		if _, err := s.Post(ctx, Movement{
			ItemID: in.ItemID, BranchID: in.FromBranchID, Kind: domain.MovementTransferOut,
			Qty: -in.Qty, ReferenceType: &ref.Type, ReferenceID: &ref.ID, ReferenceNumber: &ref.Number,
		}, actor); err != nil {
			return err
		}

		item, err := s.repo.Item(ctx, in.ItemID, false)
		if err != nil {
			return err
		}
		// Arriving at its existing average cost: moving stock between shelves
		// does not change what it cost to buy.
		_, err = s.Post(ctx, Movement{
			ItemID: in.ItemID, BranchID: in.ToBranchID, Kind: domain.MovementTransferIn,
			Qty: in.Qty, UnitCostIDR: item.UnitCostIDR,
			ReferenceType: &ref.Type, ReferenceID: &ref.ID, ReferenceNumber: &ref.Number,
		}, actor)
		return err
	})
	if err != nil {
		return domain.StockTransfer{}, err
	}
	s.record(ctx, "inventory.item", in.ItemID, "TRANSFER", actor, in.Note)
	return transfer, nil
}

func (s *Service) Transfers(ctx context.Context, itemID string, limit int) ([]domain.StockTransfer, error) {
	return s.repo.Transfers(ctx, itemID, limit)
}

// ── Valuation ────────────────────────────────────────────────────────────────

// Overview is the stockroom dashboard.
type Overview struct {
	Valuation Valuation              `json:"valuation"`
	LowStock  []StockRow             `json:"lowStock"`
	Recent    []domain.StockMovement `json:"recent"`
}

func (s *Service) Overview(ctx context.Context, branchID string) (Overview, error) {
	valuation, err := s.repo.Valuation(ctx, branchID)
	if err != nil {
		return Overview{}, err
	}
	low, err := s.Stock(ctx, LevelFilter{BranchID: branchID, LowOnly: true, Limit: 20})
	if err != nil {
		return Overview{}, err
	}
	recent, err := s.repo.Movements(ctx, MovementFilter{BranchID: branchID, Limit: 15})
	if err != nil {
		return Overview{}, err
	}
	return Overview{Valuation: valuation, LowStock: low, Recent: recent}, nil
}

// ── Stock takes ──────────────────────────────────────────────────────────────

// StockTakeView is a count with its lines and what they add up to.
type StockTakeView struct {
	domain.StockTake
	BranchName string                  `json:"branchName"`
	Lines      []StockTakeLineView     `json:"lines"`
	Summary    domain.StockTakeSummary `json:"summary"`
}

// StockTakeLineView is one counted item.
type StockTakeLineView struct {
	domain.StockTakeLine
	ItemName string          `json:"itemName"`
	SKU      string          `json:"sku"`
	Unit     string          `json:"unit"`
	Variance domain.Quantity `json:"variance"`
	ValueIDR float64         `json:"valueIdr"`
}

func (s *Service) StockTakes(ctx context.Context, branchID, status string, limit int) ([]domain.StockTake, error) {
	return s.repo.StockTakes(ctx, branchID, status, limit)
}

func (s *Service) StockTake(ctx context.Context, takeID string) (StockTakeView, error) {
	take, err := s.repo.StockTake(ctx, takeID, false)
	if err != nil {
		return StockTakeView{}, err
	}
	return s.viewStockTake(ctx, take)
}

func (s *Service) viewStockTake(ctx context.Context, take domain.StockTake) (StockTakeView, error) {
	lines, err := s.repo.StockTakeLines(ctx, take.ID)
	if err != nil {
		return StockTakeView{}, err
	}
	items, err := s.repo.Items(ctx, ItemFilter{})
	if err != nil {
		return StockTakeView{}, err
	}
	byID := map[string]domain.InventoryItem{}
	costs := map[string]float64{}
	for _, item := range items {
		byID[item.ID] = item
		costs[item.ID] = item.UnitCostIDR
	}
	branches, err := s.branchNames(ctx)
	if err != nil {
		return StockTakeView{}, err
	}

	views := make([]StockTakeLineView, 0, len(lines))
	for _, line := range lines {
		item := byID[line.ItemID]
		views = append(views, StockTakeLineView{
			StockTakeLine: line, ItemName: item.Name, SKU: item.SKU, Unit: item.Unit,
			Variance: line.Variance(),
			ValueIDR: float64(line.Variance()) * item.UnitCostIDR,
		})
	}
	return StockTakeView{
		StockTake: take, BranchName: branches[take.BranchID],
		Lines: views, Summary: domain.SummarizeStockTake(lines, costs),
	}, nil
}

// OpenStockTake starts a count for a branch.
func (s *Service) OpenStockTake(ctx context.Context, branchID string, countedOn domain.Date, note *string, actor Actor) (StockTakeView, error) {
	if countedOn == "" {
		countedOn = domain.DateOf(s.clock.Now(), s.studio)
	}
	number := fmt.Sprintf("STK-%s-%s", strings.ReplaceAll(string(countedOn), "-", ""),
		shortCode(s.ids.New(id.StockTake)))

	take, err := s.repo.InsertStockTake(ctx, domain.StockTake{
		ID: s.ids.New(id.StockTake), TakeNumber: number, BranchID: branchID,
		Status: domain.StockTakeDraft, CountedOn: countedOn, Note: note,
	})
	if err != nil {
		return StockTakeView{}, err
	}
	s.record(ctx, "inventory.stock_take", take.ID, "OPEN", actor, note)
	return s.viewStockTake(ctx, take)
}

// CountInput records what was physically on the shelf.
type CountInput struct {
	ItemID     string
	QtyCounted domain.Quantity
	Note       *string
}

// Count adds or corrects a line. The expected quantity is frozen the first
// time an item is counted, so the variance is measured against what the system
// believed at counting time rather than whatever it says by the time somebody
// gets round to applying it.
func (s *Service) Count(ctx context.Context, takeID string, in CountInput, actor Actor) (StockTakeView, error) {
	take, err := s.repo.StockTake(ctx, takeID, false)
	if err != nil {
		return StockTakeView{}, err
	}
	if take.Status != domain.StockTakeDraft {
		return StockTakeView{}, httpx.Conflict("INVALID_TRANSITION",
			"That count is %s and cannot be changed.", strings.ToLower(string(take.Status)))
	}
	if in.QtyCounted < 0 {
		return StockTakeView{}, httpx.Invalid("A counted quantity cannot be negative.")
	}

	existing, err := s.repo.StockTakeLines(ctx, takeID)
	if err != nil {
		return StockTakeView{}, err
	}
	line := domain.StockTakeLine{
		ID: s.ids.New(id.StockTakeLine), StockTakeID: takeID, ItemID: in.ItemID,
		QtyCounted: in.QtyCounted, Note: in.Note,
	}
	found := false
	for _, l := range existing {
		if l.ItemID == in.ItemID {
			line.ID = l.ID
			line.QtyExpected = l.QtyExpected
			found = true
			break
		}
	}
	if !found {
		level, err := s.repo.Level(ctx, in.ItemID, take.BranchID, false)
		if err != nil {
			return StockTakeView{}, err
		}
		line.QtyExpected = level.QtyOnHand
	}

	if _, err := s.repo.UpsertStockTakeLine(ctx, line); err != nil {
		return StockTakeView{}, err
	}
	return s.viewStockTake(ctx, take)
}

func (s *Service) RemoveCount(ctx context.Context, takeID, lineID string) (StockTakeView, error) {
	take, err := s.repo.StockTake(ctx, takeID, false)
	if err != nil {
		return StockTakeView{}, err
	}
	if take.Status != domain.StockTakeDraft {
		return StockTakeView{}, httpx.Conflict("INVALID_TRANSITION",
			"That count is %s and cannot be changed.", strings.ToLower(string(take.Status)))
	}
	if err := s.repo.DeleteStockTakeLine(ctx, lineID); err != nil {
		return StockTakeView{}, err
	}
	return s.viewStockTake(ctx, take)
}

// ApplyStockTake turns a count into adjustment movements.
//
// One movement per varied line, each pointing back at the count. Lines that
// matched write nothing: a movement of nothing is not a movement.
func (s *Service) ApplyStockTake(ctx context.Context, takeID string, actor Actor) (StockTakeView, error) {
	var applied domain.StockTake
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		take, err := s.repo.StockTake(ctx, takeID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.StockTakeTransitions, take.Status, domain.StockTakeApplied)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That count is %s and cannot be applied.", strings.ToLower(string(take.Status)))
		}

		lines, err := s.repo.StockTakeLines(ctx, takeID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return httpx.Conflict("NO_LINES", "That count has nothing on it.")
		}

		reason := fmt.Sprintf("Stock take %s", take.TakeNumber)
		ref := Reference{Type: "STOCK_TAKE", ID: take.ID, Number: take.TakeNumber}
		for _, line := range lines {
			variance := line.Variance()
			if variance == 0 {
				continue
			}
			if _, err := s.Post(ctx, Movement{
				ItemID: line.ItemID, BranchID: take.BranchID, Kind: domain.MovementAdjustment,
				Qty: variance, Reason: &reason, Note: line.Note,
				ReferenceType: &ref.Type, ReferenceID: &ref.ID, ReferenceNumber: &ref.Number,
			}, actor); err != nil {
				return err
			}
		}

		now := s.clock.Now()
		take.Status = next
		take.AppliedBy = &actor.ID
		take.AppliedAt = &now
		applied, err = s.repo.UpdateStockTakeStatus(ctx, take)
		return err
	})
	if err != nil {
		return StockTakeView{}, err
	}
	s.record(ctx, "inventory.stock_take", takeID, "APPLY", actor, nil)
	return s.viewStockTake(ctx, applied)
}

// CancelStockTake abandons a draft count without touching stock.
func (s *Service) CancelStockTake(ctx context.Context, takeID string, actor Actor) (StockTakeView, error) {
	take, err := s.repo.StockTake(ctx, takeID, false)
	if err != nil {
		return StockTakeView{}, err
	}
	next, err := domain.Transition(domain.StockTakeTransitions, take.Status, domain.StockTakeCancelled)
	if err != nil {
		return StockTakeView{}, httpx.Conflict("INVALID_TRANSITION",
			"That count is %s and cannot be cancelled.", strings.ToLower(string(take.Status)))
	}
	take.Status = next
	cancelled, err := s.repo.UpdateStockTakeStatus(ctx, take)
	if err != nil {
		return StockTakeView{}, err
	}
	s.record(ctx, "inventory.stock_take", takeID, "CANCEL", actor, nil)
	return s.viewStockTake(ctx, cancelled)
}

// ItemNames is the catalogue as a lookup, for modules that need to label a
// line without a second round trip.
func (s *Service) ItemNames(ctx context.Context) (map[string]string, error) {
	items, err := s.repo.Items(ctx, ItemFilter{})
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(items))
	for _, item := range items {
		names[item.ID] = item.Name
	}
	return names, nil
}

// UnitCost is an item's weighted-average cost, for callers that need to freeze
// a margin at the moment of sale.
func (s *Service) UnitCost(ctx context.Context, itemID string) (float64, error) {
	item, err := s.repo.Item(ctx, itemID, false)
	if err != nil {
		return 0, err
	}
	return item.UnitCostIDR, nil
}

// OnHand is what one branch actually has, for a till deciding whether it can
// sell what it is showing.
func (s *Service) OnHand(ctx context.Context, itemID, branchID string) (domain.Quantity, error) {
	level, err := s.repo.Level(ctx, itemID, branchID, false)
	if err != nil {
		return 0, err
	}
	return level.QtyOnHand, nil
}
