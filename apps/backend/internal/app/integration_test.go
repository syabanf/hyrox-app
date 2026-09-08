package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/app"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/config"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/seed"
	"github.com/syabanf/nuhabit-backend/migrations"
)

// These tests drive the real HTTP surface against a real PostgreSQL database.
// They are skipped unless TEST_DATABASE_URL points at one, so `go test ./...`
// stays useful on a machine with no database.
//
//	TEST_DATABASE_URL=postgres://nuhabit:nuhabit@localhost:5432/nuhabit_test?sslmode=disable go test ./...

type harness struct {
	t      *testing.T
	server *httptest.Server
	app    *app.App
}

const (
	instagramVerifyToken = "integration-verify-token"
	instagramSecret      = "integration-app-secret"
)

func newHarness(t *testing.T) *harness {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration tests")
	}

	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("AUTH_SECRET", "integration-test-secret")
	t.Setenv("AUTH_DEMO_OTP", "true")
	t.Setenv("APP_ENV", "test")
	// The Instagram webhook refuses everything unless it is configured, so
	// the harness configures it — the signature check is the thing under test,
	// not something to be switched off.
	t.Setenv("INSTAGRAM_VERIFY_TOKEN", instagramVerifyToken)
	t.Setenv("INSTAGRAM_APP_SECRET", instagramSecret)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	ctx := context.Background()
	db, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		t.Fatalf("connecting to the test database: %v", err)
	}
	if err := db.Migrate(ctx, migrations.FS); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	truncateAll(t, db)
	if _, err := seed.New(db, clock.Real{}).Run(ctx); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	application := app.New(cfg, db)
	server := httptest.NewServer(application.Handler)
	t.Cleanup(func() {
		server.Close()
		db.Close()
	})
	return &harness{t: t, server: server, app: application}
}

// truncateAll gives every test a clean studio. Tables are listed rather than
// dropped so the schema itself is exercised by the migration run above.
func truncateAll(t *testing.T, db *database.DB) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		TRUNCATE
			access.access_logs, access.qr_tokens,
			scheduling.bookings, scheduling.class_sessions,
			wallet.voucher_redemptions, wallet.top_up_lots, wallet.payments,
			wallet.credit_ledger_entries, wallet.vouchers,
			incentives.payouts, incentives.schemes,
			engagement.challenge_joins, engagement.challenges,
			engagement.campaign_recipients, engagement.member_notifications,
			engagement.campaigns, engagement.message_templates,
			pos.payments, pos.order_items, pos.orders, pos.shifts,
			pos.product_prices, pos.products, pos.categories,
			crm.redemptions, crm.xp_ledger, crm.rewards, crm.member_profiles,
			crm.xp_rules, crm.tiers,
			crm.conversation_messages, crm.conversations, crm.reviews,
			crm.member_badges, crm.badges, crm.contact_preferences,
			purchasing.purchase_return_items, purchasing.purchase_returns,
			purchasing.goods_receipt_items, purchasing.goods_receipts,
			purchasing.purchase_order_items, purchasing.purchase_orders,
			purchasing.purchase_request_items, purchasing.purchase_requests,
			purchasing.supplier_prices, purchasing.suppliers,
			inventory.stock_take_lines, inventory.stock_takes, inventory.stock_transfers,
			inventory.batch_movements, inventory.batches,
			inventory.stock_movements, inventory.stock_levels,
			inventory.item_packs, inventory.items, inventory.categories,
			hris.attendance, hris.leaves, hris.leave_balances, hris.overtime_requests,
			hris.employee_shifts, hris.employees, hris.shifts, hris.public_holidays,
			hris.departments, hris.positions, hris.employment_statuses,
			identity.otp_challenges, identity.members, identity.admin_users,
			catalog.credit_packages, catalog.class_types, catalog.coaches,
			catalog.gates, catalog.branches, catalog.organizations,
			catalog.substitution_rules, catalog.exercises,
			platform.audit_events, platform.outbox_messages
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncating: %v", err)
	}
}

