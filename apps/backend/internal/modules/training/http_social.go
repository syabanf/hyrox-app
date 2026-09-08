package training

import (
	"net/http"
	"sort"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// The screens that put one athlete next to another: segment leaderboards,
// challenges, clubs, and the follow graph.

// leaderboardRows is how many places a board shows. Past five, a leaderboard
// stops being a nudge and starts being a table.
const leaderboardRows = 5

// ── Segments ─────────────────────────────────────────────────────────────────

func (h *Handler) segments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	segments, err := h.service.repo.Segments(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	efforts, err := h.service.repo.AllEfforts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bySegment := map[string][]domain.SegmentEffort{}
	for _, e := range efforts {
		bySegment[e.SegmentID] = append(bySegment[e.SegmentID], e)
	}

	type segmentRow struct {
		Segment          domain.Segment `json:"segment"`
		EffortCount      int            `json:"effortCount"`
		BestElapsedSec   *int           `json:"bestElapsedSec"`
		MyBestElapsedSec *int           `json:"myBestElapsedSec"`
		MyRank           *int           `json:"myRank"`
	}

	out := make([]segmentRow, 0, len(segments))
	for _, segment := range segments {
		all := bySegment[segment.ID]
		board := bestPerMember(all)
		row := segmentRow{Segment: segment, EffortCount: len(all)}
		if len(board) > 0 {
			best := board[0].ElapsedSec
			row.BestElapsedSec = &best
		}
		for i, entry := range board {
			if entry.MemberID == me {
				rank, mine := i+1, entry.ElapsedSec
				row.MyRank, row.MyBestElapsedSec = &rank, &mine
				break
			}
		}
		out = append(out, row)
	}
	httpx.OK(w, out)
}

func (h *Handler) segment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	segment, err := h.service.repo.Segment(ctx, httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	efforts, err := h.service.repo.Efforts(ctx, segment.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	board := bestPerMember(efforts)

	ids := map[string]bool{}
	for _, e := range board {
		ids[e.MemberID] = true
	}
	athletes, err := h.service.members.Athletes(ctx, keys(ids))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	type boardRow struct {
		Rank       int       `json:"rank"`
		MemberID   string    `json:"memberId"`
		MemberName string    `json:"memberName"`
		ElapsedSec int       `json:"elapsedSec"`
		CreatedAt  time.Time `json:"createdAt"`
		IsMe       bool      `json:"isMe"`
	}

	// Twenty places is a leaderboard somebody scrolls; the full field on a
	// popular segment is a database dump.
	const boardSize = 20
	rows := []boardRow{}
	var myRank *int
	for i, effort := range board {
		if i == 0 || effort.MemberID == me {
			if effort.MemberID == me {
				rank := i + 1
				myRank = &rank
			}
		}
		if i >= boardSize {
			continue
		}
		name := "Athlete"
		if athlete, ok := athletes[effort.MemberID]; ok {
			name = athlete.Name
		}
		rows = append(rows, boardRow{
			Rank: i + 1, MemberID: effort.MemberID, MemberName: name,
			ElapsedSec: effort.ElapsedSec, CreatedAt: effort.CreatedAt,
			IsMe: effort.MemberID == me,
		})
	}

	httpx.OK(w, map[string]any{"segment": segment, "leaderboard": rows, "myRank": myRank})
}

// ── Challenges ───────────────────────────────────────────────────────────────

func (h *Handler) challenges(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	challenges, err := h.service.challenges.Running(ctx, h.service.clock.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	participants, err := h.service.challenges.Participants(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Every leaderboard is computed from the same activities, so they are
	// fetched once rather than once per challenge.
	activities, err := h.service.repo.FeedCandidates(ctx, feedLimit*5)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	byMember := map[string][]domain.ChallengeContribution{}
	for _, a := range activities {
		byMember[a.MemberID] = append(byMember[a.MemberID],
			domain.ChallengeContribution{Type: a.Type, DistanceM: a.DistanceM, StartedAt: a.StartedAt})
	}

	names := map[string]bool{}
	for _, ids := range participants {
		for _, memberID := range ids {
			names[memberID] = true
		}
	}
	athletes, err := h.service.members.Athletes(ctx, keys(names))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	type challengeRow struct {
		Challenge        domain.Challenge   `json:"challenge"`
		Joined           bool               `json:"joined"`
		ParticipantCount int                `json:"participantCount"`
		ProgressKm       float64            `json:"progressKm"`
		Leaderboard      []leaderboardEntry `json:"leaderboard"`
	}

	out := make([]challengeRow, 0, len(challenges))
	for _, challenge := range challenges {
		joined := false
		rows := []leaderboardEntry{}
		for _, memberID := range participants[challenge.ID] {
			if memberID == me {
				joined = true
			}
			name := "Athlete"
			if athlete, ok := athletes[memberID]; ok {
				name = athlete.Name
			}
			rows = append(rows, leaderboardEntry{
				MemberName: name,
				Km:         domain.ChallengeProgressKm(challenge, byMember[memberID]),
				IsMe:       memberID == me,
			})
		}
		out = append(out, challengeRow{
			Challenge:        challenge,
			Joined:           joined,
			ParticipantCount: len(participants[challenge.ID]),
			ProgressKm:       domain.ChallengeProgressKm(challenge, byMember[me]),
			Leaderboard:      topOf(rows),
		})
	}
	httpx.OK(w, out)
}

type leaderboardEntry struct {
	MemberName string  `json:"memberName"`
	Km         float64 `json:"km"`
	IsMe       bool    `json:"isMe"`
}

func topOf(rows []leaderboardEntry) []leaderboardEntry {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Km > rows[j].Km })
	if len(rows) > leaderboardRows {
		rows = rows[:leaderboardRows]
	}
	return rows
}

func (h *Handler) joinChallenge(w http.ResponseWriter, r *http.Request) {
	err := h.service.challenges.Join(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"joined": true})
}

// ── Clubs ────────────────────────────────────────────────────────────────────

func (h *Handler) clubs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	clubs, err := h.service.repo.Clubs(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	weekly, athletes, err := h.weeklyKm(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	type clubRow struct {
		Club              domain.Club        `json:"club"`
		Joined            bool               `json:"joined"`
		MemberCount       int                `json:"memberCount"`
		WeeklyLeaderboard []leaderboardEntry `json:"weeklyLeaderboard"`
	}

	out := make([]clubRow, 0, len(clubs))
	for _, club := range clubs {
		joined := false
		rows := []leaderboardEntry{}
		for _, memberID := range club.MemberIDs {
			if memberID == me {
				joined = true
			}
			name := "Athlete"
			if athlete, ok := athletes[memberID]; ok {
				name = athlete.Name
			}
			rows = append(rows, leaderboardEntry{MemberName: name, Km: weekly[memberID], IsMe: memberID == me})
		}
		out = append(out, clubRow{
			Club: club, Joined: joined, MemberCount: len(club.MemberIDs),
			WeeklyLeaderboard: topOf(rows),
		})
	}
	httpx.OK(w, out)
}

func (h *Handler) toggleClub(w http.ResponseWriter, r *http.Request) {
	joined, err := h.service.repo.ToggleClubMembership(r.Context(),
		httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"joined": joined})
}

// weeklyKm is this week's distance for everybody who has trained recently,
// which is what both the club boards and the follow suggestions rank on.
func (h *Handler) weeklyKm(r *http.Request) (map[string]float64, map[string]Athlete, error) {
	ctx := r.Context()
	activities, err := h.service.repo.FeedCandidates(ctx, feedLimit*5)
	if err != nil {
		return nil, nil, err
	}
	weekStart := domain.StartOfWeek(h.service.clock.Now())

	metres := map[string]float64{}
	ids := map[string]bool{}
	for _, a := range activities {
		ids[a.MemberID] = true
		if !a.StartedAt.Before(weekStart) {
			metres[a.MemberID] += a.DistanceM
		}
	}
	km := make(map[string]float64, len(metres))
	for memberID, m := range metres {
		km[memberID] = roundKm(m)
	}

	athletes, err := h.service.members.Athletes(ctx, keys(ids))
	if err != nil {
		return nil, nil, err
	}
	return km, athletes, nil
}

// ── Social graph ─────────────────────────────────────────────────────────────

// suggestionCount is how many people to offer following. A wall of strangers
// is ignored; a handful is a decision.
const suggestionCount = 8

func (h *Handler) social(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	following, err := h.service.repo.Following(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	followers, err := h.service.repo.Followers(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	weekly, _, err := h.weeklyKm(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Suggestions are active members who train and whom the viewer does not
	// already follow. Suggesting somebody with an empty profile is a dead end.
	active, err := h.service.members.ActiveAthletes(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	trains := map[string]bool{}
	activities, err := h.service.repo.FeedCandidates(ctx, feedLimit*5)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for _, a := range activities {
		trains[a.MemberID] = true
	}

	suggestions := []string{}
	for _, athlete := range active {
		if athlete.ID == me || following[athlete.ID] || !trains[athlete.ID] {
			continue
		}
		suggestions = append(suggestions, athlete.ID)
		if len(suggestions) >= suggestionCount {
			break
		}
	}

	all := map[string]bool{}
	for id := range following {
		all[id] = true
	}
	for _, id := range followers {
		all[id] = true
	}
	for _, id := range suggestions {
		all[id] = true
	}
	athletes, err := h.service.members.Athletes(ctx, keys(all))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	lite := func(ids []string) []athleteLite {
		out := make([]athleteLite, 0, len(ids))
		for _, id := range ids {
			name := "Athlete"
			if athlete, ok := athletes[id]; ok {
				name = athlete.Name
			}
			out = append(out, athleteLite{
				MemberID: id, Name: name, WeeklyKm: weekly[id], IsFollowing: following[id],
			})
		}
		return out
	}

	httpx.OK(w, map[string]any{
		"following":   lite(keys(following)),
		"followers":   lite(followers),
		"suggestions": lite(suggestions),
	})
}

type athleteLite struct {
	MemberID    string  `json:"memberId"`
	Name        string  `json:"name"`
	WeeklyKm    float64 `json:"weeklyKm"`
	IsFollowing bool    `json:"isFollowing"`
}

func (h *Handler) toggleFollow(w http.ResponseWriter, r *http.Request) {
	following, err := h.service.ToggleFollow(r.Context(),
		auth.MemberID(r.Context()), httpx.Param(r, "memberId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"following": following})
}

// profileActivities is how much of somebody else's log a profile shows.
const profileActivities = 20

func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)
	targetID := httpx.Param(r, "memberId")

	athletes, err := h.service.members.Athletes(ctx, []string{targetID})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target, ok := athletes[targetID]
	if !ok {
		httpx.Fail(w, r, httpx.NotFound("member"))
		return
	}

	following, err := h.service.repo.Following(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	activities, err := h.service.repo.ActivitiesForMember(ctx, targetID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	visible := []domain.Activity{}
	totalDistance, totalMoving := 0.0, 0
	for _, a := range activities {
		if !domain.CanViewActivity(a.MemberID, a.Visibility, me, func(id string) bool { return following[id] }) {
			continue
		}
		visible = append(visible, a)
		totalDistance += a.DistanceM
		totalMoving += a.MovingSec
	}

	shown := visible
	if len(shown) > profileActivities {
		shown = shown[:profileActivities]
	}
	cards, err := h.cards(r, me, shown)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	targetFollowing, targetFollowers, err := h.service.repo.CountFollows(ctx, targetID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	httpx.OK(w, map[string]any{
		"member": map[string]any{
			"id": target.ID, "fullName": target.Name, "avatarUrl": target.AvatarURL,
		},
		"isMe":           targetID == me,
		"isFollowing":    following[targetID],
		"followerCount":  targetFollowers,
		"followingCount": targetFollowing,
		"totals": map[string]any{
			"activities": len(visible),
			"distanceKm": totalDistance / 1000,
			"movingSec":  totalMoving,
		},
		"activities": cards,
	})
}
