package domain

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Offers the till applies by itself.
//
// A price break is a property of a product: what one thing costs at a
// quantity. A promotion is about a basket, and it has a start and an end and
// somebody's decision behind it. Keeping them apart is why a promotion can be
// switched off on Monday without repricing the catalogue.

// PromotionKind is the shape of an offer.
type PromotionKind string

const (
	// PromoPercent takes a percentage off what it targets.
	PromoPercent PromotionKind = "PERCENT"
	// PromoAmount takes a fixed sum off, never more than the goods are worth.
	PromoAmount PromotionKind = "AMOUNT"
	// PromoBuyXGetY makes the cheapest of each qualifying group free.
	PromoBuyXGetY PromotionKind = "BUY_X_GET_Y"
	// PromoBundle prices a named set as one thing.
	PromoBundle PromotionKind = "BUNDLE"
)

// IsValidPromotionKind validates a kind arriving from a request.
func IsValidPromotionKind(value string) bool {
	switch PromotionKind(value) {
	case PromoPercent, PromoAmount, PromoBuyXGetY, PromoBundle:
		return true
	}
	return false
}

// Promotion is an offer.
type Promotion struct {
	ID          string        `json:"id"`
	Code        string        `json:"code"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Kind        PromotionKind `json:"kind"`

	Percent        *float64  `json:"percent"`
	AmountIDR      *float64  `json:"amountIdr"`
	BuyQty         *Quantity `json:"buyQty"`
	FreeQty        *Quantity `json:"freeQty"`
	BundlePriceIDR *float64  `json:"bundlePriceIdr"`

	MinSpendIDR float64  `json:"minSpendIdr"`
	MinQty      Quantity `json:"minQty"`
	// RequiresCode is an offer somebody has to ask for by name.
	RequiresCode bool     `json:"requiresCode"`
	Channels     []string `json:"channels"`
	// Exclusive is the only offer on the basket when it wins. Without it, two
	// generous promotions stack into a sale at a loss that nobody notices
	// until the margin report.
	Exclusive bool  `json:"exclusive"`
	Priority  int   `json:"priority"`
	StartsOn  *Date `json:"startsOn"`
	EndsOn    *Date `json:"endsOn"`

	MaxUses          *int `json:"maxUses"`
	MaxUsesPerMember *int `json:"maxUsesPerMember"`
	UsedCount        int  `json:"usedCount"`
	Active           bool `json:"active"`
}

// PromotionTarget is what an offer applies to. No targets means the basket.
type PromotionTarget struct {
	ID          string  `json:"id"`
	PromotionID string  `json:"promotionId"`
	ProductID   *string `json:"productId"`
	CategoryID  *string `json:"categoryId"`
	// Qty is how many of this product a bundle contains.
	Qty Quantity `json:"qty"`
}

// BasketLine is one line as the promotion engine sees it.
type BasketLine struct {
	LineID       string   `json:"lineId"`
	ProductID    string   `json:"productId"`
	CategoryID   *string  `json:"categoryId"`
	Qty          Quantity `json:"qty"`
	UnitPriceIDR float64  `json:"unitPriceIdr"`
	LineTotalIDR float64  `json:"lineTotalIdr"`
}

// PromotionContext is everything about the sale that is not the goods.
type PromotionContext struct {
	Channel SalesChannel
	Today   Date
	Now     time.Time
	// Code is what the customer typed, if anything.
	Code string
	// MemberUses is how many times this member has already had each offer.
	MemberUses map[string]int
}

// AppliedPromotion is one offer that landed, and what it was worth.
type AppliedPromotion struct {
	PromotionID string  `json:"promotionId"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	DiscountIDR float64 `json:"discountIdr"`
}

// PromotionOutcome is what the basket got.
type PromotionOutcome struct {
	Applied     []AppliedPromotion `json:"applied"`
	DiscountIDR float64            `json:"discountIdr"`
	// Rejected explains why a code somebody typed did nothing, which is the
	// question a cashier is about to be asked.
	Rejected PromotionRejection `json:"rejected,omitempty"`
}

// PromotionRejection says why a named offer did not apply.
type PromotionRejection string

