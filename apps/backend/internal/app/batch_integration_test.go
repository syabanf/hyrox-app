package app_test

import (
	"net/http"
	"testing"
)

// Dated stock, over real HTTP.
//
// The property that matters: a sale takes the short-dated case first, and
// nothing sells stock that is past its date. Getting the order wrong is not an
// error anybody notices until the goods are worthless.

func TestASaleTakesTheShortDatedBatchFirst(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	// The seed opens every dated item in two batches: a short one and a long
	// one. The short one has to empty first.
	status, batches := h.requestList(http.MethodGet,
		"/api/admin/inventory/batches?itemId=itm_bar&branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading batches returned %d", status)
	}
	if len(batches) < 2 {
		t.Fatalf("the seed should open two batches, got %d", len(batches))
	}
	// The list comes back in FEFO order, so the front of it is what should go.
	front := batches[0]
	frontQty := front["qtyOnHand"].(float64)
	if front["state"] != "NEAR" {
		t.Fatalf("the front of the shelf should be the near-dated batch, got %v", front["state"])
	}

	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_bar", 5)
	status, sale := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d: %v", status, sale)
	}
	h.settle(t, token, orderID, sale["totalIdr"].(float64))

	status, after := h.requestList(http.MethodGet,
		"/api/admin/inventory/batches?itemId=itm_bar&branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("re-reading batches returned %d", status)
	}
	for _, row := range after {
		if row["id"] != front["id"] {
			continue
		}
		if got := row["qtyOnHand"].(float64); got != frontQty-5 {
			t.Fatalf("the sale should have come off the short-dated batch: %v -> %v", frontQty, got)
		}
		return
	}
	t.Fatal("the short-dated batch disappeared")
}

func TestVoidingASaleReturnsStockToTheBatchItLeft(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	status, before := h.requestList(http.MethodGet,
		"/api/admin/inventory/batches?itemId=itm_iso&branchId="+senopati, token, nil)
	if status != http.StatusOK || len(before) == 0 {
		t.Fatalf("reading batches returned %d with %d rows", status, len(before))
	}
	front := before[0]
	frontQty := front["qtyOnHand"].(float64)

	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_iso", 4)
	status, sale := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d: %v", status, sale)
	}
	h.settle(t, token, orderID, sale["totalIdr"].(float64))

	status, voided := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/void", token,
		map[string]any{"reason": "Customer changed their mind"})
	if status != http.StatusOK || voided["status"] != "VOIDED" {
		t.Fatalf("voiding returned %d: %v", status, voided)
	}

	// The goods go back where they came from. Inventing a new batch would give
	// returned stock a fresh expiry date it has not earned, and asking the
	// cashier for a batch code would be asking them to guess.
	status, after := h.requestList(http.MethodGet,
		"/api/admin/inventory/batches?itemId=itm_iso&branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("re-reading batches returned %d", status)
	}
	if len(after) != len(before) {
		t.Fatalf("a void must not create a batch: %d -> %d", len(before), len(after))
	}
	for _, row := range after {
		if row["id"] != front["id"] {
			continue
		}
		if got := row["qtyOnHand"].(float64); got != frontQty {
			t.Fatalf("the void should have restored the batch to %v, got %v", frontQty, got)
		}
		return
	}
	t.Fatal("the original batch disappeared")
}

func TestGoodsWithADateCannotArriveWithoutOne(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// A positive adjustment on dated stock is a physical pile with a date on
	// it. The system has no way to guess which, so it refuses rather than
	// inventing one.
	status, refused := h.request(http.MethodPost, "/api/admin/inventory/adjust", token,
		map[string]any{
			"itemId": "itm_whey", "branchId": senopati, "qty": 5.0, "reason": "Found in the back",
		})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("dated stock arriving without a batch must be refused, got %d: %v", status, refused)
	}

	// With one, it is ordinary.
	status, accepted := h.request(http.MethodPost, "/api/admin/inventory/adjust", token,
		map[string]any{
			"itemId": "itm_whey", "branchId": senopati, "qty": 5.0, "reason": "Found in the back",
			"batchCode": "FOUND-1", "expiresOn": "2027-12-31",
		})
	if status != http.StatusCreated {
		t.Fatalf("a batched adjustment should be accepted, got %d: %v", status, accepted)
	}

	// Undated goods are unaffected: demanding a batch for a steel bottle would
	// make every delivery note a form nobody can fill in.
	status, bottle := h.request(http.MethodPost, "/api/admin/inventory/adjust", token,
		map[string]any{
			"itemId": "itm_bottle", "branchId": senopati, "qty": 3.0, "reason": "Found in the back",
		})
	if status != http.StatusCreated {
		t.Fatalf("undated stock needs no batch, got %d: %v", status, bottle)
	}
}

