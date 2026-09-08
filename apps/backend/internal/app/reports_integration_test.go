package app_test

import (
	"net/http"
	"testing"
)

// The reports, over real HTTP.
//
// Aggregation is where reporting bugs hide, because a wrong total looks
// exactly like a right one until somebody checks it against reality. These
// check it against reality: they ring up known sales and receive known
// deliveries, then assert the reports say what actually happened.

func TestTheReportsAddUpToTheSalesThatWereRungUp(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	// Two sales: three bars at 35.000, and one whey at 749.000.
	first := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, first, "prd_bar", 3)
	h.settle(t, token, first, 105_000)

	second := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, second, "prd_whey", 1)
	h.settle(t, token, second, 749_000)

	status, lines := h.requestList(http.MethodGet,
		"/api/admin/pos/reports/product-sales?branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the product-sales report returned %d", status)
	}
	if len(lines) != 2 {
		t.Fatalf("two products sold, got %d", len(lines))
	}

	// Ranked by what each contributed, not by margin. Whey sells for 749.000
	// against a 480.000 cost — 269.000 — while three bars make 51.000.
	if lines[0]["productId"] != "prd_whey" {
		t.Fatalf("whey contributed most, got %v first", lines[0]["productId"])
	}
	if lines[0]["profitIdr"].(float64) != 269_000 {
		t.Fatalf("749.000 less 480.000 is 269.000, got %v", lines[0]["profitIdr"])
	}
	// Margin is against what it sold for: 269.000 of 749.000 is 35.91%.
	if lines[0]["marginPercent"].(float64) != 35.91 {
		t.Fatalf("the margin is against the sale price, got %v", lines[0]["marginPercent"])
	}

	// The composition adds to the takings, and its shares add to 100.
	status, composition := h.request(http.MethodGet,
		"/api/admin/pos/reports/revenue-composition?branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the composition report returned %d: %v", status, composition)
	}
	if composition["totalIdr"].(float64) != 854_000 {
		t.Fatalf("105.000 plus 749.000 is 854.000, got %v", composition["totalIdr"])
	}
	var share float64
	for _, bucket := range composition["byCategory"].([]any) {
		share += bucket.(map[string]any)["share"].(float64)
	}
	if share != 100 {
		t.Fatalf("shares should add to 100, got %v", share)
	}

	// Every hour appears in the rush-hour report, including the empty ones: a
	// chart with holes reads as missing data rather than a quiet afternoon.
	status, rush := h.request(http.MethodGet,
		"/api/admin/pos/reports/rush-hour?branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the rush-hour report returned %d: %v", status, rush)
	}
	if hours := rush["hours"].([]any); len(hours) != 24 {
		t.Fatalf("all 24 hours appear, got %d", len(hours))
	}
}

func TestAVoidedSaleLeavesTheTakingsAndAppearsInItsOwnReport(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_bar", 2)
	h.settle(t, token, orderID, 70_000)

	status, before := h.request(http.MethodGet,
		"/api/admin/pos/reports/revenue-composition?branchId="+senopati, token, nil)
	if status != http.StatusOK || before["totalIdr"].(float64) != 70_000 {
		t.Fatalf("the sale should be in the takings, got %v", before["totalIdr"])
	}

	status, voided := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void", token,
		map[string]any{"reason": "Rang up twice"})
	if status != http.StatusOK || voided["status"] != "VOIDED" {
		t.Fatalf("voiding returned %d: %v", status, voided)
	}

	// Money that came in and went out again is not takings. Counting it makes
	// a day look better than it was.
	status, after := h.request(http.MethodGet,
		"/api/admin/pos/reports/revenue-composition?branchId="+senopati, token, nil)
	if status != http.StatusOK || after["totalIdr"].(float64) != 0 {
		t.Fatalf("a voided sale leaves the takings, got %v", after["totalIdr"])
	}

	// It is visible on its own, which is the point of a voids report: the one
	// action that makes money disappear should be looked at together.
	status, voids := h.requestList(http.MethodGet,
		"/api/admin/pos/reports/voids?branchId="+senopati, token, nil)
	if status != http.StatusOK || len(voids) != 1 {
		t.Fatalf("one void expected, got %d with %d rows", status, len(voids))
	}
	if voids[0]["voidReason"] != "Rang up twice" {
		t.Fatalf("the reason travels with it, got %v", voids[0]["voidReason"])
	}
}

