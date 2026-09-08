package scheduling

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Browsing by coach.
//
// The schedule answers "what is on"; this answers "who is teaching". Members
// pick a class by who is running it at least as often as by what it is called,
// and until now the app could only sort by time.

// TrainerCard is one coach on the browse screen.
type TrainerCard struct {
	Coach domain.Coach `json:"coach"`
	// BranchName is where they are based, resolved so the card does not have
	// to look it up.
	BranchName string `json:"branchName"`
	// UpcomingCount is how many classes they have coming up, which is the
	// difference between a coach on the roster and a coach members can book.
	UpcomingCount int `json:"upcomingCount"`
	// NextSessionAt is nil for a coach with nothing scheduled.
	NextSessionAt *time.Time `json:"nextSessionAt"`
	// ClassTypeNames is what they teach, most-taught first.
	ClassTypeNames []string `json:"classTypeNames"`
}

// trainerWindow is how far ahead the browse screen looks. Two weeks is the
// horizon the schedule is published over.
const trainerWindow = 14 * 24 * time.Hour

// Trainers lists the coaches members can book, with what each has coming up.
//
// Coaches with nothing scheduled are kept rather than hidden: a member looking
// for somebody by name should find them and be told they have nothing on,
// rather than be left wondering whether they have left.
func (s *Service) Trainers(ctx context.Context, branchID string) ([]TrainerCard, error) {
	coaches, err := s.catalog.Coaches(ctx)
	if err != nil {
		return nil, err
	}
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return nil, err
	}
	branchNames := make(map[string]string, len(branches))
	for _, b := range branches {
		branchNames[b.ID] = b.Name
	}
	classTypes, err := s.catalog.ClassTypes(ctx, false)
	if err != nil {
		return nil, err
	}
	classTypeNames := make(map[string]string, len(classTypes))
	for _, t := range classTypes {
		classTypeNames[t.ID] = t.Name
	}

	now := s.clock.Now()
	until := now.Add(trainerWindow)
	sessions, err := s.repo.Sessions(ctx, SessionFilter{
		BranchID: branchID,
		From:     &now,
		To:       &until,
		Statuses: []domain.SessionStatus{domain.SessionPublished, domain.SessionFull},
		Limit:    1000,
	})
	if err != nil {
		return nil, err
	}

	type tally struct {
		count   int
		next    *time.Time
		byClass map[string]int
	}
	tallies := map[string]*tally{}
	for _, session := range sessions {
		t, ok := tallies[session.CoachID]
		if !ok {
			t = &tally{byClass: map[string]int{}}
			tallies[session.CoachID] = t
		}
		t.count++
		t.byClass[session.ClassTypeID]++
		if t.next == nil || session.StartsAt.Before(*t.next) {
			startsAt := session.StartsAt
			t.next = &startsAt
		}
	}

	cards := make([]TrainerCard, 0, len(coaches))
	for _, coach := range coaches {
		if coach.Status != domain.StatusActive {
			continue
		}
		if branchID != "" && coach.BranchID != branchID {
			continue
		}
		card := TrainerCard{
			Coach:          coach,
			BranchName:     branchNames[coach.BranchID],
			ClassTypeNames: []string{},
		}
		if t := tallies[coach.ID]; t != nil {
			card.UpcomingCount = t.count
			card.NextSessionAt = t.next
			card.ClassTypeNames = rankClassTypes(t.byClass, classTypeNames)
		}
		cards = append(cards, card)
	}

	// Coaches who are teaching come first, soonest first; the rest fall to the
	// bottom in name order. A browse screen led by somebody with nothing on is
	// a browse screen nobody uses twice.
	sort.SliceStable(cards, func(i, j int) bool {
		a, b := cards[i], cards[j]
		switch {
		case (a.NextSessionAt == nil) != (b.NextSessionAt == nil):
			return b.NextSessionAt == nil
		case a.NextSessionAt != nil && !a.NextSessionAt.Equal(*b.NextSessionAt):
			return a.NextSessionAt.Before(*b.NextSessionAt)
		default:
			return a.Coach.Name < b.Coach.Name
		}
	})
	return cards, nil
}

// rankClassTypes names what a coach teaches, most-taught first.
func rankClassTypes(counts map[string]int, names map[string]string) []string {
	type row struct {
		name  string
		count int
	}
	rows := make([]row, 0, len(counts))
	for classTypeID, count := range counts {
		name, ok := names[classTypeID]
		if !ok {
			continue
		}
		rows = append(rows, row{name: name, count: count})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].name < rows[j].name
	})
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.name)
	}
	return out
}

// TrainerProfile is one coach and everything they have coming up.
type TrainerProfile struct {
	Coach          domain.Coach     `json:"coach"`
	BranchName     string           `json:"branchName"`
	UpcomingCount  int              `json:"upcomingCount"`
	ClassTypeNames []string         `json:"classTypeNames"`
	Upcoming       []SessionSummary `json:"upcoming"`
}

// Trainer is one coach's page: who they are and what they are teaching next.
func (s *Service) Trainer(ctx context.Context, coachID, memberID string) (TrainerProfile, error) {
	coach, err := s.catalog.Coach(ctx, coachID)
	if err != nil {
		return TrainerProfile{}, err
	}
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return TrainerProfile{}, err
	}
	branchName := ""
	for _, b := range branches {
		if b.ID == coach.BranchID {
			branchName = b.Name
		}
	}

	now := s.clock.Now()
	until := now.Add(trainerWindow)
	sessions, err := s.repo.Sessions(ctx, SessionFilter{
		CoachID:  coachID,
		From:     &now,
		To:       &until,
		Statuses: []domain.SessionStatus{domain.SessionPublished, domain.SessionFull},
		Limit:    200,
	})
	if err != nil {
		return TrainerProfile{}, err
	}
	// The member's own bookings come back with the sessions, so their page
	// shows what they have already booked with this coach.
	summaries, err := s.Summaries(ctx, sessions, memberID)
	if err != nil {
		return TrainerProfile{}, err
	}

	classTypes, err := s.catalog.ClassTypes(ctx, false)
	if err != nil {
		return TrainerProfile{}, err
	}
	names := make(map[string]string, len(classTypes))
	for _, t := range classTypes {
		names[t.ID] = t.Name
	}
	counts := map[string]int{}
	for _, session := range sessions {
		counts[session.ClassTypeID]++
	}

	return TrainerProfile{
		Coach:          coach,
		BranchName:     branchName,
		UpcomingCount:  len(sessions),
		ClassTypeNames: rankClassTypes(counts, names),
		Upcoming:       summaries,
	}, nil
}

// ── HTTP ─────────────────────────────────────────────────────────────────────

func (h *Handler) listTrainers(w http.ResponseWriter, r *http.Request) {
	trainers, err := h.service.Trainers(r.Context(), httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, trainers)
}

func (h *Handler) trainer(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.Trainer(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, profile)
}
