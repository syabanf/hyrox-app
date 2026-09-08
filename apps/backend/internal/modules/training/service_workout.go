package training

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// ── Generating a workout ─────────────────────────────────────────────────────

// GenerateInput is what the member chose on the generator screen.
type GenerateInput struct {
	Type                domain.WorkoutType
	Division            domain.Division
	StationOrders       []int
	ExcludedExerciseIDs []string
}

func (s *Service) GenerateWorkout(ctx context.Context, memberID string, in GenerateInput) (domain.GeneratedWorkout, error) {
	if !domain.IsValidWorkoutType(string(in.Type)) {
		return domain.GeneratedWorkout{}, httpx.Invalid("That is not a workout type.")
	}
	if !domain.IsValidDivision(string(in.Division)) {
		return domain.GeneratedWorkout{}, httpx.Invalid("That is not a division.")
	}
	for _, order := range in.StationOrders {
		if order < 1 || order > 8 {
			return domain.GeneratedWorkout{}, httpx.Invalid("Stations are numbered 1 to 8.")
		}
	}

	exercises, err := s.library.Exercises(ctx)
	if err != nil {
		return domain.GeneratedWorkout{}, err
	}
	substitutions, err := s.library.Substitutions(ctx)
	if err != nil {
		return domain.GeneratedWorkout{}, err
	}

	blocks, total := domain.GenerateWorkout(domain.GenerateWorkoutArgs{
		Type:                in.Type,
		Division:            in.Division,
		StationOrders:       in.StationOrders,
		ExcludedExerciseIDs: in.ExcludedExerciseIDs,
		Exercises:           exercises,
		Substitutions:       substitutions,
		Pick:                randomPick,
	})
	if len(blocks) == 0 {
		return domain.GeneratedWorkout{}, httpx.Conflict("NO_STATIONS",
			"There are no stations to build a workout from.")
	}
	if in.ExcludedExerciseIDs == nil {
		in.ExcludedExerciseIDs = []string{}
	}

	return s.repo.InsertWorkout(ctx, domain.GeneratedWorkout{
		ID: s.ids.New(id.Workout), MemberID: memberID,
		Type: in.Type, Division: in.Division, Blocks: blocks,
		ExcludedExerciseIDs: in.ExcludedExerciseIDs, TotalTargetSec: total,
	})
}

