import type {
  Attendance,
  Employee,
  EmployeeShift,
  Holiday,
  HolidayType,
  Leave,
  LeaveBalance,
  LeaveType,
  OvertimeRequest,
  OvertimeSource,
  RosterStatus,
} from '@nuhabit/domain';

// ── Views ───────────────────────────────────────────────────────────────────

/** An employee with what it points at named, so a table row is readable. */
export interface EmployeeView extends Employee {
  departmentName: string | null;
  positionTitle: string | null;
  branchName: string | null;
  reportingToName: string | null;
  coachName: string | null;
}

/** A pattern row with its shift resolved and its weekday spelled out. */
export interface ScheduleRowView extends EmployeeShift {
  shiftName: string | null;
  dayName: string;
}

/** Everything one person's page shows. */
export interface EmployeeDetailView {
  employee: EmployeeView;
  balance: LeaveBalance;
  schedule: ScheduleRowView[];
  attendance: Attendance[];
  leaves: Leave[];
  overtime: OvertimeRequest[];
  directReports: EmployeeView[];
}

export interface AttendanceView extends Attendance {
  employeeName: string;
  employeeNumber: string;
  shiftName: string | null;
}

export interface ExcludedHoliday {
  date: string;
  name: string;
}

export interface LeaveView extends Leave {
  employeeName: string;
  employeeNumber: string;
  /** Named days inside the range that did not cost the employee anything. */
  excludedHolidays?: ExcludedHoliday[];
}

export interface OvertimeView extends OvertimeRequest {
  employeeName: string;
  employeeNumber: string;
}

/** One employee's working day as it stands right now. */
export interface StaffRosterEntryView {
  employeeId: string;
  employeeName: string;
  employeeNumber: string;
  departmentId: string | null;
  branchId: string | null;
  shiftName: string | null;
  scheduledStart: string | null;
  scheduledEnd: string | null;
  attendance: Attendance | null;
  status: RosterStatus;
  leaveType: LeaveType | null;
  holiday: Holiday | null;
}

export interface AttendanceSummaryView {
  present: number;
  late: number;
  absent: number;
  onLeave: number;
  totalHours: number;
  lateMinutes: number;
}

/** The HR dashboard: the numbers a manager checks each morning. */
export interface HrOverviewView {
  date: string;
  activeEmployees: number;
  totalEmployees: number;
  today: AttendanceSummaryView;
  month: AttendanceSummaryView;
  pendingLeave: number;
  pendingOvertime: number;
  onLeaveToday: number;
  expectedToday: number;
  missingToday: number;
  nextHolidays: Holiday[];
}

/**
 * A signed-in staff member's own day. Every field is optional because a login
 * is not necessarily on the payroll.
 */
export interface HrSelfView {
  employee: EmployeeView | null;
  balance: LeaveBalance | null;
  today: Attendance | null;
  date: string;
  shiftName: string | null;
  expectedStart: string | null;
  expectedEnd: string | null;
  restDay: boolean;
  scheduled: boolean;
  leaves: Leave[];
}

// ── Inputs ──────────────────────────────────────────────────────────────────

/**
 * A whole employee record. Updates replace rather than patch: the admin form
 * holds the entire person, and a partial write of a record carrying bank
 * details and next of kin is a good way to lose one of them.
 */
export interface UpsertEmployeeInput {
  fullName: string;
  employeeNumber: string;
  email: string;
  phone: string;
  birthDate?: string | null;
  gender?: 'MALE' | 'FEMALE' | 'OTHER' | null;
  maritalStatus?: string | null;
  address?: string | null;
  joinDate: string;
  endDate?: string | null;
  employmentStatusCode: string;
  active?: boolean;
  departmentId?: string | null;
  positionId?: string | null;
  branchId?: string | null;
  reportingTo?: string | null;
  coachId?: string | null;
  adminUserId?: string | null;
  bankName?: string | null;
  bankAccount?: string | null;
  emergencyContactName?: string | null;
  emergencyContactPhone?: string | null;
  emergencyContactRelation?: string | null;
  photoUrl?: string | null;
  notes?: string | null;
}

export interface UpsertDepartmentInput {
  name: string;
  code: string;
  description?: string;
  parentDepartmentId?: string | null;
  costCenter?: string | null;
  active?: boolean;
}

export interface UpsertPositionInput {
  title: string;
  level: string;
  active?: boolean;
}

export interface UpsertShiftInput {
  name: string;
  startTime: string;
  endTime: string;
  breakMinutes: number;
  lateToleranceMinutes: number;
  isOvernight?: boolean;
  active?: boolean;
  sortOrder?: number;
}

export interface AssignShiftInput {
  dayOfWeek: number;
  /** Omitted or null makes the day an explicit rest day. */
  shiftId?: string | null;
  effectiveFrom: string;
  effectiveTo?: string | null;
}

export interface ClockInput {
  employeeId: string;
  /** Defaults to now; staff correcting a missed punch send a timestamp. */
  at?: string;
  notes?: string | null;
  remote?: boolean;
}

export interface MarkAttendanceInput {
  employeeId: string;
  date: string;
  status: string;
  clockIn?: string | null;
  clockOut?: string | null;
  notes?: string | null;
}

export interface RequestLeaveInput {
  employeeId: string;
  type: LeaveType;
  startDate: string;
  endDate: string;
  reason: string;
  attachmentUrl?: string | null;
}

export interface RequestOvertimeInput {
  employeeId: string;
  date: string;
  startTime: string;
  endTime: string;
  source?: OvertimeSource;
  reason?: string | null;
}

export interface UpsertHolidayInput {
  date: string;
  name: string;
  type: HolidayType;
  deductsLeave: boolean;
  note?: string | null;
  draft?: boolean;
}

export interface SetLeaveAllowanceInput {
  year: number;
  annualTotal: number;
}
