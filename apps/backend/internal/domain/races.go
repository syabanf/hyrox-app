package domain

import (
	"math"
	"time"
)

// The race calendar, and the one race a member is training for.

// RaceStatus is where an event is in its life.
type RaceStatus string

const (
	RaceAnnounced        RaceStatus = "ANNOUNCED"
	RaceRegistrationOpen RaceStatus = "REGISTRATION_OPEN"
	RaceSoldOut          RaceStatus = "SOLD_OUT"
	RaceUpcoming         RaceStatus = "UPCOMING"
	RaceOngoing          RaceStatus = "ONGOING"
	RaceCompleted        RaceStatus = "COMPLETED"
	RaceCancelled        RaceStatus = "CANCELLED"
)

// RaceRegion is the coarse filter on the calendar. Anything finer than a
// continent is a search box, not a filter.
type RaceRegion string

const (
	RegionAsia     RaceRegion = "ASIA"
	RegionEurope   RaceRegion = "EUROPE"
	RegionAmericas RaceRegion = "AMERICAS"
	RegionOceania  RaceRegion = "OCEANIA"
)

func IsValidRaceRegion(value string) bool {
	switch RaceRegion(value) {
	case RegionAsia, RegionEurope, RegionAmericas, RegionOceania:
		return true
	}
	return false
}

// RaceEvent is one race on the calendar.
type RaceEvent struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Country         string     `json:"country"`
	Region          RaceRegion `json:"region"`
	City            string     `json:"city"`
	Venue           string     `json:"venue"`
	StartsAt        time.Time  `json:"startsAt"`
	EndsAt          time.Time  `json:"endsAt"`
	RegistrationURL string     `json:"registrationUrl"`
	ImageURL        *string    `json:"imageUrl"`
	Status          RaceStatus `json:"status"`
}

// UserRaceStatus is where the member is with a race they entered.
type UserRaceStatus string

const (
	UserRaceTraining  UserRaceStatus = "TRAINING"
	UserRaceRaced     UserRaceStatus = "RACED"
	UserRaceCancelled UserRaceStatus = "CANCELLED"
)

// UserRaceTransitions: a raced race is history. Cancelling can be undone,
// because people pull out of races and then change their minds.
var UserRaceTransitions = TransitionMap[UserRaceStatus]{
	UserRaceTraining:  {UserRaceRaced, UserRaceCancelled},
	UserRaceRaced:     {},
	UserRaceCancelled: {UserRaceTraining},
}

// UserRace is a member's entry: the division they are in, what they are
// aiming for, and what they did.
type UserRace struct {
	ID          string         `json:"id"`
	MemberID    string         `json:"memberId"`
	RaceEventID string         `json:"raceEventId"`
	Division    Division       `json:"division"`
	GoalSec     *int           `json:"goalSec"`
	Status      UserRaceStatus `json:"status"`
	ResultSec   *int           `json:"resultSec"`
	CreatedAt   time.Time      `json:"createdAt"`
}

// raceDayBump is how much faster a race is than the same effort alone in a
// gym: a crowd, a start gun and a finish line are worth about three per cent.
const raceDayBump = 0.97

// PredictRaceSec is the member's likely finishing time, from their best full
// simulation. Nil until they have done one — a prediction from no data is a
// guess wearing a number.
func PredictRaceSec(fullSimulationSecs []int) *int {
	best := 0
	for _, s := range fullSimulationSecs {
		if s > 0 && (best == 0 || s < best) {
			best = s
		}
	}
	if best == 0 {
		return nil
	}
	predicted := int(math.Round(float64(best) * raceDayBump))
	return &predicted
}

// readinessWindow and readinessTarget: four weeks at three sessions a week is
// the plan a hundred per cent means.
const (
	readinessWindow = 28 * 24 * time.Hour
	readinessTarget = 12
)

// RaceReadinessScore is 0–100 for how consistently the member has trained
// lately. It counts sessions rather than distance on purpose: turning up
// three times a week is the thing that gets somebody round a race.
func RaceReadinessScore(activityDates []time.Time, now time.Time) int {
	cutoff := now.Add(-readinessWindow)
	recent := 0
	for _, d := range activityDates {
		if !d.Before(cutoff) {
			recent++
		}
	}
	score := int(math.Round(float64(recent) / readinessTarget * 100))
	if score > 100 {
		score = 100
	}
	return score
}

// RaceAnalysis is how the race went against what was hoped for.
type RaceAnalysis struct {
	VsGoalSec       *int  `json:"vsGoalSec"`
	VsPredictionSec *int  `json:"vsPredictionSec"`
	AchievedGoal    *bool `json:"achievedGoal"`
}

// AnalyzeRace compares a result with the goal and the prediction. Negative is
// faster, which is the convention every runner already reads correctly.
func AnalyzeRace(resultSec int, goalSec, predictionSec *int) RaceAnalysis {
	analysis := RaceAnalysis{}
	if goalSec != nil {
		diff := resultSec - *goalSec
		achieved := resultSec <= *goalSec
		analysis.VsGoalSec = &diff
		analysis.AchievedGoal = &achieved
	}
	if predictionSec != nil {
		diff := resultSec - *predictionSec
		analysis.VsPredictionSec = &diff
	}
	return analysis
}
