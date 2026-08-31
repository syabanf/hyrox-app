import type { IsoDate } from './shared/time';

/** Critical operational actions leave an immutable audit trail. */
export interface AuditEvent {
  id: string;
  entityType: string;
  entityId: string;
  action: string;
  previousValue: string | null;
  newValue: string | null;
  actorId: string;
  actorName: string;
  reason: string | null;
  createdAt: IsoDate;
}
