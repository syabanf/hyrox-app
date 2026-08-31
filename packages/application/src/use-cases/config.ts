import type { BusinessRules, Result } from '@hyrox/domain';
import { ok } from '@hyrox/domain';
import type { Actor, AppError } from '../common';
import { recordAudit } from '../common';
import type { UseCaseDeps } from '../ports';

export function updateRules(
  deps: UseCaseDeps,
  args: { patch: Partial<BusinessRules>; actor: Actor },
): Result<BusinessRules, AppError> {
  const previous = deps.rules.defaults();
  const next = { ...previous, ...args.patch };
  deps.rules.saveDefaults(next);
  recordAudit(deps, {
    entityType: 'BUSINESS_RULES',
    entityId: 'defaults',
    action: 'UPDATE',
    previousValue: JSON.stringify(previous),
    newValue: JSON.stringify(next),
    actor: args.actor,
  });
  return ok(next);
}
