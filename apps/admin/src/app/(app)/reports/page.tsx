'use client';

import { Spinner, formatDay, formatIdr } from '@nuhabit/ui';
import { useQuery } from '@tanstack/react-query';
import { ChevronRight } from 'lucide-react';
import Link from 'next/link';
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { api } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';
import type { Permission } from '@nuhabit/domain';
import { PageTitle, StatCard, StatRow } from '../../../components/ui';
import { FilterBar, RangePicker, drillTo, rangeLabel, useFilters } from '../../../components/filters';

const TABS = ['Overview', 'Sales', 'Visits', 'Classes', 'Credits'] as const;
type Tab = (typeof TABS)[number];

const chartStyle = {
  grid: 'rgb(0 0 0 / 0.08)',
  axis: '#5f6b62',
  bar: '#00281a',
  tooltip: { background: '#ffffff', border: '1px solid #e3dbcc', borderRadius: 8, color: '#131a1c' },
};

/**
 * Reports read top to bottom.
 *
 * Overview answers "how is the studio doing", each tab answers "why", each row
 * inside a tab is a link to the list of records behind that number, and each
 * record links to the member or session it belongs to. Nothing here is a
 * dead-end figure: if a number looks wrong, you can click until you are
 * looking at the rows that made it.
 *
 * The window and the tab live in the URL, so a report is a link — "PIK
 * refunds over the last quarter" stops being a sequence of clicks somebody
 * has to describe over the phone.
 */
const DEFAULTS = { tab: 'Overview', days: '30' };

