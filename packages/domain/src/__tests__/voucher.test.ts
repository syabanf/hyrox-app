import { describe, expect, it } from 'vitest';
import type { CreditPackage, Voucher } from '../index';
import { validateVoucher } from '../index';

const pkg: CreditPackage = {
  id: 'pkg_1',
  name: '10 Visit Pack',
  credits: 10,
  priceIdr: 1_500_000,
  validityDays: 60,
  branchId: null,
  purchaseLimitPerMember: null,
  status: 'ACTIVE',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const voucher = (partial: Partial<Voucher> = {}): Voucher => ({
  id: 'vch_1',
  code: 'HYROX100',
  type: 'FIXED_IDR',
  value: 100_000,
  startsAt: '2026-01-01T00:00:00.000Z',
  endsAt: '2026-02-01T00:00:00.000Z',
  usageLimit: 100,
  perMemberLimit: 1,
  eligibleSegment: 'ALL',
  applicablePackageIds: null,
  status: 'ACTIVE',
  createdAt: '2026-01-01T00:00:00.000Z',
  ...partial,
});

const base = {
  voucher: voucher(),
  pkg,
  memberIsNew: false,
  memberRedemptionCount: 0,
  totalRedemptionCount: 0,
  now: '2026-01-15T00:00:00.000Z',
};

describe('validateVoucher', () => {
  it('grants a fixed discount', () => {
    expect(validateVoucher(base)).toEqual({ ok: true, value: { discountIdr: 100_000 } });
  });

  it('caps a fixed discount at the package price', () => {
    const res = validateVoucher({ ...base, voucher: voucher({ value: 99_000_000 }) });
    expect(res).toEqual({ ok: true, value: { discountIdr: 1_500_000 } });
  });

  it('computes percentage discounts', () => {
    const res = validateVoucher({ ...base, voucher: voucher({ type: 'PERCENT', value: 10 }) });
    expect(res).toEqual({ ok: true, value: { discountIdr: 150_000 } });
  });

  it.each([
    [{ status: 'DRAFT' }, 'NOT_ACTIVE'],
    [{ status: 'DISABLED' }, 'NOT_ACTIVE'],
    [{ startsAt: '2026-01-20T00:00:00.000Z' }, 'NOT_STARTED'],
    [{ endsAt: '2026-01-10T00:00:00.000Z' }, 'ENDED'],
    [{ applicablePackageIds: ['pkg_other'] }, 'PACKAGE_NOT_ELIGIBLE'],
    [{ eligibleSegment: 'NEW_MEMBERS' }, 'SEGMENT_NOT_ELIGIBLE'],
  ] as const)('rejects %o with %s', (patch, reason) => {
    const res = validateVoucher({ ...base, voucher: voucher(patch as Partial<Voucher>) });
    expect(res).toEqual({ ok: false, error: { reason } });
  });

  it('rejects when usage limits are exhausted', () => {
    expect(validateVoucher({ ...base, totalRedemptionCount: 100 })).toEqual({
      ok: false,
      error: { reason: 'USAGE_LIMIT_REACHED' },
    });
    expect(validateVoucher({ ...base, memberRedemptionCount: 1 })).toEqual({
      ok: false,
      error: { reason: 'PER_MEMBER_LIMIT_REACHED' },
    });
  });

  it('accepts NEW_MEMBERS segment for a new member', () => {
    const res = validateVoucher({
      ...base,
      voucher: voucher({ eligibleSegment: 'NEW_MEMBERS' }),
      memberIsNew: true,
    });
    expect(res.ok).toBe(true);
  });
});
