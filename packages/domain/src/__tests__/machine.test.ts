import { describe, expect, it } from 'vitest';
import {
  BOOKING_STATUSES,
  BOOKING_TRANSITIONS,
  CAMPAIGN_STATUSES,
  CAMPAIGN_TRANSITIONS,
  MEMBER_STATUSES,
  MEMBER_TRANSITIONS,
  PAYMENT_STATUSES,
  PAYMENT_TRANSITIONS,
  SESSION_STATUSES,
  SESSION_TRANSITIONS,
  VOUCHER_STATUSES,
  VOUCHER_TRANSITIONS,
  canTransition,
  transition,
} from '../index';
import type { TransitionMap } from '../index';

function exhaustive<S extends string>(
  name: string,
  statuses: readonly S[],
  map: TransitionMap<S>,
  legal: ReadonlyArray<readonly [S, S]>,
) {
  describe(name, () => {
    it('allows exactly the declared edges', () => {
      for (const from of statuses) {
        for (const to of statuses) {
          const expected = legal.some(([f, t]) => f === from && t === to);
          expect(canTransition(map, from, to), `${from} -> ${to}`).toBe(expected);
        }
      }
    });
    it('transition() returns err on an illegal edge', () => {
      const [from, to] = [statuses[0]!, statuses[0]!];
      if (!legal.some(([f, t]) => f === from && t === to)) {
        const res = transition(map, from, to);
        expect(res.ok).toBe(false);
        if (!res.ok) expect(res.error.type).toBe('INVALID_TRANSITION');
      }
    });
  });
}

exhaustive('member machine', MEMBER_STATUSES, MEMBER_TRANSITIONS, [
  ['ACTIVE', 'SUSPENDED'],
  ['ACTIVE', 'INACTIVE'],
  ['ACTIVE', 'ARCHIVED'],
  ['SUSPENDED', 'ACTIVE'],
  ['SUSPENDED', 'ARCHIVED'],
  ['INACTIVE', 'ACTIVE'],
  ['INACTIVE', 'ARCHIVED'],
]);

exhaustive('payment machine', PAYMENT_STATUSES, PAYMENT_TRANSITIONS, [
  ['DRAFT', 'PENDING'],
  ['PENDING', 'PAID'],
  ['PENDING', 'FAILED'],
  ['PENDING', 'EXPIRED'],
  ['PAID', 'REFUNDED'],
]);

exhaustive('booking machine', BOOKING_STATUSES, BOOKING_TRANSITIONS, [
  ['PENDING', 'CONFIRMED'],
  ['PENDING', 'WAITLIST'],
  ['PENDING', 'CANCELLED'],
  ['CONFIRMED', 'CANCELLED'],
  ['CONFIRMED', 'CHECKED_IN'],
  ['CONFIRMED', 'NO_SHOW'],
  ['WAITLIST', 'CONFIRMED'],
  ['WAITLIST', 'CANCELLED'],
  ['CHECKED_IN', 'COMPLETED'],
]);

exhaustive('session machine', SESSION_STATUSES, SESSION_TRANSITIONS, [
  ['DRAFT', 'PUBLISHED'],
  ['DRAFT', 'CANCELLED'],
  ['PUBLISHED', 'FULL'],
  ['PUBLISHED', 'COMPLETED'],
  ['PUBLISHED', 'CANCELLED'],
  ['FULL', 'PUBLISHED'],
  ['FULL', 'COMPLETED'],
  ['FULL', 'CANCELLED'],
]);

exhaustive('voucher machine', VOUCHER_STATUSES, VOUCHER_TRANSITIONS, [
  ['DRAFT', 'SCHEDULED'],
  ['DRAFT', 'ACTIVE'],
  ['DRAFT', 'DISABLED'],
  ['SCHEDULED', 'ACTIVE'],
  ['SCHEDULED', 'DISABLED'],
  ['ACTIVE', 'EXPIRED'],
  ['ACTIVE', 'DISABLED'],
  ['DISABLED', 'ACTIVE'],
]);

exhaustive('campaign machine', CAMPAIGN_STATUSES, CAMPAIGN_TRANSITIONS, [
  ['DRAFT', 'SCHEDULED'],
  ['DRAFT', 'PROCESSING'],
  ['DRAFT', 'CANCELLED'],
  ['SCHEDULED', 'PROCESSING'],
  ['SCHEDULED', 'CANCELLED'],
  ['PROCESSING', 'SENT'],
  ['PROCESSING', 'FAILED'],
]);