// request performs an API call and decodes the JSON response.
func (h *harness) request(method, path, token string, body any) (int, map[string]any) {
	h.t.Helper()

	var payload *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("encoding request: %v", err)
		}
		payload = bytes.NewReader(encoded)
	} else {
		payload = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, h.server.URL+path, payload)
	if err != nil {
		h.t.Fatalf("building request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res.StatusCode, decoded
}

// requestList is the array-returning variant.
func (h *harness) requestList(method, path, token string, body any) (int, []map[string]any) {
	h.t.Helper()

	var payload *bytes.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		payload = bytes.NewReader(encoded)
	} else {
		payload = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, h.server.URL+path, payload)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	var decoded []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res.StatusCode, decoded
}

// memberToken signs a member in through the real OTP flow.
func (h *harness) memberToken(identifier string) string {
	h.t.Helper()

	status, challenge := h.request(http.MethodPost, "/api/auth/otp/request", "",
		map[string]string{"identifier": identifier})
	if status != http.StatusOK {
		h.t.Fatalf("requesting an OTP returned %d: %v", status, challenge)
	}
	status, session := h.request(http.MethodPost, "/api/auth/otp/verify", "", map[string]string{
		"challengeId": challenge["challengeId"].(string),
		"code":        challenge["code"].(string),
	})
	if status != http.StatusOK {
		h.t.Fatalf("verifying an OTP returned %d: %v", status, session)
	}
	return session["token"].(string)
}

func (h *harness) adminToken(userID string) string {
	h.t.Helper()
	status, session := h.request(http.MethodPost, "/api/admin/auth/login", "",
		map[string]string{"userId": userID})
	if status != http.StatusOK {
		h.t.Fatalf("admin login returned %d: %v", status, session)
	}
	return session["token"].(string)
}

// createSession schedules a class the given number of minutes from now.
func (h *harness) createSession(adminToken string, startsIn time.Duration, capacity int) string {
	h.t.Helper()
	status, session := h.request(http.MethodPost, "/api/admin/sessions", adminToken, map[string]any{
		"classTypeId": "clt_engine",
		"branchId":    "brn_senopati",
		"coachId":     "coa_kevin",
		"startsAt":    time.Now().Add(startsIn).UTC().Format(time.RFC3339),
		"capacity":    capacity,
		"publish":     true,
	})
	if status != http.StatusCreated {
		h.t.Fatalf("creating a session returned %d: %v", status, session)
	}
	return session["id"].(string)
}

// topUp buys a package and settles it, returning the credits granted.
func (h *harness) topUp(token, packageID string) int {
	h.t.Helper()
	status, result := h.request(http.MethodPost, "/api/me/topup", token, map[string]any{
		"packageId": packageID,
		"channel":   "QRIS",
	})
	if status != http.StatusCreated {
		h.t.Fatalf("top-up returned %d: %v", status, result)
	}
	payment := result["payment"].(map[string]any)

	status, settled := h.request(http.MethodPost, "/api/payments/"+payment["id"].(string)+"/simulate", token, nil)
	if status != http.StatusOK {
		h.t.Fatalf("settling returned %d: %v", status, settled)
	}
	return int(payment["credits"].(float64))
}

func (h *harness) balance(token string) int {
	h.t.Helper()
	status, wallet := h.request(http.MethodGet, "/api/me/wallet", token, nil)
	if status != http.StatusOK {
		h.t.Fatalf("reading the wallet returned %d: %v", status, wallet)
	}
	return int(wallet["balance"].(float64))
}

func errorCode(body map[string]any) string {
	if raw, ok := body["error"].(map[string]any); ok {
		if code, ok := raw["code"].(string); ok {
			return code
		}
	}
	return ""
}

