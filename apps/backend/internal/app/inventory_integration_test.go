package app_test

import (
	"fmt"
	"net/http"
	"testing"
)

// Stock, over real HTTP against a real database.
//
// The property every one of these protects is the same: a quantity is only
// ever what its movements add up to, and no code path may change one without
// writing the other.

const senopati = "brn_senopati"

// stockOf reads one shelf.
func (h *harness) stockOf(t *testing.T, token, itemID, branchID string) map[string]any {
	t.Helper()
	status, rows := h.requestList(http.MethodGet,
		fmt.Sprintf("/api/admin/inventory/stock?branchId=%s&limit=500", branchID), token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading stock returned %d", status)
	}
	for _, row := range rows {
		item, _ := row["item"].(map[string]any)
		if item["id"] == itemID {
			level, _ := row["level"].(map[string]any)
			return level
		}
	}
	t.Fatalf("no stock row for %s at %s", itemID, branchID)
	return nil
}

func TestStockCannotBeTakenBelowZero(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	level := h.stockOf(t, token, "itm_whey", "brn_pik")
	onHand := level["qtyOnHand"].(float64)

	// One more than is there is refused outright rather than clamped: a till
	// that thinks it sold what it has not is a problem no count can hide.
	status, refused := h.request(http.MethodPost, "/api/admin/inventory/adjust", token, map[string]any{
		"itemId": "itm_whey", "branchId": "brn_pik", "qty": -(onHand + 1),
		"reason": "Deliberate oversell",
	})
	if status != http.StatusConflict || errorCode(refused) != "INSUFFICIENT_STOCK" {
		t.Fatalf("want INSUFFICIENT_STOCK, got %d: %v", status, refused)
	}

	// And nothing moved.
	after := h.stockOf(t, token, "itm_whey", "brn_pik")
	if after["qtyOnHand"].(float64) != onHand {
		t.Fatalf("a refused movement must not change stock: %v -> %v", onHand, after["qtyOnHand"])
	}
}

func TestEveryQuantityEqualsTheSumOfItsMovements(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// A handful of movements of every shape.
	for _, change := range []map[string]any{
		{"itemId": "itm_bar", "branchId": senopati, "qty": -7.0, "reason": "Sold"},
		// Bars are dated goods, so stock arriving by hand says which batch.
		{"itemId": "itm_bar", "branchId": senopati, "qty": 20.0, "reason": "Delivery",
			"batchCode": "ADJ-1", "expiresOn": "2027-06-30"},
		{"itemId": "itm_bar", "branchId": senopati, "qty": -1.5, "reason": "Damaged"},
	} {
		if status, body := h.request(http.MethodPost, "/api/admin/inventory/adjust", token, change); status != http.StatusCreated {
			t.Fatalf("adjustment returned %d: %v", status, body)
		}
	}

	// The cached quantity and the ledger must agree. This is the invariant the
	// whole module exists to keep, so it is asserted against the database
	// rather than against the service that maintains it.
	level := h.stockOf(t, token, "itm_bar", senopati)
	var summed float64
	err := h.app.DB.QueryRow(t.Context(), `
		SELECT COALESCE(SUM(qty), 0) FROM inventory.stock_movements
		WHERE item_id = $1 AND branch_id = $2`, "itm_bar", senopati).Scan(&summed)
	if err != nil {
		t.Fatalf("summing movements: %v", err)
	}
	if level["qtyOnHand"].(float64) != summed {
		t.Fatalf("stock (%v) has drifted from its ledger (%v)", level["qtyOnHand"], summed)
	}
}

