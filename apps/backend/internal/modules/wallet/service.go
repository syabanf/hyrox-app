// Package wallet owns money and credits: payments, the credit ledger, expiry
// lots, vouchers and refunds.
//
// The two halves are deliberately separate. A payment is an IDR transaction; a
// ledger entry is a credit movement. A settled payment produces both, and each
// can be audited on its own terms.
package wallet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/audit"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/config"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/outbox"
)

// Catalog is the port for the package and rule facts the wallet needs.
type Catalog interface {
	Package(ctx context.Context, id string) (domain.CreditPackage, error)
	Packages(ctx context.Context, activeOnly bool) ([]domain.CreditPackage, error)
	ClassTypes(ctx context.Context, activeOnly bool) ([]domain.ClassType, error)
	Rules(ctx context.Context) (domain.BusinessRules, error)
}

// Members is the port for the member facts the wallet needs.
type Members interface {
	Member(ctx context.Context, id string) (domain.Member, error)
}

// Service implements the wallet use cases.
type Service struct {
	db      *database.DB
	repo    *Repository
	catalog Catalog
	members Members
	gateway Gateway
	ids     id.Generator
	clock   clock.Clock
	auditor audit.Recorder
	events  outbox.Publisher
	cfg     config.Payments
}

func NewService(
	db *database.DB,
	repo *Repository,
	catalog Catalog,
	members Members,
	gateway Gateway,
	ids id.Generator,
	c clock.Clock,
	auditor audit.Recorder,
	events outbox.Publisher,
	cfg config.Payments,
) *Service {
	return &Service{
		db: db, repo: repo, catalog: catalog, members: members, gateway: gateway,
		ids: ids, clock: c, auditor: auditor, events: events, cfg: cfg,
	}
}

// Actor identifies who performed an administrative action.
type Actor struct {
	ID   string
	Name string
}

// ── Reading the wallet ───────────────────────────────────────────────────────

// Balance is the port other modules use before booking or opening a gate.
func (s *Service) Balance(ctx context.Context, memberID string) (int, error) {
	return s.repo.Balance(ctx, memberID)
}

func (s *Service) Balances(ctx context.Context, memberIDs []string) (map[string]int, error) {
	return s.repo.Balances(ctx, memberIDs)
}

func (s *Service) OutstandingCredits(ctx context.Context) (int, error) {
	return s.repo.OutstandingCredits(ctx)
}

