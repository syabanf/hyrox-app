import type { TransitionMap } from './shared/machine';
import type { IsoDate } from './shared/time';

// ── Exercise library (blueprint §44) ────────────────────────────────────────
export type ExerciseCategory =
  | 'ERG'
  | 'SLED'
  | 'JUMP'
  | 'CARRY'
  | 'LUNGE'
  | 'THROW'
  | 'RUN'
  | 'CONDITIONING';

export interface Exercise {
  id: string;
  name: string;
  category: ExerciseCategory;
  equipment: string[];
  /** 1–8 when the exercise IS one of the HYROX race stations. */
  hyroxStationOrder: number | null;
  difficulty: 1 | 2 | 3;
  defaultSpec: { distanceM: number | null; reps: number | null };
  /** How-to video (YouTube) shown from the workout player. */
  videoUrl: string | null;
}

/** Substitution modeled as data (blueprint §45). */
export interface SubstitutionRule {
  originalExerciseId: string;
  alternativeExerciseId: string;
  /** 0–1: muscle + energy-system similarity. */
  similarity: number;
  conversionNote: string;
}

export function listSubstitutes(
  exerciseId: string,
  substitutions: readonly SubstitutionRule[],
  exercises: readonly Exercise[],
): { exercise: Exercise; rule: SubstitutionRule }[] {
  return substitutions
    .filter((s) => s.originalExerciseId === exerciseId)
    .sort((a, b) => b.similarity - a.similarity)
    .map((rule) => ({
      rule,
      exercise: exercises.find((e) => e.id === rule.alternativeExerciseId)!,
    }))
    .filter((x) => x.exercise !== undefined);
}

// ── Generator (blueprint §41–43, 46–47) ─────────────────────────────────────
export const WORKOUT_TYPES = ['FULL_SIMULATION', 'COVERAGE', 'QUICK', 'PRACTICE'] as const;
export type WorkoutType = (typeof WORKOUT_TYPES)[number];

export const DIVISIONS = ['MEN_OPEN', 'MEN_PRO', 'WOMEN_OPEN', 'WOMEN_PRO'] as const;
export type Division = (typeof DIVISIONS)[number];

export interface WorkoutBlock {
  order: number;
  kind: 'RUN' | 'STATION';
  exerciseId: string;
  exerciseName: string;
  /** Set when the block is a substitution for an excluded/unavailable station. */
  originalExerciseId: string | null;
  originalExerciseName: string | null;
  distanceM: number | null;
  reps: number | null;
  weightNote: string | null;
  targetSec: number;
}

export interface GeneratedWorkout {
  id: string;
  memberId: string;
  type: WorkoutType;
  division: Division;
  blocks: WorkoutBlock[];
  excludedExerciseIds: string[];
  totalTargetSec: number;
  createdAt: IsoDate;
}

/** Division-specific loads for the loaded stations (race-accurate notes). */
const WEIGHT_NOTES: Record<number, Partial<Record<Division, string>>> = {
  2: { MEN_OPEN: 'Sled 152 kg', MEN_PRO: 'Sled 202 kg', WOMEN_OPEN: 'Sled 102 kg', WOMEN_PRO: 'Sled 152 kg' },
  3: { MEN_OPEN: 'Sled 103 kg', MEN_PRO: 'Sled 153 kg', WOMEN_OPEN: 'Sled 78 kg', WOMEN_PRO: 'Sled 103 kg' },
  6: { MEN_OPEN: '2×24 kg', MEN_PRO: '2×32 kg', WOMEN_OPEN: '2×16 kg', WOMEN_PRO: '2×24 kg' },
  7: { MEN_OPEN: '20 kg bag', MEN_PRO: '30 kg bag', WOMEN_OPEN: '10 kg bag', WOMEN_PRO: '20 kg bag' },
  8: { MEN_OPEN: '6 kg ball', MEN_PRO: '9 kg ball', WOMEN_OPEN: '4 kg ball', WOMEN_PRO: '6 kg ball' },
};

/** Wall balls rep count differs per division. */
const WALL_BALL_REPS: Record<Division, number> = {
  MEN_OPEN: 100,
  MEN_PRO: 100,
  WOMEN_OPEN: 75,
  WOMEN_PRO: 100,
};

/** Rough target seconds per full station effort, per division tier. */
const STATION_TARGET_SEC: Record<number, number> = {
  1: 240, 2: 180, 3: 210, 4: 270, 5: 250, 6: 120, 7: 260, 8: 300,
};
const RUN_PACE_SEC_PER_KM: Record<Division, number> = {
  MEN_OPEN: 330,
  MEN_PRO: 300,
  WOMEN_OPEN: 360,
  WOMEN_PRO: 330,
};
const PRO_FACTOR: Record<Division, number> = {
  MEN_OPEN: 1,
  MEN_PRO: 0.9,
  WOMEN_OPEN: 1.05,
  WOMEN_PRO: 0.95,
};

export interface GenerateWorkoutArgs {
  type: WorkoutType;
  division: Division;
  /** COVERAGE/PRACTICE: which stations (1–8) to include; empty → picked via `pick`. */
  stationOrders: number[];
  excludedExerciseIds: string[];
  exercises: readonly Exercise[];
  substitutions: readonly SubstitutionRule[];
  /** Deterministic chooser: returns an int in [0, n). */
  pick: (n: number) => number;
}

