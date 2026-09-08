// Package scheduling owns the class schedule and the bookings against it:
// creating sessions, taking places, the waitlist, and attendance.
//
// Credits are checked here but never deducted here. Deduction happens at the
// gate, so a member who books and never turns up is handled by the no-show
// policy rather than by having paid twice.
package scheduling

import (
	"context"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/audit"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/outbox"
)

// checkInEarlyWindow is how long before a class starts the gate will admit a
// member for it; checkInLateWindow is how long after it ends.
const (
	checkInEarlyWindow = 45 * time.Minute
	checkInLateWindow  = 15 * time.Minute
)

// Catalog is the port for the templates and rules scheduling depends on.
type Catalog interface {
	ClassType(ctx context.Context, id string) (domain.ClassType, error)
	ClassTypes(ctx context.Context, activeOnly bool) ([]domain.ClassType, error)
	Coach(ctx context.Context, id string) (domain.Coach, error)
	Coaches(ctx context.Context) ([]domain.Coach, error)
	Branches(ctx context.Context) ([]domain.Branch, error)
	RulesForBranch(ctx context.Context, branchID string) (domain.BusinessRules, error)
}

// Members is the port for the member facts booking depends on.
type Members interface {
	Member(ctx context.Context, id string) (domain.Member, error)
	MembersByIDs(ctx context.Context, ids []string) (map[string]domain.Member, error)
}

// Wallet is the port for credits. Scheduling reads a balance and can charge a
// penalty, but never issues credits.
type Wallet interface {
	Balance(ctx context.Context, memberID string) (int, error)
	CoveredClassTypes(ctx context.Context, memberID string) ([]string, error)
	Forfeit(ctx context.Context, memberID string, amount int, description, sourceID string) (domain.CreditLedgerEntry, error)
}

// Service implements the scheduling use cases.
type Service struct {
	db      *database.DB
	repo    *Repository
	catalog Catalog
	members Members
	wallet  Wallet
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
	events  outbox.Publisher
}

func NewService(db *database.DB, repo *Repository, catalog Catalog, members Members, wallet Wallet,
	ids id.Generator, c clock.Clock, auditor audit.Recorder, events outbox.Publisher) *Service {
	return &Service{db: db, repo: repo, catalog: catalog, members: members, wallet: wallet,
		ids: ids, clock: c, auditor: auditor, events: events}
}

// Actor identifies who performed an administrative action.
type Actor struct {
	ID   string
	Name string
}

// ── Sessions ─────────────────────────────────────────────────────────────────

func (s *Service) Sessions(ctx context.Context, filter SessionFilter) ([]domain.ClassSession, error) {
	return s.repo.Sessions(ctx, filter)
}

func (s *Service) Session(ctx context.Context, id string) (domain.ClassSession, error) {
	return s.repo.Session(ctx, id)
}

func (s *Service) Bookings(ctx context.Context, sessionID string) ([]domain.Booking, error) {
	return s.repo.BookingsForSession(ctx, sessionID)
}

func (s *Service) Booking(ctx context.Context, id string) (domain.Booking, error) {
	return s.repo.Booking(ctx, id)
}

func (s *Service) MemberBookings(ctx context.Context, memberID string, limit int) ([]domain.Booking, error) {
	return s.repo.BookingsForMember(ctx, memberID, limit)
}

func (s *Service) Counts(ctx context.Context, sessionIDs []string) (map[string]SessionCounts, error) {
	return s.repo.CountsForSessions(ctx, sessionIDs)
}

func (s *Service) MemberBookingsForSessions(ctx context.Context, memberID string, sessionIDs []string) (map[string]domain.Booking, error) {
	return s.repo.MemberBookingsForSessions(ctx, memberID, sessionIDs)
}

func (s *Service) BookingsForSessions(ctx context.Context, sessionIDs []string) (map[string][]domain.Booking, error) {
	return s.repo.BookingsForSessions(ctx, sessionIDs)
}

