package training

import (
	"net/http"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// The workout generator, the player that runs one, and the race calendar.

func (h *Handler) mountWorkouts(r *httpx.Router) {
	member := h.guard.RequireMember

	r.Post("/api/workouts/generate", h.generateWorkout, member)
	r.Get("/api/workouts/{id}", h.workout, member)
	r.Post("/api/workouts/{id}/replace", h.replaceBlock, member)
	r.Post("/api/workouts/{id}/start", h.startSession, member)

	r.Get("/api/workout-sessions", h.sessions, member)
	r.Get("/api/workout-sessions/{id}", h.session, member)
	r.Post("/api/workout-sessions/{id}/block", h.recordBlock, member)
	r.Post("/api/workout-sessions/{id}/pause", h.pauseSession, member)
	r.Post("/api/workout-sessions/{id}/resume", h.resumeSession, member)
	r.Post("/api/workout-sessions/{id}/finish", h.finishSession, member)

	r.Get("/api/races", h.races, member)
	r.Get("/api/races/{id}", h.race, member)
	r.Post("/api/races/{id}/register", h.registerForRace, member)
	r.Get("/api/me/races", h.myRaces, member)
	r.Patch("/api/me/races/{id}", h.updateMyRace, member)
}

// ── Generating ───────────────────────────────────────────────────────────────

type generateRequest struct {
	Type                string   `json:"type"`
	Division            string   `json:"division"`
	StationOrders       []int    `json:"stationOrders"`
	ExcludedExerciseIDs []string `json:"excludedExerciseIds"`
}

func (g *generateRequest) Validate() error {
	if !domain.IsValidWorkoutType(g.Type) {
		return httpx.Invalid("Pick a workout format.")
	}
	if !domain.IsValidDivision(g.Division) {
		return httpx.Invalid("Pick a division.")
	}
	return nil
}

func (h *Handler) generateWorkout(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[generateRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	workout, err := h.service.GenerateWorkout(r.Context(), auth.MemberID(r.Context()), GenerateInput{
		Type:                domain.WorkoutType(body.Type),
		Division:            domain.Division(body.Division),
		StationOrders:       body.StationOrders,
		ExcludedExerciseIDs: body.ExcludedExerciseIDs,
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, workout)
}

func (h *Handler) workout(w http.ResponseWriter, r *http.Request) {
	workout, err := h.service.Workout(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, workout)
}

type replaceBlockRequest struct {
	Order      int    `json:"order"`
	ExerciseID string `json:"exerciseId"`
}

func (b *replaceBlockRequest) Validate() error {
	if b.Order < 1 {
		return httpx.Invalid("Which block is being swapped?")
	}
	if b.ExerciseID == "" {
		return httpx.Invalid("Swap it for what?")
	}
	return nil
}

func (h *Handler) replaceBlock(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[replaceBlockRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	workout, err := h.service.ReplaceBlock(r.Context(), httpx.Param(r, "id"),
		auth.MemberID(r.Context()), body.Order, body.ExerciseID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, workout)
}

// ── The player ───────────────────────────────────────────────────────────────

type sessionView struct {
	Session       domain.WorkoutSession   `json:"session"`
	Workout       domain.GeneratedWorkout `json:"workout"`
	ActiveSec     int                     `json:"activeSec"`
	CompletionPct int                     `json:"completionPct"`
}

func (h *Handler) sessionView(r *http.Request, session domain.WorkoutSession) (sessionView, error) {
	workout, err := h.service.repo.Workout(r.Context(), session.WorkoutID)
	if err != nil {
		return sessionView{}, err
	}
	return sessionView{
		Session:       session,
		Workout:       workout,
		ActiveSec:     domain.SessionActiveSec(session.BlockResults),
		CompletionPct: domain.SessionCompletionPct(session.BlockResults, len(workout.Blocks)),
	}, nil
}

// respondWithSession is the answer to every action in the player: the session
// as it now stands, so the screen never has to guess what changed.
func (h *Handler) respondWithSession(w http.ResponseWriter, r *http.Request, session domain.WorkoutSession, status int) {
	view, err := h.sessionView(r, session)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, status, view)
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.StartSession(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.respondWithSession(w, r, session, http.StatusCreated)
}

func (h *Handler) sessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessions, err := h.service.Sessions(ctx, auth.MemberID(ctx))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.WorkoutID)
	}
	workouts, err := h.service.repo.WorkoutsByIDs(ctx, ids)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	type historyItem struct {
		Session       domain.WorkoutSession `json:"session"`
		WorkoutType   domain.WorkoutType    `json:"workoutType"`
		Division      domain.Division       `json:"division"`
		TotalBlocks   int                   `json:"totalBlocks"`
		ActiveSec     int                   `json:"activeSec"`
		CompletionPct int                   `json:"completionPct"`
	}

	out := make([]historyItem, 0, len(sessions))
	for _, session := range sessions {
		workout, ok := workouts[session.WorkoutID]
		if !ok {
			continue
		}
		out = append(out, historyItem{
			Session:       session,
			WorkoutType:   workout.Type,
			Division:      workout.Division,
			TotalBlocks:   len(workout.Blocks),
			ActiveSec:     domain.SessionActiveSec(session.BlockResults),
			CompletionPct: domain.SessionCompletionPct(session.BlockResults, len(workout.Blocks)),
		})
	}
	httpx.OK(w, out)
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.Session(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.respondWithSession(w, r, session, http.StatusOK)
}

type blockResultRequest struct {
	Order       int `json:"order"`
	DurationSec int `json:"durationSec"`
}

func (b *blockResultRequest) Validate() error {
	if b.Order < 1 {
		return httpx.Invalid("Which block was that?")
	}
	if b.DurationSec < 0 {
		return httpx.Invalid("A block cannot take less than no time.")
	}
	return nil
}

func (h *Handler) recordBlock(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[blockResultRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	session, err := h.service.RecordBlock(r.Context(), httpx.Param(r, "id"),
		auth.MemberID(r.Context()), body.Order, body.DurationSec)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.respondWithSession(w, r, session, http.StatusOK)
}

func (h *Handler) pauseSession(w http.ResponseWriter, r *http.Request) {
	session, err := h.service.PauseSession(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.respondWithSession(w, r, session, http.StatusOK)
}

type resumeRequest struct {
	PausedSec int `json:"pausedSec"`
}

func (h *Handler) resumeSession(w http.ResponseWriter, r *http.Request) {
	// The phone reports how long the pause lasted; the body is optional
	// because an app that was killed mid-pause cannot say.
	body, err := httpx.Decode[resumeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	session, err := h.service.ResumeSession(r.Context(), httpx.Param(r, "id"),
		auth.MemberID(r.Context()), body.PausedSec)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.respondWithSession(w, r, session, http.StatusOK)
}

type finishRequest struct {
	Partial bool `json:"partial"`
}

func (h *Handler) finishSession(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[finishRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.service.FinishSession(r.Context(), httpx.Param(r, "id"),
		auth.MemberID(r.Context()), body.Partial)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	view, err := h.sessionView(r, result.Session)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// The activity id comes back so the app can offer "see it in your log".
	var activityID *string
	if result.Activity != nil {
		activityID = &result.Activity.ID
	}
	httpx.OK(w, struct {
		sessionView
		ActivityID *string `json:"activityId"`
	}{sessionView: view, ActivityID: activityID})
}

// ── Races ────────────────────────────────────────────────────────────────────

type raceRow struct {
	Event            domain.RaceEvent `json:"event"`
	Joined           bool             `json:"joined"`
	ParticipantCount int              `json:"participantCount"`
}

func (h *Handler) races(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	events, err := h.service.Races(ctx, RaceFilter{
		Region:  r.URL.Query().Get("region"),
		Results: r.URL.Query().Get("scope") == "results",
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, joined, err := h.service.EntryCounts(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	out := make([]raceRow, 0, len(events))
	for _, event := range events {
		out = append(out, raceRow{
			Event: event, Joined: joined[event.ID], ParticipantCount: counts[event.ID],
		})
	}
	httpx.OK(w, out)
}

func (h *Handler) race(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	event, err := h.service.Race(ctx, httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, joined, err := h.service.EntryCounts(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// The member's own entry, if they have one, so the screen can show their
	// division and goal without a second request.
	var myRace any
	entries, err := h.service.repo.UserRaces(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for _, entry := range entries {
		if entry.RaceEventID == event.ID {
			myRace = map[string]any{"userRace": entry, "event": event}
			break
		}
	}

	httpx.OK(w, map[string]any{
		"view": raceRow{
			Event: event, Joined: joined[event.ID], ParticipantCount: counts[event.ID],
		},
		"myRace": myRace,
	})
}

type registerRaceRequest struct {
	Division string `json:"division"`
	GoalSec  *int   `json:"goalSec"`
}

func (r *registerRaceRequest) Validate() error {
	if !domain.IsValidDivision(r.Division) {
		return httpx.Invalid("Pick a division.")
	}
	return nil
}

func (h *Handler) registerForRace(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[registerRaceRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entry, err := h.service.Register(r.Context(), auth.MemberID(r.Context()), httpx.Param(r, "id"),
		RegisterInput{Division: domain.Division(body.Division), GoalSec: body.GoalSec})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, entry)
}

func (h *Handler) myRaces(w http.ResponseWriter, r *http.Request) {
	races, err := h.service.MyRaces(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	type myRaceView struct {
		UserRace        domain.UserRace      `json:"userRace"`
		Event           domain.RaceEvent     `json:"event"`
		DaysToRace      int                  `json:"daysToRace"`
		PredictionSec   *int                 `json:"predictionSec"`
		ReadinessScore  int                  `json:"readinessScore"`
		SimulationCount int                  `json:"simulationCount"`
		Analysis        *domain.RaceAnalysis `json:"analysis"`
	}

	out := make([]myRaceView, 0, len(races))
	for _, race := range races {
		out = append(out, myRaceView{
			UserRace: race.Entry, Event: race.Event, DaysToRace: race.DaysToRace,
			PredictionSec: race.PredictionSec, ReadinessScore: race.ReadinessScore,
			SimulationCount: race.SimulationCount, Analysis: race.Analysis,
		})
	}
	httpx.OK(w, out)
}

type updateUserRaceRequest struct {
	Division  *string `json:"division"`
	GoalSec   **int   `json:"goalSec"`
	Status    *string `json:"status"`
	ResultSec **int   `json:"resultSec"`
}

func (u *updateUserRaceRequest) Validate() error {
	if u.Division != nil && !domain.IsValidDivision(*u.Division) {
		return httpx.Invalid("That is not a division.")
	}
	return nil
}

func (h *Handler) updateMyRace(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[updateUserRaceRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	patch := UserRacePatch{}
	if body.Division != nil {
		division := domain.Division(*body.Division)
		patch.Division = &division
	}
	if body.Status != nil {
		status := domain.UserRaceStatus(*body.Status)
		patch.Status = &status
	}
	if body.GoalSec != nil {
		patch.SetGoal = true
		patch.GoalSec = *body.GoalSec
	}
	if body.ResultSec != nil {
		patch.SetResult = true
		patch.ResultSec = *body.ResultSec
	}

	entry, err := h.service.UpdateUserRace(r.Context(), httpx.Param(r, "id"),
		auth.MemberID(r.Context()), patch)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, entry)
}

// ── The race calendar, as the studio maintains it ────────────────────────────

// MountAdmin adds the studio's side of the race calendar. It is separate from
// Mount because races are the one part of training a member does not own:
// somebody at the studio keeps the calendar, everybody else reads it.
func (h *Handler) MountAdmin(r *httpx.Router) {
	manage := h.guard.RequireAdmin(string(domain.PermCampaignsManage))
	view := h.guard.RequireAdmin(string(domain.PermEngagementView))

	r.Get("/api/admin/races", h.adminRaces, view)
	r.Post("/api/admin/races", h.createRace, manage)
	r.Patch("/api/admin/races/{id}", h.updateRace, manage)
	r.Delete("/api/admin/races/{id}", h.deleteRace, manage)
}

type adminRace struct {
	domain.RaceEvent
	Participants int `json:"participants"`
}

func (h *Handler) adminRaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Both scopes, because the panel maintains the whole calendar — including
	// the races that have already happened.
	upcoming, err := h.service.Races(ctx, RaceFilter{})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	past, err := h.service.Races(ctx, RaceFilter{Results: true})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, _, err := h.service.EntryCounts(ctx, "")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	out := make([]adminRace, 0, len(upcoming)+len(past))
	for _, event := range append(upcoming, past...) {
		out = append(out, adminRace{RaceEvent: event, Participants: counts[event.ID]})
	}
	httpx.OK(w, out)
}

type raceRequest struct {
	Name            string     `json:"name"`
	Country         string     `json:"country"`
	Region          string     `json:"region"`
	City            string     `json:"city"`
	Venue           string     `json:"venue"`
	StartsAt        time.Time  `json:"startsAt"`
	EndsAt          *time.Time `json:"endsAt"`
	RegistrationURL string     `json:"registrationUrl"`
	ImageURL        *string    `json:"imageUrl"`
	Status          string     `json:"status"`
}

func (rr *raceRequest) Validate() error {
	if len(strings.TrimSpace(rr.Name)) < 2 {
		return httpx.Invalid("A race needs a name.")
	}
	if !domain.IsValidRaceRegion(rr.Region) {
		return httpx.Invalid("Pick a region.")
	}
	if rr.StartsAt.IsZero() {
		return httpx.Invalid("When is it?")
	}
	if rr.EndsAt != nil && rr.EndsAt.Before(rr.StartsAt) {
		return httpx.Invalid("A race cannot end before it starts.")
	}
	return nil
}

func (h *Handler) saveRace(w http.ResponseWriter, r *http.Request, raceID string, status int) {
	body, err := httpx.Decode[raceRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	event, err := h.service.SaveRace(r.Context(), raceID, RaceInput{
		Name: body.Name, Country: body.Country, Region: domain.RaceRegion(body.Region),
		City: body.City, Venue: body.Venue, StartsAt: body.StartsAt, EndsAt: body.EndsAt,
		RegistrationURL: body.RegistrationURL, ImageURL: body.ImageURL,
		Status: domain.RaceStatus(body.Status),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, _, err := h.service.EntryCounts(r.Context(), "")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, status, adminRace{RaceEvent: event, Participants: counts[event.ID]})
}

func (h *Handler) createRace(w http.ResponseWriter, r *http.Request) {
	h.saveRace(w, r, "", http.StatusCreated)
}

func (h *Handler) updateRace(w http.ResponseWriter, r *http.Request) {
	h.saveRace(w, r, httpx.Param(r, "id"), http.StatusOK)
}

func (h *Handler) deleteRace(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteRace(r.Context(), httpx.Param(r, "id")); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}
