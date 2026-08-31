import type { Division, Result, UserRace, UserRaceStatus } from '@hyrox/domain';
import { USER_RACE_TRANSITIONS, canTransition, err, ok } from '@hyrox/domain';
import type { AppError } from '../common';
import { appError } from '../common';
import type { UseCaseDeps } from '../ports';

export function registerForRace(
  deps: UseCaseDeps,
  args: { memberId: string; raceEventId: string; division: Division; goalSec: number | null },
): Result<UserRace, AppError> {
  const event = deps.races.events.byId(args.raceEventId);
  if (!event) return err(appError('NOT_FOUND', 'Race not found.', 404));
  if (event.status === 'SOLD_OUT' || event.status === 'CANCELLED' || event.status === 'COMPLETED')
    return err(appError('NOT_OPEN', `This race is ${event.status.replaceAll('_', ' ').toLowerCase()}.`));
  const existing = deps.races.userRaces
    .forMember(args.memberId)
    .find((r) => r.raceEventId === args.raceEventId && r.status !== 'CANCELLED');
  if (existing) return err(appError('ALREADY_JOINED', 'This race is already on your list.', 409));
  const userRace: UserRace = {
    id: deps.ids.next('urc'),
    memberId: args.memberId,
    raceEventId: args.raceEventId,
    division: args.division,
    goalSec: args.goalSec,
    status: 'TRAINING',
    resultSec: null,
    createdAt: deps.clock.now(),
  };
  deps.races.userRaces.save(userRace);
  return ok(userRace);
}

export function updateUserRace(
  deps: UseCaseDeps,
  args: {
    userRaceId: string;
    memberId: string;
    patch: { goalSec?: number | null; status?: UserRaceStatus; resultSec?: number | null };
  },
): Result<UserRace, AppError> {
  const userRace = deps.races.userRaces.byId(args.userRaceId);
  if (!userRace || userRace.memberId !== args.memberId)
    return err(appError('NOT_FOUND', 'Race entry not found.', 404));
  if (args.patch.status && args.patch.status !== userRace.status) {
    if (!canTransition(USER_RACE_TRANSITIONS, userRace.status, args.patch.status))
      return err(
        appError('INVALID_TRANSITION', `Cannot move from ${userRace.status} to ${args.patch.status}.`),
      );
    userRace.status = args.patch.status;
  }
  if (args.patch.goalSec !== undefined) userRace.goalSec = args.patch.goalSec;
  if (args.patch.resultSec !== undefined) {
    userRace.resultSec = args.patch.resultSec;
    if (args.patch.resultSec !== null && userRace.status === 'TRAINING') userRace.status = 'RACED';
  }
  deps.races.userRaces.save(userRace);
  return ok(userRace);
}
