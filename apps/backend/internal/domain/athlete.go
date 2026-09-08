package domain

import (
	"math"
	"sort"
	"time"
)

// The athlete side of the member app: activities with a GPS track, the social
// graph around them, and the segments, clubs and gear that hang off both.
//
// Everything here is pure. A track arrives as a slice of points and leaves as
// numbers; nothing in this file knows where the points were stored or who is
// allowed to see them.

// ActivityType is what the member was doing.
type ActivityType string

const (
	ActivityRun     ActivityType = "RUN"
	ActivityRide    ActivityType = "RIDE"
	ActivityWalk    ActivityType = "WALK"
	ActivityWorkout ActivityType = "WORKOUT"
)

func IsValidActivityType(value string) bool {
	switch ActivityType(value) {
	case ActivityRun, ActivityRide, ActivityWalk, ActivityWorkout:
		return true
	}
	return false
}

// ActivityVisibility is who may see an activity.
type ActivityVisibility string

const (
	VisibilityEveryone  ActivityVisibility = "EVERYONE"
	VisibilityFollowers ActivityVisibility = "FOLLOWERS"
	VisibilityPrivate   ActivityVisibility = "PRIVATE"
)

func IsValidVisibility(value string) bool {
	switch ActivityVisibility(value) {
	case VisibilityEveryone, VisibilityFollowers, VisibilityPrivate:
		return true
	}
	return false
}

// TrackPoint is one GPS sample. T is milliseconds since the activity started,
// which keeps a track self-contained: it can be replayed without knowing when
// it was recorded.
type TrackPoint struct {
	T   int64    `json:"t"`
	Lat float64  `json:"lat"`
	Lng float64  `json:"lng"`
	Ele *float64 `json:"ele,omitempty"`
}

