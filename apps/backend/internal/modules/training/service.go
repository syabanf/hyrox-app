package training

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Athlete is one member as the athlete screens need them: a name and a face.
// Training never reads the identity schema; it asks for this instead.
type Athlete struct {
	ID        string
	Name      string
	AvatarURL *string
	Active    bool
}

// Members is what training needs from whoever owns member records.
type Members interface {
	Athletes(ctx context.Context, ids []string) (map[string]Athlete, error)
	ActiveAthletes(ctx context.Context) ([]Athlete, error)
}

// Library is the exercise catalogue the workout generator draws on.
type Library interface {
	Exercises(ctx context.Context) ([]domain.Exercise, error)
	Substitutions(ctx context.Context) ([]domain.SubstitutionRule, error)
}

// Challenges is what the athlete tab needs from whoever runs them. The
// challenges themselves belong to engagement — the studio sets them up, the
// athlete screen only shows a member their own progress.
type Challenges interface {
	Running(ctx context.Context, now time.Time) ([]domain.Challenge, error)
	Participants(ctx context.Context) (map[string][]string, error)
	Join(ctx context.Context, challengeID, memberID string) error
}

// Service implements the training use cases.
type Service struct {
	repo       *Repository
	members    Members
	library    Library
	challenges Challenges
	ids        id.Generator
	clock      clock.Clock
}

func NewService(repo *Repository, members Members, library Library, challenges Challenges, ids id.Generator, c clock.Clock) *Service {
	return &Service{repo: repo, members: members, library: library, challenges: challenges, ids: ids, clock: c}
}

// feedLimit caps how far back the feed reaches. A social feed nobody scrolls
// to the bottom of does not need to be able to.
const feedLimit = 200

// maxPhotos and maxPhotoBytes keep an activity from becoming a photo album.
// The app resizes before upload; this is the backstop.
const (
	maxPhotos     = 8
	maxPhotoBytes = 400_000
)

// ── Activities ───────────────────────────────────────────────────────────────

// SaveActivityInput is a finished effort arriving from the app.
type SaveActivityInput struct {
	Type        domain.ActivityType
	Title       string
	Description string
	StartedAt   time.Time
	Points      []domain.TrackPoint
	Photos      []string
	Visibility  domain.ActivityVisibility
	GearID      *string
	// ElapsedSec and MovingSec are only used when there is no track to measure
	// — a gym session somebody logged by hand.
	ElapsedSec int
	MovingSec  int
	DistanceM  float64
}

// SaveActivity records an effort and everything that falls out of it: the
// stats, the segments it covered, and the mileage on the shoes.
//
// The numbers are computed here rather than trusted from the client. A phone
// that has been running in a pocket for an hour will happily report a
// four-minute-mile average; the track is the evidence, and the track is what
// gets measured.
func (s *Service) SaveActivity(ctx context.Context, memberID string, in SaveActivityInput) (domain.Activity, error) {
	if !domain.IsValidActivityType(string(in.Type)) {
		return domain.Activity{}, httpx.Invalid("That is not an activity type.")
	}
	if in.Visibility == "" {
		in.Visibility = domain.VisibilityEveryone
	}
	if !domain.IsValidVisibility(string(in.Visibility)) {
		return domain.Activity{}, httpx.Invalid("That is not a visibility setting.")
	}
	if len(in.Photos) > maxPhotos {
		return domain.Activity{}, httpx.Invalid("An activity carries at most %d photos.", maxPhotos)
	}
	for _, photo := range in.Photos {
		if len(photo) > maxPhotoBytes {
			return domain.Activity{}, httpx.Invalid("One of those photos is too large.")
		}
	}
	if in.StartedAt.IsZero() {
		in.StartedAt = s.clock.Now()
	}

	activity := domain.Activity{
		ID:          s.ids.New(id.Activity),
		MemberID:    memberID,
		Type:        in.Type,
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		StartedAt:   in.StartedAt,
		Points:      in.Points,
		Photos:      in.Photos,
		Visibility:  in.Visibility,
		GearID:      in.GearID,
	}
	if activity.Title == "" {
		activity.Title = defaultTitle(in.Type, in.StartedAt)
	}
	if activity.Points == nil {
		activity.Points = []domain.TrackPoint{}
	}
	if activity.Photos == nil {
		activity.Photos = []string{}
	}

	if len(activity.Points) >= 2 {
		stats := domain.ComputeActivityStats(activity.Points)
		activity.ElapsedSec = stats.ElapsedSec
		activity.MovingSec = stats.MovingSec
		activity.DistanceM = stats.DistanceM
		activity.AvgPaceSecPerKm = stats.AvgPaceSecPerKm
		activity.ElevationGainM = stats.ElevationGainM
	} else {
		// Nothing to measure, so the client's own figures stand. A gym session
		// has no track and still took forty minutes.
		activity.ElapsedSec = maxInt(in.ElapsedSec, 0)
		activity.MovingSec = maxInt(in.MovingSec, 0)
		activity.DistanceM = math.Max(in.DistanceM, 0)
	}

	// The gear has to belong to the person claiming it.
	if activity.GearID != nil {
		gear, err := s.repo.GearItem(ctx, *activity.GearID)
		if err != nil {
			return domain.Activity{}, err
		}
		if gear.MemberID != memberID {
			return domain.Activity{}, httpx.NotFound("gear")
		}
	}

	created, err := s.repo.InsertActivity(ctx, activity)
	if err != nil {
		return domain.Activity{}, err
	}

	if err := s.matchSegments(ctx, created); err != nil {
		return domain.Activity{}, err
	}
	if created.GearID != nil && created.DistanceM > 0 {
		if err := s.repo.AddGearDistance(ctx, *created.GearID, created.DistanceM); err != nil {
			return domain.Activity{}, err
		}
	}
	return created, nil
}

