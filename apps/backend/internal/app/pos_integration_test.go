package app_test

import (
	"net/http"
	"testing"
)

// The till, over real HTTP against a real database.
//
// The property that matters most: completing a sale moves stock, awards
// points and takes the money in one transaction. A paid sale that did not
// move stock, or stock that moved for a sale nobody paid for, are both bugs
// the counter would never notice until stocktake.

// openTill starts a shift and returns its id.
func (h *harness) openTill(t *testing.T, token string, float float64) string {
	t.Helper()
	status, shift := h.request(http.MethodPost, "/api/admin/pos/shifts", token,
		map[string]any{"branchId": senopati, "openingCashIdr": float})
	if status != http.StatusCreated {
		t.Fatalf("opening a till returned %d: %v", status, shift)
	}
	return shift["id"].(string)
}

// sale opens an order, puts one product on it and returns its id.
func (h *harness) sale(t *testing.T, token, productID string, qty float64, memberID *string) string {
	t.Helper()
	body := map[string]any{"branchId": senopati}
	if memberID != nil {
		body["memberId"] = *memberID
	}
	status, order := h.request(http.MethodPost, "/api/admin/pos/orders", token, body)
	if status != http.StatusCreated {
		t.Fatalf("opening a sale returned %d: %v", status, order)
	}
	orderID := order["id"].(string)

	status, withLine := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/lines",
		token, map[string]any{"productId": productID, "qty": qty})
	if status != http.StatusCreated {
		t.Fatalf("adding a line returned %d: %v", status, withLine)
	}
	return orderID
}

func TestASaleMovesStockAndAwardsPointsTogether(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	before := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64)
	member := demoMember
	orderID := h.sale(t, token, "prd_bar", 3, &member)

	// Nothing has moved while the sale is open.
	if got := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64); got != before {
		t.Fatalf("an open sale must not move stock: %v -> %v", before, got)
	}

	status, order := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d", status)
	}
	total := order["totalIdr"].(float64)

	// It cannot be completed until it is paid for.
	status, unpaid := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete",
		token, map[string]any{})
	if status != http.StatusConflict || errorCode(unpaid) != "NOT_SETTLED" {
		t.Fatalf("want NOT_SETTLED, got %d: %v", status, unpaid)
	}

	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
		map[string]any{"method": "CASH", "amountIdr": total})
	status, completed := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete",
		token, map[string]any{})
	if status != http.StatusOK || completed["status"] != "COMPLETED" {
		t.Fatalf("completing returned %d: %v", status, completed)
	}

	// Stock went out, and points went on, in the same act.
	if got := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64); got != before-3 {
		t.Fatalf("want %v on hand, got %v", before-3, got)
	}
	if completed["xpEarned"].(float64) <= 0 {
		t.Fatalf("a sale to a named member should earn points, got %v", completed["xpEarned"])
	}
}

func TestSellingMoreThanIsOnTheShelfIsRefused(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 0)

	onHand := h.stockOf(t, token, "itm_whey", "brn_pik")["qtyOnHand"].(float64)

	// The PIK branch holds very little whey. Try to sell more than exists.
	status, order := h.request(http.MethodPost, "/api/admin/pos/orders", token,
		map[string]any{"branchId": "brn_pik"})
	if status != http.StatusConflict {
		// The till is open at Senopati, not PIK, so this is refused first —
		// which is itself the rule that a sale needs a till behind it.
		if errorCode(order) != "NO_OPEN_SHIFT" {
			t.Fatalf("want NO_OPEN_SHIFT, got %d: %v", status, order)
		}
	}

	// At Senopati, sell more shakers than are there.
	senopatiOnHand := h.stockOf(t, token, "itm_shaker", senopati)["qtyOnHand"].(float64)
	orderID := h.sale(t, token, "prd_shaker", senopatiOnHand+1, nil)
	status, read := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d", status)
	}
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
		map[string]any{"method": "CASH", "amountIdr": read["totalIdr"]})

	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete",
		token, map[string]any{})
	if status != http.StatusConflict || errorCode(refused) != "INSUFFICIENT_STOCK" {
		t.Fatalf("want INSUFFICIENT_STOCK, got %d: %v", status, refused)
	}
	// And because it all happens in one transaction, the money is not counted
	// as a completed sale either.
	status, order = h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if order["status"] != "OPEN" {
		t.Fatalf("a failed completion leaves the sale open, got %v", order["status"])
	}
	_ = onHand
}

