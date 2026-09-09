'use client';

import { Spinner, formatTime } from '@nuhabit/ui';
import { useQuery } from '@tanstack/react-query';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import Link from 'next/link';
import { useMemo, useState } from 'react';
import { api } from '../../../../lib/api';
import { PageTitle, SearchSelect } from '../../../../components/ui';

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
            <div className="max-w-40">
              <SearchSelect
                value={branchId}
                onChange={setBranchId}
                allowEmpty
                emptyLabel='All branches'
                placeholder="Search…"
                options={[
                  ...(branches ?? []).map((b) => ({ value: b.id, label: b.name })),
                ]}
              />
            </div>
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
        // grid-cols-7 divided whatever width was there by seven, so every
        // column squeezed to about 120px and every line truncated — the
        // overflow-x-auto could never fire, because the grid shrank instead of
        // overflowing. Fixed-minimum columns let a narrow window scroll the
        // week rather than crush it.
        <div className="-mx-1 overflow-x-auto px-1 pb-2">
          <div className="grid grid-flow-col auto-cols-[minmax(11.5rem,1fr)] gap-3">
            {days.map((day) => {
              const daySessions = (sessions ?? [])
                .filter((v) => new Date(v.session.startsAt).toDateString() === day.toDateString())
                .sort((a, b) => new Date(a.session.startsAt).getTime() - new Date(b.session.startsAt).getTime());
              return (
                <div key={day.toISOString()}>
                  <p
                    className={`mb-2 rounded-lg px-2 py-1.5 text-center text-xs font-black uppercase tracking-wide ${
                      isToday(day) ? 'bg-brand text-white' : 'bg-surface-raised text-muted'
                    }`}
                  >
                    {day.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric' })}
                  </p>
                  <div className="flex flex-col gap-2">
                    {daySessions.map((v) => (
                      <Link
                        key={v.session.id}
                        href={`/operations/sessions/${v.session.id}`}
                        className={`block rounded-lg border border-line border-l-4 bg-surface px-3 py-2 transition hover:border-brand hover:shadow-[0_1px_3px_rgb(19_26_28/0.08)] ${
                          STATUS_COLOR[v.session.status] ?? ''
                        }`}
                      >
                        <div className="flex items-baseline justify-between gap-2">
                          <p className="text-xs font-black">{formatTime(v.session.startsAt)}</p>
                          {/* The number people actually scan for. Kept out of
                              the coach line so neither has to truncate. */}
                          <p className="shrink-0 text-[11px] font-bold tabular-nums text-muted">
                            {v.confirmedCount}/{v.session.capacity}
                            {v.waitlistCount > 0 ? (
                              <span className="text-warn"> +{v.waitlistCount}</span>
                            ) : null}
                          </p>
                        </div>
                        {/* Two lines rather than an ellipsis: "HYROX Fundam…"
                            and "HYROX Fundamentals" are the same word count
                            and only one of them is a class name. */}
                        <p className="mt-1 line-clamp-2 text-[13px] font-bold leading-tight">
                          {v.classTypeName}
                        </p>
                        <p className="mt-0.5 truncate text-[11px] text-muted">{v.coachName}</p>
                      </Link>
                    ))}
                    {daySessions.length === 0 ? (
                      <p className="rounded-lg border border-dashed border-line py-6 text-center text-[11px] text-muted/60">
                        No classes
                      </p>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
