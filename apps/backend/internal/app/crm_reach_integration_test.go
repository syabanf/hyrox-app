package app_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"
)

// Reaching a member, and hearing back.

func TestAnOptOutIsAHardExclusionAndTransactionalStillGetsThrough(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	// Nobody has been asked, so a campaign may go out. Silence is not refusal:
	// somebody who signed up has agreed to hear from the place they signed up
	// to.
	status, prefs := h.requestList(http.MethodGet,
		"/api/admin/crm/members/"+demoMember+"/consent", token, nil)
	if status != http.StatusOK || len(prefs) != 0 {
		t.Fatalf("a member starts with no preferences, got %d with %d rows", status, len(prefs))
	}

	status, saved := h.request(http.MethodPut,
		"/api/admin/crm/members/"+demoMember+"/consent", token, map[string]any{
			"channel": "PUSH", "optedIn": false, "reason": "Too many messages",
		})
	if status != http.StatusOK || saved["optedIn"] != false {
		t.Fatalf("recording an opt-out returned %d: %v", status, saved)
	}
	// The default scope is marketing: refusing campaigns is not refusing to be
	// told your class was cancelled.
	if saved["scope"] != "MARKETING" {
		t.Fatalf("an opt-out defaults to marketing only, got %v", saved["scope"])
	}

	// And the member can change their own mind, which is how most opt-outs
	// actually happen.
	member := h.memberToken("demo@nuhabit.id")
	status, mine := h.requestList(http.MethodGet, "/api/me/consent", member, nil)
	if status != http.StatusOK || len(mine) != 1 {
		t.Fatalf("a member can read their own preferences, got %d with %d rows", status, len(mine))
	}
	status, back := h.request(http.MethodPut, "/api/me/consent", member, map[string]any{
		"channel": "PUSH", "optedIn": true,
	})
	if status != http.StatusOK || back["optedIn"] != true {
		t.Fatalf("a member can opt back in, got %d: %v", status, back)
	}
}

func TestABadgeIsEarnedOnceAndPaysItsPointsThroughTheLedger(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, badge := h.request(http.MethodPut, "/api/admin/crm/badges", token, map[string]any{
		"code": "FIRST-STEPS", "name": "First steps", "metric": "BOOKINGS",
		"threshold": 1, "bonusXp": 100,
	})
	if status != http.StatusOK {
		t.Fatalf("saving a badge returned %d: %v", status, badge)
	}

	// The badge is earned by doing the thing, so the member does it. Awarding
	// against a counter nobody moved would test the query and nothing else.
	member := h.memberToken("demo@nuhabit.id")
	h.topUp(member, "pkg_starter5")
	sessionID := h.createSession(token, 20*time.Minute, 10)
	if status, booked := h.request(http.MethodPost,
		"/api/sessions/"+sessionID+"/book", member, nil); status != http.StatusCreated {
		t.Fatalf("booking returned %d: %v", status, booked)
	}

	before := h.loyaltyOf(t, token, demoMember)

	status, awarded := h.requestList(http.MethodPost,
		"/api/admin/crm/members/"+demoMember+"/badges/evaluate", token, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("evaluating badges returned %d", status)
	}
	if len(awarded) != 1 || awarded[0]["code"] != "FIRST-STEPS" {
		t.Fatalf("the demo member has bookings, so the badge is earned: %v", awarded)
	}

	// The points arrive through the ordinary ledger, so the balance still
	// equals the sum of its entries.
	after := h.loyaltyOf(t, token, demoMember)
	if after != before+100 {
		t.Fatalf("the badge is worth 100 points, %v -> %v", before, after)
	}

	// Running it again awards nothing and pays nothing: a badge that can be
	// earned twice is a counter wearing a badge's clothes.
	status, again := h.requestList(http.MethodPost,
		"/api/admin/crm/members/"+demoMember+"/badges/evaluate", token, map[string]any{})
	if status != http.StatusOK || len(again) != 0 {
		t.Fatalf("a second evaluation awards nothing, got %d: %v", status, again)
	}
	if h.loyaltyOf(t, token, demoMember) != after {
		t.Fatal("a second evaluation must not pay again")
	}

	// And it cannot be handed out twice by hand either.
	badgeID := badge["id"].(string)
	status, refused := h.request(http.MethodPost,
		"/api/admin/crm/members/"+demoMember+"/badges", token,
		map[string]any{"badgeId": badgeID})
	if status != http.StatusConflict || errorCode(refused) != "ALREADY_EARNED" {
		t.Fatalf("want ALREADY_EARNED, got %d: %v", status, refused)
	}
}

