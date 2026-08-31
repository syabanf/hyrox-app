import type { RaceAnalysis, RaceEvent, UserRace } from '@hyrox/domain';
import { z } from 'zod';

// ── Requests ────────────────────────────────────────────────────────────────
export const RegisterRaceSchema = z.object({
  division: z.enum(['MEN_OPEN', 'MEN_PRO', 'WOMEN_OPEN', 'WOMEN_PRO']),
  goalSec: z.number().int().min(1800).max(4 * 3600).nullable().default(null),
});

export const UpdateUserRaceSchema = z.object({
  goalSec: z.number().int().min(1800).max(4 * 3600).nullable().optional(),
  status: z.enum(['TRAINING', 'RACED', 'CANCELLED']).optional(),
  resultSec: z.number().int().min(1800).max(6 * 3600).nullable().optional(),
});

export type RegisterRaceInput = z.infer<typeof RegisterRaceSchema>;
export type UpdateUserRaceInput = z.infer<typeof UpdateUserRaceSchema>;

// ── Views ───────────────────────────────────────────────────────────────────
export interface RaceEventView {
  event: RaceEvent;
  joined: boolean;
  participantCount: number;
}

export interface MyRaceView {
  userRace: UserRace;
  event: RaceEvent;
  daysToRace: number;
  predictionSec: number | null;
  readinessScore: number;
  simulationCount: number;
  analysis: RaceAnalysis | null;
}
