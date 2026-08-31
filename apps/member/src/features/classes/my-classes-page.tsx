import { Spinner, formatDay, formatDayTime } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, ChevronRight } from 'lucide-react';
import { Link, useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { classImage } from '../../lib/images';
import { useSessions, useWallet } from '../../lib/queries';

/** Class types the member is entitled to through their active packages. */
export function MyClassesPage() {
  const navigate = useNavigate();
  const { data: wallet, isLoading: walletLoading } = useWallet();
  const { data: classTypes, isLoading: typesLoading } = useQuery({
    queryKey: ['class-types-catalog'],
    queryFn: api.catalog.classTypes,
  });
  const { data: sessions } = useSessions();

  if (walletLoading || typesLoading || !wallet || !classTypes)
    return <Spinner label="Loading your classes…" />;

  const activePkgs = wallet.myPackages.filter((p) => p.active);
  const coversAll = activePkgs.some((p) => p.coverageIds === null);
  const coveredIds = new Set(activePkgs.flatMap((p) => p.coverageIds ?? []));
  const entitled = classTypes.filter((c) => coversAll || coveredIds.has(c.id));

  const packagesFor = (classTypeId: string) =>
    activePkgs.filter((p) => p.coverageIds === null || p.coverageIds.includes(classTypeId));

  const nextSessionFor = (classTypeId: string) =>
    (sessions ?? [])
      .filter(
        (v) =>
          v.session.classTypeId === classTypeId &&
          ['PUBLISHED', 'FULL'].includes(v.session.status) &&
          new Date(v.session.startsAt).getTime() > Date.now(),
      )
      .sort(
        (a, b) => new Date(a.session.startsAt).getTime() - new Date(b.session.startsAt).getTime(),
      )[0];

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <div>
        <h1 className="display text-3xl">My classes</h1>
        <p className="mt-1 text-sm text-muted">
          Everything your purchased packages let you book.
        </p>
      </div>

      {activePkgs.length === 0 ? (
        <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
          <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
          <div className="relative">
            <p className="display text-2xl leading-tight">No active package yet.</p>
            <p className="mt-1.5 text-sm text-white/60">
              Top up with a credit package and your covered classes appear here.
            </p>
            <Link to="/wallet/topup" className="btn-brand mt-4 block text-center">
              Browse packages
            </Link>
          </div>
        </div>
      ) : (
        <>
          {/* Where the entitlement comes from */}
          <div className="-mx-5 flex snap-x gap-2.5 overflow-x-auto px-5 pb-1">
            {activePkgs.map((p) => (
              <div
                key={p.lotId}
                className="surface-ink min-w-56 shrink-0 snap-start rounded-2xl p-4 text-white"
              >
                <p className="truncate text-sm font-extrabold">{p.name}</p>
                <p className="mt-0.5 text-xs text-white/55">
                  {p.coverageNames ? `${p.coverageNames.length} classes` : 'All classes'} · until{' '}
                  {formatDay(p.expiresAt)}
                </p>
              </div>
            ))}
          </div>

          <div className="flex flex-col gap-3">
            {entitled.map((c) => {
              const image = classImage(c.id);
              const next = nextSessionFor(c.id);
              const sources = packagesFor(c.id);
              return (
                <div key={c.id} className="card overflow-hidden !p-0">
                  {image ? (
                    <div className="relative h-28 w-full">
                      <img src={image} alt="" className="h-full w-full object-cover" loading="lazy" />
                      <div className="absolute inset-0 bg-gradient-to-t from-black/70 to-black/5" />
                      <p className="display absolute bottom-2.5 left-4 text-2xl text-white">
                        {c.name}
                      </p>
                    </div>
                  ) : null}
                  <div className="p-4">
                    {!image ? <p className="display text-xl">{c.name}</p> : null}
                    <p className="text-sm text-muted">{c.description}</p>
                    <div className="mt-2.5 flex flex-wrap gap-1.5">
                      <span className="chip bg-surface-raised text-muted">
                        {c.defaultDurationMin} min · {c.defaultCreditCost} cr
                      </span>
                      {sources.slice(0, 2).map((p) => (
                        <span key={p.lotId} className="chip bg-brand/10 text-brand">
                          {p.name}
                        </span>
                      ))}
                    </div>
                    {next ? (
                      <Link
                        to={`/classes/${next.session.id}`}
                        className="mt-3 flex items-center justify-between rounded-xl bg-surface-raised px-3.5 py-2.5 text-sm"
                      >
                        <span className="min-w-0">
                          <span className="block font-extrabold">Next session</span>
                          <span className="block truncate text-xs text-muted">
                            {formatDayTime(next.session.startsAt)} · {next.branchName}
                          </span>
                        </span>
                        <ChevronRight size={16} className="shrink-0 text-muted" />
                      </Link>
                    ) : (
                      <p className="mt-3 text-xs font-bold text-muted">
                        No upcoming sessions scheduled.
                      </p>
                    )}
                  </div>
                </div>
              );
            })}
            {entitled.length === 0 ? (
              <p className="card text-sm text-muted">
                Your packages don't cover any active class types.
              </p>
            ) : null}
          </div>
        </>
      )}
    </div>
  );
}