func TestSellingAServiceMovesNoStock(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 0)

	// Towel hire has a price and no inventory item: it should sell happily
	// without anything coming off a shelf.
	orderID := h.sale(t, token, "prd_towel_hire", 2, nil)
	status, order := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d", status)
	}
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
		map[string]any{"method": "QRIS", "amountIdr": order["totalIdr"]})

	status, completed := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete",
		token, map[string]any{})
	if status != http.StatusOK || completed["status"] != "COMPLETED" {
		t.Fatalf("a service should sell, got %d: %v", status, completed)
	}
}

func TestATierDiscountDoesNotCompoundAcrossLines(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 0)

	// Put the member on a tier that carries a discount.
	h.request(http.MethodPost, "/api/admin/crm/members/"+demoMember+"/adjust", token,
		map[string]any{"xpDelta": 900, "reason": "Test"})

	member := demoMember
	status, order := h.request(http.MethodPost, "/api/admin/pos/orders", token,
		map[string]any{"branchId": senopati, "memberId": member})
	if status != http.StatusCreated {
		t.Fatalf("opening a sale returned %d: %v", status, order)
	}
	orderID := order["id"].(string)

	// Scan the same product three times: the discount must stay a fixed
	// percentage of the subtotal rather than growing with every line.
	var last map[string]any
	for range 3 {
		status, updated := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/lines",
			token, map[string]any{"productId": "prd_bar", "qty": 1})
		if status != http.StatusCreated {
			t.Fatalf("adding a line returned %d: %v", status, updated)
		}
		last = updated

		subtotal := updated["subtotalIdr"].(float64)
		tier := updated["tierDiscountIdr"].(float64)
		if subtotal > 0 && (tier/subtotal < 0.049 || tier/subtotal > 0.051) {
			t.Fatalf("the tier discount compounded: %v of %v is %.2f%%",
				tier, subtotal, tier/subtotal*100)
		}
	}
	if last["totalIdr"].(float64) != 99_750 {
		t.Fatalf("want 99.750 after three bars at 5%% off, got %v", last["totalIdr"])
	}
}

func TestACardCannotOverpayButCashGivesChange(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 0)

	orderID := h.sale(t, token, "prd_bar", 2, nil)
	status, order := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d", status)
	}
	total := order["totalIdr"].(float64)

	// A card has no change to give, so the difference would simply be lost.
	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender",
		token, map[string]any{"method": "QRIS", "amountIdr": total + 50_000})
	if status != http.StatusConflict || errorCode(refused) != "OVER_TENDERED" {
		t.Fatalf("want OVER_TENDERED, got %d: %v", status, refused)
	}

	status, paid := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender",
		token, map[string]any{"method": "CASH", "amountIdr": total + 50_000})
	if status != http.StatusCreated {
		t.Fatalf("cash may overpay, got %d: %v", status, paid)
	}
	if paid["changeIdr"].(float64) != 50_000 {
		t.Fatalf("want 50.000 change, got %v", paid["changeIdr"])
	}
	if paid["paymentStatus"] != "PAID" {
		t.Fatalf("want PAID, got %v", paid["paymentStatus"])
	}
}

func TestVoidingPutsTheStockBack(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 0)

	before := h.stockOf(t, token, "itm_tee", senopati)["qtyOnHand"].(float64)
	orderID := h.sale(t, token, "prd_tee", 2, nil)
	status, order := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d", status)
	}
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
		map[string]any{"method": "CASH", "amountIdr": order["totalIdr"]})
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete", token, map[string]any{})

	if got := h.stockOf(t, token, "itm_tee", senopati)["qtyOnHand"].(float64); got != before-2 {
		t.Fatalf("the sale should have taken two: %v -> %v", before, got)
	}

	// Voiding needs a reason.
	status, noReason := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void",
		token, map[string]any{"reason": " "})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("want a validation failure, got %d: %v", status, noReason)
	}

	status, voided := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void",
		token, map[string]any{"reason": "Wrong size"})
	if status != http.StatusOK || voided["status"] != "VOIDED" {
		t.Fatalf("voiding returned %d: %v", status, voided)
	}
	if got := h.stockOf(t, token, "itm_tee", senopati)["qtyOnHand"].(float64); got != before {
		t.Fatalf("voiding must put the stock back: want %v, got %v", before, got)
	}
}

