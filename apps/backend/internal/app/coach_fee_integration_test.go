package app_test

import (
	"net/http"
	"testing"
)

// A coach's own scheme, with a rate for one class type, is what they get paid.
func TestCoachRatesOverrideTheSchemeForOneClassType(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, scheme := h.request(http.MethodPost, "/api/admin/incentives/schemes", token, map[string]any{
		"coachId":                   "coa_kevin",
		"sessionFeeIdr":             150_000,
		"perAttendeeIdr":            10_000,
		"fullClassBonusIdr":         0,
		"fullClassThresholdPercent": 80,
		"noShowPenaltyIdr":          0,
		"rates": []map[string]any{
			{"classTypeId": "clt_simulation", "sessionFeeIdr": 400_000, "perAttendeeIdr": 20_000},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("creating the scheme returned %d: %v", status, scheme)
	}
	rates := scheme["scheme"].(map[string]any)["rates"].([]any)
	if len(rates) != 1 {
		t.Fatalf("the scheme came back with %d rates, want 1", len(rates))
	}

	// The list shows it, with the class type named so an editor can render it.
	status, schemes := h.requestList(http.MethodGet, "/api/admin/incentives/schemes", token, nil)
	if status != http.StatusOK {
		t.Fatalf("listing schemes returned %d", status)
	}
	found := false
	for _, row := range schemes {
		s := row["scheme"].(map[string]any)
		if s["coachId"] == "coa_kevin" {
			found = true
			if len(s["rates"].([]any)) != 1 {
				t.Fatalf("the coach's rates did not come back: %v", s["rates"])
			}
			names := row["classTypeNames"].(map[string]any)
			if names["clt_simulation"] == nil {
				t.Fatal("the class type the rate refers to is not named")
			}
		}
	}
	if !found {
		t.Fatal("the coach's scheme is missing from the list")
	}
}

// Editing a scheme replaces its rates: one somebody deleted has to go.
func TestSavingASchemeReplacesItsRates(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	_, created := h.request(http.MethodPost, "/api/admin/incentives/schemes", token, map[string]any{
		"coachId": "coa_maya", "sessionFeeIdr": 150_000, "perAttendeeIdr": 10_000,
		"fullClassBonusIdr": 0, "fullClassThresholdPercent": 80, "noShowPenaltyIdr": 0,
		"rates": []map[string]any{
			{"classTypeId": "clt_simulation", "sessionFeeIdr": 400_000, "perAttendeeIdr": 20_000},
			{"classTypeId": "clt_engine", "sessionFeeIdr": 200_000, "perAttendeeIdr": 12_000},
		},
	})
	schemeID := created["scheme"].(map[string]any)["id"].(string)

	status, updated := h.request(http.MethodPut, "/api/admin/incentives/schemes/"+schemeID, token, map[string]any{
		"coachId": "coa_maya", "sessionFeeIdr": 150_000, "perAttendeeIdr": 10_000,
		"fullClassBonusIdr": 0, "fullClassThresholdPercent": 80, "noShowPenaltyIdr": 0,
		"active": true,
		"rates": []map[string]any{
			{"classTypeId": "clt_simulation", "sessionFeeIdr": 450_000, "perAttendeeIdr": 20_000},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("updating returned %d: %v", status, updated)
	}
	rates := updated["scheme"].(map[string]any)["rates"].([]any)
	if len(rates) != 1 {
		t.Fatalf("got %d rates after the edit, want 1 — the deleted one survived", len(rates))
	}
	if rates[0].(map[string]any)["sessionFeeIdr"].(float64) != 450_000 {
		t.Fatalf("the surviving rate was not updated: %v", rates[0])
	}
}

// A rate for a class type that does not exist is refused, not silently dropped.
func TestRateForAnUnknownClassTypeIsRefused(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, body := h.request(http.MethodPost, "/api/admin/incentives/schemes", token, map[string]any{
		"coachId": "coa_kevin", "sessionFeeIdr": 150_000, "perAttendeeIdr": 10_000,
		"fullClassBonusIdr": 0, "fullClassThresholdPercent": 80, "noShowPenaltyIdr": 0,
		"rates": []map[string]any{{"classTypeId": "clt_nonsense", "sessionFeeIdr": 1, "perAttendeeIdr": 0}},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown class type returned %d: %v", status, body)
	}

	// And a class type cannot have two rates. A different coach, because the
	// refused attempt above must not have left a scheme behind.
	status, body = h.request(http.MethodPost, "/api/admin/incentives/schemes", token, map[string]any{
		"coachId": "coa_maya", "sessionFeeIdr": 150_000, "perAttendeeIdr": 10_000,
		"fullClassBonusIdr": 0, "fullClassThresholdPercent": 80, "noShowPenaltyIdr": 0,
		"rates": []map[string]any{
			{"classTypeId": "clt_simulation", "sessionFeeIdr": 1, "perAttendeeIdr": 0},
			{"classTypeId": "clt_simulation", "sessionFeeIdr": 2, "perAttendeeIdr": 0},
		},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("two rates for one class type returned %d: %v", status, body)
	}
}

// What each coach is paid, resolved, without reading the schemes table.
func TestCoachFeesResolveTheSchemeThatApplies(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	h.request(http.MethodPost, "/api/admin/incentives/schemes", token, map[string]any{
		"coachId": "coa_kevin", "sessionFeeIdr": 250_000, "perAttendeeIdr": 20_000,
		"fullClassBonusIdr": 0, "fullClassThresholdPercent": 80, "noShowPenaltyIdr": 0,
		"rates": []map[string]any{{"classTypeId": "clt_simulation", "sessionFeeIdr": 400_000, "perAttendeeIdr": 25_000}},
	})

	status, fees := h.requestList(http.MethodGet, "/api/admin/incentives/coach-fees", token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading coach fees returned %d: %v", status, fees)
	}
	if len(fees) == 0 {
		t.Fatal("no coaches came back")
	}

	byCoach := map[string]map[string]any{}
	for _, fee := range fees {
		byCoach[fee["coachId"].(string)] = fee
	}

	kevin := byCoach["coa_kevin"]
	if kevin["ownScheme"] != true {
		t.Fatal("the coach with their own scheme is shown on the studio default")
	}
	if kevin["sessionFeeIdr"].(float64) != 250_000 || kevin["classRates"].(float64) != 1 {
		t.Fatalf("Kevin's fee reads %v", kevin)
	}

	// Everybody else falls back to the studio default, which is the point of
	// having one.
	maya := byCoach["coa_maya"]
	if maya["ownScheme"] != false {
		t.Fatal("a coach with no scheme of their own was shown as having one")
	}
	if maya["sessionFeeIdr"].(float64) <= 0 {
		t.Fatalf("the default fee did not come through: %v", maya)
	}
}
