package training

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// ── Workouts ─────────────────────────────────────────────────────────────────

const workoutColumns = `id, member_id, type, division, blocks, excluded_exercise_ids,
	total_target_sec, created_at`

func scanWorkout(scan func(...any) error) (domain.GeneratedWorkout, error) {
	var w domain.GeneratedWorkout
	var blocks, excluded []byte
	if err := scan(&w.ID, &w.MemberID, &w.Type, &w.Division, &blocks, &excluded,
		&w.TotalTargetSec, &w.CreatedAt); err != nil {
		return domain.GeneratedWorkout{}, err
	}
	_ = json.Unmarshal(blocks, &w.Blocks)
	_ = json.Unmarshal(excluded, &w.ExcludedExerciseIDs)
	if w.Blocks == nil {
		w.Blocks = []domain.WorkoutBlock{}
	}
	if w.ExcludedExerciseIDs == nil {
		w.ExcludedExerciseIDs = []string{}
	}
	return w, nil
}

func (r *Repository) Workout(ctx context.Context, id string) (domain.GeneratedWorkout, error) {
	w, err := scanWorkout(r.db.QueryRow(ctx,
		`SELECT `+workoutColumns+` FROM training.workouts WHERE id = $1`, id).Scan)
	if database.IsNoRows(err) {
		return domain.GeneratedWorkout{}, httpx.NotFound("workout")
	}
	if err != nil {
		return domain.GeneratedWorkout{}, fmt.Errorf("training: reading workout: %w", err)
	}
	return w, nil
}

// WorkoutsByIDs fetches a batch, for a history list that would otherwise ask
// for one workout per session.
func (r *Repository) WorkoutsByIDs(ctx context.Context, ids []string) (map[string]domain.GeneratedWorkout, error) {
	out := map[string]domain.GeneratedWorkout{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+workoutColumns+` FROM training.workouts WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("training: listing workouts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		w, err := scanWorkout(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("training: scanning workout: %w", err)
		}
		out[w.ID] = w
	}
	return out, rows.Err()
}

func (r *Repository) InsertWorkout(ctx context.Context, w domain.GeneratedWorkout) (domain.GeneratedWorkout, error) {
	blocks, err := json.Marshal(w.Blocks)
	if err != nil {
		return domain.GeneratedWorkout{}, fmt.Errorf("training: encoding blocks: %w", err)
	}
	excluded, err := json.Marshal(w.ExcludedExerciseIDs)
	if err != nil {
		return domain.GeneratedWorkout{}, fmt.Errorf("training: encoding exclusions: %w", err)
	}
	created, err := scanWorkout(r.db.QueryRow(ctx, `
		INSERT INTO training.workouts
			(id, member_id, type, division, blocks, excluded_exercise_ids, total_target_sec)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+workoutColumns,
		w.ID, w.MemberID, w.Type, w.Division, blocks, excluded, w.TotalTargetSec).Scan)
	if err != nil {
		return domain.GeneratedWorkout{}, fmt.Errorf("training: inserting workout: %w", err)
	}
	return created, nil
}

// SaveWorkoutBlocks rewrites the blocks after a substitution.
func (r *Repository) SaveWorkoutBlocks(ctx context.Context, workoutID string, blocks []domain.WorkoutBlock, totalTargetSec int) (domain.GeneratedWorkout, error) {
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return domain.GeneratedWorkout{}, fmt.Errorf("training: encoding blocks: %w", err)
	}
	updated, err := scanWorkout(r.db.QueryRow(ctx,
		`UPDATE training.workouts SET blocks = $2, total_target_sec = $3
		 WHERE id = $1 RETURNING `+workoutColumns,
		workoutID, encoded, totalTargetSec).Scan)
	if database.IsNoRows(err) {
		return domain.GeneratedWorkout{}, httpx.NotFound("workout")
	}
	if err != nil {
		return domain.GeneratedWorkout{}, fmt.Errorf("training: saving blocks: %w", err)
	}
	return updated, nil
}

