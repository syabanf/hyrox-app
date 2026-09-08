package app_test

import (
	"net/http"
	"testing"
)

// Buying, over real HTTP against a real database.
//
// The two properties worth protecting: nobody signs a purchase alone, and a
// delivery moves stock exactly once, by exactly what actually turned up.

// order builds a sent purchase order for one item and returns its id and line id.
func (h *harness) sentOrder(t *testing.T, token, itemID string, qty float64, price float64) (string, string) {
	t.Helper()

	status, order := h.request(http.MethodPost, "/api/admin/purchasing/orders", token, map[string]any{
		"supplierId": "sup_nutrition", "branchId": senopati, "taxPercent": 11,
	})
	if status != http.StatusCreated {
		t.Fatalf("creating an order returned %d: %v", status, order)
	}
	orderID := order["id"].(string)

	status, withLine := h.request(http.MethodPost,
		"/api/admin/purchasing/orders/"+orderID+"/lines", token, map[string]any{
			// Explicitly in base units: these tests are about approvals and
			// receiving, and leaving the unit off would silently order in
			// whatever pack the item is normally bought by. The conversion
			// has its own test.
			"itemId": itemID, "description": "Test line", "qty": qty,
			"unit": "PCS", "unitPriceIdr": price,
		})
	if status != http.StatusCreated {
		t.Fatalf("adding a line returned %d: %v", status, withLine)
	}
	items := withLine["items"].([]any)
	lineID := items[0].(map[string]any)["id"].(string)

	h.request(http.MethodPost, "/api/admin/purchasing/orders/"+orderID+"/approve", token, map[string]any{})
	if status, sent := h.request(http.MethodPost,
		"/api/admin/purchasing/orders/"+orderID+"/send", token, map[string]any{}); status != http.StatusOK {
		t.Fatalf("sending returned %d: %v", status, sent)
	}
	return orderID, lineID
}

func TestNobodySignsAPurchaseAlone(t *testing.T) {
	h := newHarness(t)
	manager := h.adminToken("adm_branch")
	finance := h.adminToken("adm_finance")
	super := h.adminToken("adm_super")

	// A request large enough to need all three signatures.
	status, request := h.request(http.MethodPost, "/api/admin/purchasing/requests", manager, map[string]any{
		"branchId": senopati, "requesterName": "Bima Prasetyo", "priority": "HIGH",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating a request returned %d: %v", status, request)
	}
	requestID := request["id"].(string)

	h.request(http.MethodPost, "/api/admin/purchasing/requests/"+requestID+"/lines", manager, map[string]any{
		"itemId": "itm_whey", "description": "Whey Protein 1kg", "qty": 80, "estimatedPriceIdr": 445_000,
	})
	status, submitted := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+requestID+"/submit", manager, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("submitting returned %d: %v", status, submitted)
	}
	if len(submitted["chain"].([]any)) != 3 {
		t.Fatalf("35.6m should need three signatures, got %v", submitted["chain"])
	}

	// The manager signs the head level, and then cannot sign the next one.
	status, headSigned := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+requestID+"/approve", manager, map[string]any{})
	if status != http.StatusOK || headSigned["status"] != "PENDING_FINANCE" {
		t.Fatalf("after the head signs want PENDING_FINANCE, got %d: %v", status, headSigned)
	}
	status, refused := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+requestID+"/approve", manager, map[string]any{})
	if status != http.StatusForbidden {
		t.Fatalf("a manager must not sign the finance level, got %d: %v", status, refused)
	}

	// Finance, then a director.
	status, financeSigned := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+requestID+"/approve", finance, map[string]any{})
	if status != http.StatusOK || financeSigned["status"] != "PENDING_DIRECTOR" {
		t.Fatalf("want PENDING_DIRECTOR, got %d: %v", status, financeSigned)
	}
	status, approved := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+requestID+"/approve", super, map[string]any{})
	if status != http.StatusOK || approved["status"] != "APPROVED" {
		t.Fatalf("want APPROVED, got %d: %v", status, approved)
	}

	// And a fully signed request cannot be signed again.
	status, again := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+requestID+"/approve", super, map[string]any{})
	if status != http.StatusConflict || errorCode(again) != "ALREADY_APPROVED" {
		t.Fatalf("want ALREADY_APPROVED, got %d: %v", status, again)
	}
}

