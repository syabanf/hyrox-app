package training

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Repository persists the athlete side of the member app. It reads and writes
// the `training` schema only; member names and avatars are fetched from
// identity through a port, never joined across schemas.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── Activities ───────────────────────────────────────────────────────────────

const activityColumns = `id, member_id, type, title, description, started_at, elapsed_sec,
	moving_sec, distance_m, avg_pace_sec_per_km, elevation_gain_m, points, photos,
	visibility, gear_id, created_at`

func scanActivity(scan func(...any) error) (domain.Activity, error) {
	var a domain.Activity
	var points, photos []byte
	if err := scan(&a.ID, &a.MemberID, &a.Type, &a.Title, &a.Description, &a.StartedAt,
		&a.ElapsedSec, &a.MovingSec, &a.DistanceM, &a.AvgPaceSecPerKm, &a.ElevationGainM,
		&points, &photos, &a.Visibility, &a.GearID, &a.CreatedAt); err != nil {
		return domain.Activity{}, err
	}
	_ = json.Unmarshal(points, &a.Points)
	_ = json.Unmarshal(photos, &a.Photos)
	if a.Points == nil {
		a.Points = []domain.TrackPoint{}
	}
	if a.Photos == nil {
		a.Photos = []string{}
	}
	return a, nil
}

