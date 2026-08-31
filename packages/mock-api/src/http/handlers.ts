import {
  adjustCredits,
  balanceOf,
  bookSession,
  cancelBooking,
  cancelSession,
  completeSession,
  confirmPayment,
  expiringCreditsFor,
  issueQr,
  manualCheckIn,
  markNoShow,
  processGateScan,
  quoteVoucher,
  refundPayment,
  registerMember,
  reverseEntry,
  runExpirySweep,
  sendCampaign,
  setMemberStatus,
  sweepMemberExpiry,
  topUpWallet,
  updateProfile,
  updateRules,
  walletSnapshot,
} from '@hyrox/application';
import type { UseCaseDeps, AppError } from '@hyrox/application';
import {
  AdjustCreditsSchema,
  AdminBookSchema,
  AdminLoginSchema,
  CreateBranchSchema,
  CreateMemberAdminSchema,
  CreateSessionSchema,
  GateScanSchema,
  OtpRequestSchema,
  OtpVerifySchema,
  RefundPaymentSchema,
  RegisterMemberSchema,
  ResolveConflictSchema,
  ReverseEntrySchema,
  SegmentPreviewSchema,
  TopUpRequestSchema,
  UpdateBranchSchema,
  UpdateMemberAdminSchema,
  UpdateProfileSchema,
  UpdateRulesSchema,
  UpdateSessionSchema,
  UpsertAdminUserSchema,
  UpdateExerciseSchema,
  UpsertCampaignSchema,
  UpsertChallengeSchema,
  UpsertClassTypeSchema,
  UpsertCoachSchema,
  UpsertGateSchema,
  UpsertPackageSchema,
  UpsertRaceEventSchema,
  UpsertVoucherSchema,
  ValidateVoucherSchema,
  VoucherStatusActionSchema,
} from '@hyrox/contracts';
import type { QrView, ScanResultView } from '@hyrox/contracts';
import type { Payment, SessionStatus, VoucherStatus } from '@hyrox/domain';
import {
  ROLE_PERMISSIONS,
  SESSION_TRANSITIONS,
  VOUCHER_TRANSITIONS,
  canTransition,
  challengeProgressKm,
  msOf,
  qrSecondsRemaining,
} from '@hyrox/domain';
import { HttpResponse, http, type HttpHandler } from 'msw';
import {
  confirmPromotion,
  generateBookingReminders,
  resolveOfflineConflict,
  segmentMembers,
} from '@hyrox/application';
import type { MockDb } from '../db';
import { createAthleteHandlers } from './athlete';
import { createWorkoutRaceHandlers } from './workout-race';
import {
  jsonError,
  memberFromRequest,
  parseBody,
  parsePatch,
  requireAdmin,
  requireMember,
} from './helpers';
import {
  accessLogView,
  bookingView,
  dashboardView,
  memberDetailView,
  memberSummaryView,
  meView,
  myPackagesView,
  paymentView,
  rosterView,
  sessionView,
  voucherView,
} from './views';

export interface MockApiState {
  db: MockDb;
  deps: UseCaseDeps;
}

const fromAppError = (error: AppError) => jsonError(error.status, error.code, error.message);

const param = (params: Record<string, string | readonly string[] | undefined>, key: string): string =>
  String(params[key] ?? '');

/** Splice an entity out of a db collection in place (persisted by the snapshot loop). */
const removeById = <T extends { id: string }>(arr: T[], id: string): boolean => {
  const idx = arr.findIndex((item) => item.id === id);
  if (idx < 0) return false;
  arr.splice(idx, 1);
  return true;
};