export default function ReportsPage() {
  const { can } = usePermissions();
  const { filters, set, clear, dirty } = useFilters(DEFAULTS);
  const days = Number(filters.days) || 30;
  const tabs = TABS.filter((t) => t !== 'Sales' || can('reports.view'));
  const tab = (tabs.includes(filters.tab as Tab) ? filters.tab : tabs[0]) as Tab;

  return (
    <div>
      <PageTitle
        title="Reports"
        subtitle="Every figure drills down to the records behind it"
      />
      <div className="mb-4 flex flex-wrap gap-1 rounded-xl bg-surface p-1">
        {tabs.map((t) => (
          <button
            key={t}
            onClick={() => set('tab', t)}
            className={`rounded-lg px-4 py-1.5 text-sm font-bold transition ${
              tab === t ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      <FilterBar
        dirty={dirty}
        onClear={clear}
        chips={[
          ...(filters.days !== DEFAULTS.days
            ? [{ key: 'days', label: rangeLabel(filters.days), onRemove: () => set('days', '') }]
            : []),
        ]}
      >
        <RangePicker value={filters.days} onChange={(d) => set('days', d)} />
        {/* Credits is a snapshot of the ledger as it stands, not a window
            over it, so the range control would be lying on that tab. */}
        {tab === 'Credits' ? (
          <span className="text-xs text-muted">Credits are a live snapshot — the window does not apply.</span>
        ) : null}
      </FilterBar>

      {tab === 'Overview' ? <Overview days={days} can={can} /> : null}
      {tab === 'Sales' ? <SalesReport days={days} /> : null}
      {tab === 'Visits' ? <VisitsReport days={days} /> : null}
      {tab === 'Classes' ? <ClassesReport days={days} /> : null}
      {tab === 'Credits' ? <CreditsReport /> : null}
    </div>
  );
}

/** One line of the overview: a figure, and where it came from. */
function DrillStat({
  label,
  value,
  hint,
  href,
}: {
  label: string;
  value: string | number;
  hint?: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      className="group flex items-center justify-between gap-4 border-b border-line px-4 py-3 last:border-0 transition hover:bg-surface-raised"
    >
      <div className="min-w-0">
        <p className="text-sm font-bold">{label}</p>
        {hint ? <p className="text-xs text-muted">{hint}</p> : null}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <span className="display text-xl font-black tabular-nums">{value}</span>
        <ChevronRight size={16} className="text-muted transition group-hover:translate-x-0.5 group-hover:text-brand" />
      </div>
    </Link>
  );
}

function Section({ title, subtitle, children }: { title: string; subtitle?: string; children: React.ReactNode }) {
  return (
    <div className="a-card !p-0">
      <div className="px-4 pb-1 pt-4">
        <p className="a-label">{title}</p>
        {subtitle ? <p className="text-xs text-muted">{subtitle}</p> : null}
      </div>
      {children}
    </div>
  );
}

/**
 * The top of the funnel: one number per part of the business, each a door
 * into the report that explains it.
 */
function Overview({ days, can }: { days: number; can: (p: Permission) => boolean }) {
  const sales = useQuery({
    queryKey: ['report-sales', days],
    queryFn: () => api.admin.reports.sales(days),
    enabled: can('reports.view'),
  });
  const visits = useQuery({ queryKey: ['report-visits', days], queryFn: () => api.admin.reports.visits(days) });
  const classes = useQuery({ queryKey: ['report-classes', days], queryFn: api.admin.reports.classes });
  const credits = useQuery({ queryKey: ['report-credits'], queryFn: api.admin.reports.credits });

  const attended = (classes.data?.perType ?? []).reduce((s, r) => s + r.attended, 0);
  const noShows = (classes.data?.perType ?? []).reduce((s, r) => s + r.noShows, 0);
  const sessions = (classes.data?.perType ?? []).reduce((s, r) => s + r.sessionsHeld, 0);

  return (
    <div className="flex flex-col gap-4">
      <StatRow>
        <StatCard
          tone="ink"
          label={`Revenue · ${days}d`}
          value={sales.data ? formatIdr(sales.data.totalIdr) : '…'}
          hint="Settled top-ups"
        />
        <StatCard tone="brand" label={`Visits · ${days}d`} value={visits.data?.total ?? '…'} hint="Doors opened" />
        <StatCard
          tone="ok"
          label="Classes held"
          value={sessions || '…'}
          hint={`${attended} attended`}
        />
        <StatCard
          tone="warn"
          label="No-shows"
          value={noShows}
          hint="Booked and did not arrive"
        />
      </StatRow>

      <div className="grid gap-4 lg:grid-cols-2">
        <Section title="Money" subtitle="Each line opens the records behind it">
          {sales.data ? (
            <>
              <DrillStat
                label="Revenue collected"
                hint={`Last ${days} days`}
                value={formatIdr(sales.data.totalIdr)}
                href={drillTo('/commercial/payments', { status: 'PAID' })}
              />
              <DrillStat
                label="Refunded"
                hint="Money that came in and went back out"
                value={formatIdr(sales.data.refundsIdr)}
                href={drillTo('/commercial/payments', { status: 'REFUNDED' })}
              />
              <DrillStat
                label="Outstanding credit liability"
                hint="What the studio still owes in classes"
                value={credits.data?.outstandingTotal ?? '…'}
                href={drillTo('/reports', { tab: 'Credits' })}
              />
            </>
          ) : (
            <p className="px-4 pb-4 text-sm text-muted">
              {can('reports.view') ? 'Loading…' : 'Your role cannot see financial figures.'}
            </p>
          )}
        </Section>

        <Section title="Operations" subtitle="Attendance, access and the schedule">
          <DrillStat
            label="Visits logged"
            hint={`Last ${days} days`}
            value={visits.data?.total ?? '…'}
            href={drillTo('/access/logs', { result: 'ALLOWED' })}
          />
          <DrillStat
            label="Denied at the gate"
            hint="Suspended, out of credit, or double-scanned"
            value={visits.data?.denied ?? '…'}
            href={drillTo('/access/logs', { result: 'DENIED' })}
          />
          <DrillStat
            label="No-shows"
            hint="Chase these before they churn"
            value={noShows}
            href={drillTo('/operations/bookings', { status: 'NO_SHOW' })}
          />
          <DrillStat
            label="Members"
            hint="Everyone on the roster"
            value={credits.data?.perMember.length ?? '…'}
            href="/members"
          />
        </Section>
      </div>
    </div>
  );
}

function SalesReport({ days }: { days: number }) {
  const { data, isLoading } = useQuery({
    queryKey: ['report-sales', days],
    queryFn: () => api.admin.reports.sales(days),
  });
  if (isLoading || !data) return <Spinner label="Crunching sales…" />;
  return (
    <div className="flex flex-col gap-4">
      <StatRow>
        <StatCard tone="ink" label={`Revenue · ${days}d`} value={formatIdr(data.totalIdr)} />
        <StatCard
          tone="danger"
          label="Refunded"
          value={formatIdr(data.refundsIdr)}
          hint="Not counted in revenue"
        />
        {data.byChannel.slice(0, 2).map((c) => (
          <StatCard tone="info" key={c.channel} label={`via ${c.channel}`} value={formatIdr(c.totalIdr)} />
        ))}
      </StatRow>
      <div className="a-card">
        <p className="a-label">Revenue by day</p>
        <ResponsiveContainer width="100%" height={240}>
          <BarChart data={data.byDay}>
            <CartesianGrid stroke={chartStyle.grid} vertical={false} />
            <XAxis dataKey="date" stroke={chartStyle.axis} fontSize={11} tickFormatter={(d: string) => d.slice(5)} />
            <YAxis stroke={chartStyle.axis} fontSize={11} tickFormatter={(v: number) => `${Math.round(v / 1e6)}M`} />
            <Tooltip contentStyle={chartStyle.tooltip} formatter={(v) => [formatIdr(Number(v)), 'Revenue']} />
            <Bar dataKey="value" fill={chartStyle.bar} radius={[4, 4, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Section title="By package" subtitle="Open the payments that made each figure">
          {data.byPackage.map((p) => (
            <DrillStat
              key={p.packageId}
              label={p.packageName}
              hint={`${p.purchaseCount} ${p.purchaseCount === 1 ? 'purchase' : 'purchases'}`}
              value={formatIdr(p.revenueIdr)}
              href={drillTo('/commercial/payments', { packageId: p.packageId, status: 'PAID' })}
            />
          ))}
        </Section>
        <Section title="By channel" subtitle="How members paid">
          {data.byChannel.map((c) => (
            <DrillStat
              key={c.channel}
              label={c.channel}
              value={formatIdr(c.totalIdr)}
              href={drillTo('/commercial/payments', { channel: c.channel, status: 'PAID' })}
            />
          ))}
        </Section>
      </div>
    </div>
  );
}

function VisitsReport({ days }: { days: number }) {
  const { data, isLoading } = useQuery({
    queryKey: ['report-visits', days],
    queryFn: () => api.admin.reports.visits(days),
  });
  if (isLoading || !data) return <Spinner label="Counting visits…" />;
  const busiest = [...data.byDay].sort((a, b) => b.value - a.value).slice(0, 5);
  return (
    <div className="flex flex-col gap-4">
      <StatRow>
        <StatCard tone="ink" label={`Visits · ${days}d`} value={data.total} />
        <StatCard tone="danger" label="Denied attempts" value={data.denied} />
        <StatCard tone="info" label="Offline transactions" value={data.offline} />
      </StatRow>
      <div className="a-card">
        <p className="a-label">Visits by day</p>
        <ResponsiveContainer width="100%" height={240}>
          <BarChart data={data.byDay}>
            <CartesianGrid stroke={chartStyle.grid} vertical={false} />
            <XAxis dataKey="date" stroke={chartStyle.axis} fontSize={11} tickFormatter={(d: string) => d.slice(5)} />
            <YAxis stroke={chartStyle.axis} fontSize={11} allowDecimals={false} />
            <Tooltip contentStyle={chartStyle.tooltip} formatter={(v) => [v, 'Visits']} />
            <Bar dataKey="value" fill={chartStyle.bar} radius={[4, 4, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Section title="Busiest days" subtitle="Where the staffing has to be">
          {busiest.map((d) => (
            <DrillStat
              key={d.date}
              label={formatDay(d.date)}
              value={d.value}
              href={drillTo('/access/logs', { on: d.date })}
            />
          ))}
        </Section>
        <Section title="Exceptions" subtitle="The scans that did not open a door">
          <DrillStat
            label="Denied"
            hint="Suspended, out of credit, or double-scanned"
            value={data.denied}
            href={drillTo('/access/logs', { result: 'DENIED' })}
          />
          <DrillStat
            label="Offline"
            hint="Recorded at the door, reconciled later"
            value={data.offline}
            href={drillTo('/access/logs', { mode: 'OFFLINE' })}
          />
        </Section>
      </div>
    </div>
  );
}

function ClassesReport({ days }: { days: number }) {
  const { data, isLoading } = useQuery({
    queryKey: ['report-classes', days],
    queryFn: api.admin.reports.classes,
  });
  if (isLoading || !data) return <Spinner label="Counting attendance…" />;
  const totals = data.perType.reduce(
    (acc, r) => ({
      sessions: acc.sessions + r.sessionsHeld,
      booked: acc.booked + r.booked,
      attended: acc.attended + r.attended,
      noShows: acc.noShows + r.noShows,
    }),
    { sessions: 0, booked: 0, attended: 0, noShows: 0 },
  );
  const rate = totals.booked > 0 ? Math.round((totals.attended / totals.booked) * 100) : 0;

  return (
    <div className="flex flex-col gap-4">
      <StatRow>
        <StatCard tone="ink" label="Sessions held" value={totals.sessions} />
        <StatCard tone="ok" label="Attended" value={totals.attended} hint={`${rate}% of bookings`} />
        <StatCard tone="warn" label="No-shows" value={totals.noShows} />
        <StatCard tone="info" label="Booked" value={totals.booked} />
      </StatRow>
      <div className="a-card !p-0">
        <p className="a-label px-4 pt-4">Attendance per class type</p>
        <p className="px-4 pb-1 text-xs text-muted">Click a row for the sessions behind it</p>
        <table className="a-table">
          <thead>
            <tr>
              <th>Class type</th>
              <th className="text-right">Sessions</th>
              <th className="text-right">Booked</th>
              <th className="text-right">Attended</th>
              <th className="text-right">No-shows</th>
              <th className="text-right">Attendance</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data.perType.map((row) => (
              <tr key={row.classTypeId} className="cursor-pointer">
                <td className="font-bold">
                  <Link
                    href={drillTo('/operations/sessions', { classTypeId: row.classTypeId })}
                    className="hover:text-brand"
                  >
                    {row.classTypeName}
                  </Link>
                </td>
                <td className="text-right">{row.sessionsHeld}</td>
                <td className="text-right">{row.booked}</td>
                <td className="text-right text-ok">{row.attended}</td>
                <td className={`text-right ${row.noShows > 0 ? 'font-bold text-danger' : 'text-muted'}`}>
                  {row.noShows}
                </td>
                <td className="text-right font-black">{row.booked > 0 ? `${row.attendanceRate}%` : '-'}</td>
                <td className="text-right">
                  <Link
                    href={drillTo('/operations/sessions', { classTypeId: row.classTypeId })}
                    className="text-muted hover:text-brand"
                    aria-label={`Sessions for ${row.classTypeName}`}
                  >
                    <ChevronRight size={16} />
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Section title="Recent no-shows" subtitle="Open a member to see the rest of their history">
        {data.recentNoShows.map((row, i) => (
          <DrillStat
            key={i}
            label={row.memberName}
            hint={`${row.classTypeName} · ${formatDay(row.startsAt)}`}
            value=""
            href={drillTo('/operations/bookings', { status: 'NO_SHOW', q: row.memberName })}
          />
        ))}
        {data.recentNoShows.length === 0 ? (
          <p className="px-4 pb-4 text-sm text-muted">No no-shows recorded.</p>
        ) : null}
      </Section>
    </div>
  );
}

function CreditsReport() {
  const { data, isLoading } = useQuery({ queryKey: ['report-credits'], queryFn: api.admin.reports.credits });
  const { filters, set, clear, dirty } = useFilters({ owing: '' });
  if (isLoading || !data) return <Spinner label="Reconciling ledgers…" />;

  const rows = data.perMember
    .filter((m) => m.balance !== 0 || m.expiring !== 0)
    .filter((m) => (filters.owing === 'expiring' ? m.expiring > 0 : true))
    .sort((a, b) => b.balance - a.balance);

  return (
    <div className="flex flex-col gap-4">
      <StatRow>
        <StatCard
          tone="ink"
          label="Outstanding credits"
          value={data.outstandingTotal}
          hint="Σ of every member's ledger — the studio's credit liability"
        />
        <StatCard
          label="Expiring credits"
          value={data.expiringTotal}
          tone="danger"
          hint="Within the reminder window"
          onClick={() => set('owing', filters.owing === 'expiring' ? '' : 'expiring')}
          active={filters.owing === 'expiring'}
        />
      </StatRow>
      <FilterBar
        dirty={dirty}
        onClear={clear}
        chips={
          filters.owing === 'expiring'
            ? [{ key: 'owing', label: 'Expiring only', onRemove: () => set('owing', '') }]
            : []
        }
      >
        <span className="text-xs text-muted">
          {rows.length} {rows.length === 1 ? 'member' : 'members'} with a balance
        </span>
      </FilterBar>
      <div className="a-card !p-0">
        <table className="a-table">
          <thead>
            <tr>
              <th>Member</th>
              <th className="text-right">Balance</th>
              <th className="text-right">Expiring</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((m) => (
              <tr key={m.memberId}>
                <td>
                  <Link href={`/members/${m.memberId}`} className="font-bold hover:text-brand">
                    {m.memberName}
                  </Link>
                </td>
                <td className="text-right font-black">{m.balance}</td>
                <td className={`text-right ${m.expiring > 0 ? 'font-bold text-danger' : 'text-muted'}`}>
                  {m.expiring || '-'}
                </td>
              </tr>
            ))}
            {rows.length === 0 ? (
              <tr>
                <td colSpan={3} className="py-6 text-center text-muted">
                  Nothing matches that filter.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
