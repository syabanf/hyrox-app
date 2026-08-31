import type {
  Division,
  GeneratedWorkout,
  Result,
  WorkoutSession,
  WorkoutType,
} from '@hyrox/domain';
import {
  WORKOUT_SESSION_TRANSITIONS,
  canTransition,
  err,
  generateWorkout,
  ok,
  replaceBlockExercise,
  sessionActiveSec,
  sessionCompletionPct,
} from '@hyrox/domain';
import type { AppError } from '../common';
import { appError } from '../common';
import type { UseCaseDeps } from '../ports';
import { saveActivity } from './athlete';

export function createWorkout(
  deps: UseCaseDeps,
  args: {
    memberId: string;
    type: WorkoutType;
    division: Division;
    stationOrders: number[];
    excludedExerciseIds: string[];
  },
): Result<GeneratedWorkout, AppError> {
  // Deterministic-enough chooser without Math.random: derive from the id counter.
  let seedCounter = 0;
  const pick = (n: number) => {
    seedCounter = (seedCounter * 31 + args.memberId.length + n * 7 + 13) % 997;
    return seedCounter % n;
  };
  const { blocks, totalTargetSec } = generateWorkout({
    type: args.type,
    division: args.division,
    stationOrders: args.stationOrders,
    excludedExerciseIds: args.excludedExerciseIds,
    exercises: deps.workout.exercises.all(),
    substitutions: deps.workout.substitutions.all(),
    pick,
  });
  if (blocks.length === 0)
    return err(appError('EMPTY_WORKOUT', 'No blocks could be generated — check the exercise library.'));
  const workout: GeneratedWorkout = {
    id: deps.ids.next('wko'),
    memberId: args.memberId,
    type: args.type,
    division: args.division,
    blocks,
    excludedExerciseIds: args.excludedExerciseIds,
    totalTargetSec,
    createdAt: deps.clock.now(),
  };
  deps.workout.workouts.save(workout);
  return ok(workout);
}

export function replaceWorkoutBlock(
  deps: UseCaseDeps,
  args: { workoutId: string; memberId: string; order: number; exerciseId: string },
): Result<GeneratedWorkout, AppError> {
  const workout = deps.workout.workouts.byId(args.workoutId);
  if (!workout || workout.memberId !== args.memberId)
    return err(appError('NOT_FOUND', 'Workout not found.', 404));
  const exercise = deps.workout.exercises.byId(args.exerciseId);
  if (!exercise) return err(appError('NOT_FOUND', 'Exercise not found.', 404));
  const idx = workout.blocks.findIndex((b) => b.order === args.order);
  if (idx < 0) return err(appError('NOT_FOUND', 'Block not found.', 404));
  if (workout.blocks[idx]!.kind !== 'STATION')
    return err(appError('NOT_A_STATION', 'Only station blocks can be replaced.'));
  workout.blocks[idx] = replaceBlockExercise(workout.blocks[idx]!, exercise);
  deps.workout.workouts.save(workout);
  return ok(workout);
}

export function startWorkoutSession(
  deps: UseCaseDeps,
  args: { workoutId: string; memberId: string },
): Result<WorkoutSession, AppError> {
  const workout = deps.workout.workouts.byId(args.workoutId);
  if (!workout || workout.memberId !== args.memberId)
    return err(appError('NOT_FOUND', 'Workout not found.', 404));
  const now = deps.clock.now();
  const session: WorkoutSession = {
    id: deps.ids.next('wses'),
    workoutId: workout.id,
    memberId: args.memberId,
    status: 'STARTED',
    currentBlock: workout.blocks[0]?.order ?? 1,
    startedAt: now,
    endedAt: null,
    blockResults: [],
    pauseCount: 0,
    totalPauseSec: 0,
    createdAt: now,
  };
  deps.workout.sessions.save(session);
  return ok(session);
}

