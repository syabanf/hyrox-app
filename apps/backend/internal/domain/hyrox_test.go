package domain_test

import (
	"testing"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
)

func station(order int, id, name string, distance, reps *int) domain.Exercise {
	return domain.Exercise{
		ID: id, Name: name, Category: domain.CategoryConditioning,
		HyroxStationOrder: &order, Difficulty: 2,
		DefaultSpec: domain.ExerciseSpec{DistanceM: distance, Reps: reps},
	}
}

func num(v int) *int { return &v }

// The eight race stations plus running, in library order.
func exerciseLibrary() []domain.Exercise {
	return []domain.Exercise{
		{ID: "ex_run", Name: "Running", Category: domain.CategoryRun},
		station(1, "ex_ski", "SkiErg", num(1000), nil),
		station(2, "ex_push", "Sled Push", num(50), nil),
		station(3, "ex_pull", "Sled Pull", num(50), nil),
		station(4, "ex_burpee", "Burpee Broad Jump", num(80), nil),
		station(5, "ex_row", "Rowing", num(1000), nil),
		station(6, "ex_carry", "Farmers Carry", num(200), nil),
		station(7, "ex_lunge", "Sandbag Lunges", num(100), nil),
		station(8, "ex_wall", "Wall Balls", nil, num(100)),
		{ID: "ex_step", Name: "Step-ups", Category: domain.CategoryJump},
	}
}

func generate(t *testing.T, args domain.GenerateWorkoutArgs) ([]domain.WorkoutBlock, int) {
	t.Helper()
	if args.Exercises == nil {
		args.Exercises = exerciseLibrary()
	}
	if args.Pick == nil {
		args.Pick = func(int) int { return 0 }
	}
	return domain.GenerateWorkout(args)
}

func TestFullSimulationIsTheRace(t *testing.T) {
	blocks, total := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutFullSimulation, Division: domain.DivisionMenOpen,
	})

	// Eight runs and eight stations, alternating, starting with a run.
	if len(blocks) != 16 {
		t.Fatalf("got %d blocks, want 16", len(blocks))
	}
	for i, b := range blocks {
		wantKind := "STATION"
		if i%2 == 0 {
			wantKind = "RUN"
		}
		if b.Kind != wantKind {
			t.Fatalf("block %d is %s, want %s", i+1, b.Kind, wantKind)
		}
		if b.Order != i+1 {
			t.Fatalf("block %d is numbered %d", i+1, b.Order)
		}
	}
	if blocks[0].DistanceM == nil || *blocks[0].DistanceM != 1000 {
		t.Fatal("a full simulation runs a kilometre between stations")
	}
	if total <= 0 {
		t.Fatal("a workout with sixteen blocks has no target time")
	}
	// The stations come in race order, not library order.
	if blocks[1].ExerciseID != "ex_ski" || blocks[15].ExerciseID != "ex_wall" {
		t.Fatalf("stations are out of race order: first %s, last %s",
			blocks[1].ExerciseID, blocks[15].ExerciseID)
	}
}

func TestDivisionChangesLoadsAndReps(t *testing.T) {
	menOpen, menOpenTotal := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutFullSimulation, Division: domain.DivisionMenOpen,
	})
	womenOpen, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutFullSimulation, Division: domain.DivisionWomenOpen,
	})
	menPro, menProTotal := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutFullSimulation, Division: domain.DivisionMenPro,
	})

	// Wall balls: the last block of each.
	if got := *menOpen[15].Reps; got != 100 {
		t.Fatalf("men's open wall balls is %d reps, want 100", got)
	}
	if got := *womenOpen[15].Reps; got != 75 {
		t.Fatalf("women's open wall balls is %d reps, want 75", got)
	}
	// The sled push weight note follows the division.
	if menOpen[3].WeightNote == nil || *menOpen[3].WeightNote != "Sled 152 kg" {
		t.Fatalf("men's open sled push note is %v", menOpen[3].WeightNote)
	}
	if *menPro[3].WeightNote != "Sled 202 kg" {
		t.Fatalf("men's pro sled push note is %s", *menPro[3].WeightNote)
	}
	// A pro field is quicker over the same course.
	if menProTotal >= menOpenTotal {
		t.Fatalf("pro target %d is not faster than open %d", menProTotal, menOpenTotal)
	}
}

