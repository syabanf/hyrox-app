package domain

import "testing"

func promo(id string, kind PromotionKind, exclusive bool) Promotion {
	return Promotion{ID: id, Code: id, Name: id, Kind: kind, Exclusive: exclusive, Active: true}
}

func basketOf(lines ...BasketLine) []BasketLine { return lines }

func line(product string, qty Quantity, unit float64) BasketLine {
	return BasketLine{
		LineID: product, ProductID: product, Qty: qty,
		UnitPriceIDR: unit, LineTotalIDR: float64(qty) * unit,
	}
}

func TestAnExclusiveOfferIsWeighedAgainstEverythingElseTogether(t *testing.T) {
	// Two stackable offers worth 30.000 between them, and one exclusive offer
	// worth 50.000. The customer gets the better deal, not both.
	tenPercent := 10.0
	fifty := 50_000.0
	twenty := 20_000.0
	ten := 10_000.0

	stackA := promo("a", PromoAmount, false)
	stackA.AmountIDR = &twenty
	stackB := promo("b", PromoAmount, false)
	stackB.AmountIDR = &ten
	solo := promo("solo", PromoAmount, true)
	solo.AmountIDR = &fifty

	basket := basketOf(line("bar", 10, 35_000))
	ctx := PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"}

	outcome := EvaluatePromotions(basket, []Promotion{stackA, stackB, solo}, nil, ctx)
	if len(outcome.Applied) != 1 || outcome.Applied[0].PromotionID != "solo" {
		t.Fatalf("the better exclusive offer wins alone, got %+v", outcome.Applied)
	}
	if outcome.DiscountIDR != 50_000 {
		t.Fatalf("50.000 beats 30.000, got %v", outcome.DiscountIDR)
	}

	// Weaken the exclusive one and the pair should win instead — and both
	// should apply, not just the better of them.
	weak := promo("weak", PromoAmount, true)
	weak.AmountIDR = &tenPercent
	outcome = EvaluatePromotions(basket, []Promotion{stackA, stackB, weak}, nil, ctx)
	if len(outcome.Applied) != 2 || outcome.DiscountIDR != 30_000 {
		t.Fatalf("the stackable pair wins together, got %+v at %v", outcome.Applied, outcome.DiscountIDR)
	}
}