// randomPick draws from a cryptographic source. Nothing here needs to be
// unguessable; it is simply the source that is already imported everywhere and
// cannot silently repeat a sequence across restarts.
func randomPick(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func (s *Service) Workout(ctx context.Context, workoutID, memberID string) (domain.GeneratedWorkout, error) {
	workout, err := s.repo.Workout(ctx, workoutID)
	if err != nil {
		return domain.GeneratedWorkout{}, err
	}
	if workout.MemberID != memberID {
		return domain.GeneratedWorkout{}, httpx.NotFound("workout")
	}
	return workout, nil
}

// ReplaceBlock swaps one block's exercise for another — the member has no
// sled today, or wants to work something else.
func (s *Service) ReplaceBlock(ctx context.Context, workoutID, memberID string, order int, exerciseID string) (domain.GeneratedWorkout, error) {
	workout, err := s.Workout(ctx, workoutID, memberID)
	if err != nil {
		return domain.GeneratedWorkout{}, err
	}
	exercises, err := s.library.Exercises(ctx)
	if err != nil {
		return domain.GeneratedWorkout{}, err
	}

	var replacement *domain.Exercise
	for i, e := range exercises {
		if e.ID == exerciseID {
			replacement = &exercises[i]
			break
		}
	}
	if replacement == nil {
		return domain.GeneratedWorkout{}, httpx.NotFound("exercise")
	}

	found := false
	for i, block := range workout.Blocks {
		if block.Order != order {
			continue
		}
		if block.Kind != "STATION" {
			return domain.GeneratedWorkout{}, httpx.Conflict("NOT_A_STATION",
				"Running is the one block that cannot be swapped.")
		}
		workout.Blocks[i] = domain.ReplaceBlockExercise(block, *replacement)
		found = true
		break
	}
	if !found {
		return domain.GeneratedWorkout{}, httpx.NotFound("block")
	}
	return s.repo.SaveWorkoutBlocks(ctx, workout.ID, workout.Blocks, workout.TotalTargetSec)
}

// ── Running a session ────────────────────────────────────────────────────────

// StartSession begins a workout. Starting one twice makes two sessions on
// purpose: a second attempt at the same workout is a second result.
func (s *Service) StartSession(ctx context.Context, workoutID, memberID string) (domain.WorkoutSession, error) {
	if _, err := s.Workout(ctx, workoutID, memberID); err != nil {
		return domain.WorkoutSession{}, err
	}
	now := s.clock.Now()
	return s.repo.InsertSession(ctx, domain.WorkoutSession{
		ID: s.ids.New(id.WorkoutRun), WorkoutID: workoutID, MemberID: memberID,
		Status: domain.WorkoutStarted, CurrentBlock: 1, StartedAt: &now,
		BlockResults: []domain.WorkoutBlockResult{},
	})
}

func (s *Service) Session(ctx context.Context, sessionID, memberID string) (domain.WorkoutSession, error) {
	session, err := s.repo.Session(ctx, sessionID)
	if err != nil {
		return domain.WorkoutSession{}, err
	}
	if session.MemberID != memberID {
		return domain.WorkoutSession{}, httpx.NotFound("session")
	}
	return session, nil
}

func (s *Service) Sessions(ctx context.Context, memberID string) ([]domain.WorkoutSession, error) {
	return s.repo.SessionsForMember(ctx, memberID)
}

// RecordBlock stores how long a block took and moves the session on.
//
// A repeat for a block already recorded overwrites it rather than appending:
// the player retries a block when somebody fumbles the phone, and two results
// for one block would put the completion percentage above a hundred.
func (s *Service) RecordBlock(ctx context.Context, sessionID, memberID string, order, durationSec int) (domain.WorkoutSession, error) {
	if durationSec < 0 {
		return domain.WorkoutSession{}, httpx.Invalid("A block cannot take less than no time.")
	}
	session, err := s.Session(ctx, sessionID, memberID)
	if err != nil {
		return domain.WorkoutSession{}, err
	}
	if session.Status != domain.WorkoutStarted && session.Status != domain.WorkoutPaused {
		return domain.WorkoutSession{}, httpx.Conflict("SESSION_FINISHED",
			"That session has already finished.")
	}
	workout, err := s.repo.Workout(ctx, session.WorkoutID)
	if err != nil {
		return domain.WorkoutSession{}, err
	}
	if order < 1 || order > len(workout.Blocks) {
		return domain.WorkoutSession{}, httpx.Invalid("That workout has no block %d.", order)
	}

	replaced := false
	for i, result := range session.BlockResults {
		if result.Order == order {
			session.BlockResults[i].DurationSec = durationSec
			replaced = true
			break
		}
	}
	if !replaced {
		session.BlockResults = append(session.BlockResults,
			domain.WorkoutBlockResult{Order: order, DurationSec: durationSec})
	}
	if next := order + 1; next > session.CurrentBlock {
		session.CurrentBlock = next
	}
	return s.repo.SaveSession(ctx, session)
}

// PauseSession stops the clock. The pause count is kept because a session
// paused eleven times is a different session from one run straight through.
func (s *Service) PauseSession(ctx context.Context, sessionID, memberID string) (domain.WorkoutSession, error) {
	session, err := s.Session(ctx, sessionID, memberID)
	if err != nil {
		return domain.WorkoutSession{}, err
	}
	next, err := domain.Transition(domain.WorkoutSessionTransitions, session.Status, domain.WorkoutPaused)
	if err != nil {
		return domain.WorkoutSession{}, httpx.Conflict("INVALID_TRANSITION",
			"A %s session cannot be paused.", lower(string(session.Status)))
	}
	session.Status = next
	session.PauseCount++
	return s.repo.SaveSession(ctx, session)
}

// ResumeSession restarts the clock, adding however long the pause lasted.
func (s *Service) ResumeSession(ctx context.Context, sessionID, memberID string, pausedSec int) (domain.WorkoutSession, error) {
	session, err := s.Session(ctx, sessionID, memberID)
	if err != nil {
		return domain.WorkoutSession{}, err
	}
	next, err := domain.Transition(domain.WorkoutSessionTransitions, session.Status, domain.WorkoutStarted)
	if err != nil {
		return domain.WorkoutSession{}, httpx.Conflict("INVALID_TRANSITION",
			"A %s session cannot be resumed.", lower(string(session.Status)))
	}
	session.Status = next
	if pausedSec > 0 {
		session.TotalPauseSec += pausedSec
	}
	return s.repo.SaveSession(ctx, session)
}

// FinishResult is a finished session and the activity it produced.
type FinishResult struct {
	Session  domain.WorkoutSession
	Activity *domain.Activity
}

// FinishSession ends a session.
//
// A completed session becomes an activity as well, so a HYROX simulation shows
// up in the training log and the weekly totals beside everything else. A
// partial one does not: the point of stopping halfway is that it did not
// happen.
func (s *Service) FinishSession(ctx context.Context, sessionID, memberID string, partial bool) (FinishResult, error) {
	session, err := s.Session(ctx, sessionID, memberID)
	if err != nil {
		return FinishResult{}, err
	}
	target := domain.WorkoutCompleted
	if partial {
		target = domain.WorkoutPartial
	}
	next, err := domain.Transition(domain.WorkoutSessionTransitions, session.Status, target)
	if err != nil {
		return FinishResult{}, httpx.Conflict("INVALID_TRANSITION",
			"A %s session cannot be finished.", lower(string(session.Status)))
	}

	now := s.clock.Now()
	session.Status = next
	session.EndedAt = &now
	saved, err := s.repo.SaveSession(ctx, session)
	if err != nil {
		return FinishResult{}, err
	}
	if partial {
		return FinishResult{Session: saved}, nil
	}

	workout, err := s.repo.Workout(ctx, saved.WorkoutID)
	if err != nil {
		return FinishResult{}, err
	}
	activeSec := domain.SessionActiveSec(saved.BlockResults)
	startedAt := saved.CreatedAt
	if saved.StartedAt != nil {
		startedAt = *saved.StartedAt
	}
	// The distance is the running in the workout: the stations are work, but
	// they are not kilometres, and counting them as such would inflate every
	// weekly total.
	distanceM := 0.0
	for _, block := range workout.Blocks {
		if block.Kind == "RUN" && block.DistanceM != nil {
			distanceM += float64(*block.DistanceM)
		}
	}

	activity, err := s.repo.InsertActivity(ctx, domain.Activity{
		ID:         s.ids.New(id.Activity),
		MemberID:   memberID,
		Type:       domain.ActivityWorkout,
		Title:      workoutTitle(workout.Type),
		StartedAt:  startedAt,
		ElapsedSec: activeSec + saved.TotalPauseSec,
		MovingSec:  activeSec,
		DistanceM:  distanceM,
		Points:     []domain.TrackPoint{},
		Photos:     []string{},
		Visibility: domain.VisibilityEveryone,
	})
	if err != nil {
		return FinishResult{}, err
	}
	return FinishResult{Session: saved, Activity: &activity}, nil
}

func workoutTitle(t domain.WorkoutType) string {
	switch t {
	case domain.WorkoutFullSimulation:
		return "HYROX simulation"
	case domain.WorkoutCoverage:
		return "HYROX coverage session"
	case domain.WorkoutQuick:
		return "Quick HYROX session"
	default:
		return "HYROX station practice"
	}
}

// ── Races ────────────────────────────────────────────────────────────────────

func (s *Service) Races(ctx context.Context, filter RaceFilter) ([]domain.RaceEvent, error) {
	if filter.Region != "" && !domain.IsValidRaceRegion(filter.Region) {
		return nil, httpx.Invalid("That is not a region.")
	}
	return s.repo.Races(ctx, filter)
}

func (s *Service) Race(ctx context.Context, id string) (domain.RaceEvent, error) {
	return s.repo.Race(ctx, id)
}

func (s *Service) EntryCounts(ctx context.Context, memberID string) (map[string]int, map[string]bool, error) {
	return s.repo.EntryCounts(ctx, memberID)
}

// RegisterInput is a member entering a race.
type RegisterInput struct {
	Division domain.Division
	GoalSec  *int
}

// Register enters a member for a race.
//
// An entry that was cancelled is revived rather than duplicated: the unique
// index only covers live entries, so a second row would be legal and wrong.
func (s *Service) Register(ctx context.Context, memberID, raceID string, in RegisterInput) (domain.UserRace, error) {
	if !domain.IsValidDivision(string(in.Division)) {
		return domain.UserRace{}, httpx.Invalid("That is not a division.")
	}
	if in.GoalSec != nil && *in.GoalSec <= 0 {
		return domain.UserRace{}, httpx.Invalid("A goal time has to be a time.")
	}
	race, err := s.repo.Race(ctx, raceID)
	if err != nil {
		return domain.UserRace{}, err
	}
	if race.Status == domain.RaceCancelled {
		return domain.UserRace{}, httpx.Conflict("RACE_CANCELLED", "That race has been cancelled.")
	}

	return s.repo.InsertUserRace(ctx, domain.UserRace{
		ID: s.ids.New(id.UserRace), MemberID: memberID, RaceEventID: raceID,
		Division: in.Division, GoalSec: in.GoalSec, Status: domain.UserRaceTraining,
	})
}

// UserRacePatch is a member updating their entry.
type UserRacePatch struct {
	Division  *domain.Division
	GoalSec   *int
	SetGoal   bool
	Status    *domain.UserRaceStatus
	ResultSec *int
	SetResult bool
}

func (s *Service) UpdateUserRace(ctx context.Context, entryID, memberID string, patch UserRacePatch) (domain.UserRace, error) {
	entry, err := s.repo.UserRace(ctx, entryID)
	if err != nil {
		return domain.UserRace{}, err
	}
	if entry.MemberID != memberID {
		return domain.UserRace{}, httpx.NotFound("entry")
	}

	if patch.Division != nil {
		if !domain.IsValidDivision(string(*patch.Division)) {
			return domain.UserRace{}, httpx.Invalid("That is not a division.")
		}
		entry.Division = *patch.Division
	}
	if patch.SetGoal {
		if patch.GoalSec != nil && *patch.GoalSec <= 0 {
			return domain.UserRace{}, httpx.Invalid("A goal time has to be a time.")
		}
		entry.GoalSec = patch.GoalSec
	}
	if patch.SetResult {
		if patch.ResultSec != nil && *patch.ResultSec <= 0 {
			return domain.UserRace{}, httpx.Invalid("A result has to be a time.")
		}
		entry.ResultSec = patch.ResultSec
	}
	if patch.Status != nil && *patch.Status != entry.Status {
		next, err := domain.Transition(domain.UserRaceTransitions, entry.Status, *patch.Status)
		if err != nil {
			return domain.UserRace{}, httpx.Conflict("INVALID_TRANSITION",
				"An entry that is %s cannot become %s.", lower(string(entry.Status)), lower(string(*patch.Status)))
		}
		entry.Status = next
	}
	// Recording a result is what makes an entry raced; asking somebody to say
	// so twice is a form to fill in for no reason.
	if entry.ResultSec != nil && entry.Status == domain.UserRaceTraining {
		entry.Status = domain.UserRaceRaced
	}
	return s.repo.SaveUserRace(ctx, entry)
}

// MyRaces is the member's entries with everything the race screen shows.
type MyRace struct {
	Entry           domain.UserRace
	Event           domain.RaceEvent
	DaysToRace      int
	PredictionSec   *int
	ReadinessScore  int
	SimulationCount int
	Analysis        *domain.RaceAnalysis
}

func (s *Service) MyRaces(ctx context.Context, memberID string) ([]MyRace, error) {
	entries, err := s.repo.UserRaces(ctx, memberID)
	if err != nil {
		return nil, err
	}
	prediction, simulations, err := s.racePrediction(ctx, memberID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	dates, err := s.repo.ActivityDates(ctx, memberID, now.Add(-90*24*time.Hour))
	if err != nil {
		return nil, err
	}
	readiness := domain.RaceReadinessScore(dates, now)

	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.RaceEventID)
	}
	events, err := s.repo.RacesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]MyRace, 0, len(entries))
	for _, entry := range entries {
		event, ok := events[entry.RaceEventID]
		if !ok {
			continue
		}
		row := MyRace{
			Entry: entry, Event: event,
			DaysToRace:      int(math.Ceil(event.StartsAt.Sub(now).Hours() / 24)),
			PredictionSec:   prediction,
			ReadinessScore:  readiness,
			SimulationCount: simulations,
		}
		if entry.ResultSec != nil {
			analysis := domain.AnalyzeRace(*entry.ResultSec, entry.GoalSec, prediction)
			row.Analysis = &analysis
		}
		out = append(out, row)
	}
	sortMyRaces(out)
	return out, nil
}

