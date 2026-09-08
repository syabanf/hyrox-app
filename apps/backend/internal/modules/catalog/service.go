// Package catalog owns what the studio configures: the organization, its
// branches and gates, coaches, class templates, credit packages, the exercise
// library, and the business rules everything else obeys.
//
// It depends on no other module, which is why it is also the module other
// modules read their rules from.
package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/audit"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// Service is the catalog module's public API. Other modules hold a narrow
// interface over it, never the struct itself.
type Service struct {
	repo    *Repository
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
}

func NewService(repo *Repository, ids id.Generator, c clock.Clock, auditor audit.Recorder) *Service {
	return &Service{repo: repo, ids: ids, clock: c, auditor: auditor}
}

// ── Rules ────────────────────────────────────────────────────────────────────

// Rules returns the organization defaults.
func (s *Service) Rules(ctx context.Context) (domain.BusinessRules, error) {
	return s.repo.Rules(ctx)
}

// RulesForBranch resolves the rules that apply at one branch: organization
// defaults with the branch's overrides merged over them.
//
// This is the method every other module calls before making a timing decision,
// so a branch can run a different QR TTL or cancellation window without any of
// them knowing branches can differ.
func (s *Service) RulesForBranch(ctx context.Context, branchID string) (domain.BusinessRules, error) {
	defaults, err := s.repo.Rules(ctx)
	if err != nil {
		return domain.BusinessRules{}, err
	}
	if branchID == "" {
		return defaults, nil
	}
	branch, err := s.repo.Branch(ctx, branchID)
	if err != nil {
		// An unknown branch falls back to the defaults rather than failing the
		// caller: the rules are advisory configuration, not the operation.
		return defaults, nil
	}
	return domain.ResolveRules(defaults, branch.RulesOverride), nil
}

// UpdateRules applies a partial update and records who changed what.
func (s *Service) UpdateRules(ctx context.Context, patch domain.RulesOverride, actor Actor) (domain.BusinessRules, error) {
	current, err := s.repo.Rules(ctx)
	if err != nil {
		return domain.BusinessRules{}, err
	}
	merged := domain.ResolveRules(current, &patch)
	if merged == current {
		return current, nil
	}

	updated, err := s.repo.UpdateRules(ctx, merged)
	if err != nil {
		return domain.BusinessRules{}, err
	}
	if err := s.auditor.Record(ctx, audit.Event{
		EntityType:    "business_rules",
		EntityID:      "singleton",
		Action:        "update",
		PreviousValue: audit.Str(fmt.Sprintf("%+v", current)),
		NewValue:      audit.Str(fmt.Sprintf("%+v", updated)),
		ActorID:       actor.ID,
		ActorName:     actor.Name,
	}); err != nil {
		return domain.BusinessRules{}, err
	}
	return updated, nil
}

// RulesView is the config screen's shape: defaults plus every branch that
// deviates from them.
type RulesView struct {
	Defaults        domain.BusinessRules `json:"defaults"`
	BranchOverrides []BranchOverrideView `json:"branchOverrides"`
}

type BranchOverrideView struct {
	BranchID   string               `json:"branchId"`
	BranchName string               `json:"branchName"`
	Override   domain.RulesOverride `json:"override"`
}

func (s *Service) RulesView(ctx context.Context) (RulesView, error) {
	defaults, err := s.repo.Rules(ctx)
	if err != nil {
		return RulesView{}, err
	}
	branches, err := s.repo.Branches(ctx)
	if err != nil {
		return RulesView{}, err
	}
	overrides := []BranchOverrideView{}
	for _, b := range branches {
		if b.RulesOverride == nil || b.RulesOverride.IsEmpty() {
			continue
		}
		overrides = append(overrides, BranchOverrideView{
			BranchID: b.ID, BranchName: b.Name, Override: *b.RulesOverride,
		})
	}
	return RulesView{Defaults: defaults, BranchOverrides: overrides}, nil
}

