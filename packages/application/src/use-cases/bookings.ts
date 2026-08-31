import type { Booking, ClassSession, CreditLedgerEntry, Result } from '@hyrox/domain';
import {
  ACTIVE_BOOKING_STATUSES,
  BOOKING_TRANSITIONS,
  SESSION_TRANSITIONS,
  canTransition,
  err,
  evaluateBookingEligibility,
  evaluateCancellation,
  msOf,
  nextWaitlistPosition,
  noShowPenalty,
  ok,
  packageCoversClass,
  pickWaitlistPromotion,
  transition,
} from '@hyrox/domain';
import type { Actor, AppError } from '../common';
import { appError, balanceOf, maybeNotifyLowBalance, notify, recordAudit, rulesFor } from '../common';
import type { UseCaseDeps } from '../ports';
import { sweepMemberExpiry } from './wallet';

function confirmedCount(deps: UseCaseDeps, sessionId: string): number {
  return deps.bookings
    .forSession(sessionId)
    .filter((b) => b.status === 'CONFIRMED' || b.status === 'CHECKED_IN').length;
}

function waitlistOf(deps: UseCaseDeps, sessionId: string): Booking[] {
  return deps.bookings.forSession(sessionId).filter((b) => b.status === 'WAITLIST');
}

function syncSessionFullness(deps: UseCaseDeps, session: ClassSession): void {
  const confirmed = confirmedCount(deps, session.id);
  if (session.status === 'PUBLISHED' && confirmed >= session.capacity) {
    session.status = 'FULL';
    deps.sessions.save(session);
  } else if (session.status === 'FULL' && confirmed < session.capacity) {
    session.status = 'PUBLISHED';
    deps.sessions.save(session);
  }
}

function classTypeName(deps: UseCaseDeps, session: ClassSession): string {
  return deps.classTypes.byId(session.classTypeId)?.name ?? 'Class';
}

export function bookSession(
  deps: UseCaseDeps,
  args: { memberId: string; sessionId: string; source: 'MEMBER' | 'ADMIN' },
): Result<{ booking: Booking; decision: 'CONFIRMED' | 'WAITLIST' }, AppError> {
  const member = deps.members.byId(args.memberId);
  if (!member) return err(appError('NOT_FOUND', 'Member not found.', 404));
  const session = deps.sessions.byId(args.sessionId);
  if (!session) return err(appError('NOT_FOUND', 'Session not found.', 404));

  sweepMemberExpiry(deps, args.memberId);
  const existing = deps.bookings
    .forMember(args.memberId)
    .some((b) => b.sessionId === session.id && ACTIVE_BOOKING_STATUSES.includes(b.status));

  const decision = evaluateBookingEligibility({
    member,
    session,
    balance: balanceOf(deps, args.memberId),
    confirmedCount: confirmedCount(deps, session.id),
    waitlistCount: waitlistOf(deps, session.id).length,
    hasExistingActiveBooking: existing,
    now: deps.clock.now(),
  });

  if (decision.kind === 'DENY') {
    return err(appError(decision.reason, bookingDenialMessage(decision.reason)));
  }

  // Package coverage: when the member holds unexpired package credits, at
  // least one of those packages must cover this class type. Members whose
  // credits came only from bonuses/adjustments are not restricted.
  const nowMs = new Date(deps.clock.now()).getTime();
  const packageLots = deps.ledger
    .lotsFor(args.memberId)
    .filter((l) => l.packageId !== null && new Date(l.expiresAt).getTime() > nowMs);
  if (packageLots.length > 0) {
    const covered = packageLots.some((l) => {
      const pkg = deps.packages.byId(l.packageId!);
      return pkg ? packageCoversClass(pkg, session.classTypeId) : false;
    });
    if (!covered)
      return err(
        appError(
          'PACKAGE_NOT_COVERED',
          'None of your packages cover this class — top up with a package that includes it.',
          422,
        ),
      );
  }

  const now = deps.clock.now();
  const booking: Booking = {
    id: deps.ids.next('bok'),
    memberId: args.memberId,
    sessionId: session.id,
    status: decision.kind === 'CONFIRM' ? 'CONFIRMED' : 'WAITLIST',
    waitlistPosition:
      decision.kind === 'WAITLIST' ? nextWaitlistPosition(waitlistOf(deps, session.id)) : null,
    source: args.source,
    createdAt: now,
    updatedAt: now,
    cancelledAt: null,
    checkedInAt: null,
    promotionOfferedAt: null,
  };
  deps.bookings.save(booking);
  syncSessionFullness(deps, session);

  const name = classTypeName(deps, session);
  if (booking.status === 'CONFIRMED') {
    notify(deps, args.memberId, 'BOOKING_CONFIRMED', 'Booking confirmed', `You're in — ${name}.`);
  } else {
    notify(
      deps,
      args.memberId,
      'BOOKING_CONFIRMED',
      'Added to waitlist',
      `${name} is full. You are #${booking.waitlistPosition} on the waitlist.`,
    );
  }
  return ok({ booking, decision: booking.status === 'CONFIRMED' ? 'CONFIRMED' : 'WAITLIST' });
}