func (r *Repository) collectActivities(ctx context.Context, sql string, args ...any) ([]domain.Activity, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("training: listing activities: %w", err)
	}
	defer rows.Close()

	out := []domain.Activity{}
	for rows.Next() {
		a, err := scanActivity(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("training: scanning activity: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActivitiesForMember is one athlete's own log, newest first.
func (r *Repository) ActivitiesForMember(ctx context.Context, memberID string) ([]domain.Activity, error) {
	return r.collectActivities(ctx,
		`SELECT `+activityColumns+` FROM training.activities
		 WHERE member_id = $1 ORDER BY started_at DESC`, memberID)
}

// FeedCandidates is everything a viewer could possibly see, newest first.
//
// The visibility rule is applied in Go rather than SQL: it depends on the
// viewer's follow set, and "who do I follow" is a fact the service already has
// in hand for the rest of the feed.
func (r *Repository) FeedCandidates(ctx context.Context, limit int) ([]domain.Activity, error) {
	return r.collectActivities(ctx,
		`SELECT `+activityColumns+` FROM training.activities
		 ORDER BY started_at DESC LIMIT $1`, limit)
}

func (r *Repository) Activity(ctx context.Context, id string) (domain.Activity, error) {
	a, err := scanActivity(r.db.QueryRow(ctx,
		`SELECT `+activityColumns+` FROM training.activities WHERE id = $1`, id).Scan)
	if database.IsNoRows(err) {
		return domain.Activity{}, httpx.NotFound("activity")
	}
	if err != nil {
		return domain.Activity{}, fmt.Errorf("training: reading activity: %w", err)
	}
	return a, nil
}

func (r *Repository) InsertActivity(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	points, err := json.Marshal(a.Points)
	if err != nil {
		return domain.Activity{}, fmt.Errorf("training: encoding track: %w", err)
	}
	photos, err := json.Marshal(a.Photos)
	if err != nil {
		return domain.Activity{}, fmt.Errorf("training: encoding photos: %w", err)
	}
	created, err := scanActivity(r.db.QueryRow(ctx, `
		INSERT INTO training.activities
			(id, member_id, type, title, description, started_at, elapsed_sec, moving_sec,
			 distance_m, avg_pace_sec_per_km, elevation_gain_m, points, photos, visibility, gear_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+activityColumns,
		a.ID, a.MemberID, a.Type, a.Title, a.Description, a.StartedAt, a.ElapsedSec, a.MovingSec,
		a.DistanceM, a.AvgPaceSecPerKm, a.ElevationGainM, points, photos, a.Visibility, a.GearID).Scan)
	if err != nil {
		return domain.Activity{}, fmt.Errorf("training: inserting activity: %w", err)
	}
	return created, nil
}

// UpdateActivity saves the fields a member may edit after the fact. The track
// itself is not among them: a recorded effort is a record.
func (r *Repository) UpdateActivity(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	updated, err := scanActivity(r.db.QueryRow(ctx, `
		UPDATE training.activities
		SET title = $2, description = $3, visibility = $4, gear_id = $5, updated_at = now()
		WHERE id = $1 RETURNING `+activityColumns,
		a.ID, a.Title, a.Description, a.Visibility, a.GearID).Scan)
	if database.IsNoRows(err) {
		return domain.Activity{}, httpx.NotFound("activity")
	}
	if err != nil {
		return domain.Activity{}, fmt.Errorf("training: updating activity: %w", err)
	}
	return updated, nil
}

// DeleteActivity removes an activity. Kudos, comments and efforts go with it
// by foreign key, which is what ON DELETE CASCADE is for.
func (r *Repository) DeleteActivity(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM training.activities WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("training: deleting activity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("activity")
	}
	return nil
}

// ── Kudos and comments ───────────────────────────────────────────────────────

// KudosCounts is how many kudos each of these activities has, and which of
// them the viewer has already given.
func (r *Repository) KudosCounts(ctx context.Context, activityIDs []string, viewerID string) (map[string]int, map[string]bool, error) {
	counts, mine := map[string]int{}, map[string]bool{}
	if len(activityIDs) == 0 {
		return counts, mine, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT activity_id, count(*), bool_or(member_id = $2)
		FROM training.kudos WHERE activity_id = ANY($1) GROUP BY activity_id`,
		activityIDs, viewerID)
	if err != nil {
		return nil, nil, fmt.Errorf("training: counting kudos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		var hasMine bool
		if err := rows.Scan(&id, &count, &hasMine); err != nil {
			return nil, nil, fmt.Errorf("training: scanning kudos: %w", err)
		}
		counts[id] = count
		mine[id] = hasMine
	}
	return counts, mine, rows.Err()
}

// ToggleKudos gives or takes back a kudos, returning whether it now stands.
func (r *Repository) ToggleKudos(ctx context.Context, activityID, memberID string) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM training.kudos WHERE activity_id = $1 AND member_id = $2`, activityID, memberID)
	if err != nil {
		return false, fmt.Errorf("training: removing kudos: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}
	if _, err := r.db.Exec(ctx,
		`INSERT INTO training.kudos (activity_id, member_id) VALUES ($1, $2)`,
		activityID, memberID); err != nil {
		return false, fmt.Errorf("training: adding kudos: %w", err)
	}
	return true, nil
}

func (r *Repository) CommentCounts(ctx context.Context, activityIDs []string) (map[string]int, error) {
	counts := map[string]int{}
	if len(activityIDs) == 0 {
		return counts, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT activity_id, count(*) FROM training.activity_comments
		 WHERE activity_id = ANY($1) GROUP BY activity_id`, activityIDs)
	if err != nil {
		return nil, fmt.Errorf("training: counting comments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("training: scanning comment count: %w", err)
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (r *Repository) Comments(ctx context.Context, activityID string) ([]domain.ActivityComment, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, activity_id, member_id, text, created_at FROM training.activity_comments
		 WHERE activity_id = $1 ORDER BY created_at`, activityID)
	if err != nil {
		return nil, fmt.Errorf("training: listing comments: %w", err)
	}
	defer rows.Close()

	out := []domain.ActivityComment{}
	for rows.Next() {
		var c domain.ActivityComment
		if err := rows.Scan(&c.ID, &c.ActivityID, &c.MemberID, &c.Text, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("training: scanning comment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) InsertComment(ctx context.Context, c domain.ActivityComment) (domain.ActivityComment, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO training.activity_comments (id, activity_id, member_id, text)
		VALUES ($1, $2, $3, $4) RETURNING created_at`,
		c.ID, c.ActivityID, c.MemberID, c.Text).Scan(&c.CreatedAt)
	if database.IsForeignKeyViolation(err) {
		return domain.ActivityComment{}, httpx.NotFound("activity")
	}
	if err != nil {
		return domain.ActivityComment{}, fmt.Errorf("training: inserting comment: %w", err)
	}
	return c, nil
}

// ── Follows ──────────────────────────────────────────────────────────────────

// Following is the set of people a member follows, as a set because every
// visibility check asks the same question.
func (r *Repository) Following(ctx context.Context, memberID string) (map[string]bool, error) {
	rows, err := r.db.Query(ctx,
		`SELECT followee_id FROM training.follows WHERE follower_id = $1`, memberID)
	if err != nil {
		return nil, fmt.Errorf("training: listing follows: %w", err)
	}
	defer rows.Close()

	set := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("training: scanning follow: %w", err)
		}
		set[id] = true
	}
	return set, rows.Err()
}

func (r *Repository) Followers(ctx context.Context, memberID string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT follower_id FROM training.follows WHERE followee_id = $1 ORDER BY created_at DESC`,
		memberID)
	if err != nil {
		return nil, fmt.Errorf("training: listing followers: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("training: scanning follower: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) CountFollows(ctx context.Context, memberID string) (following, followers int, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM training.follows WHERE follower_id = $1),
		       (SELECT count(*) FROM training.follows WHERE followee_id = $1)`, memberID).
		Scan(&following, &followers)
	if err != nil {
		return 0, 0, fmt.Errorf("training: counting follows: %w", err)
	}
	return following, followers, nil
}

// ToggleFollow follows or unfollows, returning whether the follow now stands.
func (r *Repository) ToggleFollow(ctx context.Context, followerID, followeeID string) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM training.follows WHERE follower_id = $1 AND followee_id = $2`,
		followerID, followeeID)
	if err != nil {
		return false, fmt.Errorf("training: unfollowing: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}
	if _, err := r.db.Exec(ctx,
		`INSERT INTO training.follows (follower_id, followee_id) VALUES ($1, $2)`,
		followerID, followeeID); err != nil {
		if database.IsCheckViolation(err) {
			return false, httpx.Invalid("You cannot follow yourself.")
		}
		return false, fmt.Errorf("training: following: %w", err)
	}
	return true, nil
}

// ── Segments ─────────────────────────────────────────────────────────────────

func (r *Repository) Segments(ctx context.Context) ([]domain.Segment, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, name, type, distance_m, location, path FROM training.segments ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("training: listing segments: %w", err)
	}
	defer rows.Close()

	out := []domain.Segment{}
	for rows.Next() {
		var s domain.Segment
		var path []byte
		if err := rows.Scan(&s.ID, &s.Name, &s.Type, &s.DistanceM, &s.Location, &path); err != nil {
			return nil, fmt.Errorf("training: scanning segment: %w", err)
		}
		_ = json.Unmarshal(path, &s.Path)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Segment(ctx context.Context, id string) (domain.Segment, error) {
	var s domain.Segment
	var path []byte
	err := r.db.QueryRow(ctx,
		`SELECT id, name, type, distance_m, location, path FROM training.segments WHERE id = $1`, id).
		Scan(&s.ID, &s.Name, &s.Type, &s.DistanceM, &s.Location, &path)
	if database.IsNoRows(err) {
		return domain.Segment{}, httpx.NotFound("segment")
	}
	if err != nil {
		return domain.Segment{}, fmt.Errorf("training: reading segment: %w", err)
	}
	_ = json.Unmarshal(path, &s.Path)
	return s, nil
}

// Efforts is every attempt at a segment, fastest first.
func (r *Repository) Efforts(ctx context.Context, segmentID string) ([]domain.SegmentEffort, error) {
	return r.collectEfforts(ctx,
		`SELECT id, segment_id, activity_id, member_id, elapsed_sec, created_at
		 FROM training.segment_efforts WHERE segment_id = $1 ORDER BY elapsed_sec`, segmentID)
}

func (r *Repository) EffortsForActivity(ctx context.Context, activityID string) ([]domain.SegmentEffort, error) {
	return r.collectEfforts(ctx,
		`SELECT id, segment_id, activity_id, member_id, elapsed_sec, created_at
		 FROM training.segment_efforts WHERE activity_id = $1`, activityID)
}

// AllEfforts is every effort on every segment, which is what the segment list
// needs to compute a leaderboard position for each row at once.
func (r *Repository) AllEfforts(ctx context.Context) ([]domain.SegmentEffort, error) {
	return r.collectEfforts(ctx,
		`SELECT id, segment_id, activity_id, member_id, elapsed_sec, created_at
		 FROM training.segment_efforts ORDER BY segment_id, elapsed_sec`)
}

func (r *Repository) collectEfforts(ctx context.Context, sql string, args ...any) ([]domain.SegmentEffort, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("training: listing efforts: %w", err)
	}
	defer rows.Close()

	out := []domain.SegmentEffort{}
	for rows.Next() {
		var e domain.SegmentEffort
		if err := rows.Scan(&e.ID, &e.SegmentID, &e.ActivityID, &e.MemberID, &e.ElapsedSec, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("training: scanning effort: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// InsertEffort records one attempt. A repeat for the same activity and segment
// is ignored, because re-saving an activity must not double its efforts.
func (r *Repository) InsertEffort(ctx context.Context, e domain.SegmentEffort) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO training.segment_efforts (id, segment_id, activity_id, member_id, elapsed_sec)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (activity_id, segment_id) DO NOTHING`,
		e.ID, e.SegmentID, e.ActivityID, e.MemberID, e.ElapsedSec)
	if err != nil {
		return fmt.Errorf("training: inserting effort: %w", err)
	}
	return nil
}

