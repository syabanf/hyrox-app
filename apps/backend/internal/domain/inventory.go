package domain

import (
	"math"
	"time"
)

// Stock, and the movements that change it.
//
// The shape is the credit ledger's: movements are the append-only truth and a
// level row is their running sum. That is not decoration — a sale and a goods
// receipt both change a quantity, and the only way to explain a discrepancy
// later is to have never overwritten anything.

// Quantity is an amount of stock. It is fixed-point to three decimals rather
// than a float because 0.1 + 0.2 must equal 0.3 when somebody is counting
// protein bars, and because it lands in NUMERIC(15,3) at the other end.
type Quantity float64

// RoundQuantity clamps to the three decimals the column stores, so Go and
// PostgreSQL always agree about what a quantity is.
func RoundQuantity(q Quantity) Quantity {
	return Quantity(math.Round(float64(q)*1000) / 1000)
}

// ItemKind separates what the studio sells from what it merely consumes.
type ItemKind string

const (
	// ItemRetail is sold at the counter: merchandise, drinks, supplements.
	ItemRetail ItemKind = "RETAIL"
	// ItemSupply is consumed by the studio and never sold: cleaning, towels.
	ItemSupply ItemKind = "SUPPLY"
	// ItemRaw is an ingredient of something else, for a studio with a bar.
	ItemRaw ItemKind = "RAW"
)

