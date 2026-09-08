package domain_test

import (
	"math"
	"testing"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
)

// A straight line of points a fixed distance apart, one sample per second.
func track(points int, metresPerSample float64, secondsPerSample int64) []domain.TrackPoint {
	// 0.000009 degrees of latitude is almost exactly one metre.
	const degPerMetre = 1.0 / 111_320.0
	out := make([]domain.TrackPoint, points)
	for i := range out {
		out[i] = domain.TrackPoint{
			T:   int64(i) * secondsPerSample * 1000,
			Lat: -6.2 + float64(i)*metresPerSample*degPerMetre,
			Lng: 106.8,
		}
	}
	return out
}

func TestActivityStatsMeasuresAStraightRun(t *testing.T) {
	// 500 samples, 10 m apart, 5 s each: 4990 m in 2495 s, so just under 5 km.
	stats := domain.ComputeActivityStats(track(500, 10, 5))

	if math.Abs(stats.DistanceM-4990) > 15 {
		t.Fatalf("distance is %.0f m, want about 4990", stats.DistanceM)
	}
	if stats.ElapsedSec != 2495 {
		t.Fatalf("elapsed is %d s, want 2495", stats.ElapsedSec)
	}
	if stats.MovingSec != 2495 {
		t.Fatalf("moving is %d s: a constant 2 m/s should all count as moving", stats.MovingSec)
	}
	if stats.AvgPaceSecPerKm == nil || math.Abs(float64(*stats.AvgPaceSecPerKm)-500) > 5 {
		t.Fatalf("pace is %v, want about 500 s/km", stats.AvgPaceSecPerKm)
	}
	// Four full kilometres and a partial fifth.
	full := 0
	for _, s := range stats.Splits {
		if s.Full {
			full++
		}
	}
	if full != 4 || len(stats.Splits) != 5 {
		t.Fatalf("got %d splits (%d full), want 5 with 4 full", len(stats.Splits), full)
	}
	if stats.Splits[4].Full {
		t.Fatal("the trailing part-kilometre was reported as a full split")
	}
}

// A track that stops for a while: elapsed time includes the stop, moving time
// does not. Anything else and a coffee break ruins the pace.
func TestStoppedTimeIsNotMovingTime(t *testing.T) {
	points := track(60, 10, 5) // 59 samples of movement, 295 s
	last := points[len(points)-1]
	// Ten minutes standing still at the same spot.
	points = append(points, domain.TrackPoint{T: last.T + 600_000, Lat: last.Lat, Lng: last.Lng})

	stats := domain.ComputeActivityStats(points)
	if stats.ElapsedSec != 895 {
		t.Fatalf("elapsed is %d s, want 895 — the stop belongs in elapsed time", stats.ElapsedSec)
	}
	if stats.MovingSec != 295 {
		t.Fatalf("moving is %d s, want 295 — the stop does not belong in moving time", stats.MovingSec)
	}
}

func TestShortAndEmptyTracks(t *testing.T) {
	empty := domain.ComputeActivityStats(nil)
	if empty.DistanceM != 0 || empty.AvgPaceSecPerKm != nil || len(empty.Splits) != 0 {
		t.Fatalf("an empty track produced %+v", empty)
	}
	// Ten metres is not a pace, it is a GPS fix settling down.
	short := domain.ComputeActivityStats(track(2, 10, 5))
	if short.AvgPaceSecPerKm != nil {
		t.Fatalf("a ten-metre track reported a pace of %d s/km", *short.AvgPaceSecPerKm)
	}
}

func TestElevationIgnoresJitter(t *testing.T) {
	points := track(11, 10, 5)
	// A barometer wobbling by 10 cm each sample, ending where it started.
	for i := range points {
		ele := 100.0
		if i%2 == 1 {
			ele = 100.1
		}
		points[i].Ele = &ele
	}
	if gain := domain.ComputeActivityStats(points).ElevationGainM; gain != 0 {
		t.Fatalf("jitter accumulated %.1f m of climb", gain)
	}

	// A real climb is counted.
	for i := range points {
		ele := 100 + float64(i)
		points[i].Ele = &ele
	}
	if gain := domain.ComputeActivityStats(points).ElevationGainM; gain != 10 {
		t.Fatalf("a 10 m climb measured %.1f m", gain)
	}
}