func TestASmallRequestNeedsOneSignature(t *testing.T) {
	h := newHarness(t)
	manager := h.adminToken("adm_branch")

	status, request := h.request(http.MethodPost, "/api/admin/purchasing/requests", manager, map[string]any{
		"branchId": senopati, "requesterName": "Bima Prasetyo",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating a request returned %d: %v", status, request)
	}
	requestID := request["id"].(string)

	// Small, but still not something the asker waves through alone.
	h.request(http.MethodPost, "/api/admin/purchasing/requests/"+requestID+"/lines", manager, map[string]any{
		"itemId": "itm_bar", "description": "Protein bars", "qty": 48, "estimatedPriceIdr": 16_500,
	})
	h.request(http.MethodPost, "/api/admin/purchasing/requests/"+requestID+"/submit", manager, map[string]any{})

	status, approved := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+requestID+"/approve", manager, map[string]any{})
	if status != http.StatusOK || approved["status"] != "APPROVED" {
		t.Fatalf("one signature should be enough, got %d: %v", status, approved)
	}
}

func TestAnEmptyRequestCannotBeSubmitted(t *testing.T) {
	h := newHarness(t)
	manager := h.adminToken("adm_branch")

	status, request := h.request(http.MethodPost, "/api/admin/purchasing/requests", manager, map[string]any{
		"branchId": senopati, "requesterName": "Bima Prasetyo",
	})
	if status != http.StatusCreated {
		t.Fatalf("creating a request returned %d: %v", status, request)
	}
	status, refused := h.request(http.MethodPost,
		"/api/admin/purchasing/requests/"+request["id"].(string)+"/submit", manager, map[string]any{})
	if status != http.StatusConflict || errorCode(refused) != "NO_LINES" {
		t.Fatalf("want NO_LINES, got %d: %v", status, refused)
	}
}

func TestReceivingMovesStockAndRevaluesIt(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	before := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64)
	status, itemBefore := h.request(http.MethodGet, "/api/admin/inventory/items/itm_bar", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the item returned %d", status)
	}
	costBefore := itemBefore["item"].(map[string]any)["unitCostIdr"].(float64)

	orderID, lineID := h.sentOrder(t, token, "itm_bar", 100, 16_500)

	// Sending reserves the goods, so the reorder screen stops asking for them.
	if onOrder := h.stockOf(t, token, "itm_bar", senopati)["qtyOnOrder"].(float64); onOrder != 100 {
		t.Fatalf("want 100 on order, got %v", onOrder)
	}

	// The supplier short-ships, and three arrive damaged.
	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID, "deliveryNoteNumber": "SJ-1"})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	receiptID := receipt["id"].(string)

	status, withLine := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/lines", token, map[string]any{
			"orderItemId": lineID, "qtyAccepted": 57, "qtyRejected": 3,
		})
	if status != http.StatusCreated {
		t.Fatalf("adding a delivery line returned %d: %v", status, withLine)
	}
	if got := withLine["items"].([]any)[0].(map[string]any)["qcStatus"]; got != "PARTIALLY_REJECTED" {
		t.Fatalf("want PARTIALLY_REJECTED, got %v", got)
	}

	// Nothing has moved until it is posted.
	if got := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64); got != before {
		t.Fatalf("a draft receipt must not move stock: %v -> %v", before, got)
	}

	status, posted := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/post", token, map[string]any{})
	if status != http.StatusOK || posted["status"] != "POSTED" {
		t.Fatalf("posting returned %d: %v", status, posted)
	}

	// Accepted goods entered stock; rejected ones did not.
	after := h.stockOf(t, token, "itm_bar", senopati)
	if after["qtyOnHand"].(float64) != before+57 {
		t.Fatalf("want %v on hand, got %v", before+57, after["qtyOnHand"])
	}
	// Neither the accepted nor the rejected 60 are on order any more.
	if after["qtyOnOrder"].(float64) != 40 {
		t.Fatalf("want 40 still on order, got %v", after["qtyOnOrder"])
	}

	// And the average cost moved towards what was actually paid.
	status, itemAfter := h.request(http.MethodGet, "/api/admin/inventory/items/itm_bar", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the item returned %d", status)
	}
	costAfter := itemAfter["item"].(map[string]any)["unitCostIdr"].(float64)
	if costAfter >= costBefore || costAfter <= 16_500 {
		t.Fatalf("a receipt at 16.500 against stock at %v should land between them, got %v",
			costBefore, costAfter)
	}

	// The order caught up with what has and has not arrived.
	status, order := h.request(http.MethodGet, "/api/admin/purchasing/orders/"+orderID, token, nil)
	if status != http.StatusOK || order["status"] != "PARTIALLY_RECEIVED" {
		t.Fatalf("want PARTIALLY_RECEIVED, got %d: %v", status, order["status"])
	}
}

