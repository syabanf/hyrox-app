package training

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Handler serves the training surface: the athlete tab, the workout player and
// the race calendar.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	member := h.guard.RequireMember

	// Settings.
	r.Get("/api/me/settings", h.getSettings, member)
	r.Put("/api/me/settings", h.updateSettings, member)

	// Activities and the feed.
	r.Get("/api/athlete/feed", h.feed, member)
	r.Get("/api/athlete/activities", h.myActivities, member)
	r.Post("/api/athlete/activities", h.saveActivity, member)
	r.Get("/api/athlete/activities/{id}", h.activity, member)
	r.Patch("/api/athlete/activities/{id}", h.updateActivity, member)
	r.Delete("/api/athlete/activities/{id}", h.deleteActivity, member)
	r.Post("/api/athlete/activities/{id}/kudos", h.toggleKudos, member)
	r.Post("/api/athlete/activities/{id}/comments", h.comment, member)

	// Routes and the heatmap.
	r.Get("/api/athlete/routes", h.routes, member)
	r.Post("/api/athlete/routes", h.saveRoute, member)
	r.Get("/api/athlete/routes/{id}", h.route, member)
	r.Delete("/api/athlete/routes/{id}", h.deleteRoute, member)
	r.Get("/api/athlete/heatmap", h.heatmap, member)

	// Statistics and gear.
	r.Get("/api/athlete/stats", h.stats, member)
	r.Post("/api/athlete/gear", h.createGear, member)
	r.Patch("/api/athlete/gear/{id}", h.updateGear, member)

	// Segments, challenges, clubs and the social graph.
	r.Get("/api/athlete/segments", h.segments, member)
	r.Get("/api/athlete/segments/{id}", h.segment, member)
	r.Get("/api/athlete/challenges", h.challenges, member)
	r.Post("/api/athlete/challenges/{id}/join", h.joinChallenge, member)
	r.Get("/api/athlete/clubs", h.clubs, member)
	r.Post("/api/athlete/clubs/{id}/toggle", h.toggleClub, member)
	r.Get("/api/athlete/social", h.social, member)
	r.Get("/api/athlete/profile/{memberId}", h.profile, member)
	r.Post("/api/athlete/follow/{memberId}", h.toggleFollow, member)

	h.mountWorkouts(r)
}

// ── Views ────────────────────────────────────────────────────────────────────

// thumbnailPoints is how much of a track a feed card draws. Forty points is a
// recognisable shape at the size a card renders it.
const thumbnailPoints = 40

type activityCard struct {
	ID              string                    `json:"id"`
	MemberID        string                    `json:"memberId"`
	MemberName      string                    `json:"memberName"`
	MemberAvatarURL *string                   `json:"memberAvatarUrl"`
	IsOwn           bool                      `json:"isOwn"`
	Type            domain.ActivityType       `json:"type"`
	Title           string                    `json:"title"`
	StartedAt       time.Time                 `json:"startedAt"`
	DistanceM       float64                   `json:"distanceM"`
	MovingSec       int                       `json:"movingSec"`
	ElapsedSec      int                       `json:"elapsedSec"`
	AvgPaceSecPerKm *int                      `json:"avgPaceSecPerKm"`
	ElevationGainM  float64                   `json:"elevationGainM"`
	Visibility      domain.ActivityVisibility `json:"visibility"`
	Thumbnail       []domain.TrackPoint       `json:"thumbnail"`
	PhotoCount      int                       `json:"photoCount"`
	KudosCount      int                       `json:"kudosCount"`
	HasKudoed       bool                      `json:"hasKudoed"`
	CommentCount    int                       `json:"commentCount"`
}

// cardBuilder holds the per-request lookups a batch of cards needs, so a feed
// of thirty activities is four queries rather than a hundred and twenty.
type cardBuilder struct {
	viewer   string
	athletes map[string]Athlete
	kudos    map[string]int
	kudoed   map[string]bool
	comments map[string]int
}

func (b cardBuilder) card(a domain.Activity) activityCard {
	name, avatar := "Athlete", (*string)(nil)
	if athlete, ok := b.athletes[a.MemberID]; ok {
		name, avatar = athlete.Name, athlete.AvatarURL
	}
	return activityCard{
		ID: a.ID, MemberID: a.MemberID, MemberName: name, MemberAvatarURL: avatar,
		IsOwn: a.MemberID == b.viewer,
		Type:  a.Type, Title: a.Title, StartedAt: a.StartedAt,
		DistanceM: a.DistanceM, MovingSec: a.MovingSec, ElapsedSec: a.ElapsedSec,
		AvgPaceSecPerKm: a.AvgPaceSecPerKm, ElevationGainM: a.ElevationGainM,
		Visibility: a.Visibility,
		Thumbnail:  domain.Downsample(a.Points, thumbnailPoints),
		PhotoCount: len(a.Photos),
		KudosCount: b.kudos[a.ID], HasKudoed: b.kudoed[a.ID], CommentCount: b.comments[a.ID],
	}
}

