import { ApiError } from '@hyrox/api-client';
import type { MyRaceView, RaceEventView } from '@hyrox/contracts';
import type { Division } from '@hyrox/domain';
import { Spinner, StatusBadge, formatDay, formatDuration } from '@hyrox/ui';
import { Flag, MapPin, Trophy } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useMyRaces, useRaces } from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';

const TABS = ['Upcoming', 'Results', 'My Races'] as const;
type Tab = (typeof TABS)[number];
const REGIONS = ['', 'ASIA', 'EUROPE', 'AMERICAS', 'OCEANIA'];
const DIVISIONS: { id: Division; label: string }[] = [
  { id: 'MEN_OPEN', label: 'Men Open' },
  { id: 'MEN_PRO', label: 'Men Pro' },
  { id: 'WOMEN_OPEN', label: 'Women Open' },
  { id: 'WOMEN_PRO', label: 'Women Pro' },
];

export function RacesPage() {
  const [tab, setTab] = useState<Tab>('Upcoming');
  const [region, setRegion] = useState('');

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-2">
        <Flag size={22} className="text-brand" />
        <h1 className="display text-3xl">Races</h1>
      </div>
      <div className="flex rounded-xl bg-surface p-1">
        {TABS.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`flex-1 rounded-lg py-2 text-sm font-black uppercase tracking-wide ${
              tab === t ? 'bg-brand text-white' : 'text-muted'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      {tab !== 'My Races' ? (
        <div className="flex gap-2 overflow-x-auto pb-1">
          {REGIONS.map((r) => (
            <button
              key={r}
              onClick={() => setRegion(r)}
              className={`whitespace-nowrap rounded-full px-4 py-1.5 text-sm font-bold ${
                region === r ? 'bg-ink text-white' : 'bg-surface text-muted'
              }`}
            >
              {r === '' ? 'All regions' : r[0] + r.slice(1).toLowerCase()}
            </button>
          ))}
        </div>
      ) : null}
      {tab === 'My Races' ? (
        <MyRacesTab />
      ) : (
        <DiscoveryTab scope={tab === 'Results' ? 'results' : 'upcoming'} region={region} />
      )}
    </div>
  );
}

function DiscoveryTab({ scope, region }: { scope: 'upcoming' | 'results'; region: string }) {
  const { data, isLoading } = useRaces({ scope, region: region || undefined });
  const [registerTarget, setRegisterTarget] = useState<RaceEventView | null>(null);
  const invalidate = useInvalidateAll();

  if (isLoading) return <Spinner label="Finding races…" />;

  return (
    <div className="flex flex-col gap-3">
      {(data ?? []).map((v) => (
        <div key={v.event.id} className="card overflow-hidden !p-0">
          <div className="relative h-40 w-full">
            {v.event.imageUrl ? (
              <img src={v.event.imageUrl} alt="" className="h-full w-full object-cover" loading="lazy" />
            ) : (
              <div className="surface-brand h-full w-full" />
            )}
            <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/20 to-transparent" />
            <div className="absolute right-3 top-3">
              <StatusBadge status={v.event.status} />
            </div>
            <div className="absolute inset-x-0 bottom-0 p-4 text-white">
              <p className="text-[11px] font-extrabold uppercase tracking-[0.14em] opacity-90">
                {formatDay(v.event.startsAt)}
              </p>
              <p className="display text-2xl leading-tight">{v.event.name}</p>
              <p className="flex items-center gap-1 text-xs font-bold opacity-90">
                <MapPin size={12} />
                {v.event.venue}, {v.event.city}
              </p>
            </div>
          </div>
          <div className="flex items-center justify-between gap-3 p-3.5">
            <p className="text-xs font-bold text-muted">
              {v.participantCount} from this studio
            </p>
            {scope === 'upcoming' ? (
              v.joined ? (
                <p className="text-sm font-extrabold text-ok">On your race list</p>
              ) : v.event.status === 'REGISTRATION_OPEN' || v.event.status === 'ANNOUNCED' ? (
                <button className="btn-brand !px-4 !py-2 text-sm" onClick={() => setRegisterTarget(v)}>
                  Add to my races
                </button>
              ) : null
            ) : null}
          </div>
        </div>
      ))}
      {(data ?? []).length === 0 ? <p className="card text-sm text-muted">No races here yet.</p> : null}
      {registerTarget ? (
        <RegisterSheet
          view={registerTarget}
          onClose={() => setRegisterTarget(null)}
          onDone={() => {
            setRegisterTarget(null);
            invalidate();
          }}
        />
      ) : null}
    </div>
  );
}

function RegisterSheet({
  view,
  onClose,
  onDone,
}: {
  view: RaceEventView;
  onClose: () => void;
  onDone: () => void;
}) {
  const [division, setDivision] = useState<Division>('MEN_OPEN');
  const [goal, setGoal] = useState('01:30:00');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const parseGoal = (value: string): number | null => {
    const m = value.trim().match(/^(\d{1,2}):(\d{2}):(\d{2})$/);
    if (!m) return null;
    return Number(m[1]) * 3600 + Number(m[2]) * 60 + Number(m[3]);
  };

  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      await api.races.register(view.event.id, { division, goalSec: parseGoal(goal) });
      onDone();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Registration failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/50" onClick={onClose}>
      <div className="w-full max-w-md rounded-t-3xl bg-surface p-5" onClick={(e) => e.stopPropagation()}>
        <h2 className="display mb-1 text-xl">{view.event.name}</h2>
        <p className="mb-4 text-sm text-muted">
          Adds the race to My Races for goal setting and training. Official registration happens on
          hyrox.com.
        </p>
        <div className="flex flex-col gap-3">
          <div>
            <p className="label">Division</p>
            <div className="grid grid-cols-2 gap-2">
              {DIVISIONS.map((d) => (
                <button
                  key={d.id}
                  onClick={() => setDivision(d.id)}
                  className={`rounded-xl border px-3 py-2 text-sm font-black uppercase ${
                    division === d.id ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                  }`}
                >
                  {d.label}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="label">Goal time (hh:mm:ss, optional)</label>
            <input className="input" value={goal} onChange={(e) => setGoal(e.target.value)} placeholder="01:30:00" />
          </div>
          {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
          <button className="btn-brand" disabled={busy} onClick={() => void submit()}>
            Add to my races
          </button>
        </div>
      </div>
    </div>
  );
}

function MyRacesTab() {
  const navigate = useNavigate();
  const { data, isLoading } = useMyRaces();
  const invalidate = useInvalidateAll();
  const [resultTarget, setResultTarget] = useState<MyRaceView | null>(null);
  const [busy, setBusy] = useState(false);

  if (isLoading) return <Spinner label="Loading your races…" />;
  if (!data || data.length === 0) {
    return <p className="card text-sm text-muted">Nothing yet — add a race from the Upcoming tab.</p>;
  }

  const runSimulation = async (race: MyRaceView) => {
    setBusy(true);
    try {
      const workout = await api.workout.generate({
        type: 'FULL_SIMULATION',
        division: race.userRace.division,
        stationOrders: [],
        excludedExerciseIds: [],
      });
      navigate(`/workout/preview/${workout.id}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-3">
      {data.map((race) => (
        <div key={race.userRace.id} className="card overflow-hidden !p-0">
          <div className="relative h-28 w-full">
            {race.event.imageUrl ? (
              <img src={race.event.imageUrl} alt="" className="h-full w-full object-cover" loading="lazy" />
            ) : (
              <div className="surface-brand h-full w-full" />
            )}
            <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/25 to-transparent" />
            <div className="absolute right-3 top-3">
              <StatusBadge status={race.userRace.status} />
            </div>
            <div className="absolute inset-x-0 bottom-0 px-4 pb-2.5 text-white">
              <p className="display text-2xl leading-tight">{race.event.name}</p>
              <p className="text-xs font-bold opacity-90">
                {formatDay(race.event.startsAt)} ·{' '}
                {race.userRace.division.replaceAll('_', ' ').toLowerCase()}
              </p>
            </div>
          </div>
          <div className="p-4 pt-1">

          {race.userRace.status === 'TRAINING' ? (
            <>
              <div className="mt-3 grid grid-cols-3 gap-2 text-center text-sm">
                <div className="rounded-xl bg-surface-raised p-2">
                  <p className="label !mb-0">Days out</p>
                  <p className="display text-xl">{Math.max(0, race.daysToRace)}</p>
                </div>
                <div className="rounded-xl bg-surface-raised p-2">
                  <p className="label !mb-0">Goal</p>
                  <p className="display text-xl">
                    {race.userRace.goalSec ? formatDuration(race.userRace.goalSec) : '—'}
                  </p>
                </div>
                <div className="rounded-xl bg-surface-raised p-2">
                  <p className="label !mb-0">Prediction</p>
                  <p className="display text-xl">
                    {race.predictionSec ? formatDuration(race.predictionSec) : '—'}
                  </p>
                </div>
              </div>
              <div className="mt-2">
                <div className="flex items-center justify-between text-xs font-bold text-muted">
                  <span>Race readiness</span>
                  <span>{race.readinessScore}%</span>
                </div>
                <div className="mt-1 h-2 overflow-hidden rounded-full bg-surface-raised">
                  <div className="h-full rounded-full bg-brand" style={{ width: `${race.readinessScore}%` }} />
                </div>
                <p className="mt-1 text-xs text-muted">
                  {race.simulationCount} full simulation{race.simulationCount === 1 ? '' : 's'} completed
                </p>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-2">
                <button className="btn-brand !py-2 text-sm" disabled={busy} onClick={() => void runSimulation(race)}>
                  Run simulation
                </button>
                <button className="btn-ghost !py-2 text-sm" onClick={() => setResultTarget(race)}>
                  Enter result
                </button>
              </div>
            </>
          ) : race.analysis ? (
            <div className="mt-3 rounded-xl bg-surface-raised p-3 text-sm">
              <p className="flex items-center gap-1.5 font-black">
                <Trophy size={15} className="text-brand" />
                Finished in {formatDuration(race.userRace.resultSec!)}
              </p>
              <p className={`mt-1 font-bold ${race.analysis.achievedGoal ? 'text-ok' : 'text-warn'}`}>
                {race.analysis.vsGoalSec !== null
                  ? race.analysis.achievedGoal
                    ? `Goal beaten by ${formatDuration(Math.abs(race.analysis.vsGoalSec))}`
                    : `${formatDuration(race.analysis.vsGoalSec)} over goal`
                  : 'No goal was set'}
              </p>
              {race.analysis.vsPredictionSec !== null ? (
                <p className="text-muted">
                  {race.analysis.vsPredictionSec <= 0
                    ? `${formatDuration(Math.abs(race.analysis.vsPredictionSec))} faster than predicted`
                    : `${formatDuration(race.analysis.vsPredictionSec)} slower than predicted`}
                </p>
              ) : null}
            </div>
          ) : null}
          </div>
        </div>
      ))}

      {resultTarget ? (
        <ResultSheet
          race={resultTarget}
          onClose={() => setResultTarget(null)}
          onDone={() => {
            setResultTarget(null);
            invalidate();
          }}
        />
      ) : null}
    </div>
  );
}

function ResultSheet({
  race,
  onClose,
  onDone,
}: {
  race: MyRaceView;
  onClose: () => void;
  onDone: () => void;
}) {
  const [result, setResult] = useState('01:32:00');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    const m = result.trim().match(/^(\d{1,2}):(\d{2}):(\d{2})$/);
    if (!m) {
      setError('Use hh:mm:ss.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await api.races.update(race.userRace.id, {
        resultSec: Number(m[1]) * 3600 + Number(m[2]) * 60 + Number(m[3]),
      });
      onDone();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Could not save result.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/50" onClick={onClose}>
      <div className="w-full max-w-md rounded-t-3xl bg-surface p-5" onClick={(e) => e.stopPropagation()}>
        <h2 className="display mb-1 text-xl">Race result</h2>
        <p className="mb-3 text-sm text-muted">{race.event.name}</p>
        <label className="label">Finish time (hh:mm:ss)</label>
        <input className="input" value={result} onChange={(e) => setResult(e.target.value)} />
        {error ? <p className="mt-2 text-sm font-bold text-danger">{error}</p> : null}
        <button className="btn-brand mt-3 w-full" disabled={busy} onClick={() => void submit()}>
          Save result
        </button>
      </div>
    </div>
  );
}