const (
	PromoRejectUnknown   PromotionRejection = "NO_SUCH_CODE"
	PromoRejectInactive  PromotionRejection = "NOT_ACTIVE"
	PromoRejectWindow    PromotionRejection = "OUTSIDE_WINDOW"
	PromoRejectChannel   PromotionRejection = "WRONG_CHANNEL"
	PromoRejectMinSpend  PromotionRejection = "BELOW_MINIMUM"
	PromoRejectExhausted PromotionRejection = "FULLY_USED"
	PromoRejectPerMember PromotionRejection = "ALREADY_USED"
	PromoRejectNoQualify PromotionRejection = "NOTHING_QUALIFIES"
)

// Live reports whether an offer is running at all.
func (p Promotion) Live(today Date) bool {
	if !p.Active {
		return false
	}
	if p.StartsOn != nil && today.Before(*p.StartsOn) {
		return false
	}
	if p.EndsOn != nil && today.After(*p.EndsOn) {
		return false
	}
	return true
}

// InChannel reports whether an offer applies to how the customer is buying.
// No channels listed means every channel, which is the common case.
func (p Promotion) InChannel(channel SalesChannel) bool {
	if len(p.Channels) == 0 {
		return true
	}
	for _, candidate := range p.Channels {
		if SalesChannel(candidate) == channel {
			return true
		}
	}
	return false
}

// EvaluatePromotions decides which offers a basket gets, and what they save.
//
// The stacking rule is the one decision that matters. An exclusive offer
// cannot be combined with anything, so the engine works out what the best
// exclusive offer alone would save, works out what every non-exclusive offer
// together would save, and gives the customer whichever is larger. Letting
// them stack instead is how a shop discovers, in the margin report, that three
// perfectly reasonable offers combined to sell something below cost.
func EvaluatePromotions(basket []BasketLine, promotions []Promotion,
	targetsOf map[string][]PromotionTarget, ctx PromotionContext) PromotionOutcome {

	if len(basket) == 0 {
		return PromotionOutcome{Applied: []AppliedPromotion{}}
	}

	code := strings.ToUpper(strings.TrimSpace(ctx.Code))
	var subtotal float64
	var units Quantity
	for _, line := range basket {
		subtotal += line.LineTotalIDR
		units += line.Qty
	}

	var exclusive, stacking []AppliedPromotion
	// Tracked so a typed code that saved nothing can say why.
	var typedRejection PromotionRejection
	typedFound := false

	for _, promotion := range promotions {
		typed := code != "" && strings.EqualFold(promotion.Code, code)
		if typed {
			typedFound = true
		}
		// An offer that has to be asked for is not applied to somebody who
		// did not ask.
		if promotion.RequiresCode && !typed {
			continue
		}

		if rejection := qualify(promotion, subtotal, units, ctx); rejection != "" {
			if typed {
				typedRejection = rejection
			}
			continue
		}

		discount := discountFor(promotion, targetsOf[promotion.ID], basket)
		if discount <= 0 {
			if typed {
				typedRejection = PromoRejectNoQualify
			}
			continue
		}

		applied := AppliedPromotion{
			PromotionID: promotion.ID, Code: promotion.Code,
			Name: promotion.Name, DiscountIDR: discount,
		}
		if promotion.Exclusive {
			exclusive = append(exclusive, applied)
		} else {
			stacking = append(stacking, applied)
		}
	}

	// The best single exclusive offer, against everything else added together.
	sort.SliceStable(exclusive, func(a, b int) bool {
		return exclusive[a].DiscountIDR > exclusive[b].DiscountIDR
	})
	var stackedTotal float64
	for _, applied := range stacking {
		stackedTotal += applied.DiscountIDR
	}

	outcome := PromotionOutcome{Applied: []AppliedPromotion{}}
	if len(exclusive) > 0 && exclusive[0].DiscountIDR >= stackedTotal {
		outcome.Applied = exclusive[:1]
		outcome.DiscountIDR = exclusive[0].DiscountIDR
	} else {
		sort.SliceStable(stacking, func(a, b int) bool {
			return stacking[a].DiscountIDR > stacking[b].DiscountIDR
		})
		outcome.Applied = stacking
		outcome.DiscountIDR = round2(stackedTotal)
	}

	// A discount can never exceed the basket: an offer that would make the
	// shop pay the customer is capped rather than honoured.
	if outcome.DiscountIDR > subtotal {
		outcome.DiscountIDR = round2(subtotal)
	}

	if code != "" && len(outcome.Applied) == 0 {
		if !typedFound {
			typedRejection = PromoRejectUnknown
		}
		outcome.Rejected = typedRejection
	}
	return outcome
}

