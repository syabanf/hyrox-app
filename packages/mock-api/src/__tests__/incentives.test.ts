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

const login = async (userId: string) =>
  (await call('POST', '/api/admin/auth/login', { body: { userId } })).data.token as string;

const monthLabel = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
const now = new Date();
const thisMonth = monthLabel(now);
const lastMonth = monthLabel(new Date(now.getFullYear(), now.getMonth() - 1, 1));

describe('coach incentives', () => {
  it('RBAC: finance manages, branch manager views, front desk sees nothing', async () => {
    const fd = await login('adm_fd');
    expect((await call('GET', '/api/admin/incentives/schemes', { token: fd })).status).toBe(403);
    const bm = await login('adm_bm');
    expect((await call('GET', '/api/admin/incentives/schemes', { token: bm })).status).toBe(200);
    const denied = await call('POST', '/api/admin/incentives/payouts', {
      token: bm,
      body: { coachId: 'coa_1', periodMonth: thisMonth },
    });
    expect(denied.status).toBe(403);
    const fin = await login('adm_fin');
    expect((await call('GET', '/api/admin/incentives/payouts', { token: fin })).status).toBe(200);
  });

  it('seeds a default scheme plus coach overrides', async () => {
    const token = await login('adm_hq');
    const res = await call('GET', '/api/admin/incentives/schemes', { token });
    expect(res.status).toBe(200);
    expect(res.data[0].scheme.coachId).toBeNull();
    expect(res.data[0].scheme.sessionFeeIdr).toBe(150_000);
    expect(res.data.filter((v: any) => v.scheme.coachId !== null).length).toBe(2);
    expect(res.data.find((v: any) => v.scheme.coachId === 'coa_1').coachName).toBe('Kevin Hartono');
  });

  it("seeded last-month payouts equal the statement recomputed from the seed's sessions", async () => {
    const token = await login('adm_fin');
    const payouts = await call('GET', `/api/admin/incentives/payouts?period=${lastMonth}`, {
      token,
    });
    expect(payouts.status).toBe(200);
    expect(payouts.data.map((v: any) => v.payout.status).sort()).toEqual([
      'APPROVED',
      'DRAFT',
      'PAID',
    ]);
    const statements = await call('GET', `/api/admin/incentives/statements?period=${lastMonth}`, {
      token,
    });
    expect(statements.status).toBe(200);
    for (const v of payouts.data) {
      const live = statements.data.find((s: any) => s.coachId === v.payout.coachId);
      expect(live.totals).toEqual(v.payout.statement.totals);
      expect(live.payout.payout.id).toBe(v.payout.id);
      expect(v.payout.statement.totals.totalIdr).toBeGreaterThan(0);
    }
    const paid = payouts.data.find((v: any) => v.payout.status === 'PAID');
    expect(paid.payout.paymentReference).toBeTruthy();
    expect(paid.payout.paidAt).toBeTruthy();
  });

  it('statement lines follow the resolved scheme (override for coa_1, default elsewhere)', async () => {
    const token = await login('adm_hq');
    const res = await call('GET', `/api/admin/incentives/statements?period=${lastMonth}`, {
      token,
    });
    const kevin = res.data.find((s: any) => s.coachId === 'coa_1');
    expect(kevin.scheme.id).toBe('inc_coa_1');
    const maya = res.data.find((s: any) => s.coachId === 'coa_2');
    expect(maya.scheme.id).toBe('inc_default');
    for (const line of kevin.lines) {
      expect(line.sessionFeeIdr).toBe(200_000);
      expect(line.attendeeIdr).toBe(line.attended * 12_500);
      expect(line.totalIdr).toBe(
        Math.max(0, line.sessionFeeIdr + line.attendeeIdr + line.bonusIdr - line.penaltyIdr),
      );
    }
    // Branch filter narrows to that branch's coaches.
    const pik = await call(
      'GET',
      `/api/admin/incentives/statements?period=${lastMonth}&branchId=brn_pik`,
      {
        token,
      },
    );
    expect(pik.data.every((s: any) => s.branchId === 'brn_pik')).toBe(true);
  });

  it('creates a payout for this month, refuses a duplicate, and walks the state machine', async () => {
    const token = await login('adm_fin');
    const statements = await call('GET', `/api/admin/incentives/statements?period=${thisMonth}`, {
      token,
    });
    const target = statements.data.find((s: any) => s.payout === null && s.totals.sessions > 0);
    expect(target).toBeTruthy();

    const created = await call('POST', '/api/admin/incentives/payouts', {
      token,
      body: { coachId: target.coachId, periodMonth: thisMonth },
    });
    expect(created.status).toBe(201);
    expect(created.data.payout.status).toBe('DRAFT');
    expect(created.data.payout.statement.totals).toEqual(target.totals);
    expect(created.data.periodMonth).toBe(thisMonth);
    const id = created.data.payout.id;

    const dup = await call('POST', '/api/admin/incentives/payouts', {
      token,
      body: { coachId: target.coachId, periodMonth: thisMonth },
    });
    expect(dup.status).toBe(409);
    expect(dup.data.error.code).toBe('PAYOUT_EXISTS');

    // pay before approve → illegal
    const early = await call('POST', `/api/admin/incentives/payouts/${id}/pay`, {
      token,
      body: { paymentReference: 'TRF-1' },
    });
    expect(early.status).toBe(409);

    const approved = await call('POST', `/api/admin/incentives/payouts/${id}/approve`, {
      token,
      body: {},
    });
    expect(approved.status).toBe(200);
    expect(approved.data.payout.status).toBe('APPROVED');
    expect(approved.data.payout.approvedBy).toBe('adm_fin');

    const noRef = await call('POST', `/api/admin/incentives/payouts/${id}/pay`, {
      token,
      body: {},
    });
    expect(noRef.status).toBe(422);

    const paid = await call('POST', `/api/admin/incentives/payouts/${id}/pay`, {
      token,
      body: { paymentReference: 'TRF-TEST-42' },
    });
    expect(paid.status).toBe(200);
    expect(paid.data.payout.status).toBe('PAID');
    expect(paid.data.payout.paymentReference).toBe('TRF-TEST-42');

    const voidPaid = await call('POST', `/api/admin/incentives/payouts/${id}/void`, {
      token,
      body: { note: 'oops' },
    });
    expect(voidPaid.status).toBe(409); // PAID is terminal

    const audit = api.state.db.audit.filter((a) => a.entityId === id).map((a) => a.action);
    expect(audit).toEqual(['CREATE', 'APPROVE', 'PAY']);
  });

  it('a voided draft frees the period for a new payout', async () => {
    const token = await login('adm_super');
    const statements = await call('GET', `/api/admin/incentives/statements?period=${thisMonth}`, {
      token,
    });
    const target = statements.data.find((s: any) => s.payout === null && s.totals.sessions > 0);
    expect(target).toBeTruthy();
    const created = await call('POST', '/api/admin/incentives/payouts', {
      token,
      body: { coachId: target.coachId, periodMonth: thisMonth },
    });
    const id = created.data.payout.id;
    const noReason = await call('POST', `/api/admin/incentives/payouts/${id}/void`, {
      token,
      body: {},
    });
    expect(noReason.status).toBe(422);
    const voided = await call('POST', `/api/admin/incentives/payouts/${id}/void`, {
      token,
      body: { note: 'Wrong month' },
    });
    expect(voided.data.payout.status).toBe('VOID');
    const again = await call('POST', '/api/admin/incentives/payouts', {
      token,
      body: { coachId: target.coachId, periodMonth: thisMonth },
    });
    expect(again.status).toBe(201);
  });

  it('scheme upserts are audited and validated', async () => {
    const token = await login('adm_hq');
    const created = await call('POST', '/api/admin/incentives/schemes', {
      token,
      body: {
        coachId: 'coa_5',
        sessionFeeIdr: 160_000,
        perAttendeeIdr: 11_000,
        fullClassBonusIdr: 90_000,
        fullClassThresholdPercent: 70,
        noShowPenaltyIdr: 5_000,
      },
    });
    expect(created.status).toBe(201);
    expect(created.data.scheme.active).toBe(true);
    const updated = await call('PUT', `/api/admin/incentives/schemes/${created.data.scheme.id}`, {
      token,
      body: { ...created.data.scheme, sessionFeeIdr: 170_000 },
    });
    expect(updated.status).toBe(200);
    expect(updated.data.scheme.sessionFeeIdr).toBe(170_000);
    const unknownCoach = await call('POST', '/api/admin/incentives/schemes', {
      token,
      body: {
        coachId: 'coa_nope',
        sessionFeeIdr: 1,
        perAttendeeIdr: 1,
        fullClassBonusIdr: 1,
        fullClassThresholdPercent: 50,
        noShowPenaltyIdr: 0,
      },
    });
    expect(unknownCoach.status).toBe(404);
    const badPercent = await call('POST', '/api/admin/incentives/schemes', {
      token,
      body: {
        coachId: null,
        sessionFeeIdr: 1,
        perAttendeeIdr: 1,
        fullClassBonusIdr: 1,
        fullClassThresholdPercent: 120,
        noShowPenaltyIdr: 0,
      },
    });
    expect(badPercent.status).toBe(400);
    const audit = api.state.db.audit.filter((a) => a.entityType === 'INCENTIVE_SCHEME');
    expect(audit.map((a) => a.action)).toEqual(['CREATE', 'UPDATE']);
  });

  it('dashboard exposes this month’s payable total', async () => {
    const token = await login('adm_super');
    const dash = await call('GET', '/api/admin/reports/dashboard', { token });
    const statements = await call('GET', `/api/admin/incentives/statements?period=${thisMonth}`, {
      token,
    });
    const sum = statements.data.reduce((s: number, v: any) => s + v.totals.totalIdr, 0);
    expect(dash.data.coachIncentivesPayableIdr).toBe(sum);
  });
});