// InventoryItem is a thing the studio counts.
type InventoryItem struct {
	ID          string   `json:"id"`
	SKU         string   `json:"sku"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CategoryID  *string  `json:"categoryId"`
	Unit        string   `json:"unit"`
	Kind        ItemKind `json:"kind"`
	// UnitCostIDR is the weighted average of what the stock on hand cost, not
	// the price it is sold at and not the last price paid.
	UnitCostIDR float64 `json:"unitCostIdr"`
	TrackStock  bool    `json:"trackStock"`
	// TrackBatches turns on dates. Most of a catalogue does not want it — a
	// steel bottle has no date on it, and demanding one would make every
	// receipt a form nobody can fill in — so it is off unless asked for.
	TrackBatches bool `json:"trackBatches"`
	// ExpiryWarningDays is how long before the printed date somebody should
	// be told. A drink with six months of life wants a longer warning than
	// something with a fortnight.
	ExpiryWarningDays int       `json:"expiryWarningDays"`
	Barcode           *string   `json:"barcode"`
	ImageURL          *string   `json:"imageUrl"`
	Active            bool      `json:"active"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// InventoryCategory groups items for the shelf and the report.
type InventoryCategory struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	Active    bool      `json:"active"`
	SortOrder int       `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// StockLevel is one item at one branch. "Do we have protein bars" is not a
// question until it says where.
type StockLevel struct {
	ItemID    string   `json:"itemId"`
	BranchID  string   `json:"branchId"`
	QtyOnHand Quantity `json:"qtyOnHand"`
	// QtyOnOrder is ordered from a supplier but not yet received. Reorder
	// maths counts it, so stock already on its way is not ordered twice.
	QtyOnOrder     Quantity   `json:"qtyOnOrder"`
	QtyMinimum     Quantity   `json:"qtyMinimum"`
	QtyMaximum     *Quantity  `json:"qtyMaximum"`
	BinLocation    *string    `json:"binLocation"`
	LastMovementAt *time.Time `json:"lastMovementAt"`
	Note           *string    `json:"note"`
}

// MovementKind says why a quantity changed. The sign lives on the movement
// itself, because a RETURN is stock leaving when it goes back to a supplier
// and stock arriving when a member brings a shirt back.
type MovementKind string

const (
	MovementIn          MovementKind = "IN"
	MovementOut         MovementKind = "OUT"
	MovementAdjustment  MovementKind = "ADJUSTMENT"
	MovementTransferIn  MovementKind = "TRANSFER_IN"
	MovementTransferOut MovementKind = "TRANSFER_OUT"
	MovementReturn      MovementKind = "RETURN"
)

// IsValidMovementKind validates a kind arriving from a request.
func IsValidMovementKind(value string) bool {
	switch MovementKind(value) {
	case MovementIn, MovementOut, MovementAdjustment,
		MovementTransferIn, MovementTransferOut, MovementReturn:
		return true
	}
	return false
}

// StockMovement is one change to one level, kept forever.
type StockMovement struct {
	ID       string       `json:"id"`
	ItemID   string       `json:"itemId"`
	BranchID string       `json:"branchId"`
	Kind     MovementKind `json:"kind"`
	// Qty is signed: what was added to the level, negative when taken away.
	Qty          Quantity `json:"qty"`
	QtyBefore    Quantity `json:"qtyBefore"`
	QtyAfter     Quantity `json:"qtyAfter"`
	UnitCostIDR  float64  `json:"unitCostIdr"`
	TotalCostIDR float64  `json:"totalCostIdr"`
	// What was physically handled, when it was not base units: ten cartons
	// rather than 240 pieces. The database refuses the pair if it does not
	// multiply out to Qty.
	PackUnit   *string   `json:"packUnit"`
	PackQty    *Quantity `json:"packQty"`
	PackFactor *float64  `json:"packFactor"`
	// What caused it — a goods receipt, a sale, a stock take.
	ReferenceType   *string   `json:"referenceType"`
	ReferenceID     *string   `json:"referenceId"`
	ReferenceNumber *string   `json:"referenceNumber"`
	Reason          *string   `json:"reason"`
	Note            *string   `json:"note"`
	ActorID         *string   `json:"actorId"`
	ActorName       *string   `json:"actorName"`
	CreatedAt       time.Time `json:"createdAt"`
}

// StockRejection explains why a movement cannot be posted.
type StockRejection string

const (
	StockRejectZeroQty      StockRejection = "ZERO_QUANTITY"
	StockRejectInsufficient StockRejection = "INSUFFICIENT_STOCK"
	StockRejectNotTracked   StockRejection = "NOT_TRACKED"
	StockRejectInactive     StockRejection = "ITEM_INACTIVE"
)

// PostedMovement is the outcome of applying a change to a level.
type PostedMovement struct {
	Kind      MovementKind
	Qty       Quantity
	QtyBefore Quantity
	QtyAfter  Quantity
	// UnitCostAfter is the item's weighted-average cost once this movement is
	// counted. Unchanged by anything that only takes stock away.
	UnitCostAfter float64
	TotalCostIDR  float64
	Rejection     StockRejection
}

// Allowed reports whether the movement may be posted.
func (p PostedMovement) Allowed() bool { return p.Rejection == "" }

// PostMovement is the one place a stock quantity changes.
//
// Everything else — a goods receipt, a sale, a stock take, a transfer — is a
// caller that decides the kind and the sign and then comes through here, so
// there is exactly one implementation of "you cannot take out what is not
// there" and exactly one of the weighted-average cost.
func PostMovement(item InventoryItem, level StockLevel, kind MovementKind, qty Quantity, unitCostIDR float64) PostedMovement {
	qty = RoundQuantity(qty)

	if !item.Active {
		return PostedMovement{Rejection: StockRejectInactive}
	}
	if !item.TrackStock {
		return PostedMovement{Rejection: StockRejectNotTracked}
	}
	if qty == 0 {
		return PostedMovement{Rejection: StockRejectZeroQty}
	}

	after := RoundQuantity(level.QtyOnHand + qty)
	if after < 0 {
		return PostedMovement{Rejection: StockRejectInsufficient}
	}

	posted := PostedMovement{
		Kind:          kind,
		Qty:           qty,
		QtyBefore:     level.QtyOnHand,
		QtyAfter:      after,
		UnitCostAfter: item.UnitCostIDR,
		TotalCostIDR:  math.Round(math.Abs(float64(qty))*unitCostIDR*100) / 100,
	}

	// Stock arriving at a stated price moves the average; stock leaving is
	// valued at the average it already has, and does not change it.
	if qty > 0 && unitCostIDR > 0 {
		posted.UnitCostAfter = WeightedAverageCost(level.QtyOnHand, item.UnitCostIDR, qty, unitCostIDR)
	}
	if qty < 0 {
		posted.TotalCostIDR = math.Round(math.Abs(float64(qty))*item.UnitCostIDR*100) / 100
	}
	return posted
}

// WeightedAverageCost is what the stock on hand costs once a receipt lands.
//
// Averaging rather than FIFO is a deliberate choice: FIFO needs a lot per
// receipt and a consumption order, which is the right model for credits that
// expire and the wrong one for protein bars that do not. Averaging gives one
// number per item that a stock report can be honest about.
func WeightedAverageCost(qtyOnHand Quantity, costOnHand float64, qtyIn Quantity, costIn float64) float64 {
	total := qtyOnHand + qtyIn
	if total <= 0 {
		return costIn
	}
	// Stock that arrived free (a sample, a correction with no price) must not
	// drag the average to zero, so a zero incoming cost is treated as "no new
	// information" by the caller before it reaches here.
	average := (float64(qtyOnHand)*costOnHand + float64(qtyIn)*costIn) / float64(total)
	return math.Round(average*100) / 100
}

// IsLowStock reports whether an item has reached the level at which somebody
// should reorder it. Stock already on its way counts: a purchase order that
// has been raised is not a reason to raise another.
func IsLowStock(level StockLevel) bool {
	return level.QtyOnHand+level.QtyOnOrder <= level.QtyMinimum
}

// ReorderQuantity is how much to buy to bring an item back up.
//
// Up to the maximum where one is set, otherwise up to the minimum — never
// below zero, and never counting stock already ordered twice.
func ReorderQuantity(level StockLevel) Quantity {
	target := level.QtyMinimum
	if level.QtyMaximum != nil && *level.QtyMaximum > target {
		target = *level.QtyMaximum
	}
	needed := target - (level.QtyOnHand + level.QtyOnOrder)
	if needed <= 0 {
		return 0
	}
	return RoundQuantity(needed)
}

// StockValue is what one level is worth at the item's average cost.
func StockValue(item InventoryItem, level StockLevel) float64 {
	return math.Round(float64(level.QtyOnHand)*item.UnitCostIDR*100) / 100
}

// ── Stock takes ──────────────────────────────────────────────────────────────

// StockTakeStatus is where a count has got to.
type StockTakeStatus string

const (
	StockTakeDraft     StockTakeStatus = "DRAFT"
	StockTakeApplied   StockTakeStatus = "APPLIED"
	StockTakeCancelled StockTakeStatus = "CANCELLED"
)

// StockTakeTransitions: an applied count is final. It has already written
// adjustment movements, and those cannot be taken back — a second count is a
// second stock take.
var StockTakeTransitions = TransitionMap[StockTakeStatus]{
	StockTakeDraft:     {StockTakeApplied, StockTakeCancelled},
	StockTakeApplied:   {},
	StockTakeCancelled: {},
}

// StockTake is a physical count of one branch on one day.
type StockTake struct {
	ID         string          `json:"id"`
	TakeNumber string          `json:"takeNumber"`
	BranchID   string          `json:"branchId"`
	Status     StockTakeStatus `json:"status"`
	CountedOn  Date            `json:"countedOn"`
	Note       *string         `json:"note"`
	AppliedBy  *string         `json:"appliedBy"`
	AppliedAt  *time.Time      `json:"appliedAt"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

// StockTakeLine is one item on a count.
type StockTakeLine struct {
	ID          string `json:"id"`
	StockTakeID string `json:"stockTakeId"`
	ItemID      string `json:"itemId"`
	// QtyExpected is frozen when the line is added: the variance is against
	// what the system believed at counting time, not whatever it says now.
	QtyExpected Quantity `json:"qtyExpected"`
	QtyCounted  Quantity `json:"qtyCounted"`
	Note        *string  `json:"note"`
}

// Variance is counted minus expected: negative is shrinkage.
func (l StockTakeLine) Variance() Quantity {
	return RoundQuantity(l.QtyCounted - l.QtyExpected)
}

// StockTakeSummary is what a count found.
type StockTakeSummary struct {
	Lines         int      `json:"lines"`
	LinesVaried   int      `json:"linesVaried"`
	QtyOver       Quantity `json:"qtyOver"`
	QtyShort      Quantity `json:"qtyShort"`
	ValueVariance float64  `json:"valueVarianceIdr"`
}

// SummarizeStockTake totals a count, valuing the variance at each item's
// average cost so the number means money rather than units of nothing.
func SummarizeStockTake(lines []StockTakeLine, costOf map[string]float64) StockTakeSummary {
	summary := StockTakeSummary{Lines: len(lines)}
	for _, line := range lines {
		variance := line.Variance()
		if variance == 0 {
			continue
		}
		summary.LinesVaried++
		if variance > 0 {
			summary.QtyOver += variance
		} else {
			summary.QtyShort += -variance
		}
		summary.ValueVariance += float64(variance) * costOf[line.ItemID]
	}
	summary.QtyOver = RoundQuantity(summary.QtyOver)
	summary.QtyShort = RoundQuantity(summary.QtyShort)
	summary.ValueVariance = math.Round(summary.ValueVariance*100) / 100
	return summary
}

// StockTransfer moves stock between branches. It exists so the two movements
// it produces are one act rather than two hopeful ones.
type StockTransfer struct {
	ID             string    `json:"id"`
	TransferNumber string    `json:"transferNumber"`
	ItemID         string    `json:"itemId"`
	FromBranchID   string    `json:"fromBranchId"`
	ToBranchID     string    `json:"toBranchId"`
	Qty            Quantity  `json:"qty"`
	Note           *string   `json:"note"`
	ActorID        *string   `json:"actorId"`
	ActorName      *string   `json:"actorName"`
	CreatedAt      time.Time `json:"createdAt"`
}