// qualify checks everything about an offer that is not the goods.
func qualify(promotion Promotion, subtotal float64, units Quantity, ctx PromotionContext) PromotionRejection {
	if !promotion.Active {
		return PromoRejectInactive
	}
	if !promotion.Live(ctx.Today) {
		return PromoRejectWindow
	}
	if !promotion.InChannel(ctx.Channel) {
		return PromoRejectChannel
	}
	if subtotal < promotion.MinSpendIDR || units < promotion.MinQty {
		return PromoRejectMinSpend
	}
	if promotion.MaxUses != nil && promotion.UsedCount >= *promotion.MaxUses {
		return PromoRejectExhausted
	}
	if promotion.MaxUsesPerMember != nil {
		if ctx.MemberUses[promotion.ID] >= *promotion.MaxUsesPerMember {
			return PromoRejectPerMember
		}
	}
	return ""
}

// qualifying is the part of the basket an offer touches. No targets means all
// of it.
func qualifying(targets []PromotionTarget, basket []BasketLine) []BasketLine {
	if len(targets) == 0 {
		return basket
	}
	products := map[string]bool{}
	categories := map[string]bool{}
	for _, target := range targets {
		if target.ProductID != nil {
			products[*target.ProductID] = true
		}
		if target.CategoryID != nil {
			categories[*target.CategoryID] = true
		}
	}

	out := make([]BasketLine, 0, len(basket))
	for _, line := range basket {
		if products[line.ProductID] || (line.CategoryID != nil && categories[*line.CategoryID]) {
			out = append(out, line)
		}
	}
	return out
}

// discountFor is what one offer saves on this basket.
func discountFor(promotion Promotion, targets []PromotionTarget, basket []BasketLine) float64 {
	lines := qualifying(targets, basket)
	if len(lines) == 0 {
		return 0
	}

	var value float64
	for _, line := range lines {
		value += line.LineTotalIDR
	}

	switch promotion.Kind {
	case PromoPercent:
		if promotion.Percent == nil {
			return 0
		}
		return round2(value * *promotion.Percent / 100)

	case PromoAmount:
		if promotion.AmountIDR == nil {
			return 0
		}
		// Never more than the goods it targets are worth.
		return round2(math.Min(*promotion.AmountIDR, value))

	case PromoBuyXGetY:
		return buyXGetY(promotion, lines)

	case PromoBundle:
		return bundle(promotion, targets, basket)
	}
	return 0
}

// buyXGetY makes the cheapest of each qualifying group free.
//
// Cheapest, not first: "three for two" with a bar and two shakers should cost
// the two shakers, and giving away the dearest item instead is a promotion the
// shop cannot afford.
func buyXGetY(promotion Promotion, lines []BasketLine) float64 {
	if promotion.BuyQty == nil || promotion.FreeQty == nil {
		return 0
	}
	buy, free := int(*promotion.BuyQty), int(*promotion.FreeQty)
	if buy <= 0 || free <= 0 {
		return 0
	}

	// Every unit in the qualifying set, at what it actually costs.
	prices := []float64{}
	for _, line := range lines {
		count := int(line.Qty)
		unit := line.UnitPriceIDR
		if unit <= 0 && line.Qty > 0 {
			unit = line.LineTotalIDR / float64(line.Qty)
		}
		for i := 0; i < count; i++ {
			prices = append(prices, unit)
		}
	}
	group := buy + free
	if len(prices) < group {
		return 0
	}

	sort.Float64s(prices)
	groups := len(prices) / group
	var discount float64
	// The cheapest units overall are the free ones, taken from the front of
	// the sorted list.
	for i := 0; i < groups*free && i < len(prices); i++ {
		discount += prices[i]
	}
	return round2(discount)
}

// bundle prices a named set as one thing.
//
// The whole set has to be present. A bundle that half-applies is not a bundle,
// and discounting a partial set is how "meal deal" pricing leaks onto a basket
// containing one item of it.
func bundle(promotion Promotion, targets []PromotionTarget, basket []BasketLine) float64 {
	if promotion.BundlePriceIDR == nil || len(targets) == 0 {
		return 0
	}

	held := map[string]Quantity{}
	priceOf := map[string]float64{}
	for _, line := range basket {
		held[line.ProductID] += line.Qty
		if line.UnitPriceIDR > 0 {
			priceOf[line.ProductID] = line.UnitPriceIDR
		} else if line.Qty > 0 {
			priceOf[line.ProductID] = line.LineTotalIDR / float64(line.Qty)
		}
	}

	// How many complete bundles the basket holds.
	sets := -1
	var normal float64
	for _, target := range targets {
		if target.ProductID == nil || target.Qty <= 0 {
			return 0
		}
		available := held[*target.ProductID]
		if available < target.Qty {
			return 0
		}
		possible := int(available / target.Qty)
		if sets < 0 || possible < sets {
			sets = possible
		}
		normal += float64(target.Qty) * priceOf[*target.ProductID]
	}
	if sets <= 0 {
		return 0
	}

	saving := normal - *promotion.BundlePriceIDR
	if saving <= 0 {
		return 0
	}
	return round2(saving * float64(sets))
}

