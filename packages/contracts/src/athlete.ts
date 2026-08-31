import type {
  ActivitySplit,
  ActivityType,
  ActivityVisibility,
  AthleteSettings,
  Challenge,
  Club,
  Gear,
  PersonalRecords,
  Segment,
  TrackPoint,
  WeekBucket,
} from '@hyrox/domain';
import { z } from 'zod';

// ── Requests ────────────────────────────────────────────────────────────────
export const TrackPointSchema = z.object({
  t: z.number().min(0),
  lat: z.number().min(-90).max(90),
  lng: z.number().min(-180).max(180),
  ele: z.number().optional(),
});

export const SaveActivitySchema = z.object({
  type: z.enum(['RUN', 'RIDE', 'WALK', 'WORKOUT']),
  title: z.string().min(1).max(120),
  description: z.string().max(2000).default(''),
  startedAt: z.string(),
  points: z.array(TrackPointSchema).max(50_000),
  /** For point-less workouts: manual duration. */
  manualElapsedSec: z.number().int().positive().nullable().default(null),
  gearId: z.string().nullable().default(null),
  visibility: z.enum(['EVERYONE', 'FOLLOWERS', 'PRIVATE']).default('EVERYONE'),
  /** Small data-URL photos, resized client-side. */
  photos: z.array(z.string().max(400_000)).max(2).default([]),
});

export const UpdateActivitySchema = z.object({
  title: z.string().min(1).max(120).optional(),
  description: z.string().max(2000).optional(),
  visibility: z.enum(['EVERYONE', 'FOLLOWERS', 'PRIVATE']).optional(),
  gearId: z.string().nullable().optional(),
  photos: z.array(z.string().max(400_000)).max(2).optional(),
});

export const SaveRouteSchema = z.object({
  activityId: z.string(),
  name: z.string().min(2).max(80),
});

export const ActivityCommentSchema = z.object({ text: z.string().min(1).max(500) });

export const UpsertGearSchema = z.object({
  name: z.string().min(2).max(60),
  kind: z.enum(['SHOES', 'BIKE']),
  retired: z.boolean().default(false),
});

export const UpdateAthleteSettingsSchema = z.object({
  units: z.enum(['METRIC', 'IMPERIAL']).optional(),
  bookingReminders: z.boolean().optional(),
  weeklyGoalKm: z.number().positive().max(1000).nullable().optional(),
  language: z.enum(['EN', 'ID']).optional(),
});

export type SaveActivityInput = z.infer<typeof SaveActivitySchema>;
export type UpdateActivityInput = z.infer<typeof UpdateActivitySchema>;
export type SaveRouteInput = z.infer<typeof SaveRouteSchema>;
export type UpsertGearInput = z.infer<typeof UpsertGearSchema>;
export type UpdateAthleteSettingsInput = z.infer<typeof UpdateAthleteSettingsSchema>;

// ── Views ───────────────────────────────────────────────────────────────────
export interface ActivityCardView {
  id: string;
  memberId: string;
  memberName: string;
  memberAvatarUrl: string | null;
  isOwn: boolean;
  type: ActivityType;
  title: string;
  startedAt: string;
  distanceM: number;
  movingSec: number;
  elapsedSec: number;
  avgPaceSecPerKm: number | null;
  elevationGainM: number;
  visibility: ActivityVisibility;
  /** Downsampled polyline for the card thumbnail. */
  thumbnail: TrackPoint[];
  photoCount: number;
  kudosCount: number;
  hasKudoed: boolean;
  commentCount: number;
}

export interface SegmentEffortView {
  segmentId: string;
  segmentName: string;
  distanceM: number;
  elapsedSec: number;
  rank: number;
  totalEfforts: number;
  isPersonalBest: boolean;
}

export interface ActivityCommentView {
  id: string;
  memberId: string;
  memberName: string;
  text: string;
  createdAt: string;
}

export interface ActivityDetailView extends ActivityCardView {
  description: string;
  points: TrackPoint[];
  photos: string[];
  splits: ActivitySplit[];
  bestSplitPaceSec: number | null;
  gearName: string | null;
  efforts: SegmentEffortView[];
  comments: ActivityCommentView[];
  /** Strava-style "grouped with" — others who trained together. */
  groupedWith: { activityId: string; memberName: string }[];
}

export interface RouteView {
  id: string;
  name: string;
  distanceM: number;
  points: TrackPoint[];
  createdAt: string;
}

export interface HeatmapView {
  tracks: TrackPoint[][];
}

export interface AthleteStatsView {
  weekly: WeekBucket[];
  totals: { activities: number; distanceKm: number; movingSec: number };
  thisWeekKm: number;
  goal: { targetKm: number | null; currentKm: number };
  prs: PersonalRecords;
  gear: Gear[];
  settings: AthleteSettings;
  followingCount: number;
  followerCount: number;
}

export interface SegmentListView {
  segment: Segment;
  effortCount: number;
  bestElapsedSec: number | null;
  myBestElapsedSec: number | null;
  myRank: number | null;
}

export interface SegmentLeaderboardRow {
  rank: number;
  memberId: string;
  memberName: string;
  elapsedSec: number;
  createdAt: string;
  isMe: boolean;
}

export interface SegmentDetailView {
  segment: Segment;
  leaderboard: SegmentLeaderboardRow[];
  myRank: number | null;
}

export interface ChallengeView {
  challenge: Challenge;
  joined: boolean;
  participantCount: number;
  progressKm: number;
  leaderboard: { memberName: string; km: number; isMe: boolean }[];
}

export interface ClubView {
  club: Club;
  joined: boolean;
  memberCount: number;
  weeklyLeaderboard: { memberName: string; km: number; isMe: boolean }[];
}

export interface AthleteLite {
  memberId: string;
  name: string;
  weeklyKm: number;
  isFollowing: boolean;
}

export interface SocialView {
  following: AthleteLite[];
  followers: AthleteLite[];
  suggestions: AthleteLite[];
}
