import { Spinner } from '@nuhabit/ui';
import { useState } from 'react';
import { Link } from 'react-router';
import { initialsOf } from '../../lib/initials';
import { useBranches, useTrainers } from '../../lib/queries';

/**
 * Browsing by who is teaching.
 *
 * People pick a class by the coach as often as by the hour, and the schedule
 * could only be read by time. Coaches with nothing on stay on the list — a
 * member looking for somebody by name should find them and be told they have
 * nothing coming up, rather than be left wondering whether they have left.
 */
export function TrainersPage() {
  const [branchId, setBranchId] = useState('');
  const { data: branches } = useBranches();
  const { data: trainers, isLoading } = useTrainers(branchId || undefined);

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="display text-3xl font-black">Trainers</h1>
        <p className="mt-1 text-sm text-muted">Who is teaching over the next fortnight.</p>
      </div>

      <div className="-mx-5 flex gap-2 overflow-x-auto px-5 pb-1">
        <button
          onClick={() => setBranchId('')}
          className={`shrink-0 rounded-full px-4 py-1.5 text-sm font-bold transition ${
            branchId === '' ? 'bg-brand/10 text-brand ring-1 ring-brand/20' : 'bg-surface text-muted'
          }`}
        >
          All branches
        </button>
        {(branches ?? []).map((b) => (
          <button
            key={b.id}
            onClick={() => setBranchId(b.id)}
            className={`shrink-0 rounded-full px-4 py-1.5 text-sm font-bold transition ${
              branchId === b.id
                ? 'bg-brand/10 text-brand ring-1 ring-brand/20'
                : 'bg-surface text-muted'
            }`}
          >
            {b.name}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Spinner label="Loading trainers…" />
      ) : (trainers ?? []).length === 0 ? (
        <div className="card text-sm text-muted">No trainers at this branch yet.</div>
      ) : (
        <div className="flex flex-col gap-2">
          {(trainers ?? []).map((t) => (
            <Link key={t.coach.id} to={`/trainers/${t.coach.id}`} className="card flex gap-4">
              <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-brand text-sm font-black text-lime">
                {initialsOf(t.coach.name)}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate font-black">{t.coach.name}</span>
                <span className="block truncate text-sm text-muted">
                  {t.coach.specialization || t.branchName}
                </span>
                {t.classTypeNames.length > 0 ? (
                  <span className="mt-1.5 flex flex-wrap gap-1">
                    {t.classTypeNames.slice(0, 2).map((name) => (
                      <span
                        key={name}
                        className="rounded-full bg-brand/10 px-2 py-0.5 text-[11px] font-bold text-brand"
                      >
                        {name}
                      </span>
                    ))}
                  </span>
                ) : null}
              </span>
              <span className="shrink-0 text-right">
                {t.upcomingCount > 0 ? (
                  <>
                    <span className="display block text-lg font-black leading-tight">
                      {t.upcomingCount}
                    </span>
                    <span className="block text-[11px] text-muted">
                      {t.upcomingCount === 1 ? 'class' : 'classes'}
                    </span>
                  </>
                ) : (
                  <span className="text-[11px] text-muted">Nothing on</span>
                )}
              </span>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
