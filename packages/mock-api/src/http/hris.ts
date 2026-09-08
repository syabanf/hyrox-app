import type {
  Attendance,
  Employee,
  Holiday,
  Leave,
  LeaveBalance,
  OvertimeRequest,
  Shift,
} from '@nuhabit/domain';
import { WEEKDAYS } from '@nuhabit/domain';
import { http, HttpResponse } from 'msw';
import {
  addDays,
  computeLateness,
  describeLeaveDays,
  overtimeHours,
  scheduledWindow,
  studioDate,
  workHours,
} from '../hris-rules';
import { jsonError, requireAdmin } from './helpers';
import type { MockApiState } from './handlers';

/**
 * The People pages, answered in-process.
 *
 * This mirrors the Go module route for route so the panel behaves the same
 * with or without a backend. The backend is the source of truth for every rule
 * here; see `hris-rules.ts`.
 */
export function createHrisHandlers(state: MockApiState) {
  const db = () => state.db;
  const nowIso = () => new Date().toISOString();
  const today = () => studioDate(nowIso());
  const nextId = (prefix: string) => {
    const counters = db().counters;
    counters[prefix] = (counters[prefix] ?? 0) + 1;
    return `${prefix}_${String(counters[prefix]).padStart(4, '0')}`;
  };

  // ── Small readers ─────────────────────────────────────────────────────────

  const employeeOr404 = (id: string): Employee | null =>
    db().employees.find((e) => e.id === id) ?? null;

  const nameEmployee = (employeeId: string) => {
    const e = employeeOr404(employeeId);
    return { employeeName: e?.fullName ?? '', employeeNumber: e?.employeeNumber ?? '' };
  };

  const employeeView = (e: Employee) => ({
    ...e,
    departmentName: db().departments.find((d) => d.id === e.departmentId)?.name ?? null,
    positionTitle: db().positions.find((p) => p.id === e.positionId)?.title ?? null,
    branchName: db().branches.find((b) => b.id === e.branchId)?.name ?? null,
    reportingToName: db().employees.find((p) => p.id === e.reportingTo)?.fullName ?? null,
    coachName: db().coaches.find((c) => c.id === e.coachId)?.name ?? null,
  });

  const attendanceView = (a: Attendance) => ({
    ...a,
    ...nameEmployee(a.employeeId),
    shiftName: db().shifts.find((s) => s.id === a.shiftId)?.name ?? null,
  });

  const leaveView = (l: Leave) => ({ ...l, ...nameEmployee(l.employeeId) });
  const overtimeView = (o: OvertimeRequest) => ({ ...o, ...nameEmployee(o.employeeId) });

  /** The pattern row governing a date: latest effective_from wins. */
  const scheduleRowFor = (employeeId: string, date: string) => {
    const day = new Date(`${date}T00:00:00Z`).getUTCDay() || 7;
    return db()
      .employeeShifts.filter(
        (r) =>
          r.employeeId === employeeId &&
          r.dayOfWeek === day &&
          r.effectiveFrom <= date &&
          (!r.effectiveTo || r.effectiveTo >= date),
      )
      .sort((a, b) => b.effectiveFrom.localeCompare(a.effectiveFrom))[0];
  };

  const shiftFor = (employeeId: string, date: string): Shift | null => {
    const row = scheduleRowFor(employeeId, date);
    if (!row?.shiftId) return null;
    return db().shifts.find((s) => s.id === row.shiftId) ?? null;
  };

  /** Reads a balance, creating the year's row on first look. */
  const balanceFor = (employeeId: string, year: number): LeaveBalance => {
    let balance = db().leaveBalances.find((b) => b.employeeId === employeeId && b.year === year);
    if (!balance) {
      balance = {
        employeeId,
        year,
        annualTotal: 12,
        annualUsed: 0,
        sickUsed: 0,
        unpaidUsed: 0,
        maternityUsed: 0,
        paternityUsed: 0,
        emergencyUsed: 0,
        pilgrimageUsed: 0,
        menstrualUsed: 0,
      };
      db().leaveBalances.push(balance);
    }
    return balance;
  };

  const usedKey: Record<string, keyof LeaveBalance> = {
    ANNUAL: 'annualUsed',
    SICK: 'sickUsed',
    UNPAID: 'unpaidUsed',
    MATERNITY: 'maternityUsed',
    PATERNITY: 'paternityUsed',
    EMERGENCY: 'emergencyUsed',
    PILGRIMAGE: 'pilgrimageUsed',
    MENSTRUAL: 'menstrualUsed',
  };

  const adjustUsage = (employeeId: string, year: number, type: string, days: number) => {
    const balance = balanceFor(employeeId, year);
    const key = usedKey[type];
    if (!key) return;
    (balance[key] as number) = Math.max(0, (balance[key] as number) + days);
  };

  const activeHolidays = (from?: string, to?: string): Holiday[] =>
    db()
      .holidays.filter((h) => (!from || h.date >= from) && (!to || h.date <= to))
      .sort((a, b) => a.date.localeCompare(b.date));

  const query = (request: Request, key: string) =>
    new URL(request.url).searchParams.get(key)?.trim() ?? '';

  return [
    // ── Overview and self-service ───────────────────────────────────────────

    http.get('*/api/admin/hris/overview', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;

      const date = today();
      const summarize = (rows: Attendance[]) => ({
        present: rows.filter((r) => ['PRESENT', 'REMOTE', 'HALF_DAY'].includes(r.status)).length,
        late: rows.filter((r) => r.status === 'LATE').length,
        absent: rows.filter((r) => r.status === 'ABSENT').length,
        onLeave: rows.filter((r) => r.status === 'ON_LEAVE').length,
        totalHours: rows.reduce((sum, r) => sum + r.workHours, 0),
        lateMinutes: rows.reduce((sum, r) => sum + r.lateMinutes, 0),
      });

      const monthStart = `${date.slice(0, 8)}01`;
      const roster = buildRoster(date, '');
      return HttpResponse.json({
        date,
        activeEmployees: db().employees.filter((e) => e.active).length,
        totalEmployees: db().employees.length,
        today: summarize(db().attendance.filter((a) => a.date === date)),
        month: summarize(db().attendance.filter((a) => a.date >= monthStart && a.date <= date)),
        pendingLeave: db().leaves.filter((l) => l.status === 'PENDING').length,
        pendingOvertime: db().overtimeRequests.filter((o) => o.status === 'PENDING').length,
        onLeaveToday: roster.filter((r) => r.status === 'ON_LEAVE').length,
        expectedToday: roster.filter((r) => ['EXPECTED', 'WORKING', 'LATE'].includes(r.status))
          .length,
        missingToday: roster.filter((r) => r.status === 'MISSING').length,
        nextHolidays: activeHolidays(date, addDays(date, 365)).slice(0, 5),
      });
    }),

    http.get('*/api/admin/hris/me', ({ request }) => {
      const auth = requireAdmin(db(), request);
      if (!auth.ok) return auth.response;

      const date = today();
      const employee = db().employees.find((e) => e.adminUserId === auth.value.id);
      if (!employee) {
        return HttpResponse.json({
          employee: null,
          balance: null,
          today: null,
          date,
          shiftName: null,
          expectedStart: null,
          expectedEnd: null,
          restDay: false,
          scheduled: false,
          leaves: [],
        });
      }
      const row = scheduleRowFor(employee.id, date);
      const shift = shiftFor(employee.id, date);
      const window = shift ? scheduledWindow(date, shift) : null;
      return HttpResponse.json({
        employee: employeeView(employee),
        balance: balanceFor(employee.id, Number(date.slice(0, 4))),
        today: db().attendance.find((a) => a.employeeId === employee.id && a.date === date) ?? null,
        date,
        shiftName: shift?.name ?? null,
        expectedStart: window?.start ?? null,
        expectedEnd: window?.end ?? null,
        restDay: !!row && !row.shiftId,
        scheduled: !!row,
        leaves: db().leaves.filter((l) => l.employeeId === employee.id).slice(0, 10),
      });
    }),

    http.post('*/api/admin/hris/me/clock-in', ({ request }) => {
      const auth = requireAdmin(db(), request);
      if (!auth.ok) return auth.response;
      const employee = db().employees.find((e) => e.adminUserId === auth.value.id);
      if (!employee)
        return jsonError(409, 'NOT_AN_EMPLOYEE', 'That account is not linked to an employee record.');
      return punchIn(employee.id, nowIso());
    }),

    http.post('*/api/admin/hris/me/clock-out', ({ request }) => {
      const auth = requireAdmin(db(), request);
      if (!auth.ok) return auth.response;
      const employee = db().employees.find((e) => e.adminUserId === auth.value.id);
      if (!employee)
        return jsonError(409, 'NOT_AN_EMPLOYEE', 'That account is not linked to an employee record.');
      return punchOut(employee.id, nowIso());
    }),

    // ── Organization ────────────────────────────────────────────────────────

    http.get('*/api/admin/hris/departments', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().departments);
    }),

    http.get('*/api/admin/hris/positions', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().positions);
    }),

    http.get('*/api/admin/hris/employment-statuses', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(db().employmentStatuses);
    }),

    // ── People ──────────────────────────────────────────────────────────────

    http.get('*/api/admin/hris/employees', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;

      const q = query(request, 'query').toLowerCase();
      const department = query(request, 'departmentId');
      const branch = query(request, 'branchId');
      const activeOnly = query(request, 'activeOnly') === 'true';

      return HttpResponse.json(
        db()
          .employees.filter(
            (e) =>
              (!q ||
                `${e.fullName} ${e.email} ${e.employeeNumber}`.toLowerCase().includes(q)) &&
              (!department || e.departmentId === department) &&
              (!branch || e.branchId === branch) &&
              (!activeOnly || e.active),
          )
          .sort((a, b) => a.fullName.localeCompare(b.fullName))
          .map(employeeView),
      );
    }),

    http.get('*/api/admin/hris/employees/:id/schedule', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(scheduleViewFor(String(params.id)));
    }),

    http.post('*/api/admin/hris/employees/:id/schedule', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as {
        dayOfWeek: number;
        shiftId?: string | null;
        effectiveFrom: string;
        effectiveTo?: string | null;
      };
      if (body.dayOfWeek < 1 || body.dayOfWeek > 7)
        return jsonError(400, 'VALIDATION_FAILED', 'A weekday runs from 1 (Monday) to 7 (Sunday).');

      const row = {
        id: nextId('esh'),
        employeeId: String(params.id),
        dayOfWeek: body.dayOfWeek,
        shiftId: body.shiftId || null,
        effectiveFrom: body.effectiveFrom,
        effectiveTo: body.effectiveTo || null,
      };
      db().employeeShifts.push(row);
      return HttpResponse.json(
        {
          ...row,
          shiftName: db().shifts.find((s) => s.id === row.shiftId)?.name ?? null,
          dayName: WEEKDAYS[row.dayOfWeek] ?? '',
        },
        { status: 201 },
      );
    }),

    http.delete('*/api/admin/hris/employees/:id/schedule/:rowId', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const before = db().employeeShifts.length;
      db().employeeShifts = db().employeeShifts.filter((r) => r.id !== String(params.rowId));
      if (db().employeeShifts.length === before)
        return jsonError(404, 'NOT_FOUND', 'That schedule row does not exist.');
      return HttpResponse.json({ ok: true });
    }),

    http.get('*/api/admin/hris/employees/:id/balance', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      const year = Number(query(request, 'year')) || Number(today().slice(0, 4));
      if (!employeeOr404(String(params.id)))
        return jsonError(404, 'NOT_FOUND', 'That employee does not exist.');
      return HttpResponse.json(balanceFor(String(params.id), year));
    }),

    http.put('*/api/admin/hris/employees/:id/balance', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as { year?: number; annualTotal: number };
      const year = body.year || Number(today().slice(0, 4));
      const balance = balanceFor(String(params.id), year);
      if (body.annualTotal < balance.annualUsed)
        return jsonError(409, 'BELOW_USED', 'The allowance cannot be set below what has been taken.');
      balance.annualTotal = body.annualTotal;
      return HttpResponse.json(balance);
    }),

    http.get('*/api/admin/hris/employees/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      const employee = employeeOr404(String(params.id));
      if (!employee) return jsonError(404, 'NOT_FOUND', 'That employee does not exist.');

      return HttpResponse.json({
        employee: employeeView(employee),
        balance: balanceFor(employee.id, Number(today().slice(0, 4))),
        schedule: scheduleViewFor(employee.id),
        attendance: db()
          .attendance.filter((a) => a.employeeId === employee.id)
          .sort((a, b) => b.date.localeCompare(a.date))
          .slice(0, 30),
        leaves: db()
          .leaves.filter((l) => l.employeeId === employee.id)
          .sort((a, b) => b.startDate.localeCompare(a.startDate)),
        overtime: db()
          .overtimeRequests.filter((o) => o.employeeId === employee.id)
          .sort((a, b) => b.date.localeCompare(a.date)),
        directReports: db()
          .employees.filter((e) => e.reportingTo === employee.id)
          .map(employeeView),
      });
    }),

    http.post('*/api/admin/hris/employees', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as Partial<Employee>;
      if (db().employees.some((e) => e.employeeNumber === body.employeeNumber))
        return jsonError(409, 'DUPLICATE', 'An employee already uses that number.');

      const created: Employee = {
        ...blankEmployee(nextId('emp'), nowIso()),
        ...body,
        id: nextId('emp'),
      } as Employee;
      created.id = created.id ?? nextId('emp');
      db().employees.push(created);
      return HttpResponse.json(employeeView(created), { status: 201 });
    }),

    http.put('*/api/admin/hris/employees/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const existing = employeeOr404(String(params.id));
      if (!existing) return jsonError(404, 'NOT_FOUND', 'That employee does not exist.');

      const body = (await request.json()) as Partial<Employee>;
      Object.assign(existing, body, { id: existing.id, updatedAt: nowIso() });
      return HttpResponse.json(employeeView(existing));
    }),

    // ── Shifts ──────────────────────────────────────────────────────────────

    http.get('*/api/admin/hris/shifts', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      const activeOnly = query(request, 'activeOnly') === 'true';
      return HttpResponse.json(
        db()
          .shifts.filter((s) => !activeOnly || s.active)
          .sort((a, b) => a.sortOrder - b.sortOrder),
      );
    }),

    http.post('*/api/admin/hris/shifts', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as Partial<Shift>;
      const created: Shift = {
        id: nextId('shf'),
        name: body.name ?? 'Shift',
        startTime: normalizeClock(body.startTime ?? '08:00'),
        endTime: normalizeClock(body.endTime ?? '16:00'),
        breakMinutes: body.breakMinutes ?? 60,
        lateToleranceMinutes: body.lateToleranceMinutes ?? 10,
        isOvernight: body.isOvernight ?? false,
        active: body.active ?? true,
        sortOrder: body.sortOrder ?? db().shifts.length + 1,
      };
      db().shifts.push(created);
      return HttpResponse.json(created, { status: 201 });
    }),

    http.put('*/api/admin/hris/shifts/:id', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const shift = db().shifts.find((s) => s.id === String(params.id));
      if (!shift) return jsonError(404, 'NOT_FOUND', 'That shift does not exist.');
      const body = (await request.json()) as Partial<Shift>;
      Object.assign(shift, body, {
        id: shift.id,
        startTime: normalizeClock(body.startTime ?? shift.startTime),
        endTime: normalizeClock(body.endTime ?? shift.endTime),
      });
      return HttpResponse.json(shift);
    }),

    // ── Roster and attendance ───────────────────────────────────────────────

    http.get('*/api/admin/hris/roster', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      return HttpResponse.json(buildRoster(query(request, 'date') || today(), query(request, 'branchId')));
    }),

    http.get('*/api/admin/hris/attendance', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      const employeeId = query(request, 'employeeId');
      const from = query(request, 'from');
      const to = query(request, 'to');
      const status = query(request, 'status');

      return HttpResponse.json(
        db()
          .attendance.filter(
            (a) =>
              (!employeeId || a.employeeId === employeeId) &&
              (!from || a.date >= from) &&
              (!to || a.date <= to) &&
              (!status || a.status === status),
          )
          .sort((a, b) => b.date.localeCompare(a.date))
          .map(attendanceView),
      );
    }),

    http.post('*/api/admin/hris/attendance/clock-in', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.attendance');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as { employeeId: string; at?: string; notes?: string | null };
      return punchIn(body.employeeId, body.at || nowIso(), body.notes ?? null);
    }),

    http.post('*/api/admin/hris/attendance/clock-out', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.attendance');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as { employeeId: string; at?: string };
      return punchOut(body.employeeId, body.at || nowIso());
    }),

    http.post('*/api/admin/hris/attendance/mark', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.attendance');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as {
        employeeId: string;
        date: string;
        status: Attendance['status'];
        notes?: string | null;
      };
      if (!employeeOr404(body.employeeId))
        return jsonError(404, 'NOT_FOUND', 'That employee does not exist.');

      let row = db().attendance.find(
        (a) => a.employeeId === body.employeeId && a.date === body.date,
      );
      if (!row) {
        row = blankAttendance(nextId('att'), body.employeeId, body.date);
        db().attendance.push(row);
      }
      const shift = shiftFor(body.employeeId, body.date);
      Object.assign(row, {
        status: body.status,
        clockIn: null,
        clockOut: null,
        workHours: 0,
        isLate: false,
        lateMinutes: 0,
        notes: body.notes ?? null,
        shiftId: shift?.id ?? row.shiftId,
        updatedAt: nowIso(),
      });
      return HttpResponse.json(attendanceView(row));
    }),

    // ── Leave ───────────────────────────────────────────────────────────────

    http.get('*/api/admin/hris/leaves', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      const employeeId = query(request, 'employeeId');
      const status = query(request, 'status');
      const type = query(request, 'type');

      return HttpResponse.json(
        db()
          .leaves.filter(
            (l) =>
              (!employeeId || l.employeeId === employeeId) &&
              (!status || l.status === status) &&
              (!type || l.type === type),
          )
          .sort((a, b) => b.startDate.localeCompare(a.startDate))
          .map(leaveView),
      );
    }),

    http.post('*/api/admin/hris/leaves', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.attendance');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as {
        employeeId: string;
        type: Leave['type'];
        startDate: string;
        endDate: string;
        reason: string;
      };
      const employee = employeeOr404(body.employeeId);
      if (!employee) return jsonError(404, 'NOT_FOUND', 'That employee does not exist.');
      if (body.endDate < body.startDate)
        return jsonError(400, 'VALIDATION_FAILED', 'A leave request cannot end before it starts.');

      const breakdown = describeLeaveDays(
        body.startDate,
        body.endDate,
        activeHolidays(body.startDate, body.endDate),
      );
      if (breakdown.totalDays === 0)
        return jsonError(
          409,
          'NO_WORKING_DAYS',
          'Every day in that range is already a weekend or a public holiday.',
        );

      const overlaps = db().leaves.some(
        (l) =>
          l.employeeId === body.employeeId &&
          (l.status === 'PENDING' || l.status === 'APPROVED') &&
          l.startDate <= body.endDate &&
          l.endDate >= body.startDate,
      );
      if (overlaps)
        return jsonError(409, 'OVERLAPS_EXISTING', 'That range overlaps a request already on file.');

      const year = Number(body.startDate.slice(0, 4));
      const balance = balanceFor(body.employeeId, year);
      // Pending days are spoken for even though nothing has been deducted, so
      // two requests cannot together overspend the year.
      const reserved = db()
        .leaves.filter(
          (l) =>
            l.employeeId === body.employeeId &&
            l.status === 'PENDING' &&
            l.type === 'ANNUAL' &&
            l.startDate.startsWith(String(year)),
        )
        .reduce((sum, l) => sum + l.totalDays, 0);
      const remaining = balance.annualTotal - balance.annualUsed - reserved;

      if (body.type === 'ANNUAL' && breakdown.totalDays > remaining)
        return jsonError(
          409,
          'INSUFFICIENT_BALANCE',
          `That request needs ${breakdown.totalDays.toFixed(1)} days and only ${remaining.toFixed(1)} are left.`,
        );

      const leave: Leave = {
        id: nextId('lve'),
        employeeId: body.employeeId,
        type: body.type,
        startDate: body.startDate,
        endDate: body.endDate,
        totalDays: breakdown.totalDays,
        reason: body.reason,
        attachmentUrl: null,
        status: 'PENDING',
        approvedBy: null,
        approvedAt: null,
        rejectionReason: null,
        createdAt: nowIso(),
        updatedAt: nowIso(),
      };
      db().leaves.push(leave);
      return HttpResponse.json(
        { ...leaveView(leave), excludedHolidays: breakdown.excludedHolidays },
        { status: 201 },
      );
    }),

    http.post('*/api/admin/hris/leaves/:id/:action', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.approve');
      if (!auth.ok) return auth.response;
      const leave = db().leaves.find((l) => l.id === String(params.id));
      if (!leave) return jsonError(404, 'NOT_FOUND', 'That leave request does not exist.');

      const action = String(params.action);
      const body = (await request.json().catch(() => ({}))) as { reason?: string };
      const year = Number(leave.startDate.slice(0, 4));

      if (action === 'approve') {
        if (leave.status !== 'PENDING')
          return jsonError(409, 'INVALID_TRANSITION', `A ${leave.status} request cannot be approved.`);
        // The allowance moves only now.
        adjustUsage(leave.employeeId, year, leave.type, leave.totalDays);
        Object.assign(leave, {
          status: 'APPROVED',
          approvedBy: auth.value.id,
          approvedAt: nowIso(),
          rejectionReason: null,
        });
      } else if (action === 'reject') {
        if (leave.status !== 'PENDING')
          return jsonError(409, 'INVALID_TRANSITION', `A ${leave.status} request cannot be rejected.`);
        if (!body.reason) return jsonError(400, 'VALIDATION_FAILED', 'A rejection needs a reason.');
        Object.assign(leave, {
          status: 'REJECTED',
          approvedBy: auth.value.id,
          approvedAt: nowIso(),
          rejectionReason: body.reason,
        });
      } else if (action === 'cancel') {
        if (leave.status !== 'PENDING' && leave.status !== 'APPROVED')
          return jsonError(409, 'INVALID_TRANSITION', `A ${leave.status} request cannot be cancelled.`);
        // Cancelling an approved request hands the allowance back.
        if (leave.status === 'APPROVED')
          adjustUsage(leave.employeeId, year, leave.type, -leave.totalDays);
        leave.status = 'CANCELLED';
      } else {
        return jsonError(400, 'VALIDATION_FAILED', 'A request is approved, rejected or cancelled.');
      }
      leave.updatedAt = nowIso();
      return HttpResponse.json(leaveView(leave));
    }),

    // ── Overtime ────────────────────────────────────────────────────────────

    http.get('*/api/admin/hris/overtime', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      const employeeId = query(request, 'employeeId');
      const status = query(request, 'status');
      return HttpResponse.json(
        db()
          .overtimeRequests.filter(
            (o) => (!employeeId || o.employeeId === employeeId) && (!status || o.status === status),
          )
          .sort((a, b) => b.date.localeCompare(a.date))
          .map(overtimeView),
      );
    }),

    http.post('*/api/admin/hris/overtime', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.attendance');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as {
        employeeId: string;
        date: string;
        startTime: string;
        endTime: string;
        source?: OvertimeRequest['source'];
        reason?: string | null;
      };
      if (!employeeOr404(body.employeeId))
        return jsonError(404, 'NOT_FOUND', 'That employee does not exist.');

      const hours = overtimeHours(body.startTime, body.endTime);
      if (hours <= 0) return jsonError(400, 'VALIDATION_FAILED', 'Overtime must cover some time.');

      const claim: OvertimeRequest = {
        id: nextId('ovt'),
        employeeId: body.employeeId,
        date: body.date,
        startTime: normalizeClock(body.startTime),
        endTime: normalizeClock(body.endTime),
        hours,
        source: body.source === 'COMPANY' ? 'COMPANY' : 'EMPLOYEE',
        status: 'PENDING',
        reason: body.reason ?? null,
        decidedBy: null,
        decidedAt: null,
        rejectionReason: null,
        createdAt: nowIso(),
        updatedAt: nowIso(),
      };
      db().overtimeRequests.push(claim);
      return HttpResponse.json(overtimeView(claim), { status: 201 });
    }),

    http.post('*/api/admin/hris/overtime/:id/:action', async ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.approve');
      if (!auth.ok) return auth.response;
      const claim = db().overtimeRequests.find((o) => o.id === String(params.id));
      if (!claim) return jsonError(404, 'NOT_FOUND', 'That overtime claim does not exist.');

      const action = String(params.action);
      const body = (await request.json().catch(() => ({}))) as { reason?: string };
      const day = db().attendance.find(
        (a) => a.employeeId === claim.employeeId && a.date === claim.date,
      );

      if (action === 'approve') {
        if (claim.status !== 'PENDING')
          return jsonError(409, 'INVALID_TRANSITION', `A ${claim.status} claim cannot be approved.`);
        // Approved hours belong on the timesheet, so the two cannot disagree.
        if (day) day.overtimeHours += claim.hours;
        Object.assign(claim, {
          status: 'APPROVED',
          decidedBy: auth.value.id,
          decidedAt: nowIso(),
          rejectionReason: null,
        });
      } else if (action === 'reject') {
        if (claim.status !== 'PENDING')
          return jsonError(409, 'INVALID_TRANSITION', `A ${claim.status} claim cannot be rejected.`);
        if (!body.reason) return jsonError(400, 'VALIDATION_FAILED', 'A rejection needs a reason.');
        Object.assign(claim, {
          status: 'REJECTED',
          decidedBy: auth.value.id,
          decidedAt: nowIso(),
          rejectionReason: body.reason,
        });
      } else if (action === 'cancel') {
        if (claim.status !== 'PENDING' && claim.status !== 'APPROVED')
          return jsonError(409, 'INVALID_TRANSITION', `A ${claim.status} claim cannot be cancelled.`);
        if (claim.status === 'APPROVED' && day)
          day.overtimeHours = Math.max(0, day.overtimeHours - claim.hours);
        claim.status = 'CANCELLED';
      } else {
        return jsonError(400, 'VALIDATION_FAILED', 'A claim is approved, rejected or cancelled.');
      }
      claim.updatedAt = nowIso();
      return HttpResponse.json(overtimeView(claim));
    }),

    // ── Holidays ────────────────────────────────────────────────────────────

    http.get('*/api/admin/hris/holidays', ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.view');
      if (!auth.ok) return auth.response;
      const from = query(request, 'from');
      const to = query(request, 'to');
      return HttpResponse.json(activeHolidays(from || undefined, to || undefined));
    }),

    http.post('*/api/admin/hris/holidays', async ({ request }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const body = (await request.json()) as Omit<Holiday, 'id'>;
      if (
        db().holidays.some(
          (h) => h.date === body.date && h.name.toLowerCase() === body.name.toLowerCase(),
        )
      )
        return jsonError(409, 'DUPLICATE', 'That holiday is already on the calendar.');

      const created: Holiday = { ...body, id: nextId('hol'), note: body.note ?? null };
      db().holidays.push(created);
      return HttpResponse.json(created, { status: 201 });
    }),

    http.delete('*/api/admin/hris/holidays/:id', ({ request, params }) => {
      const auth = requireAdmin(db(), request, 'hris.manage');
      if (!auth.ok) return auth.response;
      const before = db().holidays.length;
      db().holidays = db().holidays.filter((h) => h.id !== String(params.id));
      if (db().holidays.length === before)
        return jsonError(404, 'NOT_FOUND', 'That holiday does not exist.');
      return HttpResponse.json({ ok: true });
    }),
  ];

  // ── Shared behaviour ──────────────────────────────────────────────────────

  function scheduleViewFor(employeeId: string) {
    return db()
      .employeeShifts.filter((r) => r.employeeId === employeeId)
      .sort((a, b) => a.dayOfWeek - b.dayOfWeek || b.effectiveFrom.localeCompare(a.effectiveFrom))
      .map((r) => ({
        ...r,
        shiftName: db().shifts.find((s) => s.id === r.shiftId)?.name ?? null,
        dayName: WEEKDAYS[r.dayOfWeek] ?? '',
      }));
  }

  function punchIn(employeeId: string, atIso: string, notes: string | null = null) {
    const employee = employeeOr404(employeeId);
    if (!employee) return jsonError(404, 'NOT_FOUND', 'That employee does not exist.');
    const date = studioDate(atIso);
    if (!employee.active)
      return jsonError(409, 'NOT_EMPLOYED', `${employee.fullName} is not on the payroll.`);

    let row = db().attendance.find((a) => a.employeeId === employeeId && a.date === date);
    if (row?.clockIn)
      return jsonError(
        409,
        'ALREADY_CLOCKED_IN',
        `${employee.fullName} already clocked in on ${date}.`,
      );
    if (!row) {
      row = blankAttendance(nextId('att'), employeeId, date);
      db().attendance.push(row);
    }

    const shift = shiftFor(employeeId, date);
    row.clockIn = atIso;
    row.notes = notes;
    row.status = 'PRESENT';
    if (shift) {
      const window = scheduledWindow(date, shift);
      const late = computeLateness(atIso, date, shift);
      Object.assign(row, {
        shiftId: shift.id,
        scheduledStart: window.start,
        scheduledEnd: window.end,
        breakMinutes: shift.breakMinutes,
        isLate: late.isLate,
        lateMinutes: late.lateMinutes,
        status: late.isLate ? 'LATE' : 'PRESENT',
      });
    }
    row.updatedAt = atIso;
    return HttpResponse.json(attendanceView(row), { status: 201 });
  }

  function punchOut(employeeId: string, atIso: string) {
    const date = studioDate(atIso);
    const row =
      db().attendance.find((a) => a.employeeId === employeeId && a.date === date) ??
      db().attendance.find((a) => a.employeeId === employeeId && a.date === addDays(date, -1));
    if (!row?.clockIn)
      return jsonError(409, 'NOT_CLOCKED_IN', 'There is no open attendance record to close.');
    if (row.clockOut) return jsonError(409, 'ALREADY_CLOCKED_OUT', 'That day is already closed.');

    row.clockOut = atIso;
    row.workHours = workHours(row.clockIn, atIso, row.breakMinutes);
    row.updatedAt = atIso;
    return HttpResponse.json(attendanceView(row));
  }

  function buildRoster(date: string, branchId: string) {
    const holidays = activeHolidays(date, date);
    const now = Date.now();

    return db()
      .employees.filter((e) => e.active && (!branchId || e.branchId === branchId))
      .filter((e) => e.joinDate <= date && (!e.endDate || e.endDate >= date))
      .sort((a, b) => a.fullName.localeCompare(b.fullName))
      .map((employee) => {
        const row = scheduleRowFor(employee.id, date);
        const shift = row?.shiftId ? db().shifts.find((s) => s.id === row.shiftId) : null;
        const window = shift ? scheduledWindow(date, shift) : null;

        let status: string;
        if (!row) status = 'UNSCHEDULED';
        else if (!row.shiftId) status = 'REST_DAY';
        else status = 'EXPECTED';

        const holiday = status === 'EXPECTED' && holidays.length > 0 ? holidays[0]! : null;
        if (holiday) status = 'HOLIDAY';

        const leave = db().leaves.find(
          (l) =>
            l.employeeId === employee.id &&
            l.status === 'APPROVED' &&
            l.startDate <= date &&
            l.endDate >= date,
        );
        if (leave) status = 'ON_LEAVE';

        const attendance =
          db().attendance.find((a) => a.employeeId === employee.id && a.date === date) ?? null;
        if (attendance) {
          if (attendance.clockOut) status = 'DONE';
          else if (attendance.isLate) status = 'LATE';
          else if (attendance.clockIn) status = 'WORKING';
          else if (attendance.status === 'ABSENT') status = 'MISSING';
        } else if (status === 'EXPECTED' && window && now > new Date(window.end).getTime()) {
          // The shift ended and nobody clocked in. Saying MISSING is a fact;
          // marking it ABSENT is HR's decision, not the roster's.
          status = 'MISSING';
        }

        return {
          employeeId: employee.id,
          employeeName: employee.fullName,
          employeeNumber: employee.employeeNumber,
          departmentId: employee.departmentId,
          branchId: employee.branchId,
          shiftName: shift?.name ?? null,
          scheduledStart: window?.start ?? null,
          scheduledEnd: window?.end ?? null,
          attendance,
          status,
          leaveType: leave?.type ?? null,
          holiday,
        };
      });
  }
}

