/**
 * The people side of the studio: who works here, when they are expected,
 * whether they turned up, and the leave they are owed.
 *
 * An employee is deliberately neither a member nor an admin user. The same
 * person can be all three, and each of the three ends independently — leaving
 * the company does not cancel a membership, and losing a login does not erase
 * a timesheet.
 *
 * The rules that decide any of this live in the Go backend, which owns the
 * arithmetic. What is here is the shape of the data and the labels the panel
 * puts on it.
 */

/** A calendar date as `YYYY-MM-DD`, free of timezone drift. */
export type CalendarDate = string;

/** A wall-clock time as `HH:MM:SS`. */
export type WallClock = string;

export interface Department {
  id: string;
  name: string;
  code: string;
  description: string;
  parentDepartmentId: string | null;
  costCenter: string | null;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Position {
  id: string;
  title: string;
  level: string;
  active: boolean;
  createdAt: string;
}

export interface EmploymentStatus {
  id: string;
  code: string;
  name: string;
  description: string;
  active: boolean;
}

export const MARITAL_STATUSES = ['SINGLE', 'MARRIED', 'DIVORCED', 'WIDOWED'] as const;
export type MaritalStatus = (typeof MARITAL_STATUSES)[number];

export interface Employee {
  id: string;
  fullName: string;
  /** The studio's own identifier, unique and human-quotable. */
  employeeNumber: string;
  email: string;
  phone: string;
  birthDate: CalendarDate | null;
  gender: 'MALE' | 'FEMALE' | 'OTHER' | null;
  maritalStatus: MaritalStatus | null;
  address: string | null;
  joinDate: CalendarDate;
  endDate: CalendarDate | null;
  employmentStatusCode: string;
  active: boolean;
  departmentId: string | null;
  positionId: string | null;
  branchId: string | null;
  /** Another employee: this is what makes the org chart. */
  reportingTo: string | null;
  /** The coach they teach as, where they teach. */
  coachId: string | null;
  adminUserId: string | null;
  bankName: string | null;
  bankAccount: string | null;
  emergencyContactName: string | null;
  emergencyContactPhone: string | null;
  emergencyContactRelation: string | null;
  photoUrl: string | null;
  notes: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Shift {
  id: string;
  name: string;
  startTime: WallClock;
  endTime: WallClock;
  breakMinutes: number;
  /**
   * Grace before an arrival counts as late. It decides *whether* someone is
   * late; the minutes themselves are counted from the shift's start time.
   */
  lateToleranceMinutes: number;
  isOvernight: boolean;
  active: boolean;
  sortOrder: number;
}

/**
 * One row of a weekly pattern. A row with no `shiftId` is a scheduled rest
 * day, which is different from having no pattern at all: the roster needs to
 * tell "resting" from "unrostered".
 */
export interface EmployeeShift {
  id: string;
  employeeId: string;
  /** ISO weekday: 1 is Monday, 7 is Sunday. */
  dayOfWeek: number;
  shiftId: string | null;
  effectiveFrom: CalendarDate;
  effectiveTo: CalendarDate | null;
}

export const ATTENDANCE_STATUSES = [
  'PRESENT',
  'LATE',
  'ABSENT',
  'HALF_DAY',
  'REMOTE',
  'ON_LEAVE',
  'HOLIDAY',
  'REST_DAY',
] as const;
export type AttendanceStatus = (typeof ATTENDANCE_STATUSES)[number];

export interface Attendance {
  id: string;
  employeeId: string;
  date: CalendarDate;
  clockIn: string | null;
  clockOut: string | null;
  shiftId: string | null;
  /** Frozen at clock-in, so editing a shift never rewrites a past Tuesday. */
  scheduledStart: string | null;
  scheduledEnd: string | null;
  workHours: number;
  breakMinutes: number;
  status: AttendanceStatus;
  isLate: boolean;
  lateMinutes: number;
  overtimeHours: number;
  notes: string | null;
  createdAt: string;
  updatedAt: string;
}

export const HOLIDAY_TYPES = ['NATIONAL', 'COLLECTIVE', 'COMPANY'] as const;
export type HolidayType = (typeof HOLIDAY_TYPES)[number];

export interface Holiday {
  id: string;
  date: CalendarDate;
  name: string;
  type: HolidayType;
  /**
   * The whole reason this exists: Indonesian collective leave (*cuti bersama*)
   * comes out of the annual allowance, and a national holiday does not.
   */
  deductsLeave: boolean;
  note: string | null;
}

export const LEAVE_TYPES = [
  'ANNUAL',
  'SICK',
  'MATERNITY',
  'PATERNITY',
  'UNPAID',
  'EMERGENCY',
  'PILGRIMAGE',
  'MENSTRUAL',
] as const;
export type LeaveType = (typeof LEAVE_TYPES)[number];

export const LEAVE_STATUSES = ['PENDING', 'APPROVED', 'REJECTED', 'CANCELLED'] as const;
export type LeaveStatus = (typeof LEAVE_STATUSES)[number];

export interface Leave {
  id: string;
  employeeId: string;
  type: LeaveType;
  startDate: CalendarDate;
  endDate: CalendarDate;
  /** Working days the request costs; weekends and free holidays are excluded. */
  totalDays: number;
  reason: string;
  attachmentUrl: string | null;
  status: LeaveStatus;
  approvedBy: string | null;
  approvedAt: string | null;
  rejectionReason: string | null;
  createdAt: string;
  updatedAt: string;
}

/**
 * One employee's allowance for one year. Only annual leave is rationed; the
 * other counters exist so HR can see usage, not to refuse a request.
 */
export interface LeaveBalance {
  employeeId: string;
  year: number;
  annualTotal: number;
  annualUsed: number;
  sickUsed: number;
  unpaidUsed: number;
  maternityUsed: number;
  paternityUsed: number;
  emergencyUsed: number;
  pilgrimageUsed: number;
  menstrualUsed: number;
}

export const OVERTIME_STATUSES = ['PENDING', 'APPROVED', 'REJECTED', 'CANCELLED'] as const;
export type OvertimeStatus = (typeof OVERTIME_STATUSES)[number];

/** Who asked: company-directed overtime is payable on different terms. */
export type OvertimeSource = 'EMPLOYEE' | 'COMPANY';

export interface OvertimeRequest {
  id: string;
  employeeId: string;
  date: CalendarDate;
  startTime: WallClock;
  endTime: WallClock;
  hours: number;
  source: OvertimeSource;
  status: OvertimeStatus;
  reason: string | null;
  decidedBy: string | null;
  decidedAt: string | null;
  rejectionReason: string | null;
  createdAt: string;
  updatedAt: string;
}

/** The roster's own vocabulary: where a person stands right now. */
export const ROSTER_STATUSES = [
  'WORKING',
  'DONE',
  'LATE',
  'EXPECTED',
  'MISSING',
  'ON_LEAVE',
  'HOLIDAY',
  'REST_DAY',
  'UNSCHEDULED',
] as const;
export type RosterStatus = (typeof ROSTER_STATUSES)[number];

/** ISO weekday names, indexed 1–7 so `WEEKDAYS[date.getISODay()]` reads right. */
export const WEEKDAYS = [
  '',
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
  'Sunday',
] as const;

/** The remaining annual allowance — the only counter that can run out. */
export function annualRemaining(balance: LeaveBalance): number {
  return balance.annualTotal - balance.annualUsed;
}

/** `HH:MM:SS` trimmed to the `HH:MM` a person reads. */
export function shortTime(clock: WallClock): string {
  return clock.slice(0, 5);
}