func (s *Service) CountSessionsForClassType(ctx context.Context, classTypeID string) (int, error) {
	return s.repo.CountSessionsForClassType(ctx, classTypeID)
}

func (s *Service) CountUpcomingForCoach(ctx context.Context, coachID string) (int, error) {
	return s.repo.CountUpcomingForCoach(ctx, coachID, s.clock.Now())
}

// SessionInput creates a scheduled class.
type SessionInput struct {
	ClassTypeID string
	BranchID    string
	CoachID     string
	StartsAt    time.Time
	DurationMin int
	Capacity    int
	CreditCost  int
	Area        *string
	Publish     bool
}

// CreateSession schedules a class, copying unset values from the class type
// and deriving the booking window from the branch's rules.
func (s *Service) CreateSession(ctx context.Context, in SessionInput, actor Actor) (domain.ClassSession, error) {
	classType, err := s.catalog.ClassType(ctx, in.ClassTypeID)
	if err != nil {
		return domain.ClassSession{}, err
	}
	if _, err := s.catalog.Coach(ctx, in.CoachID); err != nil {
		return domain.ClassSession{}, err
	}
	rules, err := s.catalog.RulesForBranch(ctx, in.BranchID)
	if err != nil {
		return domain.ClassSession{}, err
	}

	duration := in.DurationMin
	if duration <= 0 {
		duration = classType.DefaultDurationMin
	}
	capacity := in.Capacity
	if capacity <= 0 {
		capacity = classType.DefaultCapacity
	}
	creditCost := in.CreditCost
	if creditCost <= 0 {
		creditCost = classType.DefaultCreditCost
	}

	opens, closes := domain.DeriveBookingWindow(in.StartsAt, rules)
	status := domain.SessionDraft
	if in.Publish {
		status = domain.SessionPublished
	}

	session := domain.ClassSession{
		ID:              s.ids.New(id.Session),
		ClassTypeID:     classType.ID,
		BranchID:        in.BranchID,
		CoachID:         in.CoachID,
		StartsAt:        in.StartsAt,
		EndsAt:          in.StartsAt.Add(time.Duration(duration) * time.Minute),
		Capacity:        capacity,
		CreditCost:      creditCost,
		BookingOpensAt:  opens,
		BookingClosesAt: closes,
		Status:          status,
		Area:            in.Area,
	}

	created, err := s.repo.InsertSession(ctx, session)
	if err != nil {
		return domain.ClassSession{}, err
	}
	return created, s.auditor.Record(ctx, audit.Event{
		EntityType: "class_session", EntityID: created.ID, Action: "create",
		NewValue: audit.Str(created.StartsAt.Format(time.RFC3339)),
		ActorID:  actor.ID, ActorName: actor.Name,
	})
}

// SessionPatch is a partial edit of a scheduled class.
type SessionPatch struct {
	CoachID     *string
	Capacity    *int
	StartsAt    *time.Time
	DurationMin *int
	Area        *string
	SetArea     bool
}