// Actor identifies who is performing an administrative action.
type Actor struct {
	ID   string
	Name string
}

// ── Branches ─────────────────────────────────────────────────────────────────

func (s *Service) Branches(ctx context.Context) ([]domain.Branch, error) { return s.repo.Branches(ctx) }

func (s *Service) Branch(ctx context.Context, id string) (domain.Branch, error) {
	return s.repo.Branch(ctx, id)
}

// BranchInput carries a create or update from the transport layer.
type BranchInput struct {
	Name           string
	Address        string
	Timezone       string
	OperatingHours string
	Status         domain.ActiveStatus
	ManagerName    *string
}

func (s *Service) CreateBranch(ctx context.Context, in BranchInput, actor Actor) (domain.Branch, error) {
	org, err := s.repo.Organization(ctx)
	if err != nil {
		return domain.Branch{}, err
	}
	branch := domain.Branch{
		ID:             s.ids.New(id.Branch),
		OrganizationID: org.ID,
		Name:           strings.TrimSpace(in.Name),
		Address:        strings.TrimSpace(in.Address),
		Timezone:       defaultString(in.Timezone, "Asia/Jakarta"),
		OperatingHours: defaultString(in.OperatingHours, "06:00 - 22:00"),
		Status:         defaultStatus(in.Status),
		ManagerName:    in.ManagerName,
	}
	created, err := s.repo.InsertBranch(ctx, branch)
	if err != nil {
		return domain.Branch{}, err
	}
	return created, s.record(ctx, "branch", created.ID, "create", nil, audit.Str(created.Name), actor)
}

func (s *Service) UpdateBranch(ctx context.Context, id string, in BranchInput, actor Actor) (domain.Branch, error) {
	current, err := s.repo.Branch(ctx, id)
	if err != nil {
		return domain.Branch{}, err
	}
	updated := current
	if in.Name != "" {
		updated.Name = strings.TrimSpace(in.Name)
	}
	if in.Address != "" {
		updated.Address = strings.TrimSpace(in.Address)
	}
	if in.Timezone != "" {
		updated.Timezone = in.Timezone
	}
	if in.OperatingHours != "" {
		updated.OperatingHours = in.OperatingHours
	}
	if in.Status != "" {
		updated.Status = in.Status
	}
	if in.ManagerName != nil {
		updated.ManagerName = in.ManagerName
	}

	saved, err := s.repo.UpdateBranch(ctx, updated)
	if err != nil {
		return domain.Branch{}, err
	}
	return saved, s.record(ctx, "branch", id, "update", audit.Str(current.Name), audit.Str(saved.Name), actor)
}

