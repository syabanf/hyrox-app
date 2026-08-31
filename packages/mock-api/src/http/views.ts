import type { UseCaseDeps } from '@hyrox/application';
import { balanceOf, expiringCreditsFor } from '@hyrox/application';
import type {
  AccessLogView,
  BookingView,
  DashboardStatsView,
  MemberDetailView,
  MemberSummaryView,
  MeView,
  PaymentView,
  RosterEntryView,
  SessionView,
  VoucherView,
} from '@hyrox/contracts';
import type { AccessLog, Booking, ClassSession, Member, Payment, Voucher } from '@hyrox/domain';
import { msOf } from '@hyrox/domain';
import type { MockDb } from '../db';

export function sessionView(db: MockDb, session: ClassSession, memberId?: string): SessionView {
  const bookings = db.bookings.filter((b) => b.sessionId === session.id);
  const confirmed = bookings.filter(
    (b) => b.status === 'CONFIRMED' || b.status === 'CHECKED_IN',
  ).length;
  const waitlist = bookings.filter((b) => b.status === 'WAITLIST').length;
  const mine = memberId
    ? (bookings.find(
        (b) =>
          b.memberId === memberId &&
          ['PENDING', 'CONFIRMED', 'WAITLIST', 'CHECKED_IN'].includes(b.status),
      ) ?? null)
    : null;
  return {
    session,
    classTypeName: db.classTypes.find((t) => t.id === session.classTypeId)?.name ?? 'Class',
    coachName: db.coaches.find((c) => c.id === session.coachId)?.name ?? 'Coach',
    branchName: db.branches.find((b) => b.id === session.branchId)?.name ?? 'Branch',
    confirmedCount: confirmed,
    waitlistCount: waitlist,
    spotsLeft: Math.max(0, session.capacity - confirmed),
    myBooking: mine
      ? {
          id: mine.id,
          status: mine.status,
          waitlistPosition: mine.waitlistPosition,
          promotionOfferedAt: mine.promotionOfferedAt,
        }
      : null,
  };
}

export function bookingView(db: MockDb, booking: Booking): BookingView | null {
  const session = db.sessions.find((s) => s.id === booking.sessionId);
  if (!session) return null;
  return {
    booking,
    session,
    classTypeName: db.classTypes.find((t) => t.id === session.classTypeId)?.name ?? 'Class',
    coachName: db.coaches.find((c) => c.id === session.coachId)?.name ?? 'Coach',
    branchName: db.branches.find((b) => b.id === session.branchId)?.name ?? 'Branch',
  };
}

export function accessLogView(db: MockDb, log: AccessLog): AccessLogView {
  return {
    log,
    memberName: log.memberId
      ? (db.members.find((m) => m.id === log.memberId)?.fullName ?? null)
      : null,
    gateName: db.gates.find((g) => g.id === log.gateId)?.name ?? log.gateId,
    branchName: db.branches.find((b) => b.id === log.branchId)?.name ?? log.branchId,
  };
}

export function meView(db: MockDb, deps: UseCaseDeps, member: Member): MeView {
  const balance = balanceOf(deps, member.id);
  return {
    member,
    balance,
    expiringCredits: expiringCreditsFor(deps, member.id),
    lowBalance: balance < db.rules.lowBalanceThreshold,
    unreadNotifications: db.notifications.filter(
      (n) => n.memberId === member.id && n.readAt === null,
    ).length,
  };
}

export function memberSummaryView(db: MockDb, deps: UseCaseDeps, member: Member): MemberSummaryView {
  const visits = db.accessLogs.filter(
    (l) =>
      l.memberId === member.id &&
      (l.result === 'ALLOWED' || l.result === 'OFFLINE_ALLOWED' || l.result === 'SYNCED'),
  );
  const last = visits.sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))[0];
  return {
    member,
    balance: balanceOf(deps, member.id),
    totalVisits: visits.length,
    lastVisitAt: last?.createdAt ?? null,
  };
}

export function myPackagesView(db: MockDb, memberId: string) {
  const nowMs = Date.now();
  return db.lots
    .filter((l) => l.memberId === memberId && l.packageId)
    .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))
    .map((l) => {
      const pkg = db.packages.find((x) => x.id === l.packageId);
      const coverageIds = pkg?.applicableClassTypeIds ?? null;
      return {
        lotId: l.id,
        packageId: l.packageId!,
        name: pkg?.name ?? 'Credit package',
        credits: l.credits,
        purchasedAt: l.createdAt,
        expiresAt: l.expiresAt,
        active: msOf(l.expiresAt) > nowMs,
        coverageIds,
        coverageNames: coverageIds
          ? coverageIds.map((id) => db.classTypes.find((c) => c.id === id)?.name ?? id)
          : null,
      };
    });
}

