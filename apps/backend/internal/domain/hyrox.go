package domain

import (
	"math"
	"sort"
	"time"
)

// The HYROX side of training: generating a workout from the exercise library,
// and running one.
//
// A race is eight one-kilometre runs, each followed by a station, always in the
// same order. Everything here is derived from that one fact — the shorter
// formats are the same shape with fewer stations and less of each.

// WorkoutType is how much of a race the session covers.
type WorkoutType string

const (
	WorkoutFullSimulation WorkoutType = "FULL_SIMULATION"
	WorkoutCoverage       WorkoutType = "COVERAGE"
	WorkoutQuick          WorkoutType = "QUICK"
	WorkoutPractice       WorkoutType = "PRACTICE"
)

func IsValidWorkoutType(value string) bool {
	switch WorkoutType(value) {
	case WorkoutFullSimulation, WorkoutCoverage, WorkoutQuick, WorkoutPractice:
		return true
	}
	return false
}

// Division sets the loads and the rep counts.
type Division string

const (
	DivisionMenOpen   Division = "MEN_OPEN"
	DivisionMenPro    Division = "MEN_PRO"
	DivisionWomenOpen Division = "WOMEN_OPEN"
	DivisionWomenPro  Division = "WOMEN_PRO"
)

func IsValidDivision(value string) bool {
	switch Division(value) {
	case DivisionMenOpen, DivisionMenPro, DivisionWomenOpen, DivisionWomenPro:
		return true
	}
	return false
}

// WorkoutBlock is one run or one station.
type WorkoutBlock struct {
	Order        int    `json:"order"`
	Kind         string `json:"kind"`
	ExerciseID   string `json:"exerciseId"`
	ExerciseName string `json:"exerciseName"`
	// Original* is set when this block stands in for a station the member
	// excluded, so the player can say what it is replacing.
	OriginalExerciseID   *string `json:"originalExerciseId"`
	OriginalExerciseName *string `json:"originalExerciseName"`
	DistanceM            *int    `json:"distanceM"`
	Reps                 *int    `json:"reps"`
	WeightNote           *string `json:"weightNote"`
	TargetSec            int     `json:"targetSec"`
}

// GeneratedWorkout is a workout as authored: read as a unit, never queried
// into, which is why the blocks live in one JSON column.
type GeneratedWorkout struct {
	ID                  string         `json:"id"`
	MemberID            string         `json:"memberId"`
	Type                WorkoutType    `json:"type"`
	Division            Division       `json:"division"`
	Blocks              []WorkoutBlock `json:"blocks"`
	ExcludedExerciseIDs []string       `json:"excludedExerciseIds"`
	TotalTargetSec      int            `json:"totalTargetSec"`
	CreatedAt           time.Time      `json:"createdAt"`
}

// Race-accurate loads for the stations that carry one.
var stationWeightNotes = map[int]map[Division]string{
	2: {DivisionMenOpen: "Sled 152 kg", DivisionMenPro: "Sled 202 kg", DivisionWomenOpen: "Sled 102 kg", DivisionWomenPro: "Sled 152 kg"},
	3: {DivisionMenOpen: "Sled 103 kg", DivisionMenPro: "Sled 153 kg", DivisionWomenOpen: "Sled 78 kg", DivisionWomenPro: "Sled 103 kg"},
	6: {DivisionMenOpen: "2×24 kg", DivisionMenPro: "2×32 kg", DivisionWomenOpen: "2×16 kg", DivisionWomenPro: "2×24 kg"},
	7: {DivisionMenOpen: "20 kg bag", DivisionMenPro: "30 kg bag", DivisionWomenOpen: "10 kg bag", DivisionWomenPro: "20 kg bag"},
	8: {DivisionMenOpen: "6 kg ball", DivisionMenPro: "9 kg ball", DivisionWomenOpen: "4 kg ball", DivisionWomenPro: "6 kg ball"},
}

// Wall balls is the one station whose rep count changes with the division.
var wallBallReps = map[Division]int{
	DivisionMenOpen: 100, DivisionMenPro: 100, DivisionWomenOpen: 75, DivisionWomenPro: 100,
}

// Target seconds for a full effort at each station, before the division factor.
var stationTargetSec = map[int]int{1: 240, 2: 180, 3: 210, 4: 270, 5: 250, 6: 120, 7: 260, 8: 300}

var runPaceSecPerKm = map[Division]int{
	DivisionMenOpen: 330, DivisionMenPro: 300, DivisionWomenOpen: 360, DivisionWomenPro: 330,
}

