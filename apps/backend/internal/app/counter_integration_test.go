package app_test

import (
	"net/http"
	"strings"
	"testing"
)

// Offers, cards, tenders, receipts and the manager standing there.

func TestAnOfferAppliesItselfAndACodeHasToBeAskedFor(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	// Three-for-two ships switched off: an offer that discounts every sale
	// from the moment the shop opens is one nobody decided to run.
	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_bar", 3)
	status, plain := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK || plain["totalIdr"].(float64) != 105_000 {
		t.Fatalf("three bars at 35.000 is 105.000, got %v", plain["totalIdr"])
	}

	// Switch it on.
	status, saved := h.request(http.MethodPut, "/api/admin/pos/promotions", token, map[string]any{
		"code": "BAR3FOR2", "name": "Protein bars: three for two", "kind": "BUY_X_GET_Y",
		"buyQty": 2, "freeQty": 1, "priority": 10, "active": true,
		"targets": []map[string]any{{"productId": "prd_bar", "qty": 1}},
	})
	if status != http.StatusOK {
		t.Fatalf("saving the offer returned %d: %v", status, saved)
	}

	// A new basket gets it without anybody asking.
	second := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, second, "prd_bar", 3)
	status, discounted := h.request(http.MethodGet, "/api/admin/pos/orders/"+second, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d", status)
	}
	if discounted["promoDiscountIdr"].(float64) != 35_000 {
		t.Fatalf("the cheapest of three is free, got %v", discounted["promoDiscountIdr"])
	}
	if discounted["totalIdr"].(float64) != 70_000 {
		t.Fatalf("three for two is 70.000, got %v", discounted["totalIdr"])
	}

	// An offer that has to be asked for does nothing until it is.
	third := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, third, "prd_whey", 1)
	status, before := h.request(http.MethodGet, "/api/admin/pos/orders/"+third, token, nil)
	if status != http.StatusOK || before["promoDiscountIdr"].(float64) != 0 {
		t.Fatalf("a code offer applies to nobody who did not ask, got %v", before)
	}

	status, coded := h.request(http.MethodPost, "/api/admin/pos/orders/"+third+"/code", token,
		map[string]any{"code": "LAUNCH25"})
	if status != http.StatusOK {
		t.Fatalf("applying a code returned %d: %v", status, coded)
	}
	// 25% of 749.000.
	if coded["promoDiscountIdr"].(float64) != 187_250 {
		t.Fatalf("25%% of 749.000 is 187.250, got %v", coded["promoDiscountIdr"])
	}

	// A code that saves nothing says why, because the cashier is about to be
	// asked.
	fourth := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, fourth, "prd_bar", 1)
	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+fourth+"/code", token,
		map[string]any{"code": "WEEKEND10"})
	if status != http.StatusConflict || errorCode(refused) != "BELOW_MINIMUM" {
		t.Fatalf("want BELOW_MINIMUM, got %d: %v", status, refused)
	}
	status, unknown := h.request(http.MethodPost, "/api/admin/pos/orders/"+fourth+"/code", token,
		map[string]any{"code": "NONSENSE"})
	if status != http.StatusNotFound {
		t.Fatalf("an unknown code is a miss, got %d: %v", status, unknown)
	}
}

func TestAGiftCardIsSpentAtTheTillAndCannotBeSpentTwice(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	status, card := h.request(http.MethodPost, "/api/admin/pos/gift-cards", token, map[string]any{
		"code": "GIFT-100", "amountIdr": 100_000, "note": "Birthday",
	})
	if status != http.StatusCreated {
		t.Fatalf("issuing a card returned %d: %v", status, card)
	}
	// The opening balance is a movement, so the card's ledger explains its
	// balance from the first rupiah.
	entries := card["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["kind"] != "ISSUE" {
		t.Fatalf("the issue should be on the ledger, got %v", entries)
	}

	// Buy a shaker at 79.000 with it.
	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_shaker", 1)
	status, tendered := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender",
		token, map[string]any{"method": "GIFT_CARD", "amountIdr": 79_000, "reference": "GIFT-100"})
	if status != http.StatusCreated {
		t.Fatalf("paying by card returned %d: %v", status, tendered)
	}
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete", token, map[string]any{})

	status, after := h.request(http.MethodGet, "/api/admin/pos/gift-cards/GIFT-100", token, nil)
	if status != http.StatusOK || after["balanceIdr"].(float64) != 21_000 {
		t.Fatalf("100.000 less 79.000 is 21.000, got %v", after["balanceIdr"])
	}

	// It cannot pay for more than is on it.
	second := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, second, "prd_shaker", 1)
	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+second+"/tender",
		token, map[string]any{"method": "GIFT_CARD", "amountIdr": 79_000, "reference": "GIFT-100"})
	if status != http.StatusConflict || errorCode(refused) != "INSUFFICIENT_BALANCE" {
		t.Fatalf("want INSUFFICIENT_BALANCE, got %d: %v", status, refused)
	}

	// Freezing stops it, which is what freezing is for.
	h.request(http.MethodPut, "/api/admin/pos/gift-cards/GIFT-100/status", token,
		map[string]any{"status": "FROZEN"})
	status, frozen := h.request(http.MethodPost, "/api/admin/pos/orders/"+second+"/tender",
		token, map[string]any{"method": "GIFT_CARD", "amountIdr": 10_000, "reference": "GIFT-100"})
	if status != http.StatusConflict || errorCode(frozen) != "CARD_NOT_ACTIVE" {
		t.Fatalf("want CARD_NOT_ACTIVE, got %d: %v", status, frozen)
	}
}

