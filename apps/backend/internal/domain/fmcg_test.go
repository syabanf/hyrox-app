package domain

import "testing"

func TestPackConversionIsInvertible(t *testing.T) {
	// Ten cartons of twenty-four is two hundred and forty pieces, and two
	// hundred and forty pieces is ten cartons. A conversion that does not
	// survive the round trip is a stock report that drifts.
	base := PackToBase(10, 24)
	if base != 240 {
		t.Fatalf("10 cartons of 24 is 240 pieces, got %v", base)
	}
	if back := BaseToPack(base, 24); back != 10 {
		t.Fatalf("240 pieces is 10 cartons, got %v", back)
	}
}

func TestAPartCartonIsNotRoundedAway(t *testing.T) {
	// Fifty-one pieces really is two and an eighth cartons, and saying "2"
	// loses three pieces of stock every time somebody looks.
	if got := BaseToPack(51, 24); got != 2.125 {
		t.Fatalf("51 pieces is 2.125 cartons, got %v", got)
	}
	packs, remainder := SplitToPacks(51, 24)
	if packs != 2 || remainder != 3 {
		t.Fatalf("51 pieces is 2 cartons and 3 pieces, got %d and %v", packs, remainder)
	}
}

func TestAPackPriceBecomesAUnitPrice(t *testing.T) {
	// The receipt divides once. Booking a carton price in as a unit cost is
	// how an average cost inflates by exactly the size of the pack.
	if got := PackPriceToBase(396_000, 24); got != 16_500 {
		t.Fatalf("a carton of 24 at 396.000 is 16.500 a piece, got %v", got)
	}
	// A base pack divides by one and comes back unchanged.
	if got := PackPriceToBase(16_500, 1); got != 16_500 {
		t.Fatalf("a piece price must survive its own conversion, got %v", got)
	}
}

func TestAMissingFactorIsTreatedAsOne(t *testing.T) {
	// Old rows predate packs and carry no factor. They are pieces, not zero
	// pieces: a division by zero here would erase a quantity rather than
	// report a problem.
	if got := PackToBase(5, 0); got != 5 {
		t.Fatalf("a missing factor means base units, got %v", got)
	}
	if got := BaseToPack(5, 0); got != 5 {
		t.Fatalf("a missing factor means base units, got %v", got)
	}
}

func TestACountedUnitCannotHoldAFraction(t *testing.T) {
	packs := []ItemPack{{ID: "base", UnitCode: "PCS", Factor: 1, IsBase: true, Active: true}}

	half := ItemPack{ID: "new", UnitCode: "CTN", Factor: 24.5}
	if got := EvaluatePack(packs, half, UnitCount); got != PackRejectFractional {
		t.Fatalf("a carton of 24.5 pieces is a typo, got %q", got)
	}
	// The same factor is ordinary for something measured rather than counted.
	if got := EvaluatePack(packs, half, UnitMeasure); got != "" {
		t.Fatalf("24.5 kg is a real quantity, got %q", got)
	}
}

func TestOnlyOneBaseAndOneFactorPerUnit(t *testing.T) {
	packs := []ItemPack{
		{ID: "base", UnitCode: "PCS", Factor: 1, IsBase: true, Active: true},
		{ID: "ctn", UnitCode: "CTN", Factor: 24, Active: true},
	}

	if got := EvaluatePack(packs, ItemPack{ID: "other", UnitCode: "CTN", Factor: 12}, UnitCount); got != PackRejectDuplicate {
		t.Fatalf("two cartons of different sizes is a contradiction, got %q", got)
	}
	// Editing the existing carton is not a duplicate of itself.
	if got := EvaluatePack(packs, ItemPack{ID: "ctn", UnitCode: "CTN", Factor: 12}, UnitCount); got != "" {
		t.Fatalf("a pack may be resized, got %q", got)
	}
	if got := EvaluatePack(packs, ItemPack{ID: "b2", UnitCode: "BOX", Factor: 6, IsBase: true}, UnitCount); got != PackRejectBaseFactor {
		t.Fatalf("the base pack holds exactly one base unit, got %q", got)
	}
}

