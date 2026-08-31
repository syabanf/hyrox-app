import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';

export const PAYMENT_STATUSES = [
  'DRAFT',
  'PENDING',
  'PAID',
  'FAILED',
  'EXPIRED',
  'REFUNDED',
] as const;
export type PaymentStatus = (typeof PAYMENT_STATUSES)[number];

export const PAYMENT_TRANSITIONS: TransitionMap<PaymentStatus> = {
  DRAFT: ['PENDING'],
  PENDING: ['PAID', 'FAILED', 'EXPIRED'],
  PAID: ['REFUNDED'],
  FAILED: [],
  EXPIRED: [],
  REFUNDED: [],
};

export const PAYMENT_CHANNELS = ['QRIS', 'EWALLET', 'VIRTUAL_ACCOUNT', 'CARD'] as const;
export type PaymentChannel = (typeof PAYMENT_CHANNELS)[number];

/**
 * PAYMENT ≠ CREDIT LEDGER. A paid payment *produces* a TOP_UP ledger entry
 * (and a TopUpLot); it never mutates a balance itself.
 */
export interface Payment {
  id: string;
  memberId: string;
  packageId: string;
  credits: number;
  amountIdr: number;
  discountIdr: number;
  totalIdr: number;
  voucherCode: string | null;
  channel: PaymentChannel;
  status: PaymentStatus;
  createdAt: IsoDate;
  paidAt: IsoDate | null;
  refundedAt: IsoDate | null;
}
