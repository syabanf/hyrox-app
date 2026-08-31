import type {
  Division,
  Exercise,
  GeneratedWorkout,
  SubstitutionRule,
  WorkoutSession,
  WorkoutType,
} from '@hyrox/domain';
import { z } from 'zod';

// ── Requests ────────────────────────────────────────────────────────────────
export const GenerateWorkoutSchema = z.object({
  type: z.enum(['FULL_SIMULATION', 'COVERAGE', 'QUICK', 'PRACTICE']),
  division: z.enum(['MEN_OPEN', 'MEN_PRO', 'WOMEN_OPEN', 'WOMEN_PRO']),
  /** COVERAGE/PRACTICE: chosen station numbers (1–8); empty → random. */
  stationOrders: z.array(z.number().int().min(1).max(8)).default([]),
  excludedExerciseIds: z.array(z.string()).default([]),
});

export const ReplaceBlockSchema = z.object({
  order: z.number().int().positive(),
  exerciseId: z.string(),
});

export const BlockResultSchema = z.object({
  order: z.number().int().positive(),
  durationSec: z.number().int().min(1).max(3600),
});

export const FinishSessionSchema = z.object({
  partial: z.boolean().default(false),
});

export type GenerateWorkoutInput = z.infer<typeof GenerateWorkoutSchema>;
export type ReplaceBlockInput = z.infer<typeof ReplaceBlockSchema>;
export type BlockResultInput = z.infer<typeof BlockResultSchema>;

// ── Views ───────────────────────────────────────────────────────────────────
export interface ExerciseLibraryView {
  exercises: Exercise[];
  substitutions: SubstitutionRule[];
}

export type WorkoutView = GeneratedWorkout;

export interface WorkoutSessionView {
  session: WorkoutSession;
  workout: GeneratedWorkout;
  activeSec: number;
  completionPct: number;
}

export interface WorkoutHistoryItemView {
  session: WorkoutSession;
  workoutType: WorkoutType;
  division: Division;
  totalBlocks: number;
  activeSec: number;
  completionPct: number;
}