func TestAThreadReopensWhenTheMemberWritesAgain(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	status, opened := h.request(http.MethodPost, "/api/admin/crm/conversations", token,
		map[string]any{
			"memberId": demoMember, "contactName": "Demo Member", "channel": "INBOX",
			"subject": "Class was full", "body": "I could not get in", "inbound": true,
		})
	if status != http.StatusCreated {
		t.Fatalf("opening a conversation returned %d: %v", status, opened)
	}
	conversationID := opened["id"].(string)
	if opened["waiting"] != true {
		t.Fatal("a thread nobody has answered is waiting")
	}

	// An internal note is not an answer, so it must not stop the clock.
	status, noted := h.request(http.MethodPost,
		"/api/admin/crm/conversations/"+conversationID+"/messages", token,
		map[string]any{"body": "Checking the roster", "internal": true})
	if status != http.StatusCreated {
		t.Fatalf("posting a note returned %d: %v", status, noted)
	}
	if noted["waiting"] != true {
		t.Fatal("a note to a colleague is not an answer to the member")
	}
	if noted["firstResponseSeconds"] != nil {
		t.Fatal("an internal note must not set the first-response time")
	}

	status, answered := h.request(http.MethodPost,
		"/api/admin/crm/conversations/"+conversationID+"/messages", token,
		map[string]any{"body": "Sorry — we have added you to the next one"})
	if status != http.StatusCreated {
		t.Fatalf("replying returned %d: %v", status, answered)
	}
	if answered["waiting"] != false || answered["status"] != "PENDING" {
		t.Fatalf("an answered thread waits on the member, got %v", answered["status"])
	}
	if answered["firstResponseSeconds"] == nil {
		t.Fatal("the first real reply sets the response time")
	}

	// Resolve it, then the member writes again. This is the case a status
	// field alone gets wrong, because nobody remembers to set it back.
	status, resolved := h.request(http.MethodPut,
		"/api/admin/crm/conversations/"+conversationID, token,
		map[string]any{"status": "RESOLVED"})
	if status != http.StatusOK || resolved["status"] != "RESOLVED" {
		t.Fatalf("resolving returned %d: %v", status, resolved)
	}

	status, reopened := h.request(http.MethodPost,
		"/api/admin/crm/conversations/"+conversationID+"/messages", token,
		map[string]any{"body": "It happened again", "inbound": true})
	if status != http.StatusCreated {
		t.Fatalf("a member writing again returned %d: %v", status, reopened)
	}
	if reopened["status"] != "OPEN" || reopened["waiting"] != true {
		t.Fatalf("a member writing again reopens the thread, got %v", reopened["status"])
	}
}