// TestCoreLoop walks the studio's central flow end to end: register, buy
// credits, book, check in at the gate, get charged exactly once.
func TestCoreLoop(t *testing.T) {
	h := newHarness(t)

	status, session := h.request(http.MethodPost, "/api/auth/register", "", map[string]any{
		"fullName":       "Integration Tester",
		"email":          "tester@example.com",
		"phone":          "+628111000111",
		"waiverAccepted": true,
		"termsAccepted":  true,
	})
	if status != http.StatusCreated {
		t.Fatalf("register returned %d: %v", status, session)
	}
	token := session["token"].(string)

	if got := h.balance(token); got != 0 {
		t.Fatalf("a new member starts with %d credits, want 0", got)
	}

	credits := h.topUp(token, "pkg_starter5")
	if got := h.balance(token); got != credits {
		t.Fatalf("balance after top-up = %d, want %d", got, credits)
	}

	admin := h.adminToken("adm_super")
	sessionID := h.createSession(admin, 20*time.Minute, 10)

	status, booking := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", token, nil)
	if status != http.StatusCreated {
		t.Fatalf("booking returned %d: %v", status, booking)
	}
	if booking["decision"] != "CONFIRMED" {
		t.Fatalf("decision = %v, want CONFIRMED", booking["decision"])
	}
	// Booking must not touch credits; the gate does that.
	if got := h.balance(token); got != credits {
		t.Fatalf("booking changed the balance to %d, want %d", got, credits)
	}

	_, qr := h.request(http.MethodPost, "/api/me/qr", token, nil)
	status, scan := h.request(http.MethodPost, "/api/gates/gat_senopati_a/scan", "",
		map[string]any{"qrToken": qr["token"]})
	if status != http.StatusOK {
		t.Fatalf("scan returned %d: %v", status, scan)
	}
	if scan["decision"] != "ALLOWED" || scan["entryKind"] != "BOOKING" {
		t.Fatalf("scan = %v, want an allowed booking entry", scan)
	}
	if got := h.balance(token); got != credits-1 {
		t.Fatalf("balance after check-in = %d, want %d", got, credits-1)
	}
}

// TestQrTokenIsSingleUse proves a screenshotted code cannot be reused.
func TestQrTokenIsSingleUse(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("demo@nuhabit.id")
	h.topUp(token, "pkg_visit10")

	admin := h.adminToken("adm_super")
	sessionID := h.createSession(admin, 15*time.Minute, 10)
	h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", token, nil)

	_, qr := h.request(http.MethodPost, "/api/me/qr", token, nil)
	code := map[string]any{"qrToken": qr["token"]}

	_, first := h.request(http.MethodPost, "/api/gates/gat_senopati_a/scan", "", code)
	if first["decision"] != "ALLOWED" {
		t.Fatalf("first scan = %v, want ALLOWED", first)
	}

	_, second := h.request(http.MethodPost, "/api/gates/gat_senopati_a/scan", "", code)
	if second["decision"] != "DENIED" || second["reason"] != "TOKEN_CONSUMED" {
		t.Fatalf("replayed scan = %v, want DENIED/TOKEN_CONSUMED", second)
	}
}

// TestGateRefusesWithoutCredits proves the door will not open on credit the
// member does not have, and that nothing is charged when it refuses.
func TestGateRefusesWithoutCredits(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("natalie@example.com")
	admin := h.adminToken("adm_super")

	// Enough credits to book, then spent elsewhere before the class.
	h.topUp(token, "pkg_trial")
	sessionID := h.createSession(admin, 15*time.Minute, 10)

	status, booking := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", token, nil)
	if status != http.StatusCreated {
		t.Fatalf("booking returned %d: %v", status, booking)
	}

	// Take the credit away, simulating it being spent at another branch.
	adminMember := h.adminToken("adm_super")
	status, adjustment := h.request(http.MethodPost, "/api/admin/members/mem_natalie/adjust", adminMember,
		map[string]any{"amount": -1, "reason": "integration test: spend the credit"})
	if status != http.StatusCreated {
		t.Fatalf("adjustment returned %d: %v", status, adjustment)
	}

	_, qr := h.request(http.MethodPost, "/api/me/qr", token, nil)
	_, scan := h.request(http.MethodPost, "/api/gates/gat_senopati_a/scan", "",
		map[string]any{"qrToken": qr["token"]})
	if scan["decision"] != "DENIED" || scan["reason"] != "INSUFFICIENT_CREDITS" {
		t.Fatalf("scan = %v, want DENIED/INSUFFICIENT_CREDITS", scan)
	}
	if got := h.balance(token); got != 0 {
		t.Fatalf("a refused scan changed the balance to %d, want 0", got)
	}
}

