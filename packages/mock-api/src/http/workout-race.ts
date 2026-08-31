import {
  createWorkout,
  finishWorkoutSession,
  pauseWorkoutSession,
  recordBlockResult,
  registerForRace,
  replaceWorkoutBlock,
  startWorkoutSession,
  updateUserRace,
} from '@hyrox/application';
import type { AppError } from '@hyrox/application';
import {
  BlockResultSchema,
  FinishSessionSchema,
  GenerateWorkoutSchema,
  RegisterRaceSchema,
  ReplaceBlockSchema,
  UpdateUserRaceSchema,
} from '@hyrox/contracts';
import type { MyRaceView, RaceEventView, WorkoutHistoryItemView, WorkoutSessionView } from '@hyrox/contracts';
import {
  analyzeRace,
  msOf,
  predictRaceSec,
  raceReadinessScore,
  sessionActiveSec,
  sessionCompletionPct,
} from '@hyrox/domain';
import { HttpResponse, http, type HttpHandler } from 'msw';
import type { MockApiState } from './handlers';
import { jsonError, parseBody, requireMember } from './helpers';

const fromAppError = (error: AppError) => jsonError(error.status, error.code, error.message);
const param = (params: Record<string, string | readonly string[] | undefined>, key: string): string =>
  String(params[key] ?? '');