// Activity is one recorded effort.
type Activity struct {
	ID          string       `json:"id"`
	MemberID    string       `json:"memberId"`
	Type        ActivityType `json:"type"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	StartedAt   time.Time    `json:"startedAt"`
	ElapsedSec  int          `json:"elapsedSec"`
	MovingSec   int          `json:"movingSec"`
	DistanceM   float64      `json:"distanceM"`
	// AvgPaceSecPerKm is moving pace, and is nil for an activity with no
	// meaningful distance — a gym session recorded without a track.
	AvgPaceSecPerKm *int               `json:"avgPaceSecPerKm"`
	ElevationGainM  float64            `json:"elevationGainM"`
	Points          []TrackPoint       `json:"points"`
	Photos          []string           `json:"photos"`
	Visibility      ActivityVisibility `json:"visibility"`
	GearID          *string            `json:"gearId"`
	CreatedAt       time.Time          `json:"createdAt"`
}

// ActivitySplit is one kilometre of the track. The last one is usually partial.
type ActivitySplit struct {
	Km           int     `json:"km"`
	DistanceM    float64 `json:"distanceM"`
	PaceSecPerKm int     `json:"paceSecPerKm"`
	Full         bool    `json:"full"`
}

// ActivityStats is everything derivable from a track.
type ActivityStats struct {
	DistanceM        float64         `json:"distanceM"`
	ElapsedSec       int             `json:"elapsedSec"`
	MovingSec        int             `json:"movingSec"`
	AvgPaceSecPerKm  *int            `json:"avgPaceSecPerKm"`
	ElevationGainM   float64         `json:"elevationGainM"`
	Splits           []ActivitySplit `json:"splits"`
	BestSplitPaceSec *int            `json:"bestSplitPaceSec"`
}

const earthRadiusM = 6_371_000

// HaversineM is the great-circle distance between two points, in metres.
func HaversineM(a, b TrackPoint) float64 {
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := rad(b.Lat - a.Lat)
	dLng := rad(b.Lng - a.Lng)
	s := math.Pow(math.Sin(dLat/2), 2) +
		math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Pow(math.Sin(dLng/2), 2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(s))
}

// movingThresholdMPS is the speed below which a gap counts as standing still.
// Without it, a coffee stop lands in the moving time and ruins the pace.
const movingThresholdMPS = 0.5

// minElevationRiseM ignores GPS jitter: a barometer wobbling by a few
// centimetres would otherwise accumulate a mountain over an hour.
const minElevationRiseM = 0.3

// ComputeActivityStats turns a track into distance, moving time, climb and
// kilometre splits.
//
// Splits are accounted inside the loop rather than afterwards because one GPS
// sample can cross a kilometre boundary: at ten seconds between points, a
// cyclist covers a hundred metres, and attributing all of it to one side of
// the line puts the split times out.
func ComputeActivityStats(points []TrackPoint) ActivityStats {
	if len(points) < 2 {
		elapsed := 0
		if len(points) == 1 {
			elapsed = int(math.Round(float64(points[0].T) / 1000))
		}
		return ActivityStats{ElapsedSec: elapsed, Splits: []ActivitySplit{}}
	}

	var distanceM, elevationGainM, splitDist, splitMoving, movingSec float64
	splits := []ActivitySplit{}

	for i := 1; i < len(points); i++ {
		prev, curr := points[i-1], points[i]
		dt := float64(curr.T-prev.T) / 1000
		if dt <= 0 {
			continue
		}
		d := HaversineM(prev, curr)
		distanceM += d
		if prev.Ele != nil && curr.Ele != nil {
			if rise := *curr.Ele - *prev.Ele; rise > minElevationRiseM {
				elevationGainM += rise
			}
		}
		moving := d/dt >= movingThresholdMPS
		if moving {
			movingSec += dt
		}

		remaining := d
		remainingT := 0.0
		if moving {
			remainingT = dt
		}
		for remaining > 0 {
			room := 1000 - splitDist
			take := math.Min(room, remaining)
			frac := take / remaining
			splitDist += take
			splitMoving += remainingT * frac
			remainingT *= 1 - frac
			remaining -= take
			if splitDist >= 1000 {
				splits = append(splits, ActivitySplit{
					Km:           len(splits) + 1,
					DistanceM:    1000,
					PaceSecPerKm: int(math.Round(splitMoving)),
					Full:         true,
				})
				splitDist, splitMoving = 0, 0
			}
		}
	}
	// A trailing fifty metres is rounding, not a split worth showing.
	if splitDist > 50 {
		splits = append(splits, ActivitySplit{
			Km:           len(splits) + 1,
			DistanceM:    math.Round(splitDist),
			PaceSecPerKm: int(math.Round(splitMoving / splitDist * 1000)),
			Full:         false,
		})
	}

	stats := ActivityStats{
		DistanceM:      math.Round(distanceM),
		ElapsedSec:     int(math.Round(float64(points[len(points)-1].T-points[0].T) / 1000)),
		MovingSec:      int(math.Round(movingSec)),
		ElevationGainM: math.Round(elevationGainM),
		Splits:         splits,
	}
	// Below fifty metres a "pace" is noise dressed up as a number.
	if distanceM >= 50 {
		pace := int(math.Round(movingSec / (distanceM / 1000)))
		stats.AvgPaceSecPerKm = &pace
	}
	best := 0
	for _, s := range splits {
		if s.Full && (best == 0 || s.PaceSecPerKm < best) {
			best = s.PaceSecPerKm
		}
	}
	if best > 0 {
		stats.BestSplitPaceSec = &best
	}
	return stats
}

// ── Social graph ─────────────────────────────────────────────────────────────

// Follow is one athlete following another.
type Follow struct {
	FollowerID string    `json:"followerId"`
	FolloweeID string    `json:"followeeId"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ActivityComment is one reply under an activity.
type ActivityComment struct {
	ID         string    `json:"id"`
	ActivityID string    `json:"activityId"`
	MemberID   string    `json:"memberId"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"createdAt"`
}

// CanViewActivity answers whether a viewer may see an activity.
//
// The follow test is a function rather than a list because the caller usually
// has the answer to hand — a set built once for a whole feed — and passing the
// whole graph in would make this the wrong place to look at it.
func CanViewActivity(memberID string, visibility ActivityVisibility, viewerID string, viewerFollows func(string) bool) bool {
	switch {
	case memberID == viewerID:
		return true
	case visibility == VisibilityEveryone:
		return true
	case visibility == VisibilityFollowers:
		return viewerFollows(memberID)
	default:
		return false
	}
}

// ── Segments ─────────────────────────────────────────────────────────────────

// Segment is a stretch of road people race each other over.
type Segment struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Type      ActivityType `json:"type"`
	DistanceM float64      `json:"distanceM"`
	Location  string       `json:"location"`
	Path      []TrackPoint `json:"path"`
}