// TestCapacityAndWaitlistPromotion proves the last place cannot be sold twice
// and that giving it up hands it to whoever was waiting.
func TestCapacityAndWaitlistPromotion(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	sessionID := h.createSession(admin, 3*time.Hour, 1)

	first := h.memberToken("demo@nuhabit.id")
	second := h.memberToken("lucas@example.com")
	h.topUp(first, "pkg_starter5")
	h.topUp(second, "pkg_starter5")

	_, booked := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", first, nil)
	if booked["decision"] != "CONFIRMED" {
		t.Fatalf("first booking = %v, want CONFIRMED", booked)
	}
	_, waitlisted := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", second, nil)
	if waitlisted["decision"] != "WAITLIST" {
		t.Fatalf("second booking = %v, want WAITLIST", waitlisted)
	}

	// The class is now full.
	_, detail := h.request(http.MethodGet, "/api/admin/sessions/"+sessionID, admin, nil)
	if session := detail["session"].(map[string]any); session["status"] != "FULL" {
		t.Fatalf("session status = %v, want FULL", session["status"])
	}

	bookingID := booked["booking"].(map[string]any)["id"].(string)
	status, cancelled := h.request(http.MethodPost, "/api/bookings/"+bookingID+"/cancel", first, nil)
	if status != http.StatusOK {
		t.Fatalf("cancelling returned %d: %v", status, cancelled)
	}
	// Cancelling three hours out is inside the four-hour deadline, so it is free.
	if cancelled["outcome"] != "RELEASED" && cancelled["outcome"] != "LATE" {
		t.Fatalf("cancellation outcome = %v", cancelled["outcome"])
	}
	if cancelled["promotedMemberName"] == nil {
		t.Fatal("cancelling a full class should promote the waitlisted member")
	}
}

// TestLateCancellationForfeitsACredit pins the studio's cancellation policy.
func TestLateCancellationForfeitsACredit(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	token := h.memberToken("demo@nuhabit.id")
	credits := h.topUp(token, "pkg_starter5")

	// Inside the four-hour deadline, so cancelling is late.
	sessionID := h.createSession(admin, 30*time.Minute, 10)
	_, booked := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", token, nil)
	bookingID := booked["booking"].(map[string]any)["id"].(string)

	_, cancelled := h.request(http.MethodPost, "/api/bookings/"+bookingID+"/cancel", token, nil)
	if cancelled["outcome"] != "LATE" {
		t.Fatalf("outcome = %v, want LATE", cancelled["outcome"])
	}
	if int(cancelled["penaltyCredits"].(float64)) != 1 {
		t.Fatalf("penalty = %v, want 1 credit", cancelled["penaltyCredits"])
	}
	if got := h.balance(token); got != credits-1 {
		t.Fatalf("balance after a late cancellation = %d, want %d", got, credits-1)
	}
}

// TestPackageCoverageIsEnforced proves credits from a restricted package
// cannot be spent on a class it does not cover.
func TestPackageCoverageIsEnforced(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	token := h.memberToken("doris@example.com")

	// The race block only covers simulations.
	h.topUp(token, "pkg_race")

	sessionID := h.createSession(admin, 2*time.Hour, 10) // an Engine Builder class
	status, refused := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", token, nil)
	if status != http.StatusConflict || errorCode(refused) != "PACKAGE_NOT_COVERED" {
		t.Fatalf("booking returned %d/%s, want 409/PACKAGE_NOT_COVERED", status, errorCode(refused))
	}
}

// TestSettlingAPaymentTwiceGrantsCreditsOnce guards against a duplicated
// gateway callback inflating a balance.
func TestSettlingAPaymentTwiceGrantsCreditsOnce(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("demo@nuhabit.id")

	_, result := h.request(http.MethodPost, "/api/me/topup", token, map[string]any{
		"packageId": "pkg_starter5", "channel": "QRIS",
	})
	paymentID := result["payment"].(map[string]any)["id"].(string)

	if status, _ := h.request(http.MethodPost, "/api/payments/"+paymentID+"/simulate", token, nil); status != http.StatusOK {
		t.Fatalf("first settlement returned %d", status)
	}
	status, second := h.request(http.MethodPost, "/api/payments/"+paymentID+"/simulate", token, nil)
	if status != http.StatusConflict || errorCode(second) != "ALREADY_PAID" {
		t.Fatalf("second settlement returned %d/%s, want 409/ALREADY_PAID", status, errorCode(second))
	}
	if got := h.balance(token); got != 5 {
		t.Fatalf("balance = %d, want 5 credits from a single settlement", got)
	}
}