function bookingDenialMessage(reason: string): string {
  const messages: Record<string, string> = {
    MEMBER_NOT_ACTIVE: 'Membership is not active.',
    SESSION_NOT_BOOKABLE: 'This class cannot be booked.',
    BOOKING_NOT_OPEN_YET: 'Booking has not opened yet for this class.',
    BOOKING_WINDOW_CLOSED: 'Booking is closed for this class.',
    ALREADY_BOOKED: 'You already have a booking for this class.',
    INSUFFICIENT_CREDITS: 'Not enough credits — top up first.',
  };
  return messages[reason] ?? 'Booking not possible.';
}

export interface CancelOutcome {
  booking: Booking;
  outcome: 'RELEASED' | 'LATE';
  penaltyCredits: number;
  promotedBooking: Booking | null;
}

export function cancelBooking(
  deps: UseCaseDeps,
  args: { bookingId: string; actor: Actor | null },
): Result<CancelOutcome, AppError> {
  const booking = deps.bookings.byId(args.bookingId);
  if (!booking) return err(appError('NOT_FOUND', 'Booking not found.', 404));
  const session = deps.sessions.byId(booking.sessionId);
  if (!session) return err(appError('NOT_FOUND', 'Session not found.', 404));
  if (!canTransition(BOOKING_TRANSITIONS, booking.status, 'CANCELLED')) {
    return err(appError('INVALID_TRANSITION', `A ${booking.status} booking cannot be cancelled.`));
  }

  const wasConfirmed = booking.status === 'CONFIRMED';
  const now = deps.clock.now();
  const rules = rulesFor(deps, session.branchId);

  // Leaving a waitlist is always free; only a confirmed slot has a deadline.
  const evaluation = wasConfirmed
    ? evaluateCancellation({ session, rules, now })
    : ({ kind: 'RELEASED' } as const);

  booking.status = 'CANCELLED';
  booking.cancelledAt = now;
  booking.updatedAt = now;
  deps.bookings.save(booking);

  let penaltyCredits = 0;
  if (evaluation.kind === 'LATE' && evaluation.penaltyCredits > 0) {
    penaltyCredits = evaluation.penaltyCredits;
    const before = balanceOf(deps, booking.memberId);
    const entry: CreditLedgerEntry = {
      id: deps.ids.next('led'),
      memberId: booking.memberId,
      type: 'VISIT_DEDUCTION',
      amount: -penaltyCredits,
      description: `Late cancellation — ${classTypeName(deps, session)}`,
      sourceType: 'BOOKING',
      sourceId: booking.id,
      reversesEntryId: null,
      actorId: args.actor?.id ?? null,
      reason: 'Late cancellation policy',
      createdAt: now,
    };
    deps.ledger.append(entry);
    maybeNotifyLowBalance(deps, booking.memberId, before, before - penaltyCredits);
  }

  // A freed confirmed slot promotes the waitlist. Policy-controlled: either
  // auto-promote, or offer the spot and let the member confirm it themselves.
  let promotedBooking: Booking | null = null;
  if (wasConfirmed) {
    const next = pickWaitlistPromotion(waitlistOf(deps, session.id));
    if (next) {
      if (rules.waitlistAutoPromote) {
        next.status = 'CONFIRMED';
        next.waitlistPosition = null;
        next.updatedAt = now;
        deps.bookings.save(next);
        promotedBooking = next;
        notify(
          deps,
          next.memberId,
          'WAITLIST_PROMOTED',
          'You got a spot!',
          `A slot opened up — you're confirmed for ${classTypeName(deps, session)}.`,
        );
      } else if (next.promotionOfferedAt === null) {
        next.promotionOfferedAt = now;
        next.updatedAt = now;
        deps.bookings.save(next);
        notify(
          deps,
          next.memberId,
          'WAITLIST_PROMOTED',
          'A spot opened up',
          `Confirm your spot in ${classTypeName(deps, session)} before someone else takes it.`,
        );
      }
    }
  }
  syncSessionFullness(deps, session);

  if (args.actor) {
    recordAudit(deps, {
      entityType: 'BOOKING',
      entityId: booking.id,
      action: 'CANCELLED',
      previousValue: wasConfirmed ? 'CONFIRMED' : 'WAITLIST',
      newValue: 'CANCELLED',
      actor: args.actor,
    });
  }

  return ok({
    booking,
    outcome: evaluation.kind === 'LATE' ? 'LATE' : 'RELEASED',
    penaltyCredits,
    promotedBooking,
  });
}