export function generateWorkout(
  args: GenerateWorkoutArgs,
): { blocks: WorkoutBlock[]; totalTargetSec: number } {
  const { division, exercises, substitutions } = args;
  const stations = exercises
    .filter((e) => e.hyroxStationOrder !== null)
    .sort((a, b) => a.hyroxStationOrder! - b.hyroxStationOrder!);
  const running = exercises.find((e) => e.category === 'RUN');
  const blocks: WorkoutBlock[] = [];
  let order = 1;

  const resolveStation = (station: Exercise): { exercise: Exercise; original: Exercise | null } => {
    if (!args.excludedExerciseIds.includes(station.id)) return { exercise: station, original: null };
    const sub = listSubstitutes(station.id, substitutions, exercises).find(
      (s) => !args.excludedExerciseIds.includes(s.exercise.id),
    );
    return sub ? { exercise: sub.exercise, original: station } : { exercise: station, original: null };
  };

  const pushRun = (distanceM: number) => {
    blocks.push({
      order: order++,
      kind: 'RUN',
      exerciseId: running?.id ?? 'ex_run',
      exerciseName: 'Running',
      originalExerciseId: null,
      originalExerciseName: null,
      distanceM,
      reps: null,
      weightNote: null,
      targetSec: Math.round((RUN_PACE_SEC_PER_KM[division] * distanceM) / 1000),
    });
  };

  const pushStation = (station: Exercise, volumeFactor: number) => {
    const { exercise, original } = resolveStation(station);
    const stationOrder = station.hyroxStationOrder!;
    const isWallBalls = stationOrder === 8;
    const baseReps = isWallBalls ? WALL_BALL_REPS[division] : station.defaultSpec.reps;
    blocks.push({
      order: order++,
      kind: 'STATION',
      exerciseId: exercise.id,
      exerciseName: exercise.name,
      originalExerciseId: original?.id ?? null,
      originalExerciseName: original?.name ?? null,
      distanceM: station.defaultSpec.distanceM
        ? Math.round(station.defaultSpec.distanceM * volumeFactor)
        : null,
      reps: baseReps ? Math.round(baseReps * volumeFactor) : null,
      weightNote: WEIGHT_NOTES[stationOrder]?.[division] ?? null,
      targetSec: Math.round(STATION_TARGET_SEC[stationOrder]! * PRO_FACTOR[division] * volumeFactor),
    });
  };

  const chooseStations = (count: number): Exercise[] => {
    if (args.stationOrders.length > 0) {
      return stations.filter((s) => args.stationOrders.includes(s.hyroxStationOrder!));
    }
    const pool = [...stations];
    const chosen: Exercise[] = [];
    while (chosen.length < count && pool.length > 0) {
      chosen.push(pool.splice(args.pick(pool.length), 1)[0]!);
    }
    return chosen.sort((a, b) => a.hyroxStationOrder! - b.hyroxStationOrder!);
  };

  switch (args.type) {
    case 'FULL_SIMULATION':
      for (const station of stations) {
        pushRun(1000);
        pushStation(station, 1);
      }
      break;
    case 'COVERAGE':
      for (const station of chooseStations(4)) {
        pushRun(600);
        pushStation(station, 1);
      }
      break;
    case 'QUICK':
      for (const station of chooseStations(4)) {
        pushRun(400);
        pushStation(station, 0.5);
      }
      break;
    case 'PRACTICE': {
      const target = chooseStations(1)[0] ?? stations[0]!;
      for (let round = 0; round < 3; round++) {
        pushRun(200);
        pushStation(target, 0.5);
      }
      break;
    }
  }

  return { blocks, totalTargetSec: blocks.reduce((s, b) => s + b.targetSec, 0) };
}

/** Customize step (§46): swap one station block for a substitute exercise. */
export function replaceBlockExercise(
  block: WorkoutBlock,
  replacement: Exercise,
): WorkoutBlock {
  return {
    ...block,
    exerciseId: replacement.id,
    exerciseName: replacement.name,
    originalExerciseId: block.originalExerciseId ?? block.exerciseId,
    originalExerciseName: block.originalExerciseName ?? block.exerciseName,
  };
}

// ── Active workout session (blueprint §48–49) ───────────────────────────────
export const WORKOUT_SESSION_STATUSES = [
  'READY',
  'STARTED',
  'PAUSED',
  'COMPLETED',
  'PARTIAL',
] as const;
export type WorkoutSessionStatus = (typeof WORKOUT_SESSION_STATUSES)[number];

export const WORKOUT_SESSION_TRANSITIONS: TransitionMap<WorkoutSessionStatus> = {
  READY: ['STARTED'],
  STARTED: ['PAUSED', 'COMPLETED', 'PARTIAL'],
  PAUSED: ['STARTED', 'PARTIAL'],
  COMPLETED: [],
  PARTIAL: [],
};

export interface WorkoutBlockResult {
  order: number;
  durationSec: number;
}

export interface WorkoutSession {
  id: string;
  workoutId: string;
  memberId: string;
  status: WorkoutSessionStatus;
  /** Order of the block currently being executed. */
  currentBlock: number;
  startedAt: IsoDate | null;
  endedAt: IsoDate | null;
  blockResults: WorkoutBlockResult[];
  pauseCount: number;
  totalPauseSec: number;
  createdAt: IsoDate;
}

export function sessionActiveSec(session: Pick<WorkoutSession, 'blockResults'>): number {
  return session.blockResults.reduce((s, r) => s + r.durationSec, 0);
}

export function sessionCompletionPct(
  session: Pick<WorkoutSession, 'blockResults'>,
  totalBlocks: number,
): number {
  if (totalBlocks === 0) return 0;
  return Math.round((session.blockResults.length / totalBlocks) * 100);
}
