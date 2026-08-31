import type {
  Activity,
  ActivityComment,
  ActivityType,
  ActivityVisibility,
  AthleteSettings,
  Gear,
  Result,
  Route,
  SegmentEffort,
  TrackPoint,
} from '@hyrox/domain';
import { computeActivityStats, err, matchSegments, msOf, ok } from '@hyrox/domain';
import type { AppError } from '../common';
import { appError, notify } from '../common';
import type { UseCaseDeps } from '../ports';

export interface SaveActivityArgs {
  memberId: string;
  type: ActivityType;
  title: string;
  description: string;
  startedAt: string;
  points: TrackPoint[];
  manualElapsedSec: number | null;
  gearId: string | null;
  visibility: ActivityVisibility;
  photos: string[];
}

export function saveActivity(
  deps: UseCaseDeps,
  args: SaveActivityArgs,
): Result<{ activity: Activity; efforts: SegmentEffort[] }, AppError> {
  const member = deps.members.byId(args.memberId);
  if (!member) return err(appError('NOT_FOUND', 'Member not found.', 404));
  if (args.points.length < 2 && !args.manualElapsedSec) {
    return err(appError('EMPTY_ACTIVITY', 'Record some movement (or a manual duration) first.'));
  }
  const stats = computeActivityStats(args.points);
  const now = deps.clock.now();
  const activity: Activity = {
    id: deps.ids.next('act'),
    memberId: args.memberId,
    type: args.type,
    title: args.title,
    description: args.description,
    startedAt: args.startedAt,
    elapsedSec: args.manualElapsedSec ?? stats.elapsedSec,
    movingSec: stats.movingSec || (args.manualElapsedSec ?? 0),
    distanceM: stats.distanceM,
    avgPaceSecPerKm: stats.avgPaceSecPerKm,
    elevationGainM: stats.elevationGainM,
    points: args.points,
    photos: args.photos,
    visibility: args.visibility,
    gearId: args.gearId,
    createdAt: now,
  };
  deps.athlete.activities.save(activity);

  // Gear mileage accumulates automatically.
  if (args.gearId) {
    const gear = deps.athlete.gear.byId(args.gearId);
    if (gear && gear.memberId === args.memberId) {
      gear.distanceM += activity.distanceM;
      deps.athlete.gear.save(gear);
    }
  }

  // Segment efforts (GPS polyline matching) + PR notifications.
  const efforts: SegmentEffort[] = [];
  for (const match of matchSegments(deps.athlete.segments.all(), activity, activity.points)) {
    const previousBest = deps.athlete.efforts
      .forMember(args.memberId)
      .filter((e) => e.segmentId === match.segment.id)
      .reduce<number | null>((best, e) => (best === null ? e.elapsedSec : Math.min(best, e.elapsedSec)), null);
    const effort: SegmentEffort = {
      id: deps.ids.next('eff'),
      segmentId: match.segment.id,
      activityId: activity.id,
      memberId: args.memberId,
      elapsedSec: match.elapsedSec,
      createdAt: now,
    };
    deps.athlete.efforts.add(effort);
    efforts.push(effort);
    if (previousBest !== null && match.elapsedSec < previousBest) {
      notify(
        deps,
        args.memberId,
        'ANNOUNCEMENT',
        'New personal best!',
        `You set a PR on ${match.segment.name}.`,
      );
    }
  }
  return ok({ activity, efforts });
}

export function updateActivity(
  deps: UseCaseDeps,
  args: {
    activityId: string;
    memberId: string;
    patch: Partial<Pick<Activity, 'title' | 'description' | 'visibility' | 'gearId' | 'photos'>>;
  },
): Result<Activity, AppError> {
  const activity = deps.athlete.activities.byId(args.activityId);
  if (!activity) return err(appError('NOT_FOUND', 'Activity not found.', 404));
  if (activity.memberId !== args.memberId)
    return err(appError('FORBIDDEN', 'You can only edit your own activities.', 403));
  Object.assign(activity, args.patch);
  deps.athlete.activities.save(activity);
  return ok(activity);
}

