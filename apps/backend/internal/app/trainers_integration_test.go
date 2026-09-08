package app_test

import (
	"net/http"
	"testing"
	"time"
)

// Browsing by who is teaching, rather than by when.
func TestTrainersListWhoIsTeaching(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")

	// Two classes for Kevin, one for somebody else.
	h.createSession(admin, 2*time.Hour, 10)
	h.createSession(admin, 26*time.Hour, 10)

	status, trainers := h.requestList(http.MethodGet, "/api/coaches", "", nil)
	if status != http.StatusOK {
		t.Fatalf("listing trainers returned %d: %v", status, trainers)
	}
	if len(trainers) == 0 {
		t.Fatal("no trainers came back")
	}

	var kevin map[string]any
	for _, row := range trainers {
		coach := row["coach"].(map[string]any)
		if coach["id"] == "coa_kevin" {
			kevin = row
		}
	}
	if kevin == nil {
		t.Fatalf("the coach teaching the seeded classes is missing from %v", trainers)
	}
	if kevin["upcomingCount"].(float64) < 2 {
		t.Fatalf("Kevin has %v classes coming up, want at least the two just created", kevin["upcomingCount"])
	}
	if kevin["nextSessionAt"] == nil {
		t.Fatal("a coach with classes has no next class")
	}
	if names, ok := kevin["classTypeNames"].([]any); !ok || len(names) == 0 {
		t.Fatalf("Kevin teaches nothing: %v", kevin["classTypeNames"])
	}
	if kevin["branchName"] == "" {
		t.Fatal("the trainer card does not say where they are based")
	}

	// A coach who is teaching sorts above one who is not.
	first := trainers[0]["coach"].(map[string]any)
	if trainers[0]["nextSessionAt"] == nil {
		t.Fatalf("the list is led by %v, who has nothing on", first["name"])
	}
}

// A trainer's page is their classes, and the member's own bookings among them.
func TestTrainerPageShowsTheirClasses(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	sessionID := h.createSession(admin, 3*time.Hour, 10)

	member := h.memberToken("demo@nuhabit.id")
	h.topUp(member, "pkg_starter5")
	if status, body := h.request(http.MethodPost, "/api/sessions/"+sessionID+"/book", member, nil); status != http.StatusCreated {
		t.Fatalf("booking returned %d: %v", status, body)
	}

	status, profile := h.request(http.MethodGet, "/api/coaches/coa_kevin", member, nil)
	if status != http.StatusOK {
		t.Fatalf("the trainer page returned %d: %v", status, profile)
	}
	coach := profile["coach"].(map[string]any)
	if coach["name"] == "" {
		t.Fatal("the trainer page has no name on it")
	}

	upcoming := profile["upcoming"].([]any)
	if len(upcoming) == 0 {
		t.Fatal("a coach with a published class has nothing upcoming")
	}
	booked := false
	for _, row := range upcoming {
		summary := row.(map[string]any)
		if summary["session"].(map[string]any)["id"] == sessionID {
			if summary["myBooking"] == nil {
				t.Fatal("the member's own booking is missing from the trainer's page")
			}
			booked = true
		}
	}
	if !booked {
		t.Fatal("the class that was just booked is not on the trainer's page")
	}
}

// The schedule filters by coach, which is what the app's trainer chip sends.
func TestSessionsFilterByTrainer(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken("adm_super")
	h.createSession(admin, 4*time.Hour, 10)

	status, mine := h.requestList(http.MethodGet, "/api/sessions?coachId=coa_kevin", "", nil)
	if status != http.StatusOK {
		t.Fatalf("filtering by coach returned %d", status)
	}
	if len(mine) == 0 {
		t.Fatal("filtering by the coach who teaches them returned nothing")
	}
	for _, row := range mine {
		if got := row["session"].(map[string]any)["coachId"]; got != "coa_kevin" {
			t.Fatalf("the filter let through a class taught by %v", got)
		}
	}

	// A coach with nothing on returns an empty list, not everybody's classes.
	status, others := h.requestList(http.MethodGet, "/api/sessions?coachId=coa_nobody", "", nil)
	if status != http.StatusOK || len(others) != 0 {
		t.Fatalf("an unknown coach returned %d with %d classes", status, len(others))
	}
}

// An unknown coach is a 404, not an empty page pretending to be somebody.
func TestUnknownTrainerIsNotFound(t *testing.T) {
	h := newHarness(t)
	if status, _ := h.request(http.MethodGet, "/api/coaches/coa_nobody", "", nil); status != http.StatusNotFound {
		t.Fatalf("an unknown coach returned %d, want 404", status)
	}
}