// ── Clubs ────────────────────────────────────────────────────────────────────

func (r *Repository) Clubs(ctx context.Context) ([]domain.Club, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.name, c.description, c.location, c.created_at,
		       coalesce(array_agg(m.member_id) FILTER (WHERE m.member_id IS NOT NULL), '{}')
		FROM training.clubs c
		LEFT JOIN training.club_members m ON m.club_id = c.id
		GROUP BY c.id ORDER BY c.name`)
	if err != nil {
		return nil, fmt.Errorf("training: listing clubs: %w", err)
	}
	defer rows.Close()

	out := []domain.Club{}
	for rows.Next() {
		var c domain.Club
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Location, &c.CreatedAt, &c.MemberIDs); err != nil {
			return nil, fmt.Errorf("training: scanning club: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ToggleClubMembership joins or leaves, returning whether the member is in.
func (r *Repository) ToggleClubMembership(ctx context.Context, clubID, memberID string) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM training.club_members WHERE club_id = $1 AND member_id = $2`, clubID, memberID)
	if err != nil {
		return false, fmt.Errorf("training: leaving club: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}
	if _, err := r.db.Exec(ctx,
		`INSERT INTO training.club_members (club_id, member_id) VALUES ($1, $2)`,
		clubID, memberID); err != nil {
		if database.IsForeignKeyViolation(err) {
			return false, httpx.NotFound("club")
		}
		return false, fmt.Errorf("training: joining club: %w", err)
	}
	return true, nil
}

// ── Gear ─────────────────────────────────────────────────────────────────────

func (r *Repository) Gear(ctx context.Context, memberID string) ([]domain.Gear, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, member_id, name, kind, distance_m, retired FROM training.gear
		 WHERE member_id = $1 ORDER BY retired, name`, memberID)
	if err != nil {
		return nil, fmt.Errorf("training: listing gear: %w", err)
	}
	defer rows.Close()

	out := []domain.Gear{}
	for rows.Next() {
		var g domain.Gear
		if err := rows.Scan(&g.ID, &g.MemberID, &g.Name, &g.Kind, &g.DistanceM, &g.Retired); err != nil {
			return nil, fmt.Errorf("training: scanning gear: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *Repository) GearItem(ctx context.Context, id string) (domain.Gear, error) {
	var g domain.Gear
	err := r.db.QueryRow(ctx,
		`SELECT id, member_id, name, kind, distance_m, retired FROM training.gear WHERE id = $1`, id).
		Scan(&g.ID, &g.MemberID, &g.Name, &g.Kind, &g.DistanceM, &g.Retired)
	if database.IsNoRows(err) {
		return domain.Gear{}, httpx.NotFound("gear")
	}
	if err != nil {
		return domain.Gear{}, fmt.Errorf("training: reading gear: %w", err)
	}
	return g, nil
}

func (r *Repository) UpsertGear(ctx context.Context, g domain.Gear) (domain.Gear, error) {
	var saved domain.Gear
	err := r.db.QueryRow(ctx, `
		INSERT INTO training.gear (id, member_id, name, kind, distance_m, retired)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, kind = EXCLUDED.kind,
			retired = EXCLUDED.retired, updated_at = now()
		RETURNING id, member_id, name, kind, distance_m, retired`,
		g.ID, g.MemberID, g.Name, g.Kind, g.DistanceM, g.Retired).
		Scan(&saved.ID, &saved.MemberID, &saved.Name, &saved.Kind, &saved.DistanceM, &saved.Retired)
	if err != nil {
		return domain.Gear{}, fmt.Errorf("training: saving gear: %w", err)
	}
	return saved, nil
}

// AddGearDistance accumulates the mileage on a pair of shoes. Shoes wear out
// by distance, so the number has to follow the activities logged against them.
func (r *Repository) AddGearDistance(ctx context.Context, gearID string, distanceM float64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE training.gear SET distance_m = greatest(0, distance_m + $2), updated_at = now()
		 WHERE id = $1`, gearID, distanceM)
	if err != nil {
		return fmt.Errorf("training: updating gear distance: %w", err)
	}
	return nil
}

