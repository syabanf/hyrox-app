import { describe, expect, it } from 'vitest';
import type { Booking, ClassSession, IncentiveScheme } from '../index';
import {
  PAYOUT_ACTION_TARGET,
  PAYOUT_STATUSES,
  PAYOUT_TRANSITIONS,
  canTransition,
  computeCoachStatement,
  monthPeriod,
  periodMonthOf,
  resolveScheme,
} from '../index';

const NOW = '2026-08-20T10:00:00.000Z';

const defaultScheme: IncentiveScheme = {
  id: 'inc_default',
  coachId: null,
  sessionFeeIdr: 150_000,
  perAttendeeIdr: 10_000,
  fullClassBonusIdr: 100_000,
  fullClassThresholdPercent: 80,
  noShowPenaltyIdr: 0,
  rates: [],
  active: true,
  updatedAt: NOW,
};

const session = (id: string, over: Partial<ClassSession> = {}): ClassSession => ({
  id,
  classTypeId: 'cls_fund',
  branchId: 'brn_senopati',
  coachId: 'coa_1',
  startsAt: '2026-08-10T07:00:00.000Z',
  endsAt: '2026-08-10T08:00:00.000Z',
  capacity: 10,
  creditCost: 1,
  bookingOpensAt: '2026-08-03T07:00:00.000Z',
  bookingClosesAt: '2026-08-10T07:00:00.000Z',
  status: 'COMPLETED',
  area: null,
  ...over,
});

const booking = (sessionId: string, status: Booking['status'], n: number): Booking => ({
  id: `bok_${sessionId}_${status}_${n}`,
  memberId: `mem_${n}`,
  sessionId,
  status,
  waitlistPosition: null,
  source: 'MEMBER',
  createdAt: NOW,
  updatedAt: NOW,
  cancelledAt: null,
  checkedInAt: null,
  promotionOfferedAt: null,
});

const bookingsOf = (sessionId: string, counts: Partial<Record<Booking['status'], number>>) =>
  Object.entries(counts).flatMap(([status, count]) =>
    Array.from({ length: count }, (_, i) => booking(sessionId, status as Booking['status'], i)),
  );

const period = { start: '2026-08-01T00:00:00.000Z', end: '2026-09-01T00:00:00.000Z' };
const classTypesById = { cls_fund: { name: 'HYROX Fundamentals' } };

const compute = (
  sessions: ClassSession[],
  bookingsBySession: Record<string, Booking[]>,
  scheme: IncentiveScheme = defaultScheme,
) =>
  computeCoachStatement({
    coach: { id: 'coa_1' },
    sessions,
    bookingsBySession,
    classTypesById,
    scheme,
    period,
  });

describe('computeCoachStatement', () => {
  it('pays the session fee plus per-attendee for checked-in members only', () => {
    const s = session('ses_1');
    const res = compute([s], {
      ses_1: bookingsOf('ses_1', { CHECKED_IN: 3, COMPLETED: 2, NO_SHOW: 1, CANCELLED: 4 }),
    });
    expect(res.lines).toHaveLength(1);
    const line = res.lines[0]!;
    expect(line.booked).toBe(6); // cancelled never held a slot
    expect(line.attended).toBe(5);
    expect(line.noShows).toBe(1);
    expect(line.sessionFeeIdr).toBe(150_000);
    expect(line.attendeeIdr).toBe(50_000);
    expect(line.bonusIdr).toBe(0); // 5/10 = 50% < 80%
    expect(line.penaltyIdr).toBe(0);
    expect(line.totalIdr).toBe(200_000);
    expect(line.classTypeName).toBe('HYROX Fundamentals');
  });

  it('awards the full-class bonus exactly at the threshold', () => {
    const s = session('ses_1');
    const at = compute([s], { ses_1: bookingsOf('ses_1', { CHECKED_IN: 8 }) });
    expect(at.lines[0]!.bonusIdr).toBe(100_000);
    expect(at.lines[0]!.totalIdr).toBe(150_000 + 80_000 + 100_000);
    const below = compute([s], { ses_1: bookingsOf('ses_1', { CHECKED_IN: 7 }) });
    expect(below.lines[0]!.bonusIdr).toBe(0);
  });

  it('bonus counts attendance, not bookings', () => {
    const s = session('ses_1');
    const res = compute([s], { ses_1: bookingsOf('ses_1', { CONFIRMED: 9, CHECKED_IN: 1 }) });
    expect(res.lines[0]!.booked).toBe(10);
    expect(res.lines[0]!.bonusIdr).toBe(0);
  });

  it('applies the no-show penalty per no-show and never goes below zero', () => {
    const harsh = { ...defaultScheme, sessionFeeIdr: 50_000, noShowPenaltyIdr: 40_000 };
    const s = session('ses_1');
    const res = compute([s], { ses_1: bookingsOf('ses_1', { CHECKED_IN: 1, NO_SHOW: 3 }) }, harsh);
    const line = res.lines[0]!;
    expect(line.penaltyIdr).toBe(120_000);
    expect(line.totalIdr).toBe(0); // 50k + 10k − 120k → clamped
    expect(res.totals.totalIdr).toBe(0);
    expect(res.totals.penaltyIdr).toBe(120_000);
  });

  it('only counts COMPLETED sessions coached by this coach inside the period', () => {
    const sessions = [
      session('in'),
      session('other_coach', { coachId: 'coa_2' }),
      session('cancelled', { status: 'CANCELLED' }),
      session('upcoming', { status: 'PUBLISHED', startsAt: '2026-08-25T07:00:00.000Z' }),
      session('last_month', { startsAt: '2026-07-31T23:00:00.000Z' }),
      session('boundary_end', { startsAt: '2026-09-01T00:00:00.000Z' }),
      session('boundary_start', { startsAt: '2026-08-01T00:00:00.000Z' }),
    ];
    const res = compute(sessions, {});
    expect(res.lines.map((l) => l.sessionId)).toEqual(['boundary_start', 'in']);
    expect(res.totals.sessions).toBe(2);
    expect(res.totals.totalIdr).toBe(300_000); // fee only, no attendees
  });

  it('sums totals across lines', () => {
    const res = compute([session('a'), session('b', { startsAt: '2026-08-11T07:00:00.000Z' })], {
      a: bookingsOf('a', { CHECKED_IN: 8, NO_SHOW: 1 }),
      b: bookingsOf('b', { CHECKED_IN: 2 }),
    });
    expect(res.totals).toEqual({
      sessions: 2,
      attended: 10,
      noShows: 1,
      sessionFeeIdr: 300_000,
      attendeeIdr: 100_000,
      bonusIdr: 100_000,
      penaltyIdr: 0,
      totalIdr: 500_000,
    });
    expect(res.coachId).toBe('coa_1');
  });

  it('a zero-capacity session never earns the bonus', () => {
    const res = compute([session('z', { capacity: 0 })], { z: bookingsOf('z', { CHECKED_IN: 1 }) });
    expect(res.lines[0]!.bonusIdr).toBe(0);
  });
});

