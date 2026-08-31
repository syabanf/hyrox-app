'use client';

import { Spinner, StatusBadge, formatIdr, formatTime } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { useMemo, useState } from 'react';
import { api } from '../../../lib/api';
import { PageTitle, Pager, SearchSelect, StatCard } from '../../../components/ui';

const PAGE_SIZE = 8;

export default function DashboardPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard'],
    queryFn: api.admin.reports.dashboard,
    refetchInterval: 10_000,
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const [branchFilter, setBranchFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [page, setPage] = useState(0);

  const sessions = data?.todaySessions ?? [];
  const filtered = useMemo(
    () =>
      sessions.filter(
        (v) =>
          (!branchFilter || v.session.branchId === branchFilter) &&
          (!statusFilter || v.session.status === statusFilter),
      ),
    [sessions, branchFilter, statusFilter],
  );
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const paged = filtered.slice(safePage * PAGE_SIZE, safePage * PAGE_SIZE + PAGE_SIZE);
  const statusOptions = [...new Set(sessions.map((v) => v.session.status))].map((st) => ({
    value: st,
    label: st,
  }));

  if (isLoading || !data) return <Spinner label="Loading dashboard…" />;

  return (
    <div>
      <PageTitle title="Dashboard" subtitle="Today at a glance" />
      {/* Hero — the day's headline numbers on a premium black card */}
      <div className="surface-ink relative mb-4 overflow-hidden rounded-3xl p-6 text-white shadow-[0_18px_40px_rgb(13_13_16/0.25)]">
        <div className="pointer-events-none absolute -right-20 -top-28 h-64 w-64 rounded-full bg-brand/25 blur-3xl" />
        <div className="relative grid grid-cols-2 gap-6 sm:grid-cols-3">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
              Visitors today
            </p>
            <p className="display mt-1 text-4xl leading-none sm:text-5xl">{data.visitorsToday}</p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
              Revenue today
            </p>
            <p className="display mt-1 text-4xl leading-none sm:text-5xl">
              {formatIdr(data.revenueTodayIdr)}
            </p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
              Classes today
            </p>
            <p className="display mt-1 text-4xl leading-none sm:text-5xl">{data.classesToday}</p>
          </div>
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Active members" value={data.activeMembers} />
        <StatCard label="Top-ups today" value={formatIdr(data.topUpsTodayIdr)} />
        <StatCard label="Outstanding credits" value={data.outstandingCredits} hint="Total liability" />
        <StatCard
          label="Expiring credits"
          value={data.expiringCredits}
          tone={data.expiringCredits > 0 ? 'danger' : undefined}
          hint="Within reminder window"
        />
      </div>

      <div className="a-card mt-6 !p-0">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
          <h2 className="display text-lg font-black">Today's sessions</h2>
          <div className="flex flex-wrap items-center gap-2">
            <div className="w-44">
              <SearchSelect
                value={branchFilter}
                onChange={(v) => {
                  setBranchFilter(v);
                  setPage(0);
                }}
                options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
                allowEmpty
                emptyLabel="All branches"
                placeholder="Search branch…"
              />
            </div>
            <div className="w-40">
              <SearchSelect
                value={statusFilter}
                onChange={(v) => {
                  setStatusFilter(v);
                  setPage(0);
                }}
                options={statusOptions}
                allowEmpty
                emptyLabel="All statuses"
                placeholder="Search status…"
              />
            </div>
            <Link href="/operations/sessions" className="text-sm font-bold text-brand">
              All sessions →
            </Link>
          </div>
        </div>
        <table className="a-table mt-2">
          <thead>
            <tr>
              <th>Time</th>
              <th>Class</th>
              <th>Branch</th>
              <th>Coach</th>
              <th>Booked</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {paged.map((v) => (
              <tr key={v.session.id}>
                <td className="font-bold">{formatTime(v.session.startsAt)}</td>
                <td>
                  <Link href={`/operations/sessions/${v.session.id}`} className="font-bold hover:text-brand">
                    {v.classTypeName}
                  </Link>
                </td>
                <td>{v.branchName}</td>
                <td>{v.coachName}</td>
                <td>
                  {v.confirmedCount}/{v.session.capacity}
                  {v.waitlistCount > 0 ? <span className="text-warn"> +{v.waitlistCount} WL</span> : null}
                </td>
                <td>
                  <StatusBadge status={v.session.status} />
                </td>
              </tr>
            ))}
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-6 text-center text-muted">
                  {sessions.length === 0 ? 'No sessions today.' : 'Nothing matches these filters.'}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
        <Pager page={safePage} pageCount={pageCount} onPage={setPage} />
      </div>
    </div>
  );
}