// ── Routes ───────────────────────────────────────────────────────────────────

func (r *Repository) Routes(ctx context.Context, memberID string) ([]domain.Route, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, member_id, name, points, distance_m, created_at FROM training.routes
		 WHERE member_id = $1 ORDER BY created_at DESC`, memberID)
	if err != nil {
		return nil, fmt.Errorf("training: listing routes: %w", err)
	}
	defer rows.Close()

	out := []domain.Route{}
	for rows.Next() {
		var route domain.Route
		var points []byte
		if err := rows.Scan(&route.ID, &route.MemberID, &route.Name, &points, &route.DistanceM, &route.CreatedAt); err != nil {
			return nil, fmt.Errorf("training: scanning route: %w", err)
		}
		_ = json.Unmarshal(points, &route.Points)
		out = append(out, route)
	}
	return out, rows.Err()
}

func (r *Repository) Route(ctx context.Context, id string) (domain.Route, error) {
	var route domain.Route
	var points []byte
	err := r.db.QueryRow(ctx,
		`SELECT id, member_id, name, points, distance_m, created_at FROM training.routes WHERE id = $1`, id).
		Scan(&route.ID, &route.MemberID, &route.Name, &points, &route.DistanceM, &route.CreatedAt)
	if database.IsNoRows(err) {
		return domain.Route{}, httpx.NotFound("route")
	}
	if err != nil {
		return domain.Route{}, fmt.Errorf("training: reading route: %w", err)
	}
	_ = json.Unmarshal(points, &route.Points)
	return route, nil
}

func (r *Repository) InsertRoute(ctx context.Context, route domain.Route) (domain.Route, error) {
	points, err := json.Marshal(route.Points)
	if err != nil {
		return domain.Route{}, fmt.Errorf("training: encoding route: %w", err)
	}
	err = r.db.QueryRow(ctx, `
		INSERT INTO training.routes (id, member_id, name, points, distance_m)
		VALUES ($1, $2, $3, $4, $5) RETURNING created_at`,
		route.ID, route.MemberID, route.Name, points, route.DistanceM).Scan(&route.CreatedAt)
	if err != nil {
		return domain.Route{}, fmt.Errorf("training: inserting route: %w", err)
	}
	return route, nil
}

func (r *Repository) DeleteRoute(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM training.routes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("training: deleting route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("route")
	}
	return nil
}

// ── Settings ─────────────────────────────────────────────────────────────────

// Settings reads a member's preferences, returning the defaults for a member
// who has never changed them rather than requiring a row to exist.
func (r *Repository) Settings(ctx context.Context, memberID string) (domain.AthleteSettings, error) {
	s := domain.DefaultAthleteSettings()
	err := r.db.QueryRow(ctx, `
		SELECT units, booking_reminders, weekly_goal_km, language
		FROM training.athlete_settings WHERE member_id = $1`, memberID).
		Scan(&s.Units, &s.BookingReminders, &s.WeeklyGoalKm, &s.Language)
	if database.IsNoRows(err) {
		return domain.DefaultAthleteSettings(), nil
	}
	if err != nil {
		return domain.AthleteSettings{}, fmt.Errorf("training: reading settings: %w", err)
	}
	return s, nil
}

func (r *Repository) SaveSettings(ctx context.Context, memberID string, s domain.AthleteSettings) (domain.AthleteSettings, error) {
	var saved domain.AthleteSettings
	err := r.db.QueryRow(ctx, `
		INSERT INTO training.athlete_settings (member_id, units, booking_reminders, weekly_goal_km, language)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (member_id) DO UPDATE SET
			units = EXCLUDED.units,
			booking_reminders = EXCLUDED.booking_reminders,
			weekly_goal_km = EXCLUDED.weekly_goal_km,
			language = EXCLUDED.language,
			updated_at = now()
		RETURNING units, booking_reminders, weekly_goal_km, language`,
		memberID, s.Units, s.BookingReminders, s.WeeklyGoalKm, s.Language).
		Scan(&saved.Units, &saved.BookingReminders, &saved.WeeklyGoalKm, &saved.Language)
	if err != nil {
		return domain.AthleteSettings{}, fmt.Errorf("training: saving settings: %w", err)
	}
	return saved, nil
}
