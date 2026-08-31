import type { CreditPackage } from './packages';
import type { TransitionMap } from './shared/machine';
import type { Result } from './shared/result';
import { err, ok } from './shared/result';
import type { IsoDate } from './shared/time';
import { isAfter, isBefore } from './shared/time';

export const VOUCHER_STATUSES = ['DRAFT', 'SCHEDULED', 'ACTIVE', 'EXPIRED', 'DISABLED'] as const;
export type VoucherStatus = (typeof VOUCHER_STATUSES)[number];

export const VOUCHER_TRANSITIONS: TransitionMap<VoucherStatus> = {
  DRAFT: ['SCHEDULED', 'ACTIVE', 'DISABLED'],
  SCHEDULED: ['ACTIVE', 'DISABLED'],
  ACTIVE: ['EXPIRED', 'DISABLED'],
  EXPIRED: [],
  DISABLED: ['ACTIVE'],
};

export interface Voucher {
  id: string;
  code: string;
  type: 'FIXED_IDR' | 'PERCENT';
  value: number;
  startsAt: IsoDate;
  endsAt: IsoDate;
  usageLimit: number | null;
  perMemberLimit: number | null;
  eligibleSegment: 'ALL' | 'NEW_MEMBERS';
  /** null = applicable to every package. */
  applicablePackageIds: string[] | null;
  status: VoucherStatus;
  createdAt: IsoDate;
}

/** Redemptions are separate records: VOUCHER → REDEMPTION → PAYMENT. */
export interface VoucherRedemption {
  id: string;
  voucherId: string;
  memberId: string;
  paymentId: string;
  discountIdr: number;
  createdAt: IsoDate;
}

export type VoucherRejection =
  | 'NOT_ACTIVE'
  | 'NOT_STARTED'
  | 'ENDED'
  | 'USAGE_LIMIT_REACHED'
  | 'PER_MEMBER_LIMIT_REACHED'
  | 'PACKAGE_NOT_ELIGIBLE'
  | 'SEGMENT_NOT_ELIGIBLE';

export function validateVoucher(args: {
  voucher: Voucher;
  pkg: CreditPackage;
  memberIsNew: boolean;
  memberRedemptionCount: number;
  totalRedemptionCount: number;
  now: IsoDate;
}): Result<{ discountIdr: number }, { reason: VoucherRejection }> {
  const { voucher, pkg, now } = args;
  if (voucher.status !== 'ACTIVE') return err({ reason: 'NOT_ACTIVE' });
  if (isBefore(now, voucher.startsAt)) return err({ reason: 'NOT_STARTED' });
  if (isAfter(now, voucher.endsAt)) return err({ reason: 'ENDED' });
  if (voucher.usageLimit !== null && args.totalRedemptionCount >= voucher.usageLimit)
    return err({ reason: 'USAGE_LIMIT_REACHED' });
  if (voucher.perMemberLimit !== null && args.memberRedemptionCount >= voucher.perMemberLimit)
    return err({ reason: 'PER_MEMBER_LIMIT_REACHED' });
  if (voucher.applicablePackageIds !== null && !voucher.applicablePackageIds.includes(pkg.id))
    return err({ reason: 'PACKAGE_NOT_ELIGIBLE' });
  if (voucher.eligibleSegment === 'NEW_MEMBERS' && !args.memberIsNew)
    return err({ reason: 'SEGMENT_NOT_ELIGIBLE' });

  const discountIdr =
    voucher.type === 'FIXED_IDR'
      ? Math.min(voucher.value, pkg.priceIdr)
      : Math.round((pkg.priceIdr * voucher.value) / 100);
  return ok({ discountIdr });
}
