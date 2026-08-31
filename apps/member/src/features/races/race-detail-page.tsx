import { Spinner, StatusBadge, formatDayTime, formatDuration } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, ExternalLink, MapPin } from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';
import { useInvalidateAll } from '../../lib/queries';
import { RegisterSheet } from './races-page';

export function RaceDetailPage() {
  const { raceId = '' } = useParams();
  const navigate = useNavigate();
  const invalidate = useInvalidateAll();
  const [registerOpen, setRegisterOpen] = useState(false);
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['race', raceId],
    queryFn: () => api.races.get(raceId),
  });

  if (isLoading || !data) return <Spinner label="Loading race…" />;
  const { view, myRace } = data;
  const e = view.event;
  const daysToRace = Math.ceil((new Date(e.startsAt).getTime() - Date.now()) / (24 * 3600_000));
  const upcoming = daysToRace > 0 && e.status !== 'COMPLETED' && e.status !== 'CANCELLED';

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>

      <div className="card relative overflow-hidden !border-0 !p-0 text-white">
        {e.imageUrl ? (
          <img src={e.imageUrl} alt="" className="h-56 w-full object-cover" />
        ) : (
          <div className="surface-ink h-56 w-full" />
        )}
        <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/30 to-black/10" />
        <div className="absolute inset-x-0 bottom-0 p-6">
          <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/60">
            {formatDayTime(e.startsAt)}
          </p>
          <p className="display mt-0.5 text-4xl leading-tight">{e.name}</p>
          <p className="mt-1 flex items-center gap-1 text-sm font-bold text-white/70">
            <MapPin size={13} /> {e.venue} · {e.city}, {e.country}
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <StatusBadge status={e.status} />
            {upcoming ? (
              <span className="chip bg-white/15 text-white backdrop-blur">{daysToRace} days away</span>
            ) : null}
            <span className="chip bg-white/15 text-white backdrop-blur">
              {view.participantCount} from this studio
            </span>
          </div>
        </div>
      </div>

      {myRace ? (
        <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
          <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
          <div className="relative">
            <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
              You're registered
            </p>
            <div className="mt-2 grid grid-cols-2 gap-4 text-sm">
              <div>
                <p className="text-white/50">Division</p>
                <p className="font-extrabold">{myRace.userRace.division.replaceAll('_', ' ')}</p>
              </div>
              <div>
                <p className="text-white/50">Goal</p>
                <p className="display text-xl">
                  {myRace.userRace.goalSec ? formatDuration(myRace.userRace.goalSec) : '—'}
                </p>
              </div>
            </div>
            <p className="mt-3 text-xs text-white/50">
              Train with full simulations in Record — your readiness score lives on the Races tab.
            </p>
          </div>
        </div>
      ) : upcoming ? (
        <button className="btn-brand" onClick={() => setRegisterOpen(true)}>
          Add to my races
        </button>
      ) : null}

      <a
        href={e.registrationUrl}
        target="_blank"
        rel="noreferrer"
        className="btn-ghost flex items-center justify-center gap-2"
      >
        Official registration <ExternalLink size={15} />
      </a>

      {registerOpen ? (
        <RegisterSheet
          view={view}
          onClose={() => setRegisterOpen(false)}
          onDone={() => {
            setRegisterOpen(false);
            invalidate();
            void refetch();
          }}
        />
      ) : null}
    </div>
  );
}
