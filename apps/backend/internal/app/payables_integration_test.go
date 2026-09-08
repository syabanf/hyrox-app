package app_test

import (
	"net/http"
	"testing"
)

// What arrived, what we owe, and who signed.

func TestADeliveryIsCountedBeforeAnybodyJudgesIt(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	orderID, lineID := h.sentOrder(t, token, "itm_bar", 100, 16_500)

	before := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64)

	status, delivery := h.request(http.MethodPost, "/api/admin/purchasing/deliveries", token,
		map[string]any{
			"orderId": orderID, "deliveryNoteNumber": "SJ-991",
			"driverName": "Pak Yanto", "vehicle": "B 1234 XY",
		})
	if status != http.StatusCreated {
		t.Fatalf("opening a delivery returned %d: %v", status, delivery)
	}
	deliveryID := delivery["id"].(string)

	status, withLine := h.request(http.MethodPost,
		"/api/admin/purchasing/deliveries/"+deliveryID+"/lines", token,
		map[string]any{"orderItemId": lineID, "qty": 70,
			"batchNumber": "PB-DEL", "expiresOn": "2027-05-31"})
	if status != http.StatusCreated {
		t.Fatalf("counting a delivery line returned %d: %v", status, withLine)
	}

	// This is the whole point of the document: goods are on the bay and
	// nothing has moved. Folding delivery and receipt together is what makes a
	// short delivery indistinguishable from a rejection.
	if got := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64); got != before {
		t.Fatalf("a delivery must not move stock: %v -> %v", before, got)
	}

	// And a truck cannot bring more than was ordered. Seventy have arrived, so
	// forty more would be a hundred and ten.
	status, over := h.request(http.MethodPost,
		"/api/admin/purchasing/deliveries/"+deliveryID+"/lines", token,
		map[string]any{"orderItemId": lineID, "qty": 40})
	if status != http.StatusConflict || errorCode(over) != "OVER_DELIVERED" {
		t.Fatalf("want OVER_DELIVERED, got %d: %v", status, over)
	}

	// Nothing has been inspected, so the delivery cannot close.
	status, early := h.request(http.MethodPost,
		"/api/admin/purchasing/deliveries/"+deliveryID+"/close", token, map[string]any{})
	if status != http.StatusConflict || errorCode(early) != "NOT_INSPECTED" {
		t.Fatalf("want NOT_INSPECTED, got %d: %v", status, early)
	}

	// Inspect it: sixty good, ten damaged. Both are decisions, so the delivery
	// is fully judged and closes.
	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID, "deliveryId": deliveryID})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	receiptID := receipt["id"].(string)
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receiptID+"/lines", token,
		map[string]any{"orderItemId": lineID, "qtyAccepted": 60, "qtyRejected": 10,
			"batchNumber": "PB-DEL", "expiresOn": "2027-05-31"})
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receiptID+"/post", token,
		map[string]any{})

	status, closed := h.request(http.MethodPost,
		"/api/admin/purchasing/deliveries/"+deliveryID+"/close", token, map[string]any{})
	if status != http.StatusOK || closed["status"] != "INSPECTED" {
		t.Fatalf("closing the delivery returned %d: %v", status, closed)
	}
	if got := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64); got != before+60 {
		t.Fatalf("only the accepted sixty enter stock, got %v", got-before)
	}
}

