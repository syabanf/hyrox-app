import type {
  AccessLog,
  Booking,
  Gate,
  GateScanEvaluation,
  Member,
  QrToken,
  Result,
} from '@hyrox/domain';
import {
  checkQrToken,
  err,
  evaluateGateScan,
  issueQrToken,
  msOf,
  ok,
} from '@hyrox/domain';
import type { AppError } from '../common';
import { appError, balanceOf, maybeNotifyLowBalance, notify, rulesFor } from '../common';
import type { UseCaseDeps } from '../ports';
import { sweepMemberExpiry } from './wallet';

/**
 * Manual reconciliation of an OFFLINE CONFLICT row: approve applies the missed
 * deduction and marks the row SYNCED; reject marks it DENIED. Audited.
 */
export function resolveOfflineConflict(
  deps: UseCaseDeps,
  args: { logId: string; action: 'APPROVE' | 'REJECT'; actor: { id: string; name: string }; reason: string },
): Result<AccessLog, AppError> {
  const log = deps.accessLogs.all().find((l) => l.id === args.logId);
  if (!log) return err(appError('NOT_FOUND', 'Access log not found.', 404));
  if (log.result !== 'CONFLICT')
    return err(appError('NOT_A_CONFLICT', 'Only CONFLICT rows can be resolved.'));
  const now = deps.clock.now();
  if (args.action === 'APPROVE') {
    log.result = 'SYNCED';
    if (log.memberId) {
      const cost = rulesFor(deps, log.branchId).openGymCreditCost;
      log.creditDelta = -cost;
      deps.ledger.append({
        id: deps.ids.next('led'),
        memberId: log.memberId,
        type: 'VISIT_DEDUCTION',
        amount: -cost,
        description: 'Offline entry reconciled',
        sourceType: 'ACCESS',
        sourceId: log.id,
        reversesEntryId: null,
        actorId: args.actor.id,
        reason: args.reason,
        createdAt: now,
      });
    }
  } else {
    log.result = 'DENIED';
  }
  deps.audit.append({
    id: deps.ids.next('aud'),
    entityType: 'ACCESS_LOG',
    entityId: log.id,
    action: `OFFLINE_${args.action}`,
    previousValue: 'CONFLICT',
    newValue: log.result,
    actorId: args.actor.id,
    actorName: args.actor.name,
    reason: args.reason,
    createdAt: now,
  });
  return ok(log);
}

export function issueQr(deps: UseCaseDeps, memberId: string): Result<QrToken, AppError> {
  const member = deps.members.byId(memberId);
  if (!member) return err(appError('NOT_FOUND', 'Member not found.', 404));
  const token = issueQrToken({
    memberId,
    now: deps.clock.now(),
    ttlSeconds: rulesFor(deps, member.preferredBranchId).qrTtlSeconds,
    nonce: deps.ids.next('nonce'),
  });
  deps.qrTokens.save(token);
  return ok(token);
}

/** The member's CONFIRMED booking happening around now at this branch, if any. */
function findCandidateBooking(
  deps: UseCaseDeps,
  memberId: string,
  branchId: string,
): (Booking & { creditCost: number }) | null {
  const now = msOf(deps.clock.now());
  const windowMs = 60 * 60_000; // may check in up to 60 min before start
  const candidates = deps.bookings
    .forMember(memberId)
    .filter((b) => b.status === 'CONFIRMED')
    .map((b) => ({ booking: b, session: deps.sessions.byId(b.sessionId) }))
    .filter(
      (x): x is { booking: Booking; session: NonNullable<typeof x.session> } =>
        x.session !== null &&
        x.session.branchId === branchId &&
        (x.session.status === 'PUBLISHED' || x.session.status === 'FULL') &&
        msOf(x.session.startsAt) - windowMs <= now &&
        now <= msOf(x.session.endsAt),
    )
    .sort((a, b) => msOf(a.session.startsAt) - msOf(b.session.startsAt));
  const first = candidates[0];
  return first ? { ...first.booking, creditCost: first.session.creditCost } : null;
}

