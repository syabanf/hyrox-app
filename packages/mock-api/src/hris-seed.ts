import type { MockDb } from './db';
import { addDays, computeLateness, scheduledWindow, studioDate, workHours } from './hris-rules';

/**
 * The people side of the demo studio: the org chart, the shift patterns
 * everybody works, this year's holidays, and a fortnight of punches so the
 * timesheet is not empty on first load.
 *
 * Deliberately the same nine people the Go seeder loads, with the same employee
 * ids, so a demo looks identical whether the panel is talking to the mock or to
 * the backend. The `adminUserId` links use the mock's own admin ids, which
 * predate the backend and differ from its.
 */
export function seedHris(db: MockDb, nowIso: string): void {
  // Idempotent by construction: the snapshot already carries a seeded HRIS and
  // the browser re-seeds it against today, so every collection this owns is
  // replaced rather than appended to.
  db.attendance = [];
  db.leaves = [];
  db.overtimeRequests = [];

  const today = studioDate(nowIso);
  const year = Number(today.slice(0, 4));

  db.departments = [
    ['dep_ops', 'Operations', 'OPS'],
    ['dep_coaching', 'Coaching', 'COACH'],
    ['dep_frontoffice', 'Front Office', 'FO'],
    ['dep_finance', 'Finance', 'FIN'],
  ].map(([id, name, code]) => ({
    id: id!,
    name: name!,
    code: code!,
    description: '',
    parentDepartmentId: null,
    costCenter: null,
    active: true,
    createdAt: nowIso,
    updatedAt: nowIso,
  }));

  db.positions = [
    ['pos_director', 'Director', 'DIRECTOR'],
    ['pos_manager', 'Studio Manager', 'MANAGER'],
    ['pos_headcoach', 'Head Coach', 'SUPERVISOR'],
    ['pos_coach', 'Coach', 'STAFF'],
    ['pos_frontdesk', 'Front Desk Officer', 'STAFF'],
    ['pos_finance', 'Finance Officer', 'STAFF'],
  ].map(([id, title, level]) => ({
    id: id!,
    title: title!,
    level: level!,
    active: true,
    createdAt: nowIso,
  }));

  db.employmentStatuses = [
    ['PERMANENT', 'Permanent', 'Open-ended contract.'],
    ['PROBATION', 'Probation', 'First three months.'],
    ['CONTRACT', 'Contract', 'Fixed term.'],
    ['PART_TIME', 'Part time', 'Paid by the session.'],
  ].map(([code, name, description]) => ({
    id: code!,
    code: code!,
    name: name!,
    description: description!,
    active: true,
  }));

  db.shifts = [
    { id: 'shf_opening', name: 'Opening', startTime: '05:30:00', endTime: '13:30:00', sortOrder: 1 },
    { id: 'shf_middle', name: 'Middle', startTime: '10:00:00', endTime: '18:00:00', sortOrder: 2 },
    { id: 'shf_closing', name: 'Closing', startTime: '13:00:00', endTime: '21:00:00', sortOrder: 3 },
    { id: 'shf_office', name: 'Office', startTime: '09:00:00', endTime: '17:00:00', sortOrder: 4 },
  ].map((s) => ({
    ...s,
    breakMinutes: 60,
    lateToleranceMinutes: s.id === 'shf_office' ? 15 : 10,
    isOvernight: false,
    active: true,
  }));

  const staff: {
    id: string;
    number: string;
    name: string;
    email: string;
    department: string;
    position: string;
    branch: string;
    adminUserId: string | null;
    coachId: string | null;
    status: string;
    yearsAgo: number;
    reportsTo: string | null;
    shift: string;
    workDays: number[];
  }[] = [
    { id: 'emp_alya', number: 'NH-0001', name: 'Alya Santoso', email: 'alya@nuhabit.id', department: 'dep_ops', position: 'pos_director', branch: 'brn_senopati', adminUserId: 'adm_super', coachId: null, status: 'PERMANENT', yearsAgo: 4, reportsTo: null, shift: 'shf_office', workDays: [1, 2, 3, 4, 5] },
    { id: 'emp_raka', number: 'NH-0002', name: 'Raka Wibowo', email: 'raka@nuhabit.id', department: 'dep_ops', position: 'pos_manager', branch: 'brn_pik', adminUserId: 'adm_hq', coachId: null, status: 'PERMANENT', yearsAgo: 3, reportsTo: 'emp_alya', shift: 'shf_office', workDays: [1, 2, 3, 4, 5] },
    { id: 'emp_bima', number: 'NH-0003', name: 'Bima Prasetyo', email: 'bima@nuhabit.id', department: 'dep_ops', position: 'pos_manager', branch: 'brn_senopati', adminUserId: 'adm_bm', coachId: null, status: 'PERMANENT', yearsAgo: 2, reportsTo: 'emp_alya', shift: 'shf_middle', workDays: [1, 2, 3, 4, 5, 6] },
    { id: 'emp_nadia', number: 'NH-0004', name: 'Nadia Putri', email: 'nadia@nuhabit.id', department: 'dep_frontoffice', position: 'pos_frontdesk', branch: 'brn_senopati', adminUserId: 'adm_fd', coachId: null, status: 'PERMANENT', yearsAgo: 1, reportsTo: 'emp_bima', shift: 'shf_opening', workDays: [1, 2, 3, 4, 5, 6] },
    { id: 'emp_kevin', number: 'NH-0005', name: 'Kevin Hartono', email: 'kevin@nuhabit.id', department: 'dep_coaching', position: 'pos_headcoach', branch: 'brn_senopati', adminUserId: 'adm_coach', coachId: 'coa_kevin', status: 'PERMANENT', yearsAgo: 3, reportsTo: 'emp_bima', shift: 'shf_opening', workDays: [1, 2, 3, 4, 5] },
    { id: 'emp_sinta', number: 'NH-0006', name: 'Sinta Halim', email: 'sinta@nuhabit.id', department: 'dep_finance', position: 'pos_finance', branch: 'brn_senopati', adminUserId: 'adm_fin', coachId: null, status: 'PERMANENT', yearsAgo: 2, reportsTo: 'emp_alya', shift: 'shf_office', workDays: [1, 2, 3, 4, 5] },
    { id: 'emp_maya', number: 'NH-0007', name: 'Maya Kusuma', email: 'maya@nuhabit.id', department: 'dep_coaching', position: 'pos_coach', branch: 'brn_pik', adminUserId: null, coachId: 'coa_maya', status: 'PERMANENT', yearsAgo: 2, reportsTo: 'emp_raka', shift: 'shf_closing', workDays: [1, 2, 3, 4, 5] },
    { id: 'emp_rizky', number: 'NH-0008', name: 'Rizky Ramadhan', email: 'rizky@nuhabit.id', department: 'dep_coaching', position: 'pos_coach', branch: 'brn_senopati', adminUserId: null, coachId: 'coa_rizky', status: 'CONTRACT', yearsAgo: 1, reportsTo: 'emp_kevin', shift: 'shf_closing', workDays: [2, 3, 4, 5, 6] },
    { id: 'emp_tara', number: 'NH-0009', name: 'Tara Widjaja', email: 'tara@nuhabit.id', department: 'dep_coaching', position: 'pos_coach', branch: 'brn_pik', adminUserId: null, coachId: 'coa_tara', status: 'PART_TIME', yearsAgo: 1, reportsTo: 'emp_raka', shift: 'shf_middle', workDays: [3, 4, 5, 6] },
  ];

  db.employees = staff.map((e, index) => ({
    id: e.id,
    fullName: e.name,
    employeeNumber: e.number,
    email: e.email,
    phone: `+62811000000${index + 1}`,
    birthDate: null,
    gender: null,
    maritalStatus: null,
    address: null,
    joinDate: `${year - e.yearsAgo}-02-01`,
    endDate: null,
    employmentStatusCode: e.status,
    active: true,
    departmentId: e.department,
    positionId: e.position,
    branchId: e.branch,
    reportingTo: e.reportsTo,
    coachId: e.coachId,
    adminUserId: e.adminUserId,
    bankName: 'Bank Mandiri',
    bankAccount: `1370${String(index + 1).padStart(8, '0')}`,
    emergencyContactName: null,
    emergencyContactPhone: null,
    emergencyContactRelation: null,
    photoUrl: null,
    notes: null,
    createdAt: nowIso,
    updatedAt: nowIso,
  }));

  // A rest day is an explicit row with no shift, so the roster can tell
  // "resting" from "nobody ever rostered them".
  const patternFrom = `${year}-01-01`;
  db.employeeShifts = staff.flatMap((e) =>
    [1, 2, 3, 4, 5, 6, 7].map((day) => ({
      id: `esh_${e.id.slice(4)}_${day}`,
      employeeId: e.id,
      dayOfWeek: day,
      shiftId: e.workDays.includes(day) ? e.shift : null,
      effectiveFrom: patternFrom,
      effectiveTo: null,
    })),
  );

  db.leaveBalances = staff.map((e) => ({
    employeeId: e.id,
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
  }));

  db.holidays = [
    ['hol_newyear', '01-01', 'Tahun Baru Masehi', 'NATIONAL', false],
    ['hol_labour', '05-01', 'Hari Buruh Internasional', 'NATIONAL', false],
    ['hol_pancasila', '06-01', 'Hari Lahir Pancasila', 'NATIONAL', false],
    ['hol_independence', '08-17', 'Hari Kemerdekaan RI', 'NATIONAL', false],
    ['hol_christmas', '12-25', 'Hari Raya Natal', 'NATIONAL', false],
    ['hol_collective', '12-26', 'Cuti Bersama Natal', 'COLLECTIVE', true],
  ].map(([id, monthDay, name, type, deducts]) => ({
    id: `${id as string}_${year}`,
    date: `${year}-${monthDay as string}`,
    name: name as string,
    type: type as 'NATIONAL' | 'COLLECTIVE' | 'COMPANY',
    deductsLeave: deducts as boolean,
    note: null,
  }));

  // ── A fortnight of punches ────────────────────────────────────────────────
  const shiftsById = new Map(db.shifts.map((s) => [s.id, s]));
  const holidayDates = new Set(db.holidays.filter((h) => !h.deductsLeave).map((h) => h.date));
  // One person late each week and one absence, so the roster and the timesheet
  // have something other than a clean sheet to show. Nothing random.
  const lateness: Record<string, number> = { emp_nadia: 18, emp_rizky: 7 };

  for (let offset = -14; offset < 0; offset += 1) {
    const date = addDays(today, offset);
    if (holidayDates.has(date)) continue;

    for (const person of staff) {
      const row = db.employeeShifts.find(
        (r) => r.employeeId === person.id && r.dayOfWeek === isoDay(date),
      );
      if (!row?.shiftId) continue;
      const shift = shiftsById.get(row.shiftId);
      if (!shift) continue;
      if (person.id === 'emp_tara' && offset === -3) continue;

      const { start, end } = scheduledWindow(date, shift);
      const minutesLate = offset % 7 === -3 ? (lateness[person.id] ?? 0) : 0;
      const clockIn = new Date(new Date(start).getTime() + minutesLate * 60_000).toISOString();
      const late = computeLateness(clockIn, date, shift);

      db.attendance.push({
        id: `att_${person.id.slice(4)}_${date}`,
        employeeId: person.id,
        date,
        clockIn,
        clockOut: end,
        shiftId: shift.id,
        scheduledStart: start,
        scheduledEnd: end,
        workHours: workHours(clockIn, end, shift.breakMinutes),
        breakMinutes: shift.breakMinutes,
        status: late.isLate ? 'LATE' : 'PRESENT',
        isLate: late.isLate,
        lateMinutes: late.lateMinutes,
        overtimeHours: 0,
        notes: null,
        createdAt: clockIn,
        updatedAt: end,
      });
    }
  }

  // One decided request and one waiting, so the approval screen is not empty.
  db.leaves = [
    {
      id: 'lve_demo_approved',
      employeeId: 'emp_maya',
      type: 'ANNUAL',
      startDate: addDays(today, -9),
      endDate: addDays(today, -8),
      totalDays: 2,
      reason: 'Family trip to Bandung.',
      attachmentUrl: null,
      status: 'APPROVED',
      approvedBy: 'adm_hq',
      approvedAt: nowIso,
      rejectionReason: null,
      createdAt: nowIso,
      updatedAt: nowIso,
    },
    {
      id: 'lve_demo_pending',
      employeeId: 'emp_nadia',
      type: 'ANNUAL',
      startDate: addDays(today, 7),
      endDate: addDays(today, 9),
      totalDays: 3,
      reason: "Sister's wedding.",
      attachmentUrl: null,
      status: 'PENDING',
      approvedBy: null,
      approvedAt: null,
      rejectionReason: null,
      createdAt: nowIso,
      updatedAt: nowIso,
    },
  ];
  const mayaBalance = db.leaveBalances.find((b) => b.employeeId === 'emp_maya');
  if (mayaBalance) mayaBalance.annualUsed = 2;
}

function isoDay(date: string): number {
  const day = new Date(`${date}T00:00:00Z`).getUTCDay();
  return day === 0 ? 7 : day;
}
