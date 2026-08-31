import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';

/** CLASS TYPE ≠ CLASS SESSION: the type is the reusable template. */
export interface ClassType {
  id: string;
  name: string;
  description: string;
  defaultDurationMin: number;
  defaultCreditCost: number;
  defaultCapacity: number;
  active: boolean;
}

export const SESSION_STATUSES = ['DRAFT', 'PUBLISHED', 'FULL', 'COMPLETED', 'CANCELLED'] as const;
export type SessionStatus = (typeof SESSION_STATUSES)[number];

export const SESSION_TRANSITIONS: TransitionMap<SessionStatus> = {
  DRAFT: ['PUBLISHED', 'CANCELLED'],
  PUBLISHED: ['FULL', 'COMPLETED', 'CANCELLED'],
  FULL: ['PUBLISHED', 'COMPLETED', 'CANCELLED'],
  COMPLETED: [],
  CANCELLED: [],
};

export interface ClassSession {
  id: string;
  classTypeId: string;
  branchId: string;
  coachId: string;
  startsAt: IsoDate;
  endsAt: IsoDate;
  capacity: number;
  creditCost: number;
  bookingOpensAt: IsoDate;
  bookingClosesAt: IsoDate;
  status: SessionStatus;
  area: string | null;
}