// UpdateSession edits a class. Moving the start time re-derives the booking
// window, so a rescheduled class does not keep the old one.
func (s *Service) UpdateSession(ctx context.Context, sessionID string, patch SessionPatch, actor Actor) (domain.ClassSession, error) {
	session, err := s.repo.Session(ctx, sessionID)
	if err != nil {
		return domain.ClassSession{}, err
	}
	previousStart := session.StartsAt
	duration := session.EndsAt.Sub(session.StartsAt)

	if patch.CoachID != nil {
		if _, err := s.catalog.Coach(ctx, *patch.CoachID); err != nil {
			return domain.ClassSession{}, err
		}
		session.CoachID = *patch.CoachID
	}
	if patch.Capacity != nil {
		if *patch.Capacity <= 0 {
			return domain.ClassSession{}, httpx.Invalid("Capacity must be greater than zero.")
		}
		counts, err := s.repo.CountsForSession(ctx, sessionID)
		if err != nil {
			return domain.ClassSession{}, err
		}
		// Shrinking below the people already booked would silently strand them.
		if *patch.Capacity < counts.Confirmed {
			return domain.ClassSession{}, httpx.Conflict("CAPACITY_BELOW_BOOKED",
				"%d members are already booked; capacity cannot go below that.", counts.Confirmed)
		}
		session.Capacity = *patch.Capacity
	}
	if patch.DurationMin != nil && *patch.DurationMin > 0 {
		duration = time.Duration(*patch.DurationMin) * time.Minute
	}
	if patch.StartsAt != nil {
		session.StartsAt = *patch.StartsAt
	}
	session.EndsAt = session.StartsAt.Add(duration)
	if patch.SetArea {
		session.Area = patch.Area
	}

	if !session.StartsAt.Equal(previousStart) {
		rules, err := s.catalog.RulesForBranch(ctx, session.BranchID)
		if err != nil {
			return domain.ClassSession{}, err
		}
		session.BookingOpensAt, session.BookingClosesAt = domain.DeriveBookingWindow(session.StartsAt, rules)
	}

	updated, err := s.repo.UpdateSession(ctx, session)
	if err != nil {
		return domain.ClassSession{}, err
	}
	if !updated.StartsAt.Equal(previousStart) {
		// Members holding a place need to know the class moved.
		if err := s.events.Publish(ctx, outbox.TopicBookingConfirmed, map[string]any{
			"sessionId": updated.ID,
			"changed":   "schedule",
		}, nil); err != nil {
			return domain.ClassSession{}, err
		}
	}
	return updated, s.auditor.Record(ctx, audit.Event{
		EntityType: "class_session", EntityID: sessionID, Action: "update",
		PreviousValue: audit.Str(previousStart.Format(time.RFC3339)),
		NewValue:      audit.Str(updated.StartsAt.Format(time.RFC3339)),
		ActorID:       actor.ID, ActorName: actor.Name,
	})
}

// TransitionSession moves a class through its lifecycle.
func (s *Service) TransitionSession(ctx context.Context, sessionID string, target domain.SessionStatus, actor Actor) (domain.ClassSession, error) {
	session, err := s.repo.Session(ctx, sessionID)
	if err != nil {
		return domain.ClassSession{}, err
	}
	previous := session.Status
	next, err := domain.Transition(domain.SessionTransitions, session.Status, target)
	if err != nil {
		return domain.ClassSession{}, httpx.Conflict("INVALID_TRANSITION",
			"A %s session cannot become %s.", previous, target)
	}
	session.Status = next

	updated, err := s.repo.UpdateSession(ctx, session)
	if err != nil {
		return domain.ClassSession{}, err
	}
	return updated, s.auditor.Record(ctx, audit.Event{
		EntityType: "class_session", EntityID: sessionID, Action: "status_change",
		PreviousValue: audit.Str(string(previous)), NewValue: audit.Str(string(next)),
		ActorID: actor.ID, ActorName: actor.Name,
	})
}

func (s *Service) DeleteSession(ctx context.Context, sessionID string, actor Actor) error {
	counts, err := s.repo.CountsForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if counts.Confirmed > 0 || counts.Waitlist > 0 {
		return httpx.ErrInUse.WithMessage("This session has bookings. Cancel it instead.")
	}
	if err := s.repo.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	return s.auditor.Record(ctx, audit.Event{
		EntityType: "class_session", EntityID: sessionID, Action: "delete",
		ActorID: actor.ID, ActorName: actor.Name,
	})
}

// ── Booking ──────────────────────────────────────────────────────────────────

// BookResult is the outcome of taking a place.
type BookResult struct {
	Booking  domain.Booking `json:"booking"`
	Decision string         `json:"decision"`
}