func TestAnInstagramDeliveryIsSignedAndLandsOnce(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")

	body := `{"object":"instagram","entry":[{"id":"page","messaging":[` +
		`{"sender":{"id":"ig_42"},"timestamp":1,"message":{"mid":"m_1","text":"Do you open on Sunday?"}}]}]}`

	// Unsigned is refused. A public endpoint that writes to the support inbox
	// without checking a signature is an open door.
	status, _ := h.raw(http.MethodPost, "/api/webhooks/instagram", nil, body)
	if status != http.StatusForbidden {
		t.Fatalf("an unsigned delivery must be refused, got %d", status)
	}

	sign := func(payload string) map[string]string {
		mac := hmac.New(sha256.New, []byte(instagramSecret))
		mac.Write([]byte(payload))
		return map[string]string{"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil))}
	}

	// A signature over different bytes is not a signature over these.
	status, _ = h.raw(http.MethodPost, "/api/webhooks/instagram", sign(body+" "), body)
	if status != http.StatusForbidden {
		t.Fatalf("a signature over other bytes must be refused, got %d", status)
	}

	status, accepted := h.raw(http.MethodPost, "/api/webhooks/instagram", sign(body), body)
	if status != http.StatusOK {
		t.Fatalf("a signed delivery returned %d: %v", status, accepted)
	}

	status, threads := h.requestList(http.MethodGet,
		"/api/admin/crm/conversations?channel=INSTAGRAM", token, nil)
	if status != http.StatusOK || len(threads) != 1 {
		t.Fatalf("one conversation expected, got %d with %d", status, len(threads))
	}

	// Meta redelivers. The same message must land once — the provider's id is
	// the whole of the idempotency story.
	status, _ = h.raw(http.MethodPost, "/api/webhooks/instagram", sign(body), body)
	if status != http.StatusOK {
		t.Fatalf("a redelivery is accepted, got %d", status)
	}
	status, after := h.requestList(http.MethodGet,
		"/api/admin/crm/conversations?channel=INSTAGRAM", token, nil)
	if status != http.StatusOK || len(after) != 1 {
		t.Fatalf("a redelivery must not open a second thread, got %d", len(after))
	}

	conversation, err := h.requestObject(http.MethodGet,
		"/api/admin/crm/conversations/"+after[0]["id"].(string), token)
	if err != nil {
		t.Fatalf("reading the thread: %v", err)
	}
	if messages := conversation["messages"].([]any); len(messages) != 1 {
		t.Fatalf("a redelivery must not post the message twice, got %d", len(messages))
	}
}

func TestHiddenReviewsLeaveTheAverageWithoutBeingDeleted(t *testing.T) {
	h := newHarness(t)
	token := h.adminToken("adm_super")
	member := h.memberToken("demo@nuhabit.id")

	status, review := h.request(http.MethodPost, "/api/me/reviews", member, map[string]any{
		"subjectType": "BRANCH", "subjectId": senopati, "rating": 2,
		"comment": "Too crowded at six",
	})
	if status != http.StatusCreated {
		t.Fatalf("leaving a review returned %d: %v", status, review)
	}
	// Reviewing a branch is not backed by a visit to a class, so it is honest
	// about being unverified rather than refused.
	if review["verified"] != false {
		t.Fatalf("a branch review has no visit behind it, got %v", review["verified"])
	}

	status, summary := h.request(http.MethodGet,
		"/api/admin/crm/reviews/summary?subjectType=BRANCH&subjectId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the summary returned %d: %v", status, summary)
	}
	if summary["summary"].(map[string]any)["average"].(float64) != 2 {
		t.Fatalf("one two-star review averages 2, got %v", summary["summary"])
	}

	status, hidden := h.request(http.MethodPut,
		"/api/admin/crm/reviews/"+review["id"].(string)+"/status", token,
		map[string]any{"status": "HIDDEN"})
	if status != http.StatusOK || hidden["status"] != "HIDDEN" {
		t.Fatalf("hiding returned %d: %v", status, hidden)
	}

	// Hiding is not deleting: the review stays, it leaves the average, and
	// there is a record that somebody decided.
	status, after := h.request(http.MethodGet,
		"/api/admin/crm/reviews/summary?subjectType=BRANCH&subjectId="+senopati, token, nil)
	if status != http.StatusOK {
		t.Fatalf("the summary returned %d", status)
	}
	if after["summary"].(map[string]any)["count"].(float64) != 0 {
		t.Fatalf("a hidden review leaves the average, got %v", after["summary"])
	}
	status, all := h.requestList(http.MethodGet, "/api/admin/crm/reviews", token, nil)
	if status != http.StatusOK || len(all) != 1 {
		t.Fatalf("the review itself is still there, got %d rows", len(all))
	}
}
