'use client';

import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import { api, ApiError } from '../../../../../lib/api';
import { usePermissions } from '../../../../../lib/auth';
import { ErrorNote, PageTitle, StatCard } from '../../../../../components/ui';

export default function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const { data: v, isLoading } = useQuery({
    queryKey: ['admin-session', id],
    queryFn: () => api.admin.sessions.get(id),
  });

  const onError = (e: unknown) => setError(e instanceof ApiError ? e.message : 'Action failed.');
  const refresh = () => {
    setError(null);
    void qc.invalidateQueries();
  };

  const action = useMutation({
    mutationFn: (a: 'publish' | 'cancel' | 'complete') => api.admin.sessions.action(id, a),
    onSuccess: refresh,
    onError,
  });
  const noShow = useMutation({
    mutationFn: (bookingId: string) => api.admin.bookings.noShow(bookingId),
    onSuccess: refresh,
    onError,
  });
  const checkIn = useMutation({
    mutationFn: (bookingId: string) => api.admin.bookings.checkIn(bookingId),
    onSuccess: refresh,
    onError,
  });
  const cancelBooking = useMutation({
    mutationFn: (bookingId: string) => api.bookings.cancel(bookingId),
    onSuccess: (res) => {
      refresh();
      setNotice(
        res.promotedMemberName
          ? `Cancelled — ${res.promotedMemberName} was auto-promoted from the waitlist.`
          : 'Booking cancelled.',
      );
    },
    onError,
  });

  if (isLoading || !v) return <Spinner label="Loading session…" />;

  const canManage = can('sessions.manage');
  const canAttend = can('attendance.manage');

  return (
    <div>
      <PageTitle
        title={v.classTypeName}
        subtitle={`${formatDayTime(v.session.startsAt)} · ${v.branchName} · ${v.coachName}`}
        actions={
          canManage ? (
            <>
              {v.session.status === 'DRAFT' ? (
                <button className="a-btn" onClick={() => action.mutate('publish')}>
                  Publish
                </button>
              ) : null}
              {['PUBLISHED', 'FULL'].includes(v.session.status) ? (
                <>
                  <button className="a-btn-ghost" onClick={() => action.mutate('complete')}>
                    Complete session
                  </button>
                  <button className="a-btn-danger" onClick={() => action.mutate('cancel')}>
                    Cancel session
                  </button>
                </>
              ) : null}
            </>
          ) : undefined
        }
      />
      <div className="mb-2 flex items-center gap-2">
        <StatusBadge status={v.session.status} />
        <span className="text-sm text-muted">
          {v.session.creditCost} credit{v.session.creditCost === 1 ? '' : 's'} per entry
        </span>
      </div>
      <ErrorNote message={error} />
      {notice ? (
        <p className="rounded-lg bg-ok/10 px-3 py-2 text-sm font-bold text-ok">{notice}</p>
      ) : null}

      <div className="my-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Confirmed" value={`${v.confirmedCount}/${v.session.capacity}`} tone="brand" />
        <StatCard label="Waitlist" value={v.waitlistCount} />
        <StatCard label="Spots left" value={v.spotsLeft} />
      </div>

      <div className="a-card !p-0">
        <p className="a-label px-4 pt-4">Roster & attendance</p>
        <table className="a-table">
          <thead>
            <tr>
              <th>Member</th>
              <th>Status</th>
              <th>Waitlist</th>
              <th className="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {v.roster.map((r) => (
              <tr key={r.booking.id}>
                <td className="font-bold">{r.memberName}</td>
                <td>
                  <StatusBadge status={r.booking.status} />
                </td>
                <td className="text-muted">
                  {r.booking.waitlistPosition ? `#${r.booking.waitlistPosition}` : '—'}
                </td>
                <td className="text-right">
                  <div className="flex justify-end gap-2">
                    {canAttend && r.booking.status === 'CONFIRMED' ? (
                      <>
                        <button className="a-btn !px-2.5 !py-1 text-xs" onClick={() => checkIn.mutate(r.booking.id)}>
                          Check in
                        </button>
                        <button
                          className="a-btn-ghost !px-2.5 !py-1 text-xs"
                          onClick={() => noShow.mutate(r.booking.id)}
                        >
                          No-show
                        </button>
                      </>
                    ) : null}
                    {can('bookings.manage') && ['CONFIRMED', 'WAITLIST'].includes(r.booking.status) ? (
                      <button
                        className="a-btn-danger !px-2.5 !py-1 text-xs"
                        onClick={() => cancelBooking.mutate(r.booking.id)}
                      >
                        Cancel
                      </button>
                    ) : null}
                  </div>
                </td>
              </tr>
            ))}
            {v.roster.length === 0 ? (
              <tr>
                <td colSpan={4} className="py-6 text-center text-muted">
                  Nobody booked yet.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