// An excluded station is substituted, not dropped: the work still has to happen.
func TestExcludedStationIsSubstituted(t *testing.T) {
	blocks, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutFullSimulation, Division: domain.DivisionMenOpen,
		ExcludedExerciseIDs: []string{"ex_push"},
		Substitutions: []domain.SubstitutionRule{
			{OriginalExerciseID: "ex_push", AlternativeExerciseID: "ex_step", Similarity: 0.8},
		},
	})

	if len(blocks) != 16 {
		t.Fatalf("excluding a station changed the shape: %d blocks", len(blocks))
	}
	sled := blocks[3]
	if sled.ExerciseID != "ex_step" {
		t.Fatalf("the sled push was not substituted: %s", sled.ExerciseID)
	}
	if sled.OriginalExerciseName == nil || *sled.OriginalExerciseName != "Sled Push" {
		t.Fatal("the substitute does not say what it is standing in for")
	}
	// The other stations are untouched.
	if blocks[1].OriginalExerciseID != nil {
		t.Fatal("a station that was not excluded was marked as a substitution")
	}
}

// With nothing to substitute, the station stays: a hole in the workout would
// be worse than a station somebody has to improvise.
func TestExcludedStationWithNoSubstituteStays(t *testing.T) {
	blocks, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutFullSimulation, Division: domain.DivisionMenOpen,
		ExcludedExerciseIDs: []string{"ex_push"},
	})
	if blocks[3].ExerciseID != "ex_push" || blocks[3].OriginalExerciseID != nil {
		t.Fatalf("got %s (original %v)", blocks[3].ExerciseID, blocks[3].OriginalExerciseID)
	}
}

func TestShorterFormats(t *testing.T) {
	coverage, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutCoverage, Division: domain.DivisionMenOpen,
		StationOrders: []int{1, 5},
	})
	if len(coverage) != 4 {
		t.Fatalf("two chosen stations gave %d blocks, want 4", len(coverage))
	}
	if *coverage[0].DistanceM != 600 {
		t.Fatalf("coverage runs %d m between stations, want 600", *coverage[0].DistanceM)
	}
	if coverage[1].ExerciseID != "ex_ski" || coverage[3].ExerciseID != "ex_row" {
		t.Fatal("coverage did not use the stations it was given")
	}

	// Quick is the same stations at half volume.
	quick, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutQuick, Division: domain.DivisionMenOpen, StationOrders: []int{1},
	})
	if *quick[1].DistanceM != 500 {
		t.Fatalf("a half-volume SkiErg is %d m, want 500", *quick[1].DistanceM)
	}

	// Practice is one station, three times.
	practice, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutPractice, Division: domain.DivisionMenOpen, StationOrders: []int{8},
	})
	if len(practice) != 6 {
		t.Fatalf("practice gave %d blocks, want 6 (three rounds)", len(practice))
	}
	for i := 1; i < 6; i += 2 {
		if practice[i].ExerciseID != "ex_wall" {
			t.Fatalf("practice round %d is %s, want the chosen station", i/2+1, practice[i].ExerciseID)
		}
	}
}

// Left to choose, the generator takes four stations and puts them in race order.
func TestChosenStationsComeBackInRaceOrder(t *testing.T) {
	blocks, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutCoverage, Division: domain.DivisionMenOpen,
		// Always take the last of what is left, so the picks are unordered.
		Pick: func(n int) int { return n - 1 },
	})
	if len(blocks) != 8 {
		t.Fatalf("got %d blocks, want 8 (four stations)", len(blocks))
	}
	previous := 0
	for i := 1; i < len(blocks); i += 2 {
		order := 0
		for _, e := range exerciseLibrary() {
			if e.ID == blocks[i].ExerciseID && e.HyroxStationOrder != nil {
				order = *e.HyroxStationOrder
			}
		}
		if order <= previous {
			t.Fatalf("station %d comes after station %d", order, previous)
		}
		previous = order
	}
}

func TestReplaceBlockRemembersTheOriginalOnce(t *testing.T) {
	blocks, _ := generate(t, domain.GenerateWorkoutArgs{
		Type: domain.WorkoutPractice, Division: domain.DivisionMenOpen, StationOrders: []int{8},
	})
	first := domain.ReplaceBlockExercise(blocks[1], domain.Exercise{ID: "ex_step", Name: "Step-ups"})
	if *first.OriginalExerciseName != "Wall Balls" {
		t.Fatalf("original is %s", *first.OriginalExerciseName)
	}

	second := domain.ReplaceBlockExercise(first, domain.Exercise{ID: "ex_row", Name: "Rowing"})
	if *second.OriginalExerciseName != "Wall Balls" {
		t.Fatalf("swapping twice lost the race station: original is %s", *second.OriginalExerciseName)
	}
	if second.ExerciseID != "ex_row" {
		t.Fatalf("the second swap did not take: %s", second.ExerciseID)
	}
}

