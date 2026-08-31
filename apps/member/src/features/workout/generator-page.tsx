import { ApiError } from '@hyrox/api-client';
import type { Division, WorkoutType } from '@hyrox/domain';
import { Spinner, StatusBadge, formatDayTime, formatDuration } from '@hyrox/ui';
import { Dumbbell } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useExerciseLibrary, useWorkoutSessions } from '../../lib/athlete-queries';
import { useT } from '../../lib/i18n';

const TYPE_INFO: { id: WorkoutType; label: string; hint: string }[] = [
  { id: 'FULL_SIMULATION', label: 'Full Simulation', hint: '8× 1 km run + all 8 stations, race order' },
  { id: 'COVERAGE', label: 'Coverage', hint: '600 m runs + your chosen stations' },
  { id: 'QUICK', label: 'Quick', hint: '400 m runs + 4 stations at half volume' },
  { id: 'PRACTICE', label: 'Practice', hint: '3 rounds on a single station' },
];
const DIVISIONS: { id: Division; label: string }[] = [
  { id: 'MEN_OPEN', label: 'Men Open' },
  { id: 'MEN_PRO', label: 'Men Pro' },
  { id: 'WOMEN_OPEN', label: 'Women Open' },
  { id: 'WOMEN_PRO', label: 'Women Pro' },
];

export function WorkoutGeneratorPage() {
  const navigate = useNavigate();
  const t = useT();
  const { data: library } = useExerciseLibrary();
  const { data: sessions, isLoading: historyLoading } = useWorkoutSessions();

  const [type, setType] = useState<WorkoutType>('FULL_SIMULATION');
  const [division, setDivision] = useState<Division>('MEN_OPEN');
  const [stationOrders, setStationOrders] = useState<number[]>([]);
  const [excluded, setExcluded] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const stations = (library?.exercises ?? []).filter((e) => e.hyroxStationOrder !== null);
  const needsStations = type === 'COVERAGE' || type === 'PRACTICE';

  const generate = async () => {
    setBusy(true);
    setError('');
    try {
      const workout = await api.workout.generate({
        type,
        division,
        stationOrders: needsStations ? stationOrders : [],
        excludedExerciseIds: excluded,
      });
      navigate(`/workout/preview/${workout.id}`);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Generation failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-2">
        <Dumbbell size={22} className="text-brand" />
        <h1 className="display text-3xl">{t('Workout generator')}</h1>
      </div>

      <div>
        <p className="label">Workout type</p>
        <div className="flex flex-col gap-2">
          {TYPE_INFO.map((info) => (
            <button
              key={info.id}
              onClick={() => setType(info.id)}
              className={`card flex items-center justify-between text-left !py-3 ${
                type === info.id ? '!border-brand' : ''
              }`}
            >
              <div>
                <p className="font-black">{info.label}</p>
                <p className="text-xs text-muted">{info.hint}</p>
              </div>
              {type === info.id ? <span className="text-brand">●</span> : null}
            </button>
          ))}
        </div>
      </div>

      <div>
        <p className="label">{t('Division')}</p>
        <div className="grid grid-cols-2 gap-2">
          {DIVISIONS.map((d) => (
            <button
              key={d.id}
              onClick={() => setDivision(d.id)}
              className={`rounded-xl border px-3 py-2.5 text-sm font-black uppercase ${
                division === d.id ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
              }`}
            >
              {d.label}
            </button>
          ))}
        </div>
      </div>

      {needsStations ? (
        <div>
          <p className="label">
            Stations ({type === 'PRACTICE' ? 'pick one' : 'pick any — empty = random 4'})
          </p>
          <div className="grid grid-cols-2 gap-2">
            {stations.map((s) => {
              const order = s.hyroxStationOrder!;
              const selected = stationOrders.includes(order);
              return (
                <button
                  key={s.id}
                  onClick={() =>
                    setStationOrders((prev) =>
                      type === 'PRACTICE'
                        ? [order]
                        : selected
                          ? prev.filter((o) => o !== order)
                          : [...prev, order],
                    )
                  }
                  className={`rounded-xl border px-3 py-2 text-left text-sm font-bold ${
                    selected ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                  }`}
                >
                  {order}. {s.name}
                </button>
              );
            })}
          </div>
        </div>
      ) : null}

      <div>
        <p className="label">Exclude exercises (no equipment? we substitute)</p>
        <div className="flex flex-wrap gap-2">
          {stations.map((s) => {
            const isExcluded = excluded.includes(s.id);
            return (
              <button
                key={s.id}
                onClick={() =>
                  setExcluded((prev) =>
                    isExcluded ? prev.filter((id) => id !== s.id) : [...prev, s.id],
                  )
                }
                className={`rounded-full px-3 py-1.5 text-xs font-bold ${
                  isExcluded ? 'bg-danger/15 text-danger line-through' : 'bg-surface text-muted'
                }`}
              >
                {s.name}
              </button>
            );
          })}
        </div>
      </div>

      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
      <button className="btn-brand" disabled={busy || (type === 'PRACTICE' && stationOrders.length === 0)} onClick={() => void generate()}>
        {t('Generate')}
      </button>

      <section>
        <h2 className="display mb-2 text-xl">History</h2>
        {historyLoading ? (
          <Spinner label="Loading history…" />
        ) : !sessions || sessions.length === 0 ? (
          <p className="card text-sm text-muted">No workouts yet.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {sessions.map((item) => (
              <div key={item.session.id} className="card flex items-center justify-between !py-3">
                <div>
                  <p className="text-sm font-black">
                    {TYPE_INFO.find((x) => x.id === item.workoutType)?.label ?? item.workoutType}
                  </p>
                  <p className="text-xs text-muted">
                    {formatDayTime(item.session.createdAt)} ·{' '}
                    {item.division.replaceAll('_', ' ').toLowerCase()}
                  </p>
                </div>
                <div className="text-right">
                  <StatusBadge status={item.session.status} />
                  <p className="mt-0.5 text-xs font-bold text-muted">
                    {formatDuration(item.activeSec)} · {item.completionPct}%
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
