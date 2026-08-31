import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';

export const MEMBER_STATUSES = ['ACTIVE', 'SUSPENDED', 'INACTIVE', 'ARCHIVED'] as const;
export type MemberStatus = (typeof MEMBER_STATUSES)[number];

/** Members are never hard-deleted; ARCHIVED is terminal. */
export const MEMBER_TRANSITIONS: TransitionMap<MemberStatus> = {
  ACTIVE: ['SUSPENDED', 'INACTIVE', 'ARCHIVED'],
  SUSPENDED: ['ACTIVE', 'ARCHIVED'],
  INACTIVE: ['ACTIVE', 'ARCHIVED'],
  ARCHIVED: [],
};

export interface EmergencyContact {
  name: string;
  phone: string;
  relation: string;
}

export interface Member {
  id: string;
  fullName: string;
  email: string;
  phone: string;
  dateOfBirth: IsoDate | null;
  gender: 'MALE' | 'FEMALE' | 'OTHER' | null;
  emergencyContact: EmergencyContact | null;
  preferredBranchId: string | null;
  /** Small data-URL avatar (mock file storage). */
  avatarUrl: string | null;
  status: MemberStatus;
  waiverVersion: string | null;
  waiverAcceptedAt: IsoDate | null;
  notes: string | null;
  createdAt: IsoDate;
  updatedAt: IsoDate;
}