func TestANamedCardNeedsItsOwnerOnTheSale(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	status, card := h.request(http.MethodPost, "/api/admin/pos/gift-cards", token, map[string]any{
		"code": "GIFT-NAMED", "amountIdr": 200_000, "memberId": demoMember,
	})
	if status != http.StatusCreated {
		t.Fatalf("issuing a named card returned %d: %v", status, card)
	}

	// Nobody named on the sale: the card belongs to somebody.
	anon := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, anon, "prd_bar", 1)
	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+anon+"/tender",
		token, map[string]any{"method": "GIFT_CARD", "amountIdr": 35_000, "reference": "GIFT-NAMED"})
	if status != http.StatusConflict || errorCode(refused) != "WRONG_MEMBER" {
		t.Fatalf("want WRONG_MEMBER, got %d: %v", status, refused)
	}

	// Name them and it works.
	status, named := h.request(http.MethodPost, "/api/admin/pos/orders", token,
		map[string]any{"branchId": senopati, "memberId": demoMember, "channel": "RETAIL"})
	if status != http.StatusCreated {
		t.Fatalf("opening a member sale returned %d: %v", status, named)
	}
	namedID := named["id"].(string)
	h.addLine(t, token, namedID, "prd_bar", 1)
	status, paid := h.request(http.MethodPost, "/api/admin/pos/orders/"+namedID+"/tender",
		token, map[string]any{"method": "GIFT_CARD", "amountIdr": 33_250, "reference": "GIFT-NAMED"})
	if status != http.StatusCreated {
		t.Fatalf("the owner may spend their own card, got %d: %v", status, paid)
	}
}

func TestVoidingWithoutThePermissionNeedsAManagerStandingThere(t *testing.T) {
	h := newHarness(t)
	manager := h.adminToken("adm_super")
	desk := h.adminToken("adm_desk")

	// Give the manager a PIN.
	status, set := h.request(http.MethodPut, "/api/admin/users/adm_branch/supervisor-pin",
		manager, map[string]any{"pin": "4417"})
	if status != http.StatusOK {
		t.Fatalf("setting a PIN returned %d: %v", status, set)
	}
	// A PIN on somebody who cannot authorise would be typed at a till and
	// silently refused, so it is refused here instead.
	status, pointless := h.request(http.MethodPut, "/api/admin/users/adm_coach/supervisor-pin",
		manager, map[string]any{"pin": "1234"})
	if status != http.StatusConflict || errorCode(pointless) != "CANNOT_AUTHORISE" {
		t.Fatalf("want CANNOT_AUTHORISE, got %d: %v", status, pointless)
	}

	// The front desk rings up and completes a sale.
	h.openTill(t, desk, 500_000)
	orderID := h.openSale(t, desk, "RETAIL")
	h.addLine(t, desk, orderID, "prd_bar", 1)
	h.settle(t, desk, orderID, 35_000)

	// They cannot void it alone.
	status, alone := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void", desk,
		map[string]any{"reason": "Rang up twice"})
	if status != http.StatusForbidden {
		t.Fatalf("the front desk cannot void unaided, got %d: %v", status, alone)
	}
	// Nor with the wrong PIN.
	status, wrong := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void", desk,
		map[string]any{"reason": "Rang up twice", "supervisorPin": "0000"})
	if status != http.StatusForbidden {
		t.Fatalf("a wrong PIN authorises nothing, got %d: %v", status, wrong)
	}

	// With a manager beside them, it goes through — and the manager is on it.
	status, voided := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void", desk,
		map[string]any{"reason": "Rang up twice", "supervisorPin": "4417"})
	if status != http.StatusOK || voided["status"] != "VOIDED" {
		t.Fatalf("voiding with a PIN returned %d: %v", status, voided)
	}
	if voided["authorisedBy"] == nil {
		t.Fatal("the manager who authorised it should be recorded")
	}

	// A manager needs no PIN at all: the permission already answers it.
	h.openTill(t, manager, 500_000)
	own := h.openSale(t, manager, "RETAIL")
	h.addLine(t, manager, own, "prd_bar", 1)
	h.settle(t, manager, own, 35_000)
	status, direct := h.request(http.MethodPost, "/api/admin/pos/orders/"+own+"/void", manager,
		map[string]any{"reason": "Customer changed their mind"})
	if status != http.StatusOK || direct["status"] != "VOIDED" {
		t.Fatalf("a manager voids outright, got %d: %v", status, direct)
	}
}

