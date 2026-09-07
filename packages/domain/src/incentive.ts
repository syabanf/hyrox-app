import type { Booking } from './booking';
import type { ClassSession, ClassType } from './classes';
import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';
import { msOf } from './shared/time';

/**
 * Coach incentive scheme: how a coach is paid (IDR payroll) for the classes
 * they deliver. Independent of the member credit ledger. `coachId: null` is
 * the organization default; a coach-specific row overrides it.
 */
export interface IncentiveScheme {
  id: string;
  coachId: string | null;
  /** Flat fee per completed session. */
  sessionFeeIdr: number;
  /** Paid per member who actually attended (checked in). */
  perAttendeeIdr: number;
  /** Paid on top when attendance reaches the threshold below. */
  fullClassBonusIdr: number;
  /** e.g. 80 → bonus when attended ≥ 80% of capacity. */
  fullClassThresholdPercent: number;
  /** Deducted per no-show; may be 0. Never drives a line below zero. */
  noShowPenaltyIdr: number;
  active: boolean;
  updatedAt: IsoDate;
}

/** The coach-specific scheme wins when present and active; else the org default. */
export function resolveScheme(
  defaultScheme: IncentiveScheme,
  coachOverride: IncentiveScheme | null,
): IncentiveScheme {
  return coachOverride && coachOverride.active ? coachOverride : defaultScheme;
}

export interface StatementPeriod {
  start: IsoDate;
  /** Exclusive. */
  end: IsoDate;
}

/**
 * Calendar-month boundaries for a `YYYY-MM` period in the runtime's local time
 * zone (the studio's), matching how sessions are scheduled.
 */
export function monthPeriod(periodMonth: string): StatementPeriod {
  const match = /^(\d{4})-(\d{2})$/.exec(periodMonth);
  if (!match) throw new Error(`Invalid period month: ${periodMonth}`);
  const year = Number(match[1]);
  const month = Number(match[2]);
  return {
    start: new Date(year, month - 1, 1, 0, 0, 0, 0).toISOString(),
    end: new Date(year, month, 1, 0, 0, 0, 0).toISOString(),
  };
}

/** `YYYY-MM` of an ISO timestamp in the runtime's local time zone. */
export function periodMonthOf(iso: IsoDate): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

export interface CoachStatementLine {
  sessionId: string;
  startsAt: IsoDate;
  classTypeName: string;
  capacity: number;
  /** Bookings that held a slot (confirmed, checked in, completed or no-show). */
  booked: number;
  /** Members who actually came (CHECKED_IN, or COMPLETED after check-in). */
  attended: number;
  noShows: number;
  sessionFeeIdr: number;
  attendeeIdr: number;
  bonusIdr: number;
  penaltyIdr: number;
  /** fee + attendee + bonus − penalty, clamped at 0. */
  totalIdr: number;
}

export interface CoachStatementTotals {
  sessions: number;
  attended: number;
  noShows: number;
  sessionFeeIdr: number;
  attendeeIdr: number;
  bonusIdr: number;
  penaltyIdr: number;
  totalIdr: number;
}

export interface CoachStatement {
  coachId: string;
  lines: CoachStatementLine[];
  totals: CoachStatementTotals;
}

const HELD_SLOT: readonly Booking['status'][] = ['CONFIRMED', 'CHECKED_IN', 'COMPLETED', 'NO_SHOW'];
const ATTENDED: readonly Booking['status'][] = ['CHECKED_IN', 'COMPLETED'];

/**
 * Pure statement math for one coach over a period. Only COMPLETED sessions
 * coached by `coach` that start inside `[period.start, period.end)` count.
 *
 *   line = sessionFee + attended × perAttendee
 *        + (attended ≥ threshold% of capacity ? fullClassBonus : 0)
 *        − noShows × noShowPenalty            → clamped at 0
 */