/** Deleting an activity removes its social artifacts and gives gear its mileage back. */
export function deleteActivity(
  deps: UseCaseDeps,
  args: {
    activityId: string;
    memberId: string;
    removeArtifacts: (activityId: string) => void;
  },
): Result<{ deleted: true }, AppError> {
  const activity = deps.athlete.activities.byId(args.activityId);
  if (!activity) return err(appError('NOT_FOUND', 'Activity not found.', 404));
  if (activity.memberId !== args.memberId)
    return err(appError('FORBIDDEN', 'You can only delete your own activities.', 403));
  if (activity.gearId) {
    const gear = deps.athlete.gear.byId(activity.gearId);
    if (gear) {
      gear.distanceM = Math.max(0, gear.distanceM - activity.distanceM);
      deps.athlete.gear.save(gear);
    }
  }
  args.removeArtifacts(activity.id);
  return ok({ deleted: true });
}

export function saveRouteFromActivity(
  deps: UseCaseDeps,
  args: { memberId: string; activityId: string; name: string },
): Result<Route, AppError> {
  const activity = deps.athlete.activities.byId(args.activityId);
  if (!activity) return err(appError('NOT_FOUND', 'Activity not found.', 404));
  if (activity.memberId !== args.memberId)
    return err(appError('FORBIDDEN', 'You can only save routes from your own activities.', 403));
  if (activity.points.length < 2)
    return err(appError('NO_TRACK', 'This activity has no GPS track.'));
  const route: Route = {
    id: deps.ids.next('rte'),
    memberId: args.memberId,
    name: args.name,
    points: activity.points,
    distanceM: activity.distanceM,
    createdAt: deps.clock.now(),
  };
  deps.athlete.routes.save(route);
  return ok(route);
}

export function toggleKudos(
  deps: UseCaseDeps,
  args: { activityId: string; memberId: string },
): Result<{ kudoed: boolean; count: number }, AppError> {
  const activity = deps.athlete.activities.byId(args.activityId);
  if (!activity) return err(appError('NOT_FOUND', 'Activity not found.', 404));
  const kudoed = deps.athlete.kudos.toggle(args.activityId, args.memberId, deps.clock.now());
  if (kudoed && activity.memberId !== args.memberId) {
    const giver = deps.members.byId(args.memberId);
    notify(
      deps,
      activity.memberId,
      'ANNOUNCEMENT',
      'Kudos received',
      `${giver?.fullName ?? 'Someone'} gave you kudos on "${activity.title}".`,
    );
  }
  return ok({ kudoed, count: deps.athlete.kudos.forActivity(args.activityId).length });
}

export function addActivityComment(
  deps: UseCaseDeps,
  args: { activityId: string; memberId: string; text: string },
): Result<ActivityComment, AppError> {
  const activity = deps.athlete.activities.byId(args.activityId);
  if (!activity) return err(appError('NOT_FOUND', 'Activity not found.', 404));
  const comment: ActivityComment = {
    id: deps.ids.next('cmt'),
    activityId: args.activityId,
    memberId: args.memberId,
    text: args.text,
    createdAt: deps.clock.now(),
  };
  deps.athlete.comments.add(comment);
  if (activity.memberId !== args.memberId) {
    const author = deps.members.byId(args.memberId);
    notify(
      deps,
      activity.memberId,
      'ANNOUNCEMENT',
      'New comment',
      `${author?.fullName ?? 'Someone'} commented on "${activity.title}".`,
    );
  }
  return ok(comment);
}

