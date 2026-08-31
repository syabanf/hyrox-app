import type { IsoDate } from './shared/time';

export interface CreditPackage {
  id: string;
  name: string;
  credits: number;
  priceIdr: number;
  validityDays: number;
  /** null = available at all branches. */
  branchId: string | null;
  purchaseLimitPerMember: number | null;
  /** Packages with transaction history are ARCHIVED, never deleted. */
  status: 'ACTIVE' | 'ARCHIVED';
  createdAt: IsoDate;
}