func TestVisibility(t *testing.T) {
	follows := func(id string) bool { return id == "mem_followed" }

	cases := []struct {
		name       string
		owner      string
		visibility domain.ActivityVisibility
		want       bool
	}{
		{"my own private activity", "mem_me", domain.VisibilityPrivate, true},
		{"somebody else's private activity", "mem_other", domain.VisibilityPrivate, false},
		{"a public activity", "mem_other", domain.VisibilityEveryone, true},
		{"followers-only, and I follow them", "mem_followed", domain.VisibilityFollowers, true},
		{"followers-only, and I do not", "mem_other", domain.VisibilityFollowers, false},
	}
	for _, c := range cases {
		if got := domain.CanViewActivity(c.owner, c.visibility, "mem_me", follows); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSegmentMatching(t *testing.T) {
	// A 1 km segment along the first kilometre of a 2 km run.
	full := track(200, 10, 5)
	segment := domain.Segment{
		ID: "seg_1", Type: domain.ActivityRun, DistanceM: 1000,
		Path: []domain.TrackPoint{full[0], full[100]},
	}

	matches := domain.MatchSegments([]domain.Segment{segment}, domain.ActivityRun, full)
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}
	// The gates have a 60 m radius, so the effort ends as soon as the track
	// comes within that of the end point — six samples, 30 s, before the
	// nominal kilometre. A gate is a gate, not a finish line photo.
	if matches[0].ElapsedSec != 470 {
		t.Fatalf("effort was %d s, want 470 (500 s to the point, less the gate radius)", matches[0].ElapsedSec)
	}

	// The same track on a bike does not count for a running segment.
	if got := domain.MatchSegments([]domain.Segment{segment}, domain.ActivityRide, full); len(got) != 0 {
		t.Fatal("a ride matched a running segment")
	}

	// A track that never reaches the start gate does not match.
	away := track(200, 10, 5)
	for i := range away {
		away[i].Lng = 107.5
	}
	if got := domain.MatchSegments([]domain.Segment{segment}, domain.ActivityRun, away); len(got) != 0 {
		t.Fatal("a run on the other side of the city matched the segment")
	}
}

// Passing through both gates the long way round is not the segment.
func TestSegmentRejectsTheScenicRoute(t *testing.T) {
	full := track(400, 10, 5)
	segment := domain.Segment{
		ID: "seg_1", Type: domain.ActivityRun, DistanceM: 1000,
		// Gates 1 km apart, but the track between them covers 4 km.
		Path: []domain.TrackPoint{full[0], full[399]},
	}
	segment.DistanceM = 1000
	if got := domain.MatchSegments([]domain.Segment{segment}, domain.ActivityRun, full); len(got) != 0 {
		t.Fatal("a 4 km route between the gates counted as a 1 km segment")
	}
}

func TestGroupedActivities(t *testing.T) {
	start := domain.TrackPoint{T: 0, Lat: -6.2, Lng: 106.8}
	nearby := domain.TrackPoint{T: 0, Lat: -6.2005, Lng: 106.8} // ~55 m away
	faraway := domain.TrackPoint{T: 0, Lat: -6.3, Lng: 106.8}   // ~11 km away
	now := time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)

	mine := domain.GroupCandidate{
		ID: "act_me", MemberID: "mem_me", Type: domain.ActivityRun, StartedAt: now, Start: &start,
	}
	candidates := []domain.GroupCandidate{
		{ID: "act_together", MemberID: "mem_a", Type: domain.ActivityRun, StartedAt: now.Add(4 * time.Minute), Start: &nearby},
		{ID: "act_elsewhere", MemberID: "mem_b", Type: domain.ActivityRun, StartedAt: now, Start: &faraway},
		{ID: "act_later", MemberID: "mem_c", Type: domain.ActivityRun, StartedAt: now.Add(3 * time.Hour), Start: &nearby},
		{ID: "act_cycling", MemberID: "mem_d", Type: domain.ActivityRide, StartedAt: now, Start: &nearby},
		{ID: "act_mine_again", MemberID: "mem_me", Type: domain.ActivityRun, StartedAt: now, Start: &nearby},
	}

	got := domain.FindGroupedActivities(mine, candidates)
	if len(got) != 1 || got[0] != "act_together" {
		t.Fatalf("grouped with %v, want only act_together", got)
	}
}

