package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

const exerciseColumns = `id, name, category, equipment, hyrox_station_order, difficulty, default_spec, video_url`

func scanExercise(row pgx.Row) (domain.Exercise, error) {
	var e domain.Exercise
	var equipment, spec []byte
	if err := row.Scan(&e.ID, &e.Name, &e.Category, &equipment,
		&e.HyroxStationOrder, &e.Difficulty, &spec, &e.VideoURL); err != nil {
		return domain.Exercise{}, err
	}
	if len(equipment) > 0 {
		if err := json.Unmarshal(equipment, &e.Equipment); err != nil {
			return domain.Exercise{}, fmt.Errorf("catalog: decoding equipment: %w", err)
		}
	}
	if e.Equipment == nil {
		e.Equipment = []string{}
	}
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &e.DefaultSpec); err != nil {
			return domain.Exercise{}, fmt.Errorf("catalog: decoding exercise spec: %w", err)
		}
	}
	return e, nil
}

// Exercises lists the movement library, race stations first so the eight
// stations read in race order.
func (r *Repository) Exercises(ctx context.Context) ([]domain.Exercise, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+exerciseColumns+` FROM catalog.exercises
		ORDER BY hyrox_station_order NULLS LAST, name`)
	if err != nil {
		return nil, fmt.Errorf("catalog: listing exercises: %w", err)
	}
	defer rows.Close()

	exercises := []domain.Exercise{}
	for rows.Next() {
		e, err := scanExercise(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: scanning exercise: %w", err)
		}
		exercises = append(exercises, e)
	}
	return exercises, rows.Err()
}

func (r *Repository) Exercise(ctx context.Context, id string) (domain.Exercise, error) {
	e, err := scanExercise(r.db.QueryRow(ctx, `SELECT `+exerciseColumns+` FROM catalog.exercises WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Exercise{}, httpx.NotFound("exercise")
	}
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("catalog: reading exercise: %w", err)
	}
	return e, nil
}

func (r *Repository) InsertExercise(ctx context.Context, e domain.Exercise) (domain.Exercise, error) {
	equipment, err := json.Marshal(orEmpty(e.Equipment))
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("catalog: encoding equipment: %w", err)
	}
	spec, err := json.Marshal(e.DefaultSpec)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("catalog: encoding exercise spec: %w", err)
	}
	created, err := scanExercise(r.db.QueryRow(ctx, `
		INSERT INTO catalog.exercises (id, name, category, equipment, hyrox_station_order, difficulty, default_spec, video_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+exerciseColumns,
		e.ID, e.Name, e.Category, equipment, e.HyroxStationOrder, e.Difficulty, spec, e.VideoURL))
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("catalog: inserting exercise: %w", err)
	}
	return created, nil
}

// UpdateExercise saves the fields the admin panel can edit: the name, the
// difficulty dots, and the how-to video shown to members.
func (r *Repository) UpdateExercise(ctx context.Context, id, name string, difficulty int, videoURL *string) (domain.Exercise, error) {
	updated, err := scanExercise(r.db.QueryRow(ctx, `
		UPDATE catalog.exercises SET name = $2, difficulty = $3, video_url = $4, updated_at = now()
		WHERE id = $1 RETURNING `+exerciseColumns,
		id, name, difficulty, videoURL))
	if database.IsNoRows(err) {
		return domain.Exercise{}, httpx.NotFound("exercise")
	}
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("catalog: updating exercise: %w", err)
	}
	return updated, nil
}

func (r *Repository) Substitutions(ctx context.Context) ([]domain.SubstitutionRule, error) {
	rows, err := r.db.Query(ctx, `
		SELECT original_exercise_id, alternative_exercise_id, similarity, conversion_note
		FROM catalog.substitution_rules ORDER BY original_exercise_id, similarity DESC`)
	if err != nil {
		return nil, fmt.Errorf("catalog: listing substitutions: %w", err)
	}
	defer rows.Close()

	rules := []domain.SubstitutionRule{}
	for rows.Next() {
		var s domain.SubstitutionRule
		if err := rows.Scan(&s.OriginalExerciseID, &s.AlternativeExerciseID, &s.Similarity, &s.ConversionNote); err != nil {
			return nil, fmt.Errorf("catalog: scanning substitution: %w", err)
		}
		rules = append(rules, s)
	}
	return rules, rows.Err()
}

func (r *Repository) InsertSubstitution(ctx context.Context, s domain.SubstitutionRule) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO catalog.substitution_rules (original_exercise_id, alternative_exercise_id, similarity, conversion_note)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (original_exercise_id, alternative_exercise_id)
		DO UPDATE SET similarity = EXCLUDED.similarity, conversion_note = EXCLUDED.conversion_note`,
		s.OriginalExerciseID, s.AlternativeExerciseID, s.Similarity, s.ConversionNote)
	if err != nil {
		return fmt.Errorf("catalog: inserting substitution: %w", err)
	}
	return nil
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
