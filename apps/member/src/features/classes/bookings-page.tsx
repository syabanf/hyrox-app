import { EmptyState, Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useState } from 'react';
import { Link } from 'react-router';
import { useMyBookings } from '../../lib/queries';

export function BookingsPage() {
  const { data: bookings, isLoading } = useMyBookings();
  const [tab, setTab] = useState<'upcoming' | 'history'>('upcoming');

  if (isLoading) return <Spinner label="Loading bookings…" />;

  const now = Date.now();
  const list = (bookings ?? []).filter((b) =>
    tab === 'upcoming'
      ? new Date(b.session.startsAt).getTime() > now &&
        ['CONFIRMED', 'WAITLIST', 'PENDING'].includes(b.booking.status)
      : new Date(b.session.startsAt).getTime() <= now ||
        ['CANCELLED', 'COMPLETED', 'NO_SHOW', 'CHECKED_IN'].includes(b.booking.status),
  );

  return (
    <div className="flex flex-col gap-5">
      <h1 className="display text-3xl font-black">My bookings</h1>
      <div className="flex rounded-xl bg-surface p-1">
        {(['upcoming', 'history'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`flex-1 rounded-lg py-2 text-sm font-black uppercase tracking-wide ${
              tab === t ? 'bg-brand text-white' : 'text-muted'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      {list.length === 0 ? (
        <EmptyState
          title={tab === 'upcoming' ? 'Nothing booked' : 'No history yet'}
          hint={tab === 'upcoming' ? 'Find your next session on the schedule.' : undefined}
          action={
            tab === 'upcoming' ? (
              <Link to="/classes" className="btn-brand mt-2 !py-2 text-sm">
                Browse classes
              </Link>
            ) : undefined
          }
        />
      ) : (
        <div className="flex flex-col gap-2">
          {list.map((b) => (
            <Link key={b.booking.id} to={`/classes/${b.session.id}`} className="card flex flex-col gap-2">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate font-black">{b.classTypeName}</p>
                  <p className="truncate text-sm text-muted">
                    {formatDayTime(b.session.startsAt)} · {b.branchName}
                  </p>
                </div>
                <StatusBadge status={b.booking.status} />
              </div>
              {b.booking.status === 'WAITLIST' && b.booking.promotionOfferedAt ? (
                <p className="rounded-lg bg-brand/10 px-3 py-1.5 text-xs font-black text-brand">
                  A spot opened up — tap to confirm it.
                </p>
              ) : null}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
