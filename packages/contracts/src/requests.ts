import { z } from 'zod';

// ── Auth ────────────────────────────────────────────────────────────────────
export const OtpRequestSchema = z.object({
  identifier: z.string().min(3), // phone or email
});

export const OtpVerifySchema = z.object({
  challengeId: z.string(),
  code: z.string().min(4).max(8),
});

export const RegisterMemberSchema = z.object({
  fullName: z.string().min(2),
  email: z.string().email(),
  phone: z.string().min(6),
  dateOfBirth: z.string().nullable().default(null),
  gender: z.enum(['MALE', 'FEMALE', 'OTHER']).nullable().default(null),
  emergencyContact: z
    .object({ name: z.string().min(1), phone: z.string().min(6), relation: z.string().min(1) })
    .nullable()
    .default(null),
  preferredBranchId: z.string().nullable().default(null),
  waiverAccepted: z.literal(true),
  termsAccepted: z.literal(true),
});

export const AdminLoginSchema = z.object({ userId: z.string() });

// ── Profile ─────────────────────────────────────────────────────────────────
export const UpdateProfileSchema = z.object({
  fullName: z.string().min(2).optional(),
  email: z.string().email().optional(),
  phone: z.string().min(6).optional(),
  emergencyContact: z
    .object({ name: z.string(), phone: z.string(), relation: z.string() })
    .nullable()
    .optional(),
  preferredBranchId: z.string().nullable().optional(),
  /** Small data-URL image (the app resizes before upload). */
  avatarUrl: z.string().max(300_000).nullable().optional(),
});

// ── Wallet / commercial ─────────────────────────────────────────────────────
export const TopUpRequestSchema = z.object({
  packageId: z.string(),
  voucherCode: z.string().nullable().default(null),
  channel: z.enum(['QRIS', 'EWALLET', 'VIRTUAL_ACCOUNT', 'CARD']),
});

export const ValidateVoucherSchema = z.object({
  code: z.string().min(1),
  packageId: z.string(),
});

export const AdjustCreditsSchema = z.object({
  amount: z.number().int().refine((n) => n !== 0, 'amount must be non-zero'),
  reason: z.string().min(3),
});

export const ReverseEntrySchema = z.object({ reason: z.string().min(3) });
export const RefundPaymentSchema = z.object({ reason: z.string().min(3) });

export const UpsertPackageSchema = z.object({
  name: z.string().min(2),
  credits: z.number().int().positive(),
  priceIdr: z.number().int().positive(),
  validityDays: z.number().int().positive(),
  branchId: z.string().nullable().default(null),
  purchaseLimitPerMember: z.number().int().positive().nullable().default(null),
  status: z.enum(['ACTIVE', 'ARCHIVED']).default('ACTIVE'),
});

export const UpsertVoucherSchema = z.object({
  code: z.string().min(3),
  type: z.enum(['FIXED_IDR', 'PERCENT']),
  value: z.number().positive(),
  startsAt: z.string(),
  endsAt: z.string(),
  usageLimit: z.number().int().positive().nullable().default(null),
  perMemberLimit: z.number().int().positive().nullable().default(null),
  eligibleSegment: z.enum(['ALL', 'NEW_MEMBERS']).default('ALL'),
  applicablePackageIds: z.array(z.string()).nullable().default(null),
});

export const VoucherStatusActionSchema = z.object({
  status: z.enum(['DRAFT', 'SCHEDULED', 'ACTIVE', 'EXPIRED', 'DISABLED']),
});

// ── Members (admin) ─────────────────────────────────────────────────────────
export const UpdateMemberAdminSchema = z.object({
  status: z.enum(['ACTIVE', 'SUSPENDED', 'INACTIVE', 'ARCHIVED']).optional(),
  notes: z.string().nullable().optional(),
  preferredBranchId: z.string().nullable().optional(),
  reason: z.string().optional(),
});

// ── Operations ──────────────────────────────────────────────────────────────
export const UpsertClassTypeSchema = z.object({
  name: z.string().min(2),
  description: z.string().default(''),
  defaultDurationMin: z.number().int().positive(),
  defaultCreditCost: z.number().int().positive(),
  defaultCapacity: z.number().int().positive(),
  active: z.boolean().default(true),
});

export const CreateSessionSchema = z.object({
  classTypeId: z.string(),
  branchId: z.string(),
  coachId: z.string(),
  startsAt: z.string(),
  durationMin: z.number().int().positive().optional(),
  capacity: z.number().int().positive().optional(),
  creditCost: z.number().int().positive().optional(),
  area: z.string().nullable().default(null),
  publish: z.boolean().default(true),
});

export const UpdateSessionSchema = z.object({
  coachId: z.string().optional(),
  capacity: z.number().int().positive().optional(),
  startsAt: z.string().optional(),
  durationMin: z.number().int().positive().optional(),
  area: z.string().nullable().optional(),
});

export const AdminBookSchema = z.object({
  memberId: z.string(),
  sessionId: z.string(),
});

export const UpsertCoachSchema = z.object({
  name: z.string().min(2),
  bio: z.string().default(''),
  specialization: z.string().default(''),
  branchId: z.string(),
  status: z.enum(['ACTIVE', 'INACTIVE']).default('ACTIVE'),
});

// ── Gate ────────────────────────────────────────────────────────────────────
export const GateScanSchema = z
  .object({
    qrToken: z.string().optional(),
    /** Simulator mode: scan on behalf of a member (issues a fresh token server-side). */
    memberId: z.string().optional(),
  })
  .refine((v) => v.qrToken || v.memberId, 'qrToken or memberId is required');

