import { Spinner, formatDayTime } from '@nuhabit/ui';
import { ChevronLeft } from 'lucide-react';
import { Link, useParams } from 'react-router';
import { initialsOf } from '../../lib/initials';
import { useTrainer } from '../../lib/queries';

/** One trainer: who they are, and every class of theirs a member can book. */
export function TrainerDetailPage() {
  const { coachId = '' } = useParams();
  const { data, isLoading, isError } = useTrainer(coachId);

  if (isLoading) return <Spinner label="Loading trainer…" />;
  if (isError || !data) {
    return (
      <div className="flex flex-col gap-4">
        <BackLink />
        <div className="card text-sm text-muted">We could not find that trainer.</div>
      </div>
    );
  }

  const { coach, branchName, upcoming, classTypeNames } = data;

  return (
    <div className="flex flex-col gap-5">
      <BackLink />

      <div className="surface-ink relative overflow-hidden rounded-3xl p-6 text-white">
        <div className="pattern-brand pointer-events-none absolute inset-0" aria-hidden />
        <div className="relative flex items-center gap-4">
          <span className="flex h-16 w-16 shrink-0 items-center justify-center rounded-full bg-lime text-lg font-black text-ink">
            {initialsOf(coach.name)}
          </span>
          <div className="min-w-0">
            <h1 className="display truncate text-2xl font-black">{coach.name}</h1>
            <p className="truncate text-sm text-white/60">
              {coach.specialization || 'Coach'} · {branchName}
            </p>
          </div>
        </div>
        {coach.bio ? <p className="relative mt-4 text-sm text-white/70">{coach.bio}</p> : null}
        {classTypeNames.length > 0 ? (
          <div className="relative mt-4 flex flex-wrap gap-1.5">
            {classTypeNames.map((name) => (
              <span
                key={name}
                className="rounded-full bg-white/10 px-2.5 py-0.5 text-[11px] font-bold text-white/80"
              >
                {name}
              </span>
            ))}
          </div>
        ) : null}
      </div>

      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-black uppercase tracking-wider text-muted">Coming up</h2>
        <Link to={`/classes`} className="text-xs font-bold text-brand">
          Full schedule →
        </Link>
      </div>

      {upcoming.length === 0 ? (
        <div className="card text-sm text-muted">
          {coach.name.split(' ')[0]} has nothing scheduled in the next fortnight.
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {upcoming.map((v) => {
            const mine = v.myBooking;
            const full = v.spotsLeft === 0;
            return (
              <Link
                key={v.session.id}
                to={`/classes/${v.session.id}`}
                className="card flex items-center gap-4"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate font-black">{v.classTypeName}</p>
                  <p className="truncate text-sm text-muted">
                    {formatDayTime(v.session.startsAt)} · {v.branchName}
                  </p>
                </div>
                <div className="shrink-0 text-right text-xs font-black uppercase">
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

function BackLink() {
  return (
    <Link to="/trainers" className="flex items-center gap-1 text-sm font-bold text-muted">
      <ChevronLeft size={16} />
      Trainers
    </Link>
  );
}
