package app_test

import (
	"net/http"
	"testing"
)

// Buying by the carton and selling by the piece.
//
// This is the one arithmetic an FMCG counter cannot get wrong, and the classic
// way it goes wrong is that two places each do the multiplication: a purchase
// order for ten cartons is received as ten pieces, and from then on the stock
// report is fiction. These tests drive the whole path over HTTP and check the
// quantity and the money at each end of it.

// openSale starts a sale on the given channel and returns its id.
func (h *harness) openSale(t *testing.T, token, channel string) string {
	t.Helper()
	status, order := h.request(http.MethodPost, "/api/admin/pos/orders", token,
		map[string]any{"branchId": senopati, "channel": channel})
	if status != http.StatusCreated {
		t.Fatalf("opening a %s sale returned %d: %v", channel, status, order)
	}
	return order["id"].(string)
}

func (h *harness) addLine(t *testing.T, token, orderID, productID string, qty float64) map[string]any {
	t.Helper()
	status, withLine := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/lines",
		token, map[string]any{"productId": productID, "qty": qty})
	if status != http.StatusCreated {
		t.Fatalf("adding %v of %s returned %d: %v", qty, productID, status, withLine)
	}
	return withLine
}

func (h *harness) settle(t *testing.T, token, orderID string, amount float64) {
	t.Helper()
	status, tendered := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender",
		token, map[string]any{"method": "CASH", "amountIdr": amount})
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("tendering returned %d: %v", status, tendered)
	}
	status, completed := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete",
		token, map[string]any{})
	if status != http.StatusOK || completed["status"] != "COMPLETED" {
		t.Fatalf("completing returned %d: %v", status, completed)
	}
}

func TestOrderingInCartonsReceivesInPieces(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	before := h.stockOf(t, token, "itm_bar", senopati)["qtyOnHand"].(float64)

	// Ten cartons at the carton price. Nobody types 240, and nobody types
	// 16.500 — both are the system's job.
	status, order := h.request(http.MethodPost, "/api/admin/purchasing/orders", token, map[string]any{
		"supplierId": "sup_nutrition", "branchId": senopati, "taxPercent": 11,
	})
	if status != http.StatusCreated {
		t.Fatalf("creating an order returned %d: %v", status, order)
	}
	orderID := order["id"].(string)

	status, withLine := h.request(http.MethodPost,
		"/api/admin/purchasing/orders/"+orderID+"/lines", token, map[string]any{
			"itemId": "itm_bar", "description": "Protein bars", "qty": 10,
			"unit": "CTN", "unitPriceIdr": 396_000,
		})
	if status != http.StatusCreated {
		t.Fatalf("adding a carton line returned %d: %v", status, withLine)
	}
	line := withLine["items"].([]any)[0].(map[string]any)
	lineID := line["id"].(string)

	if line["packFactor"].(float64) != 24 {
		t.Fatalf("the line should carry the carton's factor, got %v", line["packFactor"])
	}
	// The order is worth ten cartons, not ten pieces: 3.960.000 plus 11%.
	if got := withLine["subtotalIdr"].(float64); got != 3_960_000 {
		t.Fatalf("ten cartons at 396.000 is 3.960.000, got %v", got)
	}

	h.request(http.MethodPost, "/api/admin/purchasing/orders/"+orderID+"/approve", token, map[string]any{})
	h.request(http.MethodPost, "/api/admin/purchasing/orders/"+orderID+"/send", token, map[string]any{})

	// On-order is a stock figure, so it is in pieces. Reserving ten would let
	// the reorder screen raise a second order for stock already coming.
	if onOrder := h.stockOf(t, token, "itm_bar", senopati)["qtyOnOrder"].(float64); onOrder != 240 {
		t.Fatalf("ten cartons on order is 240 pieces, got %v", onOrder)
	}

	status, receipt := h.request(http.MethodPost, "/api/admin/purchasing/receipts", token,
		map[string]any{"orderId": orderID, "deliveryNoteNumber": "SJ-CTN-1"})
	if status != http.StatusCreated {
		t.Fatalf("opening a receipt returned %d: %v", status, receipt)
	}
	receiptID := receipt["id"].(string)

	// Eight cartons turn up. The delivery note says cartons, so that is what
	// the receiving clerk types.
	status, received := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/lines", token, map[string]any{
			"orderItemId": lineID, "qtyAccepted": 8,
		})
	if status != http.StatusCreated {
		t.Fatalf("receiving cartons returned %d: %v", status, received)
	}
	if got := received["items"].([]any)[0].(map[string]any)["unit"]; got != "CTN" {
		t.Fatalf("the receipt line inherits the ordered pack, got %v", got)
	}

	status, posted := h.request(http.MethodPost,
		"/api/admin/purchasing/receipts/"+receiptID+"/post", token, map[string]any{})
	if status != http.StatusOK || posted["status"] != "POSTED" {
		t.Fatalf("posting returned %d: %v", status, posted)
	}

	after := h.stockOf(t, token, "itm_bar", senopati)
	if got := after["qtyOnHand"].(float64); got != before+192 {
		t.Fatalf("eight cartons of 24 is 192 pieces, want %v got %v", before+192, got)
	}
	// Two cartons are still owed — 48 pieces, not 2.
	if got := after["qtyOnOrder"].(float64); got != 48 {
		t.Fatalf("two cartons still owed is 48 pieces, got %v", got)
	}

	// The seed holds bars at 18.000; a carton at 396.000 is 16.500 a piece, so
	// the average must land between the two. Booking the carton price in
	// undivided would have sent it to nearly 400.000.
	status, item := h.request(http.MethodGet, "/api/admin/inventory/items/itm_bar", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the item returned %d", status)
	}
	cost := item["item"].(map[string]any)["unitCostIdr"].(float64)
	if cost <= 16_500 || cost >= 18_000 {
		t.Fatalf("a receipt at 16.500 against stock at 18.000 lands between them, got %v", cost)
	}

	// And the ledger explains itself in the unit the goods arrived in.
	status, rows := h.requestList(http.MethodGet,
		"/api/admin/inventory/movements?itemId=itm_bar&referenceType=GOODS_RECEIPT", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading movements returned %d", status)
	}
	if len(rows) == 0 {
		t.Fatal("the receipt should have written a movement")
	}
	row := rows[0]
	if row["qty"].(float64) != 192 || row["packQty"].(float64) != 8 || row["packUnit"] != "CTN" {
		t.Fatalf("the movement should read 192 pieces as 8 CTN, got %v", row)
	}
}

