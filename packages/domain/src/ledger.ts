import type { Result } from './shared/result';
import { err, ok } from './shared/result';
import type { IsoDate } from './shared/time';
import { addDaysIso, isSameOrBefore, msOf } from './shared/time';

export const LEDGER_ENTRY_TYPES = [
  'TOP_UP',
  'VISIT_DEDUCTION',
  'REFUND',
  'BONUS',
  'PROMO',
  'EXPIRATION',
  'ADJUSTMENT',
  'REVERSAL',
] as const;
export type LedgerEntryType = (typeof LEDGER_ENTRY_TYPES)[number];

export type LedgerSourceType = 'PAYMENT' | 'BOOKING' | 'ACCESS' | 'ADMIN' | 'SYSTEM';

/**
 * A finalized ledger entry is immutable. Corrections are REVERSAL entries
 * referencing the original — never edits, never deletes.
 */
export interface CreditLedgerEntry {
  id: string;
  memberId: string;
  type: LedgerEntryType;
  /** Signed integer credits. */
  amount: number;
  description: string;
  sourceType: LedgerSourceType | null;
  sourceId: string | null;
  /** Set on REVERSAL entries: the entry being reversed. */
  reversesEntryId: string | null;
  actorId: string | null;
  reason: string | null;
  createdAt: IsoDate;
}

/** A top-up lot tracks a batch of credits and when they expire (FIFO consumption). */
export interface TopUpLot {
  id: string;
  memberId: string;
  ledgerEntryId: string;
  /** The purchased package (null for bonus/adjustment credits). */
  packageId: string | null;
  credits: number;
  expiresAt: IsoDate;
  createdAt: IsoDate;
}

/** THE wallet rule: balance is always derived, never stored. */
export const computeBalance = (entries: readonly CreditLedgerEntry[]): number =>
  entries.reduce((sum, e) => sum + e.amount, 0);

export interface LotRemainder {
  lot: TopUpLot;
  remaining: number;
}

/**
 * Allocates consumption across lots FIFO (earliest expiry first) and returns
 * what is left in each lot. EXPIRATION entries are pinned to their lot via sourceId.
 */
export function computeLotRemainders(
  lots: readonly TopUpLot[],
  entries: readonly CreditLedgerEntry[],
): LotRemainder[] {
  const sorted = [...lots].sort((a, b) => msOf(a.expiresAt) - msOf(b.expiresAt));

  // Consumption pool: everything negative except EXPIRATION (those are lot-specific),
  // minus positive restorations (REFUND / positive REVERSAL give credits back).
  let consumed = 0;
  for (const e of entries) {
    if (e.type === 'EXPIRATION') continue;
    if (e.amount < 0) consumed += -e.amount;
    else if ((e.type === 'REFUND' || e.type === 'REVERSAL') && e.amount > 0) consumed -= e.amount;
  }
  consumed = Math.max(0, consumed);

  const expiredByLot = new Map<string, number>();
  for (const e of entries) {
    if (e.type === 'EXPIRATION' && e.sourceId) {
      expiredByLot.set(e.sourceId, (expiredByLot.get(e.sourceId) ?? 0) + -e.amount);
    }
  }

  return sorted.map((lot) => {
    const afterExpiry = lot.credits - (expiredByLot.get(lot.id) ?? 0);
    const take = Math.min(afterExpiry, consumed);
    consumed -= take;
    return { lot, remaining: afterExpiry - take };
  });
}

export interface DraftExpirationEntry {
  memberId: string;
  type: 'EXPIRATION';
  amount: number;
  description: string;
  sourceType: 'SYSTEM';
  sourceId: string;
}

/** Emits the EXPIRATION drafts for lots past their expiry that still hold credits. */
export function deriveExpirationEntries(
  lots: readonly TopUpLot[],
  entries: readonly CreditLedgerEntry[],
  now: IsoDate,
): DraftExpirationEntry[] {
  return computeLotRemainders(lots, entries)
    .filter(({ lot, remaining }) => remaining > 0 && isSameOrBefore(lot.expiresAt, now))
    .map(({ lot, remaining }) => ({
      memberId: lot.memberId,
      type: 'EXPIRATION' as const,
      amount: -remaining,
      description: `Credits expired (lot ${lot.id})`,
      sourceType: 'SYSTEM' as const,
      sourceId: lot.id,
    }));
}

/** Credits that will expire within `withinDays` (excluding already-expired lots). */
export function computeExpiringCredits(
  lots: readonly TopUpLot[],
  entries: readonly CreditLedgerEntry[],
  now: IsoDate,
  withinDays: number,
): number {
  const horizon = addDaysIso(now, withinDays);
  return computeLotRemainders(lots, entries)
    .filter(
      ({ lot, remaining }) =>
        remaining > 0 && msOf(lot.expiresAt) > msOf(now) && isSameOrBefore(lot.expiresAt, horizon),
    )
    .reduce((sum, { remaining }) => sum + remaining, 0);
}

export type ReversalProblem =
  | { type: 'CANNOT_REVERSE_REVERSAL' }
  | { type: 'ALREADY_REVERSED' };

export interface DraftReversalEntry {
  memberId: string;
  type: 'REVERSAL';
  amount: number;
  description: string;
  sourceType: 'ADMIN';
  sourceId: string | null;
  reversesEntryId: string;
  actorId: string;
  reason: string;
}

/** Builds the compensating entry for `original`. Caller must confirm no prior reversal exists. */
export function buildReversalEntry(
  original: CreditLedgerEntry,
  args: { actorId: string; reason: string; alreadyReversed: boolean },
): Result<DraftReversalEntry, ReversalProblem> {
  if (original.type === 'REVERSAL') return err({ type: 'CANNOT_REVERSE_REVERSAL' });
  if (args.alreadyReversed) return err({ type: 'ALREADY_REVERSED' });
  return ok({
    memberId: original.memberId,
    type: 'REVERSAL',
    amount: -original.amount,
    description: `Reversal of ${original.type} (${original.id})`,
    sourceType: 'ADMIN',
    sourceId: original.sourceId,
    reversesEntryId: original.id,
    actorId: args.actorId,
    reason: args.reason,
  });
}