export function toggleFollow(
  deps: UseCaseDeps,
  args: { followerId: string; followeeId: string },
): Result<{ following: boolean }, AppError> {
  if (args.followerId === args.followeeId)
    return err(appError('SELF_FOLLOW', 'You cannot follow yourself.'));
  const followee = deps.members.byId(args.followeeId);
  if (!followee) return err(appError('NOT_FOUND', 'Member not found.', 404));
  const following = deps.athlete.follows.toggle(args.followerId, args.followeeId);
  if (following) {
    const follower = deps.members.byId(args.followerId);
    notify(
      deps,
      args.followeeId,
      'ANNOUNCEMENT',
      'New follower',
      `${follower?.fullName ?? 'Someone'} started following you.`,
    );
  }
  return ok({ following });
}

export function joinChallenge(
  deps: UseCaseDeps,
  args: { challengeId: string; memberId: string },
): Result<{ joined: true }, AppError> {
  const challenge = deps.athlete.challenges.byId(args.challengeId);
  if (!challenge) return err(appError('NOT_FOUND', 'Challenge not found.', 404));
  deps.athlete.challenges.join(args.challengeId, args.memberId);
  return ok({ joined: true });
}

export function toggleClubMembership(
  deps: UseCaseDeps,
  args: { clubId: string; memberId: string },
): Result<{ joined: boolean }, AppError> {
  const club = deps.athlete.clubs.byId(args.clubId);
  if (!club) return err(appError('NOT_FOUND', 'Club not found.', 404));
  const idx = club.memberIds.indexOf(args.memberId);
  if (idx >= 0) club.memberIds.splice(idx, 1);
  else club.memberIds.push(args.memberId);
  deps.athlete.clubs.save(club);
  return ok({ joined: idx < 0 });
}

export function upsertGear(
  deps: UseCaseDeps,
  args: {
    memberId: string;
    gearId: string | null;
    name: string;
    kind: 'SHOES' | 'BIKE';
    retired: boolean;
  },
): Result<Gear, AppError> {
  if (args.gearId) {
    const gear = deps.athlete.gear.byId(args.gearId);
    if (!gear || gear.memberId !== args.memberId)
      return err(appError('NOT_FOUND', 'Gear not found.', 404));
    gear.name = args.name;
    gear.kind = args.kind;
    gear.retired = args.retired;
    deps.athlete.gear.save(gear);
    return ok(gear);
  }
  const gear: Gear = {
    id: deps.ids.next('gear'),
    memberId: args.memberId,
    name: args.name,
    kind: args.kind,
    distanceM: 0,
    retired: false,
  };
  deps.athlete.gear.save(gear);
  return ok(gear);
}

export function updateAthleteSettings(
  deps: UseCaseDeps,
  args: { memberId: string; patch: Partial<AthleteSettings> },
): AthleteSettings {
  const current = deps.athlete.settings.get(args.memberId);
  const next = { ...current, ...args.patch };
  deps.athlete.settings.save(args.memberId, next);
  return next;
}

/**
 * Scheduled booking reminders, mock-style: generated lazily whenever the member
 * checks in with the app. Each CONFIRMED booking starting within the next 24h
 * gets exactly one reminder (a real backend would run this on a scheduler).
 */
export function generateBookingReminders(deps: UseCaseDeps, memberId: string): number {
  const settings = deps.athlete.settings.get(memberId);
  if (!settings.bookingReminders) return 0;
  const nowMs = msOf(deps.clock.now());
  let created = 0;
  for (const booking of deps.bookings.forMember(memberId)) {
    if (booking.status !== 'CONFIRMED') continue;
    if (deps.athlete.remindersSent.has(booking.id)) continue;
    const session = deps.sessions.byId(booking.sessionId);
    if (!session) continue;
    const startMs = msOf(session.startsAt);
    if (startMs <= nowMs || startMs - nowMs > 24 * 3600_000) continue;
    const classType = deps.classTypes.byId(session.classTypeId);
    const time = new Date(session.startsAt).toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
    });
    notify(
      deps,
      memberId,
      'BOOKING_REMINDER',
      'Upcoming class',
      `${classType?.name ?? 'Your class'} starts at ${time}. Bring your QR.`,
    );
    deps.athlete.remindersSent.add(booking.id);
    created += 1;
  }
  return created;
}
