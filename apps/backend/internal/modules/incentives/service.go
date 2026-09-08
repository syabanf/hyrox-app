// Package incentives is coach payroll: the schemes that say how a coach is
// paid, the statements those schemes produce, and the payouts that freeze a
// statement for a month.
//
// It has nothing to do with member credits. Coaches are paid money; members
// spend credits, and the two ledgers never meet.
package incentives

import (
	"context"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/modules/scheduling"
	"github.com/syabanf/nuhabit-backend/internal/platform/audit"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Catalog is the port for coaches and branches.
type Catalog interface {
	Coach(ctx context.Context, id string) (domain.Coach, error)
	Coaches(ctx context.Context) ([]domain.Coach, error)
	Branches(ctx context.Context) ([]domain.Branch, error)
	ClassTypes(ctx context.Context, activeOnly bool) ([]domain.ClassType, error)
}

// Scheduling is the port for the classes a coach delivered.
type Scheduling interface {
	Sessions(ctx context.Context, filter scheduling.SessionFilter) ([]domain.ClassSession, error)
	BookingsForSessions(ctx context.Context, sessionIDs []string) (map[string][]domain.Booking, error)
}

// Service implements the payroll use cases.
type Service struct {
	db      *database.DB
	repo    *Repository
	catalog Catalog
	sched   Scheduling
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
	studio  *time.Location
}

func NewService(db *database.DB, repo *Repository, catalog Catalog, sched Scheduling,
	ids id.Generator, c clock.Clock, auditor audit.Recorder, studio *time.Location) *Service {
	if studio == nil {
		studio = time.UTC
	}
	return &Service{db: db, repo: repo, catalog: catalog, sched: sched,
		ids: ids, clock: c, auditor: auditor, studio: studio}
}

// Actor identifies who performed an administrative action.
type Actor struct {
	ID   string
	Name string
}

// SchemeView is a scheme with the coach it belongs to named.
type SchemeView struct {
	Scheme    domain.IncentiveScheme `json:"scheme"`
	CoachName *string                `json:"coachName"`
}

func (s *Service) Schemes(ctx context.Context) ([]SchemeView, error) {
	schemes, err := s.repo.Schemes(ctx)
	if err != nil {
		return nil, err
	}
	coaches, err := s.catalog.Coaches(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, c := range coaches {
		names[c.ID] = c.Name
	}

	views := make([]SchemeView, 0, len(schemes))
	for _, scheme := range schemes {
		view := SchemeView{Scheme: scheme}
		if scheme.CoachID != nil {
			if name, ok := names[*scheme.CoachID]; ok {
				view.CoachName = &name
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// SchemeInput creates or replaces a scheme.
type SchemeInput struct {
	CoachID                   *string
	SessionFeeIDR             int64
	PerAttendeeIDR            int64
	FullClassBonusIDR         int64
	FullClassThresholdPercent int
	NoShowPenaltyIDR          int64
	Active                    bool
}

func (s *Service) CreateScheme(ctx context.Context, in SchemeInput, actor Actor) (SchemeView, error) {
	if in.CoachID != nil {
		if _, err := s.catalog.Coach(ctx, *in.CoachID); err != nil {
			return SchemeView{}, err
		}
	}
	scheme := domain.IncentiveScheme{
		ID:                        s.ids.New(id.Scheme),
		CoachID:                   in.CoachID,
		SessionFeeIDR:             in.SessionFeeIDR,
		PerAttendeeIDR:            in.PerAttendeeIDR,
		FullClassBonusIDR:         in.FullClassBonusIDR,
		FullClassThresholdPercent: in.FullClassThresholdPercent,
		NoShowPenaltyIDR:          in.NoShowPenaltyIDR,
		Active:                    in.Active,
	}
	created, err := s.repo.InsertScheme(ctx, scheme)
	if err != nil {
		return SchemeView{}, err
	}
	if err := s.auditor.Record(ctx, audit.Event{
		EntityType: "incentive_scheme", EntityID: created.ID, Action: "create",
		ActorID: actor.ID, ActorName: actor.Name,
	}); err != nil {
		return SchemeView{}, err
	}
	return SchemeView{Scheme: created}, nil
}

// UpdateScheme replaces a scheme's terms. A scheme cannot change owner, and
// the organization default cannot be switched off: every coach needs a
// fallback to be paid by.
func (s *Service) UpdateScheme(ctx context.Context, schemeID string, in SchemeInput, actor Actor) (SchemeView, error) {
	current, err := s.repo.Scheme(ctx, schemeID)
	if err != nil {
		return SchemeView{}, err
	}
	if in.CoachID != nil && (current.CoachID == nil || *current.CoachID != *in.CoachID) {
		return SchemeView{}, httpx.Conflict("COACH_MISMATCH", "A scheme cannot be moved to another coach.")
	}
	if current.CoachID == nil && !in.Active {
		return SchemeView{}, httpx.Conflict("DEFAULT_REQUIRED", "The organization default scheme must stay active.")
	}

	current.SessionFeeIDR = in.SessionFeeIDR
	current.PerAttendeeIDR = in.PerAttendeeIDR
	current.FullClassBonusIDR = in.FullClassBonusIDR
	current.FullClassThresholdPercent = in.FullClassThresholdPercent
	current.NoShowPenaltyIDR = in.NoShowPenaltyIDR
	current.Active = in.Active

	updated, err := s.repo.UpdateScheme(ctx, current)
	if err != nil {
		return SchemeView{}, err
	}
	if err := s.auditor.Record(ctx, audit.Event{
		EntityType: "incentive_scheme", EntityID: schemeID, Action: "update",
		ActorID: actor.ID, ActorName: actor.Name,
	}); err != nil {
		return SchemeView{}, err
	}
	return SchemeView{Scheme: updated}, nil
}

// StatementView is one coach's earnings for a period, with a payout attached
// when the month has already been frozen.
type StatementView struct {
	CoachID     string                      `json:"coachId"`
	CoachName   string                      `json:"coachName"`
	BranchID    string                      `json:"branchId"`
	BranchName  string                      `json:"branchName"`
	PeriodMonth string                      `json:"periodMonth"`
	PeriodStart time.Time                   `json:"periodStart"`
	PeriodEnd   time.Time                   `json:"periodEnd"`
	Scheme      domain.IncentiveScheme      `json:"scheme"`
	Lines       []domain.CoachStatementLine `json:"lines"`
	Totals      domain.CoachStatementTotals `json:"totals"`
	Payout      *PayoutView                 `json:"payout"`
}

// Statements computes every coach's earnings for a month.
func (s *Service) Statements(ctx context.Context, periodMonth, branchID string) ([]StatementView, error) {
	period, err := domain.MonthPeriod(periodMonth, s.studio)
	if err != nil {
		return nil, httpx.Invalid("Period must be in YYYY-MM form.")
	}
	defaultScheme, err := s.repo.DefaultScheme(ctx)
	if err != nil {
		return nil, err
	}
	schemes, err := s.repo.Schemes(ctx)
	if err != nil {
		return nil, err
	}
	byCoach := map[string]domain.IncentiveScheme{}
	for _, scheme := range schemes {
		if scheme.CoachID != nil {
			byCoach[*scheme.CoachID] = scheme
		}
	}

	coaches, err := s.catalog.Coaches(ctx)
	if err != nil {
		return nil, err
	}
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return nil, err
	}
	branchNames := map[string]string{}
	for _, b := range branches {
		branchNames[b.ID] = b.Name
	}
	classTypes, err := s.catalog.ClassTypes(ctx, false)
	if err != nil {
		return nil, err
	}
	classTypeNames := map[string]string{}
	for _, t := range classTypes {
		classTypeNames[t.ID] = t.Name
	}

	// One query for the whole period rather than one per coach.
	sessions, err := s.sched.Sessions(ctx, scheduling.SessionFilter{
		From:     &period.Start,
		To:       &period.End,
		Statuses: []domain.SessionStatus{domain.SessionCompleted},
		Limit:    5000,
	})
	if err != nil {
		return nil, err
	}
	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
	}
	bookings, err := s.sched.BookingsForSessions(ctx, sessionIDs)
	if err != nil {
		return nil, err
	}

	payouts, err := s.repo.Payouts(ctx, PayoutFilter{PeriodStart: &period.Start})
	if err != nil {
		return nil, err
	}
	payoutByCoach := map[string]domain.IncentivePayout{}
	for _, p := range payouts {
		if p.Status != domain.PayoutVoid {
			payoutByCoach[p.CoachID] = p
		}
	}

	views := make([]StatementView, 0, len(coaches))
	for _, coach := range coaches {
		if branchID != "" && coach.BranchID != branchID {
			continue
		}
		override, hasOverride := byCoach[coach.ID]
		scheme := defaultScheme
		if hasOverride {
			scheme = domain.ResolveScheme(defaultScheme, &override)
		}

		statement := domain.ComputeCoachStatement(domain.StatementInput{
			CoachID:           coach.ID,
			Sessions:          sessions,
			BookingsBySession: bookings,
			ClassTypeNames:    classTypeNames,
			Scheme:            scheme,
			Period:            period,
		})

		view := StatementView{
			CoachID:     coach.ID,
			CoachName:   coach.Name,
			BranchID:    coach.BranchID,
			BranchName:  branchNames[coach.BranchID],
			PeriodMonth: periodMonth,
			PeriodStart: period.Start,
			PeriodEnd:   period.End,
			Scheme:      scheme,
			Lines:       statement.Lines,
			Totals:      statement.Totals,
		}
		if payout, ok := payoutByCoach[coach.ID]; ok {
			payoutView := s.payoutView(payout, coach.Name, branchNames[payout.BranchID])
			view.Payout = &payoutView
		}
		views = append(views, view)
	}
	return views, nil
}

// PayoutView is a payout with names and its period label.
type PayoutView struct {
	Payout      domain.IncentivePayout `json:"payout"`
	CoachName   string                 `json:"coachName"`
	BranchName  string                 `json:"branchName"`
	PeriodMonth string                 `json:"periodMonth"`
}

func (s *Service) payoutView(p domain.IncentivePayout, coachName, branchName string) PayoutView {
	return PayoutView{
		Payout:      p,
		CoachName:   coachName,
		BranchName:  branchName,
		PeriodMonth: domain.PeriodMonthOf(p.PeriodStart, s.studio),
	}
}

func (s *Service) Payouts(ctx context.Context, periodMonth, coachID, status string) ([]PayoutView, error) {
	filter := PayoutFilter{CoachID: coachID, Status: status}
	if periodMonth != "" {
		period, err := domain.MonthPeriod(periodMonth, s.studio)
		if err != nil {
			return nil, httpx.Invalid("Period must be in YYYY-MM form.")
		}
		filter.PeriodStart = &period.Start
	}

	payouts, err := s.repo.Payouts(ctx, filter)
	if err != nil {
		return nil, err
	}
	coaches, err := s.catalog.Coaches(ctx)
	if err != nil {
		return nil, err
	}
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return nil, err
	}
	coachNames := map[string]string{}
	for _, c := range coaches {
		coachNames[c.ID] = c.Name
	}
	branchNames := map[string]string{}
	for _, b := range branches {
		branchNames[b.ID] = b.Name
	}

	views := make([]PayoutView, 0, len(payouts))
	for _, p := range payouts {
		views = append(views, s.payoutView(p, coachNames[p.CoachID], branchNames[p.BranchID]))
	}
	return views, nil
}

// CreatePayout freezes a coach's statement for a month.
//
// The statement is stored as it stands right now. Later attendance edits do
// not reach back into an approved or paid payout: to correct one, void it and
// create a replacement.
func (s *Service) CreatePayout(ctx context.Context, coachID, periodMonth string, actor Actor) (PayoutView, error) {
	coach, err := s.catalog.Coach(ctx, coachID)
	if err != nil {
		return PayoutView{}, err
	}
	statements, err := s.Statements(ctx, periodMonth, "")
	if err != nil {
		return PayoutView{}, err
	}

	var statement *StatementView
	for i := range statements {
		if statements[i].CoachID == coachID {
			statement = &statements[i]
			break
		}
	}
	if statement == nil || len(statement.Lines) == 0 {
		return PayoutView{}, httpx.Conflict("NO_SESSIONS",
			"That coach delivered no classes in %s.", periodMonth)
	}

	now := s.clock.Now()
	payout := domain.IncentivePayout{
		ID:          s.ids.New(id.Payout),
		CoachID:     coachID,
		BranchID:    coach.BranchID,
		PeriodStart: statement.PeriodStart,
		PeriodEnd:   statement.PeriodEnd,
		Statement: domain.CoachStatement{
			CoachID: coachID, Lines: statement.Lines, Totals: statement.Totals,
		},
		Status:    domain.PayoutDraft,
		CreatedBy: actor.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	created, err := s.repo.InsertPayout(ctx, payout)
	if err != nil {
		return PayoutView{}, err
	}
	if err := s.auditor.Record(ctx, audit.Event{
		EntityType: "incentive_payout", EntityID: created.ID, Action: "create",
		NewValue: audit.Str(periodMonth), ActorID: actor.ID, ActorName: actor.Name,
	}); err != nil {
		return PayoutView{}, err
	}
	return s.payoutView(created, coach.Name, statement.BranchName), nil
}

// ActPayout moves a payout along the approval chain.
func (s *Service) ActPayout(ctx context.Context, payoutID string, action domain.PayoutAction, paymentReference, note *string, actor Actor) (PayoutView, error) {
	target, ok := domain.PayoutActionTarget[action]
	if !ok {
		return PayoutView{}, httpx.NotFound("action").WithMessage("Unknown payout action %q.", action)
	}

	var view PayoutView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		payout, err := s.repo.Payout(ctx, payoutID)
		if err != nil {
			return err
		}
		previous := payout.Status
		next, err := domain.Transition(domain.PayoutTransitions, payout.Status, target)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "Cannot %s a %s payout.", action, previous)
		}

		now := s.clock.Now()
		payout.Status = next
		switch action {
		case domain.ActionApprove:
			payout.ApprovedBy = &actor.ID
			payout.ApprovedAt = &now
		case domain.ActionPay:
			// Paying without a reference leaves nothing to reconcile the bank
			// statement against.
			if paymentReference == nil || *paymentReference == "" {
				return httpx.Invalid("A payment reference is required to mark a payout paid.")
			}
			payout.PaymentReference = paymentReference
			payout.PaidAt = &now
		case domain.ActionVoid:
			if note == nil || *note == "" {
				return httpx.Invalid("A note is required to void a payout.")
			}
			payout.Note = note
		}

		updated, err := s.repo.UpdatePayoutStatus(ctx, payout)
		if err != nil {
			return err
		}

		coach, err := s.catalog.Coach(ctx, updated.CoachID)
		if err != nil {
			return err
		}
		branches, err := s.catalog.Branches(ctx)
		if err != nil {
			return err
		}
		branchName := ""
		for _, b := range branches {
			if b.ID == updated.BranchID {
				branchName = b.Name
			}
		}
		view = s.payoutView(updated, coach.Name, branchName)

		return s.auditor.Record(ctx, audit.Event{
			EntityType: "incentive_payout", EntityID: payoutID, Action: string(action),
			PreviousValue: audit.Str(string(previous)), NewValue: audit.Str(string(next)),
			ActorID: actor.ID, ActorName: actor.Name,
		})
	})
	return view, err
}

// PayableIDR is what the studio still owes coaches for a month.
func (s *Service) PayableIDR(ctx context.Context, periodMonth string) (int64, error) {
	period, err := domain.MonthPeriod(periodMonth, s.studio)
	if err != nil {
		return 0, nil
	}
	return s.repo.PayableIDR(ctx, period.Start)
}