func (s *Service) DeleteBranch(ctx context.Context, id string, actor Actor) error {
	branch, err := s.repo.Branch(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteBranch(ctx, id); err != nil {
		return err
	}
	return s.record(ctx, "branch", id, "delete", audit.Str(branch.Name), nil, actor)
}

// ── Gates ────────────────────────────────────────────────────────────────────

func (s *Service) Gates(ctx context.Context) ([]domain.Gate, error) { return s.repo.Gates(ctx) }

func (s *Service) Gate(ctx context.Context, id string) (domain.Gate, error) {
	return s.repo.Gate(ctx, id)
}

type GateInput struct {
	Name     string
	BranchID string
	Status   domain.GateStatus
}

func (s *Service) CreateGate(ctx context.Context, in GateInput, actor Actor) (domain.Gate, error) {
	gate := domain.Gate{
		ID:       s.ids.New(id.Gate),
		BranchID: in.BranchID,
		Name:     strings.TrimSpace(in.Name),
		Status:   in.Status,
	}
	if gate.Status == "" {
		gate.Status = domain.GateOnline
	}
	created, err := s.repo.InsertGate(ctx, gate)
	if err != nil {
		return domain.Gate{}, err
	}
	return created, s.record(ctx, "gate", created.ID, "create", nil, audit.Str(created.Name), actor)
}

func (s *Service) UpdateGate(ctx context.Context, id string, in GateInput, actor Actor) (domain.Gate, error) {
	current, err := s.repo.Gate(ctx, id)
	if err != nil {
		return domain.Gate{}, err
	}
	updated := current
	if in.Name != "" {
		updated.Name = strings.TrimSpace(in.Name)
	}
	if in.BranchID != "" {
		updated.BranchID = in.BranchID
	}
	if in.Status != "" {
		updated.Status = in.Status
	}
	saved, err := s.repo.UpdateGate(ctx, updated)
	if err != nil {
		return domain.Gate{}, err
	}
	return saved, s.record(ctx, "gate", id, "update",
		audit.Str(string(current.Status)), audit.Str(string(saved.Status)), actor)
}

func (s *Service) DeleteGate(ctx context.Context, id string, actor Actor) error {
	if err := s.repo.DeleteGate(ctx, id); err != nil {
		return err
	}
	return s.record(ctx, "gate", id, "delete", nil, nil, actor)
}

// ── Coaches ──────────────────────────────────────────────────────────────────

func (s *Service) Coaches(ctx context.Context) ([]domain.Coach, error) { return s.repo.Coaches(ctx) }

func (s *Service) Coach(ctx context.Context, id string) (domain.Coach, error) {
	return s.repo.Coach(ctx, id)
}

type CoachInput struct {
	Name           string
	Bio            string
	Specialization string
	BranchID       string
	Status         domain.ActiveStatus
}

func (s *Service) CreateCoach(ctx context.Context, in CoachInput, actor Actor) (domain.Coach, error) {
	coach := domain.Coach{
		ID:             s.ids.New(id.Coach),
		Name:           strings.TrimSpace(in.Name),
		Bio:            in.Bio,
		Specialization: in.Specialization,
		BranchID:       in.BranchID,
		Status:         defaultStatus(in.Status),
	}
	created, err := s.repo.InsertCoach(ctx, coach)
	if err != nil {
		return domain.Coach{}, err
	}
	return created, s.record(ctx, "coach", created.ID, "create", nil, audit.Str(created.Name), actor)
}

func (s *Service) UpdateCoach(ctx context.Context, id string, in CoachInput, actor Actor) (domain.Coach, error) {
	current, err := s.repo.Coach(ctx, id)
	if err != nil {
		return domain.Coach{}, err
	}
	updated := current
	if in.Name != "" {
		updated.Name = strings.TrimSpace(in.Name)
	}
	if in.Bio != "" {
		updated.Bio = in.Bio
	}
	if in.Specialization != "" {
		updated.Specialization = in.Specialization
	}
	if in.BranchID != "" {
		updated.BranchID = in.BranchID
	}
	if in.Status != "" {
		updated.Status = in.Status
	}
	saved, err := s.repo.UpdateCoach(ctx, updated)
	if err != nil {
		return domain.Coach{}, err
	}
	return saved, s.record(ctx, "coach", id, "update", audit.Str(current.Name), audit.Str(saved.Name), actor)
}

// DeleteCoach refuses while the coach still has classes to teach: deleting
// them would leave sessions pointing at nobody.
func (s *Service) DeleteCoach(ctx context.Context, id string, upcoming int, actor Actor) error {
	if upcoming > 0 {
		return httpx.ErrInUse.WithMessage(
			"This coach is assigned to %d upcoming session(s). Reassign them first.", upcoming)
	}
	if err := s.repo.DeleteCoach(ctx, id); err != nil {
		return err
	}
	return s.record(ctx, "coach", id, "delete", nil, nil, actor)
}

// ── Class types ──────────────────────────────────────────────────────────────

func (s *Service) ClassTypes(ctx context.Context, activeOnly bool) ([]domain.ClassType, error) {
	return s.repo.ClassTypes(ctx, activeOnly)
}

func (s *Service) ClassType(ctx context.Context, id string) (domain.ClassType, error) {
	return s.repo.ClassType(ctx, id)
}

type ClassTypeInput struct {
	Name               string
	Description        string
	DefaultDurationMin int
	DefaultCreditCost  int
	DefaultCapacity    int
	Active             *bool
}

func (s *Service) CreateClassType(ctx context.Context, in ClassTypeInput, actor Actor) (domain.ClassType, error) {
	classType := domain.ClassType{
		ID:                 s.ids.New(id.ClassType),
		Name:               strings.TrimSpace(in.Name),
		Description:        in.Description,
		DefaultDurationMin: in.DefaultDurationMin,
		DefaultCreditCost:  in.DefaultCreditCost,
		DefaultCapacity:    in.DefaultCapacity,
		Active:             in.Active == nil || *in.Active,
	}
	created, err := s.repo.InsertClassType(ctx, classType)
	if err != nil {
		return domain.ClassType{}, err
	}
	return created, s.record(ctx, "class_type", created.ID, "create", nil, audit.Str(created.Name), actor)
}

func (s *Service) UpdateClassType(ctx context.Context, id string, in ClassTypeInput, actor Actor) (domain.ClassType, error) {
	current, err := s.repo.ClassType(ctx, id)
	if err != nil {
		return domain.ClassType{}, err
	}
	updated := current
	if in.Name != "" {
		updated.Name = strings.TrimSpace(in.Name)
	}
	if in.Description != "" {
		updated.Description = in.Description
	}
	if in.DefaultDurationMin > 0 {
		updated.DefaultDurationMin = in.DefaultDurationMin
	}
	if in.DefaultCreditCost > 0 {
		updated.DefaultCreditCost = in.DefaultCreditCost
	}
	if in.DefaultCapacity > 0 {
		updated.DefaultCapacity = in.DefaultCapacity
	}
	if in.Active != nil {
		updated.Active = *in.Active
	}
	saved, err := s.repo.UpdateClassType(ctx, updated)
	if err != nil {
		return domain.ClassType{}, err
	}
	return saved, s.record(ctx, "class_type", id, "update", audit.Str(current.Name), audit.Str(saved.Name), actor)
}

// DeleteClassType refuses while sessions still reference the template.
func (s *Service) DeleteClassType(ctx context.Context, id string, sessions int, actor Actor) error {
	if sessions > 0 {
		return httpx.ErrInUse.WithMessage(
			"%d session(s) use this class type. Deactivate it instead.", sessions)
	}
	if err := s.repo.DeleteClassType(ctx, id); err != nil {
		return err
	}
	return s.record(ctx, "class_type", id, "delete", nil, nil, actor)
}

// ── Packages ─────────────────────────────────────────────────────────────────

func (s *Service) Packages(ctx context.Context, activeOnly bool) ([]domain.CreditPackage, error) {
	return s.repo.Packages(ctx, activeOnly)
}

func (s *Service) Package(ctx context.Context, id string) (domain.CreditPackage, error) {
	return s.repo.Package(ctx, id)
}

type PackageInput struct {
	Name                   string
	Credits                int
	PriceIDR               int64
	ValidityDays           int
	BranchID               *string
	PurchaseLimitPerMember *int
	ApplicableClassTypeIDs []string
	Status                 domain.PackageStatus
}

func (s *Service) CreatePackage(ctx context.Context, in PackageInput, actor Actor) (domain.CreditPackage, error) {
	pkg := domain.CreditPackage{
		ID:                     s.ids.New(id.Package),
		Name:                   strings.TrimSpace(in.Name),
		Credits:                in.Credits,
		PriceIDR:               in.PriceIDR,
		ValidityDays:           in.ValidityDays,
		BranchID:               in.BranchID,
		PurchaseLimitPerMember: in.PurchaseLimitPerMember,
		ApplicableClassTypeIDs: in.ApplicableClassTypeIDs,
		Status:                 in.Status,
		CreatedAt:              s.clock.Now(),
	}
	if pkg.Status == "" {
		pkg.Status = domain.PackageActive
	}
	created, err := s.repo.InsertPackage(ctx, pkg)
	if err != nil {
		return domain.CreditPackage{}, err
	}
	return created, s.record(ctx, "package", created.ID, "create", nil, audit.Str(created.Name), actor)
}

func (s *Service) UpdatePackage(ctx context.Context, id string, in PackageInput, setCoverage bool, actor Actor) (domain.CreditPackage, error) {
	current, err := s.repo.Package(ctx, id)
	if err != nil {
		return domain.CreditPackage{}, err
	}
	updated := current
	if in.Name != "" {
		updated.Name = strings.TrimSpace(in.Name)
	}
	if in.Credits > 0 {
		updated.Credits = in.Credits
	}
	if in.PriceIDR > 0 {
		updated.PriceIDR = in.PriceIDR
	}
	if in.ValidityDays > 0 {
		updated.ValidityDays = in.ValidityDays
	}
	if in.Status != "" {
		updated.Status = in.Status
	}
	updated.BranchID = in.BranchID
	updated.PurchaseLimitPerMember = in.PurchaseLimitPerMember
	// Coverage is only touched when the caller actually sent the field, so a
	// partial update cannot silently open a restricted package to every class.
	if setCoverage {
		updated.ApplicableClassTypeIDs = in.ApplicableClassTypeIDs
	}

	saved, err := s.repo.UpdatePackage(ctx, updated)
	if err != nil {
		return domain.CreditPackage{}, err
	}
	return saved, s.record(ctx, "package", id, "update", audit.Str(current.Name), audit.Str(saved.Name), actor)
}

// DeletePackage refuses once the package has been sold: history must resolve,
// so a sold package is archived instead.
func (s *Service) DeletePackage(ctx context.Context, id string, payments int, actor Actor) error {
	if payments > 0 {
		return httpx.ErrInUse.WithMessage("Payments reference this package. Archive it instead.")
	}
	if err := s.repo.DeletePackage(ctx, id); err != nil {
		return err
	}
	return s.record(ctx, "package", id, "delete", nil, nil, actor)
}

// ── Exercises ────────────────────────────────────────────────────────────────

func (s *Service) Exercises(ctx context.Context) ([]domain.Exercise, error) {
	return s.repo.Exercises(ctx)
}

func (s *Service) Substitutions(ctx context.Context) ([]domain.SubstitutionRule, error) {
	return s.repo.Substitutions(ctx)
}

// UpdateExercise edits the fields the admin panel exposes.
func (s *Service) UpdateExercise(ctx context.Context, id, name string, difficulty int, videoURL *string, setVideo bool, actor Actor) (domain.Exercise, error) {
	current, err := s.repo.Exercise(ctx, id)
	if err != nil {
		return domain.Exercise{}, err
	}
	if name == "" {
		name = current.Name
	}
	if difficulty == 0 {
		difficulty = current.Difficulty
	}
	video := current.VideoURL
	if setVideo {
		video = videoURL
	}

	saved, err := s.repo.UpdateExercise(ctx, id, strings.TrimSpace(name), difficulty, video)
	if err != nil {
		return domain.Exercise{}, err
	}
	return saved, s.record(ctx, "exercise", id, "update", audit.Str(current.Name), audit.Str(saved.Name), actor)
}

func (s *Service) record(ctx context.Context, entityType, entityID, action string, previous, next *string, actor Actor) error {
	return s.auditor.Record(ctx, audit.Event{
		EntityType:    entityType,
		EntityID:      entityID,
		Action:        action,
		PreviousValue: previous,
		NewValue:      next,
		ActorID:       actor.ID,
		ActorName:     actor.Name,
	})
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultStatus(status domain.ActiveStatus) domain.ActiveStatus {
	if status == "" {
		return domain.StatusActive
	}
	return status
}
