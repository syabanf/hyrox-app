import type { Division } from './hyrox';
import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';
import { msOf } from './shared/time';

// ── Race events (blueprint §50) ─────────────────────────────────────────────
export const RACE_STATUSES = [
  'ANNOUNCED',
  'REGISTRATION_OPEN',
  'SOLD_OUT',
  'UPCOMING',
  'ONGOING',
  'COMPLETED',
  'CANCELLED',
] as const;
export type RaceStatus = (typeof RACE_STATUSES)[number];

export const RACE_REGIONS = ['ASIA', 'EUROPE', 'AMERICAS', 'OCEANIA'] as const;
export type RaceRegion = (typeof RACE_REGIONS)[number];

export interface RaceEvent {
  id: string;
  name: string;
  country: string;
  region: RaceRegion;
  city: string;
  venue: string;
  startsAt: IsoDate;
  endsAt: IsoDate;
  registrationUrl: string;
  status: RaceStatus;
}

// ── My Race (blueprint §51) ─────────────────────────────────────────────────
export const USER_RACE_STATUSES = ['TRAINING', 'RACED', 'CANCELLED'] as const;
export type UserRaceStatus = (typeof USER_RACE_STATUSES)[number];

export const USER_RACE_TRANSITIONS: TransitionMap<UserRaceStatus> = {
  TRAINING: ['RACED', 'CANCELLED'],
  RACED: [],
  CANCELLED: ['TRAINING'],
};

export interface UserRace {
  id: string;
  memberId: string;
  raceEventId: string;
  division: Division;
  goalSec: number | null;
  status: UserRaceStatus;
  resultSec: number | null;
  createdAt: IsoDate;
}

// ── Prediction & analysis (blueprint §52) ───────────────────────────────────
/**
 * Race prediction from completed FULL_SIMULATION sessions: the best simulation,
 * minus a small race-day bump. Null until at least one simulation exists.
 */
export function predictRaceSec(fullSimActiveSecs: readonly number[]): number | null {
  const valid = fullSimActiveSecs.filter((s) => s > 0);
  if (valid.length === 0) return null;
  return Math.round(Math.min(...valid) * 0.97);
}

/** 0–100: training consistency over the last 4 weeks vs a 3-sessions/week plan. */
export function raceReadinessScore(activityDates: readonly IsoDate[], now: IsoDate): number {
  const cutoff = msOf(now) - 28 * 24 * 3600_000;
  const recent = activityDates.filter((d) => msOf(d) >= cutoff).length;
  return Math.min(100, Math.round((recent / 12) * 100));
}

export interface RaceAnalysis {
  vsGoalSec: number | null;
  vsPredictionSec: number | null;
  achievedGoal: boolean | null;
}

export function analyzeRace(
  resultSec: number,
  goalSec: number | null,
  predictionSec: number | null,
): RaceAnalysis {
  return {
    vsGoalSec: goalSec !== null ? resultSec - goalSec : null,
    vsPredictionSec: predictionSec !== null ? resultSec - predictionSec : null,
    achievedGoal: goalSec !== null ? resultSec <= goalSec : null,
  };
}