export function createHandlers(state: MockApiState, onReset: () => void): HttpHandler[] {
  // `state` is a mutable holder so devReset can swap the db behind every closure.
  const db = () => state.db;
  const deps = () => state.deps;

  return [
    // ── Auth: member OTP + registration ─────────────────────────────────────
    http.post('*/api/auth/otp/request', async ({ request }) => {
      const body = await parseBody(request, OtpRequestSchema);
      if (!body.ok) return body.response;
      const challengeId = deps().ids.next('otp');
      db().otpChallenges[challengeId] = body.data.identifier;
      const member = deps().members.byIdentifier(body.data.identifier);
      return HttpResponse.json({
        challengeId,
        memberExists: member !== null,
        hint: 'Demo mode: any 6-digit code works, try 123456.',
      });
    }),

    http.post('*/api/auth/otp/verify', async ({ request }) => {
      const body = await parseBody(request, OtpVerifySchema);
      if (!body.ok) return body.response;
      const identifier = db().otpChallenges[body.data.challengeId];
      if (!identifier) return jsonError(400, 'CHALLENGE_NOT_FOUND', 'Request a new code.');
      const member = deps().members.byIdentifier(identifier);
      if (!member)
        return jsonError(404, 'MEMBER_NOT_FOUND', 'No member with that email or phone — register first.');
      delete db().otpChallenges[body.data.challengeId];
      return HttpResponse.json({ token: `member:${member.id}`, member });
    }),

    http.post('*/api/auth/register', async ({ request }) => {
      const body = await parseBody(request, RegisterMemberSchema);
      if (!body.ok) return body.response;
      const res = registerMember(deps(), body.data);
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json({ token: `member:${res.value.id}`, member: res.value }, { status: 201 });
    }),

    // ── Auth: admin ─────────────────────────────────────────────────────────
    http.get('*/api/admin/auth/users', () => HttpResponse.json(db().adminUsers)),

    http.post('*/api/admin/auth/login', async ({ request }) => {
      const body = await parseBody(request, AdminLoginSchema);
      if (!body.ok) return body.response;
      const user = deps().adminUsers.byId(body.data.userId);
      if (!user) return jsonError(404, 'USER_NOT_FOUND', 'Unknown admin user.');
      return HttpResponse.json({
        token: `admin:${user.id}`,
        user,
        permissions: ROLE_PERMISSIONS[user.role],
      });
    }),

    // ── Member: me / profile / wallet ───────────────────────────────────────
    http.get('*/api/me', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      sweepMemberExpiry(deps(), auth.value.id);
      return HttpResponse.json(meView(db(), deps(), auth.value));
    }),

    http.patch('*/api/me', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateProfileSchema);
      if (!body.ok) return body.response;
      const res = updateProfile(deps(), auth.value.id, body.data);
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // Member home feed: announcements (sent broadcasts), promos (live
    // vouchers), today's bookable classes, and the member's challenge progress.
    http.get('*/api/home', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const now = deps().clock.now();

      const announcements = db()
        .campaigns.filter((c) => c.status === 'SENT')
        .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))
        .slice(0, 5)
        .map((c) => ({
          id: c.id,
          title: c.name,
          message: c.message,
          deepLink: c.deepLink,
          imageUrl: c.imageUrl,
          createdAt: c.createdAt,
        }));

      const promos = db()
        .vouchers.filter(
          (v) => v.status === 'ACTIVE' && msOf(v.startsAt) <= msOf(now) && msOf(v.endsAt) >= msOf(now),
        )
        .map((v) => ({
          voucherId: v.id,
          code: v.code,
          label: v.type === 'PERCENT' ? `${v.value}% OFF` : `Rp${v.value.toLocaleString('id-ID')} OFF`,
          description:
            v.applicablePackageIds === null
              ? 'Valid on every credit package'
              : `Valid on ${v.applicablePackageIds
                  .map((id) => db().packages.find((p) => p.id === id)?.name ?? id)
                  .join(' & ')}`,
          endsAt: v.endsAt,
          newMembersOnly: v.eligibleSegment === 'NEW_MEMBERS',
        }));

      const bookableOn = (day: Date) =>
        db()
          .sessions.filter((s) => {
            const d = new Date(s.startsAt);
            return (
              d.toDateString() === day.toDateString() &&
              (s.status === 'PUBLISHED' || s.status === 'FULL') &&
              msOf(s.endsAt) > msOf(now)
            );
          })
          .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt))
          .slice(0, 10)
          .map((s) => sessionView(db(), s, me));

      const today = new Date(now);
      let railDay: 'TODAY' | 'TOMORROW' = 'TODAY';
      let todaySessions = bookableOn(today);
      if (todaySessions.length === 0) {
        const tomorrow = new Date(today);
        tomorrow.setDate(tomorrow.getDate() + 1);
        todaySessions = bookableOn(tomorrow);
        railDay = 'TOMORROW';
      }

      const joined = db()
        .challenges.filter((c) => deps().athlete.challenges.isJoined(c.id, me))
        .map((c) => ({
          id: c.id,
          name: c.name,
          targetKm: c.targetKm,
          progressKm: challengeProgressKm(
            c,
            db().activities.filter((a) => a.memberId === me),
          ),
        }))
        .sort((a, b) => b.progressKm / b.targetKm - a.progressKm / a.targetKm);

      // Spotlight: the member's own next race, else the next joinable event.
      const myNextRace = deps()
        .races.userRaces.forMember(me)
        .filter((r) => r.status === 'TRAINING')
        .map((r) => ({ userRace: r, event: deps().races.events.byId(r.raceEventId)! }))
        .filter((x) => msOf(x.event.startsAt) > msOf(now))
        .sort((a, b) => msOf(a.event.startsAt) - msOf(b.event.startsAt))[0];
      const nextOpenEvent = db()
        .raceEvents.filter(
          (e) =>
            (e.status === 'REGISTRATION_OPEN' || e.status === 'ANNOUNCED') &&
            msOf(e.startsAt) > msOf(now),
        )
        .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt))[0];
      const spotlightEvent = myNextRace?.event ?? nextOpenEvent ?? null;
      const spotlightRace = spotlightEvent
        ? {
            raceEventId: spotlightEvent.id,
            name: spotlightEvent.name,
            city: spotlightEvent.city,
            imageUrl: spotlightEvent.imageUrl,
            startsAt: spotlightEvent.startsAt,
            daysToRace: Math.ceil((msOf(spotlightEvent.startsAt) - msOf(now)) / (24 * 3600_000)),
            joined: myNextRace !== undefined,
            goalSec: myNextRace?.userRace.goalSec ?? null,
          }
        : null;

      return HttpResponse.json({
        announcements,
        promos,
        railDay,
        todaySessions,
        challenge: joined[0] ?? null,
        spotlightRace,
      });
    }),

    // Announcement detail (a SENT campaign).
    http.get('*/api/announcements/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const c = db().campaigns.find((x) => x.id === param(params, 'id') && x.status === 'SENT');
      if (!c) return jsonError(404, 'NOT_FOUND', 'Announcement not found.');
      return HttpResponse.json({
        id: c.id,
        title: c.name,
        message: c.message,
        deepLink: c.deepLink,
        imageUrl: c.imageUrl,
        createdAt: c.createdAt,
      });
    }),

    // Promo detail (voucher by code).
    http.get('*/api/promos/:code', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const code = param(params, 'code').toUpperCase();
      const now = deps().clock.now();
      const v = db().vouchers.find((x) => x.code === code);
      if (!v) return jsonError(404, 'NOT_FOUND', 'Promo not found.');
      return HttpResponse.json({
        code: v.code,
        label:
          v.type === 'PERCENT' ? `${v.value}% OFF` : `Rp${v.value.toLocaleString('id-ID')} OFF`,
        live: v.status === 'ACTIVE' && msOf(v.startsAt) <= msOf(now) && msOf(v.endsAt) >= msOf(now),
        startsAt: v.startsAt,
        endsAt: v.endsAt,
        perMemberLimit: v.perMemberLimit,
        usageLimit: v.usageLimit,
        newMembersOnly: v.eligibleSegment === 'NEW_MEMBERS',
        packageNames: v.applicablePackageIds
          ? v.applicablePackageIds.map((id) => db().packages.find((x) => x.id === id)?.name ?? id)
          : null,
      });
    }),

    http.get('*/api/me/wallet', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const snapshot = walletSnapshot(deps(), auth.value.id);
      snapshot.entries = [...snapshot.entries].sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt));
      return HttpResponse.json({ ...snapshot, myPackages: myPackagesView(db(), auth.value.id) });
    }),

    http.post('*/api/me/topup', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, TopUpRequestSchema);
      if (!body.ok) return body.response;
      const res = topUpWallet(deps(), { memberId: auth.value.id, ...body.data });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    http.post('*/api/vouchers/validate', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, ValidateVoucherSchema);
      if (!body.ok) return body.response;
      const res = quoteVoucher(deps(), { memberId: auth.value.id, ...body.data });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // The member's own payment, for the mock checkout page (channel, totals).
    http.get('*/api/payments/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const payment = deps().payments.byId(param(params, 'id'));
      if (!payment || payment.memberId !== auth.value.id)
        return jsonError(404, 'NOT_FOUND', 'Payment not found.');
      const pkg = db().packages.find((x) => x.id === payment.packageId);
      return HttpResponse.json({ payment, packageName: pkg?.name ?? 'Credit package' });
    }),

    // Mock Xendit webhook — the paying member (from their payment page) or an
    // admin with payments.simulate can flip PENDING → PAID.
    http.post('*/api/payments/:id/simulate', ({ request, params }) => {
      const paymentId = param(params, 'id');
      const payment = deps().payments.byId(paymentId);
      if (!payment) return jsonError(404, 'NOT_FOUND', 'Payment not found.');
      const member = memberFromRequest(db(), request);
      const isOwner = member?.id === payment.memberId;
      if (!isOwner) {
        const admin = requireAdmin(db(), request, 'payments.simulate');
        if (!admin.ok) return admin.response;
      }
      const res = confirmPayment(deps(), paymentId);
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // ── Shared catalog ──────────────────────────────────────────────────────
    http.get('*/api/branches', () => HttpResponse.json(db().branches)),

    http.get('*/api/class-types', () =>
      HttpResponse.json(db().classTypes.filter((c) => c.active)),
    ),

    http.get('*/api/packages', () =>
      HttpResponse.json(
        db()
          .packages.filter((p) => p.status === 'ACTIVE')
          .map((p) => ({
            ...p,
            coverageNames: p.applicableClassTypeIds
              ? p.applicableClassTypeIds.map(
                  (id) => db().classTypes.find((c) => c.id === id)?.name ?? id,
                )
              : null,
          })),
      ),
    ),

    http.get('*/api/sessions', ({ request }) => {
      const url = new URL(request.url);
      const branchId = url.searchParams.get('branchId');
      const from = url.searchParams.get('from');
      const to = url.searchParams.get('to');
      const member = memberFromRequest(db(), request);
      const views = db()
        .sessions.filter((s) => s.status !== 'DRAFT')
        .filter((s) => !branchId || s.branchId === branchId)
        .filter((s) => !from || msOf(s.startsAt) >= msOf(from))
        .filter((s) => !to || msOf(s.startsAt) <= msOf(to))
        .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt))
        .map((s) => sessionView(db(), s, member?.id));
      return HttpResponse.json(views);
    }),

    http.get('*/api/sessions/:id', ({ request, params }) => {
      const session = deps().sessions.byId(param(params, 'id'));
      if (!session) return jsonError(404, 'NOT_FOUND', 'Session not found.');
      const member = memberFromRequest(db(), request);
      return HttpResponse.json(sessionView(db(), session, member?.id));
    }),

    http.post('*/api/sessions/:id/book', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = bookSession(deps(), {
        memberId: auth.value.id,
        sessionId: param(params, 'id'),
        source: 'MEMBER',
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    // Member accepts an offered waitlist promotion (manual-confirm policy).
    http.post('*/api/bookings/:id/confirm-spot', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = confirmPromotion(deps(), {
        bookingId: param(params, 'id'),
        memberId: auth.value.id,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    http.post('*/api/bookings/:id/cancel', ({ request, params }) => {
      const bookingId = param(params, 'id');
      const booking = deps().bookings.byId(bookingId);
      if (!booking) return jsonError(404, 'NOT_FOUND', 'Booking not found.');
      const member = memberFromRequest(db(), request);
      let actor = null;
      if (member?.id !== booking.memberId) {
        const admin = requireAdmin(db(), request, 'bookings.manage');
        if (!admin.ok) return admin.response;
        actor = { id: admin.value.id, name: admin.value.name };
      }
      const res = cancelBooking(deps(), { bookingId, actor });
      if (!res.ok) return fromAppError(res.error);
      const promoted = res.value.promotedBooking;
      return HttpResponse.json({
        booking: res.value.booking,
        outcome: res.value.outcome,
        penaltyCredits: res.value.penaltyCredits,
        promotedMemberName: promoted
          ? (deps().members.byId(promoted.memberId)?.fullName ?? null)
          : null,
      });
    }),

    http.get('*/api/me/bookings', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const views = deps()
        .bookings.forMember(auth.value.id)
        .map((b) => bookingView(db(), b))
        .filter((v) => v !== null)
        .sort((a, b) => msOf(b!.session.startsAt) - msOf(a!.session.startsAt));
      return HttpResponse.json(views);
    }),

    // ── QR + access ─────────────────────────────────────────────────────────
    http.post('*/api/me/qr', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = issueQr(deps(), auth.value.id);
      if (!res.ok) return fromAppError(res.error);
      const view: QrView = {
        token: res.value.token,
        issuedAt: res.value.issuedAt,
        expiresAt: res.value.expiresAt,
        ttlSeconds: qrSecondsRemaining(res.value, deps().clock.now()),
      };
      return HttpResponse.json(view);
    }),

    http.get('*/api/me/visits', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const views = deps()
        .accessLogs.forMember(auth.value.id)
        .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))
        .map((l) => accessLogView(db(), l));
      return HttpResponse.json(views);
    }),

    http.post('*/api/gates/:gateId/scan', async ({ request, params }) => {
      const body = await parseBody(request, GateScanSchema);
      if (!body.ok) return body.response;
      // Simulator mode (scan by memberId) needs authority: the member themself
      // (dev button) or an admin with access.simulate. A raw qrToken behaves
      // like gate hardware and needs no auth.
      if (body.data.memberId) {
        const member = memberFromRequest(db(), request);
        if (member?.id !== body.data.memberId) {
          const admin = requireAdmin(db(), request, 'access.simulate');
          if (!admin.ok) return admin.response;
        }
      }
      const res = processGateScan(deps(), {
        gateId: param(params, 'gateId'),
        qrToken: body.data.qrToken ?? null,
        memberId: body.data.memberId ?? null,
      });
      if (!res.ok) return fromAppError(res.error);
      const view: ScanResultView = {
        decision: res.value.evaluation.decision,
        reason: res.value.evaluation.reason,
        entryKind: res.value.evaluation.entryKind,
        memberName: res.value.member?.fullName ?? null,
        remainingCredits: res.value.remainingCredits,
        gateName: res.value.gate.name,
        accessLog: res.value.accessLog,
      };
      return HttpResponse.json(view);
    }),

    // ── Notifications ───────────────────────────────────────────────────────
    http.get('*/api/me/notifications', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      // Scheduled reminders, demo-style: materialize any due booking reminders
      // whenever the member checks their notifications.
      generateBookingReminders(deps(), auth.value.id);
      const list = deps()
        .notifications.forMember(auth.value.id)
        .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt));
      return HttpResponse.json(list);
    }),

    http.post('*/api/me/notifications/read-all', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      deps().notifications.markAllRead(auth.value.id, deps().clock.now());
      return HttpResponse.json({ ok: true });
    }),

    // ── Admin: members ──────────────────────────────────────────────────────
    http.get('*/api/admin/members', ({ request }) => {
      const auth = requireAdmin(db(), request, 'members.view');
      if (!auth.ok) return auth.response;
      const url = new URL(request.url);
      const query = url.searchParams.get('query')?.toLowerCase() ?? '';
      const status = url.searchParams.get('status');
      const views = db()
        .members.filter(
          (m) =>
            (!query ||
              m.fullName.toLowerCase().includes(query) ||
              m.email.toLowerCase().includes(query) ||
              m.phone.includes(query)) &&
            (!status || m.status === status),
        )
        .map((m) => memberSummaryView(db(), deps(), m));
      return HttpResponse.json(views);
    }),

    http.get('*/api/admin/members/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'members.view');
      if (!auth.ok) return auth.response;
      const member = deps().members.byId(param(params, 'id'));
      if (!member) return jsonError(404, 'NOT_FOUND', 'Member not found.');
      sweepMemberExpiry(deps(), member.id);
      return HttpResponse.json(memberDetailView(db(), deps(), member));
    }),

    http.patch('*/api/admin/members/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'members.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateMemberAdminSchema);
      if (!body.ok) return body.response;
      const member = deps().members.byId(param(params, 'id'));
      if (!member) return jsonError(404, 'NOT_FOUND', 'Member not found.');
      const actor = { id: auth.value.id, name: auth.value.name };
      if (body.data.status && body.data.status !== member.status) {
        const res = setMemberStatus(deps(), {
          memberId: member.id,
          status: body.data.status,
          actor,
          reason: body.data.reason,
        });
        if (!res.ok) return fromAppError(res.error);
      }
      if (body.data.notes !== undefined) member.notes = body.data.notes;
      if (body.data.preferredBranchId !== undefined)
        member.preferredBranchId = body.data.preferredBranchId;
      deps().members.save(member);
      return HttpResponse.json(memberDetailView(db(), deps(), member));
    }),

    http.post('*/api/admin/members/:id/adjust', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'members.adjust_credits');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, AdjustCreditsSchema);
      if (!body.ok) return body.response;
      const res = adjustCredits(deps(), {
        memberId: param(params, 'id'),
        amount: body.data.amount,
        reason: body.data.reason,
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    http.post('*/api/admin/ledger/:entryId/reverse', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'ledger.reverse');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, ReverseEntrySchema);
      if (!body.ok) return body.response;
      const res = reverseEntry(deps(), {
        entryId: param(params, 'entryId'),
        reason: body.data.reason,
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    // ── Admin: operations ───────────────────────────────────────────────────
    http.get('*/api/admin/class-types', ({ request }) => {
      const auth = requireAdmin(db(), request, 'operations.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().classTypes);
    }),

    http.post('*/api/admin/class-types', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'class_types.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertClassTypeSchema);
      if (!body.ok) return body.response;
      const classType = { id: deps().ids.next('cls'), ...body.data };
      deps().classTypes.save(classType);
      return HttpResponse.json(classType, { status: 201 });
    }),

    http.patch('*/api/admin/class-types/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'class_types.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertClassTypeSchema);
      if (!body.ok) return body.response;
      const classType = deps().classTypes.byId(param(params, 'id'));
      if (!classType) return jsonError(404, 'NOT_FOUND', 'Class type not found.');
      Object.assign(classType, body.data);
      deps().classTypes.save(classType);
      return HttpResponse.json(classType);
    }),

    http.get('*/api/admin/sessions', ({ request }) => {
      const auth = requireAdmin(db(), request, 'operations.view');
      if (!auth.ok) return auth.response;
      const url = new URL(request.url);
      const branchId = url.searchParams.get('branchId');
      const from = url.searchParams.get('from');
      const to = url.searchParams.get('to');
      const views = db()
        .sessions.filter((s) => !branchId || s.branchId === branchId)
        .filter((s) => !from || msOf(s.startsAt) >= msOf(from))
        .filter((s) => !to || msOf(s.startsAt) <= msOf(to))
        .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt))
        .map((s) => sessionView(db(), s));
      return HttpResponse.json(views);
    }),

    http.get('*/api/admin/sessions/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'operations.view');
      if (!auth.ok) return auth.response;
      const session = deps().sessions.byId(param(params, 'id'));
      if (!session) return jsonError(404, 'NOT_FOUND', 'Session not found.');
      return HttpResponse.json({
        ...sessionView(db(), session),
        roster: rosterView(db(), session.id),
      });
    }),

    http.post('*/api/admin/sessions', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'sessions.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, CreateSessionSchema);
      if (!body.ok) return body.response;
      const data = body.data;
      const classType = deps().classTypes.byId(data.classTypeId);
      if (!classType) return jsonError(404, 'NOT_FOUND', 'Class type not found.');
      const durationMin = data.durationMin ?? classType.defaultDurationMin;
      const startsMs = msOf(data.startsAt);
      const rules = deps().rules.defaults();
      const session = {
        id: deps().ids.next('ses'),
        classTypeId: classType.id,
        branchId: data.branchId,
        coachId: data.coachId,
        startsAt: new Date(startsMs).toISOString(),
        endsAt: new Date(startsMs + durationMin * 60_000).toISOString(),
        capacity: data.capacity ?? classType.defaultCapacity,
        creditCost: data.creditCost ?? classType.defaultCreditCost,
        bookingOpensAt: new Date(
          startsMs - rules.bookingOpensDaysBefore * 24 * 3600_000,
        ).toISOString(),
        bookingClosesAt: new Date(
          startsMs - rules.bookingClosesMinutesBefore * 60_000,
        ).toISOString(),
        status: (data.publish ? 'PUBLISHED' : 'DRAFT') as SessionStatus,
        area: data.area,
      };
      deps().sessions.save(session);
      return HttpResponse.json(sessionView(db(), session), { status: 201 });
    }),

    http.patch('*/api/admin/sessions/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'sessions.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateSessionSchema);
      if (!body.ok) return body.response;
      const session = deps().sessions.byId(param(params, 'id'));
      if (!session) return jsonError(404, 'NOT_FOUND', 'Session not found.');
      const { durationMin, ...rest } = body.data;
      Object.assign(session, rest);
      if (body.data.startsAt || durationMin) {
        const startMs = msOf(session.startsAt);
        const dur = durationMin ?? Math.round((msOf(session.endsAt) - startMs) / 60_000);
        session.endsAt = new Date(startMs + dur * 60_000).toISOString();
      }
      deps().sessions.save(session);
      return HttpResponse.json(sessionView(db(), session));
    }),

    http.post('*/api/admin/sessions/:id/publish', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'sessions.manage');
      if (!auth.ok) return auth.response;
      const session = deps().sessions.byId(param(params, 'id'));
      if (!session) return jsonError(404, 'NOT_FOUND', 'Session not found.');
      if (!canTransition(SESSION_TRANSITIONS, session.status, 'PUBLISHED'))
        return jsonError(422, 'INVALID_TRANSITION', `A ${session.status} session cannot publish.`);
      session.status = 'PUBLISHED';
      deps().sessions.save(session);
      return HttpResponse.json(sessionView(db(), session));
    }),

    http.post('*/api/admin/sessions/:id/cancel', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'sessions.manage');
      if (!auth.ok) return auth.response;
      const res = cancelSession(deps(), {
        sessionId: param(params, 'id'),
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(sessionView(db(), res.value));
    }),

    http.post('*/api/admin/sessions/:id/complete', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'sessions.manage');
      if (!auth.ok) return auth.response;
      const res = completeSession(deps(), {
        sessionId: param(params, 'id'),
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(sessionView(db(), res.value));
    }),

    http.post('*/api/admin/bookings', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'bookings.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, AdminBookSchema);
      if (!body.ok) return body.response;
      const res = bookSession(deps(), { ...body.data, source: 'ADMIN' });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    http.post('*/api/admin/bookings/:id/no-show', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'attendance.manage');
      if (!auth.ok) return auth.response;
      const res = markNoShow(deps(), {
        bookingId: param(params, 'id'),
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    http.post('*/api/admin/bookings/:id/check-in', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'attendance.manage');
      if (!auth.ok) return auth.response;
      const res = manualCheckIn(deps(), {
        bookingId: param(params, 'id'),
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    http.get('*/api/admin/coaches', ({ request }) => {
      const auth = requireAdmin(db(), request, 'operations.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().coaches);
    }),

    http.post('*/api/admin/coaches', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'coaches.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertCoachSchema);
      if (!body.ok) return body.response;
      const coach = { id: deps().ids.next('coa'), ...body.data };
      deps().coaches.save(coach);
      return HttpResponse.json(coach, { status: 201 });
    }),

    http.patch('*/api/admin/coaches/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'coaches.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertCoachSchema);
      if (!body.ok) return body.response;
      const coach = deps().coaches.byId(param(params, 'id'));
      if (!coach) return jsonError(404, 'NOT_FOUND', 'Coach not found.');
      Object.assign(coach, body.data);
      deps().coaches.save(coach);
      return HttpResponse.json(coach);
    }),

    // ── Admin: access ───────────────────────────────────────────────────────
    http.get('*/api/admin/gates', ({ request }) => {
      const auth = requireAdmin(db(), request, 'access.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().gates);
    }),

    http.get('*/api/admin/access-logs', ({ request }) => {
      const auth = requireAdmin(db(), request, 'access.view');
      if (!auth.ok) return auth.response;
      const url = new URL(request.url);
      const branchId = url.searchParams.get('branchId');
      const gateId = url.searchParams.get('gateId');
      const result = url.searchParams.get('result');
      const mode = url.searchParams.get('mode');
      const limit = Number(url.searchParams.get('limit') ?? 100);
      const views = db()
        .accessLogs.filter((l) => !branchId || l.branchId === branchId)
        .filter((l) => !gateId || l.gateId === gateId)
        .filter((l) =>
          !result ? true : result === 'ALLOWED'
            ? ['ALLOWED', 'OFFLINE_ALLOWED', 'SYNCED'].includes(l.result)
            : l.result === result,
        )
        .filter((l) => !mode || l.mode === mode)
        .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))
        .slice(0, limit)
        .map((l) => accessLogView(db(), l));
      return HttpResponse.json(views);
    }),

    // ── Admin: commercial ───────────────────────────────────────────────────
    http.get('*/api/admin/packages', ({ request }) => {
      const auth = requireAdmin(db(), request, 'commercial.view');
      if (!auth.ok) return auth.response;
      const stats = db().packages.map((pkg) => {
        const paid = db().payments.filter((p) => p.packageId === pkg.id && p.status === 'PAID');
        return {
          pkg,
          purchaseCount: paid.length,
          revenueIdr: paid.reduce((s, p) => s + p.totalIdr, 0),
        };
      });
      return HttpResponse.json(stats);
    }),

    http.post('*/api/admin/packages', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'packages.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertPackageSchema);
      if (!body.ok) return body.response;
      const pkg = { id: deps().ids.next('pkg'), createdAt: deps().clock.now(), ...body.data };
      deps().packages.save(pkg);
      return HttpResponse.json(pkg, { status: 201 });
    }),

    http.patch('*/api/admin/packages/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'packages.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertPackageSchema);
      if (!body.ok) return body.response;
      const pkg = deps().packages.byId(param(params, 'id'));
      if (!pkg) return jsonError(404, 'NOT_FOUND', 'Package not found.');
      Object.assign(pkg, body.data);
      deps().packages.save(pkg);
      return HttpResponse.json(pkg);
    }),

    http.get('*/api/admin/payments', ({ request }) => {
      const auth = requireAdmin(db(), request, 'payments.view');
      if (!auth.ok) return auth.response;
      const views = [...db().payments]
        .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))
        .map((p) => paymentView(db(), p));
      return HttpResponse.json(views);
    }),

    http.post('*/api/admin/payments/:id/refund', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'refunds.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, RefundPaymentSchema);
      if (!body.ok) return body.response;
      const res = refundPayment(deps(), {
        paymentId: param(params, 'id'),
        actor: { id: auth.value.id, name: auth.value.name },
        reason: body.data.reason,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    http.get('*/api/admin/vouchers', ({ request }) => {
      const auth = requireAdmin(db(), request, 'commercial.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().vouchers.map((v) => voucherView(db(), v)));
    }),

    http.post('*/api/admin/vouchers', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'vouchers.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertVoucherSchema);
      if (!body.ok) return body.response;
      const voucher = {
        id: deps().ids.next('vch'),
        status: 'DRAFT' as VoucherStatus,
        createdAt: deps().clock.now(),
        ...body.data,
      };
      deps().vouchers.save(voucher);
      return HttpResponse.json(voucherView(db(), voucher), { status: 201 });
    }),

    http.patch('*/api/admin/vouchers/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'vouchers.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertVoucherSchema);
      if (!body.ok) return body.response;
      const voucher = deps().vouchers.byId(param(params, 'id'));
      if (!voucher) return jsonError(404, 'NOT_FOUND', 'Voucher not found.');
      Object.assign(voucher, body.data);
      deps().vouchers.save(voucher);
      return HttpResponse.json(voucherView(db(), voucher));
    }),

    http.post('*/api/admin/vouchers/:id/status', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'vouchers.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, VoucherStatusActionSchema);
      if (!body.ok) return body.response;
      const voucher = deps().vouchers.byId(param(params, 'id'));
      if (!voucher) return jsonError(404, 'NOT_FOUND', 'Voucher not found.');
      if (!canTransition(VOUCHER_TRANSITIONS, voucher.status, body.data.status))
        return jsonError(
          422,
          'INVALID_TRANSITION',
          `A ${voucher.status} voucher cannot become ${body.data.status}.`,
        );
      voucher.status = body.data.status;
      deps().vouchers.save(voucher);
      return HttpResponse.json(voucherView(db(), voucher));
    }),

    // ── Admin: engagement ───────────────────────────────────────────────────
    http.get('*/api/admin/campaigns', ({ request }) => {
      const auth = requireAdmin(db(), request, 'engagement.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(
        [...db().campaigns].sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt)),
      );
    }),

    http.post('*/api/admin/campaigns', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertCampaignSchema);
      if (!body.ok) return body.response;
      const campaign = {
        id: deps().ids.next('cmp'),
        status: (body.data.scheduledAt ? 'SCHEDULED' : 'DRAFT') as 'SCHEDULED' | 'DRAFT',
        sentCount: null,
        createdAt: deps().clock.now(),
        ...body.data,
      };
      deps().campaigns.save(campaign);
      return HttpResponse.json(campaign, { status: 201 });
    }),

    http.post('*/api/admin/campaigns/:id/send', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const res = sendCampaign(deps(), param(params, 'id'));
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    http.patch('*/api/admin/campaigns/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertCampaignSchema);
      if (!body.ok) return body.response;
      const campaign = db().campaigns.find((c) => c.id === param(params, 'id'));
      if (!campaign) return jsonError(404, 'NOT_FOUND', 'Campaign not found.');
      Object.assign(campaign, body.data);
      deps().campaigns.save(campaign);
      return HttpResponse.json(campaign);
    }),

    http.delete('*/api/admin/campaigns/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      if (!removeById(db().campaigns, param(params, 'id')))
        return jsonError(404, 'NOT_FOUND', 'Campaign not found.');
      return HttpResponse.json({ ok: true });
    }),

    // ── Admin: challenges ───────────────────────────────────────────────────
    http.get('*/api/admin/challenges', ({ request }) => {
      const auth = requireAdmin(db(), request, 'engagement.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(
        [...db().challenges]
          .sort((a, b) => msOf(b.startsAt) - msOf(a.startsAt))
          .map((c) => ({
            challenge: c,
            participantCount: db().challengeJoins.filter((j) => j.challengeId === c.id).length,
          })),
      );
    }),

    http.post('*/api/admin/challenges', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertChallengeSchema);
      if (!body.ok) return body.response;
      const challenge = { id: deps().ids.next('chal'), ...body.data };
      db().challenges.push(challenge);
      return HttpResponse.json({ challenge, participantCount: 0 }, { status: 201 });
    }),

    http.patch('*/api/admin/challenges/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertChallengeSchema);
      if (!body.ok) return body.response;
      const challenge = db().challenges.find((c) => c.id === param(params, 'id'));
      if (!challenge) return jsonError(404, 'NOT_FOUND', 'Challenge not found.');
      Object.assign(challenge, body.data);
      return HttpResponse.json({
        challenge,
        participantCount: db().challengeJoins.filter((j) => j.challengeId === challenge.id).length,
      });
    }),

    http.delete('*/api/admin/challenges/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      if (!removeById(db().challenges, id))
        return jsonError(404, 'NOT_FOUND', 'Challenge not found.');
      const joins = db().challengeJoins;
      for (let i = joins.length - 1; i >= 0; i--) {
        if (joins[i]!.challengeId === id) joins.splice(i, 1);
      }
      return HttpResponse.json({ ok: true });
    }),

    // ── Admin: exercise guides ──────────────────────────────────────────────
    http.get('*/api/admin/exercises', ({ request }) => {
      const auth = requireAdmin(db(), request, 'operations.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().exercises);
    }),

    http.patch('*/api/admin/exercises/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'class_types.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpdateExerciseSchema);
      if (!body.ok) return body.response;
      const exercise = db().exercises.find((e) => e.id === param(params, 'id'));
      if (!exercise) return jsonError(404, 'NOT_FOUND', 'Exercise not found.');
      Object.assign(exercise, body.data);
      return HttpResponse.json(exercise);
    }),

    // ── Admin: race events ──────────────────────────────────────────────────
    http.get('*/api/admin/races', ({ request }) => {
      const auth = requireAdmin(db(), request, 'engagement.view');
      if (!auth.ok) return auth.response;
      const events = [...db().raceEvents]
        .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt))
        .map((e) => ({
          ...e,
          participants: db().userRaces.filter((r) => r.raceEventId === e.id).length,
        }));
      return HttpResponse.json(events);
    }),

    http.post('*/api/admin/races', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertRaceEventSchema);
      if (!body.ok) return body.response;
      const { endsAt, ...rest } = body.data;
      const event = {
        id: deps().ids.next('rce'),
        ...rest,
        endsAt: endsAt ?? new Date(msOf(body.data.startsAt) + 24 * 3600_000).toISOString(),
      };
      db().raceEvents.push(event);
      return HttpResponse.json({ ...event, participants: 0 }, { status: 201 });
    }),

    http.patch('*/api/admin/races/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertRaceEventSchema);
      if (!body.ok) return body.response;
      const event = db().raceEvents.find((e) => e.id === param(params, 'id'));
      if (!event) return jsonError(404, 'NOT_FOUND', 'Race event not found.');
      const { endsAt, ...rest } = body.data;
      Object.assign(event, rest);
      if (endsAt !== undefined && endsAt !== null) event.endsAt = endsAt;
      else if (rest.startsAt && msOf(event.endsAt) < msOf(event.startsAt))
        event.endsAt = new Date(msOf(event.startsAt) + 24 * 3600_000).toISOString();
      return HttpResponse.json({
        ...event,
        participants: db().userRaces.filter((r) => r.raceEventId === event.id).length,
      });
    }),

    http.delete('*/api/admin/races/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'campaigns.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      if (db().userRaces.some((r) => r.raceEventId === id))
        return jsonError(409, 'IN_USE', 'Members have this race on their calendar. Cancel it instead.');
      if (!removeById(db().raceEvents, id))
        return jsonError(404, 'NOT_FOUND', 'Race event not found.');
      return HttpResponse.json({ ok: true });
    }),

    // ── Admin: create + delete completions for the config/catalog menus ─────
    http.post('*/api/admin/members', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'members.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, CreateMemberAdminSchema);
      if (!body.ok) return body.response;
      if (deps().members.byIdentifier(body.data.email))
        return jsonError(409, 'DUPLICATE', 'A member with this email already exists.');
      const now = deps().clock.now();
      const member = {
        id: deps().ids.next('mem'),
        fullName: body.data.fullName,
        email: body.data.email,
        phone: body.data.phone,
        dateOfBirth: null,
        gender: null,
        emergencyContact: null,
        preferredBranchId: body.data.preferredBranchId,
        avatarUrl: null,
        status: 'ACTIVE' as const,
        waiverVersion: null,
        waiverAcceptedAt: null,
        notes: body.data.notes,
        createdAt: now,
        updatedAt: now,
      };
      deps().members.save(member);
      return HttpResponse.json(memberDetailView(db(), deps(), member), { status: 201 });
    }),

    http.delete('*/api/admin/class-types/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'class_types.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      if (db().sessions.some((s) => s.classTypeId === id))
        return jsonError(409, 'IN_USE', 'Sessions use this class type. Cancel or delete them first.');
      if (!removeById(db().classTypes, id))
        return jsonError(404, 'NOT_FOUND', 'Class type not found.');
      return HttpResponse.json({ ok: true });
    }),

    http.delete('*/api/admin/coaches/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'coaches.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      const now = deps().clock.now();
      if (
        db().sessions.some(
          (s) =>
            s.coachId === id &&
            ['DRAFT', 'PUBLISHED', 'FULL'].includes(s.status) &&
            msOf(s.startsAt) > msOf(now),
        )
      )
        return jsonError(409, 'IN_USE', 'This coach has upcoming sessions. Reassign them first.');
      if (!removeById(db().coaches, id)) return jsonError(404, 'NOT_FOUND', 'Coach not found.');
      return HttpResponse.json({ ok: true });
    }),

    http.delete('*/api/admin/sessions/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'sessions.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      const session = deps().sessions.byId(id);
      if (!session) return jsonError(404, 'NOT_FOUND', 'Session not found.');
      if (db().bookings.some((b) => b.sessionId === id && b.status !== 'CANCELLED'))
        return jsonError(409, 'IN_USE', 'This session has bookings. Cancel the session instead.');
      removeById(db().sessions, id);
      return HttpResponse.json({ ok: true });
    }),

    http.delete('*/api/admin/packages/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'packages.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      if (db().payments.some((p) => p.packageId === id))
        return jsonError(409, 'IN_USE', 'Payments reference this package. Archive it instead.');
      if (!removeById(db().packages, id))
        return jsonError(404, 'NOT_FOUND', 'Package not found.');
      return HttpResponse.json({ ok: true });
    }),

    http.delete('*/api/admin/vouchers/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'vouchers.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      if (db().redemptions.some((r) => r.voucherId === id))
        return jsonError(409, 'IN_USE', 'This voucher has redemptions. Disable it instead.');
      if (!removeById(db().vouchers, id))
        return jsonError(404, 'NOT_FOUND', 'Voucher not found.');
      return HttpResponse.json({ ok: true });
    }),

    http.delete('*/api/admin/branches/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'branches.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      if (db().gates.some((g) => g.branchId === id) || db().sessions.some((s) => s.branchId === id))
        return jsonError(409, 'IN_USE', 'Gates or sessions belong to this branch. Move them first.');
      if (!removeById(db().branches, id))
        return jsonError(404, 'NOT_FOUND', 'Branch not found.');
      return HttpResponse.json({ ok: true });
    }),

    http.delete('*/api/admin/gates/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'gates.manage');
      if (!auth.ok) return auth.response;
      if (!removeById(db().gates, param(params, 'id')))
        return jsonError(404, 'NOT_FOUND', 'Gate not found.');
      return HttpResponse.json({ ok: true });
    }),

    http.delete('*/api/admin/users/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'users.manage');
      if (!auth.ok) return auth.response;
      const id = param(params, 'id');
      if (id === auth.value.id)
        return jsonError(409, 'IN_USE', 'You cannot delete the account you are signed in with.');
      const target = deps().adminUsers.byId(id);
      if (!target) return jsonError(404, 'NOT_FOUND', 'User not found.');
      if (
        target.role === 'SUPER_ADMIN' &&
        db().adminUsers.filter((u) => u.role === 'SUPER_ADMIN').length <= 1
      )
        return jsonError(409, 'IN_USE', 'At least one Super Admin must remain.');
      removeById(db().adminUsers, id);
      return HttpResponse.json({ ok: true });
    }),

    // ── Admin: dashboard & reports ──────────────────────────────────────────
    http.get('*/api/admin/reports/dashboard', ({ request }) => {
      const auth = requireAdmin(db(), request, 'dashboard.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(dashboardView(db(), deps()));
    }),

    http.get('*/api/admin/reports/sales', ({ request }) => {
      const auth = requireAdmin(db(), request, 'reports.view');
      if (!auth.ok) return auth.response;
      const url = new URL(request.url);
      const days = Number(url.searchParams.get('days') ?? 30);
      const cutoff = Date.now() - days * 24 * 3600_000;
      const paid = db().payments.filter(
        (p) => (p.status === 'PAID' || p.status === 'REFUNDED') && p.paidAt && msOf(p.paidAt) >= cutoff,
      );
      const byDayMap = new Map<string, number>();
      const byChannelMap = new Map<string, number>();
      for (const p of paid) {
        const day = p.paidAt!.slice(0, 10);
        byDayMap.set(day, (byDayMap.get(day) ?? 0) + p.totalIdr);
        byChannelMap.set(p.channel, (byChannelMap.get(p.channel) ?? 0) + p.totalIdr);
      }
      const byPackage = db().packages.map((pkg) => {
        const pkgPaid = paid.filter((p) => p.packageId === pkg.id);
        return {
          pkg,
          purchaseCount: pkgPaid.length,
          revenueIdr: pkgPaid.reduce((s, p) => s + p.totalIdr, 0),
        };
      });
      return HttpResponse.json({
        totalIdr: paid.reduce((s, p) => s + p.totalIdr, 0),
        byDay: [...byDayMap.entries()]
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([date, value]) => ({ date, value })),
        byChannel: [...byChannelMap.entries()].map(([channel, totalIdr]) => ({
          channel,
          totalIdr,
        })),
        byPackage,
      });
    }),

    http.get('*/api/admin/reports/visits', ({ request }) => {
      const auth = requireAdmin(db(), request, 'reports.view');
      if (!auth.ok) return auth.response;
      const url = new URL(request.url);
      const days = Number(url.searchParams.get('days') ?? 30);
      const cutoff = Date.now() - days * 24 * 3600_000;
      const logs = db().accessLogs.filter((l) => msOf(l.createdAt) >= cutoff);
      const allowed = logs.filter((l) =>
        ['ALLOWED', 'OFFLINE_ALLOWED', 'SYNCED'].includes(l.result),
      );
      const byDayMap = new Map<string, number>();
      for (const l of allowed) {
        const day = l.createdAt.slice(0, 10);
        byDayMap.set(day, (byDayMap.get(day) ?? 0) + 1);
      }
      return HttpResponse.json({
        total: allowed.length,
        byDay: [...byDayMap.entries()]
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([date, value]) => ({ date, value })),
        denied: logs.filter((l) => l.result === 'DENIED').length,
        offline: logs.filter((l) => l.mode === 'OFFLINE').length,
      });
    }),

    http.get('*/api/admin/reports/credits', ({ request }) => {
      const auth = requireAdmin(db(), request, 'reports.view');
      if (!auth.ok) return auth.response;
      const perMember = db()
        .members.filter((m) => m.status !== 'ARCHIVED')
        .map((m) => ({
          memberId: m.id,
          memberName: m.fullName,
          balance: balanceOf(deps(), m.id),
          expiring: expiringCreditsFor(deps(), m.id),
        }))
        .sort((a, b) => b.balance - a.balance);
      return HttpResponse.json({
        outstandingTotal: perMember.reduce((s, m) => s + m.balance, 0),
        expiringTotal: perMember.reduce((s, m) => s + m.expiring, 0),
        perMember,
      });
    }),

    http.get('*/api/admin/audit', ({ request }) => {
      const auth = requireAdmin(db(), request, 'config.view');
      if (!auth.ok) return auth.response;
      const url = new URL(request.url);
      const limit = Number(url.searchParams.get('limit') ?? 100);
      return HttpResponse.json(
        [...db().audit].sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt)).slice(0, limit),
      );
    }),

    // ── Admin: config ───────────────────────────────────────────────────────
    http.get('*/api/admin/branches', ({ request }) => {
      const auth = requireAdmin(db(), request, 'config.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().branches);
    }),

    http.post('*/api/admin/branches', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'branches.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, CreateBranchSchema);
      if (!body.ok) return body.response;
      const branch = {
        id: deps().ids.next('brn'),
        organizationId: db().organization.id,
        status: 'ACTIVE' as const,
        rulesOverride: null,
        ...body.data,
      };
      deps().branches.save(branch);
      return HttpResponse.json(branch, { status: 201 });
    }),

    http.post('*/api/admin/gates', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'gates.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertGateSchema);
      if (!body.ok) return body.response;
      if (!deps().branches.byId(body.data.branchId))
        return jsonError(404, 'NOT_FOUND', 'Branch not found.');
      const gate = { id: deps().ids.next('gat'), ...body.data };
      deps().gates.save(gate);
      return HttpResponse.json(gate, { status: 201 });
    }),

    http.patch('*/api/admin/gates/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'gates.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertGateSchema);
      if (!body.ok) return body.response;
      const gate = deps().gates.byId(param(params, 'id'));
      if (!gate) return jsonError(404, 'NOT_FOUND', 'Gate not found.');
      Object.assign(gate, body.data);
      deps().gates.save(gate);
      return HttpResponse.json(gate);
    }),

    http.post('*/api/admin/users', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'users.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertAdminUserSchema);
      if (!body.ok) return body.response;
      const user = { id: deps().ids.next('adm'), ...body.data };
      deps().adminUsers.save(user);
      return HttpResponse.json(user, { status: 201 });
    }),

    http.patch('*/api/admin/users/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'users.manage');
      if (!auth.ok) return auth.response;
      const body = await parsePatch(request, UpsertAdminUserSchema);
      if (!body.ok) return body.response;
      const user = deps().adminUsers.byId(param(params, 'id'));
      if (!user) return jsonError(404, 'NOT_FOUND', 'User not found.');
      Object.assign(user, body.data);
      deps().adminUsers.save(user);
      return HttpResponse.json(user);
    }),

    http.post('*/api/admin/segments/preview', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'engagement.view');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, SegmentPreviewSchema);
      if (!body.ok) return body.response;
      const audience = segmentMembers(deps(), body.data.segment, body.data.customFilter);
      return HttpResponse.json({
        count: audience.length,
        sample: audience.slice(0, 5).map((m) => m.fullName),
      });
    }),

    http.get('*/api/admin/reports/classes', ({ request }) => {
      const auth = requireAdmin(db(), request, 'reports.view');
      if (!auth.ok) return auth.response;
      const perType = db().classTypes.map((classType) => {
        const sessions = db().sessions.filter(
          (s) => s.classTypeId === classType.id && s.status === 'COMPLETED',
        );
        const sessionIds = new Set(sessions.map((s) => s.id));
        const bookings = db().bookings.filter((b) => sessionIds.has(b.sessionId));
        const attended = bookings.filter(
          (b) => b.status === 'COMPLETED' || b.status === 'CHECKED_IN',
        ).length;
        const noShows = bookings.filter((b) => b.status === 'NO_SHOW').length;
        const booked = attended + noShows;
        return {
          classTypeId: classType.id,
          classTypeName: classType.name,
          sessionsHeld: sessions.length,
          booked,
          attended,
          noShows,
          attendanceRate: booked > 0 ? Math.round((attended / booked) * 100) : 0,
        };
      });
      const recentNoShows = db()
        .bookings.filter((b) => b.status === 'NO_SHOW')
        .map((b) => {
          const session = db().sessions.find((s) => s.id === b.sessionId);
          return {
            memberName: db().members.find((m) => m.id === b.memberId)?.fullName ?? b.memberId,
            classTypeName:
              db().classTypes.find((t) => t.id === session?.classTypeId)?.name ?? 'Class',
            startsAt: session?.startsAt ?? b.createdAt,
          };
        })
        .sort((a, b) => msOf(b.startsAt) - msOf(a.startsAt))
        .slice(0, 15);
      return HttpResponse.json({ perType, recentNoShows });
    }),

    http.post('*/api/admin/access-logs/:id/resolve', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'access.simulate');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, ResolveConflictSchema);
      if (!body.ok) return body.response;
      const res = resolveOfflineConflict(deps(), {
        logId: param(params, 'id'),
        action: body.data.action,
        reason: body.data.reason,
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(accessLogView(db(), res.value));
    }),

    http.patch('*/api/admin/branches/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'branches.manage');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateBranchSchema);
      if (!body.ok) return body.response;
      const branch = deps().branches.byId(param(params, 'id'));
      if (!branch) return jsonError(404, 'NOT_FOUND', 'Branch not found.');
      Object.assign(branch, body.data);
      deps().branches.save(branch);
      return HttpResponse.json(branch);
    }),

    http.get('*/api/admin/rules', ({ request }) => {
      const auth = requireAdmin(db(), request, 'config.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json({
        defaults: db().rules,
        branchOverrides: db()
          .branches.filter((b) => b.rulesOverride)
          .map((b) => ({ branchId: b.id, branchName: b.name, override: b.rulesOverride! })),
      });
    }),

    http.put('*/api/admin/rules', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'rules.update');
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateRulesSchema);
      if (!body.ok) return body.response;
      const res = updateRules(deps(), {
        patch: body.data,
        actor: { id: auth.value.id, name: auth.value.name },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // ── Devtools ────────────────────────────────────────────────────────────
    http.post('*/api/dev/reset', () => {
      onReset();
      return HttpResponse.json({ ok: true });
    }),

    http.post('*/api/dev/expiry-sweep', () => {
      const result = runExpirySweep(deps());
      return HttpResponse.json(result);
    }),

    // ── Athlete module (Strava-style) ───────────────────────────────────────
    ...createAthleteHandlers(state),
    // ── HYROX workouts (phase 3) + races (phase 4) ──────────────────────────
    ...createWorkoutRaceHandlers(state),
  ];
}

/** Small helper used by handlers above (kept exported for tests). */
export function pendingPaymentsFor(db: MockDb, memberId: string): Payment[] {
  return db.payments.filter((p) => p.memberId === memberId && p.status === 'PENDING');
}