// MyPackage is one purchased package as the member sees it in their wallet.
type MyPackage struct {
	LotID         string    `json:"lotId"`
	PackageID     string    `json:"packageId"`
	Name          string    `json:"name"`
	Credits       int       `json:"credits"`
	Remaining     int       `json:"remaining"`
	PurchasedAt   time.Time `json:"purchasedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	Active        bool      `json:"active"`
	CoverageIDs   []string  `json:"coverageIds"`
	CoverageNames []string  `json:"coverageNames"`
}

// WalletView is the member's wallet screen.
type WalletView struct {
	Balance             int                        `json:"balance"`
	ExpiringCredits     int                        `json:"expiringCredits"`
	LowBalance          bool                       `json:"lowBalance"`
	LowBalanceThreshold int                        `json:"lowBalanceThreshold"`
	Entries             []domain.CreditLedgerEntry `json:"entries"`
	Lots                []domain.TopUpLot          `json:"lots"`
	MyPackages          []MyPackage                `json:"myPackages"`
}

// Wallet returns the member's balance, history and packages, sweeping any
// credits that expired since the last look.
func (s *Service) Wallet(ctx context.Context, memberID string) (WalletView, error) {
	if _, err := s.SweepMember(ctx, memberID); err != nil {
		return WalletView{}, err
	}
	rules, err := s.catalog.Rules(ctx)
	if err != nil {
		return WalletView{}, err
	}
	entries, err := s.repo.Entries(ctx, memberID)
	if err != nil {
		return WalletView{}, err
	}
	lots, err := s.repo.Lots(ctx, memberID)
	if err != nil {
		return WalletView{}, err
	}
	packages, err := s.catalog.Packages(ctx, false)
	if err != nil {
		return WalletView{}, err
	}

	balance := domain.ComputeBalance(entries)
	now := s.clock.Now()
	byID := map[string]domain.CreditPackage{}
	for _, p := range packages {
		byID[p.ID] = p
	}

	myPackages := []MyPackage{}
	for _, remainder := range domain.ComputeLotRemainders(lots, entries) {
		lot := remainder.Lot
		if lot.PackageID == nil {
			continue
		}
		pkg, ok := byID[*lot.PackageID]
		if !ok {
			continue
		}
		myPackages = append(myPackages, MyPackage{
			LotID:       lot.ID,
			PackageID:   pkg.ID,
			Name:        pkg.Name,
			Credits:     lot.Credits,
			Remaining:   remainder.Remaining,
			PurchasedAt: lot.CreatedAt,
			ExpiresAt:   lot.ExpiresAt,
			Active:      remainder.Remaining > 0 && lot.ExpiresAt.After(now),
			CoverageIDs: pkg.ApplicableClassTypeIDs,
		})
	}

	return WalletView{
		Balance:             balance,
		ExpiringCredits:     domain.ComputeExpiringCredits(lots, entries, now, rules.ExpiryReminderDays),
		LowBalance:          balance <= rules.LowBalanceThreshold,
		LowBalanceThreshold: rules.LowBalanceThreshold,
		Entries:             entries,
		Lots:                lots,
		MyPackages:          myPackages,
	}, nil
}

func (s *Service) Entries(ctx context.Context, memberID string) ([]domain.CreditLedgerEntry, error) {
	return s.repo.Entries(ctx, memberID)
}

func (s *Service) Lots(ctx context.Context, memberID string) ([]domain.TopUpLot, error) {
	return s.repo.Lots(ctx, memberID)
}

// ExpiringCredits is what the member is about to lose, for reminders and the
// dashboard.
func (s *Service) ExpiringCredits(ctx context.Context, memberID string) (int, error) {
	rules, err := s.catalog.Rules(ctx)
	if err != nil {
		return 0, err
	}
	entries, err := s.repo.Entries(ctx, memberID)
	if err != nil {
		return 0, err
	}
	lots, err := s.repo.Lots(ctx, memberID)
	if err != nil {
		return 0, err
	}
	return domain.ComputeExpiringCredits(lots, entries, s.clock.Now(), rules.ExpiryReminderDays), nil
}

// CoveredClassTypes reports which class types the member's live credits may
// book: nil means unrestricted.
//
// Credits that came from a restricted package can only book the classes that
// package covers; bonus and adjustment credits carry no restriction at all, so
// holding any of them lifts the limit.
func (s *Service) CoveredClassTypes(ctx context.Context, memberID string) ([]string, error) {
	entries, err := s.repo.Entries(ctx, memberID)
	if err != nil {
		return nil, err
	}
	lots, err := s.repo.Lots(ctx, memberID)
	if err != nil {
		return nil, err
	}
	packages, err := s.catalog.Packages(ctx, false)
	if err != nil {
		return nil, err
	}
	byID := map[string]domain.CreditPackage{}
	for _, p := range packages {
		byID[p.ID] = p
	}

	now := s.clock.Now()
	covered := map[string]bool{}
	for _, remainder := range domain.ComputeLotRemainders(lots, entries) {
		if remainder.Remaining <= 0 || !remainder.Lot.ExpiresAt.After(now) {
			continue
		}
		if remainder.Lot.PackageID == nil {
			return nil, nil
		}
		pkg, ok := byID[*remainder.Lot.PackageID]
		if !ok || pkg.ApplicableClassTypeIDs == nil {
			return nil, nil
		}
		for _, classTypeID := range pkg.ApplicableClassTypeIDs {
			covered[classTypeID] = true
		}
	}
	if len(covered) == 0 {
		// No live lots at all: nothing to restrict, and the balance check will
		// refuse the booking anyway.
		return nil, nil
	}
	out := make([]string, 0, len(covered))
	for classTypeID := range covered {
		out = append(out, classTypeID)
	}
	return out, nil
}

// ── Top-up ───────────────────────────────────────────────────────────────────

// TopUpResult is the checkout response.
type TopUpResult struct {
	Payment     domain.Payment `json:"payment"`
	DiscountIDR int64          `json:"discountIdr"`
	Invoice     Invoice        `json:"invoice"`
}

// TopUp starts a credit purchase: it prices the package, applies any voucher,
// records a PENDING payment and asks the gateway for an invoice.
//
// No credits move here. They are created only when the payment settles, which
// is what keeps an abandoned checkout from inflating a balance.
func (s *Service) TopUp(ctx context.Context, memberID, packageID string, voucherCode *string, channel domain.PaymentChannel) (TopUpResult, error) {
	member, err := s.members.Member(ctx, memberID)
	if err != nil {
		return TopUpResult{}, err
	}
	if !member.IsActive() {
		return TopUpResult{}, httpx.Conflict("MEMBER_NOT_ACTIVE", "This membership cannot buy credits right now.")
	}
	pkg, err := s.catalog.Package(ctx, packageID)
	if err != nil {
		return TopUpResult{}, err
	}
	if pkg.Status != domain.PackageActive {
		return TopUpResult{}, httpx.Conflict("PACKAGE_NOT_AVAILABLE", "That package is no longer on sale.")
	}
	if pkg.PurchaseLimitPerMember != nil {
		count, err := s.repo.CountPurchases(ctx, memberID, pkg.ID)
		if err != nil {
			return TopUpResult{}, err
		}
		if count >= *pkg.PurchaseLimitPerMember {
			return TopUpResult{}, httpx.Conflict("PURCHASE_LIMIT_REACHED",
				"You have reached the purchase limit for this package.")
		}
	}

	discount := int64(0)
	var appliedCode *string
	if voucherCode != nil && strings.TrimSpace(*voucherCode) != "" {
		quote, err := s.QuoteVoucher(ctx, memberID, *voucherCode, pkg.ID)
		if err != nil {
			return TopUpResult{}, err
		}
		discount = quote.DiscountIDR
		code := quote.Voucher.Code
		appliedCode = &code
	}

	now := s.clock.Now()
	payment := domain.Payment{
		ID:          s.ids.New(id.Payment),
		MemberID:    memberID,
		PackageID:   pkg.ID,
		Credits:     pkg.Credits,
		AmountIDR:   pkg.PriceIDR,
		DiscountIDR: discount,
		TotalIDR:    pkg.PriceIDR - discount,
		VoucherCode: appliedCode,
		Channel:     channel,
		Status:      domain.PaymentPending,
		CreatedAt:   now,
	}

	invoice, err := s.gateway.CreateInvoice(ctx, InvoiceRequest{
		PaymentID:   payment.ID,
		MemberID:    member.ID,
		MemberName:  member.FullName,
		MemberEmail: member.Email,
		AmountIDR:   payment.TotalIDR,
		Channel:     channel,
		Description: pkg.Name,
		ExpiresAt:   now.Add(s.cfg.InvoiceTTL),
	})
	if err != nil {
		return TopUpResult{}, fmt.Errorf("wallet: creating invoice: %w", err)
	}

	created, err := s.repo.InsertPayment(ctx, payment, &invoice.ExternalID)
	if err != nil {
		return TopUpResult{}, err
	}
	return TopUpResult{Payment: created, DiscountIDR: discount, Invoice: invoice}, nil
}

// SettlementResult is what a settled payment produced.
type SettlementResult struct {
	Payment domain.Payment           `json:"payment"`
	Entry   domain.CreditLedgerEntry `json:"entry"`
	Lot     domain.TopUpLot          `json:"lot"`
}

// SettlePayment marks a payment paid and issues the credits.
//
// Everything happens in one transaction: the payment status, the TOP_UP entry,
// the expiry lot and any voucher redemption. A partial application here would
// mean money taken with no credits, or credits granted twice.
func (s *Service) SettlePayment(ctx context.Context, paymentID string) (SettlementResult, error) {
	var result SettlementResult

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		// The row lock is what makes a duplicate webhook harmless: the second
		// one waits, then sees the payment is already PAID.
		payment, err := s.repo.LockPayment(ctx, paymentID)
		if err != nil {
			return err
		}
		if payment.Status == domain.PaymentPaid {
			return httpx.Conflict("ALREADY_PAID", "That payment has already been settled.")
		}
		if _, err := domain.Transition(domain.PaymentTransitions, payment.Status, domain.PaymentPaid); err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s payment cannot be marked paid.", payment.Status)
		}

		pkg, err := s.catalog.Package(ctx, payment.PackageID)
		if err != nil {
			return err
		}
		now := s.clock.Now()

		updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, domain.PaymentPaid, &now, nil)
		if err != nil {
			return err
		}

		sourceType := domain.SourcePayment
		entry, err := s.repo.InsertEntry(ctx, domain.CreditLedgerEntry{
			ID:          s.ids.New(id.LedgerEntry),
			MemberID:    payment.MemberID,
			Type:        domain.LedgerTopUp,
			Amount:      payment.Credits,
			Description: fmt.Sprintf("Top-up: %s", pkg.Name),
			SourceType:  &sourceType,
			SourceID:    &payment.ID,
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}

		lot, err := s.repo.InsertLot(ctx, domain.TopUpLot{
			ID:            s.ids.New(id.Lot),
			MemberID:      payment.MemberID,
			LedgerEntryID: entry.ID,
			PackageID:     &pkg.ID,
			Credits:       payment.Credits,
			ExpiresAt:     now.AddDate(0, 0, pkg.ValidityDays),
			CreatedAt:     now,
		})
		if err != nil {
			return err
		}

		// The voucher is only consumed now, when the money actually arrived.
		if payment.VoucherCode != nil {
			voucher, err := s.repo.VoucherByCode(ctx, *payment.VoucherCode)
			if err == nil {
				if err := s.repo.InsertRedemption(ctx, domain.VoucherRedemption{
					ID:          s.ids.New(id.Redemption),
					VoucherID:   voucher.ID,
					MemberID:    payment.MemberID,
					PaymentID:   payment.ID,
					DiscountIDR: payment.DiscountIDR,
					CreatedAt:   now,
				}); err != nil {
					return err
				}
			}
		}

		if err := s.events.Publish(ctx, outbox.TopicPaymentPaid, map[string]any{
			"paymentId": payment.ID,
			"memberId":  payment.MemberID,
			"credits":   payment.Credits,
			"totalIdr":  payment.TotalIDR,
		}, &payment.ID); err != nil {
			return err
		}

		result = SettlementResult{Payment: updated, Entry: entry, Lot: lot}
		return nil
	})
	return result, err
}

// RefundResult is a refunded payment and the entry that took the credits back.
type RefundResult struct {
	Payment  domain.Payment            `json:"payment"`
	Reversal *domain.CreditLedgerEntry `json:"reversal"`
}

// RefundPayment returns the money and reverses the credits it bought.
//
// The credits are not deleted: the original TOP_UP stays and a REVERSAL
// cancels it, so the member's history still shows what happened.
func (s *Service) RefundPayment(ctx context.Context, paymentID, reason string, actor Actor) (RefundResult, error) {
	var result RefundResult

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		payment, err := s.repo.LockPayment(ctx, paymentID)
		if err != nil {
			return err
		}
		if _, err := domain.Transition(domain.PaymentTransitions, payment.Status, domain.PaymentRefunded); err != nil {
			return httpx.Conflict("INVALID_TRANSITION", "A %s payment cannot be refunded.", payment.Status)
		}

		now := s.clock.Now()
		updated, err := s.repo.UpdatePaymentStatus(ctx, payment.ID, domain.PaymentRefunded, nil, &now)
		if err != nil {
			return err
		}
		result.Payment = updated

		original, err := s.repo.EntryBySource(ctx, domain.SourcePayment, payment.ID, domain.LedgerTopUp)
		if err != nil {
			// A payment with no credit entry (an abandoned or manual case)
			// still refunds; there is simply nothing to reverse.
			if httpx.AsError(err).Status == 404 {
				return s.auditPayment(ctx, payment.ID, "refund", reason, actor)
			}
			return err
		}

		reversed, err := s.repo.HasReversal(ctx, original.ID)
		if err != nil {
			return err
		}
		draft, err := domain.BuildReversal(original, actor.ID, reason, reversed)
		if err != nil {
			return translateReversalError(err)
		}

		sourceType := domain.SourceAdmin
		entry, err := s.repo.InsertEntry(ctx, domain.CreditLedgerEntry{
			ID:              s.ids.New(id.LedgerEntry),
			MemberID:        draft.MemberID,
			Type:            domain.LedgerReversal,
			Amount:          draft.Amount,
			Description:     draft.Description,
			SourceType:      &sourceType,
			SourceID:        draft.SourceID,
			ReversesEntryID: &draft.ReversesEntryID,
			ActorID:         &draft.ActorID,
			Reason:          &draft.Reason,
			CreatedAt:       now,
		})
		if err != nil {
			return err
		}
		result.Reversal = &entry
		return s.auditPayment(ctx, payment.ID, "refund", reason, actor)
	})
	return result, err
}

func (s *Service) auditPayment(ctx context.Context, paymentID, action, reason string, actor Actor) error {
	return s.auditor.Record(ctx, audit.Event{
		EntityType: "payment", EntityID: paymentID, Action: action,
		Reason: &reason, ActorID: actor.ID, ActorName: actor.Name,
	})
}

func (s *Service) Payments(ctx context.Context, filter PaymentFilter) ([]domain.Payment, error) {
	return s.repo.Payments(ctx, filter)
}

func (s *Service) Payment(ctx context.Context, id string) (domain.Payment, error) {
	return s.repo.Payment(ctx, id)
}

func (s *Service) PackageSales(ctx context.Context) (map[string]PackageSale, error) {
	return s.repo.PackageSales(ctx)
}

func (s *Service) CountPaymentsForPackage(ctx context.Context, packageID string) (int, error) {
	return s.repo.CountPaymentsForPackage(ctx, packageID)
}

// ── Credit movements ─────────────────────────────────────────────────────────

// Deduct removes credits, and is how the gate charges for a visit. It runs on
// the caller's transaction so the deduction commits with the check-in.
func (s *Service) Deduct(ctx context.Context, memberID string, amount int, description string, sourceType domain.LedgerSourceType, sourceID string) (domain.CreditLedgerEntry, error) {
	if amount <= 0 {
		return domain.CreditLedgerEntry{}, httpx.Invalid("A deduction must be greater than zero.")
	}
	return s.repo.InsertEntry(ctx, domain.CreditLedgerEntry{
		ID:          s.ids.New(id.LedgerEntry),
		MemberID:    memberID,
		Type:        domain.LedgerVisitDeduction,
		Amount:      -amount,
		Description: description,
		SourceType:  &sourceType,
		SourceID:    &sourceID,
		CreatedAt:   s.clock.Now(),
	})
}

// Forfeit charges the penalty for a late cancellation or a no-show.
func (s *Service) Forfeit(ctx context.Context, memberID string, amount int, description string, sourceID string) (domain.CreditLedgerEntry, error) {
	if amount <= 0 {
		return domain.CreditLedgerEntry{}, httpx.Invalid("A forfeit must be greater than zero.")
	}
	sourceType := domain.SourceBooking
	return s.repo.InsertEntry(ctx, domain.CreditLedgerEntry{
		ID:          s.ids.New(id.LedgerEntry),
		MemberID:    memberID,
		Type:        domain.LedgerAdjustment,
		Amount:      -amount,
		Description: description,
		SourceType:  &sourceType,
		SourceID:    &sourceID,
		CreatedAt:   s.clock.Now(),
	})
}

// AdjustCredits is a manual correction by staff. A reason is required: an
// unexplained credit movement is exactly what an audit trail exists to prevent.
//
// Credits added this way get their own lot, so they expire like any others.
func (s *Service) AdjustCredits(ctx context.Context, memberID string, amount int, reason string, actor Actor) (domain.CreditLedgerEntry, error) {
	if amount == 0 {
		return domain.CreditLedgerEntry{}, httpx.Invalid("An adjustment cannot be zero.")
	}
	if len(strings.TrimSpace(reason)) < 3 {
		return domain.CreditLedgerEntry{}, httpx.Invalid("A reason is required for a credit adjustment.")
	}
	if _, err := s.members.Member(ctx, memberID); err != nil {
		return domain.CreditLedgerEntry{}, err
	}

	var entry domain.CreditLedgerEntry
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		sourceType := domain.SourceAdmin
		created, err := s.repo.InsertEntry(ctx, domain.CreditLedgerEntry{
			ID:          s.ids.New(id.LedgerEntry),
			MemberID:    memberID,
			Type:        domain.LedgerAdjustment,
			Amount:      amount,
			Description: fmt.Sprintf("Manual adjustment by %s", actor.Name),
			SourceType:  &sourceType,
			ActorID:     &actor.ID,
			Reason:      &reason,
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}
		entry = created

		if amount > 0 {
			rules, err := s.catalog.Rules(ctx)
			if err != nil {
				return err
			}
			if _, err := s.repo.InsertLot(ctx, domain.TopUpLot{
				ID:            s.ids.New(id.Lot),
				MemberID:      memberID,
				LedgerEntryID: created.ID,
				Credits:       amount,
				ExpiresAt:     now.AddDate(0, 0, rules.DefaultCreditExpiryDays),
				CreatedAt:     now,
			}); err != nil {
				return err
			}
		}

		return s.auditor.Record(ctx, audit.Event{
			EntityType: "member", EntityID: memberID, Action: "adjust_credits",
			NewValue: audit.Str(fmt.Sprintf("%+d", amount)), Reason: &reason,
			ActorID: actor.ID, ActorName: actor.Name,
		})
	})
	return entry, err
}

// ReverseEntry writes the compensating entry for a mistake.
func (s *Service) ReverseEntry(ctx context.Context, entryID, reason string, actor Actor) (domain.CreditLedgerEntry, error) {
	if len(strings.TrimSpace(reason)) < 3 {
		return domain.CreditLedgerEntry{}, httpx.Invalid("A reason is required to reverse an entry.")
	}

	var reversal domain.CreditLedgerEntry
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		original, err := s.repo.Entry(ctx, entryID)
		if err != nil {
			return err
		}
		reversed, err := s.repo.HasReversal(ctx, entryID)
		if err != nil {
			return err
		}
		draft, err := domain.BuildReversal(original, actor.ID, reason, reversed)
		if err != nil {
			return translateReversalError(err)
		}

		sourceType := domain.SourceAdmin
		created, err := s.repo.InsertEntry(ctx, domain.CreditLedgerEntry{
			ID:              s.ids.New(id.LedgerEntry),
			MemberID:        draft.MemberID,
			Type:            domain.LedgerReversal,
			Amount:          draft.Amount,
			Description:     draft.Description,
			SourceType:      &sourceType,
			SourceID:        draft.SourceID,
			ReversesEntryID: &draft.ReversesEntryID,
			ActorID:         &draft.ActorID,
			Reason:          &draft.Reason,
			CreatedAt:       s.clock.Now(),
		})
		if err != nil {
			return err
		}
		reversal = created

		return s.auditor.Record(ctx, audit.Event{
			EntityType: "ledger_entry", EntityID: entryID, Action: "reverse",
			PreviousValue: audit.Str(fmt.Sprintf("%+d", original.Amount)),
			NewValue:      audit.Str(fmt.Sprintf("%+d", created.Amount)),
			Reason:        &reason, ActorID: actor.ID, ActorName: actor.Name,
		})
	})
	return reversal, err
}

func translateReversalError(err error) error {
	switch err {
	case domain.ErrCannotReverseReversal:
		return httpx.Conflict("CANNOT_REVERSE_REVERSAL", "A reversal cannot itself be reversed.")
	case domain.ErrAlreadyReversed:
		return httpx.Conflict("ALREADY_REVERSED", "That entry has already been reversed.")
	default:
		return err
	}
}

// ── Expiry ───────────────────────────────────────────────────────────────────

// SweepResult reports what an expiry sweep wrote.
type SweepResult struct {
	AffectedMembers int `json:"affectedMembers"`
	Entries         int `json:"entries"`
}

// SweepMember expires the member's lapsed credits. It is safe to call on every
// wallet read: a second pass finds nothing left to expire.
func (s *Service) SweepMember(ctx context.Context, memberID string) (int, error) {
	entries, err := s.repo.Entries(ctx, memberID)
	if err != nil {
		return 0, err
	}
	lots, err := s.repo.Lots(ctx, memberID)
	if err != nil {
		return 0, err
	}
	drafts := domain.DeriveExpirationEntries(lots, entries, s.clock.Now())
	if len(drafts) == 0 {
		return 0, nil
	}

	written := 0
	err = s.db.InTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		sourceType := domain.SourceSystem
		for _, draft := range drafts {
			lotID := draft.LotID
			if _, err := s.repo.InsertEntry(ctx, domain.CreditLedgerEntry{
				ID:          s.ids.New(id.LedgerEntry),
				MemberID:    draft.MemberID,
				Type:        domain.LedgerExpiration,
				Amount:      draft.Amount,
				Description: draft.Description,
				SourceType:  &sourceType,
				SourceID:    &lotID,
				CreatedAt:   now,
			}); err != nil {
				return err
			}
			written++
		}
		return nil
	})
	return written, err
}

// SweepAll expires credits for every member holding a lapsed lot. This is the
// scheduled job; the per-member sweep covers members who visit in between.
func (s *Service) SweepAll(ctx context.Context) (SweepResult, error) {
	members, err := s.repo.MembersWithExpiredLots(ctx, s.clock.Now())
	if err != nil {
		return SweepResult{}, err
	}
	result := SweepResult{}
	for _, memberID := range members {
		written, err := s.SweepMember(ctx, memberID)
		if err != nil {
			return result, err
		}
		if written > 0 {
			result.AffectedMembers++
			result.Entries += written
		}
	}
	return result, nil
}

// ── Vouchers ─────────────────────────────────────────────────────────────────

// VoucherQuote is a validated discount.
type VoucherQuote struct {
	Voucher     domain.Voucher `json:"voucher"`
	DiscountIDR int64          `json:"discountIdr"`
}

// QuoteVoucher validates a code against a package for one member.
func (s *Service) QuoteVoucher(ctx context.Context, memberID, code, packageID string) (VoucherQuote, error) {
	voucher, err := s.repo.VoucherByCode(ctx, code)
	if err != nil {
		return VoucherQuote{}, err
	}
	pkg, err := s.catalog.Package(ctx, packageID)
	if err != nil {
		return VoucherQuote{}, err
	}
	member, err := s.members.Member(ctx, memberID)
	if err != nil {
		return VoucherQuote{}, err
	}
	total, byMember, err := s.repo.RedemptionCounts(ctx, voucher.ID, memberID)
	if err != nil {
		return VoucherQuote{}, err
	}

	// "New" means the member has never completed a purchase, which is what a
	// welcome code is actually for.
	purchases, err := s.repo.CountMemberPurchases(ctx, memberID)
	if err != nil {
		return VoucherQuote{}, err
	}
	memberIsNew := purchases == 0 && member.CreatedAt.After(s.clock.Now().AddDate(0, -1, 0))

	discount, rejection := domain.ValidateVoucher(domain.VoucherCheck{
		Voucher:               voucher,
		Package:               pkg,
		MemberIsNew:           memberIsNew,
		MemberRedemptionCount: byMember,
		TotalRedemptionCount:  total,
		Now:                   s.clock.Now(),
	})
	if rejection != "" {
		return VoucherQuote{}, httpx.Conflict("VOUCHER_"+string(rejection), "%s", voucherMessage(rejection))
	}
	return VoucherQuote{Voucher: voucher, DiscountIDR: discount}, nil
}

func voucherMessage(reason domain.VoucherRejection) string {
	switch reason {
	case domain.VoucherNotActive:
		return "That code is not active."
	case domain.VoucherNotStarted:
		return "That code is not valid yet."
	case domain.VoucherEnded:
		return "That code has expired."
	case domain.VoucherUsageLimitReached:
		return "That code has been fully redeemed."
	case domain.VoucherPerMemberLimitReached:
		return "You have already used that code."
	case domain.VoucherPackageNotEligible:
		return "That code does not apply to this package."
	case domain.VoucherSegmentNotEligible:
		return "That code is for new members only."
	default:
		return "That code cannot be used."
	}
}

func (s *Service) Vouchers(ctx context.Context) ([]domain.Voucher, error) {
	return s.repo.Vouchers(ctx)
}

func (s *Service) LiveVouchers(ctx context.Context) ([]domain.Voucher, error) {
	return s.repo.LiveVouchers(ctx, s.clock.Now())
}

func (s *Service) VoucherByCode(ctx context.Context, code string) (domain.Voucher, error) {
	return s.repo.VoucherByCode(ctx, code)
}

func (s *Service) RedemptionTotals(ctx context.Context) (map[string]int, error) {
	return s.repo.RedemptionTotals(ctx)
}

// VoucherInput creates or edits a discount code.
type VoucherInput struct {
	Code                 string
	Type                 domain.VoucherType
	Value                int64
	StartsAt             time.Time
	EndsAt               time.Time
	UsageLimit           *int
	PerMemberLimit       *int
	EligibleSegment      domain.VoucherSegment
	ApplicablePackageIDs []string
}

// CreateVoucher creates a code in DRAFT: publishing is a separate, deliberate
// step, so a half-configured code is never live.
func (s *Service) CreateVoucher(ctx context.Context, in VoucherInput, actor Actor) (domain.Voucher, error) {
	voucher := domain.Voucher{
		ID:                   s.ids.New(id.Voucher),
		Code:                 strings.ToUpper(strings.TrimSpace(in.Code)),
		Type:                 in.Type,
		Value:                in.Value,
		StartsAt:             in.StartsAt,
		EndsAt:               in.EndsAt,
		UsageLimit:           in.UsageLimit,
		PerMemberLimit:       in.PerMemberLimit,
		EligibleSegment:      in.EligibleSegment,
		ApplicablePackageIDs: in.ApplicablePackageIDs,
		Status:               domain.VoucherDraft,
		CreatedAt:            s.clock.Now(),
	}
	if voucher.EligibleSegment == "" {
		voucher.EligibleSegment = domain.SegmentAll
	}
	created, err := s.repo.InsertVoucher(ctx, voucher)
	if err != nil {
		return domain.Voucher{}, err
	}
	return created, s.auditor.Record(ctx, audit.Event{
		EntityType: "voucher", EntityID: created.ID, Action: "create",
		NewValue: audit.Str(created.Code), ActorID: actor.ID, ActorName: actor.Name,
	})
}

func (s *Service) UpdateVoucher(ctx context.Context, voucherID string, in VoucherInput, setPackages bool, actor Actor) (domain.Voucher, error) {
	voucher, err := s.repo.Voucher(ctx, voucherID)
	if err != nil {
		return domain.Voucher{}, err
	}
	if in.Code != "" {
		voucher.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	}
	if in.Type != "" {
		voucher.Type = in.Type
	}
	if in.Value > 0 {
		voucher.Value = in.Value
	}
	if !in.StartsAt.IsZero() {
		voucher.StartsAt = in.StartsAt
	}
	if !in.EndsAt.IsZero() {
		voucher.EndsAt = in.EndsAt
	}
	if in.EligibleSegment != "" {
		voucher.EligibleSegment = in.EligibleSegment
	}
	voucher.UsageLimit = in.UsageLimit
	voucher.PerMemberLimit = in.PerMemberLimit
	if setPackages {
		voucher.ApplicablePackageIDs = in.ApplicablePackageIDs
	}

	updated, err := s.repo.UpdateVoucher(ctx, voucher)
	if err != nil {
		return domain.Voucher{}, err
	}
	return updated, s.auditor.Record(ctx, audit.Event{
		EntityType: "voucher", EntityID: voucherID, Action: "update",
		NewValue: audit.Str(updated.Code), ActorID: actor.ID, ActorName: actor.Name,
	})
}

// SetVoucherStatus moves a code through its lifecycle, refusing illegal moves
// such as reviving an expired code.
func (s *Service) SetVoucherStatus(ctx context.Context, voucherID string, status domain.VoucherStatus, actor Actor) (domain.Voucher, error) {
	voucher, err := s.repo.Voucher(ctx, voucherID)
	if err != nil {
		return domain.Voucher{}, err
	}
	previous := voucher.Status
	next, err := domain.Transition(domain.VoucherTransitions, voucher.Status, status)
	if err != nil {
		return domain.Voucher{}, httpx.Conflict("INVALID_TRANSITION",
			"A %s voucher cannot become %s.", previous, status)
	}
	voucher.Status = next

	updated, err := s.repo.UpdateVoucher(ctx, voucher)
	if err != nil {
		return domain.Voucher{}, err
	}
	return updated, s.auditor.Record(ctx, audit.Event{
		EntityType: "voucher", EntityID: voucherID, Action: "status_change",
		PreviousValue: audit.Str(string(previous)), NewValue: audit.Str(string(next)),
		ActorID: actor.ID, ActorName: actor.Name,
	})
}

func (s *Service) DeleteVoucher(ctx context.Context, voucherID string, actor Actor) error {
	if err := s.repo.DeleteVoucher(ctx, voucherID); err != nil {
		return err
	}
	return s.auditor.Record(ctx, audit.Event{
		EntityType: "voucher", EntityID: voucherID, Action: "delete",
		ActorID: actor.ID, ActorName: actor.Name,
	})
}