export function memberDetailView(db: MockDb, deps: UseCaseDeps, member: Member): MemberDetailView {
  const summary = memberSummaryView(db, deps, member);
  const bookings = db.bookings
    .filter((b) => b.memberId === member.id)
    .map((b) => bookingView(db, b))
    .filter((v): v is BookingView => v !== null)
    .sort((a, b) => msOf(b.session.startsAt) - msOf(a.session.startsAt));
  const nowMs = Date.now();
  return {
    member,
    balance: summary.balance,
    expiringCredits: expiringCreditsFor(deps, member.id),
    totalVisits: summary.totalVisits,
    lastVisitAt: summary.lastVisitAt,
    upcomingBookings: bookings.filter(
      (b) => msOf(b.session.startsAt) > nowMs && ['CONFIRMED', 'WAITLIST'].includes(b.booking.status),
    ),
    entries: db.ledger
      .filter((e) => e.memberId === member.id)
      .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt)),
    lots: db.lots.filter((l) => l.memberId === member.id),
    packages: myPackagesView(db, member.id),
    bookings,
    visits: db.accessLogs
      .filter((l) => l.memberId === member.id)
      .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))
      .map((l) => accessLogView(db, l)),
    payments: db.payments
      .filter((p) => p.memberId === member.id)
      .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt)),
    audit: db.audit
      .filter((a) => a.entityId === member.id || db.ledger.some((e) => e.id === a.entityId && e.memberId === member.id))
      .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt)),
  };
}

export function paymentView(db: MockDb, payment: Payment): PaymentView {
  return {
    payment,
    memberName: db.members.find((m) => m.id === payment.memberId)?.fullName ?? payment.memberId,
    packageName: db.packages.find((p) => p.id === payment.packageId)?.name ?? payment.packageId,
  };
}

export function voucherView(db: MockDb, voucher: Voucher): VoucherView {
  return {
    voucher,
    redemptionCount: db.redemptions.filter((r) => r.voucherId === voucher.id).length,
  };
}

export function rosterView(db: MockDb, sessionId: string): RosterEntryView[] {
  return db.bookings
    .filter((b) => b.sessionId === sessionId)
    .map((booking) => {
      const member = db.members.find((m) => m.id === booking.memberId);
      return {
        booking,
        memberName: member?.fullName ?? booking.memberId,
        memberStatus: member?.status ?? 'ACTIVE',
      };
    })
    .sort((a, b) => {
      const order = ['CHECKED_IN', 'CONFIRMED', 'WAITLIST', 'COMPLETED', 'NO_SHOW', 'CANCELLED', 'PENDING'];
      return order.indexOf(a.booking.status) - order.indexOf(b.booking.status);
    });
}

const sameDay = (iso: string, ref: Date): boolean => {
  const d = new Date(iso);
  return (
    d.getFullYear() === ref.getFullYear() &&
    d.getMonth() === ref.getMonth() &&
    d.getDate() === ref.getDate()
  );
};

export function dashboardView(db: MockDb, deps: UseCaseDeps): DashboardStatsView {
  const today = new Date();
  const activeMembers = db.members.filter((m) => m.status === 'ACTIVE');
  const visitorsToday = db.accessLogs.filter(
    (l) =>
      sameDay(l.createdAt, today) &&
      (l.result === 'ALLOWED' || l.result === 'OFFLINE_ALLOWED' || l.result === 'SYNCED'),
  ).length;
  const todaySessions = db.sessions
    .filter((s) => sameDay(s.startsAt, today) && s.status !== 'DRAFT')
    .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt));
  const paidToday = db.payments.filter((p) => p.paidAt && sameDay(p.paidAt, today));
  const outstanding = activeMembers.reduce((sum, m) => sum + balanceOf(deps, m.id), 0);
  const expiring = activeMembers.reduce((sum, m) => sum + expiringCreditsFor(deps, m.id), 0);
  return {
    visitorsToday,
    classesToday: todaySessions.length,
    revenueTodayIdr: paidToday.reduce((s, p) => s + p.totalIdr, 0),
    topUpsTodayIdr: paidToday.reduce((s, p) => s + p.totalIdr, 0),
    outstandingCredits: outstanding,
    expiringCredits: expiring,
    activeMembers: activeMembers.length,
    todaySessions: todaySessions.map((s) => sessionView(db, s)),
  };
}