/** `HH:MM` from a form becomes the `HH:MM:SS` the rest of the system speaks. */
function normalizeClock(value: string): string {
  return value.length === 5 ? `${value}:00` : value;
}

function blankEmployee(id: string, nowIso: string): Employee {
  return {
    id,
    fullName: '',
    employeeNumber: '',
    email: '',
    phone: '',
    birthDate: null,
    gender: null,
    maritalStatus: null,
    address: null,
    joinDate: nowIso.slice(0, 10),
    endDate: null,
    employmentStatusCode: 'PERMANENT',
    active: true,
    departmentId: null,
    positionId: null,
    branchId: null,
    reportingTo: null,
    coachId: null,
    adminUserId: null,
    bankName: null,
    bankAccount: null,
    emergencyContactName: null,
    emergencyContactPhone: null,
    emergencyContactRelation: null,
    photoUrl: null,
    notes: null,
    createdAt: nowIso,
    updatedAt: nowIso,
  };
}

function blankAttendance(id: string, employeeId: string, date: string): Attendance {
  const now = new Date().toISOString();
  return {
    id,
    employeeId,
    date,
    clockIn: null,
    clockOut: null,
    shiftId: null,
    scheduledStart: null,
    scheduledEnd: null,
    workHours: 0,
    breakMinutes: 60,
    status: 'PRESENT',
    isLate: false,
    lateMinutes: 0,
    overtimeHours: 0,
    notes: null,
    createdAt: now,
    updatedAt: now,
  };
}
