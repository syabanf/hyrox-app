import type {
  Booking,
  Coach,
  CoachStatement,
  IncentivePayout,
  IncentiveScheme,
  SchemeRate,
  PayoutAction,
  Result,
  StatementPeriod,
} from '@nuhabit/domain';
import {
  PAYOUT_ACTION_TARGET,
  PAYOUT_TRANSITIONS,
  canTransition,
  computeCoachStatement,
  monthPeriod,
  periodMonthOf,
  resolveScheme,
} from '@nuhabit/domain';
import { err, ok } from '@nuhabit/domain';
import type { Actor, AppError } from '../common';
import { appError, recordAudit } from '../common';
import type { UseCaseDeps } from '../ports';

const PERIOD_RE = /^\d{4}-\d{2}$/;

function periodOf(periodMonth: string): Result<StatementPeriod, AppError> {
  if (!PERIOD_RE.test(periodMonth))
    return err(appError('INVALID_PERIOD', 'Period must be YYYY-MM.', 400));
  return ok(monthPeriod(periodMonth));
}

// ── Schemes ───────────────────────────────────────────────────────────────────

/** The organization default plus every coach override, default first. */
export function getSchemes(deps: UseCaseDeps): IncentiveScheme[] {
  return [...deps.incentiveSchemes.all()].sort((a, b) =>
    a.coachId === null ? -1 : b.coachId === null ? 1 : a.coachId.localeCompare(b.coachId),
  );
}

export function defaultScheme(deps: UseCaseDeps): IncentiveScheme | null {
  return deps.incentiveSchemes.all().find((s) => s.coachId === null) ?? null;
}

export function schemeFor(deps: UseCaseDeps, coachId: string): Result<IncentiveScheme, AppError> {
  const base = defaultScheme(deps);
  if (!base)
    return err(
      appError('NO_DEFAULT_SCHEME', 'Set the organization default incentive scheme first.', 409),
    );
  const override = deps.incentiveSchemes.all().find((s) => s.coachId === coachId) ?? null;
  return ok(resolveScheme(base, override));
}

export interface UpsertSchemeInput {
  coachId: string | null;
  sessionFeeIdr: number;
  perAttendeeIdr: number;
  fullClassBonusIdr: number;
  fullClassThresholdPercent: number;
  noShowPenaltyIdr: number;
  /** Per-class-type rates, replaced wholesale on every save. */
  rates?: SchemeRate[];
  active: boolean;
}

/**
 * Create or update the default scheme (`coachId: null`) or a coach override.
 * One row per coach: saving for a coach that already has one updates it.
 * Audited like business rules.
 */
export function upsertScheme(
  deps: UseCaseDeps,
  args: { id?: string | null; input: UpsertSchemeInput; actor: Actor },
): Result<IncentiveScheme, AppError> {
  const { input } = args;
  if (input.coachId !== null && !deps.coaches.byId(input.coachId))
    return err(appError('NOT_FOUND', 'Coach not found.', 404));
  const byId = args.id ? deps.incentiveSchemes.byId(args.id) : null;
  if (args.id && !byId) return err(appError('NOT_FOUND', 'Incentive scheme not found.', 404));
  const existing =
    byId ?? deps.incentiveSchemes.all().find((s) => s.coachId === input.coachId) ?? null;
  if (existing && existing.coachId !== input.coachId)
    return err(appError('COACH_MISMATCH', 'A scheme cannot be moved to another coach.'));
  if (existing?.coachId === null && !input.active)
    return err(appError('DEFAULT_REQUIRED', 'The organization default scheme must stay active.'));

  const now = deps.clock.now();
  const scheme: IncentiveScheme = {
    id: existing?.id ?? deps.ids.next('inc'),
    coachId: input.coachId,
    sessionFeeIdr: input.sessionFeeIdr,
    perAttendeeIdr: input.perAttendeeIdr,
    fullClassBonusIdr: input.fullClassBonusIdr,
    fullClassThresholdPercent: input.fullClassThresholdPercent,
    noShowPenaltyIdr: input.noShowPenaltyIdr,
    rates: input.rates ?? [],
    active: input.active,
    updatedAt: now,
  };
  deps.incentiveSchemes.save(scheme);
  recordAudit(deps, {
    entityType: 'INCENTIVE_SCHEME',
    entityId: scheme.id,
    action: existing ? 'UPDATE' : 'CREATE',
    previousValue: existing ? JSON.stringify(existing) : null,
    newValue: JSON.stringify(scheme),
    actor: args.actor,
  });
  return ok(scheme);
}

// ── Statements ────────────────────────────────────────────────────────────────

export interface CoachStatementResult {
  coach: Coach;
  periodMonth: string;
  period: StatementPeriod;
  scheme: IncentiveScheme;
  statement: CoachStatement;
  /** The live (non-VOID) payout for this coach + period, if one exists. */
  payout: IncentivePayout | null;
}

function bookingsBySession(
  deps: UseCaseDeps,
  sessionIds: readonly string[],
): Record<string, Booking[]> {
  const out: Record<string, Booking[]> = {};
  for (const id of sessionIds) out[id] = deps.bookings.forSession(id);
  return out;
}

function classTypesById(deps: UseCaseDeps): Record<string, { name: string }> {
  return Object.fromEntries(deps.classTypes.all().map((t) => [t.id, { name: t.name }]));
}

/** The live payout for a coach + period (VOID ones don't block a new one). */
export function livePayoutFor(
  deps: UseCaseDeps,
  coachId: string,
  periodMonth: string,
): IncentivePayout | null {
  return (
    deps.incentivePayouts
      .all()
      .find(
        (p) =>
          p.coachId === coachId &&
          p.status !== 'VOID' &&
          periodMonthOf(p.periodStart) === periodMonth,
      ) ?? null
  );
}

