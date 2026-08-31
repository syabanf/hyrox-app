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
  /** Class types this package's credits may book; null = every class. */
  applicableClassTypeIds: string[] | null;
  /** Packages with transaction history are ARCHIVED, never deleted. */
  status: 'ACTIVE' | 'ARCHIVED';
  createdAt: IsoDate;
}

/** Does this package's coverage include the class type? */
export const packageCoversClass = (pkg: CreditPackage, classTypeId: string): boolean =>
  pkg.applicableClassTypeIds === null || pkg.applicableClassTypeIds.includes(classTypeId);