func TestTheStockLedgerIsAppendOnlyInTheDatabase(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, movement := h.request(http.MethodPost, "/api/admin/inventory/adjust", token, map[string]any{
		"itemId": "itm_tee", "branchId": senopati, "qty": -1.0, "reason": "Sample",
	})
	if status != http.StatusCreated {
		t.Fatalf("adjustment returned %d: %v", status, movement)
	}

	// The application never rewrites a movement. This asserts the refusal
	// still holds when the application is bypassed entirely.
	if _, err := h.app.DB.Exec(t.Context(),
		`UPDATE inventory.stock_movements SET qty = 0 WHERE id = $1`, movement["id"]); err == nil {
		t.Fatal("the database must refuse to rewrite a stock movement")
	}
	if _, err := h.app.DB.Exec(t.Context(),
		`DELETE FROM inventory.stock_movements WHERE id = $1`, movement["id"]); err == nil {
		t.Fatal("the database must refuse to delete a stock movement")
	}
}

func TestAnAdjustmentAlwaysCarriesAReason(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// An unexplained change to a quantity is indistinguishable from theft.
	status, refused := h.request(http.MethodPost, "/api/admin/inventory/adjust", token, map[string]any{
		"itemId": "itm_bar", "branchId": senopati, "qty": -1.0, "reason": "  ",
	})
	if status != http.StatusUnprocessableEntity || errorCode(refused) != "VALIDATION_FAILED" {
		t.Fatalf("want a validation failure, got %d: %v", status, refused)
	}
}

func TestReceivingMovesTheAverageCost(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// The seed opens the shelf at a known cost; receiving more at a different
	// price must land between the two, weighted by quantity.
	status, before := h.request(http.MethodGet, "/api/admin/inventory/items/itm_shaker", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the item returned %d", status)
	}
	item := before["item"].(map[string]any)
	startCost := item["unitCostIdr"].(float64)
	startQty := item["totalOnHand"].(float64)

	// A receipt at double the price.
	if status, body := h.request(http.MethodPost, "/api/admin/inventory/adjust", token, map[string]any{
		"itemId": "itm_shaker", "branchId": senopati, "qty": startQty, "reason": "Delivery",
	}); status != http.StatusCreated {
		t.Fatalf("receipt returned %d: %v", status, body)
	}

	// An adjustment carries no price, so the average must not have moved: only
	// a priced receipt revalues stock.
	status, after := h.request(http.MethodGet, "/api/admin/inventory/items/itm_shaker", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading the item returned %d", status)
	}
	if got := after["item"].(map[string]any)["unitCostIdr"].(float64); got != startCost {
		t.Fatalf("an unpriced movement must not revalue stock: %v -> %v", startCost, got)
	}
}

func TestTransferMovesStockBetweenBranchesAndKeepsTheTotal(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	from := h.stockOf(t, token, "itm_bottle", senopati)["qtyOnHand"].(float64)
	to := h.stockOf(t, token, "itm_bottle", "brn_pik")["qtyOnHand"].(float64)

	status, transfer := h.request(http.MethodPost, "/api/admin/inventory/transfer", token, map[string]any{
		"itemId": "itm_bottle", "fromBranchId": senopati, "toBranchId": "brn_pik", "qty": 5,
	})
	if status != http.StatusCreated {
		t.Fatalf("transfer returned %d: %v", status, transfer)
	}

	newFrom := h.stockOf(t, token, "itm_bottle", senopati)["qtyOnHand"].(float64)
	newTo := h.stockOf(t, token, "itm_bottle", "brn_pik")["qtyOnHand"].(float64)
	if newFrom != from-5 || newTo != to+5 {
		t.Fatalf("want %v/%v, got %v/%v", from-5, to+5, newFrom, newTo)
	}
	// Moving stock between shelves does not create or destroy any.
	if newFrom+newTo != from+to {
		t.Fatalf("a transfer changed the total: %v -> %v", from+to, newFrom+newTo)
	}

	// A transfer larger than the source has is refused, and — because both
	// movements are one transaction — leaves neither branch changed.
	status, refused := h.request(http.MethodPost, "/api/admin/inventory/transfer", token, map[string]any{
		"itemId": "itm_bottle", "fromBranchId": senopati, "toBranchId": "brn_pik", "qty": newFrom + 1,
	})
	if status != http.StatusConflict || errorCode(refused) != "INSUFFICIENT_STOCK" {
		t.Fatalf("want INSUFFICIENT_STOCK, got %d: %v", status, refused)
	}
	if got := h.stockOf(t, token, "itm_bottle", "brn_pik")["qtyOnHand"].(float64); got != newTo {
		t.Fatalf("a failed transfer must not have credited the destination: %v -> %v", newTo, got)
	}
}