// matchSegments records an effort for every segment the track covered.
func (s *Service) matchSegments(ctx context.Context, activity domain.Activity) error {
	if len(activity.Points) < 2 {
		return nil
	}
	segments, err := s.repo.Segments(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	for _, match := range domain.MatchSegments(segments, activity.Type, activity.Points) {
		if err := s.repo.InsertEffort(ctx, domain.SegmentEffort{
			ID:         s.ids.New(id.Effort),
			SegmentID:  match.Segment.ID,
			ActivityID: activity.ID,
			MemberID:   activity.MemberID,
			ElapsedSec: match.ElapsedSec,
			CreatedAt:  now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func defaultTitle(t domain.ActivityType, at time.Time) string {
	part := "Evening"
	switch hour := at.Hour(); {
	case hour < 12:
		part = "Morning"
	case hour < 17:
		part = "Afternoon"
	}
	word := map[domain.ActivityType]string{
		domain.ActivityRun: "run", domain.ActivityRide: "ride",
		domain.ActivityWalk: "walk", domain.ActivityWorkout: "workout",
	}[t]
	return part + " " + word
}

// ActivityPatch is what a member may change after the fact.
type ActivityPatch struct {
	Title       *string
	Description *string
	Visibility  *domain.ActivityVisibility
	GearID      *string
	SetGear     bool
}

func (s *Service) UpdateActivity(ctx context.Context, activityID, memberID string, patch ActivityPatch) (domain.Activity, error) {
	activity, err := s.ownActivity(ctx, activityID, memberID)
	if err != nil {
		return domain.Activity{}, err
	}
	previousGear := activity.GearID

	if patch.Title != nil {
		activity.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Description != nil {
		activity.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Visibility != nil {
		if !domain.IsValidVisibility(string(*patch.Visibility)) {
			return domain.Activity{}, httpx.Invalid("That is not a visibility setting.")
		}
		activity.Visibility = *patch.Visibility
	}
	if patch.SetGear {
		if patch.GearID != nil {
			gear, err := s.repo.GearItem(ctx, *patch.GearID)
			if err != nil {
				return domain.Activity{}, err
			}
			if gear.MemberID != memberID {
				return domain.Activity{}, httpx.NotFound("gear")
			}
		}
		activity.GearID = patch.GearID
	}

	updated, err := s.repo.UpdateActivity(ctx, activity)
	if err != nil {
		return domain.Activity{}, err
	}

	// Moving an activity between pairs of shoes moves the mileage with it,
	// or the numbers on the gear screen quietly stop meaning anything.
	if patch.SetGear && !samePtr(previousGear, updated.GearID) && updated.DistanceM > 0 {
		if previousGear != nil {
			if err := s.repo.AddGearDistance(ctx, *previousGear, -updated.DistanceM); err != nil {
				return domain.Activity{}, err
			}
		}
		if updated.GearID != nil {
			if err := s.repo.AddGearDistance(ctx, *updated.GearID, updated.DistanceM); err != nil {
				return domain.Activity{}, err
			}
		}
	}
	return updated, nil
}

func (s *Service) DeleteActivity(ctx context.Context, activityID, memberID string) error {
	activity, err := s.ownActivity(ctx, activityID, memberID)
	if err != nil {
		return err
	}
	if activity.GearID != nil && activity.DistanceM > 0 {
		if err := s.repo.AddGearDistance(ctx, *activity.GearID, -activity.DistanceM); err != nil {
			return err
		}
	}
	return s.repo.DeleteActivity(ctx, activityID)
}

// ownActivity reads an activity that belongs to the caller. Somebody else's
// activity is reported as missing rather than forbidden: whether it exists is
// itself none of their business.
func (s *Service) ownActivity(ctx context.Context, activityID, memberID string) (domain.Activity, error) {
	activity, err := s.repo.Activity(ctx, activityID)
	if err != nil {
		return domain.Activity{}, err
	}
	if activity.MemberID != memberID {
		return domain.Activity{}, httpx.NotFound("activity")
	}
	return activity, nil
}

// Activity reads one activity, if the viewer is allowed to see it.
func (s *Service) Activity(ctx context.Context, activityID, viewerID string) (domain.Activity, error) {
	activity, err := s.repo.Activity(ctx, activityID)
	if err != nil {
		return domain.Activity{}, err
	}
	following, err := s.repo.Following(ctx, viewerID)
	if err != nil {
		return domain.Activity{}, err
	}
	if !domain.CanViewActivity(activity.MemberID, activity.Visibility, viewerID, func(id string) bool {
		return following[id]
	}) {
		return domain.Activity{}, httpx.ErrForbidden.WithMessage("That activity is private.")
	}
	return activity, nil
}

// Feed is the activities a viewer may see, newest first.
//
// followingOnly narrows it to the people they follow, which is the difference
// between "what is everyone up to" and "how are my friends doing".
func (s *Service) Feed(ctx context.Context, viewerID string, followingOnly bool, limit int) ([]domain.Activity, error) {
	following, err := s.repo.Following(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	candidates, err := s.repo.FeedCandidates(ctx, feedLimit)
	if err != nil {
		return nil, err
	}

	out := []domain.Activity{}
	for _, a := range candidates {
		if !domain.CanViewActivity(a.MemberID, a.Visibility, viewerID, func(id string) bool { return following[id] }) {
			continue
		}
		if followingOnly && a.MemberID != viewerID && !following[a.MemberID] {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Service) MyActivities(ctx context.Context, memberID string) ([]domain.Activity, error) {
	return s.repo.ActivitiesForMember(ctx, memberID)
}

// ── Kudos and comments ───────────────────────────────────────────────────────

// ToggleKudos gives or takes back a kudos on an activity the caller can see.
func (s *Service) ToggleKudos(ctx context.Context, activityID, memberID string) (bool, error) {
	if _, err := s.Activity(ctx, activityID, memberID); err != nil {
		return false, err
	}
	return s.repo.ToggleKudos(ctx, activityID, memberID)
}

const maxCommentLength = 500

func (s *Service) AddComment(ctx context.Context, activityID, memberID, text string) (domain.ActivityComment, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.ActivityComment{}, httpx.Invalid("Write something first.")
	}
	if len([]rune(text)) > maxCommentLength {
		return domain.ActivityComment{}, httpx.Invalid("A comment is at most %d characters.", maxCommentLength)
	}
	if _, err := s.Activity(ctx, activityID, memberID); err != nil {
		return domain.ActivityComment{}, err
	}
	return s.repo.InsertComment(ctx, domain.ActivityComment{
		ID: s.ids.New(id.Comment), ActivityID: activityID, MemberID: memberID, Text: text,
	})
}

// ── Social graph ─────────────────────────────────────────────────────────────

func (s *Service) ToggleFollow(ctx context.Context, followerID, followeeID string) (bool, error) {
	if followerID == followeeID {
		return false, httpx.Invalid("You cannot follow yourself.")
	}
	athletes, err := s.members.Athletes(ctx, []string{followeeID})
	if err != nil {
		return false, err
	}
	if _, ok := athletes[followeeID]; !ok {
		return false, httpx.NotFound("member")
	}
	return s.repo.ToggleFollow(ctx, followerID, followeeID)
}

// ── Gear ─────────────────────────────────────────────────────────────────────

// GearInput creates or edits a pair of shoes.
type GearInput struct {
	Name    string
	Kind    domain.GearKind
	Retired bool
}

func (s *Service) UpsertGear(ctx context.Context, memberID string, gearID *string, in GearInput) (domain.Gear, error) {
	if strings.TrimSpace(in.Name) == "" {
		return domain.Gear{}, httpx.Invalid("Gear needs a name.")
	}
	if !domain.IsValidGearKind(string(in.Kind)) {
		return domain.Gear{}, httpx.Invalid("Gear is either SHOES or a BIKE.")
	}

	gear := domain.Gear{
		ID: s.ids.New(id.Gear), MemberID: memberID,
		Name: strings.TrimSpace(in.Name), Kind: in.Kind, Retired: in.Retired,
	}
	if gearID != nil {
		existing, err := s.repo.GearItem(ctx, *gearID)
		if err != nil {
			return domain.Gear{}, err
		}
		if existing.MemberID != memberID {
			return domain.Gear{}, httpx.NotFound("gear")
		}
		// The distance is not editable: it is the sum of what was run in them.
		gear.ID, gear.DistanceM = existing.ID, existing.DistanceM
	}
	return s.repo.UpsertGear(ctx, gear)
}

func (s *Service) Gear(ctx context.Context, memberID string) ([]domain.Gear, error) {
	return s.repo.Gear(ctx, memberID)
}

// ── Routes ───────────────────────────────────────────────────────────────────

// SaveRouteFromActivity turns a track somebody liked into a route they can run
// again.
func (s *Service) SaveRouteFromActivity(ctx context.Context, memberID, activityID, name string) (domain.Route, error) {
	activity, err := s.ownActivity(ctx, activityID, memberID)
	if err != nil {
		return domain.Route{}, err
	}
	if len(activity.Points) < 2 {
		return domain.Route{}, httpx.Invalid("That activity has no track to save.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = activity.Title
	}
	return s.repo.InsertRoute(ctx, domain.Route{
		ID: s.ids.New(id.Route), MemberID: memberID, Name: name,
		// Routes are drawn on a map, not raced against, so a couple of hundred
		// points is plenty and keeps the list cheap to load.
		Points:    domain.Downsample(activity.Points, 500),
		DistanceM: activity.DistanceM,
	})
}

func (s *Service) Routes(ctx context.Context, memberID string) ([]domain.Route, error) {
	return s.repo.Routes(ctx, memberID)
}

func (s *Service) Route(ctx context.Context, routeID, memberID string) (domain.Route, error) {
	route, err := s.repo.Route(ctx, routeID)
	if err != nil {
		return domain.Route{}, err
	}
	if route.MemberID != memberID {
		return domain.Route{}, httpx.NotFound("route")
	}
	return route, nil
}

func (s *Service) DeleteRoute(ctx context.Context, routeID, memberID string) error {
	if _, err := s.Route(ctx, routeID, memberID); err != nil {
		return err
	}
	return s.repo.DeleteRoute(ctx, routeID)
}

// Heatmap is every track the member has recorded, thinned for drawing.
func (s *Service) Heatmap(ctx context.Context, memberID string) ([][]domain.TrackPoint, error) {
	activities, err := s.repo.ActivitiesForMember(ctx, memberID)
	if err != nil {
		return nil, err
	}
	tracks := [][]domain.TrackPoint{}
	for _, a := range activities {
		if len(a.Points) > 1 {
			tracks = append(tracks, domain.Downsample(a.Points, 120))
		}
	}
	return tracks, nil
}

// ── Settings ─────────────────────────────────────────────────────────────────

func (s *Service) Settings(ctx context.Context, memberID string) (domain.AthleteSettings, error) {
	return s.repo.Settings(ctx, memberID)
}

// SettingsPatch is a partial settings update. The goal is nullable, so
// clearing it is different from leaving it alone.
type SettingsPatch struct {
	Units            *string
	BookingReminders *bool
	WeeklyGoalKm     *float64
	SetWeeklyGoal    bool
	Language         *string
}

func (s *Service) UpdateSettings(ctx context.Context, memberID string, patch SettingsPatch) (domain.AthleteSettings, error) {
	current, err := s.repo.Settings(ctx, memberID)
	if err != nil {
		return domain.AthleteSettings{}, err
	}
	if patch.Units != nil {
		current.Units = *patch.Units
	}
	if patch.BookingReminders != nil {
		current.BookingReminders = *patch.BookingReminders
	}
	if patch.SetWeeklyGoal {
		current.WeeklyGoalKm = patch.WeeklyGoalKm
	}
	if patch.Language != nil {
		current.Language = *patch.Language
	}
	return s.repo.SaveSettings(ctx, memberID, current)
}

func samePtr(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
