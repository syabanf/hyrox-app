import { Spinner, formatTime } from '@nuhabit/ui';
import { Users } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { initialsOf } from '../../lib/initials';
import { useBranches, useSessions, useTrainers } from '../../lib/queries';

function dayKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

export function SchedulePage() {
  const [branchId, setBranchId] = useState<string>('');
  const [coachId, setCoachId] = useState<string>('');
  const { data: branches } = useBranches();
  // The trainer chips come from the same list the browse screen uses, so a
  // coach with nothing on this fortnight is not offered as a filter.
  const { data: trainers } = useTrainers(branchId || undefined);
  const { data: sessions, isLoading } = useSessions(branchId || undefined, coachId || undefined);

  const selectedTrainer = (trainers ?? []).find((t) => t.coach.id === coachId);

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
    <div className="flex flex-col gap-5">
      <div className="flex items-center justify-between gap-3">
        <h1 className="display text-3xl font-black">Classes</h1>
        <Link
          to="/trainers"
          className="flex items-center gap-1.5 rounded-full bg-surface px-3.5 py-1.5 text-sm font-bold text-brand"
        >
          <Users size={15} />
          Trainers
        </Link>
      </div>

      <div className="flex gap-2 overflow-x-auto pb-1">
        {days.map((d) => (
          <button
            key={d.key}
            onClick={() => setSelectedDay(d.key)}
            className={`flex min-w-14 flex-col items-center rounded-2xl border px-3 py-2 transition ${
              selectedDay === d.key
                ? 'surface-ink border-transparent text-white shadow-[0_8px_20px_rgb(0_40_26/0.25)]'
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
          className={`rounded-full px-4 py-1.5 text-sm font-bold transition ${
            branchId === '' ? 'bg-brand/10 text-brand ring-1 ring-brand/20' : 'bg-surface text-muted'
          }`}
        >
          All branches
        </button>
        {(branches ?? []).map((b) => (
          <button
            key={b.id}
            onClick={() => setBranchId(b.id)}
            className={`rounded-full px-4 py-1.5 text-sm font-bold transition ${
              branchId === b.id ? 'bg-brand/10 text-brand ring-1 ring-brand/20' : 'bg-surface text-muted'
            }`}
          >
            {b.name}
          </button>
        ))}
      </div>

      {/* Who is teaching. Only coaches with classes in the window appear, so a
          chip never leads to an empty day. */}
      {(trainers ?? []).some((t) => t.upcomingCount > 0) ? (
        <div className="-mx-5 flex gap-2 overflow-x-auto px-5 pb-1">
          <button
            onClick={() => setCoachId('')}
            className={`shrink-0 rounded-full px-4 py-1.5 text-sm font-bold transition ${
              coachId === '' ? 'bg-brand/10 text-brand ring-1 ring-brand/20' : 'bg-surface text-muted'
            }`}
          >
            Any trainer
          </button>
          {(trainers ?? [])
            .filter((t) => t.upcomingCount > 0)
            .map((t) => (
              <button
                key={t.coach.id}
                onClick={() => setCoachId(coachId === t.coach.id ? '' : t.coach.id)}
                className={`shrink-0 rounded-full px-4 py-1.5 text-sm font-bold transition ${
                  coachId === t.coach.id
                    ? 'bg-brand/10 text-brand ring-1 ring-brand/20'
                    : 'bg-surface text-muted'
                }`}
              >
                {t.coach.name.split(' ')[0]}
              </button>
            ))}
        </div>
      ) : null}

      {selectedTrainer ? (
        <Link to={`/trainers/${selectedTrainer.coach.id}`} className="card flex items-center gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand text-sm font-black text-lime">
            {initialsOf(selectedTrainer.coach.name)}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-black">{selectedTrainer.coach.name}</span>
            <span className="block truncate text-sm text-muted">
              {selectedTrainer.coach.specialization || selectedTrainer.branchName}
            </span>
          </span>
          <span className="text-xs font-bold text-brand">See profile →</span>
        </Link>
      ) : null}

      {isLoading ? (
        <Spinner label="Loading schedule…" />
      ) : visible.length === 0 ? (
        <div className="card text-sm text-muted">
          {coachId
            ? 'Nothing from this trainer on this day. Try another day, or clear the filter.'
            : 'No more classes this day.'}
        </div>
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