export function computeCoachStatement(args: {
  coach: { id: string };
  sessions: readonly ClassSession[];
  bookingsBySession: Readonly<Record<string, readonly Booking[]>>;
  classTypesById: Readonly<Record<string, Pick<ClassType, 'name'>>>;
  scheme: IncentiveScheme;
  period: StatementPeriod;
}): CoachStatement {
  const { scheme, period } = args;
  const startMs = msOf(period.start);
  const endMs = msOf(period.end);

  const lines: CoachStatementLine[] = args.sessions
    .filter(
      (s) =>
        s.coachId === args.coach.id &&
        s.status === 'COMPLETED' &&
        msOf(s.startsAt) >= startMs &&
        msOf(s.startsAt) < endMs,
    )
    .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt))
    .map((session) => {
      const bookings = args.bookingsBySession[session.id] ?? [];
      const booked = bookings.filter((b) => HELD_SLOT.includes(b.status)).length;
      const attended = bookings.filter((b) => ATTENDED.includes(b.status)).length;
      const noShows = bookings.filter((b) => b.status === 'NO_SHOW').length;
      const fillPercent = session.capacity > 0 ? (attended / session.capacity) * 100 : 0;
      const sessionFeeIdr = scheme.sessionFeeIdr;
      const attendeeIdr = attended * scheme.perAttendeeIdr;
      const bonusIdr =
        session.capacity > 0 && fillPercent >= scheme.fullClassThresholdPercent
          ? scheme.fullClassBonusIdr
          : 0;
      const penaltyIdr = noShows * scheme.noShowPenaltyIdr;
      return {
        sessionId: session.id,
        startsAt: session.startsAt,
        classTypeName: args.classTypesById[session.classTypeId]?.name ?? 'Class',
        capacity: session.capacity,
        booked,
        attended,
        noShows,
        sessionFeeIdr,
        attendeeIdr,
        bonusIdr,
        penaltyIdr,
        totalIdr: Math.max(0, sessionFeeIdr + attendeeIdr + bonusIdr - penaltyIdr),
      };
    });

  const totals = lines.reduce<CoachStatementTotals>(
    (acc, line) => ({
      sessions: acc.sessions + 1,
      attended: acc.attended + line.attended,
      noShows: acc.noShows + line.noShows,
      sessionFeeIdr: acc.sessionFeeIdr + line.sessionFeeIdr,
      attendeeIdr: acc.attendeeIdr + line.attendeeIdr,
      bonusIdr: acc.bonusIdr + line.bonusIdr,
      penaltyIdr: acc.penaltyIdr + line.penaltyIdr,
      totalIdr: acc.totalIdr + line.totalIdr,
    }),
    {
      sessions: 0,
      attended: 0,
      noShows: 0,
      sessionFeeIdr: 0,
      attendeeIdr: 0,
      bonusIdr: 0,
      penaltyIdr: 0,
      totalIdr: 0,
    },
  );

  return { coachId: args.coach.id, lines, totals };
}

// ── Payouts ───────────────────────────────────────────────────────────────────

export const PAYOUT_STATUSES = ['DRAFT', 'APPROVED', 'PAID', 'VOID'] as const;
export type PayoutStatus = (typeof PAYOUT_STATUSES)[number];

export const PAYOUT_TRANSITIONS: TransitionMap<PayoutStatus> = {
  DRAFT: ['APPROVED', 'VOID'],
  APPROVED: ['PAID', 'VOID'],
  PAID: [],
  VOID: [],
};

export const PAYOUT_ACTIONS = ['approve', 'pay', 'void'] as const;
export type PayoutAction = (typeof PAYOUT_ACTIONS)[number];

/** Each admin action targets exactly one state; the transition map decides legality. */
export const PAYOUT_ACTION_TARGET: Record<PayoutAction, PayoutStatus> = {
  approve: 'APPROVED',
  pay: 'PAID',
  void: 'VOID',
};

/**
 * A payout freezes a coach's statement for one calendar month. Later
 * attendance edits never change a payout - void it and create a new one.
 */
export interface IncentivePayout {
  id: string;
  coachId: string;
  branchId: string;
  periodStart: IsoDate;
  /** Exclusive month boundary. */
  periodEnd: IsoDate;
  statement: CoachStatement;
  status: PayoutStatus;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: IsoDate | null;
  paidAt: IsoDate | null;
  paymentReference: string | null;
  note: string | null;
  createdAt: IsoDate;
  updatedAt: IsoDate;
}