func TestMoreCannotBeDeliveredThanWasOrdered(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	orderID, lineID := h.sentOrder(t, token, "itm_iso", 20, 10_500)

	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	receiptID := receipt["id"].(string)

	status, refused := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/lines", token, map[string]any{
			"orderItemId": lineID, "qtyAccepted": 21,
		})
	if status != http.StatusConflict || errorCode(refused) != "OVER_DELIVERED" {
		t.Fatalf("want OVER_DELIVERED, got %d: %v", status, refused)
	}

	// Two lines on the same draft cannot together overshoot either.
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receiptID+"/lines", token,
		map[string]any{"orderItemId": lineID, "qtyAccepted": 15})
	status, second := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/lines", token, map[string]any{
			"orderItemId": lineID, "qtyAccepted": 6,
		})
	if status != http.StatusConflict || errorCode(second) != "OVER_DELIVERED" {
		t.Fatalf("two lines must not overshoot together, got %d: %v", status, second)
	}
}

func TestAReceivedOrderCannotBeCancelled(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	orderID, lineID := h.sentOrder(t, token, "itm_shaker", 10, 31_000)

	// Before anything arrives, it can be — and the reservation is released.
	status, cancelled := h.request(http.MethodPost,
		"/api/admin/purchasing/orders/"+orderID+"/cancel", token,
		map[string]any{"reason": "Changed supplier"})
	if status != http.StatusOK || cancelled["status"] != "CANCELLED" {
		t.Fatalf("cancelling before delivery returned %d: %v", status, cancelled)
	}
	if onOrder := h.stockOf(t, token, "itm_shaker", senopati)["qtyOnOrder"].(float64); onOrder != 0 {
		t.Fatalf("a cancelled order must release its reservation, got %v", onOrder)
	}

	// After a delivery it cannot: stock has moved, and the ledger would point
	// at a document claiming nothing was ordered.
	orderID, lineID = h.sentOrder(t, token, "itm_shaker", 10, 31_000)
	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receipt["id"].(string)+"/lines",
		token, map[string]any{"orderItemId": lineID, "qtyAccepted": 4})
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receipt["id"].(string)+"/post",
		token, map[string]any{})

	status, refused := h.request(http.MethodPost,
		"/api/admin/purchasing/orders/"+orderID+"/cancel", token,
		map[string]any{"reason": "Too late"})
	if status != http.StatusConflict || errorCode(refused) != "INVALID_TRANSITION" {
		t.Fatalf("want INVALID_TRANSITION, got %d: %v", status, refused)
	}
}