function requireOwnSession(
  deps: UseCaseDeps,
  sessionId: string,
  memberId: string,
): Result<WorkoutSession, AppError> {
  const session = deps.workout.sessions.byId(sessionId);
  if (!session || session.memberId !== memberId)
    return err(appError('NOT_FOUND', 'Session not found.', 404));
  return ok(session);
}

export function recordBlockResult(
  deps: UseCaseDeps,
  args: { sessionId: string; memberId: string; order: number; durationSec: number },
): Result<WorkoutSession, AppError> {
  const found = requireOwnSession(deps, args.sessionId, args.memberId);
  if (!found.ok) return found;
  const session = found.value;
  if (session.status !== 'STARTED')
    return err(appError('NOT_RUNNING', `Session is ${session.status}.`));
  const workout = deps.workout.workouts.byId(session.workoutId)!;
  if (session.blockResults.some((r) => r.order === args.order))
    return err(appError('ALREADY_DONE', 'Block already completed.'));
  session.blockResults.push({ order: args.order, durationSec: args.durationSec });
  const next = workout.blocks.find(
    (b) => !session.blockResults.some((r) => r.order === b.order),
  );
  session.currentBlock = next?.order ?? args.order;
  deps.workout.sessions.save(session);
  return ok(session);
}

export function pauseWorkoutSession(
  deps: UseCaseDeps,
  args: { sessionId: string; memberId: string; resume: boolean; pausedSec?: number },
): Result<WorkoutSession, AppError> {
  const found = requireOwnSession(deps, args.sessionId, args.memberId);
  if (!found.ok) return found;
  const session = found.value;
  const to = args.resume ? 'STARTED' : 'PAUSED';
  if (!canTransition(WORKOUT_SESSION_TRANSITIONS, session.status, to))
    return err(appError('INVALID_TRANSITION', `Session is ${session.status}.`));
  session.status = to;
  if (args.resume && args.pausedSec) {
    session.pauseCount += 1;
    session.totalPauseSec += args.pausedSec;
  }
  deps.workout.sessions.save(session);
  return ok(session);
}

/**
 * COMPLETED when every block is done, PARTIAL otherwise (§49 "Stop & Save").
 * Either way the effort lands in the training log as a WORKOUT activity.
 */
export function finishWorkoutSession(
  deps: UseCaseDeps,
  args: { sessionId: string; memberId: string; partial: boolean },
): Result<{ session: WorkoutSession; activityId: string | null }, AppError> {
  const found = requireOwnSession(deps, args.sessionId, args.memberId);
  if (!found.ok) return found;
  const session = found.value;
  const workout = deps.workout.workouts.byId(session.workoutId)!;
  const allDone = session.blockResults.length >= workout.blocks.length;
  const to = allDone && !args.partial ? 'COMPLETED' : 'PARTIAL';
  if (!canTransition(WORKOUT_SESSION_TRANSITIONS, session.status, to))
    return err(appError('INVALID_TRANSITION', `Session is ${session.status}.`));
  session.status = to;
  session.endedAt = deps.clock.now();
  deps.workout.sessions.save(session);

  let activityId: string | null = null;
  const activeSec = sessionActiveSec(session);
  if (activeSec > 0) {
    const labels: Record<WorkoutType, string> = {
      FULL_SIMULATION: 'HYROX Full Simulation',
      COVERAGE: 'HYROX Coverage',
      QUICK: 'HYROX Quick Workout',
      PRACTICE: 'HYROX Station Practice',
    };
    const saved = saveActivity(deps, {
      memberId: args.memberId,
      type: 'WORKOUT',
      title: labels[workout.type],
      description: `${sessionCompletionPct(session, workout.blocks.length)}% complete · ${workout.division.replaceAll('_', ' ')}`,
      startedAt: session.startedAt ?? session.createdAt,
      points: [],
      manualElapsedSec: activeSec,
      gearId: null,
      visibility: 'EVERYONE',
      photos: [],
    });
    if (saved.ok) activityId = saved.value.activity.id;
  }
  return ok({ session, activityId });
}
