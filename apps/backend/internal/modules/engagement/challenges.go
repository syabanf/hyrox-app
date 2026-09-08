package engagement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Community challenges: the studio sets a distance and a window, members join,
// and the athlete tab shows them where they are against everybody else.
//
// The challenge lives here because the studio runs it. Progress is computed in
// the training module, which owns the activities it is measured from.

const challengeColumns = `id, name, description, type, target_km, starts_at, ends_at, created_at`

func scanChallenge(scan func(...any) error) (domain.Challenge, error) {
	var c domain.Challenge
	err := scan(&c.ID, &c.Name, &c.Description, &c.Type, &c.TargetKm,
		&c.StartsAt, &c.EndsAt, &c.CreatedAt)
	return c, err
}

func (r *Repository) Challenges(ctx context.Context) ([]domain.Challenge, error) {
	return r.collectChallenges(ctx,
		`SELECT `+challengeColumns+` FROM engagement.challenges ORDER BY starts_at DESC`)
}

// RunningChallenges is what a member could still contribute to: anything that
// has not finished. A challenge that closed last month is studio history, not
// something to show on the training tab.
func (r *Repository) RunningChallenges(ctx context.Context, now time.Time) ([]domain.Challenge, error) {
	return r.collectChallenges(ctx,
		`SELECT `+challengeColumns+` FROM engagement.challenges
		 WHERE ends_at >= $1 ORDER BY starts_at`, now)
}

