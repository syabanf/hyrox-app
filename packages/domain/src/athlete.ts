import type { IsoDate } from './shared/time';
import { msOf } from './shared/time';

// ── Activities ──────────────────────────────────────────────────────────────
export const ACTIVITY_TYPES = ['RUN', 'RIDE', 'WALK', 'WORKOUT'] as const;
export type ActivityType = (typeof ACTIVITY_TYPES)[number];

export const ACTIVITY_VISIBILITIES = ['EVERYONE', 'FOLLOWERS', 'PRIVATE'] as const;
export type ActivityVisibility = (typeof ACTIVITY_VISIBILITIES)[number];

/** One GPS sample; `t` is milliseconds since activity start, `ele` meters (optional). */
export interface TrackPoint {
  t: number;
  lat: number;
  lng: number;
  ele?: number;
}

export interface Activity {
  id: string;
  memberId: string;
  type: ActivityType;
  title: string;
  description: string;
  startedAt: IsoDate;
  elapsedSec: number;
  movingSec: number;
  distanceM: number;
  /** Moving-time pace; null for point-less workouts. */
  avgPaceSecPerKm: number | null;
  elevationGainM: number;
  points: TrackPoint[];
  /** Small data-URL photos (capped by the API). */
  photos: string[];
  visibility: ActivityVisibility;
  gearId: string | null;
  createdAt: IsoDate;
}

export interface ActivitySplit {
  /** 1-based km index. */
  km: number;
  distanceM: number;
  paceSecPerKm: number;
  /** Full 1000m split (last one may be partial). */
  full: boolean;
}

export interface ActivityStats {
  distanceM: number;
  elapsedSec: number;
  movingSec: number;
  avgPaceSecPerKm: number | null;
  elevationGainM: number;
  splits: ActivitySplit[];
  bestSplitPaceSec: number | null;
}

const EARTH_R = 6_371_000;
const rad = (d: number) => (d * Math.PI) / 180;

/** Great-circle distance in meters. */
export function haversineM(a: Pick<TrackPoint, 'lat' | 'lng'>, b: Pick<TrackPoint, 'lat' | 'lng'>): number {
  const dLat = rad(b.lat - a.lat);
  const dLng = rad(b.lng - a.lng);
  const s =
    Math.sin(dLat / 2) ** 2 + Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(dLng / 2) ** 2;
  return 2 * EARTH_R * Math.asin(Math.sqrt(s));
}

/** Below this speed a gap counts as stopped (auto-pause), not moving. */
const MOVING_THRESHOLD_MPS = 0.5;

export function computeActivityStats(points: readonly TrackPoint[]): ActivityStats {
  if (points.length < 2) {
    return {
      distanceM: 0,
      elapsedSec: points.length ? Math.round(points[points.length - 1]!.t / 1000) : 0,
      movingSec: 0,
      avgPaceSecPerKm: null,
      elevationGainM: 0,
      splits: [],
      bestSplitPaceSec: null,
    };
  }

  let distanceM = 0;
  let movingSec = 0;
  let elevationGainM = 0;
  const splits: ActivitySplit[] = [];
  let splitDist = 0;
  let splitMoving = 0;

  for (let i = 1; i < points.length; i++) {
    const prev = points[i - 1]!;
    const curr = points[i]!;
    const dt = (curr.t - prev.t) / 1000;
    if (dt <= 0) continue;
    const d = haversineM(prev, curr);
    distanceM += d;
    if (prev.ele !== undefined && curr.ele !== undefined) {
      const rise = curr.ele - prev.ele;
      if (rise > 0.3) elevationGainM += rise; // ignore GPS jitter
    }
    const isMoving = d / dt >= MOVING_THRESHOLD_MPS;
    if (isMoving) movingSec += dt;

    // Split accounting (may cross a km boundary inside one sample).
    let remaining = d;
    let remainingT = isMoving ? dt : 0;
    while (remaining > 0) {
      const room = 1000 - splitDist;
      const take = Math.min(room, remaining);
      const frac = remaining > 0 ? take / remaining : 0;
      splitDist += take;
      splitMoving += remainingT * frac;
      remainingT *= 1 - frac;
      remaining -= take;
      if (splitDist >= 1000) {
        splits.push({
          km: splits.length + 1,
          distanceM: 1000,
          paceSecPerKm: Math.round(splitMoving),
          full: true,
        });
        splitDist = 0;
        splitMoving = 0;
      }
    }
  }
  if (splitDist > 50) {
    splits.push({
      km: splits.length + 1,
      distanceM: Math.round(splitDist),
      paceSecPerKm: Math.round((splitMoving / splitDist) * 1000),
      full: false,
    });
  }

  const elapsedSec = Math.round((points[points.length - 1]!.t - points[0]!.t) / 1000);
  const fullSplits = splits.filter((s) => s.full);
  return {
    distanceM: Math.round(distanceM),
    elapsedSec,
    movingSec: Math.round(movingSec),
    avgPaceSecPerKm: distanceM >= 50 ? Math.round(movingSec / (distanceM / 1000)) : null,
    elevationGainM: Math.round(elevationGainM),
    splits,
    bestSplitPaceSec: fullSplits.length
      ? Math.min(...fullSplits.map((s) => s.paceSecPerKm))
      : null,
  };
}