func TestAnOrderIsScheduledPaidAndSettled(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	orderID, _ := h.sentOrder(t, token, "itm_iso", 100, 10_500)

	status, payables := h.request(http.MethodPost,
		"/api/admin/purchasing/orders/"+orderID+"/payables/schedule", token, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("scheduling returned %d: %v", status, payables)
	}
	terms := payables["terms"].([]any)
	if len(terms) != 1 {
		t.Fatalf("a term code makes one instalment, got %d", len(terms))
	}
	total := payables["position"].(map[string]any)["totalIdr"].(float64)

	// Split it in two, which is the thing a term code cannot say.
	status, split := h.request(http.MethodPut,
		"/api/admin/purchasing/orders/"+orderID+"/payables/terms", token, map[string]any{
			"terms": []map[string]any{
				{"sequence": 1, "label": "On order", "dueOn": "2026-09-08", "percent": 50},
				{"sequence": 2, "label": "On delivery", "dueOn": "2026-10-08", "percent": 50},
			},
		})
	if status != http.StatusOK {
		t.Fatalf("saving instalments returned %d: %v", status, split)
	}
	splitTerms := split["terms"].([]any)
	var scheduled float64
	for _, term := range splitTerms {
		scheduled += term.(map[string]any)["amountIdr"].(float64)
	}
	// The instalments add up to the order exactly — the last one absorbs the
	// rounding, because a payables report that is a rupiah out is one nobody
	// trusts.
	if scheduled != total {
		t.Fatalf("instalments must add to %v, got %v", total, scheduled)
	}

	// A schedule that does not add up to the whole order is refused.
	status, lopsided := h.request(http.MethodPut,
		"/api/admin/purchasing/orders/"+orderID+"/payables/terms", token, map[string]any{
			"terms": []map[string]any{
				{"sequence": 1, "dueOn": "2026-09-08", "percent": 40},
			},
		})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a 40%% schedule must be refused, got %d: %v", status, lopsided)
	}

	firstTerm := splitTerms[0].(map[string]any)
	termAmount := firstTerm["amountIdr"].(float64)

	// Paying more than an instalment needs is either a mistake or a payment
	// against something else. Swallowing the difference loses whichever.
	status, big := h.request(http.MethodPost, "/api/admin/purchasing/payments", token,
		map[string]any{
			"supplierId": "sup_nutrition", "orderId": orderID,
			"termId": firstTerm["id"], "amountIdr": termAmount + 500_000,
		})
	if status != http.StatusCreated {
		t.Fatalf("recording a payment returned %d: %v", status, big)
	}
	status, refused := h.request(http.MethodPost,
		"/api/admin/purchasing/payments/"+big["id"].(string)+"/post", token, map[string]any{})
	if status != http.StatusConflict || errorCode(refused) != "OVERPAID" {
		t.Fatalf("want OVERPAID, got %d: %v", status, refused)
	}

	// The right amount settles the instalment and nothing else.
	status, payment := h.request(http.MethodPost, "/api/admin/purchasing/payments", token,
		map[string]any{
			"supplierId": "sup_nutrition", "orderId": orderID,
			"termId": firstTerm["id"], "amountIdr": termAmount, "method": "TRANSFER",
		})
	if status != http.StatusCreated {
		t.Fatalf("recording a payment returned %d: %v", status, payment)
	}
	paymentID := payment["id"].(string)

	// A draft payment has not moved any money yet.
	status, before := h.request(http.MethodGet,
		"/api/admin/purchasing/orders/"+orderID+"/payables", token, nil)
	if status != http.StatusOK || before["position"].(map[string]any)["paidIdr"].(float64) != 0 {
		t.Fatalf("a draft payment pays nothing, got %v", before["position"])
	}

	status, posted := h.request(http.MethodPost,
		"/api/admin/purchasing/payments/"+paymentID+"/post", token, map[string]any{})
	if status != http.StatusOK || posted["status"] != "POSTED" {
		t.Fatalf("posting the payment returned %d: %v", status, posted)
	}

	status, after := h.request(http.MethodGet,
		"/api/admin/purchasing/orders/"+orderID+"/payables", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading payables returned %d", status)
	}
	position := after["position"].(map[string]any)
	if position["paidIdr"].(float64) != termAmount {
		t.Fatalf("the instalment is paid, got %v", position["paidIdr"])
	}
	if position["outstandingIdr"].(float64) != total-termAmount {
		t.Fatalf("half is still owed, got %v", position["outstandingIdr"])
	}

	// Voiding gives the debt back, rather than quietly leaving it settled.
	status, voided := h.request(http.MethodPost,
		"/api/admin/purchasing/payments/"+paymentID+"/void", token,
		map[string]any{"reason": "Paid the wrong supplier"})
	if status != http.StatusOK || voided["status"] != "VOIDED" {
		t.Fatalf("voiding returned %d: %v", status, voided)
	}
	status, unwound := h.request(http.MethodGet,
		"/api/admin/purchasing/orders/"+orderID+"/payables", token, nil)
	if status != http.StatusOK {
		t.Fatalf("re-reading payables returned %d", status)
	}
	if unwound["position"].(map[string]any)["paidIdr"].(float64) != 0 {
		t.Fatalf("a voided payment owes the money again, got %v", unwound["position"])
	}
}

