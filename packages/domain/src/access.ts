import type { Booking } from './booking';
import type { Member } from './member';
import type { QrToken, QrTokenProblem } from './qr';
import type { BusinessRules } from './rules';
import type { Result } from './shared/result';
import type { IsoDate } from './shared/time';
import { minutesBetween } from './shared/time';

export const ACCESS_LOG_STATES = [
  'REQUESTED',
  'ALLOWED',
  'DENIED',
  'OFFLINE_ALLOWED',
  'SYNCED',
  'CONFLICT',
] as const;
export type AccessLogState = (typeof ACCESS_LOG_STATES)[number];

export type GateDenialReason =
  | 'TOKEN_INVALID'
  | 'TOKEN_EXPIRED'
  | 'TOKEN_CONSUMED'
  | 'MEMBER_NOT_ACTIVE'
  | 'ANTI_PASSBACK'
  | 'INSUFFICIENT_CREDITS';

export interface AccessLog {
  id: string;
  memberId: string | null;
  gateId: string;
  branchId: string;
  result: AccessLogState;
  reasonCode: GateDenialReason | null;
  /** Signed credits applied by this entry (0 for denied / re-entry). */
  creditDelta: number;
  mode: 'ONLINE' | 'OFFLINE';
  bookingId: string | null;
  createdAt: IsoDate;
}

export type GateEffect =
  | { kind: 'CONSUME_TOKEN'; token: string }
  | { kind: 'DEDUCT_CREDITS'; amount: number; description: string }
  | { kind: 'CHECK_IN_BOOKING'; bookingId: string };

export type GateEntryKind = 'BOOKING' | 'OPEN_GYM' | 'RE_ENTRY';

export interface GateScanEvaluation {
  decision: 'ALLOWED' | 'DENIED';
  reason: GateDenialReason | null;
  entryKind: GateEntryKind | null;
  effects: GateEffect[];
}

/**
 * The gate validation pipeline (QR → membership → anti-passback → credit →
 * decision). Pure: returns a decision plus an *effects list*; the application
 * layer applies effects atomically so the gate never opens without its
 * deduction (and vice versa).
 */
export function evaluateGateScan(args: {
  tokenCheck: Result<QrToken, { reason: QrTokenProblem }>;
  member: Member | null;
  balance: number;
  lastAllowedEntryAt: IsoDate | null;
  /** The member's CONFIRMED booking around `now` at this branch, if any. */
  candidateBooking: (Pick<Booking, 'id'> & { creditCost: number }) | null;
  rules: BusinessRules;
  now: IsoDate;
}): GateScanEvaluation {
  const { tokenCheck, member, rules, now } = args;

  const denied = (reason: GateDenialReason, effects: GateEffect[] = []): GateScanEvaluation => ({
    decision: 'DENIED',
    reason,
    entryKind: null,
    effects,
  });

  // 1. QR valid?
  if (!tokenCheck.ok) {
    const map: Record<QrTokenProblem, GateDenialReason> = {
      NOT_FOUND: 'TOKEN_INVALID',
      EXPIRED: 'TOKEN_EXPIRED',
      CONSUMED: 'TOKEN_CONSUMED',
    };
    return denied(map[tokenCheck.error.reason]);
  }
  const consume: GateEffect = { kind: 'CONSUME_TOKEN', token: tokenCheck.value.token };

  // 2. Membership active?
  if (!member || member.status !== 'ACTIVE') return denied('MEMBER_NOT_ACTIVE', [consume]);

  // 3. Anti-passback / re-entry grace.
  if (args.lastAllowedEntryAt !== null) {
    const minsSince = minutesBetween(args.lastAllowedEntryAt, now);
    if (minsSince >= 0 && minsSince <= rules.reEntryGraceMinutes) {
      return { decision: 'ALLOWED', reason: null, entryKind: 'RE_ENTRY', effects: [consume] };
    }
    if (minsSince >= 0 && minsSince <= rules.antiPassbackMinutes) {
      return denied('ANTI_PASSBACK', [consume]);
    }
  }

  // 4. Credit + policy: booked entry deducts the session cost, open gym the configured cost.
  if (args.candidateBooking) {
    if (args.balance < args.candidateBooking.creditCost)
      return denied('INSUFFICIENT_CREDITS', [consume]);
    return {
      decision: 'ALLOWED',
      reason: null,
      entryKind: 'BOOKING',
      effects: [
        consume,
        {
          kind: 'DEDUCT_CREDITS',
          amount: args.candidateBooking.creditCost,
          description: 'Class check-in',
        },
        { kind: 'CHECK_IN_BOOKING', bookingId: args.candidateBooking.id },
      ],
    };
  }

  if (args.balance < rules.openGymCreditCost) return denied('INSUFFICIENT_CREDITS', [consume]);
  return {
    decision: 'ALLOWED',
    reason: null,
    entryKind: 'OPEN_GYM',
    effects: [
      consume,
      { kind: 'DEDUCT_CREDITS', amount: rules.openGymCreditCost, description: 'Studio entry' },
    ],
  };
}
