import type { ClassSession } from './classes';
import type { Member } from './member';
import type { BusinessRules } from './rules';
import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';
import { addHoursIso, isAfter, isBefore, msOf } from './shared/time';

export const BOOKING_STATUSES = [
  'PENDING',
  'CONFIRMED',
  'WAITLIST',
  'CANCELLED',
  'CHECKED_IN',
  'COMPLETED',
  'NO_SHOW',
] as const;
export type BookingStatus = (typeof BOOKING_STATUSES)[number];

export const BOOKING_TRANSITIONS: TransitionMap<BookingStatus> = {
  PENDING: ['CONFIRMED', 'WAITLIST', 'CANCELLED'],
  CONFIRMED: ['CANCELLED', 'CHECKED_IN', 'NO_SHOW'],
  WAITLIST: ['CONFIRMED', 'CANCELLED'],
  CHECKED_IN: ['COMPLETED'],
  CANCELLED: [],
  COMPLETED: [],
  NO_SHOW: [],
};

/** Booking states a member can still act on / that hold a slot or waitlist place. */
export const ACTIVE_BOOKING_STATUSES: readonly BookingStatus[] = [
  'PENDING',
  'CONFIRMED',
  'WAITLIST',
  'CHECKED_IN',
];

export interface Booking {
  id: string;
  memberId: string;
  sessionId: string;
  status: BookingStatus;
  waitlistPosition: number | null;
  source: 'MEMBER' | 'ADMIN';
  createdAt: IsoDate;
  updatedAt: IsoDate;
  cancelledAt: IsoDate | null;
  checkedInAt: IsoDate | null;
  /** Set when a freed slot was offered to this waitlisted member (manual-confirm policy). */
  promotionOfferedAt: IsoDate | null;
}

export type BookingDenialReason =
  | 'MEMBER_NOT_ACTIVE'
  | 'SESSION_NOT_BOOKABLE'
  | 'BOOKING_NOT_OPEN_YET'
  | 'BOOKING_WINDOW_CLOSED'
  | 'ALREADY_BOOKED'
  | 'INSUFFICIENT_CREDITS';

export type BookingDecision =
  | { kind: 'CONFIRM' }
  | { kind: 'WAITLIST'; position: number }
  | { kind: 'DENY'; reason: BookingDenialReason };

/**
 * Credits are deducted at CHECK-IN (gate), not at booking. Booking validates
 * that the member *could* pay, holds the slot, and richer states do the rest.
 */
export function evaluateBookingEligibility(args: {
  member: Member;
  session: ClassSession;
  balance: number;
  confirmedCount: number;
  waitlistCount: number;
  hasExistingActiveBooking: boolean;
  now: IsoDate;
}): BookingDecision {
  const { member, session, now } = args;
  if (member.status !== 'ACTIVE') return { kind: 'DENY', reason: 'MEMBER_NOT_ACTIVE' };
  if (session.status !== 'PUBLISHED' && session.status !== 'FULL')
    return { kind: 'DENY', reason: 'SESSION_NOT_BOOKABLE' };
  if (isBefore(now, session.bookingOpensAt)) return { kind: 'DENY', reason: 'BOOKING_NOT_OPEN_YET' };
  if (isAfter(now, session.bookingClosesAt))
    return { kind: 'DENY', reason: 'BOOKING_WINDOW_CLOSED' };
  if (args.hasExistingActiveBooking) return { kind: 'DENY', reason: 'ALREADY_BOOKED' };
  if (args.balance < session.creditCost) return { kind: 'DENY', reason: 'INSUFFICIENT_CREDITS' };
  if (args.confirmedCount >= session.capacity)
    return { kind: 'WAITLIST', position: args.waitlistCount + 1 };
  return { kind: 'CONFIRM' };
}

export type CancellationOutcome =
  | { kind: 'RELEASED'; deadline: IsoDate }
  | { kind: 'LATE'; deadline: IsoDate; penaltyCredits: number };

export function cancellationDeadline(session: ClassSession, rules: BusinessRules): IsoDate {
  return addHoursIso(session.startsAt, -rules.cancellationDeadlineHours);
}

export function evaluateCancellation(args: {
  session: ClassSession;
  rules: BusinessRules;
  now: IsoDate;
}): CancellationOutcome {
  const deadline = cancellationDeadline(args.session, args.rules);
  if (isAfter(args.now, deadline)) {
    return {
      kind: 'LATE',
      deadline,
      penaltyCredits:
        args.rules.lateCancellationPolicy === 'FORFEIT' ? args.session.creditCost : 0,
    };
  }
  return { kind: 'RELEASED', deadline };
}

export function noShowPenalty(session: ClassSession, rules: BusinessRules): number {
  return rules.noShowPolicy === 'FORFEIT' ? session.creditCost : 0;
}

/** FIFO: lowest waitlist position wins; ties broken by creation time. */
export function pickWaitlistPromotion(waitlist: readonly Booking[]): Booking | null {
  const candidates = waitlist.filter((b) => b.status === 'WAITLIST');
  if (candidates.length === 0) return null;
  return [...candidates].sort(
    (a, b) =>
      (a.waitlistPosition ?? Number.MAX_SAFE_INTEGER) -
        (b.waitlistPosition ?? Number.MAX_SAFE_INTEGER) || msOf(a.createdAt) - msOf(b.createdAt),
  )[0]!;
}

export function nextWaitlistPosition(waitlist: readonly Booking[]): number {
  return (
    waitlist
      .filter((b) => b.status === 'WAITLIST')
      .reduce((max, b) => Math.max(max, b.waitlistPosition ?? 0), 0) + 1
  );
}
