import { describe, expect, it } from 'vitest';
import type { Exercise, SubstitutionRule } from '../index';
import {
  analyzeRace,
  generateWorkout,
  listSubstitutes,
  predictRaceSec,
  raceReadinessScore,
  replaceBlockExercise,
  sessionActiveSec,
  sessionCompletionPct,
} from '../index';

const station = (
  id: string,
  name: string,
  order: number,
  spec: { distanceM?: number; reps?: number },
): Exercise => ({
  id,
  name,
  category: 'CONDITIONING',
  equipment: [name],
  hyroxStationOrder: order,
  difficulty: 2,
  defaultSpec: { distanceM: spec.distanceM ?? null, reps: spec.reps ?? null },
  videoUrl: null,
});

const EXERCISES: Exercise[] = [
  { id: 'ex_run', name: 'Running', category: 'RUN', equipment: [], hyroxStationOrder: null, difficulty: 1, defaultSpec: { distanceM: 1000, reps: null }, videoUrl: null },
  station('ex_ski', 'SkiErg', 1, { distanceM: 1000 }),
  station('ex_push', 'Sled Push', 2, { distanceM: 50 }),
  station('ex_pull', 'Sled Pull', 3, { distanceM: 50 }),
  station('ex_bbj', 'Burpee Broad Jump', 4, { distanceM: 80 }),
  station('ex_row', 'Row', 5, { distanceM: 1000 }),
  station('ex_carry', 'Farmers Carry', 6, { distanceM: 200 }),
  station('ex_lunge', 'Sandbag Lunge', 7, { distanceM: 100 }),
  station('ex_wb', 'Wall Balls', 8, { reps: 100 }),
  { id: 'ex_slam', name: 'Ball Slam', category: 'CONDITIONING', equipment: ['ball'], hyroxStationOrder: null, difficulty: 2, defaultSpec: { distanceM: null, reps: 30 }, videoUrl: null },
];
const SUBS: SubstitutionRule[] = [
  { originalExerciseId: 'ex_ski', alternativeExerciseId: 'ex_slam', similarity: 0.8, conversionNote: '1000m → 60 slams' },
  { originalExerciseId: 'ex_ski', alternativeExerciseId: 'ex_row', similarity: 0.9, conversionNote: '1:1 distance' },
];

const gen = (over: Partial<Parameters<typeof generateWorkout>[0]> = {}) =>
  generateWorkout({
    type: 'FULL_SIMULATION',
    division: 'MEN_OPEN',
    stationOrders: [],
    excludedExerciseIds: [],
    exercises: EXERCISES,
    substitutions: SUBS,
    pick: () => 0,
    ...over,
  });

describe('generateWorkout', () => {
  it('full simulation = 8 × (run 1km + station) in race order', () => {
    const { blocks } = gen();
    expect(blocks).toHaveLength(16);
    expect(blocks.filter((b) => b.kind === 'RUN')).toHaveLength(8);
    expect(blocks[0]!.kind).toBe('RUN');
    expect(blocks[0]!.distanceM).toBe(1000);
    expect(blocks[1]!.exerciseName).toBe('SkiErg');
    expect(blocks[15]!.exerciseName).toBe('Wall Balls');
    expect(blocks[15]!.reps).toBe(100);
  });

  it('division changes loads, reps and run targets', () => {
    const women = gen({ division: 'WOMEN_OPEN' });
    const wallBalls = women.blocks.find((b) => b.exerciseName === 'Wall Balls')!;
    expect(wallBalls.reps).toBe(75);
    expect(wallBalls.weightNote).toBe('4 kg ball');
    const men = gen();
    const menRun = men.blocks[0]!.targetSec;
    const womenRun = women.blocks[0]!.targetSec;
    expect(womenRun).toBeGreaterThan(menRun);
  });

  it('excluded station is substituted by the most similar alternative, keeping the original name', () => {
    const { blocks } = gen({ excludedExerciseIds: ['ex_ski'] });
    const first = blocks[1]!;
    expect(first.exerciseName).toBe('Row'); // similarity 0.9 beats 0.8
    expect(first.originalExerciseName).toBe('SkiErg');
  });

  it('quick workouts halve station volume', () => {
    const { blocks } = gen({ type: 'QUICK', stationOrders: [2] });
    const sled = blocks.find((b) => b.kind === 'STATION')!;
    expect(sled.distanceM).toBe(25);
    expect(blocks.find((b) => b.kind === 'RUN')!.distanceM).toBe(400);
  });

  it('practice repeats one station for 3 rounds', () => {
    const { blocks } = gen({ type: 'PRACTICE', stationOrders: [8] });
    expect(blocks).toHaveLength(6);
    expect(blocks.filter((b) => b.exerciseName === 'Wall Balls')).toHaveLength(3);
  });

  it('coverage respects the chosen stations', () => {
    const { blocks } = gen({ type: 'COVERAGE', stationOrders: [2, 6] });
    const names = blocks.filter((b) => b.kind === 'STATION').map((b) => b.exerciseName);
    expect(names).toEqual(['Sled Push', 'Farmers Carry']);
  });
});

describe('customize & session helpers', () => {
  it('replaceBlockExercise keeps the original for display', () => {
    const { blocks } = gen();
    const replaced = replaceBlockExercise(blocks[1]!, EXERCISES.find((e) => e.id === 'ex_slam')!);
    expect(replaced.exerciseName).toBe('Ball Slam');
    expect(replaced.originalExerciseName).toBe('SkiErg');
  });

  it('listSubstitutes sorts by similarity', () => {
    const subs = listSubstitutes('ex_ski', SUBS, EXERCISES);
    expect(subs.map((s) => s.exercise.name)).toEqual(['Row', 'Ball Slam']);
  });

  it('active time and completion percentage', () => {
    const session = { blockResults: [{ order: 1, durationSec: 300 }, { order: 2, durationSec: 200 }] };
    expect(sessionActiveSec(session)).toBe(500);
    expect(sessionCompletionPct(session, 16)).toBe(13);
    expect(sessionCompletionPct({ blockResults: [] }, 0)).toBe(0);
  });
});

describe('race prediction & analysis', () => {
  it('prediction is the best full simulation minus a race-day bump', () => {
    expect(predictRaceSec([5400, 5100, 5700])).toBe(Math.round(5100 * 0.97));
    expect(predictRaceSec([])).toBeNull();
  });

  it('readiness counts the last 4 weeks against 3 sessions/week', () => {
    const now = '2026-08-31T00:00:00.000Z';
    const dates = Array.from({ length: 6 }, (_, i) => new Date(Date.parse(now) - i * 4 * 24 * 3600_000).toISOString());
    expect(raceReadinessScore(dates, now)).toBe(50); // 6 of 12
    expect(raceReadinessScore([], now)).toBe(0);
  });

  it('analysis compares result to goal and prediction', () => {
    const a = analyzeRace(4500, 4600, 4400);
    expect(a.vsGoalSec).toBe(-100);
    expect(a.vsPredictionSec).toBe(100);
    expect(a.achievedGoal).toBe(true);
    expect(analyzeRace(4500, null, null).achievedGoal).toBeNull();
  });
});