func TestTheTillSellsTheSameItemBySingleAndByCarton(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	before := h.stockOf(t, token, "itm_iso", senopati)["qtyOnHand"].(float64)
	h.openTill(t, token, 500_000)
	orderID := h.openSale(t, token, "RETAIL")

	// A single, scanned by its own barcode.
	status, scanned := h.request(http.MethodGet,
		"/api/admin/pos/scan?barcode=8991234500028&branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("scanning a single returned %d: %v", status, scanned)
	}
	if scanned["id"] != "prd_iso" || scanned["packFactor"].(float64) != 1 {
		t.Fatalf("that barcode is one bottle, got %v", scanned)
	}

	// The carton's barcode is a different product of the same item.
	status, carton := h.request(http.MethodGet,
		"/api/admin/pos/scan?barcode=18991234500025&branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("scanning a carton returned %d: %v", status, carton)
	}
	if carton["id"] != "prd_iso_ctn" || carton["packFactor"].(float64) != 24 {
		t.Fatalf("that barcode is a carton of 24, got %v", carton)
	}
	// Stock is reported in the pack that was scanned, so a carton row says
	// cartons. Five bottles on the shelf is not one carton.
	if carton["onHand"].(float64) != before/24 {
		t.Fatalf("on hand in cartons is %v, got %v", before/24, carton["onHand"])
	}

	h.addLine(t, token, orderID, "prd_iso", 2)
	h.addLine(t, token, orderID, "prd_iso_ctn", 1)

	status, sale := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d: %v", status, sale)
	}
	// Two singles at 25.000 and a carton at 552.000.
	if got := sale["subtotalIdr"].(float64); got != 602_000 {
		t.Fatalf("two singles and a carton is 602.000, got %v", got)
	}

	h.settle(t, token, orderID, 602_000)

	// Twenty-six bottles left the shelf, not three items.
	after := h.stockOf(t, token, "itm_iso", senopati)["qtyOnHand"].(float64)
	if after != before-26 {
		t.Fatalf("two singles and a carton is 26 bottles, want %v got %v", before-26, after)
	}
}

func TestAResellerPaysTheWholesalePriceAndAVolumeBuyerTheBreak(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	h.openTill(t, token, 500_000)

	// A dozen bars over the retail counter reaches the volume break: twelve at
	// 31.000 rather than twelve at 35.000.
	retail := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, retail, "prd_bar", 12)
	status, sale := h.request(http.MethodGet, "/api/admin/pos/orders/"+retail, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d: %v", status, sale)
	}
	if got := sale["subtotalIdr"].(float64); got != 372_000 {
		t.Fatalf("a dozen at the 31.000 break is 372.000, got %v", got)
	}
	h.settle(t, token, retail, 372_000)

	// The same dozen on a wholesale ticket is cheaper again, from the first
	// unit: the channel is the price list, not a discount somebody remembered.
	wholesale := h.openSale(t, token, "WHOLESALE")
	h.addLine(t, token, wholesale, "prd_bar", 12)
	status, bulk := h.request(http.MethodGet, "/api/admin/pos/orders/"+wholesale, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the wholesale sale returned %d: %v", status, bulk)
	}
	if got := bulk["subtotalIdr"].(float64); got != 330_000 {
		t.Fatalf("a dozen at the wholesale 27.500 is 330.000, got %v", got)
	}
}

func TestAnItemCannotBeOrderedInAPackItHasNot(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, order := h.request(http.MethodPost, "/api/admin/purchasing/orders", token, map[string]any{
		"supplierId": "sup_nutrition", "branchId": senopati,
	})
	if status != http.StatusCreated {
		t.Fatalf("creating an order returned %d: %v", status, order)
	}

	// Whey comes in pieces and boxes of six. A pallet is not a unit it has,
	// and guessing a factor of one here is exactly how ten pallets arrive as
	// ten tubs.
	status, refused := h.request(http.MethodPost,
		"/api/admin/purchasing/orders/"+order["id"].(string)+"/lines", token, map[string]any{
			"itemId": "itm_whey", "description": "Whey", "qty": 10,
			"unit": "PALLET", "unitPriceIdr": 1_000_000,
		})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an undefined pack must be refused, got %d: %v", status, refused)
	}
}