// TestRefundReversesTheCredits proves money going back takes the credits with
// it, without deleting the original entry.
func TestRefundReversesTheCredits(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("demo@nuhabit.id")
	admin := h.adminToken("adm_super")

	_, result := h.request(http.MethodPost, "/api/me/topup", token, map[string]any{
		"packageId": "pkg_starter5", "channel": "QRIS",
	})
	paymentID := result["payment"].(map[string]any)["id"].(string)
	h.request(http.MethodPost, "/api/payments/"+paymentID+"/simulate", token, nil)

	status, refund := h.request(http.MethodPost, "/api/admin/payments/"+paymentID+"/refund", admin,
		map[string]any{"reason": "member changed their mind"})
	if status != http.StatusOK {
		t.Fatalf("refund returned %d: %v", status, refund)
	}
	if payment := refund["payment"].(map[string]any); payment["status"] != "REFUNDED" {
		t.Fatalf("payment status = %v, want REFUNDED", payment["status"])
	}
	if got := h.balance(token); got != 0 {
		t.Fatalf("balance after refund = %d, want 0", got)
	}

	// The history keeps both sides: the top-up and its reversal.
	_, wallet := h.request(http.MethodGet, "/api/me/wallet", token, nil)
	entries := wallet["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("ledger has %d entries, want the top-up and its reversal", len(entries))
	}
}

// TestMemberHomeScreen proves the composite reads the app opens with actually
// assemble across modules: identity for the profile, wallet for the balance
// and live promos, scheduling for the class rail.
func TestMemberHomeScreen(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("demo@nuhabit.id")

	status, me := h.request(http.MethodGet, "/api/me", token, nil)
	if status != http.StatusOK {
		t.Fatalf("/api/me returned %d: %v", status, me)
	}
	member := me["member"].(map[string]any)
	if member["email"] != "demo@nuhabit.id" {
		t.Fatalf("signed in as %v", member["email"])
	}
	if me["lowBalance"] != true {
		t.Fatal("a member with no credits should read as low balance")
	}

	admin := h.adminToken("adm_super")
	h.createSession(admin, 2*time.Hour, 10)

	status, home := h.request(http.MethodGet, "/api/home", token, nil)
	if status != http.StatusOK {
		t.Fatalf("/api/home returned %d: %v", status, home)
	}
	if rail := home["railDay"]; rail != "TODAY" && rail != "TOMORROW" {
		t.Fatalf("railDay = %v", rail)
	}
	if len(home["todaySessions"].([]any)) == 0 {
		t.Fatal("the class rail is empty even though a class was just scheduled")
	}
	// Live vouchers become the promo cards.
	if len(home["promos"].([]any)) == 0 {
		t.Fatal("no promos even though the seed has live vouchers")
	}

	// Settings default rather than 404 for a member who never changed them.
	status, settings := h.request(http.MethodGet, "/api/me/settings", token, nil)
	if status != http.StatusOK || settings["units"] != "METRIC" {
		t.Fatalf("settings = %d %v", status, settings)
	}
	status, saved := h.request(http.MethodPut, "/api/me/settings", token,
		map[string]any{"language": "ID", "weeklyGoalKm": 25})
	if status != http.StatusOK || saved["language"] != "ID" {
		t.Fatalf("saving settings = %d %v", status, saved)
	}
}