func TestOnlyAcceptedGoodsCanBeReturned(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	orderID, lineID := h.sentOrder(t, token, "itm_towel", 30, 19_500)

	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	receiptID := receipt["id"].(string)
	status, withLine := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/lines", token, map[string]any{
			"orderItemId": lineID, "qtyAccepted": 20, "qtyRejected": 10,
		})
	if status != http.StatusCreated {
		t.Fatalf("adding a delivery line returned %d: %v", status, withLine)
	}
	receiptLineID := withLine["items"].([]any)[0].(map[string]any)["id"].(string)
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receiptID+"/post", token, map[string]any{})

	onHand := h.stockOf(t, token, "itm_towel", senopati)["qtyOnHand"].(float64)

	status, ret := h.request(http.MethodPost, "/api/admin/purchasing/returns", token, map[string]any{
		"receiptId": receiptID, "reasonType": "DAMAGED", "reasonNote": "Frayed",
	})
	if status != http.StatusCreated {
		t.Fatalf("opening a return returned %d: %v", status, ret)
	}
	returnID := ret["id"].(string)

	// The ten rejected ones never entered stock, so only twenty can go back.
	status, refused := h.request(http.MethodPost,
		"/api/admin/purchasing/returns/"+returnID+"/lines", token, map[string]any{
			"receiptItemId": receiptLineID, "qty": 21,
		})
	if status != http.StatusConflict || errorCode(refused) != "OVER_RETURNED" {
		t.Fatalf("want OVER_RETURNED, got %d: %v", status, refused)
	}

	h.request(http.MethodPost, "/api/admin/purchasing/returns/"+returnID+"/lines", token,
		map[string]any{"receiptItemId": receiptLineID, "qty": 5})
	status, posted := h.request(http.MethodPost,
		"/api/admin/purchasing/returns/"+returnID+"/post", token, map[string]any{})
	if status != http.StatusOK || posted["status"] != "POSTED" {
		t.Fatalf("posting the return returned %d: %v", status, posted)
	}
	if got := h.stockOf(t, token, "itm_towel", senopati)["qtyOnHand"].(float64); got != onHand-5 {
		t.Fatalf("want %v after the return, got %v", onHand-5, got)
	}
}

func TestBlockedSuppliersCannotBeOrderedFromOverHTTP(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, supplier := h.request(http.MethodGet, "/api/admin/purchasing/suppliers/sup_supplies", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the supplier returned %d", status)
	}
	if status, body := h.request(http.MethodPut,
		"/api/admin/purchasing/suppliers/sup_supplies", token, map[string]any{
			"code": supplier["code"], "name": supplier["name"], "status": "BLOCKED",
		}); status != http.StatusOK {
		t.Fatalf("blocking returned %d: %v", status, body)
	}

	status, refused := h.request(http.MethodPost, "/api/admin/purchasing/orders", token, map[string]any{
		"supplierId": "sup_supplies", "branchId": senopati,
	})
	if status != http.StatusConflict || errorCode(refused) != "SUPPLIER_NOT_ORDERABLE" {
		t.Fatalf("want SUPPLIER_NOT_ORDERABLE, got %d: %v", status, refused)
	}
}

func TestPurchasingPermissionsSeparateTheThreePairsOfHands(t *testing.T) {
	h := newHarness(t)

	// Finance signs but does not raise orders, and never signs for goods.
	finance := h.adminToken("adm_finance")
	if status, _ := h.requestList(http.MethodGet, "/api/admin/purchasing/orders", finance, nil); status != http.StatusOK {
		t.Fatalf("finance should see orders, got %d", status)
	}
	if status, _ := h.request(http.MethodPost, "/api/admin/purchasing/orders", finance, map[string]any{
		"supplierId": "sup_nutrition", "branchId": senopati,
	}); status != http.StatusForbidden {
		t.Fatalf("finance must not raise orders, got %d", status)
	}
	if status, _ := h.request(http.MethodPost, "/api/admin/purchasing/receipts", finance, map[string]any{
		"orderId": "po_missing",
	}); status != http.StatusForbidden {
		t.Fatalf("finance must not sign for goods, got %d", status)
	}

	// The counter and the gym floor are not in this process at all.
	for _, user := range []string{"adm_desk", "adm_coach"} {
		token := h.adminToken(user)
		if status, _ := h.requestList(http.MethodGet, "/api/admin/purchasing/orders", token, nil); status != http.StatusForbidden {
			t.Fatalf("%s must not see purchase orders, got %d", user, status)
		}
	}
}