function statementFor(
  deps: UseCaseDeps,
  coach: Coach,
  periodMonth: string,
  period: StatementPeriod,
  branchId: string | null,
): Result<CoachStatementResult, AppError> {
  const scheme = schemeFor(deps, coach.id);
  if (!scheme.ok) return scheme;
  const sessions = deps.sessions
    .all()
    .filter((s) => s.coachId === coach.id && (!branchId || s.branchId === branchId));
  const statement = computeCoachStatement({
    coach,
    sessions,
    bookingsBySession: bookingsBySession(
      deps,
      sessions.map((s) => s.id),
    ),
    classTypesById: classTypesById(deps),
    scheme: scheme.value,
    period,
  });
  return ok({
    coach,
    periodMonth,
    period,
    scheme: scheme.value,
    statement,
    payout: livePayoutFor(deps, coach.id, periodMonth),
  });
}

/** One statement per active coach for the month (optionally one branch). */
export function buildStatements(
  deps: UseCaseDeps,
  args: { periodMonth: string; branchId?: string | null },
): Result<CoachStatementResult[], AppError> {
  const period = periodOf(args.periodMonth);
  if (!period.ok) return period;
  const branchId = args.branchId ?? null;
  const results: CoachStatementResult[] = [];
  for (const coach of deps.coaches.all()) {
    if (coach.status !== 'ACTIVE') continue;
    if (branchId && coach.branchId !== branchId) continue;
    const res = statementFor(deps, coach, args.periodMonth, period.value, branchId);
    if (!res.ok) return res;
    results.push(res.value);
  }
  return ok(results.sort((a, b) => b.statement.totals.totalIdr - a.statement.totals.totalIdr));
}

// ── Payouts ───────────────────────────────────────────────────────────────────

/**
 * Snapshot a coach's statement for the month as a DRAFT payout. Only one live
 * payout per coach + period; void the old one to re-issue.
 */
export function createPayout(
  deps: UseCaseDeps,
  args: { coachId: string; periodMonth: string; actor: Actor },
): Result<IncentivePayout, AppError> {
  const coach = deps.coaches.byId(args.coachId);
  if (!coach) return err(appError('NOT_FOUND', 'Coach not found.', 404));
  const period = periodOf(args.periodMonth);
  if (!period.ok) return period;
  const existing = livePayoutFor(deps, coach.id, args.periodMonth);
  if (existing)
    return err(
      appError(
        'PAYOUT_EXISTS',
        `A ${existing.status} payout already exists for ${coach.name} in ${args.periodMonth}. Void it to re-issue.`,
        409,
      ),
    );
  const res = statementFor(deps, coach, args.periodMonth, period.value, null);
  if (!res.ok) return res;
  if (res.value.statement.totals.sessions === 0)
    return err(
      appError('NO_SESSIONS', `${coach.name} has no completed sessions in ${args.periodMonth}.`),
    );
  const now = deps.clock.now();
  const payout: IncentivePayout = {
    id: deps.ids.next('pyo'),
    coachId: coach.id,
    branchId: coach.branchId,
    periodStart: period.value.start,
    periodEnd: period.value.end,
    statement: res.value.statement,
    status: 'DRAFT',
    createdBy: args.actor.id,
    approvedBy: null,
    approvedAt: null,
    paidAt: null,
    paymentReference: null,
    note: null,
    createdAt: now,
    updatedAt: now,
  };
  deps.incentivePayouts.save(payout);
  recordAudit(deps, {
    entityType: 'INCENTIVE_PAYOUT',
    entityId: payout.id,
    action: 'CREATE',
    newValue: `DRAFT · ${payout.statement.totals.totalIdr}`,
    actor: args.actor,
  });
  return ok(payout);
}

export function transitionPayout(
  deps: UseCaseDeps,
  args: {
    id: string;
    action: PayoutAction;
    actor: Actor;
    paymentReference?: string | null;
    note?: string | null;
  },
): Result<IncentivePayout, AppError> {
  const payout = deps.incentivePayouts.byId(args.id);
  if (!payout) return err(appError('NOT_FOUND', 'Payout not found.', 404));
  const target = PAYOUT_ACTION_TARGET[args.action];
  if (!canTransition(PAYOUT_TRANSITIONS, payout.status, target))
    return err(
      appError('INVALID_TRANSITION', `Cannot ${args.action} a ${payout.status} payout.`, 409),
    );
  const reference = args.paymentReference?.trim() ?? '';
  const note = args.note?.trim() ?? '';
  if (args.action === 'pay' && !reference)
    return err(appError('PAYMENT_REFERENCE_REQUIRED', 'Enter the transfer / payment reference.'));
  if (args.action === 'void' && !note)
    return err(appError('REASON_REQUIRED', 'Give a reason for voiding this payout.'));

  const now = deps.clock.now();
  const previous = payout.status;
  payout.status = target;
  payout.updatedAt = now;
  if (args.action === 'approve') {
    payout.approvedBy = args.actor.id;
    payout.approvedAt = now;
  } else if (args.action === 'pay') {
    payout.paidAt = now;
    payout.paymentReference = reference;
  }
  if (note) payout.note = note;
  deps.incentivePayouts.save(payout);
  recordAudit(deps, {
    entityType: 'INCENTIVE_PAYOUT',
    entityId: payout.id,
    action: args.action.toUpperCase(),
    previousValue: previous,
    newValue: payout.status,
    actor: args.actor,
    reason: note || (reference ? `ref ${reference}` : null),
  });
  return ok(payout);
}