export interface ScanOutcome {
  evaluation: GateScanEvaluation;
  accessLog: AccessLog;
  gate: Gate;
  member: Member | null;
  remainingCredits: number | null;
}

/**
 * The atomic gate transaction: evaluate the pipeline, then apply every effect
 * (consume token, deduct credits, check in booking, write the access log) in
 * one synchronous block — the gate never opens without its deduction.
 */
export function processGateScan(
  deps: UseCaseDeps,
  args: { gateId: string; qrToken?: string | null; memberId?: string | null },
): Result<ScanOutcome, AppError> {
  const gate = deps.gates.byId(args.gateId);
  if (!gate) return err(appError('NOT_FOUND', 'Gate not found.', 404));
  const now = deps.clock.now();

  // Simulator mode: scanning "as a member" issues a fresh token server-side,
  // so the flow through the pipeline is identical to hardware.
  let rawToken = args.qrToken ?? null;
  if (!rawToken && args.memberId) {
    const issued = issueQr(deps, args.memberId);
    if (!issued.ok) return issued;
    rawToken = issued.value.token;
  }

  const stored = rawToken ? deps.qrTokens.byToken(rawToken) : null;
  const tokenCheck = checkQrToken(stored, now);
  const member = stored ? deps.members.byId(stored.memberId) : null;

  if (member) sweepMemberExpiry(deps, member.id);
  const balanceBefore = member ? balanceOf(deps, member.id) : 0;

  const evaluation = evaluateGateScan({
    tokenCheck,
    member,
    balance: balanceBefore,
    lastAllowedEntryAt: member ? deps.accessLogs.lastAllowedAt(member.id) : null,
    candidateBooking: member ? findCandidateBooking(deps, member.id, gate.branchId) : null,
    rules: rulesFor(deps, gate.branchId),
    now,
  });

  // Apply effects.
  let creditDelta = 0;
  let bookingId: string | null = null;
  const logId = deps.ids.next('acc');
  for (const effect of evaluation.effects) {
    if (effect.kind === 'CONSUME_TOKEN') {
      const token = deps.qrTokens.byToken(effect.token);
      if (token) {
        token.consumedAt = now;
        deps.qrTokens.save(token);
      }
    } else if (effect.kind === 'DEDUCT_CREDITS' && member) {
      creditDelta = -effect.amount;
      deps.ledger.append({
        id: deps.ids.next('led'),
        memberId: member.id,
        type: 'VISIT_DEDUCTION',
        amount: -effect.amount,
        description: `${effect.description} — ${gate.name}`,
        sourceType: 'ACCESS',
        sourceId: logId,
        reversesEntryId: null,
        actorId: null,
        reason: null,
        createdAt: now,
      });
    } else if (effect.kind === 'CHECK_IN_BOOKING') {
      const booking = deps.bookings.byId(effect.bookingId);
      if (booking) {
        booking.status = 'CHECKED_IN';
        booking.checkedInAt = now;
        booking.updatedAt = now;
        deps.bookings.save(booking);
        bookingId = booking.id;
      }
    }
  }

  const accessLog: AccessLog = {
    id: logId,
    memberId: member?.id ?? null,
    gateId: gate.id,
    branchId: gate.branchId,
    result: evaluation.decision === 'ALLOWED' ? 'ALLOWED' : 'DENIED',
    reasonCode: evaluation.reason,
    creditDelta,
    mode: 'ONLINE',
    bookingId,
    createdAt: now,
  };
  deps.accessLogs.append(accessLog);

  let remainingCredits: number | null = null;
  if (member) {
    remainingCredits = balanceOf(deps, member.id);
    if (evaluation.decision === 'ALLOWED' && creditDelta !== 0) {
      notify(
        deps,
        member.id,
        'VISIT_LOGGED',
        'Welcome in!',
        `Checked in at ${gate.name}. ${remainingCredits} credit${remainingCredits === 1 ? '' : 's'} left.`,
      );
      maybeNotifyLowBalance(deps, member.id, balanceBefore, remainingCredits);
    }
  }

  return ok({ evaluation, accessLog, gate, member, remainingCredits });
}