// racePrediction is the member's likely finishing time, from their completed
// full simulations.
func (s *Service) racePrediction(ctx context.Context, memberID string) (*int, int, error) {
	sessions, err := s.repo.SessionsForMember(ctx, memberID)
	if err != nil {
		return nil, 0, err
	}
	ids := []string{}
	for _, session := range sessions {
		if session.Status == domain.WorkoutCompleted {
			ids = append(ids, session.WorkoutID)
		}
	}
	workouts, err := s.repo.WorkoutsByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	secs := []int{}
	for _, session := range sessions {
		if session.Status != domain.WorkoutCompleted {
			continue
		}
		if workout, ok := workouts[session.WorkoutID]; ok && workout.Type == domain.WorkoutFullSimulation {
			secs = append(secs, domain.SessionActiveSec(session.BlockResults))
		}
	}
	return domain.PredictRaceSec(secs), len(secs), nil
}

func sortMyRaces(races []MyRace) {
	sort.SliceStable(races, func(i, j int) bool {
		return races[i].Event.StartsAt.Before(races[j].Event.StartsAt)
	})
}

// lower turns a status into something that reads in a sentence: "a paused
// session", not "a PAUSED session".
func lower(status string) string {
	return strings.ToLower(strings.ReplaceAll(status, "_", " "))
}

