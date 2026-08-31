import { describe, expect, it } from 'vitest';
import type { CreditLedgerEntry, TopUpLot } from '../index';
import {
  buildReversalEntry,
  computeBalance,
  computeExpiringCredits,
  computeLotRemainders,
  deriveExpirationEntries,
} from '../index';

let n = 0;
const entry = (partial: Partial<CreditLedgerEntry>): CreditLedgerEntry => ({
  id: `led_${++n}`,
  memberId: 'mem_1',
  type: 'TOP_UP',
  amount: 0,
  description: '',
  sourceType: null,
  sourceId: null,
  reversesEntryId: null,
  actorId: null,
  reason: null,
  createdAt: '2026-01-01T00:00:00.000Z',
  ...partial,
});
const lot = (partial: Partial<TopUpLot>): TopUpLot => ({
  id: `lot_${++n}`,
  memberId: 'mem_1',
  ledgerEntryId: 'led_x',
  packageId: null,
  credits: 10,
  expiresAt: '2026-03-01T00:00:00.000Z',
  createdAt: '2026-01-01T00:00:00.000Z',
  ...partial,
});

describe('computeBalance', () => {
  it('is the sum of all entries — the blueprint example', () => {
    const entries = [
      entry({ type: 'TOP_UP', amount: 20 }),
      entry({ type: 'VISIT_DEDUCTION', amount: -1 }),
      entry({ type: 'VISIT_DEDUCTION', amount: -1 }),
      entry({ type: 'PROMO', amount: 2 }),
      entry({ type: 'EXPIRATION', amount: -3 }),
    ];
    expect(computeBalance(entries)).toBe(17);
  });
  it('is 0 with no entries', () => {
    expect(computeBalance([])).toBe(0);
  });
});

describe('computeLotRemainders (FIFO by expiry)', () => {
  it('consumes the earliest-expiring lot first', () => {
    const lots = [
      lot({ id: 'late', credits: 10, expiresAt: '2026-06-01T00:00:00.000Z' }),
      lot({ id: 'early', credits: 5, expiresAt: '2026-02-01T00:00:00.000Z' }),
    ];
    const entries = [
      entry({ type: 'TOP_UP', amount: 5 }),
      entry({ type: 'TOP_UP', amount: 10 }),
      entry({ type: 'VISIT_DEDUCTION', amount: -7 }),
    ];
    const rem = computeLotRemainders(lots, entries);
    expect(rem.map((r) => [r.lot.id, r.remaining])).toEqual([
      ['early', 0],
      ['late', 8],
    ]);
  });

  it('a positive REFUND restores consumption', () => {
    const lots = [lot({ id: 'a', credits: 5 })];
    const entries = [
      entry({ type: 'TOP_UP', amount: 5 }),
      entry({ type: 'VISIT_DEDUCTION', amount: -2 }),
      entry({ type: 'REFUND', amount: 1 }),
    ];
    expect(computeLotRemainders(lots, entries)[0]!.remaining).toBe(4);
  });
});

describe('deriveExpirationEntries', () => {
  it('emits a negative EXPIRATION draft for expired remainder', () => {
    const lots = [lot({ id: 'a', credits: 10, expiresAt: '2026-02-01T00:00:00.000Z' })];
    const entries = [
      entry({ type: 'TOP_UP', amount: 10 }),
      entry({ type: 'VISIT_DEDUCTION', amount: -4 }),
    ];
    const drafts = deriveExpirationEntries(lots, entries, '2026-02-02T00:00:00.000Z');
    expect(drafts).toHaveLength(1);
    expect(drafts[0]).toMatchObject({ amount: -6, sourceId: 'a', type: 'EXPIRATION' });
  });

  it('is idempotent: an already-expired lot produces nothing', () => {
    const lots = [lot({ id: 'a', credits: 10, expiresAt: '2026-02-01T00:00:00.000Z' })];
    const entries = [
      entry({ type: 'TOP_UP', amount: 10 }),
      entry({ type: 'VISIT_DEDUCTION', amount: -4 }),
      entry({ type: 'EXPIRATION', amount: -6, sourceId: 'a' }),
    ];
    expect(deriveExpirationEntries(lots, entries, '2026-02-02T00:00:00.000Z')).toHaveLength(0);
  });

  it('does nothing before expiry', () => {
    const lots = [lot({ credits: 10, expiresAt: '2026-02-01T00:00:00.000Z' })];
    const entries = [entry({ type: 'TOP_UP', amount: 10 })];
    expect(deriveExpirationEntries(lots, entries, '2026-01-31T00:00:00.000Z')).toHaveLength(0);
  });
});

describe('computeExpiringCredits', () => {
  it('counts only unexpired lots inside the horizon', () => {
    const lots = [
      lot({ credits: 5, expiresAt: '2026-01-05T00:00:00.000Z' }), // within 7d
      lot({ credits: 7, expiresAt: '2026-02-01T00:00:00.000Z' }), // beyond
      lot({ credits: 3, expiresAt: '2025-12-31T00:00:00.000Z' }), // already expired
    ];
    const entries = [entry({ type: 'TOP_UP', amount: 15 })];
    expect(computeExpiringCredits(lots, entries, '2026-01-01T00:00:00.000Z', 7)).toBe(5);
  });
});

describe('buildReversalEntry', () => {
  const original = entry({ id: 'led_orig', type: 'ADJUSTMENT', amount: -5 });

  it('negates the original amount and references it', () => {
    const res = buildReversalEntry(original, {
      actorId: 'adm_1',
      reason: 'Keying error',
      alreadyReversed: false,
    });
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.value.amount).toBe(5);
      expect(res.value.reversesEntryId).toBe('led_orig');
      expect(res.value.type).toBe('REVERSAL');
    }
  });

  it('refuses to reverse a reversal', () => {
    const res = buildReversalEntry(entry({ type: 'REVERSAL', amount: 5 }), {
      actorId: 'adm_1',
      reason: 'x',
      alreadyReversed: false,
    });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error.type).toBe('CANNOT_REVERSE_REVERSAL');
  });

  it('refuses a second reversal of the same entry', () => {
    const res = buildReversalEntry(original, {
      actorId: 'adm_1',
      reason: 'x',
      alreadyReversed: true,
    });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error.type).toBe('ALREADY_REVERSED');
  });
});
