package domain

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Packs: the one piece of arithmetic that separates a shop from a kitchen.
//
// Goods arrive in a carton and leave in a hand. Stock is counted in the item's
// base unit throughout, and a pack is a named multiple of it — a six-pack, a
// carton of twenty-four — carrying its own barcode because that is how a till
// tells one from the other.
//
// Every conversion in the system comes through here. Ordering in cartons and
// receipting in pieces is the standard way an FMCG stock report becomes
// fiction, and it happens when two places each do the multiplication.

// Unit is a word for an amount: piece, carton, kilogram.
type Unit struct {
	ID        string   `json:"id"`
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Kind      UnitKind `json:"kind"`
	Active    bool     `json:"active"`
	SortOrder int      `json:"sortOrder"`
}

// UnitKind separates things you count from things you measure.
type UnitKind string

const (
	// UnitCount is whole things. Half a carton is a mistake worth catching.
	UnitCount UnitKind = "COUNT"
	// UnitMeasure is continuous: 1.5 kg is an ordinary quantity.
	UnitMeasure UnitKind = "MEASURE"
)

// IsValidUnitKind validates a kind arriving from a request.
func IsValidUnitKind(value string) bool {
	switch UnitKind(value) {
	case UnitCount, UnitMeasure:
		return true
	}
	return false
}

// ItemPack is one way of handing an item over.
//
// Factor is how many base units are inside it, so the base pack's factor is 1
// by definition and the database refuses anything else.
type ItemPack struct {
	ID       string  `json:"id"`
	ItemID   string  `json:"itemId"`
	UnitCode string  `json:"unitCode"`
	Factor   float64 `json:"factor"`
	Barcode  *string `json:"barcode"`
	IsBase   bool    `json:"isBase"`
	// PurchaseDefault and SaleDefault are what a new purchase line and a new
	// product reach for, so the common case needs no decision.
	PurchaseDefault bool `json:"purchaseDefault"`
	SaleDefault     bool `json:"saleDefault"`
	Active          bool `json:"active"`
}

// Label renders a pack the way a shelf edge does: "CTN (24 PCS)", or just
// "PCS" when there is nothing to convert.
func (p ItemPack) Label(baseUnit string) string {
	if p.IsBase || p.Factor == 1 {
		return p.UnitCode
	}
	return p.UnitCode + " (" + trimFloat(p.Factor) + " " + baseUnit + ")"
}

func trimFloat(v float64) string {
	s := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 3, 64), "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// PackToBase converts a quantity in a pack to the base unit the ledger counts
// in. Ten cartons of twenty-four is two hundred and forty pieces.
func PackToBase(packQty Quantity, factor float64) Quantity {
	if factor <= 0 {
		factor = 1
	}
	return RoundQuantity(Quantity(float64(packQty) * factor))
}

// BaseToPack converts the other way, for showing a stock level in the unit
// somebody buys in. It is deliberately not rounded to whole packs: three and a
// half cartons on hand is a true and useful thing to say.
func BaseToPack(baseQty Quantity, factor float64) Quantity {
	if factor <= 0 {
		factor = 1
	}
	return RoundQuantity(Quantity(float64(baseQty) / factor))
}

// PackPriceToBase converts a price quoted per pack into a price per base unit,
// which is the only form the weighted-average cost can accept.
//
// A carton at 396,000 containing 24 is 16,500 a piece, and it is this division
// — not the multiplication — that quietly corrupts a cost when it is done in
// two places with two different factors.
func PackPriceToBase(packPriceIDR, factor float64) float64 {
	if factor <= 0 {
		return packPriceIDR
	}
	return math.Round(packPriceIDR/factor*100) / 100
}

// SplitToPacks expresses a base quantity as whole packs plus a remainder, for
// a picking list that has to say "2 cartons and 3 pieces" rather than "51".
func SplitToPacks(baseQty Quantity, factor float64) (packs int, remainder Quantity) {
	if factor <= 1 {
		return int(baseQty), 0
	}
	whole := math.Floor(float64(baseQty) / factor)
	return int(whole), RoundQuantity(baseQty - Quantity(whole*factor))
}

// FindPack picks a pack by its unit code.
func FindPack(packs []ItemPack, unitCode string) (ItemPack, bool) {
	for _, pack := range packs {
		if pack.UnitCode == unitCode && pack.Active {
			return pack, true
		}
	}
	return ItemPack{}, false
}

// BasePack is the pack the ledger counts in.
func BasePack(packs []ItemPack) (ItemPack, bool) {
	for _, pack := range packs {
		if pack.IsBase {
			return pack, true
		}
	}
	return ItemPack{}, false
}

