package domain

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// LedgerEntryType classifies a credit movement.
type LedgerEntryType string

const (
	LedgerTopUp          LedgerEntryType = "TOP_UP"
	LedgerVisitDeduction LedgerEntryType = "VISIT_DEDUCTION"
	LedgerRefund         LedgerEntryType = "REFUND"
	LedgerBonus          LedgerEntryType = "BONUS"
	LedgerPromo          LedgerEntryType = "PROMO"
	LedgerExpiration     LedgerEntryType = "EXPIRATION"
	LedgerAdjustment     LedgerEntryType = "ADJUSTMENT"
	LedgerReversal       LedgerEntryType = "REVERSAL"
)

// LedgerSourceType names the subsystem that caused an entry, so any credit
// movement can be traced back to the payment, booking or scan behind it.
type LedgerSourceType string

const (
	SourcePayment LedgerSourceType = "PAYMENT"
	SourceBooking LedgerSourceType = "BOOKING"
	SourceAccess  LedgerSourceType = "ACCESS"
	SourceAdmin   LedgerSourceType = "ADMIN"
	SourceSystem  LedgerSourceType = "SYSTEM"
)

// CreditLedgerEntry is an immutable credit movement. Rows are never updated or
// deleted: a mistake is corrected by writing a REVERSAL that points at the
// original, so the history stays auditable.
type CreditLedgerEntry struct {
	ID       string          `json:"id"`
	MemberID string          `json:"memberId"`
	Type     LedgerEntryType `json:"type"`
	// Amount is signed: positive adds credits, negative consumes them.
	Amount          int               `json:"amount"`
	Description     string            `json:"description"`
	SourceType      *LedgerSourceType `json:"sourceType"`
	SourceID        *string           `json:"sourceId"`
	ReversesEntryID *string           `json:"reversesEntryId"`
	ActorID         *string           `json:"actorId"`
	Reason          *string           `json:"reason"`
	CreatedAt       time.Time         `json:"createdAt"`
}

// TopUpLot tracks one batch of purchased credits and when it expires. Lots
// exist so expiry can be FIFO: the credits that expire soonest are spent first.
type TopUpLot struct {
	ID            string `json:"id"`
	MemberID      string `json:"memberId"`
	LedgerEntryID string `json:"ledgerEntryId"`
	// PackageID is nil for credits that were not purchased (bonus, adjustment).
	PackageID *string   `json:"packageId"`
	Credits   int       `json:"credits"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ComputeBalance is THE wallet rule: a balance is the sum of the ledger, never
// a stored column that could drift out of sync with its history.
func ComputeBalance(entries []CreditLedgerEntry) int {
	total := 0
	for _, e := range entries {
		total += e.Amount
	}
	return total
}

// LotRemainder is how much of a lot is still unspent.
type LotRemainder struct {
	Lot       TopUpLot
	Remaining int
}

// ComputeLotRemainders allocates all consumption across lots in expiry order.
//
// Consumption is pooled rather than matched entry-by-entry: every negative
// entry except EXPIRATION spends from the pool, and credits handed back
// (REFUND, or a REVERSAL that adds) reduce it again. EXPIRATION is excluded
// because those entries are pinned to a specific lot through SourceID.
func ComputeLotRemainders(lots []TopUpLot, entries []CreditLedgerEntry) []LotRemainder {
	sorted := make([]TopUpLot, len(lots))
	copy(sorted, lots)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ExpiresAt.Before(sorted[j].ExpiresAt)
	})

	consumed := 0
	expiredByLot := map[string]int{}
	for _, e := range entries {
		if e.Type == LedgerExpiration {
			if e.SourceID != nil {
				expiredByLot[*e.SourceID] += -e.Amount
			}
			continue
		}
		switch {
		case e.Amount < 0:
			consumed += -e.Amount
		case (e.Type == LedgerRefund || e.Type == LedgerReversal) && e.Amount > 0:
			consumed -= e.Amount
		}
	}
	if consumed < 0 {
		consumed = 0
	}

	out := make([]LotRemainder, 0, len(sorted))
	for _, lot := range sorted {
		afterExpiry := lot.Credits - expiredByLot[lot.ID]
		take := afterExpiry
		if consumed < take {
			take = consumed
		}
		if take < 0 {
			take = 0
		}
		consumed -= take
		out = append(out, LotRemainder{Lot: lot, Remaining: afterExpiry - take})
	}
	return out
}

// DraftExpirationEntry is an EXPIRATION the sweep should write.
type DraftExpirationEntry struct {
	MemberID    string
	Amount      int
	Description string
	LotID       string
}

// DeriveExpirationEntries returns the entries needed to zero out lots that are
// past their expiry but still hold credits. Running it twice is safe: the
// second pass sees the credits already gone and emits nothing.
func DeriveExpirationEntries(lots []TopUpLot, entries []CreditLedgerEntry, now time.Time) []DraftExpirationEntry {
	var drafts []DraftExpirationEntry
	for _, r := range ComputeLotRemainders(lots, entries) {
		if r.Remaining > 0 && !r.Lot.ExpiresAt.After(now) {
			drafts = append(drafts, DraftExpirationEntry{
				MemberID:    r.Lot.MemberID,
				Amount:      -r.Remaining,
				Description: fmt.Sprintf("Credits expired (lot %s)", r.Lot.ID),
				LotID:       r.Lot.ID,
			})
		}
	}
	return drafts
}

// ComputeExpiringCredits counts credits expiring within the horizon, excluding
// lots that already expired.
func ComputeExpiringCredits(lots []TopUpLot, entries []CreditLedgerEntry, now time.Time, withinDays int) int {
	horizon := now.AddDate(0, 0, withinDays)
	total := 0
	for _, r := range ComputeLotRemainders(lots, entries) {
		if r.Remaining > 0 && r.Lot.ExpiresAt.After(now) && !r.Lot.ExpiresAt.After(horizon) {
			total += r.Remaining
		}
	}
	return total
}

// Reversal errors. Both are business outcomes the API surfaces by code.
var (
	ErrCannotReverseReversal = errors.New("CANNOT_REVERSE_REVERSAL")
	ErrAlreadyReversed       = errors.New("ALREADY_REVERSED")
)

// DraftReversalEntry is the compensating entry for a mistaken movement.
type DraftReversalEntry struct {
	MemberID        string
	Amount          int
	Description     string
	SourceID        *string
	ReversesEntryID string
	ActorID         string
	Reason          string
}

// BuildReversal produces the entry that cancels out `original`. A reversal
// cannot itself be reversed, and an entry can only be reversed once.
func BuildReversal(original CreditLedgerEntry, actorID, reason string, alreadyReversed bool) (DraftReversalEntry, error) {
	if original.Type == LedgerReversal {
		return DraftReversalEntry{}, ErrCannotReverseReversal
	}
	if alreadyReversed {
		return DraftReversalEntry{}, ErrAlreadyReversed
	}
	return DraftReversalEntry{
		MemberID:        original.MemberID,
		Amount:          -original.Amount,
		Description:     fmt.Sprintf("Reversal of %s (%s)", original.Type, original.ID),
		SourceID:        original.SourceID,
		ReversesEntryID: original.ID,
		ActorID:         actorID,
		Reason:          reason,
	}, nil
}
