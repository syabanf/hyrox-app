// Package access owns the door: short-lived QR credentials, the gate
// validation pipeline, and the log of every scan.
//
// The pipeline itself is pure domain code. This module's job is to gather the
// facts it needs, then apply the effects it returns inside a single
// transaction, so the gate never opens without its deduction.
package access

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/audit"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
	"github.com/syabanf/nuhabit-backend/internal/platform/outbox"
)

// Catalog is the port for gates and the rules that time the door.
type Catalog interface {
	Gate(ctx context.Context, id string) (domain.Gate, error)
	Gates(ctx context.Context) ([]domain.Gate, error)
	Branches(ctx context.Context) ([]domain.Branch, error)
	RulesForBranch(ctx context.Context, branchID string) (domain.BusinessRules, error)
}

// Members is the port for membership status.
type Members interface {
	Member(ctx context.Context, id string) (domain.Member, error)
	MembersByIDs(ctx context.Context, ids []string) (map[string]domain.Member, error)
}

// Scheduling is the port for the booking that justifies an entry.
type Scheduling interface {
	CheckInCandidate(ctx context.Context, memberID, branchID string) (domain.Booking, domain.ClassSession, bool, error)
	MarkCheckedIn(ctx context.Context, bookingID string) (domain.Booking, error)
}

// Wallet is the port for credits.
type Wallet interface {
	Balance(ctx context.Context, memberID string) (int, error)
	Deduct(ctx context.Context, memberID string, amount int, description string, sourceType domain.LedgerSourceType, sourceID string) (domain.CreditLedgerEntry, error)
}

// Service implements the access use cases.
type Service struct {
	db         *database.DB
	repo       *Repository
	catalog    Catalog
	members    Members
	scheduling Scheduling
	wallet     Wallet
	ids        id.Generator
	clock      clock.Clock
	auditor    audit.Recorder
	events     outbox.Publisher
}

func NewService(db *database.DB, repo *Repository, catalog Catalog, members Members,
	scheduling Scheduling, wallet Wallet, ids id.Generator, c clock.Clock,
	auditor audit.Recorder, events outbox.Publisher) *Service {
	return &Service{db: db, repo: repo, catalog: catalog, members: members, scheduling: scheduling,
		wallet: wallet, ids: ids, clock: c, auditor: auditor, events: events}
}

// Actor identifies who performed an administrative action.
type Actor struct {
	ID   string
	Name string
}