// cards assembles the lookups for a batch of activities and renders them.
func (h *Handler) cards(r *http.Request, viewer string, activities []domain.Activity) ([]activityCard, error) {
	ctx := r.Context()
	ids := make([]string, 0, len(activities))
	memberIDs := map[string]bool{}
	for _, a := range activities {
		ids = append(ids, a.ID)
		memberIDs[a.MemberID] = true
	}

	athletes, err := h.service.members.Athletes(ctx, keys(memberIDs))
	if err != nil {
		return nil, err
	}
	kudos, kudoed, err := h.service.repo.KudosCounts(ctx, ids, viewer)
	if err != nil {
		return nil, err
	}
	comments, err := h.service.repo.CommentCounts(ctx, ids)
	if err != nil {
		return nil, err
	}

	builder := cardBuilder{viewer: viewer, athletes: athletes, kudos: kudos, kudoed: kudoed, comments: comments}
	out := make([]activityCard, 0, len(activities))
	for _, a := range activities {
		out = append(out, builder.card(a))
	}
	return out, nil
}

// ── Feed and activities ──────────────────────────────────────────────────────

// feedSize is one screen's worth of scrolling. Beyond this the feed is a
// history, and history has its own screen.
const feedSize = 30

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	me := auth.MemberID(r.Context())
	followingOnly := r.URL.Query().Get("scope") == "following"

	activities, err := h.service.Feed(r.Context(), me, followingOnly, feedSize)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cards, err := h.cards(r, me, activities)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, cards)
}

func (h *Handler) myActivities(w http.ResponseWriter, r *http.Request) {
	me := auth.MemberID(r.Context())
	activities, err := h.service.MyActivities(r.Context(), me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cards, err := h.cards(r, me, activities)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, cards)
}

