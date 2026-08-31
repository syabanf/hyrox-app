'use client';

import { Spinner, StatusBadge, formatIdr, formatTime } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { api } from '../../../lib/api';
import { PageTitle, StatCard } from '../../../components/ui';

export default function DashboardPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard'],
    queryFn: api.admin.reports.dashboard,
    refetchInterval: 10_000,
  });

  if (isLoading || !data) return <Spinner label="Loading dashboard…" />;

  return (
    <div>
      <PageTitle title="Dashboard" subtitle="Today at a glance" />
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <StatCard label="Visitors today" value={data.visitorsToday} tone="brand" />
        <StatCard label="Classes today" value={data.classesToday} />
        <StatCard label="Revenue today" value={formatIdr(data.revenueTodayIdr)} />
        <StatCard label="Active members" value={data.activeMembers} />
        <StatCard label="Outstanding credits" value={data.outstandingCredits} hint="Total liability" />
        <StatCard
          label="Expiring credits"
          value={data.expiringCredits}
          tone={data.expiringCredits > 0 ? 'danger' : undefined}
          hint="Within reminder window"
        />
      </div>

      <div className="a-card mt-6 !p-0">
        <div className="flex items-center justify-between px-4 pt-4">
          <h2 className="display text-lg font-black">Today's sessions</h2>
          <Link href="/operations/sessions" className="text-sm font-bold text-brand">
            All sessions →
          </Link>
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
            {data.todaySessions.map((v) => (
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
            {data.todaySessions.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-6 text-center text-muted">
                  No sessions today.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
