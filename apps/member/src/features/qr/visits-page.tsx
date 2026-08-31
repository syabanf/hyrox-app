import { EmptyState, Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMyVisits } from '../../lib/queries';

export function VisitsPage() {
  const { data: visits, isLoading } = useMyVisits();
  if (isLoading) return <Spinner label="Loading visits…" />;

  return (
    <div className="flex flex-col gap-5">
      <h1 className="display text-3xl font-black">Visit history</h1>
      {!visits || visits.length === 0 ? (
        <EmptyState title="No visits yet" hint="Your gate check-ins will appear here." />
      ) : (
        <div className="flex flex-col gap-2">
          {visits.map((v) => (
            <div key={v.log.id} className="card flex items-center justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate font-bold">{v.gateName}</p>
                <p className="text-sm text-muted">{formatDayTime(v.log.createdAt)}</p>
                {v.log.reasonCode ? (
                  <p className="text-xs font-bold text-danger">
                    {v.log.reasonCode.replaceAll('_', ' ')}
                  </p>
                ) : null}
              </div>
              <div className="flex flex-col items-end gap-1">
                <StatusBadge status={v.log.result} />
                <span className="text-xs font-black text-muted">
                  {v.log.creditDelta !== 0 ? `${v.log.creditDelta} cr` : '—'}
                  {v.log.mode === 'OFFLINE' ? ' · offline' : ''}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