func TestSessionProgress(t *testing.T) {
	results := []domain.WorkoutBlockResult{{Order: 1, DurationSec: 300}, {Order: 2, DurationSec: 200}}
	if got := domain.SessionActiveSec(results); got != 500 {
		t.Fatalf("active time is %d s, want 500", got)
	}
	if got := domain.SessionCompletionPct(results, 16); got != 13 {
		t.Fatalf("2 of 16 blocks is %d%%, want 13", got)
	}
	if got := domain.SessionCompletionPct(nil, 0); got != 0 {
		t.Fatalf("a workout with no blocks is %d%% done", got)
	}
}

func TestSessionTransitions(t *testing.T) {
	legal := [][2]domain.WorkoutSessionStatus{
		{domain.WorkoutReady, domain.WorkoutStarted},
		{domain.WorkoutStarted, domain.WorkoutPaused},
		{domain.WorkoutPaused, domain.WorkoutStarted},
		{domain.WorkoutStarted, domain.WorkoutCompleted},
		{domain.WorkoutPaused, domain.WorkoutPartial},
	}
	for _, move := range legal {
		if !domain.CanTransition(domain.WorkoutSessionTransitions, move[0], move[1]) {
			t.Errorf("%s → %s should be allowed", move[0], move[1])
		}
	}
	illegal := [][2]domain.WorkoutSessionStatus{
		{domain.WorkoutReady, domain.WorkoutCompleted},
		{domain.WorkoutCompleted, domain.WorkoutStarted},
		{domain.WorkoutPartial, domain.WorkoutStarted},
		{domain.WorkoutPaused, domain.WorkoutCompleted},
	}
	for _, move := range illegal {
		if domain.CanTransition(domain.WorkoutSessionTransitions, move[0], move[1]) {
			t.Errorf("%s → %s should be refused", move[0], move[1])
		}
	}
}

func TestRacePrediction(t *testing.T) {
	if got := domain.PredictRaceSec(nil); got != nil {
		t.Fatalf("predicted %d seconds from no simulations", *got)
	}
	if got := domain.PredictRaceSec([]int{0, 0}); got != nil {
		t.Fatal("empty simulations produced a prediction")
	}
	// The best simulation, less the race-day bump.
	got := domain.PredictRaceSec([]int{5400, 5000, 5600})
	if got == nil || *got != 4850 {
		t.Fatalf("predicted %v, want 4850", got)
	}
}

func TestReadinessScore(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	dates := func(n int, daysAgo int) []time.Time {
		out := make([]time.Time, n)
		for i := range out {
			out[i] = now.AddDate(0, 0, -daysAgo)
		}
		return out
	}

	if got := domain.RaceReadinessScore(nil, now); got != 0 {
		t.Fatalf("no training scored %d", got)
	}
	if got := domain.RaceReadinessScore(dates(12, 5), now); got != 100 {
		t.Fatalf("twelve recent sessions scored %d, want 100", got)
	}
	if got := domain.RaceReadinessScore(dates(6, 5), now); got != 50 {
		t.Fatalf("six recent sessions scored %d, want 50", got)
	}
	// Older than four weeks does not count.
	if got := domain.RaceReadinessScore(dates(20, 40), now); got != 0 {
		t.Fatalf("training from six weeks ago scored %d", got)
	}
	// And the score is capped.
	if got := domain.RaceReadinessScore(dates(40, 3), now); got != 100 {
		t.Fatalf("forty sessions scored %d, want 100", got)
	}
}

func TestAnalyzeRace(t *testing.T) {
	goal, prediction := 5400, 5000

	beat := domain.AnalyzeRace(5200, &goal, &prediction)
	if *beat.VsGoalSec != -200 || *beat.AchievedGoal != true {
		t.Fatalf("beating the goal read as %+v", beat)
	}
	if *beat.VsPredictionSec != 200 {
		t.Fatalf("vs prediction is %d, want 200 (slower than predicted)", *beat.VsPredictionSec)
	}

	missed := domain.AnalyzeRace(5600, &goal, nil)
	if *missed.AchievedGoal != false || missed.VsPredictionSec != nil {
		t.Fatalf("missing the goal with no prediction read as %+v", missed)
	}

	// No goal set is not a failed goal.
	none := domain.AnalyzeRace(5600, nil, nil)
	if none.AchievedGoal != nil || none.VsGoalSec != nil {
		t.Fatalf("a race with no goal was judged: %+v", none)
	}
}
