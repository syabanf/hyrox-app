import { Spinner, formatTime } from '@hyrox/ui';
import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useBranches, useSessions } from '../../lib/queries';

function dayKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

export function SchedulePage() {
  const [branchId, setBranchId] = useState<string>('');
  const { data: branches } = useBranches();
  const { data: sessions, isLoading } = useSessions(branchId || undefined);

  const days = useMemo(() => {
    const out: { key: string; date: Date; label: string }[] = [];
    for (let i = 0; i < 7; i++) {
      const d = new Date();
      d.setDate(d.getDate() + i);
      out.push({
        key: dayKey(d.toISOString()),
        date: d,
        label:
          i === 0
            ? 'Today'
            : i === 1
              ? 'Tmrw'
              : d.toLocaleDateString(undefined, { weekday: 'short' }),
      });
    }
    return out;
  }, []);
  const [selectedDay, setSelectedDay] = useState(days[0]!.key);

  const visible = (sessions ?? []).filter(
    (v) =>
      dayKey(v.session.startsAt) === selectedDay &&
      ['PUBLISHED', 'FULL'].includes(v.session.status) &&
      new Date(v.session.endsAt).getTime() > Date.now(),
  );

  return (
    <div className="flex flex-col gap-4">
      <h1 className="display text-2xl font-black">Classes</h1>

      <div className="flex gap-2 overflow-x-auto pb-1">
        {days.map((d) => (
          <button
            key={d.key}
            onClick={() => setSelectedDay(d.key)}
            className={`flex min-w-14 flex-col items-center rounded-xl border px-3 py-2 ${
              selectedDay === d.key
                ? 'border-brand bg-brand text-white'
                : 'border-line bg-surface text-muted'
            }`}
          >
            <span className="text-[11px] font-bold uppercase">{d.label}</span>
            <span className="text-lg font-black">{d.date.getDate()}</span>
          </button>
        ))}
      </div>

      <div className="flex gap-2">
        <button
          onClick={() => setBranchId('')}
          className={`rounded-full px-4 py-1.5 text-sm font-bold ${
            branchId === '' ? 'bg-brand text-white' : 'bg-surface text-muted'
          }`}
        >
          All branches
        </button>
        {(branches ?? []).map((b) => (
          <button
            key={b.id}
            onClick={() => setBranchId(b.id)}
            className={`rounded-full px-4 py-1.5 text-sm font-bold ${
              branchId === b.id ? 'bg-brand text-white' : 'bg-surface text-muted'
            }`}
          >
            {b.name}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Spinner label="Loading schedule…" />
      ) : visible.length === 0 ? (
        <div className="card text-sm text-muted">No more classes this day.</div>
      ) : (
        <div className="flex flex-col gap-2">
          {visible.map((v) => {
            const mine = v.myBooking;
            const full = v.spotsLeft === 0;
            return (
              <Link
                key={v.session.id}
                to={`/classes/${v.session.id}`}
                className="card flex items-center gap-4"
              >
                <div className="w-14 text-center">
                  <p className="display text-lg font-black leading-tight">
                    {formatTime(v.session.startsAt)}
                  </p>
                  <p className="text-[11px] text-muted">{v.session.creditCost} cr</p>
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate font-black">{v.classTypeName}</p>
                  <p className="truncate text-sm text-muted">
                    {v.branchName} · {v.coachName}
                  </p>
                </div>
                <div className="text-right text-xs font-black uppercase">
                  {mine ? (
                    <span className={mine.status === 'CONFIRMED' ? 'text-ok' : 'text-warn'}>
                      {mine.status === 'WAITLIST' ? `WL #${mine.waitlistPosition}` : 'Booked'}
                    </span>
                  ) : full ? (
                    <span className="text-warn">Full · WL</span>
                  ) : (
                    <span className="text-brand">{v.spotsLeft} left</span>
                  )}
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
