import type { Result } from './result';
import { err, ok } from './result';

/** Declarative state machine: for each state, the list of states it may move to. */
export type TransitionMap<S extends string> = Readonly<Record<S, readonly S[]>>;

export interface InvalidTransition<S extends string> {
  type: 'INVALID_TRANSITION';
  from: S;
  to: S;
}

export function canTransition<S extends string>(map: TransitionMap<S>, from: S, to: S): boolean {
  return map[from].includes(to);
}

export function transition<S extends string>(
  map: TransitionMap<S>,
  from: S,
  to: S,
): Result<S, InvalidTransition<S>> {
  return canTransition(map, from, to) ? ok(to) : err({ type: 'INVALID_TRANSITION', from, to });
}
