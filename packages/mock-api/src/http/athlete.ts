import {
  addActivityComment,
  deleteActivity,
  generateBookingReminders,
  joinChallenge,
  saveActivity,
  saveRouteFromActivity,
  toggleClubMembership,
  toggleFollow,
  toggleKudos,
  updateActivity,
  updateAthleteSettings,
  upsertGear,
} from '@hyrox/application';
import type { AppError } from '@hyrox/application';
import {
  ActivityCommentSchema,
  SaveActivitySchema,
  SaveRouteSchema,
  UpdateActivitySchema,
  UpdateAthleteSettingsSchema,
  UpsertGearSchema,
} from '@hyrox/contracts';
import type {
  ActivityCardView,
  ActivityDetailView,
  AthleteLite,
  AthleteStatsView,
  ChallengeView,
  ClubView,
  RouteView,
  SegmentDetailView,
  SegmentListView,
  SocialView,
} from '@hyrox/contracts';
import type { Activity, SegmentEffort } from '@hyrox/domain';
import {
  canViewActivity,
  challengeProgressKm,
  computeActivityStats,
  findGroupedActivities,
  msOf,
  personalRecords,
  weeklyBuckets,
} from '@hyrox/domain';
import { HttpResponse, http, type HttpHandler } from 'msw';
import type { MockDb } from '../db';
import type { MockApiState } from './handlers';
import { jsonError, parseBody, requireMember } from './helpers';

const fromAppError = (error: AppError) => jsonError(error.status, error.code, error.message);
const param = (params: Record<string, string | readonly string[] | undefined>, key: string): string =>
  String(params[key] ?? '');

const memberName = (db: MockDb, memberId: string): string =>
  db.members.find((m) => m.id === memberId)?.fullName ?? 'Athlete';
const memberAvatar = (db: MockDb, memberId: string): string | null =>
  db.members.find((m) => m.id === memberId)?.avatarUrl ?? null;

function downsample<T>(points: readonly T[], max: number): T[] {
  if (points.length <= max) return [...points];
  const step = (points.length - 1) / (max - 1);
  return Array.from({ length: max }, (_, i) => points[Math.round(i * step)]!);
}

function activityCard(db: MockDb, activity: Activity, viewerId: string): ActivityCardView {
  const kudos = db.kudos.filter((k) => k.activityId === activity.id);
  return {
    id: activity.id,
    memberId: activity.memberId,
    memberName: memberName(db, activity.memberId),
    memberAvatarUrl: memberAvatar(db, activity.memberId),
    isOwn: activity.memberId === viewerId,
    type: activity.type,
    title: activity.title,
    startedAt: activity.startedAt,
    distanceM: activity.distanceM,
    movingSec: activity.movingSec,
    elapsedSec: activity.elapsedSec,
    avgPaceSecPerKm: activity.avgPaceSecPerKm,
    elevationGainM: activity.elevationGainM,
    visibility: activity.visibility,
    thumbnail: downsample(activity.points, 40),
    photoCount: activity.photos.length,
    kudosCount: kudos.length,
    hasKudoed: kudos.some((k) => k.memberId === viewerId),
    commentCount: db.activityComments.filter((c) => c.activityId === activity.id).length,
  };
}

/** Strava-style leaderboard: each athlete's best effort, ranked. */
function segmentLeaderboard(db: MockDb, segmentId: string) {
  const bestByMember = new Map<string, SegmentEffort>();
  for (const effort of db.segmentEfforts.filter((e) => e.segmentId === segmentId)) {
    const current = bestByMember.get(effort.memberId);
    if (!current || effort.elapsedSec < current.elapsedSec) bestByMember.set(effort.memberId, effort);
  }
  return [...bestByMember.values()].sort((a, b) => a.elapsedSec - b.elapsedSec);
}

function weeklyKmFor(db: MockDb, memberId: string, nowIso: string): number {
  const buckets = weeklyBuckets(
    db.activities.filter((a) => a.memberId === memberId),
    nowIso,
    1,
  );
  return buckets[0]?.distanceKm ?? 0;
}

function athleteLite(db: MockDb, memberId: string, viewerId: string, nowIso: string): AthleteLite {
  return {
    memberId,
    name: memberName(db, memberId),
    weeklyKm: weeklyKmFor(db, memberId, nowIso),
    isFollowing: db.follows.some((f) => f.followerId === viewerId && f.followeeId === memberId),
  };
}

