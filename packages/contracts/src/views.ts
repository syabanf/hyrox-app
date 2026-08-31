import type {
  AccessLog,
  AdminRole,
  AdminUser,
  AuditEvent,
  Booking,
  BookingDenialReason,
  Branch,
  BusinessRules,
  Campaign,
  ClassSession,
  ClassType,
  Coach,
  CreditLedgerEntry,
  CreditPackage,
  Gate,
  GateDenialReason,
  GateEntryKind,
  Member,
  MemberNotification,
  Payment,
  Permission,
  TopUpLot,
  Voucher,
} from '@hyrox/domain';

/** Response envelope for errors. */
export interface ApiErrorBody {
  error: { code: string; message: string };
}

// ── Auth ────────────────────────────────────────────────────────────────────
export interface OtpChallengeView {
  challengeId: string;
  memberExists: boolean;
  hint: string;
}
export interface MemberSessionView {
  token: string;
  member: Member;
}
export interface AdminSessionView {
  token: string;
  user: AdminUser;
  permissions: Permission[];
}
export interface AdminUserView extends AdminUser {
  roleLabel: AdminRole;
}

// ── Wallet ──────────────────────────────────────────────────────────────────
export interface WalletView {
  balance: number;
  expiringCredits: number;
  lowBalance: boolean;
  lowBalanceThreshold: number;
  entries: CreditLedgerEntry[];
  lots: TopUpLot[];
}
export interface MeView {
  member: Member;
  balance: number;
  expiringCredits: number;
  lowBalance: boolean;
  unreadNotifications: number;
}
export interface TopUpView {
  payment: Payment;
  discountIdr: number;
}
export interface VoucherQuoteView {
  voucher: Voucher;
  discountIdr: number;
}

// ── Classes / bookings ──────────────────────────────────────────────────────
export interface SessionView {
  session: ClassSession;
  classTypeName: string;
  coachName: string;
  branchName: string;
  confirmedCount: number;
  waitlistCount: number;
  spotsLeft: number;
  myBooking: Pick<Booking, 'id' | 'status' | 'waitlistPosition' | 'promotionOfferedAt'> | null;
}
export interface BookingView {
  booking: Booking;
  session: ClassSession;
  classTypeName: string;
  coachName: string;
  branchName: string;
}
export interface BookResultView {
  booking: Booking;
  decision: 'CONFIRMED' | 'WAITLIST';
}
export type BookErrorCode = BookingDenialReason;
export interface CancelResultView {
  booking: Booking;
  outcome: 'RELEASED' | 'LATE';
  penaltyCredits: number;
  promotedMemberName: string | null;
}

// ── QR / access ─────────────────────────────────────────────────────────────
export interface QrView {
  token: string;
  issuedAt: string;
  expiresAt: string;
  ttlSeconds: number;
}
export interface ScanResultView {
  decision: 'ALLOWED' | 'DENIED';
  reason: GateDenialReason | null;
  entryKind: GateEntryKind | null;
  memberName: string | null;
  remainingCredits: number | null;
  gateName: string;
  accessLog: AccessLog;
}
export interface AccessLogView {
  log: AccessLog;
  memberName: string | null;
  gateName: string;
  branchName: string;
}

// ── Admin members ───────────────────────────────────────────────────────────
export interface MemberSummaryView {
  member: Member;
  balance: number;
  totalVisits: number;
  lastVisitAt: string | null;
}
export interface MemberDetailView {
  member: Member;
  balance: number;
  expiringCredits: number;
  totalVisits: number;
  lastVisitAt: string | null;
  upcomingBookings: BookingView[];
  entries: CreditLedgerEntry[];
  lots: TopUpLot[];
  bookings: BookingView[];
  visits: AccessLogView[];
  payments: Payment[];
  audit: AuditEvent[];
}

// ── Operations ──────────────────────────────────────────────────────────────
export interface RosterEntryView {
  booking: Booking;
  memberName: string;
  memberStatus: Member['status'];
}
export interface SessionDetailAdminView extends SessionView {
  roster: RosterEntryView[];
}

// ── Commercial ──────────────────────────────────────────────────────────────
export interface PaymentView {
  payment: Payment;
  memberName: string;
  packageName: string;
}
export interface VoucherView {
  voucher: Voucher;
  redemptionCount: number;
}
export interface PackageStatsView {
  pkg: CreditPackage;
  purchaseCount: number;
  revenueIdr: number;
}

// ── Reports / dashboard ─────────────────────────────────────────────────────
export interface DashboardStatsView {
  visitorsToday: number;
  classesToday: number;
  revenueTodayIdr: number;
  topUpsTodayIdr: number;
  outstandingCredits: number;
  expiringCredits: number;
  activeMembers: number;
  todaySessions: SessionView[];
}
export interface DailyPointView {
  date: string;
  value: number;
}
export interface SalesReportView {
  totalIdr: number;
  byDay: DailyPointView[];
  byChannel: { channel: string; totalIdr: number }[];
  byPackage: PackageStatsView[];
}
export interface VisitsReportView {
  total: number;
  byDay: DailyPointView[];
  denied: number;
  offline: number;
}
export interface CreditsReportView {
  outstandingTotal: number;
  expiringTotal: number;
  perMember: { memberId: string; memberName: string; balance: number; expiring: number }[];
}

export interface ClassesReportView {
  perType: {
    classTypeId: string;
    classTypeName: string;
    sessionsHeld: number;
    booked: number;
    attended: number;
    noShows: number;
    attendanceRate: number;
  }[];
  recentNoShows: { memberName: string; classTypeName: string; startsAt: string }[];
}

export interface SegmentPreviewView {
  count: number;
  sample: string[];
}

// ── Config ──────────────────────────────────────────────────────────────────
export interface RulesView {
  defaults: BusinessRules;
  branchOverrides: { branchId: string; branchName: string; override: Partial<BusinessRules> }[];
}

// Re-exported reference data views
export type { Branch, Gate, Coach, ClassType, Campaign, MemberNotification };