func TestAPaidSaleIsVoidedAndAnUnpaidOneIsCancelled(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 0)

	// Unpaid: cancelling is the right act, and nothing is unwound.
	orderID := h.sale(t, token, "prd_bar", 1, nil)
	status, cancelled := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/cancel",
		token, map[string]any{})
	if status != http.StatusOK || cancelled["status"] != "CANCELLED" {
		t.Fatalf("cancelling an unpaid sale returned %d: %v", status, cancelled)
	}

	// Paid: cancelling is refused, because conflating the two would let the
	// counter erase a paid sale by calling it a cancellation.
	orderID = h.sale(t, token, "prd_bar", 1, nil)
	status, order := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d", status)
	}
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
		map[string]any{"method": "CASH", "amountIdr": order["totalIdr"]})

	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/cancel",
		token, map[string]any{})
	if status != http.StatusConflict || errorCode(refused) != "ALREADY_PAID" {
		t.Fatalf("want ALREADY_PAID, got %d: %v", status, refused)
	}
}

func TestATillCannotBeClosedWithASaleStillOpen(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	shiftID := h.openTill(t, token, 500_000)

	h.sale(t, token, "prd_bar", 1, nil)

	// An unfinished sale would be stranded: its shift closed and its takings
	// counted without it.
	status, refused := h.request(http.MethodPost, "/api/admin/pos/shifts/"+shiftID+"/close",
		token, map[string]any{"countedCashIdr": 500_000})
	if status != http.StatusConflict || errorCode(refused) != "ORDERS_STILL_OPEN" {
		t.Fatalf("want ORDERS_STILL_OPEN, got %d: %v", status, refused)
	}
}

func TestTheDrawerCountsOnlyCash(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	shiftID := h.openTill(t, token, 500_000)

	// One cash sale and one card sale.
	for _, method := range []string{"CASH", "QRIS"} {
		orderID := h.sale(t, token, "prd_bar", 2, nil)
		status, order := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
		if status != http.StatusOK {
			t.Fatalf("reading the sale returned %d", status)
		}
		h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
			map[string]any{"method": method, "amountIdr": order["totalIdr"]})
		h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete", token, map[string]any{})
	}

	status, closed := h.request(http.MethodPost, "/api/admin/pos/shifts/"+shiftID+"/close",
		token, map[string]any{"countedCashIdr": 570_000})
	if status != http.StatusOK {
		t.Fatalf("closing returned %d: %v", status, closed)
	}
	// 500.000 opening plus the 70.000 cash sale. The card sale never touched
	// the drawer, and counting it would make every till look short by exactly
	// the card takings.
	if closed["expectedCashIdr"].(float64) != 570_000 {
		t.Fatalf("want 570.000 expected, got %v", closed["expectedCashIdr"])
	}
	if closed["varianceIdr"].(float64) != 0 {
		t.Fatalf("want no variance, got %v", closed["varianceIdr"])
	}
}

func TestPOSPermissionsSeparateSellingFromUnwinding(t *testing.T) {
	h := newHarness(t)
	desk := h.adminToken("adm_desk")
	manager := h.adminToken("adm_super")

	// The desk sells.
	shiftID := h.openTill(t, desk, 0)
	orderID := h.sale(t, desk, "prd_bar", 1, nil)
	status, order := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, desk, nil)
	if status != http.StatusOK {
		t.Fatalf("the desk should read its own sale, got %d", status)
	}
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", desk,
		map[string]any{"method": "CASH", "amountIdr": order["totalIdr"]})
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete", desk, map[string]any{})

	// But unwinding a paid sale is the one action that makes money disappear.
	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void",
		desk, map[string]any{"reason": "Changed my mind"})
	if status != http.StatusForbidden {
		t.Fatalf("the desk must not void a sale, got %d: %v", status, refused)
	}
	if status, voided := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void",
		manager, map[string]any{"reason": "Wrong size"}); status != http.StatusOK {
		t.Fatalf("a manager may void, got %d: %v", status, voided)
	}

	// And a coach is not at the counter at all.
	coach := h.adminToken("adm_coach")
	if status, _ := h.requestList(http.MethodGet, "/api/admin/pos/orders", coach, nil); status != http.StatusForbidden {
		t.Fatalf("a coach must not see the till, got %d", status)
	}
	_ = shiftID
}
