package domain

// ExerciseCategory groups the movement library. RUN and the eight race
// stations are what a HYROX simulation is built from; the rest support
// training plans.
type ExerciseCategory string

const (
	CategoryErg          ExerciseCategory = "ERG"
	CategorySled         ExerciseCategory = "SLED"
	CategoryJump         ExerciseCategory = "JUMP"
	CategoryCarry        ExerciseCategory = "CARRY"
	CategoryLunge        ExerciseCategory = "LUNGE"
	CategoryThrow        ExerciseCategory = "THROW"
	CategoryRun          ExerciseCategory = "RUN"
	CategoryConditioning ExerciseCategory = "CONDITIONING"
)

// ExerciseSpec is the default prescription for one exercise: a distance, a rep
// count, or neither.
type ExerciseSpec struct {
	DistanceM *int `json:"distanceM"`
	Reps      *int `json:"reps"`
}

// Exercise is one movement in the library, shown in the member Guides tab and
// used by the workout generator.
type Exercise struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Category  ExerciseCategory `json:"category"`
	Equipment []string         `json:"equipment"`
	// HyroxStationOrder is 1..8 when this exercise is a race station.
	HyroxStationOrder *int         `json:"hyroxStationOrder"`
	Difficulty        int          `json:"difficulty"`
	DefaultSpec       ExerciseSpec `json:"defaultSpec"`
	VideoURL          *string      `json:"videoUrl"`
}

// SubstitutionRule says which exercise can stand in for another when the
// equipment is unavailable.
type SubstitutionRule struct {
	OriginalExerciseID    string  `json:"originalExerciseId"`
	AlternativeExerciseID string  `json:"alternativeExerciseId"`
	Similarity            float64 `json:"similarity"`
	ConversionNote        string  `json:"conversionNote"`
}

// IsValidExerciseCategory validates a category arriving from a request.
func IsValidExerciseCategory(value string) bool {
	switch ExerciseCategory(value) {
	case CategoryErg, CategorySled, CategoryJump, CategoryCarry,
		CategoryLunge, CategoryThrow, CategoryRun, CategoryConditioning:
		return true
	}
	return false
}