// ── The calendar, as the studio maintains it ─────────────────────────────────

// RaceInput is a race as the panel submits it.
type RaceInput struct {
	Name            string
	Country         string
	Region          domain.RaceRegion
	City            string
	Venue           string
	StartsAt        time.Time
	EndsAt          *time.Time
	RegistrationURL string
	ImageURL        *string
	Status          domain.RaceStatus
}

func (s *Service) SaveRace(ctx context.Context, raceID string, in RaceInput) (domain.RaceEvent, error) {
	if strings.TrimSpace(in.Name) == "" {
		return domain.RaceEvent{}, httpx.Invalid("A race needs a name.")
	}
	if !domain.IsValidRaceRegion(string(in.Region)) {
		return domain.RaceEvent{}, httpx.Invalid("Pick a region.")
	}

	event := domain.RaceEvent{ID: s.ids.New(id.RaceEvent), Status: domain.RaceAnnounced}
	if raceID != "" {
		existing, err := s.repo.Race(ctx, raceID)
		if err != nil {
			return domain.RaceEvent{}, err
		}
		event = existing
	}
	event.Name = strings.TrimSpace(in.Name)
	event.Country = strings.TrimSpace(in.Country)
	event.Region = in.Region
	event.City = strings.TrimSpace(in.City)
	event.Venue = strings.TrimSpace(in.Venue)
	event.StartsAt = in.StartsAt
	// A one-day race is the common case, so an omitted end is the start.
	event.EndsAt = in.StartsAt
	if in.EndsAt != nil {
		event.EndsAt = *in.EndsAt
	}
	event.RegistrationURL = in.RegistrationURL
	event.ImageURL = in.ImageURL
	if in.Status != "" {
		event.Status = in.Status
	}
	return s.repo.UpsertRace(ctx, event)
}

// DeleteRace removes a race from the calendar. Entries go with it: a race that
// is not happening is not a race anybody is training for.
func (s *Service) DeleteRace(ctx context.Context, raceID string) error {
	return s.repo.DeleteRace(ctx, raceID)
}
