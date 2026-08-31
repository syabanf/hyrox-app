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
  Exercise,
  Follow,
  Gate,
  Gear,
  GeneratedWorkout,
  IsoDate,
  Kudos,
  Member,
  MemberNotification,
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

/**
 * Repository ports. The mock infrastructure implements them over an in-memory
 * store today; a real backend implements them over a database tomorrow.
 * Everything is synchronous because use cases run in-process against the mock —
 * the HTTP boundary (MSW today, a server later) is where async lives.
 */
export interface MemberRepository {
  byId(id: string): Member | null;
  byIdentifier(identifier: string): Member | null;
  all(): Member[];
  save(member: Member): void;
}

export interface LedgerRepository {
  byId(id: string): CreditLedgerEntry | null;
  forMember(memberId: string): CreditLedgerEntry[];
  all(): CreditLedgerEntry[];
  append(entry: CreditLedgerEntry): void;
  hasReversalOf(entryId: string): boolean;
  lotsFor(memberId: string): TopUpLot[];
  allLots(): TopUpLot[];
  addLot(lot: TopUpLot): void;
}

export interface PaymentRepository {
  byId(id: string): Payment | null;
  forMember(memberId: string): Payment[];
  all(): Payment[];
  save(payment: Payment): void;
}

export interface PackageRepository {
  byId(id: string): CreditPackage | null;
  all(): CreditPackage[];
  save(pkg: CreditPackage): void;
}

export interface VoucherRepository {
  byId(id: string): Voucher | null;
  byCode(code: string): Voucher | null;
  all(): Voucher[];
  save(voucher: Voucher): void;
  redemptionCount(voucherId: string): number;
  memberRedemptionCount(voucherId: string, memberId: string): number;
  addRedemption(redemption: VoucherRedemption): void;
  redemptions(): VoucherRedemption[];
}

export interface ClassTypeRepository {
  byId(id: string): ClassType | null;
  all(): ClassType[];
  save(classType: ClassType): void;
}

export interface SessionRepository {
  byId(id: string): ClassSession | null;
  all(): ClassSession[];
  save(session: ClassSession): void;
}

export interface BookingRepository {
  byId(id: string): Booking | null;
  forMember(memberId: string): Booking[];
  forSession(sessionId: string): Booking[];
  all(): Booking[];
  save(booking: Booking): void;
}

export interface QrTokenRepository {
  byToken(token: string): QrToken | null;
  save(token: QrToken): void;
}

export interface AccessLogRepository {
  forMember(memberId: string): AccessLog[];
  all(): AccessLog[];
  append(log: AccessLog): void;
  lastAllowedAt(memberId: string): IsoDate | null;
}

export interface NotificationRepository {
  forMember(memberId: string): MemberNotification[];
  append(notification: MemberNotification): void;
  markAllRead(memberId: string, now: IsoDate): void;
}

export interface CampaignRepository {
  byId(id: string): Campaign | null;
  all(): Campaign[];
  save(campaign: Campaign): void;
}

export interface RulesRepository {
  defaults(): BusinessRules;
  saveDefaults(rules: BusinessRules): void;
  overrideFor(branchId: string): Partial<BusinessRules> | null;
}

export interface AuditRepository {
  append(event: AuditEvent): void;
  all(): AuditEvent[];
}

export interface BranchRepository {
  byId(id: string): Branch | null;
  all(): Branch[];
  save(branch: Branch): void;
}

export interface GateRepository {
  byId(id: string): Gate | null;
  all(): Gate[];
  save(gate: Gate): void;
}

export interface CoachRepository {
  byId(id: string): Coach | null;
  all(): Coach[];
  save(coach: Coach): void;
}

export interface AdminUserRepository {
  byId(id: string): AdminUser | null;
  all(): AdminUser[];
  save(user: AdminUser): void;
}

/** Strava-style athlete module: activities, social graph, segments, gear. */
export interface AthleteStore {
  activities: {
    byId(id: string): Activity | null;
    forMember(memberId: string): Activity[];
    all(): Activity[];
    save(activity: Activity): void;
  };
  follows: {
    all(): Follow[];
    isFollowing(followerId: string, followeeId: string): boolean;
    toggle(followerId: string, followeeId: string): boolean;
  };
  kudos: {
    forActivity(activityId: string): Kudos[];
    has(activityId: string, memberId: string): boolean;
    toggle(activityId: string, memberId: string, now: IsoDate): boolean;
  };
  comments: {
    forActivity(activityId: string): ActivityComment[];
    add(comment: ActivityComment): void;
  };
  segments: {
    all(): Segment[];
    byId(id: string): Segment | null;
  };
  efforts: {
    forSegment(segmentId: string): SegmentEffort[];
    forMember(memberId: string): SegmentEffort[];
    forActivity(activityId: string): SegmentEffort[];
    add(effort: SegmentEffort): void;
  };
  challenges: {
    all(): Challenge[];
    byId(id: string): Challenge | null;
    participants(challengeId: string): string[];
    isJoined(challengeId: string, memberId: string): boolean;
    join(challengeId: string, memberId: string): void;
  };
  clubs: {
    all(): Club[];
    byId(id: string): Club | null;
    save(club: Club): void;
  };
  gear: {
    byId(id: string): Gear | null;
    forMember(memberId: string): Gear[];
    save(gear: Gear): void;
  };
  settings: {
    get(memberId: string): AthleteSettings;
    save(memberId: string, settings: AthleteSettings): void;
  };
  remindersSent: {
    has(bookingId: string): boolean;
    add(bookingId: string): void;
  };
  routes: {
    byId(id: string): Route | null;
    forMember(memberId: string): Route[];
    save(route: Route): void;
    remove(id: string): void;
  };
}

/** HYROX workout generator + active sessions (blueprint phase 3). */
export interface WorkoutStore {
  exercises: { all(): Exercise[]; byId(id: string): Exercise | null };
  substitutions: { all(): SubstitutionRule[] };
  workouts: {
    byId(id: string): GeneratedWorkout | null;
    forMember(memberId: string): GeneratedWorkout[];
    save(workout: GeneratedWorkout): void;
  };
  sessions: {
    byId(id: string): WorkoutSession | null;
    forMember(memberId: string): WorkoutSession[];
    save(session: WorkoutSession): void;
  };
}

/** Race ecosystem (blueprint phase 4). */
export interface RaceStore {
  events: { all(): RaceEvent[]; byId(id: string): RaceEvent | null };
  userRaces: {
    byId(id: string): UserRace | null;
    forMember(memberId: string): UserRace[];
    forEvent(raceEventId: string): UserRace[];
    save(userRace: UserRace): void;
  };
}

export interface Clock {
  now(): IsoDate;
}

export interface IdGenerator {
  next(prefix: string): string;
}

export interface UseCaseDeps {
  members: MemberRepository;
  ledger: LedgerRepository;
  payments: PaymentRepository;
  packages: PackageRepository;
  vouchers: VoucherRepository;
  classTypes: ClassTypeRepository;
  sessions: SessionRepository;
  bookings: BookingRepository;
  qrTokens: QrTokenRepository;
  accessLogs: AccessLogRepository;
  notifications: NotificationRepository;
  campaigns: CampaignRepository;
  rules: RulesRepository;
  audit: AuditRepository;
  branches: BranchRepository;
  gates: GateRepository;
  coaches: CoachRepository;
  adminUsers: AdminUserRepository;
  athlete: AthleteStore;
  workout: WorkoutStore;
  races: RaceStore;
  clock: Clock;
  ids: IdGenerator;
}
