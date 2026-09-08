package app_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Partners, previews and spreadsheets.

func TestAPartnerEventIsSignedMatchedAndLandsOnce(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	const secret = "a-partner-secret-long-enough"
	status, partner := h.request(http.MethodPut, "/api/admin/crm/partners", token, map[string]any{
		"code": "RACEDAY", "name": "Race Day Timing", "kind": "RACE",
		"awardsXp": true, "secret": secret,
	})
	if status != http.StatusOK {
		t.Fatalf("saving a partner returned %d: %v", status, partner)
	}
	// The secret never comes back out; only whether there is one.
	if partner["hasSecret"] != true {
		t.Fatalf("the partner should have a secret, got %v", partner)
	}
	if _, leaked := partner["secret"]; leaked {
		t.Fatal("a shared secret must never be returned")
	}

	body := `{"externalId":"race-991","eventType":"RACE_FINISH",` +
		`"subject":"demo@nuhabit.id","xp":250,"payload":{"time":"01:12:44"}}`
	sign := func(payload, key string) map[string]string {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(payload))
		return map[string]string{"X-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil))}
	}

	// Unsigned, and signed with the wrong key, are both refused: this endpoint
	// writes to the loyalty ledger.
	status, _ = h.raw(http.MethodPost, "/api/webhooks/partners/RACEDAY/events", nil, body)
	if status != http.StatusForbidden {
		t.Fatalf("an unsigned event must be refused, got %d", status)
	}
	status, _ = h.raw(http.MethodPost, "/api/webhooks/partners/RACEDAY/events",
		sign(body, "wrong-secret-long-enough"), body)
	if status != http.StatusForbidden {
		t.Fatalf("a wrongly signed event must be refused, got %d", status)
	}

	before := h.loyaltyOf(t, token, demoMember)

	status, stored := h.raw(http.MethodPost, "/api/webhooks/partners/RACEDAY/events",
		sign(body, secret), body)
	if status != http.StatusCreated {
		t.Fatalf("a signed event returned %d: %v", status, stored)
	}
	if stored["status"] != "PROCESSED" || stored["memberId"] != demoMember {
		t.Fatalf("the subject matches the demo member's email, got %v", stored)
	}
	// The partner's own figure is capped: what an event is worth is the
	// studio's decision, not the sender's.
	if stored["xpAwarded"].(float64) != 250 {
		t.Fatalf("250 is under the cap and should be awarded, got %v", stored["xpAwarded"])
	}
	if after := h.loyaltyOf(t, token, demoMember); after != before+250 {
		t.Fatalf("the points should have landed, %v -> %v", before, after)
	}

	// A retry is the same event, not a second one — and pays nothing again.
	status, again := h.raw(http.MethodPost, "/api/webhooks/partners/RACEDAY/events",
		sign(body, secret), body)
	if status != http.StatusOK {
		t.Fatalf("a redelivery is accepted, got %d: %v", status, again)
	}
	if after := h.loyaltyOf(t, token, demoMember); after != before+250 {
		t.Fatal("a redelivery must not pay twice")
	}
	status, events := h.requestList(http.MethodGet,
		"/api/admin/crm/events?partnerId="+partner["id"].(string), token, nil)
	if status != http.StatusOK || len(events) != 1 {
		t.Fatalf("one event expected, got %d", len(events))
	}
}