// ── Engagement ──────────────────────────────────────────────────────────────
export const SegmentFilterSchema = z.object({
  branchId: z.string().nullable().default(null),
  maxBalance: z.number().int().min(0).nullable().default(null),
  minDaysSinceLastVisit: z.number().int().min(1).nullable().default(null),
  joinedWithinDays: z.number().int().min(1).nullable().default(null),
});

export const UpsertCampaignSchema = z.object({
  name: z.string().min(2),
  segment: z.enum([
    'ALL_ACTIVE',
    'LOW_BALANCE',
    'EXPIRING_CREDITS',
    'NEW_MEMBERS',
    'NO_VISIT_14D',
    'CUSTOM',
  ]),
  customFilter: SegmentFilterSchema.nullable().default(null),
  message: z.string().min(3),
  deepLink: z.string().nullable().default(null),
  scheduledAt: z.string().nullable().default(null),
});

export const SegmentPreviewSchema = z.object({
  segment: z.enum([
    'ALL_ACTIVE',
    'LOW_BALANCE',
    'EXPIRING_CREDITS',
    'NEW_MEMBERS',
    'NO_VISIT_14D',
    'CUSTOM',
  ]),
  customFilter: SegmentFilterSchema.nullable().default(null),
});

// ── Config ──────────────────────────────────────────────────────────────────
export const UpdateRulesSchema = z.object({
  openGymCreditCost: z.number().int().min(0).optional(),
  defaultCreditExpiryDays: z.number().int().positive().optional(),
  cancellationDeadlineHours: z.number().min(0).optional(),
  lateCancellationPolicy: z.enum(['FORFEIT', 'FREE']).optional(),
  noShowPolicy: z.enum(['FORFEIT', 'FREE']).optional(),
  reEntryGraceMinutes: z.number().min(0).optional(),
  antiPassbackMinutes: z.number().min(0).optional(),
  qrTtlSeconds: z.number().int().min(10).optional(),
  waitlistAutoPromote: z.boolean().optional(),
  lowBalanceThreshold: z.number().int().min(0).optional(),
  expiryReminderDays: z.number().int().min(1).optional(),
  bookingOpensDaysBefore: z.number().int().min(0).optional(),
  bookingClosesMinutesBefore: z.number().int().min(0).optional(),
});

export const UpdateBranchSchema = z.object({
  name: z.string().optional(),
  address: z.string().optional(),
  operatingHours: z.string().optional(),
  status: z.enum(['ACTIVE', 'INACTIVE']).optional(),
  managerName: z.string().nullable().optional(),
});

export const CreateBranchSchema = z.object({
  name: z.string().min(2),
  address: z.string().min(3),
  operatingHours: z.string().default('06:00 – 22:00'),
  timezone: z.string().default('Asia/Jakarta'),
  managerName: z.string().nullable().default(null),
});

export const UpsertGateSchema = z.object({
  name: z.string().min(2),
  branchId: z.string(),
  status: z.enum(['ONLINE', 'OFFLINE']).default('ONLINE'),
});

export const UpsertAdminUserSchema = z.object({
  name: z.string().min(2),
  email: z.string().email(),
  role: z.enum(['SUPER_ADMIN', 'HQ_ADMIN', 'BRANCH_MANAGER', 'FRONT_DESK', 'COACH', 'FINANCE']),
  branchId: z.string().nullable().default(null),
});

export const ResolveConflictSchema = z.object({
  action: z.enum(['APPROVE', 'REJECT']),
  reason: z.string().min(3),
});

export type OtpRequest = z.infer<typeof OtpRequestSchema>;
export type OtpVerify = z.infer<typeof OtpVerifySchema>;
export type RegisterMemberInput = z.infer<typeof RegisterMemberSchema>;
export type AdminLoginInput = z.infer<typeof AdminLoginSchema>;
export type UpdateProfileInput = z.infer<typeof UpdateProfileSchema>;
export type TopUpRequest = z.infer<typeof TopUpRequestSchema>;
export type ValidateVoucherInput = z.infer<typeof ValidateVoucherSchema>;
export type AdjustCreditsInput = z.infer<typeof AdjustCreditsSchema>;
export type UpsertPackageInput = z.infer<typeof UpsertPackageSchema>;
export type UpsertVoucherInput = z.infer<typeof UpsertVoucherSchema>;
export type UpdateMemberAdminInput = z.infer<typeof UpdateMemberAdminSchema>;
export type UpsertClassTypeInput = z.infer<typeof UpsertClassTypeSchema>;
export type CreateSessionInput = z.infer<typeof CreateSessionSchema>;
export type UpdateSessionInput = z.infer<typeof UpdateSessionSchema>;
export type AdminBookInput = z.infer<typeof AdminBookSchema>;
export type UpsertCoachInput = z.infer<typeof UpsertCoachSchema>;
export type GateScanInput = z.infer<typeof GateScanSchema>;
export type UpsertCampaignInput = z.infer<typeof UpsertCampaignSchema>;
export type SegmentPreviewInput = z.infer<typeof SegmentPreviewSchema>;
export type UpdateRulesInput = z.infer<typeof UpdateRulesSchema>;
export type UpdateBranchInput = z.infer<typeof UpdateBranchSchema>;
export type CreateBranchInput = z.infer<typeof CreateBranchSchema>;
export type UpsertGateInput = z.infer<typeof UpsertGateSchema>;
export type UpsertAdminUserInput = z.infer<typeof UpsertAdminUserSchema>;
export type ResolveConflictInput = z.infer<typeof ResolveConflictSchema>;