// DefaultPurchasePack is what a new purchase line should offer: the pack
// marked for buying, falling back to the base unit.
func DefaultPurchasePack(packs []ItemPack) (ItemPack, bool) {
	for _, pack := range packs {
		if pack.PurchaseDefault && pack.Active {
			return pack, true
		}
	}
	return BasePack(packs)
}

// DefaultSalePack is the same question for the till.
func DefaultSalePack(packs []ItemPack) (ItemPack, bool) {
	for _, pack := range packs {
		if pack.SaleDefault && pack.Active {
			return pack, true
		}
	}
	return BasePack(packs)
}

// PackRejection explains why a pack definition cannot stand.
type PackRejection string

const (
	PackRejectFactor      PackRejection = "FACTOR_NOT_POSITIVE"
	PackRejectBaseFactor  PackRejection = "BASE_FACTOR_NOT_ONE"
	PackRejectDuplicate   PackRejection = "UNIT_ALREADY_DEFINED"
	PackRejectNoBase      PackRejection = "NO_BASE_PACK"
	PackRejectBaseRemoved PackRejection = "BASE_PACK_REQUIRED"
	PackRejectFractional  PackRejection = "FRACTIONAL_COUNT"
)

// EvaluatePack checks one pack against the set an item already has.
//
// The rules are small and all of them are about the conversion staying
// invertible: one base, one factor per unit, and a counted unit that holds a
// whole number of base units. A carton of 24.5 pieces is a typo every time.
func EvaluatePack(existing []ItemPack, candidate ItemPack, kind UnitKind) PackRejection {
	if candidate.Factor <= 0 {
		return PackRejectFactor
	}
	if candidate.IsBase && candidate.Factor != 1 {
		return PackRejectBaseFactor
	}
	if kind == UnitCount && candidate.Factor != math.Trunc(candidate.Factor) {
		return PackRejectFractional
	}
	for _, pack := range existing {
		if pack.ID == candidate.ID {
			continue
		}
		if pack.UnitCode == candidate.UnitCode {
			return PackRejectDuplicate
		}
	}
	return ""
}

// ── Channel pricing ──────────────────────────────────────────────────────────

// SalesChannel is what a customer is buying as. It is the only thing that
// moves a price, which is why it lives on the order rather than on the
// customer.
type SalesChannel string

const (
	// ChannelRetail is somebody buying one for themselves.
	ChannelRetail SalesChannel = "RETAIL"
	// ChannelWholesale is a reseller buying to sell on, at a break price.
	ChannelWholesale SalesChannel = "WHOLESALE"
	// ChannelStaff is an employee purchase, priced by policy rather than by
	// the market.
	ChannelStaff SalesChannel = "STAFF"
)

// IsValidChannel validates a channel arriving from a request.
func IsValidChannel(value string) bool {
	switch SalesChannel(value) {
	case ChannelRetail, ChannelWholesale, ChannelStaff:
		return true
	}
	return false
}

// ProductPrice is one price break: this channel, at this quantity and above,
// pays this.
type ProductPrice struct {
	ID        string       `json:"id"`
	ProductID string       `json:"productId"`
	Channel   SalesChannel `json:"channel"`
	MinQty    Quantity     `json:"minQty"`
	PriceIDR  float64      `json:"priceIdr"`
	Active    bool         `json:"active"`
}

// ResolvePrice is what one unit costs, given who is buying and how many.
//
// The deepest break whose minimum the quantity reaches wins. A channel with no
// break at all falls back to RETAIL, and RETAIL with no break falls back to
// the product's own price — so a catalogue with no price list still sells,
// which is the state every shop starts in.
func ResolvePrice(prices []ProductPrice, listPriceIDR float64, channel SalesChannel, qty Quantity) float64 {
	if price, ok := breakFor(prices, channel, qty); ok {
		return price
	}
	if channel != ChannelRetail {
		if price, ok := breakFor(prices, ChannelRetail, qty); ok {
			return price
		}
	}
	return listPriceIDR
}

func breakFor(prices []ProductPrice, channel SalesChannel, qty Quantity) (float64, bool) {
	candidates := make([]ProductPrice, 0, len(prices))
	for _, price := range prices {
		if price.Active && price.Channel == channel && price.MinQty <= qty {
			candidates = append(candidates, price)
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	// Deepest break first; a tie goes to the cheaper price, because two rows
	// claiming the same break should never make the customer pay more.
	sort.Slice(candidates, func(a, b int) bool {
		if candidates[a].MinQty != candidates[b].MinQty {
			return candidates[a].MinQty > candidates[b].MinQty
		}
		return candidates[a].PriceIDR < candidates[b].PriceIDR
	})
	return candidates[0].PriceIDR, true
}