// Book takes a place in a class, or a place on the waitlist.
//
// The whole decision runs inside a transaction that locks the session row, so
// concurrent requests for the last place are serialized: one confirms, the
// next is waitlisted.
func (s *Service) Book(ctx context.Context, memberID, sessionID string, source domain.BookingSource) (BookResult, error) {
	var result BookResult

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		session, err := s.repo.LockSession(ctx, sessionID)
		if err != nil {
			return err
		}
		member, err := s.members.Member(ctx, memberID)
		if err != nil {
			return err
		}
		balance, err := s.wallet.Balance(ctx, memberID)
		if err != nil {
			return err
		}
		counts, err := s.repo.CountsForSession(ctx, sessionID)
		if err != nil {
			return err
		}
		_, alreadyBooked, err := s.repo.ActiveBooking(ctx, memberID, sessionID)
		if err != nil {
			return err
		}

		decision := domain.EvaluateBooking(domain.BookingRequest{
			Member:                   member,
			Session:                  session,
			Balance:                  balance,
			ConfirmedCount:           counts.Confirmed,
			WaitlistCount:            counts.Waitlist,
			HasExistingActiveBooking: alreadyBooked,
			Now:                      s.clock.Now(),
		})
		if decision.Kind == domain.DecisionDeny {
			return bookingDenial(decision.Reason)
		}

		// Package coverage is checked separately from the balance: a member can
		// have credits that simply do not apply to this class.
		covered, err := s.wallet.CoveredClassTypes(ctx, memberID)
		if err != nil {
			return err
		}
		if covered != nil && !containsString(covered, session.ClassTypeID) {
			return httpx.Conflict(string(domain.DenyPackageNotCovered),
				"Your current package does not cover this class.")
		}

		now := s.clock.Now()
		booking := domain.Booking{
			ID:        s.ids.New(id.Booking),
			MemberID:  memberID,
			SessionID: sessionID,
			Source:    source,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if decision.Kind == domain.DecisionWaitlist {
			booking.Status = domain.BookingWaitlist
			position := decision.Position
			booking.WaitlistPosition = &position
			result.Decision = string(domain.BookingWaitlist)
		} else {
			booking.Status = domain.BookingConfirmed
			result.Decision = string(domain.BookingConfirmed)
		}

		created, err := s.repo.InsertBooking(ctx, booking)
		if err != nil {
			return err
		}
		result.Booking = created

		if created.Status == domain.BookingConfirmed {
			// The class fills as soon as the last place goes.
			if counts.Confirmed+1 >= session.Capacity && session.Status == domain.SessionPublished {
				session.Status = domain.SessionFull
				if _, err := s.repo.UpdateSession(ctx, session); err != nil {
					return err
				}
			}
			return s.events.Publish(ctx, outbox.TopicBookingConfirmed, map[string]any{
				"bookingId": created.ID,
				"memberId":  memberID,
				"sessionId": sessionID,
				"startsAt":  session.StartsAt,
			}, &created.ID)
		}
		return nil
	})
	return result, err
}

func bookingDenial(reason domain.BookingDenialReason) error {
	messages := map[domain.BookingDenialReason]string{
		domain.DenyMemberNotActive:     "This membership cannot book classes right now.",
		domain.DenySessionNotBookable:  "This class is not open for booking.",
		domain.DenyBookingNotOpenYet:   "Booking for this class has not opened yet.",
		domain.DenyBookingWindowClosed: "Booking for this class has closed.",
		domain.DenyAlreadyBooked:       "You already have a place in this class.",
		domain.DenyInsufficientCredits: "You do not have enough credits for this class.",
	}
	message, ok := messages[reason]
	if !ok {
		message = "This class cannot be booked."
	}
	return httpx.Conflict(string(reason), "%s", message)
}

// CancelResult reports what a cancellation cost and who took the freed place.
type CancelResult struct {
	Booking            domain.Booking `json:"booking"`
	Outcome            string         `json:"outcome"`
	PenaltyCredits     int            `json:"penaltyCredits"`
	PromotedMemberName *string        `json:"promotedMemberName"`
}