// ── Social graph ────────────────────────────────────────────────────────────
export interface Follow {
  followerId: string;
  followeeId: string;
}
export interface Kudos {
  activityId: string;
  memberId: string;
  createdAt: IsoDate;
}
export interface ActivityComment {
  id: string;
  activityId: string;
  memberId: string;
  text: string;
  createdAt: IsoDate;
}

/** Who may see this activity in a viewer's feed. */
export function canViewActivity(
  activity: Pick<Activity, 'memberId' | 'visibility'>,
  viewerId: string,
  viewerFollows: (memberId: string) => boolean,
): boolean {
  if (activity.memberId === viewerId) return true;
  if (activity.visibility === 'EVERYONE') return true;
  if (activity.visibility === 'FOLLOWERS') return viewerFollows(activity.memberId);
  return false;
}

// ── Segments, challenges, clubs ─────────────────────────────────────────────
export interface Segment {
  id: string;
  name: string;
  type: ActivityType;
  distanceM: number;
  location: string;
  /** The segment's own polyline; matching compares GPS tracks against it. */
  path: TrackPoint[];
}
export interface SegmentEffort {
  id: string;
  segmentId: string;
  activityId: string;
  memberId: string;
  elapsedSec: number;
  createdAt: IsoDate;
}

/** How close (meters) a track must pass to a segment endpoint to count. */
const SEGMENT_MATCH_RADIUS_M = 60;

export interface SegmentMatch {
  segment: Segment;
  startIdx: number;
  endIdx: number;
  elapsedSec: number;
}

/**
 * GPS polyline matching: the activity matches a segment when its track passes
 * through the segment's start gate and later through its end gate, having
 * covered roughly the segment distance in between. The effort time comes from
 * the actual point timestamps.
 */
export function matchSegments(
  segments: readonly Segment[],
  activity: Pick<Activity, 'type'>,
  points: readonly TrackPoint[],
): SegmentMatch[] {
  if (points.length < 2) return [];
  // Prefix sums so the traveled distance between two indices is O(1).
  const cum: number[] = [0];
  for (let i = 1; i < points.length; i++) {
    cum.push(cum[i - 1]! + haversineM(points[i - 1]!, points[i]!));
  }

  const matches: SegmentMatch[] = [];
  for (const segment of segments) {
    if (segment.type !== activity.type || segment.path.length < 2) continue;
    const gateStart = segment.path[0]!;
    const gateEnd = segment.path[segment.path.length - 1]!;

    let startIdx = -1;
    for (let i = 0; i < points.length; i++) {
      if (haversineM(points[i]!, gateStart) <= SEGMENT_MATCH_RADIUS_M) {
        startIdx = i;
        break;
      }
    }
    if (startIdx < 0) continue;

    let endIdx = -1;
    for (let j = startIdx + 1; j < points.length; j++) {
      if (haversineM(points[j]!, gateEnd) <= SEGMENT_MATCH_RADIUS_M) {
        const traveled = cum[j]! - cum[startIdx]!;
        if (traveled >= segment.distanceM * 0.8 && traveled <= segment.distanceM * 1.35) {
          endIdx = j;
          break;
        }
      }
    }
    if (endIdx < 0) continue;

    matches.push({
      segment,
      startIdx,
      endIdx,
      elapsedSec: Math.max(1, Math.round((points[endIdx]!.t - points[startIdx]!.t) / 1000)),
    });
  }
  return matches;
}

/** Strava-style "grouped activities": same type, overlapping start time, nearby start point. */
export function findGroupedActivities(
  activity: Pick<Activity, 'id' | 'type' | 'startedAt' | 'points' | 'memberId'>,
  candidates: readonly Pick<Activity, 'id' | 'type' | 'startedAt' | 'points' | 'memberId'>[],
  opts: { windowMin?: number; radiusM?: number } = {},
): string[] {
  const windowMs = (opts.windowMin ?? 45) * 60_000;
  const radiusM = opts.radiusM ?? 500;
  const start = activity.points[0];
  if (!start) return [];
  const t0 = msOf(activity.startedAt);
  return candidates
    .filter(
      (c) =>
        c.id !== activity.id &&
        c.memberId !== activity.memberId &&
        c.type === activity.type &&
        c.points.length > 0 &&
        Math.abs(msOf(c.startedAt) - t0) <= windowMs &&
        haversineM(c.points[0]!, start) <= radiusM,
    )
    .map((c) => c.id);
}

