import { describe, expect, it } from 'vitest';
import type { Booking, ClassSession, Member } from '../index';
import {
  DEFAULT_BUSINESS_RULES,
  evaluateBookingEligibility,
  evaluateCancellation,
  nextWaitlistPosition,
  noShowPenalty,
  pickWaitlistPromotion,
} from '../index';

const member = (partial: Partial<Member> = {}): Member => ({
  id: 'mem_1',
  fullName: 'Test Member',
  email: 't@example.com',
  phone: '+62812',
  dateOfBirth: null,
  gender: null,
  emergencyContact: null,
  preferredBranchId: null,
  avatarUrl: null,
  status: 'ACTIVE',
  waiverVersion: 'v1',
  waiverAcceptedAt: '2026-01-01T00:00:00.000Z',
  notes: null,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
  ...partial,
});

const session = (partial: Partial<ClassSession> = {}): ClassSession => ({
  id: 'ses_1',
  classTypeId: 'cls_1',
  branchId: 'brn_1',
  coachId: 'coa_1',
  startsAt: '2026-01-10T18:00:00.000Z',
  endsAt: '2026-01-10T19:00:00.000Z',
  capacity: 16,
  creditCost: 1,
  bookingOpensAt: '2026-01-03T18:00:00.000Z',
  bookingClosesAt: '2026-01-10T18:00:00.000Z',
  status: 'PUBLISHED',
  area: null,
  ...partial,
});

const base = {
  member: member(),
  session: session(),
  balance: 5,
  confirmedCount: 3,
  waitlistCount: 0,
  hasExistingActiveBooking: false,
  now: '2026-01-05T12:00:00.000Z',
};

describe('evaluateBookingEligibility', () => {
  it('confirms the happy path', () => {
    expect(evaluateBookingEligibility(base)).toEqual({ kind: 'CONFIRM' });
  });

  it.each([
    ['SUSPENDED', 'MEMBER_NOT_ACTIVE'],
    ['INACTIVE', 'MEMBER_NOT_ACTIVE'],
    ['ARCHIVED', 'MEMBER_NOT_ACTIVE'],
  ] as const)('denies %s member', (status, reason) => {
    const d = evaluateBookingEligibility({ ...base, member: member({ status }) });
    expect(d).toEqual({ kind: 'DENY', reason });
  });

  it.each(['DRAFT', 'COMPLETED', 'CANCELLED'] as const)('denies %s session', (status) => {
    const d = evaluateBookingEligibility({ ...base, session: session({ status }) });
    expect(d).toEqual({ kind: 'DENY', reason: 'SESSION_NOT_BOOKABLE' });
  });

  it('denies before the booking window opens', () => {
    const d = evaluateBookingEligibility({ ...base, now: '2026-01-02T00:00:00.000Z' });
    expect(d).toEqual({ kind: 'DENY', reason: 'BOOKING_NOT_OPEN_YET' });
  });

  it('denies after the booking window closes', () => {
    const d = evaluateBookingEligibility({ ...base, now: '2026-01-10T18:01:00.000Z' });
    expect(d).toEqual({ kind: 'DENY', reason: 'BOOKING_WINDOW_CLOSED' });
  });

  it('denies a duplicate booking', () => {
    const d = evaluateBookingEligibility({ ...base, hasExistingActiveBooking: true });
    expect(d).toEqual({ kind: 'DENY', reason: 'ALREADY_BOOKED' });
  });

  it('denies insufficient credit', () => {
    const d = evaluateBookingEligibility({
      ...base,
      balance: 1,
      session: session({ creditCost: 2 }),
    });
    expect(d).toEqual({ kind: 'DENY', reason: 'INSUFFICIENT_CREDITS' });
  });

  it('waitlists when full — FULL status or capacity reached', () => {
    expect(
      evaluateBookingEligibility({ ...base, confirmedCount: 16, waitlistCount: 2 }),
    ).toEqual({ kind: 'WAITLIST', position: 3 });
    expect(
      evaluateBookingEligibility({
        ...base,
        session: session({ status: 'FULL' }),
        confirmedCount: 16,
        waitlistCount: 0,
      }),
    ).toEqual({ kind: 'WAITLIST', position: 1 });
  });
});

describe('evaluateCancellation', () => {
  const rules = DEFAULT_BUSINESS_RULES; // deadline 4h before start

  it('releases before the deadline', () => {
    const out = evaluateCancellation({ session: session(), rules, now: '2026-01-10T13:59:00.000Z' });
    expect(out.kind).toBe('RELEASED');
  });

  it('is late (forfeit) after the deadline', () => {
    const out = evaluateCancellation({ session: session(), rules, now: '2026-01-10T14:01:00.000Z' });
    expect(out).toMatchObject({ kind: 'LATE', penaltyCredits: 1 });
  });

  it('late with FREE policy has no penalty', () => {
    const out = evaluateCancellation({
      session: session(),
      rules: { ...rules, lateCancellationPolicy: 'FREE' },
      now: '2026-01-10T15:00:00.000Z',
    });
    expect(out).toMatchObject({ kind: 'LATE', penaltyCredits: 0 });
  });
});

describe('no-show penalty', () => {
  it('follows the configured policy', () => {
    expect(noShowPenalty(session({ creditCost: 2 }), DEFAULT_BUSINESS_RULES)).toBe(2);
    expect(
      noShowPenalty(session({ creditCost: 2 }), { ...DEFAULT_BUSINESS_RULES, noShowPolicy: 'FREE' }),
    ).toBe(0);
  });
});

describe('waitlist', () => {
  const wl = (id: string, position: number, createdAt: string): Booking => ({
    id,
    memberId: `mem_${id}`,
    sessionId: 'ses_1',
    status: 'WAITLIST',
    waitlistPosition: position,
    source: 'MEMBER',
    createdAt,
    updatedAt: createdAt,
    cancelledAt: null,
    checkedInAt: null,
    promotionOfferedAt: null,
  });

  it('promotes the lowest position first', () => {
    const pick = pickWaitlistPromotion([
      wl('b', 2, '2026-01-01T00:00:00.000Z'),
      wl('a', 1, '2026-01-02T00:00:00.000Z'),
    ]);
    expect(pick?.id).toBe('a');
  });

  it('returns null when nobody is waiting', () => {
    expect(pickWaitlistPromotion([])).toBeNull();
  });

  it('computes the next position', () => {
    expect(nextWaitlistPosition([wl('a', 1, '2026-01-01T00:00:00.000Z')])).toBe(2);
    expect(nextWaitlistPosition([])).toBe(1);
  });
});