func TestAReceiptIsRenderedOnceAndPrintedFromTheQueue(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_bar", 2)
	h.settle(t, token, orderID, 70_000)

	status, preview := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID+"/receipt", token, nil)
	if status != http.StatusOK {
		t.Fatalf("rendering a receipt returned %d: %v", status, preview)
	}
	body := preview["body"].(string)
	for _, expected := range []string{"NUHABIT STUDIO", "Protein Bar", "TOTAL", "70.000"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("the receipt should mention %q:\n%s", expected, body)
		}
	}
	// 58mm paper is 32 columns; a line longer than the paper comes out cut.
	for _, l := range strings.Split(body, "\n") {
		if len([]rune(l)) > 32 {
			t.Fatalf("a %d-character line will not fit 58mm paper: %q", len([]rune(l)), l)
		}
	}

	status, job := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/print", token,
		map[string]any{})
	if status != http.StatusCreated || job["status"] != "QUEUED" {
		t.Fatalf("queueing a print returned %d: %v", status, job)
	}

	// The agent claims it, and a second agent gets nothing — two printers on
	// one counter must not print the same receipt twice.
	status, claimed := h.requestList(http.MethodPost,
		"/api/admin/pos/print-jobs/claim?branchId="+senopati, token, map[string]any{})
	if status != http.StatusOK || len(claimed) != 1 {
		t.Fatalf("one job to claim, got %d rows", len(claimed))
	}
	status, second := h.requestList(http.MethodPost,
		"/api/admin/pos/print-jobs/claim?branchId="+senopati, token, map[string]any{})
	if status != http.StatusOK || len(second) != 0 {
		t.Fatalf("a claimed job is not claimable again, got %d rows", len(second))
	}

	status, done := h.request(http.MethodPut,
		"/api/admin/pos/print-jobs/"+claimed[0]["id"].(string), token,
		map[string]any{"status": "PRINTED"})
	if status != http.StatusOK || done["status"] != "PRINTED" {
		t.Fatalf("finishing a job returned %d: %v", status, done)
	}

	// Sending to a phone is a job too, so a message that never went is
	// visible as one.
	status, send := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/send", token,
		map[string]any{"channel": "WHATSAPP", "destination": "+628123456789"})
	if status != http.StatusCreated || send["status"] != "QUEUED" {
		t.Fatalf("queueing a send returned %d: %v", status, send)
	}
}

func TestAPaymentMethodIsARowAndItsBehaviourIsARule(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, methods := h.requestList(http.MethodGet, "/api/admin/pos/payment-methods", token, nil)
	if status != http.StatusOK || len(methods) < 7 {
		t.Fatalf("the counter ships with its methods configured, got %d", len(methods))
	}

	// A shop signing up with a new QRIS provider adds a row, not a release.
	status, added := h.request(http.MethodPut, "/api/admin/pos/payment-methods", token,
		map[string]any{
			"code": "GOPAY", "name": "GoPay", "kind": "QR",
			"needsReference": true, "sortOrder": 8,
		})
	if status != http.StatusOK || added["code"] != "GOPAY" {
		t.Fatalf("adding a method returned %d: %v", status, added)
	}

	// Change comes out of the drawer, so a method that gives it has to be in
	// the drawer. That is a rule, and rules do not go in a form.
	status, wrong := h.request(http.MethodPut, "/api/admin/pos/payment-methods", token,
		map[string]any{
			"code": "IMPOSSIBLE", "name": "Impossible", "kind": "CARD",
			"givesChange": true, "countsInDrawer": false,
		})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a card that gives change must be refused, got %d: %v", status, wrong)
	}

	// The new method works at the till straight away.
	h.openTill(t, token, 500_000)
	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_bar", 1)
	status, paid := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
		map[string]any{"method": "GOPAY", "amountIdr": 35_000, "reference": "GP-1"})
	if status != http.StatusCreated {
		t.Fatalf("the new method should take a payment, got %d: %v", status, paid)
	}
}