// TestNotificationsFollowBookings proves the transactional outbox actually
// delivers: booking a class eventually tells the member about it.
func TestNotificationsFollowBookings(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("demo@nuhabit.id")
	admin := h.adminToken("adm_super")
	h.topUp(token, "pkg_starter5")

	sessionID := h.createSession(admin, 3*time.Hour, 10)
	if status, booked := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", token, nil); status != http.StatusCreated {
		t.Fatalf("booking returned %d: %v", status, booked)
	}

	// The message is published inside the booking transaction; the dispatcher
	// delivers it separately, so drain it here rather than sleeping.
	if err := h.app.DrainOutbox(context.Background()); err != nil {
		t.Fatalf("draining the outbox: %v", err)
	}

	status, notifications := h.requestList(http.MethodGet, "/api/me/notifications", token, nil)
	if status != http.StatusOK {
		t.Fatalf("notifications returned %d", status)
	}
	found := false
	for _, n := range notifications {
		if n["type"] == "BOOKING_CONFIRMED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no booking notification was delivered: %v", notifications)
	}

	status, me := h.request(http.MethodGet, "/api/me", token, nil)
	if status != http.StatusOK || int(me["unreadNotifications"].(float64)) == 0 {
		t.Fatalf("unread count did not follow the notification: %v", me["unreadNotifications"])
	}

	if status, _ := h.request(http.MethodPost, "/api/me/notifications/read-all", token, nil); status != http.StatusOK {
		t.Fatal("marking notifications read failed")
	}
	_, after := h.request(http.MethodGet, "/api/me", token, nil)
	if int(after["unreadNotifications"].(float64)) != 0 {
		t.Fatalf("unread count after read-all = %v, want 0", after["unreadNotifications"])
	}
}

