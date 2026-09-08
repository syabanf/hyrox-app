// Package training is the athlete side of the member app: activities,
// segments, clubs, gear, generated workouts and races.
//
// This is its first slice — athlete settings, which the app reads on every
// launch to decide units, language and reminder preferences. The rest of the
// module's tables are already migrated and waiting.
package training

import (
	"context"
	"fmt"
	"net/http"

	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Settings are one member's athlete preferences.
type Settings struct {
	Units            string   `json:"units"`
	BookingReminders bool     `json:"bookingReminders"`
	WeeklyGoalKm     *float64 `json:"weeklyGoalKm"`
	Language         string   `json:"language"`
}

// DefaultSettings are what a member gets before they change anything.
func DefaultSettings() Settings {
	return Settings{Units: "METRIC", BookingReminders: true, Language: "EN"}
}

type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// Settings reads a member's preferences, returning the defaults for a member
// who has never changed them rather than requiring a row to exist.
func (r *Repository) Settings(ctx context.Context, memberID string) (Settings, error) {
	s := DefaultSettings()
	err := r.db.QueryRow(ctx, `
		SELECT units, booking_reminders, weekly_goal_km, language
		FROM training.athlete_settings WHERE member_id = $1`, memberID).
		Scan(&s.Units, &s.BookingReminders, &s.WeeklyGoalKm, &s.Language)
	if database.IsNoRows(err) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("training: reading settings: %w", err)
	}
	return s, nil
}

func (r *Repository) SaveSettings(ctx context.Context, memberID string, s Settings) (Settings, error) {
	var saved Settings
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
		return Settings{}, fmt.Errorf("training: saving settings: %w", err)
	}
	return saved, nil
}

// Service implements the training use cases.
type Service struct {
	repo  *Repository
	clock clock.Clock
}

func NewService(repo *Repository, c clock.Clock) *Service {
	return &Service{repo: repo, clock: c}
}

func (s *Service) Settings(ctx context.Context, memberID string) (Settings, error) {
	return s.repo.Settings(ctx, memberID)
}

// UpdateSettings applies a partial change to a member's preferences.
func (s *Service) UpdateSettings(ctx context.Context, memberID string, patch SettingsPatch) (Settings, error) {
	current, err := s.repo.Settings(ctx, memberID)
	if err != nil {
		return Settings{}, err
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

// SettingsPatch is a partial settings update. The goal is nullable, so
// clearing it is different from leaving it alone.
type SettingsPatch struct {
	Units            *string
	BookingReminders *bool
	WeeklyGoalKm     *float64
	SetWeeklyGoal    bool
	Language         *string
}

// Handler serves the training surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	r.Get("/api/me/settings", h.get, h.guard.RequireMember)
	r.Put("/api/me/settings", h.update, h.guard.RequireMember)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
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
	if s.Units != nil && *s.Units != "METRIC" && *s.Units != "IMPERIAL" {
		return httpx.Invalid("Units must be METRIC or IMPERIAL.")
	}
	if s.Language != nil && *s.Language != "EN" && *s.Language != "ID" {
		return httpx.Invalid("Language must be EN or ID.")
	}
	if s.WeeklyGoalKm != nil && *s.WeeklyGoalKm != nil {
		goal := **s.WeeklyGoalKm
		if goal <= 0 || goal > 1000 {
			return httpx.Invalid("A weekly goal must be between 1 and 1000 km.")
		}
	}
	return nil
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[settingsRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	patch := SettingsPatch{
		Units:            body.Units,
		BookingReminders: body.BookingReminders,
		Language:         body.Language,
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
