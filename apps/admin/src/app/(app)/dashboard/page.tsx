'use client';

import type { DailyPointView } from '@hyrox/contracts';
import { Spinner, StatusBadge, formatDay, formatDayTime, formatIdr, formatTime } from '@hyrox/ui';
import { AlertTriangle, ChevronRight, CreditCard, Hourglass } from 'lucide-react';
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
  const { data: sales } = useQuery({ queryKey: ['dash-sales'], queryFn: () => api.admin.reports.sales(14) });
  const { data: visits } = useQuery({ queryKey: ['dash-visits'], queryFn: () => api.admin.reports.visits(14) });
  const { data: latestLogs } = useQuery({
    queryKey: ['dash-logs'],
    queryFn: () => api.admin.accessLogs.list({ limit: 5 }),
  });
  const { data: conflicts } = useQuery({
    queryKey: ['dash-conflicts'],
    queryFn: () => api.admin.accessLogs.list({ result: 'CONFLICT', limit: 50 }),
  });
  const { data: allPayments } = useQuery({ queryKey: ['dash-payments'], queryFn: api.admin.payments.list });
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

      {/* Trends — 14 days */}
      <div className="mt-4 grid gap-3 lg:grid-cols-2">
        <div className="a-card">
          <div className="mb-3 flex items-baseline justify-between">
            <p className="text-[11px] font-bold uppercase tracking-wider text-muted">Revenue · 14 days</p>
            <p className="display text-xl">{formatIdr(sales?.totalIdr ?? 0)}</p>
          </div>
          <MiniBars points={sales?.byDay ?? []} format={(v) => formatIdr(v)} />
        </div>
        <div className="a-card">
          <div className="mb-3 flex items-baseline justify-between">
            <p className="text-[11px] font-bold uppercase tracking-wider text-muted">Visits · 14 days</p>
            <p className="display text-xl">{visits?.total ?? 0}</p>
          </div>
          <MiniBars points={visits?.byDay ?? []} format={(v) => `${v} visit${v === 1 ? '' : 's'}`} />
        </div>
      </div>

      {/* Backoffice widgets */}
      <div className="mt-4 grid gap-3 lg:grid-cols-3">
        <div className="a-card">
          <p className="a-label">Needs attention</p>
          <div className="flex flex-col gap-1">
            {[
              {
                href: '/commercial/payments',
                icon: CreditCard,
                label: 'Pending payments',
                count: (allPayments ?? []).filter((x) => x.payment.status === 'PENDING').length,
                tone: 'warn',
              },
              {
                href: '/access/logs',
                icon: AlertTriangle,
                label: 'Offline conflicts',
                count: (conflicts ?? []).length,
                tone: 'danger',
              },
              {
                href: '/reports',
                icon: Hourglass,
                label: 'Credits expiring soon',
                count: data.expiringCredits,
                tone: 'warn',
              },
            ].map(({ href, icon: Icon, label, count, tone }) => (
              <Link
                key={label}
                href={href}
                className="flex items-center gap-3 rounded-xl px-2 py-2 transition hover:bg-surface-raised"
              >
                <span
                  className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${
                    count === 0
                      ? 'bg-surface-raised text-muted'
                      : tone === 'danger'
                        ? 'bg-danger/10 text-danger'
                        : 'bg-warn/10 text-warn'
                  }`}
                >
                  <Icon size={15} />
                </span>
                <span className="min-w-0 flex-1 text-sm font-bold">{label}</span>
                <span className={`display text-lg ${count > 0 ? '' : 'text-muted'}`}>{count}</span>
                <ChevronRight size={14} className="shrink-0 text-muted" />
              </Link>
            ))}
          </div>
        </div>

        <div className="a-card">
          <p className="a-label">Today's occupancy</p>
          <div className="flex flex-col gap-3">
            {(branches ?? []).map((b) => {
              const todays = sessions.filter((v) => v.session.branchId === b.id);
              const booked = todays.reduce((sum, v) => sum + v.confirmedCount, 0);
              const cap = todays.reduce((sum, v) => sum + v.session.capacity, 0);
              const pct = cap > 0 ? Math.round((booked / cap) * 100) : 0;
              return (
                <div key={b.id}>
                  <div className="flex items-baseline justify-between text-sm">
                    <span className="font-bold">{b.name}</span>
                    <span className="text-muted">
                      {booked}/{cap} · {pct}%
                    </span>
                  </div>
                  <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-surface-raised">
                    <div className="h-full rounded-full bg-[#c4161c]" style={{ width: `${pct}%` }} />
                  </div>
                </div>
              );
            })}
            {(branches ?? []).length === 0 ? <p className="text-sm text-muted">No branches.</p> : null}
          </div>
        </div>

        <div className="a-card">
          <p className="a-label">Latest check-ins</p>
          <div className="flex flex-col divide-y divide-line/60">
            {(latestLogs ?? []).slice(0, 5).map((v) => (
              <div key={v.log.id} className="flex items-center gap-3 py-2 text-sm">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-bold">{v.memberName ?? '—'}</p>
                  <p className="truncate text-xs text-muted">
                    {v.gateName} · {formatDayTime(v.log.createdAt)}
                  </p>
                </div>
                <StatusBadge status={v.log.result} />
              </div>
            ))}
            {(latestLogs ?? []).length === 0 ? <p className="py-2 text-sm text-muted">No activity yet.</p> : null}
          </div>
        </div>
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


/** Single-series daily bars: brand hue, rounded data ends, native tooltips. */
function MiniBars({ points, format }: { points: DailyPointView[]; format: (v: number) => string }) {
  const max = Math.max(1, ...points.map((p) => p.value));
  return (
    <div>
      <div className="flex h-28 items-end gap-[3px]">
        {points.map((p) => (
          <div
            key={p.date}
            title={`${formatDay(p.date)} · ${format(p.value)}`}
            className="flex h-full flex-1 items-end"
          >
            <div
              className="w-full rounded-t-[4px] transition-opacity hover:opacity-75"
              style={{
                height: `${Math.max(p.value > 0 ? 5 : 2, (p.value / max) * 100)}%`,
                background: p.value > 0 ? '#c4161c' : 'var(--color-line)',
              }}
            />
          </div>
        ))}
      </div>
      {points.length > 1 ? (
        <div className="mt-1.5 flex justify-between text-[10px] font-bold text-muted">
          <span>{formatDay(points[0]!.date)}</span>
          <span>{formatDay(points[points.length - 1]!.date)}</span>
        </div>
      ) : null}
    </div>
  );
}