// Cancel releases a place. Cancelling after the deadline forfeits a credit
// under the studio's policy, and the freed place is offered to the waitlist.
func (s *Service) Cancel(ctx context.Context, bookingID string, actor Actor) (CancelResult, error) {
	var result CancelResult

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		booking, err := s.repo.Booking(ctx, bookingID)
		if err != nil {
			return err
		}
		session, err := s.repo.LockSession(ctx, booking.SessionID)
		if err != nil {
			return err
		}
		if _, err := domain.Transition(domain.BookingTransitions, booking.Status, domain.BookingCancelled); err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s booking cannot be cancelled.", booking.Status)
		}
		rules, err := s.catalog.RulesForBranch(ctx, session.BranchID)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		outcome := domain.EvaluateCancellation(session, rules, now)
		wasConfirmed := booking.Status == domain.BookingConfirmed

		booking.Status = domain.BookingCancelled
		booking.WaitlistPosition = nil
		booking.CancelledAt = &now
		booking.UpdatedAt = now
		cancelled, err := s.repo.UpdateBooking(ctx, booking)
		if err != nil {
			return err
		}
		result.Booking = cancelled
		result.Outcome = string(outcome.Kind)
		result.PenaltyCredits = outcome.PenaltyCredits

		// A late cancellation only costs something if the member actually held
		// a confirmed place; giving up a waitlist spot is always free.
		if wasConfirmed && outcome.Kind == domain.CancelLate && outcome.PenaltyCredits > 0 {
			if _, err := s.wallet.Forfeit(ctx, booking.MemberID, outcome.PenaltyCredits,
				"Late cancellation", booking.ID); err != nil {
				return err
			}
		}

		if !wasConfirmed {
			return nil
		}

		// Offer the freed place to the waitlist.
		waitlist, err := s.repo.BookingsForSession(ctx, session.ID)
		if err != nil {
			return err
		}
		promoted := domain.PickWaitlistPromotion(waitlist)
		if promoted == nil {
			// Nobody waiting: the class is no longer full.
			if session.Status == domain.SessionFull {
				session.Status = domain.SessionPublished
				if _, err := s.repo.UpdateSession(ctx, session); err != nil {
					return err
				}
			}
			return nil
		}

		if rules.WaitlistAutoPromote {
			promoted.Status = domain.BookingConfirmed
			promoted.WaitlistPosition = nil
		} else {
			// Manual policy: the place is offered, and the member has to take it.
			promoted.PromotionOfferedAt = &now
		}
		promoted.UpdatedAt = now
		if _, err := s.repo.UpdateBooking(ctx, *promoted); err != nil {
			return err
		}

		if member, err := s.members.Member(ctx, promoted.MemberID); err == nil {
			name := member.FullName
			result.PromotedMemberName = &name
		}
		return s.events.Publish(ctx, outbox.TopicWaitlistPromoted, map[string]any{
			"bookingId":   promoted.ID,
			"memberId":    promoted.MemberID,
			"sessionId":   session.ID,
			"startsAt":    session.StartsAt,
			"autoPromote": rules.WaitlistAutoPromote,
		}, &promoted.ID)
	})
	return result, err
}