type saveActivityRequest struct {
	Type        string              `json:"type"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	StartedAt   *time.Time          `json:"startedAt"`
	Points      []domain.TrackPoint `json:"points"`
	Photos      []string            `json:"photos"`
	Visibility  string              `json:"visibility"`
	GearID      *string             `json:"gearId"`
	ElapsedSec  int                 `json:"elapsedSec"`
	MovingSec   int                 `json:"movingSec"`
	DistanceM   float64             `json:"distanceM"`
}

func (a *saveActivityRequest) Validate() error {
	if !domain.IsValidActivityType(a.Type) {
		return httpx.Invalid("An activity is a RUN, RIDE, WALK or WORKOUT.")
	}
	if len([]rune(a.Title)) > 120 {
		return httpx.Invalid("That title is too long.")
	}
	return nil
}

func (h *Handler) saveActivity(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[saveActivityRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := SaveActivityInput{
		Type: domain.ActivityType(body.Type), Title: body.Title, Description: body.Description,
		Points: body.Points, Photos: body.Photos,
		Visibility: domain.ActivityVisibility(body.Visibility), GearID: body.GearID,
		ElapsedSec: body.ElapsedSec, MovingSec: body.MovingSec, DistanceM: body.DistanceM,
	}
	if body.StartedAt != nil {
		in.StartedAt = *body.StartedAt
	}

	me := auth.MemberID(r.Context())
	activity, err := h.service.SaveActivity(r.Context(), me, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cards, err := h.cards(r, me, []domain.Activity{activity})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, cards[0])
}

func (h *Handler) activity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	activity, err := h.service.Activity(ctx, httpx.Param(r, "id"), me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cards, err := h.cards(r, me, []domain.Activity{activity})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	stats := domain.ComputeActivityStats(activity.Points)
	efforts, err := h.effortViews(r, activity)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	comments, err := h.commentViews(r, activity.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	grouped, err := h.groupedWith(r, activity)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var gearName *string
	if activity.GearID != nil {
		if gear, err := h.service.repo.GearItem(ctx, *activity.GearID); err == nil {
			gearName = &gear.Name
		}
	}

	httpx.OK(w, struct {
		activityCard
		Description      string                 `json:"description"`
		Points           []domain.TrackPoint    `json:"points"`
		Photos           []string               `json:"photos"`
		Splits           []domain.ActivitySplit `json:"splits"`
		BestSplitPaceSec *int                   `json:"bestSplitPaceSec"`
		GearName         *string                `json:"gearName"`
		Efforts          []effortView           `json:"efforts"`
		Comments         []commentView          `json:"comments"`
		GroupedWith      []groupedView          `json:"groupedWith"`
	}{
		activityCard:     cards[0],
		Description:      activity.Description,
		Points:           activity.Points,
		Photos:           activity.Photos,
		Splits:           stats.Splits,
		BestSplitPaceSec: stats.BestSplitPaceSec,
		GearName:         gearName,
		Efforts:          efforts,
		Comments:         comments,
		GroupedWith:      grouped,
	})
}

type effortView struct {
	SegmentID      string  `json:"segmentId"`
	SegmentName    string  `json:"segmentName"`
	DistanceM      float64 `json:"distanceM"`
	ElapsedSec     int     `json:"elapsedSec"`
	Rank           int     `json:"rank"`
	TotalEfforts   int     `json:"totalEfforts"`
	IsPersonalBest bool    `json:"isPersonalBest"`
}

// effortViews places each of this activity's efforts on its leaderboard.
func (h *Handler) effortViews(r *http.Request, activity domain.Activity) ([]effortView, error) {
	ctx := r.Context()
	mine, err := h.service.repo.EffortsForActivity(ctx, activity.ID)
	if err != nil {
		return nil, err
	}
	out := []effortView{}
	for _, effort := range mine {
		segment, err := h.service.repo.Segment(ctx, effort.SegmentID)
		if err != nil {
			continue
		}
		board, err := h.service.repo.Efforts(ctx, effort.SegmentID)
		if err != nil {
			return nil, err
		}
		leaders := bestPerMember(board)

		rank, personalBest := 0, effort.ElapsedSec
		for i, row := range leaders {
			if row.MemberID == effort.MemberID {
				rank = i + 1
				personalBest = row.ElapsedSec
			}
		}
		out = append(out, effortView{
			SegmentID: segment.ID, SegmentName: segment.Name, DistanceM: segment.DistanceM,
			ElapsedSec: effort.ElapsedSec, Rank: rank, TotalEfforts: len(leaders),
			IsPersonalBest: effort.ElapsedSec <= personalBest,
		})
	}
	return out, nil
}

// bestPerMember reduces a segment's efforts to one row per athlete — their
// best — ordered fastest first. A leaderboard where one person holds the top
// five places is a list of one person's mornings.
func bestPerMember(efforts []domain.SegmentEffort) []domain.SegmentEffort {
	best := map[string]domain.SegmentEffort{}
	for _, e := range efforts {
		if current, ok := best[e.MemberID]; !ok || e.ElapsedSec < current.ElapsedSec {
			best[e.MemberID] = e
		}
	}
	out := make([]domain.SegmentEffort, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ElapsedSec < out[j].ElapsedSec })
	return out
}

type commentView struct {
	ID         string    `json:"id"`
	MemberID   string    `json:"memberId"`
	MemberName string    `json:"memberName"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (h *Handler) commentViews(r *http.Request, activityID string) ([]commentView, error) {
	ctx := r.Context()
	comments, err := h.service.repo.Comments(ctx, activityID)
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, c := range comments {
		ids[c.MemberID] = true
	}
	athletes, err := h.service.members.Athletes(ctx, keys(ids))
	if err != nil {
		return nil, err
	}

	out := make([]commentView, 0, len(comments))
	for _, c := range comments {
		name := "Athlete"
		if athlete, ok := athletes[c.MemberID]; ok {
			name = athlete.Name
		}
		out = append(out, commentView{
			ID: c.ID, MemberID: c.MemberID, MemberName: name,
			Text: c.Text, CreatedAt: c.CreatedAt,
		})
	}
	return out, nil
}

type groupedView struct {
	ActivityID string `json:"activityId"`
	MemberName string `json:"memberName"`
}

// groupedWith is "you trained with these people": nearby, at the same time,
// doing the same thing.
func (h *Handler) groupedWith(r *http.Request, activity domain.Activity) ([]groupedView, error) {
	ctx := r.Context()
	candidates, err := h.service.repo.FeedCandidates(ctx, feedLimit)
	if err != nil {
		return nil, err
	}

	toCandidate := func(a domain.Activity) domain.GroupCandidate {
		c := domain.GroupCandidate{ID: a.ID, MemberID: a.MemberID, Type: a.Type, StartedAt: a.StartedAt}
		if len(a.Points) > 0 {
			c.Start = &a.Points[0]
		}
		return c
	}
	pool := make([]domain.GroupCandidate, 0, len(candidates))
	byID := map[string]domain.Activity{}
	for _, a := range candidates {
		pool = append(pool, toCandidate(a))
		byID[a.ID] = a
	}

	matched := domain.FindGroupedActivities(toCandidate(activity), pool)
	ids := map[string]bool{}
	for _, activityID := range matched {
		ids[byID[activityID].MemberID] = true
	}
	athletes, err := h.service.members.Athletes(ctx, keys(ids))
	if err != nil {
		return nil, err
	}

	out := []groupedView{}
	for _, activityID := range matched {
		name := "Athlete"
		if athlete, ok := athletes[byID[activityID].MemberID]; ok {
			name = athlete.Name
		}
		out = append(out, groupedView{ActivityID: activityID, MemberName: name})
	}
	return out, nil
}

type updateActivityRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Visibility  *string  `json:"visibility"`
	GearID      **string `json:"gearId"`
}