func TestExpiredStockCannotBeSoldAndIsWrittenOffWithItsCost(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	h.openTill(t, token, 500_000)

	// Age the near-dated batch past its date. This is the state a shop wakes
	// up to on a Monday.
	if _, err := h.app.DB.Exec(t.Context(),
		`UPDATE inventory.batches SET expires_on = CURRENT_DATE - 1
		 WHERE item_id = 'itm_whey' AND branch_id = $1 AND batch_code LIKE '%_A'`, senopati); err != nil {
		t.Fatalf("ageing the batch: %v", err)
	}

	status, batches := h.requestList(http.MethodGet,
		"/api/admin/inventory/batches?itemId=itm_whey&branchId="+senopati+"&state=EXPIRED", token, nil)
	if status != http.StatusOK || len(batches) != 1 {
		t.Fatalf("one expired batch expected, got %d with %d rows", status, len(batches))
	}
	expired := batches[0]
	expiredQty := expired["qtyOnHand"].(float64)

	// It shows up as money about to be lost, not just as a count.
	status, report := h.request(http.MethodGet, "/api/admin/inventory/expiry?branchId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the expiry report returned %d: %v", status, report)
	}
	summary := report["summary"].(map[string]any)
	if summary["expiredValueIdr"].(float64) <= 0 {
		t.Fatalf("expired stock is worth something, got %v", summary["expiredValueIdr"])
	}

	// A sale big enough to need the expired batch is refused rather than
	// quietly reaching past its date. The stock level still counts the expired
	// goods — they are physically on the shelf — so the level check passes and
	// it is FEFO that refuses, which is exactly the seam being tested.
	onHand := h.stockOf(t, token, "itm_whey", senopati)["qtyOnHand"].(float64)
	orderID := h.openSale(t, token, "RETAIL")
	h.addLine(t, token, orderID, "prd_whey", onHand)
	status, sale := h.request(http.MethodGet, "/api/admin/pos/orders/"+orderID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the sale returned %d: %v", status, sale)
	}
	h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/tender", token,
		map[string]any{"method": "CASH", "amountIdr": sale["totalIdr"].(float64)})
	status, refused := h.request(http.MethodPost, "/api/admin/pos/orders/"+orderID+"/complete",
		token, map[string]any{})
	if status != http.StatusConflict || errorCode(refused) != "EXPIRED_STOCK" {
		t.Fatalf("want EXPIRED_STOCK, got %d: %v", status, refused)
	}

	// Writing it off is an ordinary adjustment carrying a reason, so a stock
	// report a month later can still say what happened.
	before := h.stockOf(t, token, "itm_whey", senopati)["qtyOnHand"].(float64)
	status, written := h.request(http.MethodPost,
		"/api/admin/inventory/batches/"+expired["id"].(string)+"/write-off", token, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("writing off returned %d: %v", status, written)
	}
	if got := h.stockOf(t, token, "itm_whey", senopati)["qtyOnHand"].(float64); got != before-expiredQty {
		t.Fatalf("the write-off should take %v off the level, got %v", expiredQty, before-got)
	}
	// And good stock cannot be written off by calling it expired.
	status, good := h.requestList(http.MethodGet,
		"/api/admin/inventory/batches?itemId=itm_whey&branchId="+senopati, token, nil)
	if status != http.StatusOK || len(good) == 0 {
		t.Fatalf("the long-dated batch should remain, got %d rows", len(good))
	}
	status, notExpired := h.request(http.MethodPost,
		"/api/admin/inventory/batches/"+good[0]["id"].(string)+"/write-off", token, map[string]any{})
	if status != http.StatusConflict || errorCode(notExpired) != "NOT_EXPIRED" {
		t.Fatalf("want NOT_EXPIRED, got %d: %v", status, notExpired)
	}
}