// ConfirmOfferedSpot accepts a waitlist promotion under the manual policy.
func (s *Service) ConfirmOfferedSpot(ctx context.Context, bookingID, memberID string) (domain.Booking, error) {
	var confirmed domain.Booking

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		booking, err := s.repo.Booking(ctx, bookingID)
		if err != nil {
			return err
		}
		if booking.MemberID != memberID {
			return httpx.ErrForbidden.WithMessage("That booking is not yours.")
		}
		if booking.PromotionOfferedAt == nil {
			return httpx.Conflict("NO_OFFER", "No place has been offered to you for this class.")
		}
		session, err := s.repo.LockSession(ctx, booking.SessionID)
		if err != nil {
			return err
		}
		counts, err := s.repo.CountsForSession(ctx, session.ID)
		if err != nil {
			return err
		}
		// The offer can go stale: someone else may have taken the place.
		if counts.Confirmed >= session.Capacity {
			return httpx.Conflict("SLOT_TAKEN", "That place has been filled already.")
		}
		if _, err := domain.Transition(domain.BookingTransitions, booking.Status, domain.BookingConfirmed); err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s booking cannot be confirmed.", booking.Status)
		}

		now := s.clock.Now()
		booking.Status = domain.BookingConfirmed
		booking.WaitlistPosition = nil
		booking.PromotionOfferedAt = nil
		booking.UpdatedAt = now

		updated, err := s.repo.UpdateBooking(ctx, booking)
		if err != nil {
			return err
		}
		confirmed = updated

		if counts.Confirmed+1 >= session.Capacity && session.Status == domain.SessionPublished {
			session.Status = domain.SessionFull
			if _, err := s.repo.UpdateSession(ctx, session); err != nil {
				return err
			}
		}
		return nil
	})
	return confirmed, err
}

// ── Attendance ───────────────────────────────────────────────────────────────

// CheckInCandidate is what the gate asks for: the booking that justifies entry.
func (s *Service) CheckInCandidate(ctx context.Context, memberID, branchID string) (domain.Booking, domain.ClassSession, bool, error) {
	return s.repo.CheckInCandidate(ctx, memberID, branchID, s.clock.Now(), checkInEarlyWindow, checkInLateWindow)
}

// MarkCheckedIn records attendance. The gate calls this inside its own
// transaction, together with the credit deduction.
func (s *Service) MarkCheckedIn(ctx context.Context, bookingID string) (domain.Booking, error) {
	booking, err := s.repo.Booking(ctx, bookingID)
	if err != nil {
		return domain.Booking{}, err
	}
	if _, err := domain.Transition(domain.BookingTransitions, booking.Status, domain.BookingCheckedIn); err != nil {
		return domain.Booking{}, httpx.Conflict("INVALID_TRANSITION",
			"A %s booking cannot be checked in.", booking.Status)
	}
	now := s.clock.Now()
	booking.Status = domain.BookingCheckedIn
	booking.CheckedInAt = &now
	booking.UpdatedAt = now
	return s.repo.UpdateBooking(ctx, booking)
}

// CheckInManually is the front desk admitting someone at the counter. It
// charges the class the same way the gate would.
func (s *Service) CheckInManually(ctx context.Context, bookingID string, actor Actor) (domain.Booking, error) {
	var checkedIn domain.Booking

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		booking, err := s.repo.Booking(ctx, bookingID)
		if err != nil {
			return err
		}
		session, err := s.repo.Session(ctx, booking.SessionID)
		if err != nil {
			return err
		}
		balance, err := s.wallet.Balance(ctx, booking.MemberID)
		if err != nil {
			return err
		}
		if balance < session.CreditCost {
			return httpx.Conflict(string(domain.DenyInsufficientCredits),
				"This member does not have enough credits for the class.")
		}

		updated, err := s.MarkCheckedIn(ctx, bookingID)
		if err != nil {
			return err
		}
		checkedIn = updated

		if _, err := s.wallet.Forfeit(ctx, booking.MemberID, session.CreditCost,
			"Class check-in (front desk)", booking.ID); err != nil {
			return err
		}
		return s.auditor.Record(ctx, audit.Event{
			EntityType: "booking", EntityID: bookingID, Action: "manual_check_in",
			ActorID: actor.ID, ActorName: actor.Name,
		})
	})
	return checkedIn, err
}

