import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createMockServer } from '../msw/node';

const { api, server } = createMockServer();
const BASE = 'http://localhost';

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterAll(() => server.close());

async function call(
  method: string,
  path: string,
  opts: { token?: string; body?: unknown } = {},
): Promise<{ status: number; data: any }> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: {
      'content-type': 'application/json',
      ...(opts.token ? { authorization: `Bearer ${opts.token}` } : {}),
    },
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
  });
  const data = await res.json().catch(() => null);
  return { status: res.status, data };
}

describe('end-to-end core loop through the mock API', () => {
  let memberToken = '';

  it('registers a new member (waiver + terms)', async () => {
    const res = await call('POST', '/api/auth/register', {
      body: {
        fullName: 'Integration Tester',
        email: 'itest@example.com',
        phone: '+628999000111',
        dateOfBirth: null,
        gender: 'FEMALE',
        emergencyContact: { name: 'Buddy', phone: '+628999000112', relation: 'Friend' },
        preferredBranchId: 'brn_senopati',
        waiverAccepted: true,
        termsAccepted: true,
      },
    });
    expect(res.status).toBe(201);
    memberToken = res.data.token;
    expect(res.data.member.status).toBe('ACTIVE');
    expect(res.data.member.waiverAcceptedAt).toBeTruthy();
  });

  it('starts with zero balance', async () => {
    const res = await call('GET', '/api/me/wallet', { token: memberToken });
    expect(res.status).toBe(200);
    expect(res.data.balance).toBe(0);
  });

  it('tops up with the WELCOME10 voucher (new-member segment)', async () => {
    const quote = await call('POST', '/api/vouchers/validate', {
      token: memberToken,
      body: { code: 'WELCOME10', packageId: 'pkg_10' },
    });
    expect(quote.status).toBe(200);
    expect(quote.data.discountIdr).toBe(150_000);

    const topup = await call('POST', '/api/me/topup', {
      token: memberToken,
      body: { packageId: 'pkg_10', voucherCode: 'WELCOME10', channel: 'QRIS' },
    });
    expect(topup.status).toBe(201);
    expect(topup.data.payment.status).toBe('PENDING');
    expect(topup.data.payment.totalIdr).toBe(1_350_000);

    // Balance unchanged while pending — PAYMENT ≠ LEDGER.
    const before = await call('GET', '/api/me/wallet', { token: memberToken });
    expect(before.data.balance).toBe(0);

    const paid = await call('POST', `/api/payments/${topup.data.payment.id}/simulate`, {
      token: memberToken,
      body: {},
    });
    expect(paid.status).toBe(200);
    expect(paid.data.payment.status).toBe('PAID');

    const after = await call('GET', '/api/me/wallet', { token: memberToken });
    expect(after.data.balance).toBe(10);
    expect(after.data.entries.some((e: any) => e.type === 'TOP_UP' && e.amount === 10)).toBe(true);
  });

  let sessionId = '';
  let bookingId = '';

  it('books an upcoming class', async () => {
    const sessions = await call('GET', '/api/sessions?branchId=brn_senopati', {
      token: memberToken,
    });
    // Starts >24h out so the later gate scan is an open-gym entry, not this
    // booking's check-in, and cancellation stays inside the deadline.
    const bookable = sessions.data.find(
      (v: any) =>
        v.session.status === 'PUBLISHED' &&
        v.spotsLeft > 0 &&
        v.myBooking === null &&
        new Date(v.session.startsAt).getTime() > Date.now() + 24 * 3600_000,
    );
    expect(bookable).toBeTruthy();
    sessionId = bookable.session.id;

    const res = await call('POST', `/api/sessions/${sessionId}/book`, { token: memberToken, body: {} });
    expect(res.status).toBe(201);
    expect(res.data.decision).toBe('CONFIRMED');
    bookingId = res.data.booking.id;
  });

  it('rejects a duplicate booking of the same session', async () => {
    const res = await call('POST', `/api/sessions/${sessionId}/book`, { token: memberToken, body: {} });
    expect(res.status).toBe(422);
    expect(res.data.error.code).toBe('ALREADY_BOOKED');
  });

  it('issues a short-lived dynamic QR and scans it at the gate', async () => {
    const qr = await call('POST', '/api/me/qr', { token: memberToken });
    expect(qr.status).toBe(200);
    expect(qr.data.ttlSeconds).toBeGreaterThan(0);

    const scan = await call('POST', '/api/gates/gat_sen_a/scan', {
      body: { qrToken: qr.data.token },
    });
    expect(scan.status).toBe(200);
    expect(scan.data.decision).toBe('ALLOWED');
    expect(scan.data.remainingCredits).toBe(9);

    const visits = await call('GET', '/api/me/visits', { token: memberToken });
    expect(visits.data.length).toBe(1);
    expect(visits.data[0].log.result).toBe('ALLOWED');
  });

  it('a consumed token cannot be replayed, and an immediate re-scan is a free re-entry', async () => {
    const qr = await call('POST', '/api/me/qr', { token: memberToken });
    const first = await call('POST', '/api/gates/gat_sen_a/scan', {
      body: { qrToken: qr.data.token },
    });
    expect(first.data.decision).toBe('ALLOWED');
    expect(first.data.entryKind).toBe('RE_ENTRY'); // within grace — no extra deduction
    expect(first.data.remainingCredits).toBe(9);

    const replay = await call('POST', '/api/gates/gat_sen_a/scan', {
      body: { qrToken: qr.data.token },
    });
    expect(replay.data.decision).toBe('DENIED');
    expect(replay.data.reason).toBe('TOKEN_CONSUMED');
  });

  it('waitlists when a session is full, and cancellation auto-promotes', async () => {
    const admin = await call('POST', '/api/admin/auth/login', { body: { userId: 'adm_super' } });
    const adminToken = admin.data.token;

    // The seeded FULL session has 4 confirmed + waitlist.
    const sessions = await call('GET', '/api/admin/sessions', { token: adminToken });
    const full = sessions.data.find((v: any) => v.session.status === 'FULL');
    expect(full).toBeTruthy();

    // New booking lands on the waitlist.
    const res = await call('POST', `/api/sessions/${full.session.id}/book`, {
      token: memberToken,
      body: {},
    });
    expect(res.status).toBe(201);
    expect(res.data.decision).toBe('WAITLIST');

    // Admin cancels a confirmed booking → position 1 gets promoted.
    const detail = await call('GET', `/api/admin/sessions/${full.session.id}`, {
      token: adminToken,
    });
    const confirmed = detail.data.roster.find((r: any) => r.booking.status === 'CONFIRMED');
    const firstWaitlisted = detail.data.roster
      .filter((r: any) => r.booking.status === 'WAITLIST')
      .sort((a: any, b: any) => a.booking.waitlistPosition - b.booking.waitlistPosition)[0];

    const cancel = await call('POST', `/api/bookings/${confirmed.booking.id}/cancel`, {
      token: adminToken,
      body: {},
    });
    expect(cancel.status).toBe(200);
    expect(cancel.data.promotedMemberName).toBe(firstWaitlisted.memberName);
  });

  it('cancelling the earlier booking before the deadline refunds nothing but frees the slot', async () => {
    const res = await call('POST', `/api/bookings/${bookingId}/cancel`, {
      token: memberToken,
      body: {},
    });
    expect(res.status).toBe(200);
    expect(res.data.outcome).toBe('RELEASED');
    expect(res.data.penaltyCredits).toBe(0);
  });
});