func TestAReturnRaisesACreditNoteThatPaysTheNextInvoice(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// Receive, then send some back.
	orderID, lineID := h.sentOrder(t, token, "itm_towel", 40, 19_500)
	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	receiptID := receipt["id"].(string)
	status, withLine := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/lines", token,
		map[string]any{"orderItemId": lineID, "qtyAccepted": 40})
	if status != http.StatusCreated {
		t.Fatalf("adding a receipt line returned %d: %v", status, withLine)
	}
	receiptLineID := withLine["items"].([]any)[0].(map[string]any)["id"].(string)
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receiptID+"/post", token, map[string]any{})

	status, ret := h.request(http.MethodPost, "/api/admin/purchasing/returns", token, map[string]any{
		"receiptId": receiptID, "reasonType": "DAMAGED", "reasonNote": "Torn packaging",
	})
	if status != http.StatusCreated {
		t.Fatalf("opening a return returned %d: %v", status, ret)
	}
	returnID := ret["id"].(string)
	h.request(http.MethodPost, "/api/admin/purchasing/returns/"+returnID+"/lines", token,
		map[string]any{"receiptItemId": receiptLineID, "qty": 10})

	// A rejected return goes back to draft: the usual outcome of "no, not that
	// one" is a corrected return, not an abandoned one.
	h.request(http.MethodPost, "/api/admin/purchasing/returns/"+returnID+"/submit", token, map[string]any{})
	status, rejected := h.request(http.MethodPost,
		"/api/admin/purchasing/returns/"+returnID+"/reject", token,
		map[string]any{"note": "Try the supplier's own courier first"})
	if status != http.StatusOK || rejected["status"] != "REJECTED" {
		t.Fatalf("rejecting returned %d: %v", status, rejected)
	}
	// It goes back to whoever raised it, rather than dying there.
	status, revised := h.request(http.MethodPost,
		"/api/admin/purchasing/returns/"+returnID+"/revise", token, map[string]any{})
	if status != http.StatusOK || revised["status"] != "DRAFT" {
		t.Fatalf("a rejected return goes back to draft, got %d: %v", status, revised)
	}

	// Rejecting without saying why is refused.
	h.request(http.MethodPost, "/api/admin/purchasing/returns/"+returnID+"/submit", token, map[string]any{})
	status, silent := h.request(http.MethodPost,
		"/api/admin/purchasing/returns/"+returnID+"/reject", token, map[string]any{})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a rejection needs a reason, got %d: %v", status, silent)
	}

	status, approved := h.request(http.MethodPost,
		"/api/admin/purchasing/returns/"+returnID+"/approve", token,
		map[string]any{"note": "Agreed"})
	if status != http.StatusOK || approved["status"] != "APPROVED" {
		t.Fatalf("approving returned %d: %v", status, approved)
	}
	status, posted := h.request(http.MethodPost,
		"/api/admin/purchasing/returns/"+returnID+"/post", token, map[string]any{})
	if status != http.StatusOK || posted["status"] != "POSTED" {
		t.Fatalf("posting the return returned %d: %v", status, posted)
	}

	// Goods going back are money coming back, at the moment the stock leaves
	// rather than whenever somebody remembers.
	status, credits := h.requestList(http.MethodGet,
		"/api/admin/purchasing/credits?supplierId=sup_nutrition&openOnly=true", token, nil)
	if status != http.StatusOK || len(credits) != 1 {
		t.Fatalf("the return should have raised one credit note, got %d rows", len(credits))
	}
	if credits[0]["amountIdr"].(float64) != 195_000 {
		t.Fatalf("ten towels at 19.500 is 195.000, got %v", credits[0]["amountIdr"])
	}

	// The next payment spends it before any cash moves.
	status, payment := h.request(http.MethodPost, "/api/admin/purchasing/payments", token,
		map[string]any{
			"supplierId": "sup_nutrition", "amountIdr": 500_000, "useCredits": true,
		})
	if status != http.StatusCreated {
		t.Fatalf("recording a payment returned %d: %v", status, payment)
	}
	if payment["creditIdr"].(float64) != 195_000 {
		t.Fatalf("the credit note should have covered 195.000, got %v", payment["creditIdr"])
	}

	// And it cannot be spent twice.
	status, spent := h.requestList(http.MethodGet,
		"/api/admin/purchasing/credits?supplierId=sup_nutrition&openOnly=true", token, nil)
	if status != http.StatusOK || len(spent) != 0 {
		t.Fatalf("the credit note is used up, got %d still open", len(spent))
	}
}

func TestPayingASupplierIsAFinanceGrantNotABuyingOne(t *testing.T) {
	h := newHarness(t)
	manager := h.adminToken("adm_branch")
	finance := h.adminToken("adm_finance")

	// A branch manager raises and approves orders and signs for goods. They do
	// not move money out of the building.
	status, refused := h.request(http.MethodPost, "/api/admin/purchasing/payments", manager,
		map[string]any{"supplierId": "sup_nutrition", "amountIdr": 100_000})
	if status != http.StatusForbidden {
		t.Fatalf("a branch manager must not pay a supplier, got %d: %v", status, refused)
	}

	status, allowed := h.request(http.MethodPost, "/api/admin/purchasing/payments", finance,
		map[string]any{"supplierId": "sup_nutrition", "amountIdr": 100_000})
	if status != http.StatusCreated {
		t.Fatalf("finance may pay a supplier, got %d: %v", status, allowed)
	}

	// But reading what is owed is not a privilege — a buyer needs to know.
	status, payables := h.requestList(http.MethodGet, "/api/admin/purchasing/payments", manager, nil)
	if status != http.StatusOK {
		t.Fatalf("a buyer may read the payment run, got %d: %v", status, payables)
	}
}