/** Member accepts an offered waitlist promotion (manual-confirm policy). */
export function confirmPromotion(
  deps: UseCaseDeps,
  args: { bookingId: string; memberId: string },
): Result<Booking, AppError> {
  const booking = deps.bookings.byId(args.bookingId);
  if (!booking) return err(appError('NOT_FOUND', 'Booking not found.', 404));
  if (booking.memberId !== args.memberId)
    return err(appError('FORBIDDEN', 'Not your booking.', 403));
  if (booking.status !== 'WAITLIST' || booking.promotionOfferedAt === null)
    return err(appError('NO_OFFER', 'There is no open spot offer for this booking.'));
  const session = deps.sessions.byId(booking.sessionId);
  if (!session) return err(appError('NOT_FOUND', 'Session not found.', 404));
  if (confirmedCount(deps, session.id) >= session.capacity) {
    booking.promotionOfferedAt = null;
    deps.bookings.save(booking);
    return err(appError('SLOT_TAKEN', 'Sorry — that spot has been filled again.'));
  }
  const now = deps.clock.now();
  booking.status = 'CONFIRMED';
  booking.waitlistPosition = null;
  booking.promotionOfferedAt = null;
  booking.updatedAt = now;
  deps.bookings.save(booking);
  syncSessionFullness(deps, session);
  notify(
    deps,
    booking.memberId,
    'BOOKING_CONFIRMED',
    'Spot confirmed',
    `You're in — ${classTypeName(deps, session)}.`,
  );
  return ok(booking);
}

export function markNoShow(
  deps: UseCaseDeps,
  args: { bookingId: string; actor: Actor },
): Result<Booking, AppError> {
  const booking = deps.bookings.byId(args.bookingId);
  if (!booking) return err(appError('NOT_FOUND', 'Booking not found.', 404));
  const res = transition(BOOKING_TRANSITIONS, booking.status, 'NO_SHOW');
  if (!res.ok)
    return err(appError('INVALID_TRANSITION', `A ${booking.status} booking cannot be a no-show.`));
  const session = deps.sessions.byId(booking.sessionId);
  const now = deps.clock.now();
  booking.status = 'NO_SHOW';
  booking.updatedAt = now;
  deps.bookings.save(booking);

  if (session) {
    const penalty = noShowPenalty(session, rulesFor(deps, session.branchId));
    if (penalty > 0) {
      deps.ledger.append({
        id: deps.ids.next('led'),
        memberId: booking.memberId,
        type: 'VISIT_DEDUCTION',
        amount: -penalty,
        description: `No-show — ${classTypeName(deps, session)}`,
        sourceType: 'BOOKING',
        sourceId: booking.id,
        reversesEntryId: null,
        actorId: args.actor.id,
        reason: 'No-show policy',
        createdAt: now,
      });
    }
  }
  recordAudit(deps, {
    entityType: 'BOOKING',
    entityId: booking.id,
    action: 'NO_SHOW',
    actor: args.actor,
  });
  return ok(booking);
}