describe('RBAC is enforced server-side', () => {
  it('front desk cannot adjust credits or update rules', async () => {
    const login = await call('POST', '/api/admin/auth/login', { body: { userId: 'adm_fd' } });
    const token = login.data.token;

    const adjust = await call('POST', '/api/admin/members/mem_demo/adjust', {
      token,
      body: { amount: 5, reason: 'sneaky' },
    });
    expect(adjust.status).toBe(403);

    const rules = await call('PUT', '/api/admin/rules', {
      token,
      body: { qrTtlSeconds: 60 },
    });
    expect(rules.status).toBe(403);
  });

  it('super admin rule changes take effect in the gate pipeline', async () => {
    const login = await call('POST', '/api/admin/auth/login', { body: { userId: 'adm_super' } });
    const token = login.data.token;
    const updated = await call('PUT', '/api/admin/rules', { token, body: { qrTtlSeconds: 90 } });
    expect(updated.status).toBe(200);
    expect(updated.data.qrTtlSeconds).toBe(90);
    expect(api.state.db.rules.qrTtlSeconds).toBe(90);
  });

  it('unauthenticated requests are rejected', async () => {
    const res = await call('GET', '/api/admin/members');
    expect(res.status).toBe(401);
  });
});

describe('seed integrity', () => {
  it('outstanding credits report equals the sum of member balances', async () => {
    const login = await call('POST', '/api/admin/auth/login', { body: { userId: 'adm_fin' } });
    const report = await call('GET', '/api/admin/reports/credits', { token: login.data.token });
    expect(report.status).toBe(200);
    const sum = report.data.perMember.reduce((s: number, m: any) => s + m.balance, 0);
    expect(report.data.outstandingTotal).toBe(sum);
  });

  it('demo member exists with a coherent ledger', async () => {
    const otp = await call('POST', '/api/auth/otp/request', {
      body: { identifier: 'demo@hyrox.id' },
    });
    expect(otp.data.memberExists).toBe(true);
    const verify = await call('POST', '/api/auth/otp/verify', {
      body: { challengeId: otp.data.challengeId, code: '123456' },
    });
    expect(verify.status).toBe(200);
    const wallet = await call('GET', '/api/me/wallet', { token: verify.data.token });
    // Ledger types present in the demo history.
    const types = new Set(wallet.data.entries.map((e: any) => e.type));
    expect(types.has('TOP_UP')).toBe(true);
    expect(types.has('VISIT_DEDUCTION')).toBe(true);
    expect(types.has('PROMO')).toBe(true);
    expect(types.has('ADJUSTMENT')).toBe(true);
    expect(types.has('REVERSAL')).toBe(true);
    expect(wallet.data.balance).toBeGreaterThan(0);
  });
});