export function createAthleteHandlers(state: MockApiState): HttpHandler[] {
  const db = () => state.db;
  const deps = () => state.deps;
  const nowIso = () => deps().clock.now();

  return [
    // ── Feed & activities ───────────────────────────────────────────────────
    http.get('*/api/athlete/feed', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const scope = new URL(request.url).searchParams.get('scope') ?? 'everyone';
      const me = auth.value.id;
      const follows = (id: string) =>
        db().follows.some((f) => f.followerId === me && f.followeeId === id);
      const cards = db()
        .activities.filter((a) => canViewActivity(a, me, follows))
        .filter((a) => scope !== 'following' || a.memberId === me || follows(a.memberId))
        .sort((a, b) => msOf(b.startedAt) - msOf(a.startedAt))
        .slice(0, 30)
        .map((a) => activityCard(db(), a, me));
      return HttpResponse.json(cards);
    }),

    http.get('*/api/athlete/activities', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const cards = db()
        .activities.filter((a) => a.memberId === auth.value.id)
        .sort((a, b) => msOf(b.startedAt) - msOf(a.startedAt))
        .map((a) => activityCard(db(), a, auth.value.id));
      return HttpResponse.json(cards);
    }),

    http.post('*/api/athlete/activities', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, SaveActivitySchema);
      if (!body.ok) return body.response;
      const res = saveActivity(deps(), { memberId: auth.value.id, ...body.data });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(activityCard(db(), res.value.activity, auth.value.id), {
        status: 201,
      });
    }),

    http.get('*/api/athlete/activities/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const activity = db().activities.find((a) => a.id === param(params, 'id'));
      if (!activity) return jsonError(404, 'NOT_FOUND', 'Activity not found.');
      const follows = (id: string) =>
        db().follows.some((f) => f.followerId === me && f.followeeId === id);
      if (!canViewActivity(activity, me, follows))
        return jsonError(403, 'FORBIDDEN', 'This activity is private.');

      const stats = computeActivityStats(activity.points);
      const efforts = db()
        .segmentEfforts.filter((e) => e.activityId === activity.id)
        .map((effort) => {
          const segment = db().segments.find((s) => s.id === effort.segmentId)!;
          const board = segmentLeaderboard(db(), effort.segmentId);
          const myBest = db()
            .segmentEfforts.filter(
              (e) => e.segmentId === effort.segmentId && e.memberId === effort.memberId,
            )
            .reduce((best, e) => Math.min(best, e.elapsedSec), Infinity);
          return {
            segmentId: segment.id,
            segmentName: segment.name,
            distanceM: segment.distanceM,
            elapsedSec: effort.elapsedSec,
            rank: board.findIndex((b) => b.memberId === effort.memberId) + 1,
            totalEfforts: board.length,
            isPersonalBest: effort.elapsedSec <= myBest,
          };
        });
      const view: ActivityDetailView = {
        ...activityCard(db(), activity, me),
        description: activity.description,
        points: activity.points,
        photos: activity.photos,
        splits: stats.splits,
        bestSplitPaceSec: stats.bestSplitPaceSec,
        gearName: activity.gearId
          ? (db().gear.find((g) => g.id === activity.gearId)?.name ?? null)
          : null,
        efforts,
        comments: db()
          .activityComments.filter((c) => c.activityId === activity.id)
          .sort((a, b) => msOf(a.createdAt) - msOf(b.createdAt))
          .map((c) => ({
            id: c.id,
            memberId: c.memberId,
            memberName: memberName(db(), c.memberId),
            text: c.text,
            createdAt: c.createdAt,
          })),
        groupedWith: findGroupedActivities(activity, db().activities).map((activityId) => ({
          activityId,
          memberName: memberName(
            db(),
            db().activities.find((a) => a.id === activityId)?.memberId ?? '',
          ),
        })),
      };
      return HttpResponse.json(view);
    }),

    http.patch('*/api/athlete/activities/:id', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateActivitySchema);
      if (!body.ok) return body.response;
      const res = updateActivity(deps(), {
        activityId: param(params, 'id'),
        memberId: auth.value.id,
        patch: body.data,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(activityCard(db(), res.value, auth.value.id));
    }),

    http.delete('*/api/athlete/activities/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = deleteActivity(deps(), {
        activityId: param(params, 'id'),
        memberId: auth.value.id,
        removeArtifacts: (activityId) => {
          const d = db();
          d.activities.splice(d.activities.findIndex((a) => a.id === activityId), 1);
          d.kudos = d.kudos.filter((k) => k.activityId !== activityId);
          d.activityComments = d.activityComments.filter((c) => c.activityId !== activityId);
          d.segmentEfforts = d.segmentEfforts.filter((e) => e.activityId !== activityId);
        },
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // ── Routes & heatmap ────────────────────────────────────────────────────
    http.get('*/api/athlete/routes', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const views: RouteView[] = deps()
        .athlete.routes.forMember(auth.value.id)
        .map((r) => ({
          id: r.id,
          name: r.name,
          distanceM: r.distanceM,
          points: downsample(r.points, 200),
          createdAt: r.createdAt,
        }));
      return HttpResponse.json(views);
    }),

    http.get('*/api/athlete/routes/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const route = deps().athlete.routes.byId(param(params, 'id'));
      if (!route || route.memberId !== auth.value.id)
        return jsonError(404, 'NOT_FOUND', 'Route not found.');
      return HttpResponse.json(route);
    }),

    http.post('*/api/athlete/routes', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, SaveRouteSchema);
      if (!body.ok) return body.response;
      const res = saveRouteFromActivity(deps(), { memberId: auth.value.id, ...body.data });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    http.delete('*/api/athlete/routes/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const route = deps().athlete.routes.byId(param(params, 'id'));
      if (!route || route.memberId !== auth.value.id)
        return jsonError(404, 'NOT_FOUND', 'Route not found.');
      deps().athlete.routes.remove(route.id);
      return HttpResponse.json({ ok: true });
    }),

    http.get('*/api/athlete/heatmap', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const tracks = db()
        .activities.filter((a) => a.memberId === auth.value.id && a.points.length > 1)
        .map((a) => downsample(a.points, 120));
      return HttpResponse.json({ tracks });
    }),

    http.post('*/api/athlete/activities/:id/kudos', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = toggleKudos(deps(), {
        activityId: param(params, 'id'),
        memberId: auth.value.id,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    http.post('*/api/athlete/activities/:id/comments', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, ActivityCommentSchema);
      if (!body.ok) return body.response;
      const res = addActivityComment(deps(), {
        activityId: param(params, 'id'),
        memberId: auth.value.id,
        text: body.data.text,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    // ── Stats, goal, gear, settings ─────────────────────────────────────────
    http.get('*/api/athlete/stats', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const mine = db().activities.filter((a) => a.memberId === me);
      const weekly = weeklyBuckets(mine, nowIso(), 8);
      const settings = deps().athlete.settings.get(me);
      const thisWeekKm = weekly[weekly.length - 1]?.distanceKm ?? 0;
      const view: AthleteStatsView = {
        weekly,
        totals: {
          activities: mine.length,
          distanceKm: Math.round(mine.reduce((s, a) => s + a.distanceM, 0) / 100) / 10,
          movingSec: mine.reduce((s, a) => s + a.movingSec, 0),
        },
        thisWeekKm,
        goal: { targetKm: settings.weeklyGoalKm, currentKm: thisWeekKm },
        prs: personalRecords(mine),
        gear: deps().athlete.gear.forMember(me),
        settings,
        followingCount: db().follows.filter((f) => f.followerId === me).length,
        followerCount: db().follows.filter((f) => f.followeeId === me).length,
      };
      return HttpResponse.json(view);
    }),

    http.get('*/api/me/settings', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      return HttpResponse.json(deps().athlete.settings.get(auth.value.id));
    }),

    http.put('*/api/me/settings', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpdateAthleteSettingsSchema);
      if (!body.ok) return body.response;
      return HttpResponse.json(
        updateAthleteSettings(deps(), { memberId: auth.value.id, patch: body.data }),
      );
    }),

    http.post('*/api/athlete/gear', async ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertGearSchema);
      if (!body.ok) return body.response;
      const res = upsertGear(deps(), { memberId: auth.value.id, gearId: null, ...body.data });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value, { status: 201 });
    }),

    http.patch('*/api/athlete/gear/:id', async ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const body = await parseBody(request, UpsertGearSchema);
      if (!body.ok) return body.response;
      const res = upsertGear(deps(), {
        memberId: auth.value.id,
        gearId: param(params, 'id'),
        ...body.data,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // ── Segments ────────────────────────────────────────────────────────────
    http.get('*/api/athlete/segments', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const views: SegmentListView[] = db().segments.map((segment) => {
        const board = segmentLeaderboard(db(), segment.id);
        const myIdx = board.findIndex((e) => e.memberId === me);
        return {
          segment,
          effortCount: db().segmentEfforts.filter((e) => e.segmentId === segment.id).length,
          bestElapsedSec: board[0]?.elapsedSec ?? null,
          myBestElapsedSec: myIdx >= 0 ? board[myIdx]!.elapsedSec : null,
          myRank: myIdx >= 0 ? myIdx + 1 : null,
        };
      });
      return HttpResponse.json(views);
    }),

    http.get('*/api/athlete/segments/:id', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const segment = db().segments.find((s) => s.id === param(params, 'id'));
      if (!segment) return jsonError(404, 'NOT_FOUND', 'Segment not found.');
      const board = segmentLeaderboard(db(), segment.id);
      const view: SegmentDetailView = {
        segment,
        leaderboard: board.slice(0, 20).map((effort, i) => ({
          rank: i + 1,
          memberId: effort.memberId,
          memberName: memberName(db(), effort.memberId),
          elapsedSec: effort.elapsedSec,
          createdAt: effort.createdAt,
          isMe: effort.memberId === auth.value.id,
        })),
        myRank: (() => {
          const idx = board.findIndex((e) => e.memberId === auth.value.id);
          return idx >= 0 ? idx + 1 : null;
        })(),
      };
      return HttpResponse.json(view);
    }),

    // ── Challenges & clubs ──────────────────────────────────────────────────
    // Public profile of another athlete (or yourself).
    http.get('*/api/athlete/profile/:memberId', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const targetId = param(params, 'memberId');
      const target = db().members.find((m) => m.id === targetId);
      if (!target) return jsonError(404, 'NOT_FOUND', 'Athlete not found.');
      const follows = (id: string) =>
        db().follows.some((f) => f.followerId === me && f.followeeId === id);
      const visible = db()
        .activities.filter((a) => a.memberId === targetId)
        .filter((a) => canViewActivity(a, me, follows))
        .sort((a, b) => msOf(b.startedAt) - msOf(a.startedAt));
      return HttpResponse.json({
        member: { id: target.id, fullName: target.fullName, avatarUrl: target.avatarUrl },
        isMe: targetId === me,
        isFollowing: follows(targetId),
        followerCount: db().follows.filter((f) => f.followeeId === targetId).length,
        followingCount: db().follows.filter((f) => f.followerId === targetId).length,
        totals: {
          activities: visible.length,
          distanceKm: visible.reduce((sum, a) => sum + a.distanceM, 0) / 1000,
          movingSec: visible.reduce((sum, a) => sum + a.movingSec, 0),
        },
        activities: visible.slice(0, 20).map((a) => activityCard(db(), a, me)),
      });
    }),

    http.get('*/api/athlete/challenges', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const views: ChallengeView[] = db().challenges.map((challenge) => {
        const participants = deps().athlete.challenges.participants(challenge.id);
        const rows = participants
          .map((memberId) => ({
            memberId,
            km: challengeProgressKm(
              challenge,
              db().activities.filter((a) => a.memberId === memberId),
            ),
          }))
          .sort((a, b) => b.km - a.km);
        return {
          challenge,
          joined: participants.includes(me),
          participantCount: participants.length,
          progressKm: challengeProgressKm(
            challenge,
            db().activities.filter((a) => a.memberId === me),
          ),
          leaderboard: rows.slice(0, 5).map((r) => ({
            memberName: memberName(db(), r.memberId),
            km: r.km,
            isMe: r.memberId === me,
          })),
        };
      });
      return HttpResponse.json(views);
    }),

    http.post('*/api/athlete/challenges/:id/join', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = joinChallenge(deps(), {
        challengeId: param(params, 'id'),
        memberId: auth.value.id,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    http.get('*/api/athlete/clubs', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const views: ClubView[] = db().clubs.map((club) => {
        const rows = club.memberIds
          .map((memberId) => ({ memberId, km: weeklyKmFor(db(), memberId, nowIso()) }))
          .sort((a, b) => b.km - a.km);
        return {
          club,
          joined: club.memberIds.includes(me),
          memberCount: club.memberIds.length,
          weeklyLeaderboard: rows.slice(0, 5).map((r) => ({
            memberName: memberName(db(), r.memberId),
            km: r.km,
            isMe: r.memberId === me,
          })),
        };
      });
      return HttpResponse.json(views);
    }),

    http.post('*/api/athlete/clubs/:id/toggle', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = toggleClubMembership(deps(), {
        clubId: param(params, 'id'),
        memberId: auth.value.id,
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),

    // ── Social graph ────────────────────────────────────────────────────────
    http.get('*/api/athlete/social', ({ request }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const me = auth.value.id;
      const followingIds = db()
        .follows.filter((f) => f.followerId === me)
        .map((f) => f.followeeId);
      const followerIds = db()
        .follows.filter((f) => f.followeeId === me)
        .map((f) => f.followerId);
      const suggestionIds = db()
        .members.filter(
          (m) =>
            m.id !== me &&
            m.status === 'ACTIVE' &&
            !followingIds.includes(m.id) &&
            db().activities.some((a) => a.memberId === m.id),
        )
        .map((m) => m.id)
        .slice(0, 8);
      const view: SocialView = {
        following: followingIds.map((id) => athleteLite(db(), id, me, nowIso())),
        followers: followerIds.map((id) => athleteLite(db(), id, me, nowIso())),
        suggestions: suggestionIds.map((id) => athleteLite(db(), id, me, nowIso())),
      };
      return HttpResponse.json(view);
    }),

    http.post('*/api/athlete/follow/:memberId', ({ request, params }) => {
      const auth = requireMember(db(), request);
      if (!auth.ok) return auth.response;
      const res = toggleFollow(deps(), {
        followerId: auth.value.id,
        followeeId: param(params, 'memberId'),
      });
      if (!res.ok) return fromAppError(res.error);
      return HttpResponse.json(res.value);
    }),
  ];
}

export { generateBookingReminders };