func TestChallengeProgressCountsOnlyWhatItShould(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	challenge := domain.Challenge{
		Type: string(domain.ActivityRun), TargetKm: 50,
		StartsAt: from, EndsAt: from.AddDate(0, 0, 30),
	}
	activities := []domain.ChallengeContribution{
		{Type: domain.ActivityRun, DistanceM: 10_000, StartedAt: from.AddDate(0, 0, 1)},
		{Type: domain.ActivityRun, DistanceM: 5_500, StartedAt: from.AddDate(0, 0, 2)},
		{Type: domain.ActivityRide, DistanceM: 40_000, StartedAt: from.AddDate(0, 0, 3)}, // wrong type
		{Type: domain.ActivityRun, DistanceM: 10_000, StartedAt: from.AddDate(0, 0, -1)}, // before it opened
		{Type: domain.ActivityRun, DistanceM: 10_000, StartedAt: from.AddDate(0, 0, 40)}, // after it closed
	}
	if got := domain.ChallengeProgressKm(challenge, activities); got != 15.5 {
		t.Fatalf("progress is %.1f km, want 15.5", got)
	}

	// An ANY challenge counts the ride too.
	challenge.Type = domain.ChallengeAnyType
	if got := domain.ChallengeProgressKm(challenge, activities); got != 55.5 {
		t.Fatalf("an ANY challenge scored %.1f km, want 55.5", got)
	}
}

func TestWeeklyBucketsKeepEmptyWeeks(t *testing.T) {
	// A Wednesday.
	now := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	monday := domain.StartOfWeek(now)
	if monday.Weekday() != time.Monday || monday.Day() != 2 {
		t.Fatalf("week starts on %s the %d, want Monday the 2nd", monday.Weekday(), monday.Day())
	}

	buckets := domain.WeeklyBuckets([]domain.ActivitySummary{
		{StartedAt: now.AddDate(0, 0, -1), DistanceM: 8_000, MovingSec: 2400},
		{StartedAt: now.AddDate(0, 0, -15), DistanceM: 12_000, MovingSec: 3600},
	}, now, 4)

	if len(buckets) != 4 {
		t.Fatalf("got %d buckets, want 4", len(buckets))
	}
	if buckets[3].DistanceKm != 8 || buckets[3].Activities != 1 {
		t.Fatalf("this week is %+v, want 8 km over one activity", buckets[3])
	}
	if buckets[1].DistanceKm != 12 {
		t.Fatalf("two weeks ago is %.1f km, want 12", buckets[1].DistanceKm)
	}
	// The quiet week is present and zero, not missing.
	if buckets[2].Activities != 0 || buckets[2].DistanceKm != 0 {
		t.Fatalf("the quiet week reads %+v, want zeroes", buckets[2])
	}
	if !buckets[0].WeekStart.Before(buckets[3].WeekStart) {
		t.Fatal("buckets are not oldest first")
	}
}

func TestPersonalRecords(t *testing.T) {
	now := time.Now()
	activities := []domain.Activity{
		{Type: domain.ActivityRun, DistanceM: 10_000, MovingSec: 3000, StartedAt: now, Points: track(200, 10, 5)},
		{Type: domain.ActivityRun, DistanceM: 5_000, MovingSec: 1400, StartedAt: now, Points: track(100, 10, 5)},
		{Type: domain.ActivityRide, DistanceM: 40_000, MovingSec: 4800, StartedAt: now},
	}
	prs := domain.ComputePersonalRecords(activities)

	if prs.Best5kSec == nil || *prs.Best5kSec != 1400 {
		t.Fatalf("best 5k is %v, want 1400 — the actual 5k, not the scaled 10k", prs.Best5kSec)
	}
	if prs.Best10kSec == nil || *prs.Best10kSec != 3000 {
		t.Fatalf("best 10k is %v, want 3000", prs.Best10kSec)
	}
	// Longest counts the ride: it is the longest thing they did.
	if prs.LongestDistanceM != 40_000 {
		t.Fatalf("longest is %.0f m, want 40000", prs.LongestDistanceM)
	}
	if prs.Best1kPaceSec == nil {
		t.Fatal("no best kilometre from two runs with tracks")
	}
}

func TestPersonalRecordsWithNoRuns(t *testing.T) {
	prs := domain.ComputePersonalRecords([]domain.Activity{
		{Type: domain.ActivityRide, DistanceM: 40_000, MovingSec: 4800},
	})
	if prs.Best5kSec != nil || prs.Best10kSec != nil || prs.Best1kPaceSec != nil {
		t.Fatalf("a cyclist was given running records: %+v", prs)
	}
}

func TestDownsampleKeepsTheEnds(t *testing.T) {
	points := track(1000, 10, 5)
	thin := domain.Downsample(points, 40)

	if len(thin) != 40 {
		t.Fatalf("got %d points, want 40", len(thin))
	}
	if thin[0] != points[0] || thin[39] != points[999] {
		t.Fatal("downsampling dropped the start or the end of the track")
	}
	// A track already short enough comes back whole.
	if got := domain.Downsample(points[:10], 40); len(got) != 10 {
		t.Fatalf("a 10-point track came back with %d points", len(got))
	}
}