func TestBuyThreeForTwoGivesAwayTheCheapest(t *testing.T) {
	// A bar and two shakers on three-for-two should cost the two shakers.
	// Giving away the dearest item instead is a promotion the shop cannot
	// afford.
	buy, free := Quantity(2), Quantity(1)
	offer := promo("3for2", PromoBuyXGetY, false)
	offer.BuyQty, offer.FreeQty = &buy, &free

	basket := basketOf(
		line("bar", 1, 35_000),
		line("shaker", 2, 79_000),
	)
	outcome := EvaluatePromotions(basket, []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"})

	if outcome.DiscountIDR != 35_000 {
		t.Fatalf("the cheapest of the three is free, got %v", outcome.DiscountIDR)
	}

	// Two units is not a group of three, so nothing is free.
	short := EvaluatePromotions(basketOf(line("bar", 2, 35_000)), []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"})
	if short.DiscountIDR != 0 {
		t.Fatalf("an incomplete group gets nothing, got %v", short.DiscountIDR)
	}

	// Six units is two complete groups.
	six := EvaluatePromotions(basketOf(line("bar", 6, 35_000)), []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"})
	if six.DiscountIDR != 70_000 {
		t.Fatalf("two groups means two free, got %v", six.DiscountIDR)
	}
}

func TestABundleNeedsTheWholeSet(t *testing.T) {
	// Discounting a partial set is how "meal deal" pricing leaks onto a basket
	// containing one item of it.
	price := 100_000.0
	offer := promo("combo", PromoBundle, false)
	offer.BundlePriceIDR = &price
	shaker, bar := "shaker", "bar"
	targets := map[string][]PromotionTarget{"combo": {
		{ProductID: &shaker, Qty: 1},
		{ProductID: &bar, Qty: 1},
	}}
	ctx := PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"}

	half := EvaluatePromotions(basketOf(line("shaker", 1, 79_000)), []Promotion{offer}, targets, ctx)
	if half.DiscountIDR != 0 {
		t.Fatalf("half a bundle is not a bundle, got %v", half.DiscountIDR)
	}

	// 79.000 plus 35.000 is 114.000; the bundle is 100.000.
	whole := EvaluatePromotions(
		basketOf(line("shaker", 1, 79_000), line("bar", 1, 35_000)), []Promotion{offer}, targets, ctx)
	if whole.DiscountIDR != 14_000 {
		t.Fatalf("the bundle saves 14.000, got %v", whole.DiscountIDR)
	}

	// Two of everything is two bundles.
	twice := EvaluatePromotions(
		basketOf(line("shaker", 2, 79_000), line("bar", 2, 35_000)), []Promotion{offer}, targets, ctx)
	if twice.DiscountIDR != 28_000 {
		t.Fatalf("two complete bundles save twice, got %v", twice.DiscountIDR)
	}
}

func TestAnOfferOutsideItsWindowOrChannelDoesNothing(t *testing.T) {
	amount := 20_000.0
	offer := promo("weekend", PromoAmount, false)
	offer.AmountIDR = &amount
	starts, ends := Date("2026-09-01"), Date("2026-09-05")
	offer.StartsOn, offer.EndsOn = &starts, &ends
	basket := basketOf(line("bar", 5, 35_000))

	past := EvaluatePromotions(basket, []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"})
	if past.DiscountIDR != 0 {
		t.Fatalf("an expired offer does nothing, got %v", past.DiscountIDR)
	}
	during := EvaluatePromotions(basket, []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-03"})
	if during.DiscountIDR != 20_000 {
		t.Fatalf("an offer inside its window applies, got %v", during.DiscountIDR)
	}

	// Wholesale-only means retail gets nothing.
	offer.Channels = []string{"WHOLESALE"}
	wrong := EvaluatePromotions(basket, []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-03"})
	if wrong.DiscountIDR != 0 {
		t.Fatalf("a wholesale offer is not a retail offer, got %v", wrong.DiscountIDR)
	}
}

func TestACodeThatSavedNothingSaysWhy(t *testing.T) {
	// The cashier is about to be asked, so the engine answers rather than
	// silently applying nothing.
	amount := 20_000.0
	offer := promo("WEEKEND", PromoAmount, false)
	offer.AmountIDR = &amount
	offer.RequiresCode = true
	offer.MinSpendIDR = 500_000
	basket := basketOf(line("bar", 2, 35_000))
	ctx := PromotionContext{Channel: ChannelRetail, Today: "2026-09-08", Code: "WEEKEND"}

	outcome := EvaluatePromotions(basket, []Promotion{offer}, nil, ctx)
	if outcome.Rejected != PromoRejectMinSpend {
		t.Fatalf("the basket is under the minimum, got %q", outcome.Rejected)
	}

	unknown := EvaluatePromotions(basket, []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-08", Code: "NOPE"})
	if unknown.Rejected != PromoRejectUnknown {
		t.Fatalf("an unknown code says so, got %q", unknown.Rejected)
	}

	// An offer that has to be asked for is not applied to somebody who did not.
	silent := EvaluatePromotions(basket, []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"})
	if silent.DiscountIDR != 0 || silent.Rejected != "" {
		t.Fatalf("a code offer needs the code, got %+v", silent)
	}
}

func TestADiscountNeverExceedsTheBasket(t *testing.T) {
	// An offer that would make the shop pay the customer is capped.
	amount := 500_000.0
	offer := promo("generous", PromoAmount, false)
	offer.AmountIDR = &amount
	outcome := EvaluatePromotions(basketOf(line("bar", 1, 35_000)), []Promotion{offer}, nil,
		PromotionContext{Channel: ChannelRetail, Today: "2026-09-08"})

	if outcome.DiscountIDR != 35_000 {
		t.Fatalf("the discount is capped at what the basket is worth, got %v", outcome.DiscountIDR)
	}
}

func TestANamedGiftCardIsNotABearerCard(t *testing.T) {
	owner := "mem_a"
	other := "mem_b"
	card := GiftCard{
		Status: GiftCardActive, BalanceIDR: 200_000, MemberID: &owner,
	}

	if got := SpendGiftCard(card, 50_000, "2026-09-08", &other); got.Rejection != GiftRejectWrongMember {
		t.Fatalf("somebody else's card is not spendable, got %q", got.Rejection)
	}
	if got := SpendGiftCard(card, 50_000, "2026-09-08", &owner); !got.Allowed() {
		t.Fatalf("the owner may spend it, got %q", got.Rejection)
	}

	// A bearer card has no name on it, which is the point of a gift.
	bearer := GiftCard{Status: GiftCardActive, BalanceIDR: 200_000}
	if got := SpendGiftCard(bearer, 50_000, "2026-09-08", nil); !got.Allowed() {
		t.Fatalf("a bearer card is spendable by whoever holds it, got %q", got.Rejection)
	}
}

func TestAGiftCardCannotGoBelowZeroOrPastItsDate(t *testing.T) {
	lapsed := Date("2026-09-01")
	card := GiftCard{Status: GiftCardActive, BalanceIDR: 100_000, ExpiresOn: &lapsed}
	if got := SpendGiftCard(card, 10_000, "2026-09-08", nil); got.Rejection != GiftRejectExpired {
		t.Fatalf("an expired card pays for nothing, got %q", got.Rejection)
	}

	live := GiftCard{Status: GiftCardActive, BalanceIDR: 100_000}
	if got := SpendGiftCard(live, 150_000, "2026-09-08", nil); got.Rejection != GiftRejectInsufficient {
		t.Fatalf("a card cannot overdraw, got %q", got.Rejection)
	}
	posted := SpendGiftCard(live, 40_000, "2026-09-08", nil)
	if posted.BalanceAfter != 60_000 || posted.AmountIDR != -40_000 {
		t.Fatalf("spending 40.000 of 100.000 leaves 60.000, got %+v", posted)
	}
	// And a frozen card is not spendable, which is what freezing is for.
	frozen := GiftCard{Status: GiftCardFrozen, BalanceIDR: 100_000}
	if got := SpendGiftCard(frozen, 10_000, "2026-09-08", nil); got.Rejection != GiftRejectNotActive {
		t.Fatalf("a frozen card pays for nothing, got %q", got.Rejection)
	}
}

func TestChangeComesOutOfWhateverCanGiveIt(t *testing.T) {
	methods := map[string]PaymentMethod{
		"CASH":    {Code: "CASH", Kind: MethodCash, GivesChange: true, CountsInDrawer: true},
		"QRIS":    {Code: "QRIS", Kind: MethodQR},
		"VOUCHER": {Code: "VOUCHER", Kind: MethodCash, GivesChange: true, CountsInDrawer: true},
	}

	// Overpaying by card gives no change; overpaying in cash does.
	card := SettleWith(100_000, []POSPayment{{Method: "QRIS", AmountIDR: 120_000}}, methods)
	if card.ChangeIDR != 0 {
		t.Fatalf("a card gets no change, got %v", card.ChangeIDR)
	}
	cash := SettleWith(100_000, []POSPayment{{Method: "CASH", AmountIDR: 150_000}}, methods)
	if cash.ChangeIDR != 50_000 {
		t.Fatalf("cash gets its change, got %v", cash.ChangeIDR)
	}

	// Split: the change can only come out of the cash part.
	split := SettleWith(100_000, []POSPayment{
		{Method: "QRIS", AmountIDR: 80_000},
		{Method: "CASH", AmountIDR: 50_000},
	}, methods)
	if split.ChangeIDR != 30_000 {
		t.Fatalf("30.000 over, all of it giveable in cash, got %v", split.ChangeIDR)
	}

	// The drawer counts what the methods say it counts, rather than a
	// hard-coded idea of cash.
	drawer := DrawerCash(500_000, []POSPayment{
		{Method: "CASH", AmountIDR: 150_000, ChangeIDR: 50_000},
		{Method: "QRIS", AmountIDR: 200_000},
		{Method: "VOUCHER", AmountIDR: 25_000},
	}, methods)
	if drawer != 625_000 {
		t.Fatalf("500k plus 100k net cash plus 25k voucher, got %v", drawer)
	}
}
