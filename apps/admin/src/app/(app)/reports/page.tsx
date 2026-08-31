'use client';

import { Spinner, formatDay, formatIdr } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';
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
import { PageTitle, StatCard } from '../../../components/ui';

const TABS = ['Sales', 'Visits', 'Classes', 'Credits'] as const;
type Tab = (typeof TABS)[number];

const chartStyle = {
  grid: 'rgb(0 0 0 / 0.08)',
  axis: '#6f6f76',
  bar: '#ed1c24',
  tooltip: { background: '#ffffff', border: '1px solid #e4e4de', borderRadius: 8, color: '#191919' },
};

export default function ReportsPage() {
  const { can } = usePermissions();
  const [tab, setTab] = useState<Tab>(can('reports.financial') ? 'Sales' : 'Visits');

  return (
    <div>
      <PageTitle title="Reports" subtitle="Read-only aggregates derived from operational transactions" />
      <div className="mb-4 flex gap-1 rounded-xl bg-surface p-1">
        {TABS.filter((t) => t !== 'Sales' || can('reports.view')).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-lg px-4 py-1.5 text-sm font-bold ${
              tab === t ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      {tab === 'Sales' ? <SalesReport /> : null}
      {tab === 'Visits' ? <VisitsReport /> : null}
      {tab === 'Classes' ? <ClassesReport /> : null}
      {tab === 'Credits' ? <CreditsReport /> : null}
    </div>
  );
}

function ClassesReport() {
  const { data, isLoading } = useQuery({ queryKey: ['report-classes'], queryFn: api.admin.reports.classes });
  if (isLoading || !data) return <Spinner label="Counting attendance…" />;
  return (
    <div className="flex flex-col gap-4">
      <div className="a-card !p-0">
        <p className="a-label px-4 pt-4">Attendance per class type (completed sessions)</p>
        <table className="a-table">
          <thead>
            <tr>
              <th>Class type</th>
              <th className="text-right">Sessions</th>
              <th className="text-right">Booked</th>
              <th className="text-right">Attended</th>
              <th className="text-right">No-shows</th>
              <th className="text-right">Attendance</th>
            </tr>
          </thead>
          <tbody>
            {data.perType.map((row) => (
              <tr key={row.classTypeId}>
                <td className="font-bold">{row.classTypeName}</td>
                <td className="text-right">{row.sessionsHeld}</td>
                <td className="text-right">{row.booked}</td>
                <td className="text-right text-ok">{row.attended}</td>
                <td className={`text-right ${row.noShows > 0 ? 'font-bold text-danger' : 'text-muted'}`}>
                  {row.noShows}
                </td>
                <td className="text-right font-black">{row.booked > 0 ? `${row.attendanceRate}%` : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="a-card !p-0">
        <p className="a-label px-4 pt-4">Recent no-shows</p>
        <table className="a-table">
          <tbody>
            {data.recentNoShows.map((row, i) => (
              <tr key={i}>
                <td className="font-bold">{row.memberName}</td>
                <td>{row.classTypeName}</td>
                <td className="text-right text-muted">{formatDay(row.startsAt)}</td>
              </tr>
            ))}
            {data.recentNoShows.length === 0 ? (
              <tr>
                <td className="py-4 text-center text-muted">No no-shows recorded.</td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function SalesReport() {
  const { data, isLoading } = useQuery({ queryKey: ['report-sales'], queryFn: () => api.admin.reports.sales(30) });
  if (isLoading || !data) return <Spinner label="Crunching sales…" />;
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-3">
        <StatCard label="Revenue (30d)" value={formatIdr(data.totalIdr)} tone="brand" />
        {data.byChannel.slice(0, 2).map((c) => (
          <StatCard key={c.channel} label={`via ${c.channel}`} value={formatIdr(c.totalIdr)} />
        ))}
      </div>
      <div className="a-card">
        <p className="a-label">Revenue by day</p>
        <ResponsiveContainer width="100%" height={240}>
          <BarChart data={data.byDay}>
            <CartesianGrid stroke={chartStyle.grid} vertical={false} />
            <XAxis dataKey="date" stroke={chartStyle.axis} fontSize={11} tickFormatter={(d: string) => d.slice(5)} />
            <YAxis stroke={chartStyle.axis} fontSize={11} tickFormatter={(v: number) => `${Math.round(v / 1e6)}M`} />
            <Tooltip
              contentStyle={chartStyle.tooltip}
              formatter={(v) => [formatIdr(Number(v)), 'Revenue']}
            />
            <Bar dataKey="value" fill={chartStyle.bar} radius={[4, 4, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
      <div className="a-card !p-0">
        <p className="a-label px-4 pt-4">Package performance (30d)</p>
        <table className="a-table">
          <thead>
            <tr>
              <th>Package</th>
              <th className="text-right">Purchases</th>
              <th className="text-right">Revenue</th>
            </tr>
          </thead>
          <tbody>
            {data.byPackage.map((p) => (
              <tr key={p.pkg.id}>
                <td className="font-bold">{p.pkg.name}</td>
                <td className="text-right">{p.purchaseCount}</td>
                <td className="text-right font-bold">{formatIdr(p.revenueIdr)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function VisitsReport() {
  const { data, isLoading } = useQuery({ queryKey: ['report-visits'], queryFn: () => api.admin.reports.visits(30) });
  if (isLoading || !data) return <Spinner label="Counting visits…" />;
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-3">
        <StatCard label="Visits (30d)" value={data.total} tone="brand" />
        <StatCard label="Denied attempts" value={data.denied} />
        <StatCard label="Offline transactions" value={data.offline} />
      </div>
      <div className="a-card">
        <p className="a-label">Visits by day</p>
        <ResponsiveContainer width="100%" height={240}>
          <BarChart data={data.byDay}>
            <CartesianGrid stroke={chartStyle.grid} vertical={false} />
            <XAxis dataKey="date" stroke={chartStyle.axis} fontSize={11} tickFormatter={(d: string) => d.slice(5)} />
            <YAxis stroke={chartStyle.axis} fontSize={11} allowDecimals={false} />
            <Tooltip
              contentStyle={chartStyle.tooltip}
              formatter={(v) => [v, 'Visits']}
            />
            <Bar dataKey="value" fill={chartStyle.bar} radius={[4, 4, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function CreditsReport() {
  const { data, isLoading } = useQuery({ queryKey: ['report-credits'], queryFn: api.admin.reports.credits });
  if (isLoading || !data) return <Spinner label="Reconciling ledgers…" />;
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <StatCard
          label="Outstanding credits"
          value={data.outstandingTotal}
          tone="brand"
          hint="Σ of every member's ledger — the studio's credit liability"
        />
        <StatCard
          label="Expiring credits"
          value={data.expiringTotal}
          tone={data.expiringTotal > 0 ? 'danger' : undefined}
          hint="Within the reminder window"
        />
      </div>
      <div className="a-card !p-0">
        <p className="a-label px-4 pt-4">Per member</p>
        <table className="a-table">
          <thead>
            <tr>
              <th>Member</th>
              <th className="text-right">Balance</th>
              <th className="text-right">Expiring</th>
            </tr>
          </thead>
          <tbody>
            {data.perMember
              .filter((m) => m.balance !== 0 || m.expiring !== 0)
              .map((m) => (
                <tr key={m.memberId}>
                  <td>
                    <Link href={`/members/${m.memberId}`} className="font-bold hover:text-brand">
                      {m.memberName}
                    </Link>
                  </td>
                  <td className="text-right font-black text-brand">{m.balance}</td>
                  <td className={`text-right font-bold ${m.expiring > 0 ? 'text-warn' : 'text-muted'}`}>
                    {m.expiring || '—'}
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted">
        Snapshot generated {formatDay(new Date().toISOString())} — derived live from the credit ledger.
      </p>
    </div>
  );
}