func (u *updateActivityRequest) Validate() error {
	if u.Visibility != nil && !domain.IsValidVisibility(*u.Visibility) {
		return httpx.Invalid("Visibility is EVERYONE, FOLLOWERS or PRIVATE.")
	}
	return nil
}

func (h *Handler) updateActivity(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[updateActivityRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	patch := ActivityPatch{Title: body.Title, Description: body.Description}
	if body.Visibility != nil {
		visibility := domain.ActivityVisibility(*body.Visibility)
		patch.Visibility = &visibility
	}
	if body.GearID != nil {
		patch.SetGear = true
		patch.GearID = *body.GearID
	}

	me := auth.MemberID(r.Context())
	activity, err := h.service.UpdateActivity(r.Context(), httpx.Param(r, "id"), me, patch)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cards, err := h.cards(r, me, []domain.Activity{activity})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, cards[0])
}

func (h *Handler) deleteActivity(w http.ResponseWriter, r *http.Request) {
	err := h.service.DeleteActivity(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

func (h *Handler) toggleKudos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	activityID := httpx.Param(r, "id")

	kudoed, err := h.service.ToggleKudos(ctx, activityID, auth.MemberID(ctx))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, _, err := h.service.repo.KudosCounts(ctx, []string{activityID}, auth.MemberID(ctx))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]any{"kudoed": kudoed, "count": counts[activityID]})
}

type commentRequest struct {
	Text string `json:"text"`
}

func (c *commentRequest) Validate() error {
	if strings.TrimSpace(c.Text) == "" {
		return httpx.Invalid("Write something first.")
	}
	return nil
}

func (h *Handler) comment(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[commentRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	me := auth.MemberID(ctx)

	comment, err := h.service.AddComment(ctx, httpx.Param(r, "id"), me, body.Text)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	athletes, err := h.service.members.Athletes(ctx, []string{me})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, commentView{
		ID: comment.ID, MemberID: me, MemberName: athletes[me].Name,
		Text: comment.Text, CreatedAt: comment.CreatedAt,
	})
}

// ── Routes and heatmap ───────────────────────────────────────────────────────

type routeView struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	DistanceM float64             `json:"distanceM"`
	Points    []domain.TrackPoint `json:"points"`
	CreatedAt time.Time           `json:"createdAt"`
}

func (h *Handler) routes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.service.Routes(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]routeView, 0, len(routes))
	for _, route := range routes {
		out = append(out, routeView{
			ID: route.ID, Name: route.Name, DistanceM: route.DistanceM,
			// The list draws every route on one map, so each is thinned.
			Points: domain.Downsample(route.Points, 200), CreatedAt: route.CreatedAt,
		})
	}
	httpx.OK(w, out)
}

func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	route, err := h.service.Route(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, route)
}

type saveRouteRequest struct {
	ActivityID string `json:"activityId"`
	Name       string `json:"name"`
}

func (s *saveRouteRequest) Validate() error {
	if s.ActivityID == "" {
		return httpx.Invalid("Which activity is the route from?")
	}
	return nil
}

func (h *Handler) saveRoute(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[saveRouteRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	route, err := h.service.SaveRouteFromActivity(r.Context(),
		auth.MemberID(r.Context()), body.ActivityID, body.Name)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, route)
}

