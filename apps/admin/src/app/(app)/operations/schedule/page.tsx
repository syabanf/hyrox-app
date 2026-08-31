'use client';

import { Spinner, formatTime } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import Link from 'next/link';
import { useMemo, useState } from 'react';
import { api } from '../../../../lib/api';
import { PageTitle } from '../../../../components/ui';

function startOfWeek(base: Date): Date {
  const d = new Date(base);
  d.setHours(0, 0, 0, 0);
  d.setDate(d.getDate() - ((d.getDay() + 6) % 7)); // Monday
  return d;
}

const STATUS_COLOR: Record<string, string> = {
  PUBLISHED: 'border-l-ok',
  FULL: 'border-l-warn',
  COMPLETED: 'border-l-line',
  CANCELLED: 'border-l-danger',
  DRAFT: 'border-l-muted',
};

export default function SchedulePage() {
  const [weekOffset, setWeekOffset] = useState(0);
  const [branchId, setBranchId] = useState('');
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const weekStart = useMemo(() => {
    const d = startOfWeek(new Date());
    d.setDate(d.getDate() + weekOffset * 7);
    return d;
  }, [weekOffset]);
  const weekEnd = useMemo(() => {
    const d = new Date(weekStart);
    d.setDate(d.getDate() + 7);
    return d;
  }, [weekStart]);

  const { data: sessions, isLoading } = useQuery({
    queryKey: ['admin-sessions', 'week', weekStart.toISOString(), branchId],
    queryFn: () =>
      api.admin.sessions.list({
        branchId: branchId || undefined,
        from: weekStart.toISOString(),
        to: weekEnd.toISOString(),
      }),
  });

  const days = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(weekStart);
    d.setDate(d.getDate() + i);
    return d;
  });
  const isToday = (d: Date) => d.toDateString() === new Date().toDateString();

  return (
    <div>
      <PageTitle
        title="Weekly Schedule"
        subtitle="All sessions on a Monday–Sunday grid"
        actions={
          <div className="flex items-center gap-2">
            <select className="a-input max-w-40" value={branchId} onChange={(e) => setBranchId(e.target.value)}>
              <option value="">All branches</option>
              {(branches ?? []).map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name}
                </option>
              ))}
            </select>
            <button className="a-btn-ghost !px-2.5" onClick={() => setWeekOffset((w) => w - 1)} aria-label="Previous week">
              <ChevronLeft size={16} />
            </button>
            <button className="a-btn-ghost !px-3 text-xs" onClick={() => setWeekOffset(0)}>
              This week
            </button>
            <button className="a-btn-ghost !px-2.5" onClick={() => setWeekOffset((w) => w + 1)} aria-label="Next week">
              <ChevronRight size={16} />
            </button>
          </div>
        }
      />
      {isLoading ? (
        <Spinner label="Loading week…" />
      ) : (
        <div className="grid grid-cols-7 gap-2 overflow-x-auto">
          {days.map((day) => {
            const daySessions = (sessions ?? [])
              .filter((v) => new Date(v.session.startsAt).toDateString() === day.toDateString())
              .sort((a, b) => new Date(a.session.startsAt).getTime() - new Date(b.session.startsAt).getTime());
            return (
              <div key={day.toISOString()} className="min-w-32">
                <p
                  className={`mb-2 rounded-lg px-2 py-1 text-center text-xs font-black uppercase ${
                    isToday(day) ? 'bg-brand text-white' : 'text-muted'
                  }`}
                >
                  {day.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric' })}
                </p>
                <div className="flex flex-col gap-1.5">
                  {daySessions.map((v) => (
                    <Link
                      key={v.session.id}
                      href={`/operations/sessions/${v.session.id}`}
                      className={`block rounded-lg border border-line border-l-4 bg-surface px-2 py-1.5 hover:border-brand ${
                        STATUS_COLOR[v.session.status] ?? ''
                      }`}
                    >
                      <p className="text-[11px] font-black">{formatTime(v.session.startsAt)}</p>
                      <p className="truncate text-xs font-bold">{v.classTypeName}</p>
                      <p className="truncate text-[10px] text-muted">
                        {v.coachName} · {v.confirmedCount}/{v.session.capacity}
                        {v.waitlistCount > 0 ? ` +${v.waitlistCount}` : ''}
                      </p>
                    </Link>
                  ))}
                  {daySessions.length === 0 ? (
                    <p className="py-4 text-center text-[10px] text-muted/60">—</p>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