// ── Sessions ─────────────────────────────────────────────────────────────────

const sessionColumns = `id, workout_id, member_id, status, current_block, started_at, ended_at,
	block_results, pause_count, total_pause_sec, created_at`

func scanSession(scan func(...any) error) (domain.WorkoutSession, error) {
	var s domain.WorkoutSession
	var results []byte
	if err := scan(&s.ID, &s.WorkoutID, &s.MemberID, &s.Status, &s.CurrentBlock,
		&s.StartedAt, &s.EndedAt, &results, &s.PauseCount, &s.TotalPauseSec, &s.CreatedAt); err != nil {
		return domain.WorkoutSession{}, err
	}
	_ = json.Unmarshal(results, &s.BlockResults)
	if s.BlockResults == nil {
		s.BlockResults = []domain.WorkoutBlockResult{}
	}
	return s, nil
}

func (r *Repository) Session(ctx context.Context, id string) (domain.WorkoutSession, error) {
	s, err := scanSession(r.db.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM training.workout_sessions WHERE id = $1`, id).Scan)
	if database.IsNoRows(err) {
		return domain.WorkoutSession{}, httpx.NotFound("session")
	}
	if err != nil {
		return domain.WorkoutSession{}, fmt.Errorf("training: reading session: %w", err)
	}
	return s, nil
}

func (r *Repository) SessionsForMember(ctx context.Context, memberID string) ([]domain.WorkoutSession, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+sessionColumns+` FROM training.workout_sessions
		 WHERE member_id = $1 ORDER BY created_at DESC`, memberID)
	if err != nil {
		return nil, fmt.Errorf("training: listing sessions: %w", err)
	}
	defer rows.Close()

	out := []domain.WorkoutSession{}
	for rows.Next() {
		s, err := scanSession(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("training: scanning session: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) InsertSession(ctx context.Context, s domain.WorkoutSession) (domain.WorkoutSession, error) {
	results, err := json.Marshal(s.BlockResults)
	if err != nil {
		return domain.WorkoutSession{}, fmt.Errorf("training: encoding results: %w", err)
	}
	created, err := scanSession(r.db.QueryRow(ctx, `
		INSERT INTO training.workout_sessions
			(id, workout_id, member_id, status, current_block, started_at, block_results)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+sessionColumns,
		s.ID, s.WorkoutID, s.MemberID, s.Status, s.CurrentBlock, s.StartedAt, results).Scan)
	if database.IsForeignKeyViolation(err) {
		return domain.WorkoutSession{}, httpx.NotFound("workout")
	}
	if err != nil {
		return domain.WorkoutSession{}, fmt.Errorf("training: inserting session: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveSession(ctx context.Context, s domain.WorkoutSession) (domain.WorkoutSession, error) {
	results, err := json.Marshal(s.BlockResults)
	if err != nil {
		return domain.WorkoutSession{}, fmt.Errorf("training: encoding results: %w", err)
	}
	saved, err := scanSession(r.db.QueryRow(ctx, `
		UPDATE training.workout_sessions
		SET status = $2, current_block = $3, started_at = $4, ended_at = $5,
		    block_results = $6, pause_count = $7, total_pause_sec = $8, updated_at = now()
		WHERE id = $1 RETURNING `+sessionColumns,
		s.ID, s.Status, s.CurrentBlock, s.StartedAt, s.EndedAt, results,
		s.PauseCount, s.TotalPauseSec).Scan)
	if database.IsNoRows(err) {
		return domain.WorkoutSession{}, httpx.NotFound("session")
	}
	if err != nil {
		return domain.WorkoutSession{}, fmt.Errorf("training: saving session: %w", err)
	}
	return saved, nil
}

// ── Races ────────────────────────────────────────────────────────────────────

const raceColumns = `id, name, country, region, city, venue, starts_at, ends_at,
	registration_url, image_url, status`

func scanRace(scan func(...any) error) (domain.RaceEvent, error) {
	var e domain.RaceEvent
	err := scan(&e.ID, &e.Name, &e.Country, &e.Region, &e.City, &e.Venue,
		&e.StartsAt, &e.EndsAt, &e.RegistrationURL, &e.ImageURL, &e.Status)
	return e, err
}

// RaceFilter narrows the calendar.
type RaceFilter struct {
	Region string
	// Results asks for races that have already happened instead of the ones
	// still to come.
	Results bool
}

func (r *Repository) Races(ctx context.Context, filter RaceFilter) ([]domain.RaceEvent, error) {
	sql := `SELECT ` + raceColumns + ` FROM training.race_events WHERE ($1 = '' OR region = $1)`
	if filter.Results {
		sql += ` AND status = 'COMPLETED' ORDER BY starts_at DESC`
	} else {
		sql += ` AND status <> 'COMPLETED' ORDER BY starts_at`
	}
	rows, err := r.db.Query(ctx, sql, filter.Region)
	if err != nil {
		return nil, fmt.Errorf("training: listing races: %w", err)
	}
	defer rows.Close()

	out := []domain.RaceEvent{}
	for rows.Next() {
		e, err := scanRace(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("training: scanning race: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) Race(ctx context.Context, id string) (domain.RaceEvent, error) {
	e, err := scanRace(r.db.QueryRow(ctx,
		`SELECT `+raceColumns+` FROM training.race_events WHERE id = $1`, id).Scan)
	if database.IsNoRows(err) {
		return domain.RaceEvent{}, httpx.NotFound("race")
	}
	if err != nil {
		return domain.RaceEvent{}, fmt.Errorf("training: reading race: %w", err)
	}
	return e, nil
}

func (r *Repository) RacesByIDs(ctx context.Context, ids []string) (map[string]domain.RaceEvent, error) {
	out := map[string]domain.RaceEvent{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+raceColumns+` FROM training.race_events WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("training: listing races: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanRace(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("training: scanning race: %w", err)
		}
		out[e.ID] = e
	}
	return out, rows.Err()
}

func (r *Repository) UpsertRace(ctx context.Context, e domain.RaceEvent) (domain.RaceEvent, error) {
	saved, err := scanRace(r.db.QueryRow(ctx, `
		INSERT INTO training.race_events
			(id, name, country, region, city, venue, starts_at, ends_at, registration_url, image_url, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, country = EXCLUDED.country, region = EXCLUDED.region,
			city = EXCLUDED.city, venue = EXCLUDED.venue, starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at, registration_url = EXCLUDED.registration_url,
			image_url = EXCLUDED.image_url, status = EXCLUDED.status, updated_at = now()
		RETURNING `+raceColumns,
		e.ID, e.Name, e.Country, e.Region, e.City, e.Venue, e.StartsAt, e.EndsAt,
		e.RegistrationURL, e.ImageURL, e.Status).Scan)
	if err != nil {
		return domain.RaceEvent{}, fmt.Errorf("training: saving race: %w", err)
	}
	return saved, nil
}

func (r *Repository) DeleteRace(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM training.race_events WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("training: deleting race: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("race")
	}
	return nil
}

// ── Entries ──────────────────────────────────────────────────────────────────

const userRaceColumns = `id, member_id, race_event_id, division, goal_sec, status, result_sec, created_at`

func scanUserRace(scan func(...any) error) (domain.UserRace, error) {
	var u domain.UserRace
	err := scan(&u.ID, &u.MemberID, &u.RaceEventID, &u.Division, &u.GoalSec,
		&u.Status, &u.ResultSec, &u.CreatedAt)
	return u, err
}

func (r *Repository) UserRaces(ctx context.Context, memberID string) ([]domain.UserRace, error) {
	return r.collectUserRaces(ctx,
		`SELECT `+userRaceColumns+` FROM training.user_races
		 WHERE member_id = $1 AND status <> 'CANCELLED' ORDER BY created_at DESC`, memberID)
}

func (r *Repository) collectUserRaces(ctx context.Context, sql string, args ...any) ([]domain.UserRace, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("training: listing entries: %w", err)
	}
	defer rows.Close()

	out := []domain.UserRace{}
	for rows.Next() {
		u, err := scanUserRace(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("training: scanning entry: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) UserRace(ctx context.Context, id string) (domain.UserRace, error) {
	u, err := scanUserRace(r.db.QueryRow(ctx,
		`SELECT `+userRaceColumns+` FROM training.user_races WHERE id = $1`, id).Scan)
	if database.IsNoRows(err) {
		return domain.UserRace{}, httpx.NotFound("entry")
	}
	if err != nil {
		return domain.UserRace{}, fmt.Errorf("training: reading entry: %w", err)
	}
	return u, nil
}

// EntryCounts is how many people have entered each of these races, and which
// entry is the viewer's own.
func (r *Repository) EntryCounts(ctx context.Context, memberID string) (map[string]int, map[string]bool, error) {
	rows, err := r.db.Query(ctx, `
		SELECT race_event_id, count(*), bool_or(member_id = $1)
		FROM training.user_races WHERE status <> 'CANCELLED' GROUP BY race_event_id`, memberID)
	if err != nil {
		return nil, nil, fmt.Errorf("training: counting entries: %w", err)
	}
	defer rows.Close()

	counts, joined := map[string]int{}, map[string]bool{}
	for rows.Next() {
		var raceID string
		var count int
		var mine bool
		if err := rows.Scan(&raceID, &count, &mine); err != nil {
			return nil, nil, fmt.Errorf("training: scanning entry count: %w", err)
		}
		counts[raceID] = count
		joined[raceID] = mine
	}
	return counts, joined, rows.Err()
}

func (r *Repository) InsertUserRace(ctx context.Context, u domain.UserRace) (domain.UserRace, error) {
	created, err := scanUserRace(r.db.QueryRow(ctx, `
		INSERT INTO training.user_races (id, member_id, race_event_id, division, goal_sec, status)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+userRaceColumns,
		u.ID, u.MemberID, u.RaceEventID, u.Division, u.GoalSec, u.Status).Scan)
	if database.IsUniqueViolation(err) {
		return domain.UserRace{}, httpx.Conflict("ALREADY_ENTERED", "You are already entered for that race.")
	}
	if database.IsForeignKeyViolation(err) {
		return domain.UserRace{}, httpx.NotFound("race")
	}
	if err != nil {
		return domain.UserRace{}, fmt.Errorf("training: entering race: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveUserRace(ctx context.Context, u domain.UserRace) (domain.UserRace, error) {
	saved, err := scanUserRace(r.db.QueryRow(ctx, `
		UPDATE training.user_races
		SET division = $2, goal_sec = $3, status = $4, result_sec = $5, updated_at = now()
		WHERE id = $1 RETURNING `+userRaceColumns,
		u.ID, u.Division, u.GoalSec, u.Status, u.ResultSec).Scan)
	if database.IsNoRows(err) {
		return domain.UserRace{}, httpx.NotFound("entry")
	}
	if err != nil {
		return domain.UserRace{}, fmt.Errorf("training: saving entry: %w", err)
	}
	return saved, nil
}

// ActivityDates is when the member last trained, for the readiness score.
func (r *Repository) ActivityDates(ctx context.Context, memberID string, since time.Time) ([]time.Time, error) {
	rows, err := r.db.Query(ctx,
		`SELECT started_at FROM training.activities WHERE member_id = $1 AND started_at >= $2`,
		memberID, since)
	if err != nil {
		return nil, fmt.Errorf("training: listing activity dates: %w", err)
	}
	defer rows.Close()

	out := []time.Time{}
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("training: scanning activity date: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