func TestTheCashUpNamesShortRatherThanLeavingASignedNumber(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	shiftID := h.openTill(t, token, 500_000)

	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_bar", 2)
	h.settle(t, token, orderID, 70_000)

	// The drawer should hold 570.000. It holds 560.000.
	status, closed := h.request(http.MethodPost, "/api/admin/pos/shifts/"+shiftID+"/close",
		token, map[string]any{"countedCashIdr": 560_000})
	if status != http.StatusOK {
		t.Fatalf("closing the till returned %d: %v", status, closed)
	}

	status, report := h.request(http.MethodGet,
		"/api/admin/pos/reports/closing/"+shiftID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the closing report returned %d: %v", status, report)
	}
	if report["expectedCashIdr"].(float64) != 570_000 {
		t.Fatalf("500.000 opening plus 70.000 cash, got %v", report["expectedCashIdr"])
	}
	if report["varianceIdr"].(float64) != -10_000 {
		t.Fatalf("the drawer is 10.000 light, got %v", report["varianceIdr"])
	}
	// Over and short are different conversations, so the report says which.
	if report["short"] != true {
		t.Fatalf("a light drawer is short, got %v", report["short"])
	}
}

func TestSupplierPerformanceRanksOnBehaviourNotOnPrice(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// One supplier short-ships; the seeded nutrition supplier delivers in
	// full. Both are cheap; only one is dependable.
	orderID, lineID := h.sentOrder(t, token, "itm_bar", 100, 16_500)
	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	receiptID := receipt["id"].(string)
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receiptID+"/lines", token,
		map[string]any{
			"orderItemId": lineID, "qtyAccepted": 60, "qtyRejected": 10,
			"batchNumber": "PB-PERF", "expiresOn": "2027-06-30",
		})
	h.request(http.MethodPost, "/api/admin/purchasing/receipts/"+receiptID+"/post", token,
		map[string]any{})

	status, performance := h.requestList(http.MethodGet,
		"/api/admin/purchasing/reports/suppliers", token, nil)
	if status != http.StatusOK || len(performance) == 0 {
		t.Fatalf("the supplier report returned %d with %d rows", status, len(performance))
	}
	row := performance[0]
	// Sixty of a hundred arrived and were kept.
	if row["fillRate"].(float64) != 60 {
		t.Fatalf("60 of 100 is a 60%% fill rate, got %v", row["fillRate"])
	}
	// Ten of the seventy delivered failed inspection — against what was
	// delivered, not against what was ordered.
	if row["rejectRate"].(float64) != 14.29 {
		t.Fatalf("10 of 70 delivered is 14.29%%, got %v", row["rejectRate"])
	}

	// And what it actually cost, from the receipt rather than the price list.
	status, history := h.requestList(http.MethodGet,
		"/api/admin/purchasing/reports/price-history?itemId=itm_bar", token, nil)
	if status != http.StatusOK || len(history) != 1 {
		t.Fatalf("one receipt in the price history, got %d with %d rows", status, len(history))
	}
	if history[0]["unitPriceIdr"].(float64) != 16_500 {
		t.Fatalf("received at 16.500 a piece, got %v", history[0]["unitPriceIdr"])
	}
}

func TestValuationAndTheStockCardAgreeWithTheLedger(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, report := h.request(http.MethodGet,
		"/api/admin/inventory/reports/valuation?branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the valuation returned %d: %v", status, report)
	}

	// The total is the sum of the lines, and the lines are sorted with the
	// capital at the top.
	var summed float64
	lines := report["lines"].([]any)
	for _, line := range lines {
		summed += line.(map[string]any)["valueIdr"].(float64)
	}
	if round2(summed) != report["totalIdr"].(float64) {
		t.Fatalf("the total is the sum of its lines: %v vs %v", summed, report["totalIdr"])
	}
	first := lines[0].(map[string]any)["valueIdr"].(float64)
	last := lines[len(lines)-1].(map[string]any)["valueIdr"].(float64)
	if first < last {
		t.Fatalf("the most valuable line comes first: %v then %v", first, last)
	}

	// A sale writes a movement, and the stock card's balance is the ledger's
	// own qty_after rather than a second sum that could disagree with it.
	h.openTill(t, token, 500_000)
	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_bar", 4)
	h.settle(t, token, orderID, 140_000)

	status, card := h.requestList(http.MethodGet,
		"/api/admin/inventory/reports/stock-card?itemId=itm_bar&branchId="+senopati, token, nil)
	if status != http.StatusOK || len(card) < 2 {
		t.Fatalf("the stock card returned %d with %d rows", status, len(card))
	}
	last4 := card[len(card)-1]
	if last4["out"].(float64) != 4 {
		t.Fatalf("four left the shelf, got %v", last4["out"])
	}
	onHand := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64)
	if last4["balance"].(float64) != onHand {
		t.Fatalf("the card's last balance is the level: %v vs %v", last4["balance"], onHand)
	}
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