// proFactor scales the station targets: a pro field is faster over heavier
// loads, and an open women's field is a little slower over lighter ones.
var proFactor = map[Division]float64{
	DivisionMenOpen: 1, DivisionMenPro: 0.9, DivisionWomenOpen: 1.05, DivisionWomenPro: 0.95,
}

// GenerateWorkoutArgs is everything the generator needs.
type GenerateWorkoutArgs struct {
	Type     WorkoutType
	Division Division
	// StationOrders picks the stations for the shorter formats. Empty means
	// "choose for me", and Pick does the choosing.
	StationOrders       []int
	ExcludedExerciseIDs []string
	Exercises           []Exercise
	Substitutions       []SubstitutionRule
	// Pick returns an int in [0, n). It is injected so a test gets the same
	// workout every run and the service can hand it a real random source.
	Pick func(n int) int
}

// ListSubstitutes is what can stand in for an exercise, best match first.
func ListSubstitutes(exerciseID string, substitutions []SubstitutionRule, exercises []Exercise) []Exercise {
	byID := make(map[string]Exercise, len(exercises))
	for _, e := range exercises {
		byID[e.ID] = e
	}
	rules := []SubstitutionRule{}
	for _, r := range substitutions {
		if r.OriginalExerciseID == exerciseID {
			if _, ok := byID[r.AlternativeExerciseID]; ok {
				rules = append(rules, r)
			}
		}
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Similarity > rules[j].Similarity })
	out := make([]Exercise, 0, len(rules))
	for _, r := range rules {
		out = append(out, byID[r.AlternativeExerciseID])
	}
	return out
}

// GenerateWorkout builds the blocks for a session.
//
// Excluded stations are substituted rather than dropped: somebody without a
// sled still needs the work that station does, and a session with a hole in it
// teaches the wrong pacing.
func GenerateWorkout(args GenerateWorkoutArgs) ([]WorkoutBlock, int) {
	stations := []Exercise{}
	var running *Exercise
	for i, e := range args.Exercises {
		if e.HyroxStationOrder != nil {
			stations = append(stations, e)
		}
		if e.Category == CategoryRun && running == nil {
			running = &args.Exercises[i]
		}
	}
	sort.SliceStable(stations, func(i, j int) bool {
		return *stations[i].HyroxStationOrder < *stations[j].HyroxStationOrder
	})

	excluded := make(map[string]bool, len(args.ExcludedExerciseIDs))
	for _, id := range args.ExcludedExerciseIDs {
		excluded[id] = true
	}

	blocks := []WorkoutBlock{}
	order := 1

	pushRun := func(distanceM int) {
		id, name := "ex_run", "Running"
		if running != nil {
			id = running.ID
		}
		blocks = append(blocks, WorkoutBlock{
			Order: order, Kind: "RUN", ExerciseID: id, ExerciseName: name,
			DistanceM: intPtr(distanceM),
			TargetSec: int(math.Round(float64(runPaceSecPerKm[args.Division]*distanceM) / 1000)),
		})
		order++
	}

	pushStation := func(station Exercise, volume float64) {
		exercise, original := station, (*Exercise)(nil)
		if excluded[station.ID] {
			for _, sub := range ListSubstitutes(station.ID, args.Substitutions, args.Exercises) {
				if !excluded[sub.ID] {
					exercise, original = sub, &station
					break
				}
			}
		}
		stationOrder := *station.HyroxStationOrder

		block := WorkoutBlock{
			Order: order, Kind: "STATION",
			ExerciseID: exercise.ID, ExerciseName: exercise.Name,
			TargetSec: int(math.Round(float64(stationTargetSec[stationOrder]) * proFactor[args.Division] * volume)),
		}
		if original != nil {
			block.OriginalExerciseID = &original.ID
			block.OriginalExerciseName = &original.Name
		}
		if station.DefaultSpec.DistanceM != nil {
			block.DistanceM = intPtr(int(math.Round(float64(*station.DefaultSpec.DistanceM) * volume)))
		}
		reps := station.DefaultSpec.Reps
		if stationOrder == 8 {
			reps = intPtr(wallBallReps[args.Division])
		}
		if reps != nil {
			block.Reps = intPtr(int(math.Round(float64(*reps) * volume)))
		}
		if note, ok := stationWeightNotes[stationOrder][args.Division]; ok {
			block.WeightNote = &note
		}
		blocks = append(blocks, block)
		order++
	}

	chooseStations := func(count int) []Exercise {
		if len(args.StationOrders) > 0 {
			wanted := make(map[int]bool, len(args.StationOrders))
			for _, o := range args.StationOrders {
				wanted[o] = true
			}
			chosen := []Exercise{}
			for _, s := range stations {
				if wanted[*s.HyroxStationOrder] {
					chosen = append(chosen, s)
				}
			}
			return chosen
		}
		pool := append([]Exercise{}, stations...)
		chosen := []Exercise{}
		for len(chosen) < count && len(pool) > 0 {
			i := args.Pick(len(pool))
			chosen = append(chosen, pool[i])
			pool = append(pool[:i], pool[i+1:]...)
		}
		sort.SliceStable(chosen, func(i, j int) bool {
			return *chosen[i].HyroxStationOrder < *chosen[j].HyroxStationOrder
		})
		return chosen
	}

	switch args.Type {
	case WorkoutFullSimulation:
		// The race itself: every station, full volume, a kilometre between each.
		for _, station := range stations {
			pushRun(1000)
			pushStation(station, 1)
		}
	case WorkoutCoverage:
		for _, station := range chooseStations(4) {
			pushRun(600)
			pushStation(station, 1)
		}
	case WorkoutQuick:
		for _, station := range chooseStations(4) {
			pushRun(400)
			pushStation(station, 0.5)
		}
	case WorkoutPractice:
		// One station, three times: this is the format for fixing something.
		target := stations
		if chosen := chooseStations(1); len(chosen) > 0 {
			target = chosen[:1]
		}
		if len(target) > 0 {
			for range 3 {
				pushRun(200)
				pushStation(target[0], 0.5)
			}
		}
	}

	total := 0
	for _, b := range blocks {
		total += b.TargetSec
	}
	return blocks, total
}