// MarkNoShow records that a member did not turn up, applying the policy.
func (s *Service) MarkNoShow(ctx context.Context, bookingID string, actor Actor) (domain.Booking, error) {
	var marked domain.Booking

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		booking, err := s.repo.Booking(ctx, bookingID)
		if err != nil {
			return err
		}
		if _, err := domain.Transition(domain.BookingTransitions, booking.Status, domain.BookingNoShow); err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s booking cannot be marked no-show.", booking.Status)
		}
		session, err := s.repo.Session(ctx, booking.SessionID)
		if err != nil {
			return err
		}
		rules, err := s.catalog.RulesForBranch(ctx, session.BranchID)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		booking.Status = domain.BookingNoShow
		booking.UpdatedAt = now
		updated, err := s.repo.UpdateBooking(ctx, booking)
		if err != nil {
			return err
		}
		marked = updated

		if penalty := domain.NoShowPenalty(session, rules); penalty > 0 {
			if _, err := s.wallet.Forfeit(ctx, booking.MemberID, penalty, "No-show", booking.ID); err != nil {
				return err
			}
		}
		return s.auditor.Record(ctx, audit.Event{
			EntityType: "booking", EntityID: bookingID, Action: "no_show",
			ActorID: actor.ID, ActorName: actor.Name,
		})
	})
	return marked, err
}

// SweepNoShows marks confirmed bookings on finished classes as no-shows. It is
// the scheduled counterpart to the front desk doing it by hand.
func (s *Service) SweepNoShows(ctx context.Context, limit int) (int, error) {
	// A grace period after the class ends, so a late check-in still counts.
	cutoff := s.clock.Now().Add(-checkInLateWindow)
	due, err := s.repo.DueForNoShow(ctx, cutoff, limit)
	if err != nil {
		return 0, err
	}
	marked := 0
	for _, booking := range due {
		if _, err := s.MarkNoShow(ctx, booking.ID, Actor{ID: "system", Name: "Attendance sweep"}); err != nil {
			// One bad booking should not stop the sweep.
			continue
		}
		marked++
	}
	return marked, nil
}

// UpcomingForReminders is used by the notification job.
func (s *Service) UpcomingForReminders(ctx context.Context, within time.Duration) ([]domain.Booking, error) {
	now := s.clock.Now()
	return s.repo.UpcomingForReminders(ctx, now, now.Add(within))
}

func containsString(values []string, needle string) bool {
	for _, v := range values {
		if v == needle {
			return true
		}
	}
	return false
}

// SessionSummary is the denormalized session used by list views.
type SessionSummary struct {
	Session        domain.ClassSession `json:"session"`
	ClassTypeName  string              `json:"classTypeName"`
	CoachName      string              `json:"coachName"`
	BranchName     string              `json:"branchName"`
	ConfirmedCount int                 `json:"confirmedCount"`
	WaitlistCount  int                 `json:"waitlistCount"`
	SpotsLeft      int                 `json:"spotsLeft"`
	MyBooking      *MyBookingView      `json:"myBooking"`
}

// MyBookingView is the caller's own place in a class.
type MyBookingView struct {
	ID                 string               `json:"id"`
	Status             domain.BookingStatus `json:"status"`
	WaitlistPosition   *int                 `json:"waitlistPosition"`
	PromotionOfferedAt *time.Time           `json:"promotionOfferedAt"`
}

