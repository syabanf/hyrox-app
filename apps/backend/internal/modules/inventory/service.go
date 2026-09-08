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

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/audit"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
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
	Reason          *string
	Note            *string
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
	return s.Post(ctx, Movement{
		ItemID: itemID, BranchID: branchID, Kind: domain.MovementIn, Qty: qty,
		UnitCostIDR: unitCostIDR, ReferenceType: &ref.Type, ReferenceID: &ref.ID,
		ReferenceNumber: &ref.Number,
	}, actor)
}

// Issue is the entry point the till uses when something is sold, and the one
// purchasing uses when goods go back to a supplier.
func (s *Service) Issue(ctx context.Context, itemID, branchID string, qty domain.Quantity,
	kind domain.MovementKind, ref Reference, actor Actor) (domain.StockMovement, error) {
	if qty < 0 {
		qty = -qty
	}
	return s.Post(ctx, Movement{
		ItemID: itemID, BranchID: branchID, Kind: kind, Qty: -qty,
		ReferenceType: &ref.Type, ReferenceID: &ref.ID, ReferenceNumber: &ref.Number,
	}, actor)
}

// Reference is what caused a movement, so the ledger row can be traced back.
type Reference struct {
	Type   string
	ID     string
	Number string
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
	SKU         string
	Name        string
	Description string
	CategoryID  *string
	Unit        string
	Kind        domain.ItemKind
	TrackStock  bool
	Barcode     *string
	ImageURL    *string
	Active      bool
}

func (in ItemInput) apply(item domain.InventoryItem) domain.InventoryItem {
	item.SKU = strings.ToUpper(strings.TrimSpace(in.SKU))
	item.Name = strings.TrimSpace(in.Name)
	item.Description = in.Description
	item.CategoryID = in.CategoryID
	item.Unit = strings.ToUpper(strings.TrimSpace(in.Unit))
	item.Kind = in.Kind
	item.TrackStock = in.TrackStock
	item.Barcode = in.Barcode
	item.ImageURL = in.ImageURL
	item.Active = in.Active
	return item
}

func (s *Service) CreateItem(ctx context.Context, in ItemInput, actor Actor) (ItemView, error) {
	created, err := s.repo.InsertItem(ctx, in.apply(domain.InventoryItem{ID: s.ids.New(id.InventoryItem)}))
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
	Note     *string
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
			Qty: in.Qty, Reason: &in.Reason, Note: in.Note,
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