func (h *Handler) deleteRoute(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteRoute(r.Context(), httpx.Param(r, "id"), auth.MemberID(r.Context())); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

func (h *Handler) heatmap(w http.ResponseWriter, r *http.Request) {
	tracks, err := h.service.Heatmap(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]any{"tracks": tracks})
}

// ── Statistics and gear ──────────────────────────────────────────────────────

// statsWeeks is how far the training chart looks back. Two months is long
// enough to show a build and short enough to fit on a phone.
const statsWeeks = 8

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := auth.MemberID(ctx)

	activities, err := h.service.MyActivities(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	summaries := make([]domain.ActivitySummary, 0, len(activities))
	totalDistance, totalMoving := 0.0, 0
	for _, a := range activities {
		summaries = append(summaries, domain.ActivitySummary{
			StartedAt: a.StartedAt, DistanceM: a.DistanceM, MovingSec: a.MovingSec,
		})
		totalDistance += a.DistanceM
		totalMoving += a.MovingSec
	}

	weekly := domain.WeeklyBuckets(summaries, h.service.clock.Now(), statsWeeks)
	thisWeek := 0.0
	if len(weekly) > 0 {
		thisWeek = weekly[len(weekly)-1].DistanceKm
	}

	settings, err := h.service.Settings(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	gear, err := h.service.Gear(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	following, followers, err := h.service.repo.CountFollows(ctx, me)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	httpx.OK(w, map[string]any{
		"weekly": weekly,
		"totals": map[string]any{
			"activities": len(activities),
			"distanceKm": roundKm(totalDistance),
			"movingSec":  totalMoving,
		},
		"thisWeekKm":     thisWeek,
		"goal":           map[string]any{"targetKm": settings.WeeklyGoalKm, "currentKm": thisWeek},
		"prs":            domain.ComputePersonalRecords(activities),
		"gear":           gear,
		"settings":       settings,
		"followingCount": following,
		"followerCount":  followers,
	})
}

type gearRequest struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Retired bool   `json:"retired"`
}

func (g *gearRequest) Validate() error {
	if strings.TrimSpace(g.Name) == "" {
		return httpx.Invalid("Gear needs a name.")
	}
	if !domain.IsValidGearKind(g.Kind) {
		return httpx.Invalid("Gear is either SHOES or a BIKE.")
	}
	return nil
}

func (h *Handler) createGear(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[gearRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	gear, err := h.service.UpsertGear(r.Context(), auth.MemberID(r.Context()), nil,
		GearInput{Name: body.Name, Kind: domain.GearKind(body.Kind), Retired: body.Retired})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, gear)
}

func (h *Handler) updateGear(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[gearRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	gearID := httpx.Param(r, "id")
	gear, err := h.service.UpsertGear(r.Context(), auth.MemberID(r.Context()), &gearID,
		GearInput{Name: body.Name, Kind: domain.GearKind(body.Kind), Retired: body.Retired})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, gear)
}

// ── Settings ─────────────────────────────────────────────────────────────────

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.Settings(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, settings)
}

type settingsRequest struct {
	Units            *string   `json:"units"`
	BookingReminders *bool     `json:"bookingReminders"`
	WeeklyGoalKm     **float64 `json:"weeklyGoalKm"`
	Language         *string   `json:"language"`
}

func (s *settingsRequest) Validate() error {
	if s.Units != nil && !domain.IsValidUnits(*s.Units) {
		return httpx.Invalid("Units must be METRIC or IMPERIAL.")
	}
	if s.Language != nil && !domain.IsValidLanguage(*s.Language) {
		return httpx.Invalid("Language must be EN or ID.")
	}
	if s.WeeklyGoalKm != nil && *s.WeeklyGoalKm != nil {
		if goal := **s.WeeklyGoalKm; goal <= 0 || goal > 1000 {
			return httpx.Invalid("A weekly goal must be between 1 and 1000 km.")
		}
	}
	return nil
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[settingsRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	patch := SettingsPatch{
		Units: body.Units, BookingReminders: body.BookingReminders, Language: body.Language,
	}
	if body.WeeklyGoalKm != nil {
		patch.SetWeeklyGoal = true
		patch.WeeklyGoalKm = *body.WeeklyGoalKm
	}

	settings, err := h.service.UpdateSettings(r.Context(), auth.MemberID(r.Context()), patch)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, settings)
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func roundKm(metres float64) float64 {
	return float64(int(metres/100+0.5)) / 10
}
