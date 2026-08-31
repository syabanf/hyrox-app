'use client';

import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle, Pager, SearchSelect, StatCard } from '../../../../components/ui';

export default function BookingsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [bookOpen, setBookOpen] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');
  const [memberQuery, setMemberQuery] = useState('');
  const [page, setPage] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  // All bookings assembled from sessions' rosters via member detail queries is
  // heavy; instead reuse admin sessions + rosters for upcoming days.
  const { data: sessions, isLoading } = useQuery({
    queryKey: ['admin-sessions', 'all'],
    queryFn: () => api.admin.sessions.list(),
  });
  const upcomingIds = useMemo(
    () =>
      (sessions ?? [])
        .filter((v) => new Date(v.session.endsAt).getTime() > Date.now() - 24 * 3600_000)
        .map((v) => v.session.id)
        .slice(0, 30),
    [sessions],
  );
  const rosterQueries = useQuery({
    queryKey: ['admin-rosters', upcomingIds],
    enabled: upcomingIds.length > 0,
    queryFn: async () => {
      const details = await Promise.all(upcomingIds.map((id) => api.admin.sessions.get(id)));
      return details.flatMap((d) =>
        d.roster.map((r) => ({
          ...r,
          session: d.session,
          classTypeName: d.classTypeName,
          branchName: d.branchName,
        })),
      );
    },
  });

  const cancelMutation = useMutation({
    mutationFn: (bookingId: string) => api.bookings.cancel(bookingId),
    onSuccess: (res) => {
      setError(null);
      setNotice(
        res.promotedMemberName
          ? `Cancelled — ${res.promotedMemberName} auto-promoted from the waitlist.`
          : 'Booking cancelled.',
      );
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Cancel failed.'),
  });

  const all = rosterQueries.data ?? [];
  const rows = all
    .filter((r) => !statusFilter || r.booking.status === statusFilter)
    .filter((r) => !memberQuery || r.memberName.toLowerCase().includes(memberQuery.toLowerCase()))
    .sort((a, b) => new Date(a.session.startsAt).getTime() - new Date(b.session.startsAt).getTime());
  const pageCount = Math.max(1, Math.ceil(rows.length / 10));
  const safePage = Math.min(page, pageCount - 1);
  const paged = rows.slice(safePage * 10, safePage * 10 + 10);

  return (
    <div>
      <PageTitle
        title="Bookings"
        subtitle="Recent & upcoming bookings across sessions"
        actions={
          can('bookings.manage') ? (
            <button className="a-btn" onClick={() => setBookOpen(true)}>
              + Book for member
            </button>
          ) : undefined
        }
      />
      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Confirmed" value={all.filter((r) => r.booking.status === 'CONFIRMED').length} />
        <StatCard label="Waitlist" value={all.filter((r) => r.booking.status === 'WAITLIST').length} />
        <StatCard label="Checked in" value={all.filter((r) => r.booking.status === 'CHECKED_IN').length} />
        <StatCard
          label="Cancelled / no-show"
          value={all.filter((r) => ['CANCELLED', 'NO_SHOW'].includes(r.booking.status)).length}
        />
      </div>
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search member…"
          value={memberQuery}
          onChange={(e) => {
            setMemberQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-44">
          <SearchSelect
            value={statusFilter}
            onChange={(v) => {
              setStatusFilter(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All statuses"
            placeholder="Search status…"
            options={['CONFIRMED', 'WAITLIST', 'CHECKED_IN', 'COMPLETED', 'CANCELLED', 'NO_SHOW'].map((s) => ({ value: s, label: s }))}
          />
        </div>
      </div>
      <ErrorNote message={error} />
      {notice ? <p className="mb-3 rounded-lg bg-ok/10 px-3 py-2 text-sm font-bold text-ok">{notice}</p> : null}
      {isLoading || rosterQueries.isLoading ? (
        <Spinner label="Loading bookings…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Member</th>
                <th>Class</th>
                <th>When</th>
                <th>Branch</th>
                <th>Status</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {paged.map((r) => (
                <tr key={r.booking.id}>
                  <td className="font-bold">{r.memberName}</td>
                  <td>{r.classTypeName}</td>
                  <td className="text-muted">{formatDayTime(r.session.startsAt)}</td>
                  <td>{r.branchName}</td>
                  <td>
                    <StatusBadge status={r.booking.status} />
                    {r.booking.waitlistPosition ? (
                      <span className="ml-1 text-xs text-warn">#{r.booking.waitlistPosition}</span>
                    ) : null}
                  </td>
                  <td className="text-right">
                    {can('bookings.manage') && ['CONFIRMED', 'WAITLIST'].includes(r.booking.status) ? (
                      <button
                        className="a-btn-danger !px-2.5 !py-1 text-xs"
                        onClick={() => cancelMutation.mutate(r.booking.id)}
                      >
                        Cancel
                      </button>
                    ) : null}
                  </td>
                </tr>
              ))}
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-6 text-center text-muted">
                    No bookings match.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
          <Pager page={safePage} pageCount={pageCount} onPage={setPage} />
        </div>
      )}
      {bookOpen ? (
        <BookModal
          onClose={() => setBookOpen(false)}
          onDone={(msg) => {
            setBookOpen(false);
            setNotice(msg);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}

function BookModal({ onClose, onDone }: { onClose: () => void; onDone: (msg: string) => void }) {
  const { data: members } = useQuery({ queryKey: ['members', '', ''], queryFn: () => api.admin.members.list() });
  const { data: sessions } = useQuery({ queryKey: ['admin-sessions', 'all'], queryFn: () => api.admin.sessions.list() });
  const [memberId, setMemberId] = useState('');
  const [sessionId, setSessionId] = useState('');
  const [error, setError] = useState<string | null>(null);

  const bookable = (sessions ?? []).filter(
    (v) =>
      ['PUBLISHED', 'FULL'].includes(v.session.status) &&
      new Date(v.session.startsAt).getTime() > Date.now(),
  );

  const mutation = useMutation({
    mutationFn: () => api.admin.bookings.book({ memberId, sessionId }),
    onSuccess: (res) =>
      onDone(
        res.decision === 'CONFIRMED'
          ? 'Booking confirmed.'
          : `Session full — member waitlisted at #${res.booking.waitlistPosition}.`,
      ),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Booking failed.'),
  });

  return (
    <Modal title="Book for a member" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Member</label>
          <SearchSelect
            value={memberId}
            onChange={setMemberId}
            placeholder="Search member…"
            options={(members ?? [])
              .filter((m) => m.member.status === 'ACTIVE')
              .map((m) => ({
                value: m.member.id,
                label: m.member.fullName,
                hint: `${m.member.email} · ${m.balance} cr`,
              }))}
          />
        </div>
        <div>
          <label className="a-label">Session</label>
          <SearchSelect
            value={sessionId}
            onChange={setSessionId}
            placeholder="Search session…"
            options={bookable.map((v) => ({
              value: v.session.id,
              label: `${formatDayTime(v.session.startsAt)} · ${v.classTypeName}`,
              hint: `${v.branchName} · ${v.spotsLeft > 0 ? `${v.spotsLeft} left` : 'FULL → waitlist'}`,
            }))}
          />
        </div>
        <ErrorNote message={error} />
        <button className="a-btn" disabled={mutation.isPending || !memberId || !sessionId} onClick={() => mutation.mutate()}>
          Book
        </button>
      </div>
    </Modal>
  );
}