// SegmentEffort is one attempt at a segment, cut out of a longer activity.
type SegmentEffort struct {
	ID         string    `json:"id"`
	SegmentID  string    `json:"segmentId"`
	ActivityID string    `json:"activityId"`
	MemberID   string    `json:"memberId"`
	ElapsedSec int       `json:"elapsedSec"`
	CreatedAt  time.Time `json:"createdAt"`
}

// segmentMatchRadiusM is how near a track must pass a segment's end for the
// attempt to count. Sixty metres is wide enough for a GPS fix under trees and
// narrow enough that the parallel street does not match.
const segmentMatchRadiusM = 60

// SegmentMatch is a segment found inside an activity's track.
type SegmentMatch struct {
	Segment    Segment
	StartIdx   int
	EndIdx     int
	ElapsedSec int
}

// MatchSegments finds the segments an activity covered.
//
// A track matches when it passes the start gate and later the end gate, having
// covered roughly the segment's distance in between. The distance window is
// what rejects a rider who went through both gates by a longer route.
func MatchSegments(segments []Segment, activityType ActivityType, points []TrackPoint) []SegmentMatch {
	if len(points) < 2 {
		return nil
	}
	// Prefix sums, so the distance travelled between two indices is one
	// subtraction rather than a walk.
	cum := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		cum[i] = cum[i-1] + HaversineM(points[i-1], points[i])
	}

	var matches []SegmentMatch
	for _, segment := range segments {
		if segment.Type != activityType || len(segment.Path) < 2 {
			continue
		}
		gateStart := segment.Path[0]
		gateEnd := segment.Path[len(segment.Path)-1]

		startIdx := -1
		for i := range points {
			if HaversineM(points[i], gateStart) <= segmentMatchRadiusM {
				startIdx = i
				break
			}
		}
		if startIdx < 0 {
			continue
		}

		endIdx := -1
		for j := startIdx + 1; j < len(points); j++ {
			if HaversineM(points[j], gateEnd) > segmentMatchRadiusM {
				continue
			}
			travelled := cum[j] - cum[startIdx]
			if travelled >= segment.DistanceM*0.8 && travelled <= segment.DistanceM*1.35 {
				endIdx = j
				break
			}
		}
		if endIdx < 0 {
			continue
		}

		elapsed := int(math.Round(float64(points[endIdx].T-points[startIdx].T) / 1000))
		if elapsed < 1 {
			elapsed = 1
		}
		matches = append(matches, SegmentMatch{
			Segment: segment, StartIdx: startIdx, EndIdx: endIdx, ElapsedSec: elapsed,
		})
	}
	return matches
}

// GroupCandidate is the little of an activity that grouping needs.
type GroupCandidate struct {
	ID        string
	MemberID  string
	Type      ActivityType
	StartedAt time.Time
	Start     *TrackPoint
}

const (
	groupWindow  = 45 * time.Minute
	groupRadiusM = 500
)