func TestThePriceBreakIsTheDeepestOneReached(t *testing.T) {
	prices := []ProductPrice{
		{Channel: ChannelRetail, MinQty: 12, PriceIDR: 31_000, Active: true},
		{Channel: ChannelRetail, MinQty: 24, PriceIDR: 29_000, Active: true},
		{Channel: ChannelWholesale, MinQty: 1, PriceIDR: 27_500, Active: true},
	}
	const list = 35_000

	if got := ResolvePrice(prices, list, ChannelRetail, 1); got != list {
		t.Fatalf("one bar is the shelf price, got %v", got)
	}
	if got := ResolvePrice(prices, list, ChannelRetail, 12); got != 31_000 {
		t.Fatalf("a dozen reaches the first break, got %v", got)
	}
	if got := ResolvePrice(prices, list, ChannelRetail, 30); got != 29_000 {
		t.Fatalf("thirty reaches the deeper break, got %v", got)
	}
	if got := ResolvePrice(prices, list, ChannelWholesale, 1); got != 27_500 {
		t.Fatalf("a reseller pays the wholesale price from one, got %v", got)
	}
}

func TestAChannelWithNoPriceOfItsOwnFallsBackRatherThanFailing(t *testing.T) {
	prices := []ProductPrice{{Channel: ChannelRetail, MinQty: 12, PriceIDR: 31_000, Active: true}}

	// Staff have no price list here, so they pay what a retail customer pays
	// at that quantity — never nothing, and never the deepest break of some
	// other channel.
	if got := ResolvePrice(prices, 35_000, ChannelStaff, 12); got != 31_000 {
		t.Fatalf("an unpriced channel falls back to retail, got %v", got)
	}
	if got := ResolvePrice(nil, 35_000, ChannelStaff, 100); got != 35_000 {
		t.Fatalf("no price list at all still sells, got %v", got)
	}
}

func TestAnInactiveBreakDoesNotApply(t *testing.T) {
	prices := []ProductPrice{
		{Channel: ChannelRetail, MinQty: 12, PriceIDR: 31_000, Active: false},
	}
	// Switching a promotion off has to actually switch it off, which is the
	// difference between a flag and a comment.
	if got := ResolvePrice(prices, 35_000, ChannelRetail, 20); got != 35_000 {
		t.Fatalf("a disabled break must not apply, got %v", got)
	}
}

func TestASoldLineConsumesItsPack(t *testing.T) {
	line := POSOrderItem{Qty: 2, PackFactor: 6}
	if got := line.BaseQty(); got != 12 {
		t.Fatalf("two six-packs is twelve bottles, got %v", got)
	}
	// A line written before packs existed carries no factor and is pieces.
	if got := (POSOrderItem{Qty: 3}).BaseQty(); got != 3 {
		t.Fatalf("a line without a pack is base units, got %v", got)
	}
}

func TestMarginIsCountedPerBaseUnit(t *testing.T) {
	// A carton of 24 sold at 780.000 against a piece cost of 18.000 costs
	// 432.000, not 18.000. Multiplying the cost by the sold quantity instead
	// of the base quantity makes every carton look 24 times more profitable
	// than it is.
	totals := ComputeOrderPOSTotals([]POSOrderItem{
		{Qty: 1, PackFactor: 24, LineTotalIDR: 780_000, UnitCostIDR: 18_000},
	}, 0)

	if totals.CostIDR != 432_000 {
		t.Fatalf("a carton costs 24 pieces, got %v", totals.CostIDR)
	}
	if totals.GrossProfitIDR != 348_000 {
		t.Fatalf("780.000 less 432.000 is 348.000, got %v", totals.GrossProfitIDR)
	}
}
