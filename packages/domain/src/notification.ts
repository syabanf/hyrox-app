import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';

export const MEMBER_NOTIFICATION_TYPES = [
  'BOOKING_CONFIRMED',
  'BOOKING_REMINDER',
  'WAITLIST_PROMOTED',
  'LOW_BALANCE',
  'CREDIT_EXPIRY',
  'VISIT_LOGGED',
  'SESSION_CHANGED',
  'ANNOUNCEMENT',
] as const;
export type MemberNotificationType = (typeof MEMBER_NOTIFICATION_TYPES)[number];

export interface MemberNotification {
  id: string;
  memberId: string;
  type: MemberNotificationType;
  title: string;
  body: string;
  createdAt: IsoDate;
  readAt: IsoDate | null;
}

export const CAMPAIGN_STATUSES = [
  'DRAFT',
  'SCHEDULED',
  'PROCESSING',
  'SENT',
  'FAILED',
  'CANCELLED',
] as const;
export type CampaignStatus = (typeof CAMPAIGN_STATUSES)[number];

export const CAMPAIGN_TRANSITIONS: TransitionMap<CampaignStatus> = {
  DRAFT: ['SCHEDULED', 'PROCESSING', 'CANCELLED'],
  SCHEDULED: ['PROCESSING', 'CANCELLED'],
  PROCESSING: ['SENT', 'FAILED'],
  SENT: [],
  FAILED: [],
  CANCELLED: [],
};

export const MEMBER_SEGMENTS = [
  'ALL_ACTIVE',
  'LOW_BALANCE',
  'EXPIRING_CREDITS',
  'NEW_MEMBERS',
  'NO_VISIT_14D',
  'CUSTOM',
] as const;
export type MemberSegment = (typeof MEMBER_SEGMENTS)[number];

/** Criteria-based audience for CUSTOM campaigns; every set field must match. */
export interface SegmentFilter {
  branchId: string | null;
  maxBalance: number | null;
  minDaysSinceLastVisit: number | null;
  joinedWithinDays: number | null;
}

export interface Campaign {
  id: string;
  name: string;
  segment: MemberSegment;
  customFilter: SegmentFilter | null;
  message: string;
  deepLink: string | null;
  scheduledAt: IsoDate | null;
  status: CampaignStatus;
  sentCount: number | null;
  createdAt: IsoDate;
}