export function createWorkoutRaceHandlers(state: MockApiState): HttpHandler[] {
  const db = () => state.db;
  const deps = () => state.deps;

  const sessionView = (sessionId: string): WorkoutSessionView | null => {
    const session = deps().workout.sessions.byId(sessionId);
    if (!session) return null;
    const workout = deps().workout.workouts.byId(session.workoutId)!;
    return {
      session,
      workout,
      activeSec: sessionActiveSec(session),
      completionPct: sessionCompletionPct(session, workout.blocks.length),
    };
  };

  return [
    // ── Exercise library & generator ────────────────────────────────────────
    http.get('*/api/exercises', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      return HttpResponse.json({ exercises: db().exercises, substitutions: db().substitutions });
    }),

    http.post('*/api/workouts/generate', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, GenerateWorkoutSchema);
      if (!body.ok) return body.response;
      const res = createWorkout(deps(), { memberId: auth.value.id, ...body.data });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    http.get('*/api/workouts/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const workout = deps().workout.workouts.byId(param(params, 'id'));
      if (!workout || workout.memberId !== auth.value.id)
        return jsonError(404, 'NOT_FOUND', 'Workout not found.');
      return HttpResponse.json(workout);
    }),

    http.post('*/api/workouts/:id/replace', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, ReplaceBlockSchema);
      if (!body.ok) return body.response;
      const res = replaceWorkoutBlock(deps(), {
        workoutId: param(params, 'id'),
        memberId: auth.value.id,
        ...body.data,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // ── Active session ──────────────────────────────────────────────────────
    http.post('*/api/workouts/:id/start', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = startWorkoutSession(deps(), {
        workoutId: param(params, 'id'),
        memberId: auth.value.id,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(sessionView(res.value.id), { status: 201 });
    }),

    http.get('*/api/workout-sessions', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const items: WorkoutHistoryItemView[] = deps()
        .workout.sessions.forMember(auth.value.id)
        .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt))
        .map((session) => {
          const workout = deps().workout.workouts.byId(session.workoutId)!;
          return {
            session,
            workoutType: workout.type,
            division: workout.division,
            totalBlocks: workout.blocks.length,
            activeSec: sessionActiveSec(session),
            completionPct: sessionCompletionPct(session, workout.blocks.length),
          };
        });
      return HttpResponse.json(items);
    }),

    http.get('*/api/workout-sessions/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const view = sessionView(param(params, 'id'));
      if (!view || view.session.memberId !== auth.value.id)
        return jsonError(404, 'NOT_FOUND', 'Session not found.');
      return HttpResponse.json(view);
    }),

    http.post('*/api/workout-sessions/:id/block', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, BlockResultSchema);
      if (!body.ok) return body.response;
      const res = recordBlockResult(deps(), {
        sessionId: param(params, 'id'),
        memberId: auth.value.id,
        ...body.data,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(sessionView(res.value.id));
    }),

    http.post('*/api/workout-sessions/:id/pause', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = pauseWorkoutSession(deps(), {
        sessionId: param(params, 'id'),
        memberId: auth.value.id,
        resume: false,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(sessionView(res.value.id));
    }),

    http.post('*/api/workout-sessions/:id/resume', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = (await request.json().catch(() => ({}))) as { pausedSec?: number };
      const res = pauseWorkoutSession(deps(), {
        sessionId: param(params, 'id'),
        memberId: auth.value.id,
        resume: true,
        pausedSec: typeof body.pausedSec === 'number' ? body.pausedSec : undefined,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(sessionView(res.value.id));
    }),

    http.post('*/api/workout-sessions/:id/finish', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, FinishSessionSchema);
      if (!body.ok) return body.response;
      const res = finishWorkoutSession(deps(), {
        sessionId: param(params, 'id'),
        memberId: auth.value.id,
        partial: body.data.partial,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json({
        ...sessionView(res.value.session.id),
        activityId: res.value.activityId,
      });
    }),

    // ── Races ───────────────────────────────────────────────────────────────
    http.get('*/api/races', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const url = new URL(request.url);
      const region = url.searchParams.get('region');
      const scope = url.searchParams.get('scope') ?? 'upcoming';
      const me = auth.value.id;
      const views: RaceEventView[] = db()
        .raceEvents.filter((e) => !region || e.region === region)
        .filter((e) =>
          scope === 'results' ? e.status === 'COMPLETED' : e.status !== 'COMPLETED',
        )
        .sort((a, b) => msOf(a.startsAt) - msOf(b.startsAt))
        .map((event) => ({
          event,
          joined: deps()
            .races.userRaces.forEvent(event.id)
            .some((r) => r.memberId === me && r.status !== 'CANCELLED'),
          participantCount: deps()
            .races.userRaces.forEvent(event.id)
            .filter((r) => r.status !== 'CANCELLED').length,
        }));
      return HttpResponse.json(views);
    }),

    http.get('*/api/races/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const event = db().raceEvents.find((e) => e.id === param(params, 'id'));
      if (!event) return jsonError(404, 'NOT_FOUND', 'Race not found.');
      const regs = deps()
        .races.userRaces.forEvent(event.id)
        .filter((r) => r.status !== 'CANCELLED');
      const mine = regs.find((r) => r.memberId === me) ?? null;
      return HttpResponse.json({
        view: { event, joined: mine !== null, participantCount: regs.length },
        myRace: mine ? { userRace: mine, event } : null,
      });
    }),

    http.post('*/api/races/:id/register', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, RegisterRaceSchema);
      if (!body.ok) return body.response;
      const res = registerForRace(deps(), {
        memberId: auth.value.id,
        raceEventId: param(params, 'id'),
        ...body.data,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    http.get('*/api/me/races', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const sims = deps()
        .workout.sessions.forMember(me)
        .filter((s) => {
          if (s.status !== 'COMPLETED' && s.status !== 'PARTIAL') return false;
          const workout = deps().workout.workouts.byId(s.workoutId);
          return workout?.type === 'FULL_SIMULATION' && s.status === 'COMPLETED';
        });
      const predictionSec = predictRaceSec(sims.map((s) => sessionActiveSec(s)));
      const activityDates = deps()
        .athlete.activities.forMember(me)
        .map((a) => a.startedAt);
      const readinessScore = raceReadinessScore(activityDates, deps().clock.now());

      const views: MyRaceView[] = deps()
        .races.userRaces.forMember(me)
        .filter((r) => r.status !== 'CANCELLED')
        .map((userRace) => {
          const event = deps().races.events.byId(userRace.raceEventId)!;
          return {
            userRace,
            event,
            daysToRace: Math.ceil((msOf(event.startsAt) - msOf(deps().clock.now())) / (24 * 3600_000)),
            predictionSec,
            readinessScore,
            simulationCount: sims.length,
            analysis:
              userRace.resultSec !== null
                ? analyzeRace(userRace.resultSec, userRace.goalSec, predictionSec)
                : null,
          };
        })
        .sort((a, b) => msOf(a.event.startsAt) - msOf(b.event.startsAt));
      return HttpResponse.json(views);
    }),

    http.patch('*/api/me/races/:id', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateUserRaceSchema);
      if (!body.ok) return body.response;
      const res = updateUserRace(deps(), {
        userRaceId: param(params, 'id'),
        memberId: auth.value.id,
        patch: body.data,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),
  ];
}