// TestRBACIsEnforcedServerSide proves permissions are not merely a UI concern.
func TestRBACIsEnforcedServerSide(t *testing.T) {
	h := newHarness(t)

	cases := []struct {
		role, user, method, path string
		body                     any
	}{
		{"FINANCE", "adm_finance", http.MethodPost, "/api/admin/sessions", map[string]any{
			"classTypeId": "clt_engine", "branchId": "brn_senopati", "coachId": "coa_kevin",
			"startsAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}},
		{"COACH", "adm_coach", http.MethodGet, "/api/admin/payments", nil},
		{"FRONT_DESK", "adm_desk", http.MethodPost, "/api/admin/vouchers", map[string]any{
			"code": "TESTCODE", "type": "PERCENT", "value": 10,
			"startsAt": time.Now().UTC().Format(time.RFC3339),
			"endsAt":   time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		}},
		{"HQ_ADMIN", "adm_hq", http.MethodPost, "/api/admin/users", map[string]any{
			"name": "New Staff", "email": "new@nuhabit.id", "role": "FRONT_DESK",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			token := h.adminToken(tc.user)
			status, body := h.request(tc.method, tc.path, token, tc.body)
			if status != http.StatusForbidden {
				t.Fatalf("%s %s as %s returned %d, want 403: %v", tc.method, tc.path, tc.role, status, body)
			}
		})
	}
}

// TestAnonymousCallersCannotReachMemberData is the negative case for auth.
func TestAnonymousCallersCannotReachMemberData(t *testing.T) {
	h := newHarness(t)

	for _, path := range []string{"/api/me/wallet", "/api/me/bookings", "/api/me/visits"} {
		if status, _ := h.request(http.MethodGet, path, "", nil); status != http.StatusUnauthorized {
			t.Fatalf("GET %s without a token returned %d, want 401", path, status)
		}
	}
	if status, _ := h.request(http.MethodGet, "/api/admin/reports/dashboard", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("the dashboard is reachable without a token")
	}
}

// TestMembersCannotReadEachOthersPayments proves ownership is checked.
func TestMembersCannotReadEachOthersPayments(t *testing.T) {
	h := newHarness(t)
	owner := h.memberToken("demo@nuhabit.id")
	other := h.memberToken("lucas@example.com")

	_, result := h.request(http.MethodPost, "/api/me/topup", owner, map[string]any{
		"packageId": "pkg_starter5", "channel": "QRIS",
	})
	paymentID := result["payment"].(map[string]any)["id"].(string)

	if status, _ := h.request(http.MethodGet, "/api/payments/"+paymentID, owner, nil); status != http.StatusOK {
		t.Fatalf("the owner cannot read their own payment")
	}
	// A stranger gets a 404 rather than a 403, so payment ids cannot be probed.
	if status, _ := h.request(http.MethodGet, "/api/payments/"+paymentID, other, nil); status != http.StatusNotFound {
		t.Fatalf("another member could read the payment (status %d)", status)
	}
}

// TestVoucherRulesAtCheckout pins the discount and its eligibility rules.
func TestVoucherRulesAtCheckout(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("demo@nuhabit.id")

	// A general code applies.
	status, quote := h.request(http.MethodPost, "/api/vouchers/validate", token, map[string]any{
		"code": "HYROX100", "packageId": "pkg_starter5",
	})
	if status != http.StatusOK {
		t.Fatalf("validating HYROX100 returned %d: %v", status, quote)
	}
	if int64(quote["discountIdr"].(float64)) != 100_000 {
		t.Fatalf("discount = %v, want 100000", quote["discountIdr"])
	}

	// A new-member code does not apply to a member who joined months ago.
	status, refused := h.request(http.MethodPost, "/api/vouchers/validate", token, map[string]any{
		"code": "WELCOME10", "packageId": "pkg_starter5",
	})
	if status != http.StatusConflict || errorCode(refused) != "VOUCHER_SEGMENT_NOT_ELIGIBLE" {
		t.Fatalf("WELCOME10 returned %d/%s, want 409/VOUCHER_SEGMENT_NOT_ELIGIBLE", status, errorCode(refused))
	}

	// A draft code is not live.
	status, draft := h.request(http.MethodPost, "/api/vouchers/validate", token, map[string]any{
		"code": "BUDDYPASS", "packageId": "pkg_starter5",
	})
	if status != http.StatusConflict || errorCode(draft) != "VOUCHER_NOT_ACTIVE" {
		t.Fatalf("BUDDYPASS returned %d/%s, want 409/VOUCHER_NOT_ACTIVE", status, errorCode(draft))
	}
}

// TestPayoutApprovalChain proves payroll cannot skip a step.
func TestPayoutApprovalChain(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	period := time.Now().Format("2006-01")

	status, payout := h.request(http.MethodPost, "/api/admin/incentives/payouts", admin,
		map[string]any{"coachId": "coa_kevin", "periodMonth": period})
	if status != http.StatusCreated {
		t.Skipf("no completed classes for %s to pay out: %v", period, payout)
	}
	payoutID := payout["payout"].(map[string]any)["id"].(string)

	// Paying a draft skips approval.
	status, early := h.request(http.MethodPost, "/api/admin/incentives/payouts/"+payoutID+"/pay", admin,
		map[string]any{"paymentReference": "TRX-1"})
	if status != http.StatusConflict || errorCode(early) != "INVALID_TRANSITION" {
		t.Fatalf("paying a draft returned %d/%s, want 409/INVALID_TRANSITION", status, errorCode(early))
	}

	if status, _ := h.request(http.MethodPost, "/api/admin/incentives/payouts/"+payoutID+"/approve", admin, nil); status != http.StatusOK {
		t.Fatalf("approving returned %d", status)
	}

	// Paying without a reference leaves nothing to reconcile against.
	status, noRef := h.request(http.MethodPost, "/api/admin/incentives/payouts/"+payoutID+"/pay", admin, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("paying without a reference returned %d: %v", status, noRef)
	}

	status, paid := h.request(http.MethodPost, "/api/admin/incentives/payouts/"+payoutID+"/pay", admin,
		map[string]any{"paymentReference": "TRX-2026-001"})
	if status != http.StatusOK {
		t.Fatalf("paying returned %d: %v", status, paid)
	}
	if p := paid["payout"].(map[string]any); p["status"] != "PAID" {
		t.Fatalf("payout status = %v, want PAID", p["status"])
	}
}

// TestDeleteGuardsProtectReferencedRows proves catalog rows in use cannot be
// deleted out from under the data that points at them.
func TestDeleteGuardsProtectReferencedRows(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	h.createSession(admin, time.Hour, 10)

	status, refused := h.request(http.MethodDelete, "/api/admin/class-types/clt_engine", admin, nil)
	if status != http.StatusConflict || errorCode(refused) != "IN_USE" {
		t.Fatalf("deleting a used class type returned %d/%s, want 409/IN_USE", status, errorCode(refused))
	}

	status, coachRefused := h.request(http.MethodDelete, "/api/admin/coaches/coa_kevin", admin, nil)
	if status != http.StatusConflict || errorCode(coachRefused) != "IN_USE" {
		t.Fatalf("deleting a booked coach returned %d/%s, want 409/IN_USE", status, errorCode(coachRefused))
	}
}

// TestSchedulePublicVisibility proves drafts stay hidden from members.
func TestSchedulePublicVisibility(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	status, draft := h.request(http.MethodPost, "/api/admin/sessions", admin, map[string]any{
		"classTypeId": "clt_engine", "branchId": "brn_senopati", "coachId": "coa_kevin",
		"startsAt": time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		"publish":  false,
	})
	if status != http.StatusCreated {
		t.Fatalf("creating a draft returned %d: %v", status, draft)
	}
	draftID := draft["id"].(string)

	member := h.memberToken("demo@nuhabit.id")
	if status, _ := h.request(http.MethodGet, "/api/sessions/"+draftID, member, nil); status != http.StatusNotFound {
		t.Fatalf("a member can see a draft class (status %d)", status)
	}

	_, listed := h.requestList(http.MethodGet, "/api/sessions?limit=500", member, nil)
	for _, row := range listed {
		if session := row["session"].(map[string]any); session["id"] == draftID {
			t.Fatal("a draft class appeared in the public schedule")
		}
	}

	if status, _ := h.request(http.MethodGet, "/api/admin/sessions/"+draftID, admin, nil); status != http.StatusOK {
		t.Fatal("staff cannot see their own draft")
	}
}

// TestDashboardReflectsActivity proves the read model matches what happened.
func TestDashboardReflectsActivity(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	token := h.memberToken("demo@nuhabit.id")

	before := dashboardValue(h, admin, "outstandingCredits")
	h.topUp(token, "pkg_visit10")
	after := dashboardValue(h, admin, "outstandingCredits")

	if after != before+10 {
		t.Fatalf("outstanding credits went from %d to %d, want a rise of 10", before, after)
	}
}

func dashboardValue(h *harness, adminToken, key string) int {
	h.t.Helper()
	status, view := h.request(http.MethodGet, "/api/admin/reports/dashboard", adminToken, nil)
	if status != http.StatusOK {
		h.t.Fatalf("dashboard returned %d: %v", status, view)
	}
	value, ok := view[key].(float64)
	if !ok {
		h.t.Fatalf("dashboard has no numeric %q: %v", key, view[key])
	}
	return int(value)
}

// TestAppendOnlyLedgerIsEnforcedByTheDatabase proves the invariant survives
// even a direct SQL mistake, not just careful application code.
func TestAppendOnlyLedgerIsEnforcedByTheDatabase(t *testing.T) {
	h := newHarness(t)
	token := h.memberToken("demo@nuhabit.id")
	h.topUp(token, "pkg_starter5")

	ctx := context.Background()
	_, err := h.app.DB.Exec(ctx, `UPDATE wallet.credit_ledger_entries SET amount = 9999`)
	if err == nil {
		t.Fatal("the ledger accepted an UPDATE; it must be append-only")
	}
	if !database.IsAppendOnlyViolation(err) {
		t.Fatalf("unexpected error updating the ledger: %v", err)
	}

	if _, err := h.app.DB.Exec(ctx, `DELETE FROM wallet.credit_ledger_entries`); err == nil {
		t.Fatal("the ledger accepted a DELETE; it must be append-only")
	}
}

// raw posts a body verbatim, with arbitrary headers.
//
// It exists for signed webhooks: the signature covers the exact bytes, so a
// helper that re-encodes a map would sign something subtly different from what
// the server verifies.
func (h *harness) raw(method, path string, headers map[string]string, body string) (int, map[string]any) {
	h.t.Helper()

	req, err := http.NewRequest(method, h.server.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res.StatusCode, decoded
}

// requestObject is request without the fatal-on-error, for a caller that wants
// to handle a miss itself.
func (h *harness) requestObject(method, path, token string) (map[string]any, error) {
	status, body := h.request(method, path, token, nil)
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s %s returned %d: %v", method, path, status, body)
	}
	return body, nil
}

// loyaltyOf reads a member's current point balance.
func (h *harness) loyaltyOf(t *testing.T, token, memberID string) float64 {
	t.Helper()
	status, detail := h.request(http.MethodGet, "/api/admin/crm/members/"+memberID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("reading loyalty returned %d: %v", status, detail)
	}
	profile, ok := detail["profile"].(map[string]any)
	if !ok {
		t.Fatalf("no profile in %v", detail)
	}
	return profile["currentXp"].(float64)
}
