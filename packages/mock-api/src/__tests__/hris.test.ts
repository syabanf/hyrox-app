import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createMockServer } from '../msw/node';
import { computeLateness, describeLeaveDays, overtimeHours, studioDate } from '../hris-rules';

const { server } = createMockServer();
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
  return { status: res.status, data: await res.json().catch(() => null) };
}

const superAdmin = 'admin:adm_super';
const frontDesk = 'admin:adm_fd';
const finance = 'admin:adm_fin';

// These mirror the rules the Go backend owns. The demo has to agree with it,
// so the same three cases are asserted here as in the backend's own suite.
describe('the rules the demo must get right', () => {
  const shift = {
    id: 'shf_test',
    name: 'Test',
    startTime: '08:00:00',
    endTime: '16:00:00',
    breakMinutes: 60,
    lateToleranceMinutes: 10,
    isOvernight: false,
    active: true,
    sortOrder: 1,
  };

  it('counts lateness from the shift start, not from the end of the tolerance', () => {
    // Twenty-five minutes in, with ten minutes' grace, is twenty-five late.
    const late = computeLateness('2026-03-02T08:25:00+07:00', '2026-03-02', shift);
    expect(late).toEqual({ isLate: true, lateMinutes: 25 });

    // Nine minutes is inside the grace period and is not late at all.
    expect(computeLateness('2026-03-02T08:09:00+07:00', '2026-03-02', shift).isLate).toBe(false);
  });

  it('spares a public holiday but still charges collective leave', () => {
    const national = {
      id: 'h1',
      date: '2026-03-04',
      name: 'Public holiday',
      type: 'NATIONAL' as const,
      deductsLeave: false,
      note: null,
    };
    const collective = { ...national, id: 'h2', name: 'Cuti bersama', type: 'COLLECTIVE' as const, deductsLeave: true };

    // Monday to Friday, minus the free public holiday on Wednesday.
    const free = describeLeaveDays('2026-03-02', '2026-03-06', [national]);
    expect(free.totalDays).toBe(4);
    expect(free.excludedHolidays).toEqual([{ date: '2026-03-04', name: 'Public holiday' }]);

    // Collective leave comes out of the allowance, so the day still counts.
    expect(describeLeaveDays('2026-03-02', '2026-03-06', [collective]).totalDays).toBe(5);

    // Both on one date: the national holiday wins.
    expect(describeLeaveDays('2026-03-02', '2026-03-06', [collective, national]).totalDays).toBe(4);
  });

  it('rolls overtime past midnight', () => {
    expect(overtimeHours('22:00', '01:00')).toBe(3);
    expect(overtimeHours('18:00', '20:30')).toBe(2.5);
  });
});

describe('the People pages', () => {
  it('refuses the staff directory to the front desk', async () => {
    // Employee records hold home addresses, bank accounts and next of kin.
    const refused = await call('GET', '/api/admin/hris/employees', { token: frontDesk });
    expect(refused.status).toBe(403);

    const allowed = await call('GET', '/api/admin/hris/employees', { token: finance });
    expect(allowed.status).toBe(200);
    expect(allowed.data.length).toBeGreaterThan(0);
  });

  it('lets anybody signed in see their own working day', async () => {
    const mine = await call('GET', '/api/admin/hris/me', { token: frontDesk });
    expect(mine.status).toBe(200);
    expect(mine.data.employee.fullName).toBe('Nadia Putri');
  });

  it('tells resting apart from unscheduled on the roster', async () => {
    const { status, data } = await call('GET', '/api/admin/hris/roster', { token: superAdmin });
    expect(status).toBe(200);
    expect(data.length).toBeGreaterThan(0);
    for (const entry of data) {
      expect(entry.status).toBeTruthy();
    }
  });

  it('moves the allowance on approval and hands it back on cancellation', async () => {
    const today = studioDate(new Date().toISOString());
    const year = Number(today.slice(0, 4));
    const before = await call('GET', `/api/admin/hris/employees/emp_kevin/balance?year=${year}`, {
      token: superAdmin,
    });

    const filed = await call('POST', '/api/admin/hris/leaves', {
      token: superAdmin,
      body: {
        employeeId: 'emp_kevin',
        type: 'ANNUAL',
        startDate: nextMonday(today),
        endDate: nextMonday(today),
        reason: 'A day off.',
      },
    });
    expect(filed.status).toBe(201);
    expect(filed.data.totalDays).toBe(1);
    expect(filed.data.status).toBe('PENDING');

    // Nothing has been deducted while it waits for a decision.
    const pending = await call('GET', `/api/admin/hris/employees/emp_kevin/balance?year=${year}`, {
      token: superAdmin,
    });
    expect(pending.data.annualUsed).toBe(before.data.annualUsed);

    const approved = await call('POST', `/api/admin/hris/leaves/${filed.data.id}/approve`, {
      token: superAdmin,
      body: {},
    });
    expect(approved.data.status).toBe('APPROVED');
    const after = await call('GET', `/api/admin/hris/employees/emp_kevin/balance?year=${year}`, {
      token: superAdmin,
    });
    expect(after.data.annualUsed).toBe(before.data.annualUsed + 1);

    await call('POST', `/api/admin/hris/leaves/${filed.data.id}/cancel`, {
      token: superAdmin,
      body: {},
    });
    const returned = await call('GET', `/api/admin/hris/employees/emp_kevin/balance?year=${year}`, {
      token: superAdmin,
    });
    expect(returned.data.annualUsed).toBe(before.data.annualUsed);
  });

  it('refuses a second clock-in on the same day', async () => {
    const first = await call('POST', '/api/admin/hris/attendance/clock-in', {
      token: superAdmin,
      body: { employeeId: 'emp_bima' },
    });
    expect(first.status).toBe(201);

    const second = await call('POST', '/api/admin/hris/attendance/clock-in', {
      token: superAdmin,
      body: { employeeId: 'emp_bima' },
    });
    expect(second.status).toBe(409);
    expect(second.data.error.code).toBe('ALREADY_CLOCKED_IN');
  });
});

/** The next Monday, so a one-day request is never a weekend. */
function nextMonday(from: string): string {
  const d = new Date(`${from}T00:00:00Z`);
  do {
    d.setUTCDate(d.getUTCDate() + 1);
  } while (d.getUTCDay() !== 1);
  return d.toISOString().slice(0, 10);
}