func TestAnEventAboutNobodyWaitsToBeMatched(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	const secret = "another-partner-secret-here"
	h.request(http.MethodPut, "/api/admin/crm/partners", token, map[string]any{
		"code": "PARTNERGYM", "name": "Partner Gym", "kind": "GYM",
		"awardsXp": true, "secret": secret,
	})

	body := `{"externalId":"visit-1","eventType":"VISIT","subject":"stranger@example.com"}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	headers := map[string]string{"X-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil))}

	status, stored := h.raw(http.MethodPost, "/api/webhooks/partners/PARTNERGYM/events", headers, body)
	if status != http.StatusCreated {
		t.Fatalf("storing an unmatched event returned %d: %v", status, stored)
	}
	// A queue for a person, not an error: the member may join next week, and
	// throwing it away means it can never be matched then.
	if stored["status"] != "UNMATCHED" || stored["memberId"] != nil {
		t.Fatalf("an event about nobody waits, got %v", stored)
	}

	status, matched := h.request(http.MethodPost,
		"/api/admin/crm/events/"+stored["id"].(string)+"/rematch", token,
		map[string]any{"memberId": demoMember})
	if status != http.StatusOK || matched["memberId"] != demoMember {
		t.Fatalf("rematching returned %d: %v", status, matched)
	}
	// What the partner sent is untouched: only our side of the match changed.
	if matched["subject"] != "stranger@example.com" {
		t.Fatalf("the partner's own words stay put, got %v", matched["subject"])
	}
}

func TestACampaignPreviewShowsWhoWouldActuallyGetIt(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, previews := h.requestList(http.MethodPost, "/api/admin/crm/campaigns/preview", token,
		map[string]any{
			"message":   "Hi {{firstName}}, your {{missingThing}} is ready.",
			"memberIds": []string{demoMember},
		})
	if status != http.StatusOK || len(previews) != 1 {
		t.Fatalf("previewing returned %d with %d rows", status, len(previews))
	}
	preview := previews[0]
	if !strings.Contains(preview["message"].(string), "Hi Fahmi") {
		t.Fatalf("the member's own name should be filled in, got %v", preview["message"])
	}
	// The placeholder nothing filled is named, which is the thing worth
	// knowing before four hundred people read it.
	missing := preview["missing"].([]any)
	if len(missing) != 1 || missing[0] != "missingThing" {
		t.Fatalf("the unfilled placeholder should be named, got %v", missing)
	}
	if preview["deliverable"] != true {
		t.Fatalf("a member who has not opted out is deliverable, got %v", preview)
	}

	// Consent is part of the preview, because "who will get this" is the
	// question a preview is asked.
	h.request(http.MethodPut, "/api/admin/crm/members/"+demoMember+"/consent", token,
		map[string]any{"channel": "PUSH", "optedIn": false})
	status, after := h.requestList(http.MethodPost, "/api/admin/crm/campaigns/preview", token,
		map[string]any{"message": "Hi {{firstName}}", "memberIds": []string{demoMember}})
	if status != http.StatusOK {
		t.Fatalf("previewing returned %d", status)
	}
	if after[0]["deliverable"] != false || after[0]["skipReason"] == "" {
		t.Fatalf("an opted-out member is not deliverable, got %v", after[0])
	}
}

func TestAnImportSaysWhatItWouldDoBeforeItDoesIt(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// Indonesian thousands separators, an existing SKU, and one bad row.
	file := strings.Join([]string{
		"SKU,Name,Category,Unit,Cost,Kind",
		"NUT-BAR-CHC,Protein Bar (Chocolate),NUT,PCS,18.000,RETAIL",
		"NUT-NEW-001,Electrolyte Tabs,NUT,PCS,45.000,RETAIL",
		"NUT-BAD-001,Broken Row,NOSUCHCATEGORY,PCS,1.000,RETAIL",
		"",
	}, "\n")

	status, dry := h.raw(http.MethodPost, "/api/admin/inventory/items/import",
		map[string]string{"Authorization": "Bearer " + token, "Content-Type": "text/csv"}, file)
	if status != http.StatusOK {
		t.Fatalf("a dry run returned %d: %v", status, dry)
	}
	if dry["dryRun"] != true {
		t.Fatal("an import is a dry run until somebody says otherwise")
	}
	if dry["updated"].(float64) != 1 || dry["created"].(float64) != 1 {
		t.Fatalf("one existing and one new, got %v", dry)
	}
	// One bad row is reported by line number and the rest still count: a file
	// with one broken line should not lose the others.
	failures := dry["failures"].([]any)
	if len(failures) != 1 || failures[0].(map[string]any)["line"].(float64) != 4 {
		t.Fatalf("the bad row should be named by line, got %v", failures)
	}

	// Nothing has actually changed yet.
	status, before := h.requestList(http.MethodGet,
		"/api/admin/inventory/items?query=Electrolyte", token, nil)
	if status != http.StatusOK || len(before) != 0 {
		t.Fatalf("a dry run must not write, got %d rows", len(before))
	}

	status, applied := h.raw(http.MethodPost, "/api/admin/inventory/items/import?apply=true",
		map[string]string{"Authorization": "Bearer " + token, "Content-Type": "text/csv"}, file)
	if status != http.StatusOK || applied["dryRun"] != false {
		t.Fatalf("applying returned %d: %v", status, applied)
	}
	status, after := h.requestList(http.MethodGet,
		"/api/admin/inventory/items?query=Electrolyte", token, nil)
	if status != http.StatusOK || len(after) != 1 {
		t.Fatalf("the new item should exist, got %d rows", len(after))
	}
	// "45.000" is forty-five thousand, not forty-five: an Indonesian
	// spreadsheet uses dots for thousands.
	if after[0]["unitCostIdr"].(float64) != 45_000 {
		t.Fatalf("45.000 is forty-five thousand, got %v", after[0]["unitCostIdr"])
	}

	// A file with no SKU column is a file problem, said plainly.
	status, wrong := h.raw(http.MethodPost, "/api/admin/inventory/items/import",
		map[string]string{"Authorization": "Bearer " + token, "Content-Type": "text/csv"},
		"name,cost\nSomething,100")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a file without the required columns is refused, got %d: %v", status, wrong)
	}
}

func TestAnExportCanBeEditedAndImportedBack(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	body, err := h.rawText(http.MethodGet, "/api/admin/inventory/items/export", token)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	// Excel needs the BOM or every accented name arrives mangled.
	if !strings.HasPrefix(body, "\ufeff") {
		t.Fatal("the export should start with a UTF-8 BOM")
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 2 {
		t.Fatalf("the export should have a header and rows, got %d lines", len(lines))
	}
	header := strings.TrimPrefix(lines[0], "\ufeff")
	if !strings.HasPrefix(header, "sku,name") {
		t.Fatalf("the export's shape is what the importer reads, got %q", header)
	}

	// Round trip: what came out goes back in without a single edit, and
	// changes nothing.
	status, result := h.raw(http.MethodPost, "/api/admin/inventory/items/import",
		map[string]string{"Authorization": "Bearer " + token, "Content-Type": "text/csv"}, body)
	if status != http.StatusOK {
		t.Fatalf("re-importing an export returned %d: %v", status, result)
	}
	if result["created"].(float64) != 0 {
		t.Fatalf("an unedited export creates nothing, got %v", result)
	}
	if len(result["failures"].([]any)) != 0 {
		t.Fatalf("an export must import cleanly, got %v", result["failures"])
	}
	if fmt.Sprintf("%v", result["updated"]) != fmt.Sprintf("%v", result["rows"]) {
		t.Fatalf("every row should be an update, got %v", result)
	}
}