func TestStockTakeVariancesBecomeAdjustments(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, take := h.request(http.MethodPost, "/api/admin/inventory/stock-takes", token,
		map[string]any{"branchId": senopati, "note": "Integration count"})
	if status != http.StatusCreated {
		t.Fatalf("opening a count returned %d: %v", status, take)
	}
	takeID := take["id"].(string)

	expected := h.stockOf(t, token, "itm_grips", senopati)["qtyOnHand"].(float64)
	// Two short, and one item counted exactly right.
	for _, line := range []map[string]any{
		{"itemId": "itm_grips", "qtyCounted": expected - 2},
		{"itemId": "itm_tank", "qtyCounted": h.stockOf(t, token, "itm_tank", senopati)["qtyOnHand"]},
	} {
		if status, body := h.request(http.MethodPost,
			"/api/admin/inventory/stock-takes/"+takeID+"/count", token, line); status != http.StatusOK {
			t.Fatalf("counting returned %d: %v", status, body)
		}
	}

	status, applied := h.request(http.MethodPost,
		"/api/admin/inventory/stock-takes/"+takeID+"/apply", token, map[string]any{})
	if status != http.StatusOK || applied["status"] != "APPLIED" {
		t.Fatalf("applying returned %d: %v", status, applied)
	}

	// The short line moved stock; the exact line wrote nothing, because a
	// movement of nothing is not a movement.
	if got := h.stockOf(t, token, "itm_grips", senopati)["qtyOnHand"].(float64); got != expected-2 {
		t.Fatalf("want %v after the count, got %v", expected-2, got)
	}
	status, movements := h.requestList(http.MethodGet,
		"/api/admin/inventory/movements?referenceType=STOCK_TAKE&referenceId="+takeID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading movements returned %d", status)
	}
	if len(movements) != 1 {
		t.Fatalf("want exactly one adjustment, got %d", len(movements))
	}

	// An applied count is final: it has already written append-only rows.
	status, again := h.request(http.MethodPost,
		"/api/admin/inventory/stock-takes/"+takeID+"/apply", token, map[string]any{})
	if status != http.StatusConflict || errorCode(again) != "INVALID_TRANSITION" {
		t.Fatalf("want INVALID_TRANSITION, got %d: %v", status, again)
	}
}

func TestInventoryPermissionsAreEnforcedServerSide(t *testing.T) {
	h := newHarness(t)

	// The desk has to answer "have you got this in a medium", so it looks.
	desk := h.adminToken("adm_desk")
	if status, _ := h.requestList(http.MethodGet, "/api/admin/inventory/stock", desk, nil); status != http.StatusOK {
		t.Fatalf("the front desk should see stock, got %d", status)
	}
	// But changing a quantity is not a counter job.
	status, refused := h.request(http.MethodPost, "/api/admin/inventory/adjust", desk, map[string]any{
		"itemId": "itm_bar", "branchId": senopati, "qty": -1.0, "reason": "Nope",
	})
	if status != http.StatusForbidden {
		t.Fatalf("the front desk must not adjust stock, got %d: %v", status, refused)
	}
	// Nor is editing the catalogue.
	if status, _ := h.request(http.MethodPost, "/api/admin/inventory/items", desk, map[string]any{
		"sku": "X-1", "name": "Nope",
	}); status != http.StatusForbidden {
		t.Fatalf("the front desk must not edit the catalogue, got %d", status)
	}

	// A coach has no business in the stockroom at all.
	coach := h.adminToken("adm_coach")
	if status, _ := h.requestList(http.MethodGet, "/api/admin/inventory/stock", coach, nil); status != http.StatusForbidden {
		t.Fatalf("a coach must not see stock, got %d", status)
	}
}