/** Front-desk assisted check-in (no gate scan). */
export function manualCheckIn(
  deps: UseCaseDeps,
  args: { bookingId: string; actor: Actor },
): Result<Booking, AppError> {
  const booking = deps.bookings.byId(args.bookingId);
  if (!booking) return err(appError('NOT_FOUND', 'Booking not found.', 404));
  const res = transition(BOOKING_TRANSITIONS, booking.status, 'CHECKED_IN');
  if (!res.ok)
    return err(appError('INVALID_TRANSITION', `A ${booking.status} booking cannot check in.`));
  const session = deps.sessions.byId(booking.sessionId);
  if (!session) return err(appError('NOT_FOUND', 'Session not found.', 404));

  sweepMemberExpiry(deps, booking.memberId);
  const before = balanceOf(deps, booking.memberId);
  if (before < session.creditCost)
    return err(appError('INSUFFICIENT_CREDITS', 'Member has insufficient credits.'));

  const now = deps.clock.now();
  booking.status = 'CHECKED_IN';
  booking.checkedInAt = now;
  booking.updatedAt = now;
  deps.bookings.save(booking);
  deps.ledger.append({
    id: deps.ids.next('led'),
    memberId: booking.memberId,
    type: 'VISIT_DEDUCTION',
    amount: -session.creditCost,
    description: `Class check-in (front desk) — ${classTypeName(deps, session)}`,
    sourceType: 'BOOKING',
    sourceId: booking.id,
    reversesEntryId: null,
    actorId: args.actor.id,
    reason: null,
    createdAt: now,
  });
  maybeNotifyLowBalance(deps, booking.memberId, before, before - session.creditCost);
  return ok(booking);
}

/** Completing a session settles its roster: checked-in → completed, confirmed → no-show. */
export function completeSession(
  deps: UseCaseDeps,
  args: { sessionId: string; actor: Actor },
): Result<ClassSession, AppError> {
  const session = deps.sessions.byId(args.sessionId);
  if (!session) return err(appError('NOT_FOUND', 'Session not found.', 404));
  const res = transition(SESSION_TRANSITIONS, session.status, 'COMPLETED');
  if (!res.ok)
    return err(appError('INVALID_TRANSITION', `A ${session.status} session cannot complete.`));
  const now = deps.clock.now();
  session.status = 'COMPLETED';
  deps.sessions.save(session);

  for (const booking of deps.bookings.forSession(session.id)) {
    if (booking.status === 'CHECKED_IN') {
      booking.status = 'COMPLETED';
      booking.updatedAt = now;
      deps.bookings.save(booking);
    } else if (booking.status === 'CONFIRMED') {
      markNoShow(deps, { bookingId: booking.id, actor: args.actor });
    } else if (booking.status === 'WAITLIST') {
      booking.status = 'CANCELLED';
      booking.cancelledAt = now;
      booking.updatedAt = now;
      deps.bookings.save(booking);
    }
  }
  recordAudit(deps, {
    entityType: 'SESSION',
    entityId: session.id,
    action: 'COMPLETED',
    actor: args.actor,
  });
  return ok(session);
}

export function cancelSession(
  deps: UseCaseDeps,
  args: { sessionId: string; actor: Actor; reason?: string },
): Result<ClassSession, AppError> {
  const session = deps.sessions.byId(args.sessionId);
  if (!session) return err(appError('NOT_FOUND', 'Session not found.', 404));
  const res = transition(SESSION_TRANSITIONS, session.status, 'CANCELLED');
  if (!res.ok)
    return err(appError('INVALID_TRANSITION', `A ${session.status} session cannot be cancelled.`));
  const now = deps.clock.now();
  session.status = 'CANCELLED';
  deps.sessions.save(session);

  // Booked members are released without penalty and told about it.
  for (const booking of deps.bookings.forSession(session.id)) {
    if (booking.status === 'CONFIRMED' || booking.status === 'WAITLIST') {
      booking.status = 'CANCELLED';
      booking.cancelledAt = now;
      booking.updatedAt = now;
      deps.bookings.save(booking);
      notify(
        deps,
        booking.memberId,
        'SESSION_CHANGED',
        'Class cancelled',
        `${classTypeName(deps, session)} was cancelled by the studio.`,
      );
    }
  }
  recordAudit(deps, {
    entityType: 'SESSION',
    entityId: session.id,
    action: 'CANCELLED',
    actor: args.actor,
    reason: args.reason,
  });
  return ok(session);
}

/** Sort helper shared by session listings. */
export function bySessionStart(a: ClassSession, b: ClassSession): number {
  return msOf(a.startsAt) - msOf(b.startsAt);
}
