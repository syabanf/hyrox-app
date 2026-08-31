import { ApiError } from '@hyrox/api-client';
import {
  Spinner,
  formatDistanceM,
  formatDuration,
  formatPace,
} from '@hyrox/ui';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router';
import { api } from '../../lib/api';
import { useAthleteStats, useMyActivities, useUnits } from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';
import { ActivityCard } from './feed-page';
import { TrainTabs } from './train-tabs';

export function YouPage() {
  const units = useUnits();
  const invalidate = useInvalidateAll();
  const { data: stats, isLoading } = useAthleteStats();
  const { data: mine } = useMyActivities();
  const [goalDraft, setGoalDraft] = useState<string | null>(null);
  const [gearOpen, setGearOpen] = useState(false);

  if (isLoading || !stats) return <Spinner label="Loading your stats…" />;

  const goalPct =
    stats.goal.targetKm && stats.goal.targetKm > 0
      ? Math.min(1, stats.goal.currentKm / stats.goal.targetKm)
      : 0;
  const maxWeekKm = Math.max(...stats.weekly.map((w) => w.distanceKm), 1);

  const saveGoal = async () => {
    const value = goalDraft === '' ? null : Number(goalDraft);
    await api.athlete.updateSettings({ weeklyGoalKm: value && value > 0 ? value : null });
    setGoalDraft(null);
    invalidate();
  };

  const R = 34;
  const CIRC = 2 * Math.PI * R;

  return (
    <div className="flex flex-col gap-5">
      <h1 className="display text-3xl">You</h1>
      <TrainTabs />

      <div className="card flex items-center gap-5">
        <svg width="88" height="88" viewBox="0 0 88 88" aria-hidden>
          <circle cx="44" cy="44" r={R} fill="none" stroke="var(--color-line)" strokeWidth="8" />
          <circle
            cx="44"
            cy="44"
            r={R}
            fill="none"
            stroke="var(--color-brand)"
            strokeWidth="8"
            strokeLinecap="round"
            strokeDasharray={CIRC}
            strokeDashoffset={CIRC * (1 - goalPct)}
            transform="rotate(-90 44 44)"
          />
          <text x="44" y="49" textAnchor="middle" fontSize="16" fontWeight="800" fill="#191919">
            {Math.round(goalPct * 100)}%
          </text>
        </svg>
        <div className="min-w-0 flex-1">
          <p className="label !mb-0">This week</p>
          <p className="display text-3xl">{stats.thisWeekKm.toFixed(1)} km</p>
          {goalDraft === null ? (
            <button className="text-sm font-bold text-brand" onClick={() => setGoalDraft(String(stats.goal.targetKm ?? ''))}>
              {stats.goal.targetKm ? `Goal: ${stats.goal.targetKm} km · edit` : 'Set a weekly goal'}
            </button>
          ) : (
            <div className="mt-1 flex gap-2">
              <input
                className="input !w-24 !py-1.5"
                value={goalDraft}
                inputMode="decimal"
                onChange={(e) => setGoalDraft(e.target.value)}
                placeholder="km"
              />
              <button className="btn-brand !px-3 !py-1.5 text-xs" onClick={() => void saveGoal()}>
                Save
              </button>
            </div>
          )}
        </div>
        <div className="text-right text-xs text-muted">
          <p>
            <span className="font-black text-ink">{stats.followerCount}</span> followers
          </p>
          <p>
            <span className="font-black text-ink">{stats.followingCount}</span> following
          </p>
        </div>
      </div>

      <Link to="/train/heatmap" className="card flex items-center justify-between !py-3 text-sm font-bold">
        Personal heatmap
        <span className="text-brand">View →</span>
      </Link>

      <div className="card">
        <p className="label">Last 8 weeks</p>
        <div className="flex h-28 gap-1.5">
          {stats.weekly.map((w) => (
            <div key={w.weekStart} className="flex h-full flex-1 flex-col items-center justify-end gap-1">
              <div
                className="w-full rounded-t bg-brand"
                style={{
                  height: `${Math.max(4, (w.distanceKm / maxWeekKm) * 85)}%`,
                  opacity: w.distanceKm ? 1 : 0.2,
                }}
                title={`${w.distanceKm} km`}
              />
              <span className="text-[9px] font-bold text-muted">
                {new Date(w.weekStart).getDate()}/{new Date(w.weekStart).getMonth() + 1}
              </span>
            </div>
          ))}
        </div>
      </div>

      <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
        <div className="relative grid grid-cols-3 gap-3 text-center text-sm">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Activities</p>
            <p className="display mt-1 text-3xl">{stats.totals.activities}</p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Distance</p>
            <p className="display mt-1 text-3xl">{stats.totals.distanceKm.toFixed(0)} km</p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Time</p>
            <p className="display mt-1 text-3xl">{formatDuration(stats.totals.movingSec)}</p>
          </div>
        </div>
      </div>

      <div className="card text-sm">
        <p className="label">Personal records (runs)</p>
        <div className="grid grid-cols-2 gap-y-2">
          <span className="text-muted">Best 1k</span>
          <span className="text-right font-black">{formatPace(stats.prs.best1kPaceSec, units)}</span>
          <span className="text-muted">Best 5k (est.)</span>
          <span className="text-right font-black">{stats.prs.best5kSec ? formatDuration(stats.prs.best5kSec) : '—'}</span>
          <span className="text-muted">Best 10k (est.)</span>
          <span className="text-right font-black">{stats.prs.best10kSec ? formatDuration(stats.prs.best10kSec) : '—'}</span>
          <span className="text-muted">Longest</span>
          <span className="text-right font-black">{formatDistanceM(stats.prs.longestDistanceM, units)}</span>
        </div>
      </div>

      <div className="card text-sm">
        <div className="mb-2 flex items-center justify-between">
          <p className="label !mb-0">Gear</p>
          <button className="flex items-center gap-1 text-sm font-bold text-brand" onClick={() => setGearOpen(true)}>
            <Plus size={14} /> Add
          </button>
        </div>
        {stats.gear.length === 0 ? (
          <p className="text-muted">Track shoe and bike mileage here.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {stats.gear.map((g) => (
              <div key={g.id} className={`flex items-center justify-between ${g.retired ? 'opacity-50' : ''}`}>
                <div>
                  <p className="font-bold">{g.name}</p>
                  <p className="text-xs text-muted">
                    {g.kind === 'SHOES' ? 'Shoes' : 'Bike'}
                    {g.retired ? ' · retired' : ''}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <span className="font-black">{formatDistanceM(g.distanceM, units)}</span>
                  {!g.retired ? (
                    <button
                      className="text-xs font-bold text-muted"
                      onClick={async () => {
                        await api.athlete.updateGear(g.id, { name: g.name, kind: g.kind, retired: true });
                        invalidate();
                      }}
                    >
                      Retire
                    </button>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <section>
        <h2 className="display mb-2 text-xl">My activities</h2>
        <div className="flex flex-col gap-3">
          {(mine ?? []).map((a) => (
            <ActivityCard key={a.id} a={a} />
          ))}
          {(mine ?? []).length === 0 ? (
            <p className="card text-sm text-muted">
              Nothing yet —{' '}
              <Link to="/train/record" className="font-bold text-brand">
                record your first activity
              </Link>
              .
            </p>
          ) : null}
        </div>
      </section>

      {gearOpen ? <AddGearSheet onClose={() => setGearOpen(false)} onDone={() => { setGearOpen(false); invalidate(); }} /> : null}
    </div>
  );
}

function AddGearSheet({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('');
  const [kind, setKind] = useState<'SHOES' | 'BIKE'>('SHOES');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      await api.athlete.createGear({ name, kind, retired: false });
      onDone();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Save failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="sheet-backdrop fixed inset-0 z-40 flex items-end justify-center bg-black/50" onClick={onClose}>
      <div className="sheet-panel w-full max-w-md rounded-t-3xl bg-surface p-5" onClick={(e) => e.stopPropagation()}>
        <h2 className="display mb-4 text-xl">Add gear</h2>
        <div className="flex flex-col gap-3">
          <div>
            <label className="label">Name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Novablast 4" />
          </div>
          <div className="grid grid-cols-2 gap-2">
            {(['SHOES', 'BIKE'] as const).map((k) => (
              <button
                key={k}
                onClick={() => setKind(k)}
                className={`rounded-xl border px-3 py-2.5 text-sm font-black uppercase ${
                  kind === k ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                }`}
              >
                {k}
              </button>
            ))}
          </div>
          {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
          <button className="btn-brand" disabled={busy || name.length < 2} onClick={() => void save()}>
            Add gear
          </button>
        </div>
      </div>
    </div>
  );
}
