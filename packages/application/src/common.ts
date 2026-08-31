import type { BusinessRules, IsoDate, MemberNotificationType } from '@hyrox/domain';
import { computeBalance, resolveRules } from '@hyrox/domain';
import type { UseCaseDeps } from './ports';

export interface AppError {
  code: string;
  message: string;
  status: number;
}

export const appError = (code: string, message: string, status = 422): AppError => ({
  code,
  message,
  status,
});

export interface Actor {
  id: string;
  name: string;
}

export const SYSTEM_ACTOR: Actor = { id: 'system', name: 'System' };

export function balanceOf(deps: UseCaseDeps, memberId: string): number {
  return computeBalance(deps.ledger.forMember(memberId));
}

export function rulesFor(deps: UseCaseDeps, branchId?: string | null): BusinessRules {
  return resolveRules(deps.rules.defaults(), branchId ? deps.rules.overrideFor(branchId) : null);
}

export function notify(
  deps: UseCaseDeps,
  memberId: string,
  type: MemberNotificationType,
  title: string,
  body: string,
): void {
  deps.notifications.append({
    id: deps.ids.next('ntf'),
    memberId,
    type,
    title,
    body,
    createdAt: deps.clock.now(),
    readAt: null,
  });
}

export function recordAudit(
  deps: UseCaseDeps,
  args: {
    entityType: string;
    entityId: string;
    action: string;
    previousValue?: string | null;
    newValue?: string | null;
    actor: Actor;
    reason?: string | null;
  },
): void {
  deps.audit.append({
    id: deps.ids.next('aud'),
    entityType: args.entityType,
    entityId: args.entityId,
    action: args.action,
    previousValue: args.previousValue ?? null,
    newValue: args.newValue ?? null,
    actorId: args.actor.id,
    actorName: args.actor.name,
    reason: args.reason ?? null,
    createdAt: deps.clock.now(),
  });
}

export function maybeNotifyLowBalance(
  deps: UseCaseDeps,
  memberId: string,
  balanceBefore: number,
  balanceAfter: number,
): void {
  const threshold = deps.rules.defaults().lowBalanceThreshold;
  if (balanceBefore >= threshold && balanceAfter < threshold) {
    notify(
      deps,
      memberId,
      'LOW_BALANCE',
      'Low credit balance',
      `You have ${balanceAfter} credit${balanceAfter === 1 ? '' : 's'} left. Top up to keep training.`,
    );
  }
}

export function touch<T extends { updatedAt: IsoDate }>(deps: UseCaseDeps, entity: T): T {
  entity.updatedAt = deps.clock.now();
  return entity;
}