// ── Gift cards ───────────────────────────────────────────────────────────────

// GiftCardStatus is whether a card may be spent.
type GiftCardStatus string

const (
	GiftCardActive    GiftCardStatus = "ACTIVE"
	GiftCardFrozen    GiftCardStatus = "FROZEN"
	GiftCardExpired   GiftCardStatus = "EXPIRED"
	GiftCardCancelled GiftCardStatus = "CANCELLED"
)

// GiftCard is money somebody has already paid for.
//
// Deliberately not the credit wallet: credits buy classes and are a liability
// measured in sessions, while a gift card is money spendable on anything at
// the counter. Conflating them makes "what do we owe" unanswerable.
type GiftCard struct {
	ID         string         `json:"id"`
	Code       string         `json:"code"`
	Barcode    *string        `json:"barcode"`
	MemberID   *string        `json:"memberId"`
	IssuedOn   Date           `json:"issuedOn"`
	ExpiresOn  *Date          `json:"expiresOn"`
	InitialIDR float64        `json:"initialIdr"`
	BalanceIDR float64        `json:"balanceIdr"`
	Status     GiftCardStatus `json:"status"`
	IssuedBy   *string        `json:"issuedBy"`
	Note       *string        `json:"note"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

// GiftCardRejection explains why a card cannot pay.
type GiftCardRejection string

const (
	GiftRejectNotActive    GiftCardRejection = "CARD_NOT_ACTIVE"
	GiftRejectExpired      GiftCardRejection = "CARD_EXPIRED"
	GiftRejectInsufficient GiftCardRejection = "INSUFFICIENT_BALANCE"
	GiftRejectZero         GiftCardRejection = "ZERO_AMOUNT"
	GiftRejectWrongMember  GiftCardRejection = "WRONG_MEMBER"
)

// PostedGiftCard is what a movement does to a card.
type PostedGiftCard struct {
	AmountIDR     float64
	BalanceBefore float64
	BalanceAfter  float64
	Rejection     GiftCardRejection
}

// Allowed reports whether the movement may be written.
func (p PostedGiftCard) Allowed() bool { return p.Rejection == "" }

// SpendGiftCard is the one place a card's balance goes down.
//
// A named card belongs to somebody: letting a bearer spend it would make the
// name decoration. A bearer card — no member on it — is spendable by whoever
// holds it, which is the point of a gift.
func SpendGiftCard(card GiftCard, amountIDR float64, today Date, memberID *string) PostedGiftCard {
	posted := PostedGiftCard{
		AmountIDR: -round2(amountIDR), BalanceBefore: card.BalanceIDR,
	}
	if amountIDR <= 0 {
		posted.Rejection = GiftRejectZero
		return posted
	}
	if card.Status != GiftCardActive {
		posted.Rejection = GiftRejectNotActive
		return posted
	}
	if card.ExpiresOn != nil && card.ExpiresOn.Before(today) {
		posted.Rejection = GiftRejectExpired
		return posted
	}
	if card.MemberID != nil && (memberID == nil || *memberID != *card.MemberID) {
		posted.Rejection = GiftRejectWrongMember
		return posted
	}
	if round2(amountIDR) > card.BalanceIDR {
		posted.Rejection = GiftRejectInsufficient
		return posted
	}

	posted.BalanceAfter = round2(card.BalanceIDR - amountIDR)
	return posted
}

// TopUpGiftCard puts money on, which is the only other way a balance moves
// without somebody adjusting it by hand.
func TopUpGiftCard(card GiftCard, amountIDR float64) PostedGiftCard {
	posted := PostedGiftCard{
		AmountIDR: round2(amountIDR), BalanceBefore: card.BalanceIDR,
	}
	if amountIDR <= 0 {
		posted.Rejection = GiftRejectZero
		return posted
	}
	if card.Status == GiftCardCancelled || card.Status == GiftCardExpired {
		posted.Rejection = GiftRejectNotActive
		return posted
	}
	posted.BalanceAfter = round2(card.BalanceIDR + amountIDR)
	return posted
}

// SpendableIDR is what a card can pay towards a bill right now: its balance,
// or the bill, whichever is smaller.
func SpendableIDR(card GiftCard, dueIDR float64, today Date) float64 {
	if card.Status != GiftCardActive {
		return 0
	}
	if card.ExpiresOn != nil && card.ExpiresOn.Before(today) {
		return 0
	}
	return round2(math.Min(card.BalanceIDR, dueIDR))
}

// ── Tenders ──────────────────────────────────────────────────────────────────

// PaymentMethodKind is the behaviour a tender has, as opposed to its name.
//
// The name and whether it is switched on are configuration; whether it gives
// change is a rule, and rules do not belong in a form.
type PaymentMethodKind string

const (
	MethodCash         PaymentMethodKind = "CASH"
	MethodCard         PaymentMethodKind = "CARD"
	MethodQR           PaymentMethodKind = "QR"
	MethodTransfer     PaymentMethodKind = "TRANSFER"
	MethodMemberCredit PaymentMethodKind = "MEMBER_CREDIT"
	MethodGiftCard     PaymentMethodKind = "GIFT_CARD"
)

// IsValidMethodKind validates a kind arriving from a request.
func IsValidMethodKind(value string) bool {
	switch PaymentMethodKind(value) {
	case MethodCash, MethodCard, MethodQR, MethodTransfer, MethodMemberCredit, MethodGiftCard:
		return true
	}
	return false
}

// PaymentMethod is how the counter may be paid, as a row rather than a
// constant: a shop that signs up with a new QRIS provider on Tuesday should
// not need a deployment on Wednesday.
type PaymentMethod struct {
	ID             string            `json:"id"`
	Code           string            `json:"code"`
	Name           string            `json:"name"`
	Kind           PaymentMethodKind `json:"kind"`
	GivesChange    bool              `json:"givesChange"`
	NeedsReference bool              `json:"needsReference"`
	CountsInDrawer bool              `json:"countsInDrawer"`
	SortOrder      int               `json:"sortOrder"`
	Active         bool              `json:"active"`
}

// SettleWith works out where an order stands, given what each method does.
//
// It replaces the hard-coded "only cash gives change": which tenders give
// change and which count in the drawer are now properties of the method, so a
// shop adding a second cash-like tender does not have to change the rule.
func SettleWith(total float64, payments []POSPayment, methods map[string]PaymentMethod) SettlementOutcome {
	var paid, changeable float64
	for _, payment := range payments {
		paid += payment.AmountIDR
		if methods[string(payment.Method)].GivesChange {
			changeable += payment.AmountIDR
		}
	}
	paid = round2(paid)

	outcome := SettlementOutcome{PaidIDR: paid}
	switch {
	case paid <= 0:
		outcome.PaymentStatus = POSUnpaid
	case paid < total:
		outcome.PaymentStatus = POSPartial
	default:
		outcome.PaymentStatus = POSPaid
		outcome.Settled = true
		if over := round2(paid - total); over > 0 {
			// Change comes out of what was tendered in a form that can give
			// it back, never out of a card.
			outcome.ChangeIDR = math.Min(over, changeable)
		}
	}
	return outcome
}

// DrawerCash is what the till should hold: what was in it, plus every tender
// that counts in the drawer, minus the change handed back.
func DrawerCash(openingIDR float64, payments []POSPayment, methods map[string]PaymentMethod) float64 {
	total := openingIDR
	for _, payment := range payments {
		if !methods[string(payment.Method)].CountsInDrawer {
			continue
		}
		total += payment.AmountIDR - payment.ChangeIDR
	}
	return round2(total)
}

// EvaluateTenderWith checks one tender against what is still owed, given what
// the method can do.
//
// It replaces the hard-coded "a card cannot overpay": whether a tender can
// exceed the bill is now a property of the method, because the answer is the
// same question as whether it can give change back.
func EvaluateTenderWith(order POSOrder, method PaymentMethod, amount, alreadyPaid float64) POSRejection {
	if order.Status != POSOpen {
		return POSRejectOrderClosed
	}
	if order.TotalIDR <= 0 {
		return POSRejectNothingToPay
	}
	if !method.GivesChange && round2(alreadyPaid+amount) > order.TotalIDR {
		return POSRejectOverTendered
	}
	return ""
}