// Summaries denormalizes sessions for the schedule screens, resolving names
// and counts in bulk rather than per row.
func (s *Service) Summaries(ctx context.Context, sessions []domain.ClassSession, memberID string) ([]SessionSummary, error) {
	if len(sessions) == 0 {
		return []SessionSummary{}, nil
	}
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}

	counts, err := s.repo.CountsForSessions(ctx, ids)
	if err != nil {
		return nil, err
	}
	mine, err := s.repo.MemberBookingsForSessions(ctx, memberID, ids)
	if err != nil {
		return nil, err
	}
	classTypes, err := s.catalog.ClassTypes(ctx, false)
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

	classTypeNames := map[string]string{}
	for _, t := range classTypes {
		classTypeNames[t.ID] = t.Name
	}
	coachNames := map[string]string{}
	for _, c := range coaches {
		coachNames[c.ID] = c.Name
	}
	branchNames := map[string]string{}
	for _, b := range branches {
		branchNames[b.ID] = b.Name
	}

	out := make([]SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		count := counts[session.ID]
		spotsLeft := session.Capacity - count.Confirmed
		if spotsLeft < 0 {
			spotsLeft = 0
		}
		summary := SessionSummary{
			Session:        session,
			ClassTypeName:  classTypeNames[session.ClassTypeID],
			CoachName:      coachNames[session.CoachID],
			BranchName:     branchNames[session.BranchID],
			ConfirmedCount: count.Confirmed,
			WaitlistCount:  count.Waitlist,
			SpotsLeft:      spotsLeft,
		}
		if booking, ok := mine[session.ID]; ok {
			summary.MyBooking = &MyBookingView{
				ID:                 booking.ID,
				Status:             booking.Status,
				WaitlistPosition:   booking.WaitlistPosition,
				PromotionOfferedAt: booking.PromotionOfferedAt,
			}
		}
		out = append(out, summary)
	}
	return out, nil
}

// BookingSummary is a booking with the class details attached.
type BookingSummary struct {
	Booking       domain.Booking      `json:"booking"`
	Session       domain.ClassSession `json:"session"`
	ClassTypeName string              `json:"classTypeName"`
	CoachName     string              `json:"coachName"`
	BranchName    string              `json:"branchName"`
}

// BookingSummaries attaches class details to a list of bookings.
func (s *Service) BookingSummaries(ctx context.Context, bookings []domain.Booking) ([]BookingSummary, error) {
	if len(bookings) == 0 {
		return []BookingSummary{}, nil
	}
	sessionIDs := make([]string, 0, len(bookings))
	for _, b := range bookings {
		sessionIDs = append(sessionIDs, b.SessionID)
	}
	byID, err := s.repo.SessionsByIDs(ctx, sessionIDs)
	if err != nil {
		return nil, err
	}

	classTypes, err := s.catalog.ClassTypes(ctx, false)
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
	classTypeNames := map[string]string{}
	for _, t := range classTypes {
		classTypeNames[t.ID] = t.Name
	}
	coachNames := map[string]string{}
	for _, c := range coaches {
		coachNames[c.ID] = c.Name
	}
	branchNames := map[string]string{}
	for _, b := range branches {
		branchNames[b.ID] = b.Name
	}

	out := make([]BookingSummary, 0, len(bookings))
	for _, booking := range bookings {
		session, ok := byID[booking.SessionID]
		if !ok {
			// A booking whose session vanished cannot be rendered; skipping it
			// is better than showing a half-empty card.
			continue
		}
		out = append(out, BookingSummary{
			Booking:       booking,
			Session:       session,
			ClassTypeName: classTypeNames[session.ClassTypeID],
			CoachName:     coachNames[session.CoachID],
			BranchName:    branchNames[session.BranchID],
		})
	}
	return out, nil
}

// RosterEntry is one member's place in a class, for the coach's list.
type RosterEntry struct {
	Booking      domain.Booking      `json:"booking"`
	MemberName   string              `json:"memberName"`
	MemberStatus domain.MemberStatus `json:"memberStatus"`
}

// Roster lists who is coming to a class.
func (s *Service) Roster(ctx context.Context, sessionID string) ([]RosterEntry, error) {
	bookings, err := s.repo.BookingsForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	memberIDs := make([]string, 0, len(bookings))
	for _, b := range bookings {
		memberIDs = append(memberIDs, b.MemberID)
	}
	members, err := s.members.MembersByIDs(ctx, memberIDs)
	if err != nil {
		return nil, err
	}

	entries := make([]RosterEntry, 0, len(bookings))
	for _, b := range bookings {
		member, ok := members[b.MemberID]
		if !ok {
			continue
		}
		entries = append(entries, RosterEntry{
			Booking: b, MemberName: member.FullName, MemberStatus: member.Status,
		})
	}
	return entries, nil
}
