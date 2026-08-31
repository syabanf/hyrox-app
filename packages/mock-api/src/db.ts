import type {
  AccessLog,
  Activity,
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
  CreditPackage,
  Follow,
  Gate,
  Gear,
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
} from '@hyrox/domain';
import { DEFAULT_BUSINESS_RULES } from '@hyrox/domain';

/** Bump to invalidate persisted localStorage snapshots after seed/schema changes. */
export const SEED_VERSION = 4;

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
}

export function createEmptyDb(now: string): MockDb {
  return {
    seedVersion: SEED_VERSION,
    seededAt: now,
    organization: { id: 'org_hyrox', name: 'HYROX Studio Jakarta' },
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
  };
}