describe('resolveScheme', () => {
  const override: IncentiveScheme = {
    ...defaultScheme,
    id: 'inc_c1',
    coachId: 'coa_1',
    sessionFeeIdr: 200_000,
  };
  it('coach override wins when active', () => {
    expect(resolveScheme(defaultScheme, override).sessionFeeIdr).toBe(200_000);
  });
  it('falls back to the default without an override or when it is inactive', () => {
    expect(resolveScheme(defaultScheme, null)).toBe(defaultScheme);
    expect(resolveScheme(defaultScheme, { ...override, active: false })).toBe(defaultScheme);
  });
});

describe('payout state machine', () => {
  const legal: ReadonlyArray<readonly [string, string]> = [
    ['DRAFT', 'APPROVED'],
    ['DRAFT', 'VOID'],
    ['APPROVED', 'PAID'],
    ['APPROVED', 'VOID'],
  ];
  it('allows exactly the declared edges', () => {
    for (const from of PAYOUT_STATUSES) {
      for (const to of PAYOUT_STATUSES) {
        const expected = legal.some(([f, t]) => f === from && t === to);
        expect(canTransition(PAYOUT_TRANSITIONS, from, to), `${from} → ${to}`).toBe(expected);
      }
    }
  });
  it('PAID and VOID are terminal', () => {
    expect(PAYOUT_TRANSITIONS.PAID).toEqual([]);
    expect(PAYOUT_TRANSITIONS.VOID).toEqual([]);
  });
  it('every admin action targets a declared status', () => {
    for (const target of Object.values(PAYOUT_ACTION_TARGET))
      expect(PAYOUT_STATUSES).toContain(target);
    expect(PAYOUT_ACTION_TARGET).toEqual({ approve: 'APPROVED', pay: 'PAID', void: 'VOID' });
  });
});

describe('month periods', () => {
  it('produces an exclusive calendar-month window and round-trips the label', () => {
    const p = monthPeriod('2026-08');
    expect(new Date(p.start).getDate()).toBe(1);
    expect(new Date(p.end).getDate()).toBe(1);
    expect(new Date(p.end).getMonth()).toBe((new Date(p.start).getMonth() + 1) % 12);
    expect(periodMonthOf(p.start)).toBe('2026-08');
    expect(periodMonthOf(new Date(new Date(p.end).getTime() - 1).toISOString())).toBe('2026-08');
  });
  it('rolls over December', () => {
    const p = monthPeriod('2026-12');
    expect(new Date(p.end).getFullYear()).toBe(2027);
    expect(new Date(p.end).getMonth()).toBe(0);
  });
  it('rejects malformed labels', () => {
    expect(() => monthPeriod('2026-8')).toThrow();
  });
});