// FindGroupedActivities is "you ran this with these people": same activity
// type, starting around the same time, from around the same place.
//
// It is deliberately generous. Two people who set off together rarely press
// start in the same minute or from the same doorway, and a group ride that is
// not recognised is worse than one that occasionally over-reaches.
func FindGroupedActivities(activity GroupCandidate, candidates []GroupCandidate) []string {
	if activity.Start == nil {
		return nil
	}
	ids := []string{}
	for _, c := range candidates {
		if c.ID == activity.ID || c.MemberID == activity.MemberID || c.Type != activity.Type || c.Start == nil {
			continue
		}
		gap := c.StartedAt.Sub(activity.StartedAt)
		if gap < 0 {
			gap = -gap
		}
		if gap > groupWindow {
			continue
		}
		if HaversineM(*c.Start, *activity.Start) <= groupRadiusM {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// ── Challenges, clubs, gear ──────────────────────────────────────────────────

// Challenge is a distance target over a window.
type Challenge struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Type is an ActivityType, or "ANY" for a challenge that counts everything.
	Type      string    `json:"type"`
	TargetKm  float64   `json:"targetKm"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ChallengeAnyType counts every kind of activity towards a challenge.
const ChallengeAnyType = "ANY"

// ChallengeContribution is one activity's claim on a challenge.
type ChallengeContribution struct {
	Type      ActivityType
	DistanceM float64
	StartedAt time.Time
}

// ChallengeProgressKm totals the activities that fall inside a challenge,
// rounded to one decimal — a leaderboard in metres is false precision.
func ChallengeProgressKm(challenge Challenge, activities []ChallengeContribution) float64 {
	total := 0.0
	for _, a := range activities {
		if challenge.Type != ChallengeAnyType && string(a.Type) != challenge.Type {
			continue
		}
		if a.StartedAt.Before(challenge.StartsAt) || a.StartedAt.After(challenge.EndsAt) {
			continue
		}
		total += a.DistanceM
	}
	return math.Round(total/100) / 10
}

// Club is a group of athletes with a weekly leaderboard.
type Club struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	MemberIDs   []string  `json:"memberIds"`
	CreatedAt   time.Time `json:"createdAt"`
}

// GearKind separates the two things that wear out.
type GearKind string

const (
	GearShoes GearKind = "SHOES"
	GearBike  GearKind = "BIKE"
)

func IsValidGearKind(value string) bool {
	return GearKind(value) == GearShoes || GearKind(value) == GearBike
}

// Gear is a pair of shoes or a bike, with the distance it has done on it.
type Gear struct {
	ID        string   `json:"id"`
	MemberID  string   `json:"memberId"`
	Name      string   `json:"name"`
	Kind      GearKind `json:"kind"`
	DistanceM float64  `json:"distanceM"`
	Retired   bool     `json:"retired"`
}

// Route is a saved track somebody means to run again.
type Route struct {
	ID        string       `json:"id"`
	MemberID  string       `json:"memberId"`
	Name      string       `json:"name"`
	Points    []TrackPoint `json:"points"`
	DistanceM float64      `json:"distanceM"`
	CreatedAt time.Time    `json:"createdAt"`
}

// AthleteSettings is what the member chose for themselves.
type AthleteSettings struct {
	Units            string   `json:"units"`
	BookingReminders bool     `json:"bookingReminders"`
	WeeklyGoalKm     *float64 `json:"weeklyGoalKm"`
	Language         string   `json:"language"`
}

// DefaultAthleteSettings is what an athlete who has never opened the settings
// screen gets: metric, reminders on, no goal set.
func DefaultAthleteSettings() AthleteSettings {
	return AthleteSettings{Units: "METRIC", BookingReminders: true, Language: "EN"}
}

func IsValidUnits(value string) bool    { return value == "METRIC" || value == "IMPERIAL" }
func IsValidLanguage(value string) bool { return value == "EN" || value == "ID" }

// ── Stats ────────────────────────────────────────────────────────────────────

// WeekBucket is one week of training.
type WeekBucket struct {
	WeekStart  time.Time `json:"weekStart"`
	DistanceKm float64   `json:"distanceKm"`
	Activities int       `json:"activities"`
	MovingSec  int       `json:"movingSec"`
}

// ActivitySummary is the little of an activity the statistics need.
type ActivitySummary struct {
	StartedAt time.Time
	DistanceM float64
	MovingSec int
}

// StartOfWeek is the Monday of the week containing t, in t's own location.
//
// Monday rather than Sunday because a training week is a training week
// everywhere this studio operates, and a chart that starts the week on Sunday
// splits every weekend long run across two bars.
func StartOfWeek(t time.Time) time.Time {
	day := (int(t.Weekday()) + 6) % 7
	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return midnight.AddDate(0, 0, -day)
}

// WeeklyBuckets is the last n weeks of training, oldest first, with empty
// weeks kept: a gap in the chart is the point of the chart.
func WeeklyBuckets(activities []ActivitySummary, now time.Time, weeks int) []WeekBucket {
	current := StartOfWeek(now)
	buckets := make([]WeekBucket, 0, weeks)
	for i := weeks - 1; i >= 0; i-- {
		start := current.AddDate(0, 0, -7*i)
		end := start.AddDate(0, 0, 7)
		bucket := WeekBucket{WeekStart: start}
		total := 0.0
		for _, a := range activities {
			if a.StartedAt.Before(start) || !a.StartedAt.Before(end) {
				continue
			}
			total += a.DistanceM
			bucket.Activities++
			bucket.MovingSec += a.MovingSec
		}
		bucket.DistanceKm = math.Round(total/100) / 10
		buckets = append(buckets, bucket)
	}
	return buckets
}

// PersonalRecords is the member's bests.
type PersonalRecords struct {
	Best1kPaceSec    *int    `json:"best1kPaceSec"`
	Best5kSec        *int    `json:"best5kSec"`
	Best10kSec       *int    `json:"best10kSec"`
	LongestDistanceM float64 `json:"longestDistanceM"`
	LongestMovingSec int     `json:"longestMovingSec"`
}

// ComputePersonalRecords reads the bests off a member's activities.
//
// The 5k and 10k figures are estimates: they scale a longer run's moving pace
// down to the distance rather than looking for an actual 5k inside it. The UI
// labels them as estimates, because a runner who sees a personal best they did
// not run stops trusting the rest of the screen.
func ComputePersonalRecords(activities []Activity) PersonalRecords {
	prs := PersonalRecords{}
	bestSplit := 0
	for _, a := range activities {
		if a.DistanceM > prs.LongestDistanceM {
			prs.LongestDistanceM = a.DistanceM
		}
		if a.MovingSec > prs.LongestMovingSec {
			prs.LongestMovingSec = a.MovingSec
		}
		if a.Type != ActivityRun || a.DistanceM <= 0 || a.MovingSec <= 0 {
			continue
		}
		if split := ComputeActivityStats(a.Points).BestSplitPaceSec; split != nil {
			if bestSplit == 0 || *split < bestSplit {
				bestSplit = *split
			}
		}
	}
	if bestSplit > 0 {
		prs.Best1kPaceSec = &bestSplit
	}

	estimate := func(meters float64) *int {
		best := 0
		for _, a := range activities {
			if a.Type != ActivityRun || a.DistanceM < meters || a.MovingSec <= 0 {
				continue
			}
			sec := int(math.Round(float64(a.MovingSec) * meters / a.DistanceM))
			if best == 0 || sec < best {
				best = sec
			}
		}
		if best == 0 {
			return nil
		}
		return &best
	}
	prs.Best5kSec = estimate(5000)
	prs.Best10kSec = estimate(10_000)
	return prs
}

// Downsample thins a track to at most max points, keeping the first and last.
//
// A two-hour ride is thousands of samples; a thumbnail in a feed is forty
// pixels wide. Sending the whole track for every card is the difference
// between a feed that opens and one that does not.
func Downsample(points []TrackPoint, max int) []TrackPoint {
	if max < 2 || len(points) <= max {
		out := make([]TrackPoint, len(points))
		copy(out, points)
		return out
	}
	step := float64(len(points)-1) / float64(max-1)
	out := make([]TrackPoint, max)
	for i := range out {
		out[i] = points[int(math.Round(float64(i)*step))]
	}
	return out
}

// SortActivitiesNewestFirst is the order every activity list is shown in.
func SortActivitiesNewestFirst(activities []Activity) {
	sort.SliceStable(activities, func(i, j int) bool {
		return activities[i].StartedAt.After(activities[j].StartedAt)
	})
}