// ReplaceBlockExercise swaps one block's exercise, remembering what it was.
//
// The original is only recorded the first time: swapping twice still says what
// the block is standing in for at the race, not what it was a minute ago.
func ReplaceBlockExercise(block WorkoutBlock, replacement Exercise) WorkoutBlock {
	if block.OriginalExerciseID == nil {
		id, name := block.ExerciseID, block.ExerciseName
		block.OriginalExerciseID = &id
		block.OriginalExerciseName = &name
	}
	block.ExerciseID = replacement.ID
	block.ExerciseName = replacement.Name
	return block
}

// ── Running a session ────────────────────────────────────────────────────────

// WorkoutSessionStatus is where a session has got to.
type WorkoutSessionStatus string

const (
	WorkoutReady     WorkoutSessionStatus = "READY"
	WorkoutStarted   WorkoutSessionStatus = "STARTED"
	WorkoutPaused    WorkoutSessionStatus = "PAUSED"
	WorkoutCompleted WorkoutSessionStatus = "COMPLETED"
	WorkoutPartial   WorkoutSessionStatus = "PARTIAL"
)

// WorkoutSessionTransitions: a finished session is finished. Somebody who
// stopped halfway gets PARTIAL, which keeps the blocks they did without
// claiming they did the workout.
var WorkoutSessionTransitions = TransitionMap[WorkoutSessionStatus]{
	WorkoutReady:     {WorkoutStarted},
	WorkoutStarted:   {WorkoutPaused, WorkoutCompleted, WorkoutPartial},
	WorkoutPaused:    {WorkoutStarted, WorkoutPartial},
	WorkoutCompleted: {},
	WorkoutPartial:   {},
}

// WorkoutBlockResult is how long one block actually took.
type WorkoutBlockResult struct {
	Order       int `json:"order"`
	DurationSec int `json:"durationSec"`
}

// WorkoutSession is one attempt at a workout.
type WorkoutSession struct {
	ID            string               `json:"id"`
	WorkoutID     string               `json:"workoutId"`
	MemberID      string               `json:"memberId"`
	Status        WorkoutSessionStatus `json:"status"`
	CurrentBlock  int                  `json:"currentBlock"`
	StartedAt     *time.Time           `json:"startedAt"`
	EndedAt       *time.Time           `json:"endedAt"`
	BlockResults  []WorkoutBlockResult `json:"blockResults"`
	PauseCount    int                  `json:"pauseCount"`
	TotalPauseSec int                  `json:"totalPauseSec"`
	CreatedAt     time.Time            `json:"createdAt"`
}

// SessionActiveSec is time on the clock, which is the sum of the blocks —
// deliberately not wall time, because a session paused for lunch did not take
// four hours.
func SessionActiveSec(results []WorkoutBlockResult) int {
	total := 0
	for _, r := range results {
		total += r.DurationSec
	}
	return total
}

// SessionCompletionPct is how much of the workout was actually done.
func SessionCompletionPct(results []WorkoutBlockResult, totalBlocks int) int {
	if totalBlocks == 0 {
		return 0
	}
	return int(math.Round(float64(len(results)) / float64(totalBlocks) * 100))
}

func intPtr(v int) *int { return &v }