// ── Routes ──────────────────────────────────────────────────────────────────
export interface Route {
  id: string;
  memberId: string;
  name: string;
  points: TrackPoint[];
  distanceM: number;
  createdAt: IsoDate;
}

export interface Challenge {
  id: string;
  name: string;
  description: string;
  type: ActivityType | 'ANY';
  targetKm: number;
  startsAt: IsoDate;
  endsAt: IsoDate;
}

export function challengeProgressKm(
  challenge: Challenge,
  activities: readonly Pick<Activity, 'type' | 'distanceM' | 'startedAt'>[],
): number {
  const from = msOf(challenge.startsAt);
  const to = msOf(challenge.endsAt);
  const total = activities
    .filter(
      (a) =>
        (challenge.type === 'ANY' || a.type === challenge.type) &&
        msOf(a.startedAt) >= from &&
        msOf(a.startedAt) <= to,
    )
    .reduce((sum, a) => sum + a.distanceM, 0);
  return Math.round(total / 100) / 10;
}

export interface Club {
  id: string;
  name: string;
  description: string;
  location: string;
  memberIds: string[];
}

// ── Gear & goals ────────────────────────────────────────────────────────────
export interface Gear {
  id: string;
  memberId: string;
  name: string;
  kind: 'SHOES' | 'BIKE';
  distanceM: number;
  retired: boolean;
}

export interface AthleteSettings {
  units: 'METRIC' | 'IMPERIAL';
  bookingReminders: boolean;
  weeklyGoalKm: number | null;
  language: 'EN' | 'ID';
}
export const DEFAULT_ATHLETE_SETTINGS: AthleteSettings = {
  units: 'METRIC',
  bookingReminders: true,
  weeklyGoalKm: null,
  language: 'EN',
};

// ── Stats ───────────────────────────────────────────────────────────────────
export interface WeekBucket {
  /** ISO date of the Monday starting the week. */
  weekStart: string;
  distanceKm: number;
  activities: number;
  movingSec: number;
}

const startOfWeekMs = (ms: number): number => {
  const d = new Date(ms);
  d.setHours(0, 0, 0, 0);
  const day = (d.getDay() + 6) % 7; // Monday = 0
  d.setDate(d.getDate() - day);
  return d.getTime();
};

export function weeklyBuckets(
  activities: readonly Pick<Activity, 'startedAt' | 'distanceM' | 'movingSec'>[],
  now: IsoDate,
  weeks: number,
): WeekBucket[] {
  const currentStart = startOfWeekMs(msOf(now));
  const buckets: WeekBucket[] = [];
  for (let i = weeks - 1; i >= 0; i--) {
    const start = currentStart - i * 7 * 24 * 3600_000;
    const end = start + 7 * 24 * 3600_000;
    const inWeek = activities.filter((a) => msOf(a.startedAt) >= start && msOf(a.startedAt) < end);
    buckets.push({
      weekStart: new Date(start).toISOString(),
      distanceKm: Math.round(inWeek.reduce((s, a) => s + a.distanceM, 0) / 100) / 10,
      activities: inWeek.length,
      movingSec: inWeek.reduce((s, a) => s + a.movingSec, 0),
    });
  }
  return buckets;
}

export interface PersonalRecords {
  best1kPaceSec: number | null;
  best5kSec: number | null;
  best10kSec: number | null;
  longestDistanceM: number;
  longestMovingSec: number;
}

/** 5k/10k are moving-pace estimates over the whole activity (labelled as such in UI). */
export function personalRecords(activities: readonly Activity[]): PersonalRecords {
  const runs = activities.filter((a) => a.type === 'RUN' && a.distanceM > 0 && a.movingSec > 0);
  const bestSplitPaces = runs
    .map((a) => computeActivityStats(a.points).bestSplitPaceSec)
    .filter((p): p is number => p !== null);
  const estFor = (meters: number): number | null => {
    const eligible = runs.filter((a) => a.distanceM >= meters);
    if (!eligible.length) return null;
    return Math.min(...eligible.map((a) => Math.round((a.movingSec * meters) / a.distanceM)));
  };
  return {
    best1kPaceSec: bestSplitPaces.length ? Math.min(...bestSplitPaces) : null,
    best5kSec: estFor(5000),
    best10kSec: estFor(10_000),
    longestDistanceM: activities.reduce((m, a) => Math.max(m, a.distanceM), 0),
    longestMovingSec: activities.reduce((m, a) => Math.max(m, a.movingSec), 0),
  };
}
