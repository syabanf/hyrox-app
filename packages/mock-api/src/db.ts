import type {
  AccessLog,
  Activity,
  Attendance,
  ActivityComment,
  AdminUser,
  AthleteSettings,
  AuditEvent,
  Booking,
  Branch,
  BusinessRules,
  Campaign,
  Challenge,
  ClassSession,
  ClassType,
  Club,
  Coach,
  CreditLedgerEntry,
  Department,
  Employee,
  EmployeeShift,
  EmploymentStatus,
  Holiday,
  Leave,
  LeaveBalance,
  OvertimeRequest,
  Position,
  Shift,
  CreditPackage,
  Follow,
  Gate,
  Gear,
  IncentivePayout,
  IncentiveScheme,
  Kudos,
  Member,
  MemberNotification,
  Organization,
  Exercise,
  GeneratedWorkout,
  Payment,
  QrToken,
  RaceEvent,
  Route,
  Segment,
  SegmentEffort,
  SubstitutionRule,
  TopUpLot,
  UserRace,
  Voucher,
  VoucherRedemption,
  WorkoutSession,
} from '@nuhabit/domain';
import { DEFAULT_BUSINESS_RULES } from '@nuhabit/domain';

/** Bump to invalidate persisted localStorage snapshots after seed/schema changes. */
export const SEED_VERSION = 12;

export interface MockDb {
  seedVersion: number;
  seededAt: string;
  organization: Organization;
  branches: Branch[];
  gates: Gate[];
  coaches: Coach[];
  classTypes: ClassType[];
  sessions: ClassSession[];
  members: Member[];
  adminUsers: AdminUser[];
  ledger: CreditLedgerEntry[];
  lots: TopUpLot[];
  payments: Payment[];
  packages: CreditPackage[];
  vouchers: Voucher[];
  redemptions: VoucherRedemption[];
  bookings: Booking[];
  qrTokens: QrToken[];
  accessLogs: AccessLog[];
  notifications: MemberNotification[];
  campaigns: Campaign[];
  audit: AuditEvent[];
  rules: BusinessRules;
  // Coach incentives (IDR payroll, ledger-independent)
  incentiveSchemes: IncentiveScheme[];
  incentivePayouts: IncentivePayout[];
  otpChallenges: Record<string, string>;
  counters: Record<string, number>;
  // Athlete module (Strava-style)
  activities: Activity[];
  follows: Follow[];
  kudos: Kudos[];
  activityComments: ActivityComment[];
  segments: Segment[];
  segmentEfforts: SegmentEffort[];
  challenges: Challenge[];
  challengeJoins: { challengeId: string; memberId: string }[];
  clubs: Club[];
  gear: Gear[];
  athleteSettings: Record<string, AthleteSettings>;
  remindersSent: string[];
  routes: Route[];
  // HYROX workout module (phase 3)
  exercises: Exercise[];
  substitutions: SubstitutionRule[];
  workouts: GeneratedWorkout[];
  workoutSessions: WorkoutSession[];
  // Race ecosystem (phase 4)
  raceEvents: RaceEvent[];
  userRaces: UserRace[];
  // HRIS — the people side. The Go backend owns these rules; what is here is a
  // demo stand-in so the panel runs with no server behind it.
  departments: Department[];
  positions: Position[];
  employmentStatuses: EmploymentStatus[];
  employees: Employee[];
  shifts: Shift[];
  employeeShifts: EmployeeShift[];
  attendance: Attendance[];
  holidays: Holiday[];
  leaves: Leave[];
  leaveBalances: LeaveBalance[];
  overtimeRequests: OvertimeRequest[];
}

export function createEmptyDb(now: string): MockDb {
  return {
    seedVersion: SEED_VERSION,
    seededAt: now,
    organization: { id: 'org_nuhabit', name: 'NuHabit Studio Jakarta' },
    branches: [],
    gates: [],
    coaches: [],
    classTypes: [],
    sessions: [],
    members: [],
    adminUsers: [],
    ledger: [],
    lots: [],
    payments: [],
    packages: [],
    vouchers: [],
    redemptions: [],
    bookings: [],
    qrTokens: [],
    accessLogs: [],
    notifications: [],
    campaigns: [],
    audit: [],
    rules: { ...DEFAULT_BUSINESS_RULES },
    incentiveSchemes: [],
    incentivePayouts: [],
    otpChallenges: {},
    counters: {},
    activities: [],
    follows: [],
    kudos: [],
    activityComments: [],
    segments: [],
    segmentEfforts: [],
    challenges: [],
    challengeJoins: [],
    clubs: [],
    gear: [],
    athleteSettings: {},
    remindersSent: [],
    routes: [],
    exercises: [],
    substitutions: [],
    workouts: [],
    workoutSessions: [],
    raceEvents: [],
    userRaces: [],
    departments: [],
    positions: [],
    employmentStatuses: [],
    employees: [],
    shifts: [],
    employeeShifts: [],
    attendance: [],
    holidays: [],
    leaves: [],
    leaveBalances: [],
    overtimeRequests: [],
  };
}