// QRView is the credential the member app renders.
type QRView struct {
	Token      string    `json:"token"`
	IssuedAt   time.Time `json:"issuedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	TTLSeconds int       `json:"ttlSeconds"`
}

// IssueQR mints a short-lived credential for a member.
//
// The token is random, not derived from the member id: a code that leaks is
// useless within its TTL and cannot be reverse-engineered into an identity.
func (s *Service) IssueQR(ctx context.Context, memberID string) (QRView, error) {
	member, err := s.members.Member(ctx, memberID)
	if err != nil {
		return QRView{}, err
	}
	if !member.IsActive() {
		return QRView{}, httpx.ErrForbidden.WithMessage("This membership cannot check in right now.")
	}

	branchID := ""
	if member.PreferredBranchID != nil {
		branchID = *member.PreferredBranchID
	}
	rules, err := s.catalog.RulesForBranch(ctx, branchID)
	if err != nil {
		return QRView{}, err
	}

	nonce, err := randomNonce()
	if err != nil {
		return QRView{}, err
	}
	token := domain.IssueQrToken(memberID, nonce, s.clock.Now(), rules.QRTTLSeconds)
	if err := s.repo.InsertToken(ctx, token); err != nil {
		return QRView{}, err
	}
	return QRView{
		Token:      token.Token,
		IssuedAt:   token.IssuedAt,
		ExpiresAt:  token.ExpiresAt,
		TTLSeconds: rules.QRTTLSeconds,
	}, nil
}

func randomNonce() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("access: generating nonce: %w", err)
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}

// ScanResult is what the gate is told.
type ScanResult struct {
	Decision         string                   `json:"decision"`
	Reason           *domain.GateDenialReason `json:"reason"`
	EntryKind        *domain.GateEntryKind    `json:"entryKind"`
	MemberName       *string                  `json:"memberName"`
	RemainingCredits *int                     `json:"remainingCredits"`
	GateName         string                   `json:"gateName"`
	AccessLog        domain.AccessLog         `json:"accessLog"`
}

// ScanRequest is a presented credential, or a staff simulation.
type ScanRequest struct {
	GateID string
	// QRToken is the scanned credential.
	QRToken string
	// MemberID is set instead when staff simulate a scan from the monitor.
	MemberID string
	Offline  bool
}

// Scan runs the gate pipeline and applies its effects atomically.
//
// A refused scan is still recorded: the access log is the record of what
// happened at the door, not only of who got in.
func (s *Service) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	gate, err := s.catalog.Gate(ctx, req.GateID)
	if err != nil {
		return ScanResult{}, err
	}
	rules, err := s.catalog.RulesForBranch(ctx, gate.BranchID)
	if err != nil {
		return ScanResult{}, err
	}

	var result ScanResult
	err = s.db.InTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()

		// Resolve the credential. A simulated scan skips the token entirely,
		// which is why the pipeline treats "no token problem" as its input
		// rather than looking one up itself.
		var token domain.QrToken
		problem := domain.QrOK
		memberID := req.MemberID

		if req.QRToken != "" {
			stored, found, err := s.repo.Token(ctx, req.QRToken)
			if err != nil {
				return err
			}
			if !found {
				problem = domain.QrNotFound
			} else {
				token = stored
				memberID = stored.MemberID
				problem = domain.CheckQrToken(&stored, now)
			}
		}

		var member *domain.Member
		balance := 0
		var lastEntry *time.Time
		var candidate *domain.CandidateBooking
		var candidateSession domain.ClassSession

		if memberID != "" {
			loaded, err := s.members.Member(ctx, memberID)
			if err == nil {
				member = &loaded
				if balance, err = s.wallet.Balance(ctx, memberID); err != nil {
					return err
				}
				if lastEntry, err = s.repo.LastAllowedEntry(ctx, memberID, gate.BranchID); err != nil {
					return err
				}
				booking, session, found, err := s.scheduling.CheckInCandidate(ctx, memberID, gate.BranchID)
				if err != nil {
					return err
				}
				if found {
					candidate = &domain.CandidateBooking{ID: booking.ID, CreditCost: session.CreditCost}
					candidateSession = session
				}
			}
		}

		evaluation := domain.EvaluateGateScan(domain.GateScanInput{
			TokenProblem:       problem,
			Token:              token,
			Member:             member,
			Balance:            balance,
			LastAllowedEntryAt: lastEntry,
			CandidateBooking:   candidate,
			Rules:              rules,
			Now:                now,
		})

		creditDelta := 0
		var bookingID *string

		for _, effect := range evaluation.Effects {
			switch effect.Kind {
			case domain.EffectConsumeToken:
				consumed, err := s.repo.ConsumeToken(ctx, effect.Token, gate.ID, now)
				if err != nil {
					return err
				}
				if !consumed {
					// Someone else burned this token between the read and the
					// write. Treat it as the replay it is.
					reason := domain.GateTokenConsumed
					evaluation = domain.GateScanEvaluation{
						Decision: domain.AccessDenied, Reason: &reason,
					}
					creditDelta = 0
					bookingID = nil
				}
			case domain.EffectDeductCredits:
				if _, err := s.wallet.Deduct(ctx, member.ID, effect.Amount, effect.Description,
					domain.SourceAccess, gate.ID); err != nil {
					return err
				}
				creditDelta = -effect.Amount
			case domain.EffectCheckInBooking:
				if _, err := s.scheduling.MarkCheckedIn(ctx, effect.BookingID); err != nil {
					return err
				}
				id := effect.BookingID
				bookingID = &id
			}
		}

		logMemberID := (*string)(nil)
		if memberID != "" {
			value := memberID
			logMemberID = &value
		}
		mode := domain.ModeOnline
		if req.Offline {
			mode = domain.ModeOffline
		}

		log, err := s.repo.InsertLog(ctx, domain.AccessLog{
			ID:          s.ids.New(id.AccessLog),
			MemberID:    logMemberID,
			GateID:      gate.ID,
			BranchID:    gate.BranchID,
			Result:      evaluation.Decision,
			ReasonCode:  evaluation.Reason,
			CreditDelta: creditDelta,
			Mode:        mode,
			BookingID:   bookingID,
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}

		result = ScanResult{
			Decision:  string(evaluation.Decision),
			Reason:    evaluation.Reason,
			EntryKind: evaluation.EntryKind,
			GateName:  gate.Name,
			AccessLog: log,
		}
		if member != nil {
			name := member.FullName
			result.MemberName = &name
			remaining := balance + creditDelta
			result.RemainingCredits = &remaining
		}

		if evaluation.Decision == domain.AccessAllowed && bookingID != nil {
			return s.events.Publish(ctx, outbox.TopicVisitLogged, map[string]any{
				"memberId":  memberID,
				"gateId":    gate.ID,
				"branchId":  gate.BranchID,
				"sessionId": candidateSession.ID,
			}, &log.ID)
		}
		return nil
	})
	return result, err
}

// LogView is one scan with the names the monitor shows.
type LogView struct {
	Log        domain.AccessLog `json:"log"`
	MemberName *string          `json:"memberName"`
	GateName   string           `json:"gateName"`
	BranchName string           `json:"branchName"`
}

// Logs returns the access log with names resolved.
func (s *Service) Logs(ctx context.Context, filter LogFilter) ([]LogView, error) {
	logs, err := s.repo.Logs(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.decorate(ctx, logs)
}

func (s *Service) decorate(ctx context.Context, logs []domain.AccessLog) ([]LogView, error) {
	gates, err := s.catalog.Gates(ctx)
	if err != nil {
		return nil, err
	}
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return nil, err
	}
	gateNames := map[string]string{}
	for _, g := range gates {
		gateNames[g.ID] = g.Name
	}
	branchNames := map[string]string{}
	for _, b := range branches {
		branchNames[b.ID] = b.Name
	}

	memberIDs := make([]string, 0, len(logs))
	for _, l := range logs {
		if l.MemberID != nil {
			memberIDs = append(memberIDs, *l.MemberID)
		}
	}
	members, err := s.members.MembersByIDs(ctx, memberIDs)
	if err != nil {
		return nil, err
	}

	views := make([]LogView, 0, len(logs))
	for _, l := range logs {
		view := LogView{Log: l, GateName: gateNames[l.GateID], BranchName: branchNames[l.BranchID]}
		if l.MemberID != nil {
			if member, ok := members[*l.MemberID]; ok {
				name := member.FullName
				view.MemberName = &name
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// MemberVisits is the member's own entry history.
func (s *Service) MemberVisits(ctx context.Context, memberID string, limit int) ([]LogView, error) {
	logs, err := s.repo.Logs(ctx, LogFilter{MemberID: memberID, Limit: limit})
	if err != nil {
		return nil, err
	}
	return s.decorate(ctx, logs)
}

func (s *Service) VisitCounts(ctx context.Context, memberIDs []string) (map[string]VisitSummary, error) {
	return s.repo.VisitCounts(ctx, memberIDs)
}

func (s *Service) CountToday(ctx context.Context, since time.Time) (int, error) {
	return s.repo.CountToday(ctx, since)
}

func (s *Service) DailyVisits(ctx context.Context, since time.Time) (map[string]int, int, int, error) {
	return s.repo.DailyVisits(ctx, since)
}

// ResolveConflict settles an offline scan that could not be validated live.
//
// Approving charges the class now, which is the whole point of reconciliation:
// the member got in on trust, and the ledger catches up.
func (s *Service) ResolveConflict(ctx context.Context, logID, action, reason string, actor Actor) (LogView, error) {
	var view LogView

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		log, err := s.repo.Log(ctx, logID)
		if err != nil {
			return err
		}
		if log.Result != domain.AccessConflict {
			return httpx.Conflict("NOT_A_CONFLICT", "Only conflicting scans can be resolved.")
		}
		if log.MemberID == nil {
			return httpx.Conflict("NO_MEMBER", "That scan was never matched to a member.")
		}

		var updated domain.AccessLog
		switch strings.ToUpper(action) {
		case "APPROVE":
			booking, session, found, err := s.scheduling.CheckInCandidate(ctx, *log.MemberID, log.BranchID)
			if err != nil {
				return err
			}
			if !found {
				return httpx.Conflict("NO_BOOKING", "That member had no class booked at the time.")
			}
			if _, err := s.wallet.Deduct(ctx, *log.MemberID, session.CreditCost,
				"Offline check-in reconciled", domain.SourceAccess, log.ID); err != nil {
				return err
			}
			if _, err := s.scheduling.MarkCheckedIn(ctx, booking.ID); err != nil {
				return err
			}
			updated, err = s.repo.ResolveLog(ctx, logID, domain.AccessSynced, -session.CreditCost, &booking.ID)
			if err != nil {
				return err
			}
		case "REJECT":
			updated, err = s.repo.ResolveLog(ctx, logID, domain.AccessDenied, 0, nil)
			if err != nil {
				return err
			}
		default:
			return httpx.Invalid("Action must be APPROVE or REJECT.")
		}

		views, err := s.decorate(ctx, []domain.AccessLog{updated})
		if err != nil {
			return err
		}
		view = views[0]

		return s.auditor.Record(ctx, audit.Event{
			EntityType: "access_log", EntityID: logID, Action: "resolve_conflict",
			NewValue: audit.Str(strings.ToUpper(action)), Reason: &reason,
			ActorID: actor.ID, ActorName: actor.Name,
		})
	})
	return view, err
}

// PurgeExpiredTokens is called by the maintenance loop.
func (s *Service) PurgeExpiredTokens(ctx context.Context) (int64, error) {
	return s.repo.PurgeExpiredTokens(ctx, s.clock.Now().Add(-24*time.Hour))
}