func (r *Repository) collectChallenges(ctx context.Context, sql string, args ...any) ([]domain.Challenge, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("engagement: listing challenges: %w", err)
	}
	defer rows.Close()

	out := []domain.Challenge{}
	for rows.Next() {
		c, err := scanChallenge(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("engagement: scanning challenge: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Challenge(ctx context.Context, id string) (domain.Challenge, error) {
	c, err := scanChallenge(r.db.QueryRow(ctx,
		`SELECT `+challengeColumns+` FROM engagement.challenges WHERE id = $1`, id).Scan)
	if database.IsNoRows(err) {
		return domain.Challenge{}, httpx.NotFound("challenge")
	}
	if err != nil {
		return domain.Challenge{}, fmt.Errorf("engagement: reading challenge: %w", err)
	}
	return c, nil
}

func (r *Repository) UpsertChallenge(ctx context.Context, c domain.Challenge) (domain.Challenge, error) {
	saved, err := scanChallenge(r.db.QueryRow(ctx, `
		INSERT INTO engagement.challenges (id, name, description, type, target_km, starts_at, ends_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, description = EXCLUDED.description, type = EXCLUDED.type,
			target_km = EXCLUDED.target_km, starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at, updated_at = now()
		RETURNING `+challengeColumns,
		c.ID, c.Name, c.Description, c.Type, c.TargetKm, c.StartsAt, c.EndsAt).Scan)
	if database.IsCheckViolation(err) {
		return domain.Challenge{}, httpx.Invalid("A challenge has to end after it starts.")
	}
	if err != nil {
		return domain.Challenge{}, fmt.Errorf("engagement: saving challenge: %w", err)
	}
	return saved, nil
}

func (r *Repository) DeleteChallenge(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM engagement.challenges WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("engagement: deleting challenge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("challenge")
	}
	return nil
}

// ChallengeParticipants is who joined what, keyed by challenge.
func (r *Repository) ChallengeParticipants(ctx context.Context) (map[string][]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT challenge_id, member_id FROM engagement.challenge_joins ORDER BY joined_at`)
	if err != nil {
		return nil, fmt.Errorf("engagement: listing challenge joins: %w", err)
	}
	defer rows.Close()

	out := map[string][]string{}
	for rows.Next() {
		var challengeID, memberID string
		if err := rows.Scan(&challengeID, &memberID); err != nil {
			return nil, fmt.Errorf("engagement: scanning challenge join: %w", err)
		}
		out[challengeID] = append(out[challengeID], memberID)
	}
	return out, rows.Err()
}

// JoinChallenge is idempotent: pressing Join twice is one membership.
func (r *Repository) JoinChallenge(ctx context.Context, challengeID, memberID string, at time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO engagement.challenge_joins (challenge_id, member_id, joined_at)
		VALUES ($1, $2, $3) ON CONFLICT (challenge_id, member_id) DO NOTHING`,
		challengeID, memberID, at)
	if database.IsForeignKeyViolation(err) {
		return httpx.NotFound("challenge")
	}
	if err != nil {
		return fmt.Errorf("engagement: joining challenge: %w", err)
	}
	return nil
}

// ── Use cases ────────────────────────────────────────────────────────────────

// ChallengeWithCount is a challenge and how many people are in it.
type ChallengeWithCount struct {
	Challenge        domain.Challenge `json:"challenge"`
	ParticipantCount int              `json:"participantCount"`
}

func (s *Service) Challenges(ctx context.Context) ([]ChallengeWithCount, error) {
	challenges, err := s.repo.Challenges(ctx)
	if err != nil {
		return nil, err
	}
	participants, err := s.repo.ChallengeParticipants(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ChallengeWithCount, 0, len(challenges))
	for _, c := range challenges {
		out = append(out, ChallengeWithCount{Challenge: c, ParticipantCount: len(participants[c.ID])})
	}
	return out, nil
}

// ChallengeInput is a challenge as the panel submits it.
type ChallengeInput struct {
	Name        string
	Description string
	Type        string
	TargetKm    float64
	StartsAt    time.Time
	EndsAt      time.Time
}

func (s *Service) SaveChallenge(ctx context.Context, challengeID string, in ChallengeInput) (ChallengeWithCount, error) {
	if len(strings.TrimSpace(in.Name)) < 2 {
		return ChallengeWithCount{}, httpx.Invalid("A challenge needs a name.")
	}
	if in.Type != domain.ChallengeAnyType && !domain.IsValidActivityType(in.Type) {
		return ChallengeWithCount{}, httpx.Invalid("A challenge counts ANY activity, or one type of it.")
	}
	if in.TargetKm <= 0 {
		return ChallengeWithCount{}, httpx.Invalid("A challenge needs a distance to aim at.")
	}
	if !in.EndsAt.After(in.StartsAt) {
		return ChallengeWithCount{}, httpx.Invalid("A challenge has to end after it starts.")
	}

	challenge := domain.Challenge{ID: s.ids.New(id.Challenge)}
	if challengeID != "" {
		existing, err := s.repo.Challenge(ctx, challengeID)
		if err != nil {
			return ChallengeWithCount{}, err
		}
		challenge = existing
	}
	challenge.Name = strings.TrimSpace(in.Name)
	challenge.Description = strings.TrimSpace(in.Description)
	challenge.Type = in.Type
	challenge.TargetKm = in.TargetKm
	challenge.StartsAt = in.StartsAt
	challenge.EndsAt = in.EndsAt

	saved, err := s.repo.UpsertChallenge(ctx, challenge)
	if err != nil {
		return ChallengeWithCount{}, err
	}
	participants, err := s.repo.ChallengeParticipants(ctx)
	if err != nil {
		return ChallengeWithCount{}, err
	}
	return ChallengeWithCount{Challenge: saved, ParticipantCount: len(participants[saved.ID])}, nil
}

// DeleteChallenge removes a challenge. Somebody's progress towards it was
// never stored — it is computed from their activities — so nothing of theirs
// is lost beyond the challenge itself.
func (s *Service) DeleteChallenge(ctx context.Context, challengeID string) error {
	if _, err := s.repo.Challenge(ctx, challengeID); err != nil {
		return err
	}
	return s.repo.DeleteChallenge(ctx, challengeID)
}

// ── The training module's view ───────────────────────────────────────────────

// Running is what the athlete tab shows: challenges that have not closed.
func (s *Service) Running(ctx context.Context, now time.Time) ([]domain.Challenge, error) {
	return s.repo.RunningChallenges(ctx, now)
}

func (s *Service) Participants(ctx context.Context) (map[string][]string, error) {
	return s.repo.ChallengeParticipants(ctx)
}

// Join puts a member in a challenge. Joining one that has already closed is
// refused: there is nothing left to contribute to.
func (s *Service) Join(ctx context.Context, challengeID, memberID string) error {
	challenge, err := s.repo.Challenge(ctx, challengeID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	if now.After(challenge.EndsAt) {
		return httpx.Conflict("CHALLENGE_CLOSED", "That challenge has finished.")
	}
	return s.repo.JoinChallenge(ctx, challengeID, memberID, now)
}
