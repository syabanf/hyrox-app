import { ApiError } from '@hyrox/api-client';
import type { ActivityType, ActivityVisibility, Division, Route, TrackPoint } from '@hyrox/domain';
import { haversineM } from '@hyrox/domain';
import { formatDistanceM, formatDuration, formatPace } from '@hyrox/ui';
import {
  Bike,
  Bluetooth,
  ChevronRight,
  Dumbbell,
  Flame,
  Footprints,
  ImagePlus,
  MapPinned,
  Pause,
  PersonStanding,
  Play,
  Radio,
  Settings,
  Square,
  Timer,
  Volume2,
  X,
} from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { GeoMap } from '../../components/geo-map';
import { RouteMap } from '../../components/route-map';
import { api } from '../../lib/api';
import { useAthleteStats, useUnits } from '../../lib/athlete-queries';
import { useT } from '../../lib/i18n';
import { resizeImageToDataUrl } from '../../lib/image';
import { useInvalidateAll } from '../../lib/queries';
import { TrainTabs } from './train-tabs';

type Phase = 'idle' | 'recording' | 'paused' | 'saving';

const DIVISIONS: { id: Division; label: string }[] = [
  { id: 'MEN_OPEN', label: 'Men Open' },
  { id: 'WOMEN_OPEN', label: 'Women Open' },
  { id: 'MEN_PRO', label: 'Men Pro' },
  { id: 'WOMEN_PRO', label: 'Women Pro' },
];

const SIM_SPEED: Record<ActivityType, number> = { RUN: 3.2, RIDE: 7.5, WALK: 1.5, WORKOUT: 0 };

/** Recording preferences survive reloads (localStorage, per browser). */
const prefGet = (key: string): boolean => {
  try {
    return localStorage.getItem(`hyrox.rec.${key}`) === '1';
  } catch {
    return false;
  }
};
const prefSet = (key: string, value: boolean) => {
  try {
    localStorage.setItem(`hyrox.rec.${key}`, value ? '1' : '0');
  } catch {
    /* private mode */
  }
};

function Toggle({ on, onChange }: { on: boolean; onChange: (next: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      onClick={() => onChange(!on)}
      className={`h-7 w-12 shrink-0 rounded-full p-1 transition ${on ? 'bg-brand' : 'bg-line'}`}
    >
      <span
        className={`block h-5 w-5 rounded-full bg-white shadow transition-transform ${on ? 'translate-x-5' : ''}`}
      />
    </button>
  );
}

function OptionRow({
  icon: Icon,
  label,
  hint,
  right,
  onClick,
}: {
  icon: typeof Timer;
  label: string;
  hint?: string;
  right: React.ReactNode;
  onClick?: () => void;
}) {
  const Tag = onClick ? 'button' : 'div';
  return (
    <Tag
      onClick={onClick}
      className="flex w-full items-center gap-3 px-2 py-3 text-left active:bg-surface-raised"
    >
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[#1b1b1f] text-white">
        <Icon size={16} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-bold leading-tight">{label}</span>
        {hint ? <span className="block truncate text-xs text-muted">{hint}</span> : null}
      </span>
      {right}
    </Tag>
  );
}

const WORKOUT_FOCUS = [
  'Back',
  'Chest',
  'Legs',
  'Shoulders',
  'Arms',
  'Core',
  'Glutes',
  'Full body',
] as const;

const TYPE_TILES: {
  id: ActivityType | 'HYROX';
  label: string;
  hint: string;
  icon: typeof Footprints;
}[] = [
  { id: 'RUN', label: 'Run', hint: 'GPS tracked', icon: Footprints },
  { id: 'RIDE', label: 'Ride', hint: 'GPS tracked', icon: Bike },
  { id: 'WALK', label: 'Walk', hint: 'GPS tracked', icon: PersonStanding },
  { id: 'WORKOUT', label: 'Workout', hint: 'Timer only', icon: Dumbbell },
  { id: 'HYROX', label: 'HYROX Sim', hint: 'Guided race', icon: Flame },
];
const M_PER_DEG_LAT = 111_320;

function defaultTitle(type: ActivityType): string {
  const hour = new Date().getHours();
  const daypart = hour < 11 ? 'Morning' : hour < 15 ? 'Lunch' : hour < 19 ? 'Evening' : 'Night';
  const noun = { RUN: 'Run', RIDE: 'Ride', WALK: 'Walk', WORKOUT: 'Workout' }[type];
  return `${daypart} ${noun}`;
}

export function RecordPage() {
  const navigate = useNavigate();
  const t = useT();
  const units = useUnits();
  const invalidate = useInvalidateAll();
  const { data: stats } = useAthleteStats();
  const [searchParams] = useSearchParams();
  const routeId = searchParams.get('route');
  const [followRoute, setFollowRoute] = useState<Route | null>(null);
  useEffect(() => {
    if (!routeId) return;
    void api.athlete.route(routeId).then(setFollowRoute).catch(() => setFollowRoute(null));
  }, [routeId]);

  const [phase, setPhase] = useState<Phase>('idle');
  const [type, setType] = useState<ActivityType | 'HYROX'>('RUN');
  const [division, setDivision] = useState<Division>('MEN_OPEN');
  const [simBusy, setSimBusy] = useState(false);
  const [simError, setSimError] = useState('');
  const [useDemoGps, setUseDemoGps] = useState(true);
  const [trackLaps, setTrackLaps] = useState(() => prefGet('laps'));
  const [audioCues, setAudioCues] = useState(() => prefGet('cues'));
  const [liveShare, setLiveShare] = useState(false);
  const [liveCopied, setLiveCopied] = useState(false);
  const [sensorOpen, setSensorOpen] = useState(false);
  const [laps, setLaps] = useState<number[]>([]);
  const [focus, setFocus] = useState<string[]>([]);
  const [points, setPoints] = useState<TrackPoint[]>([]);
  const [elapsedSec, setElapsedSec] = useState(0);
  const [distanceM, setDistanceM] = useState(0);
  const [gpsError, setGpsError] = useState('');

  const startTsRef = useRef(0);
  const pausedRef = useRef(false);
  const simStateRef = useRef({
    lat: -6.21,
    lng: 106.82,
    heading: 0, // north, along the segment corridor
    ele: 20,
    routeIdx: 0,
  });
  const watchIdRef = useRef<number | null>(null);
  const timersRef = useRef<number[]>([]);
  const lastPointRef = useRef<TrackPoint | null>(null);

  const appendPoint = useCallback((lat: number, lng: number, ele?: number) => {
    if (pausedRef.current) return;
    const point: TrackPoint = { t: Date.now() - startTsRef.current, lat, lng, ...(ele !== undefined ? { ele } : {}) };
    const last = lastPointRef.current;
    lastPointRef.current = point;
    if (last) {
      const delta = haversineM(last, point);
      setDistanceM((d) => d + delta);
    }
    setPoints((prev) => [...prev, point]);
  }, []);

  const stopSources = useCallback(() => {
    for (const id of timersRef.current) clearInterval(id);
    timersRef.current = [];
    if (watchIdRef.current !== null) {
      navigator.geolocation?.clearWatch(watchIdRef.current);
      watchIdRef.current = null;
    }
  }, []);
  useEffect(() => stopSources, [stopSources]);

  const start = () => {
    setPoints([]);
    setDistanceM(0);
    setElapsedSec(0);
    setLaps([]);
    lastKmRef.current = 0;
    setGpsError('');
    startTsRef.current = Date.now();
    pausedRef.current = false;
    lastPointRef.current = null;
    const startPoint = followRoute?.points[0];
    simStateRef.current = {
      lat: startPoint?.lat ?? -6.21,
      lng: startPoint?.lng ?? 106.82,
      heading: 0,
      ele: 20,
      routeIdx: 0,
    };
    setPhase('recording');

    timersRef.current.push(
      window.setInterval(() => {
        if (!pausedRef.current) setElapsedSec((s) => s + 1);
      }, 1000),
    );

    if (type === 'WORKOUT' || type === 'HYROX') return; // timer only / guided

    if (!useDemoGps && navigator.geolocation) {
      watchIdRef.current = navigator.geolocation.watchPosition(
        (pos) =>
          appendPoint(
            pos.coords.latitude,
            pos.coords.longitude,
            typeof pos.coords.altitude === 'number' ? pos.coords.altitude : undefined,
          ),
        (err) => setGpsError(`GPS: ${err.message} — switch to Demo GPS to keep going.`),
        { enableHighAccuracy: true, maximumAge: 1000 },
      );
    } else {
      timersRef.current.push(
        window.setInterval(() => {
          if (pausedRef.current) return;
          const s = simStateRef.current;
          const v = SIM_SPEED[type as ActivityType] * (0.9 + Math.random() * 0.2);
          const route = followRoute;
          if (route && s.routeIdx < route.points.length - 1) {
            // Follow the saved route: head toward the next route point.
            let remaining = v;
            while (remaining > 0 && s.routeIdx < route.points.length - 1) {
              const target = route.points[s.routeIdx + 1]!;
              const dist = haversineM({ lat: s.lat, lng: s.lng }, target);
              if (dist <= remaining) {
                s.lat = target.lat;
                s.lng = target.lng;
                s.routeIdx += 1;
                remaining -= dist;
              } else {
                const frac = remaining / dist;
                s.lat += (target.lat - s.lat) * frac;
                s.lng += (target.lng - s.lng) * frac;
                remaining = 0;
              }
            }
          } else {
            // Default demo run: follow the segment corridor north, drifting a little.
            s.heading += (Math.random() - 0.5) * 0.15;
            s.heading = Math.max(-0.5, Math.min(0.5, s.heading));
            s.lat += (v * Math.cos(s.heading)) / M_PER_DEG_LAT;
            s.lng += (v * Math.sin(s.heading)) / (M_PER_DEG_LAT * Math.cos((s.lat * Math.PI) / 180));
          }
          s.ele += (Math.random() - 0.45) * 0.6;
          appendPoint(s.lat, s.lng, s.ele);
        }, 1000),
      );
    }
  };

  // Audio cue at every completed kilometre (skipped when Reduce Motion users
  // disable it or speech synthesis is unavailable).
  const lastKmRef = useRef(0);
  useEffect(() => {
    const km = Math.floor(distanceM / 1000);
    if (!audioCues || phase !== 'recording' || km <= lastKmRef.current) return;
    lastKmRef.current = km;
    try {
      const pace = distanceM > 0 ? (elapsedSec / distanceM) * 1000 : 0;
      const mm = Math.floor(pace / 60);
      const ss = Math.round(pace % 60);
      const u = new SpeechSynthesisUtterance(
        `${km} kilometer${km > 1 ? 's' : ''}. Average pace ${mm} ${ss < 10 ? 'oh ' : ''}${ss} per kilometer.`,
      );
      window.speechSynthesis?.speak(u);
    } catch {
      /* no speech support */
    }
  }, [distanceM, audioCues, phase, elapsedSec]);

  const togglePause = () => {
    pausedRef.current = !pausedRef.current;
    setPhase(pausedRef.current ? 'paused' : 'recording');
  };

  const finish = () => {
    pausedRef.current = true;
    stopSources();
    setPhase('saving');
  };

  const discard = () => {
    stopSources();
    setPhase('idle');
    setPoints([]);
    setDistanceM(0);
    setElapsedSec(0);
  };

  // Generate a full HYROX simulation and drop straight into the guided
  // station-by-station session (which saves to the feed as a WORKOUT).
  const startHyroxSim = async () => {
    setSimBusy(true);
    setSimError('');
    try {
      const workout = await api.workout.generate({
        type: 'FULL_SIMULATION',
        division,
        stationOrders: [],
        excludedExerciseIds: [],
      });
      const session = await api.workout.start(workout.id);
      navigate(`/workout/active/${session.session.id}`);
    } catch (e) {
      setSimError(e instanceof ApiError ? e.message : 'Could not start the simulation.');
      setSimBusy(false);
    }
  };

  const avgPace = distanceM > 50 ? (elapsedSec / distanceM) * 1000 : null;

  return (
    <div className="flex flex-col gap-5">
      <h1 className="display text-3xl">{t('Record')}</h1>
      <TrainTabs />

      {followRoute ? (
        <p className="rounded-xl bg-brand/10 px-3 py-2 text-sm font-bold text-brand">
          {t('Routes')}: {followRoute.name} ({formatDistanceM(followRoute.distanceM, units)}) — Demo
          GPS follows this route.
        </p>
      ) : null}

      {phase === 'idle' ? (
        <>
          <div>
            <p className="label">Activity type</p>
            <div className="-mx-5 flex snap-x gap-2.5 overflow-x-auto px-5 pb-1">
              {TYPE_TILES.map(({ id, label, hint, icon: Icon }) => {
                const selected = type === id;
                return (
                  <button
                    key={id}
                    onClick={() => setType(id)}
                    className={`flex min-w-[104px] shrink-0 snap-start flex-col items-start gap-2.5 rounded-2xl p-3.5 text-left transition active:scale-[0.97] ${
                      selected
                        ? 'surface-ink text-white shadow-[0_10px_26px_rgb(13_13_16/0.3)]'
                        : 'card !p-3.5'
                    }`}
                  >
                    <span
                      className={`flex h-9 w-9 items-center justify-center rounded-full ${
                        selected ? 'surface-brand text-white' : 'bg-[#1b1b1f] text-white'
                      }`}
                    >
                      <Icon size={17} strokeWidth={2.2} />
                    </span>
                    <span>
                      <span className="block text-sm font-extrabold leading-tight">{label}</span>
                      <span className={`block text-[11px] font-semibold ${selected ? 'text-white/55' : 'text-muted'}`}>
                        {hint}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
          {type === 'HYROX' ? null : type !== 'WORKOUT' ? (
            <label className="card flex items-center justify-between text-sm font-bold">
              Demo GPS (simulated route)
              <input
                type="checkbox"
                checked={useDemoGps}
                onChange={(e) => setUseDemoGps(e.target.checked)}
                className="h-5 w-5 accent-[var(--color-brand)]"
              />
            </label>
          ) : (
            <div className="card">
              <p className="label">What are you training?</p>
              <div className="flex flex-wrap gap-1.5">
                {WORKOUT_FOCUS.map((f) => {
                  const on = focus.includes(f);
                  return (
                    <button
                      key={f}
                      onClick={() =>
                        setFocus((prev) => (on ? prev.filter((x) => x !== f) : [...prev, f]))
                      }
                      className={`rounded-full px-3.5 py-2 text-xs font-bold transition ${
                        on ? 'bg-[#1b1b1f] text-white' : 'bg-surface-raised text-muted'
                      }`}
                    >
                      {f}
                    </button>
                  );
                })}
              </div>
              <p className="mt-2.5 text-xs text-muted">Timer only — no GPS. Focus lands in the title.</p>
            </div>
          )}
          {type !== 'HYROX' ? (
            <button onClick={start} className="btn-brand flex items-center justify-center gap-2 !py-5 text-lg">
              <Play size={22} fill="currentColor" /> {t('Start')}
            </button>
          ) : (
          <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
            <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
            <div className="relative">
              <p className="flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
                <Flame size={13} className="text-[#ff4348]" /> {t('HYROX simulation')}
              </p>
              <p className="display mt-1 text-2xl leading-tight">8 runs. 8 stations. Race pace.</p>
              <p className="mt-1.5 text-sm text-white/60">
                {t('Guided station by station with your division loads — timed like race day.')}
              </p>
              <div className="mt-4 grid grid-cols-2 gap-2">
                {DIVISIONS.map((d) => (
                  <button
                    key={d.id}
                    onClick={() => setDivision(d.id)}
                    className={`rounded-xl px-3 py-2 text-xs font-black uppercase transition ${
                      division === d.id
                        ? 'bg-white text-ink'
                        : 'bg-white/10 text-white/60 hover:bg-white/15'
                    }`}
                  >
                    {d.label}
                  </button>
                ))}
              </div>
              {simError ? <p className="mt-3 text-sm font-bold text-[#ff4348]">{simError}</p> : null}
              <button
                onClick={() => void startHyroxSim()}
                disabled={simBusy}
                className="btn-brand mt-4 flex w-full items-center justify-center gap-2 disabled:opacity-40"
              >
                <Play size={18} fill="currentColor" />
                {simBusy ? t('Preparing…') : t('Start simulation')}
              </button>
              <p className="mt-2.5 text-center text-xs text-white/40">
                {t('Finishing saves it to your training feed and race readiness.')}
              </p>
            </div>
          </div>
          )}

          {type !== 'HYROX' ? (
            <div className="card divide-y divide-line !p-2">
              {type !== 'WORKOUT' ? (
                <OptionRow
                  icon={MapPinned}
                  label="Follow a route"
                  hint={followRoute ? followRoute.name : 'Pick a saved route to guide the demo GPS'}
                  right={
                    <span className="flex items-center gap-1 text-xs font-bold text-muted">
                      {followRoute ? 'On' : 'Off'} <ChevronRight size={15} />
                    </span>
                  }
                  onClick={() => navigate('/train/explore')}
                />
              ) : null}
              <OptionRow
                icon={Timer}
                label="Track laps"
                hint="Adds a Lap button while recording"
                right={<Toggle on={trackLaps} onChange={(v) => { setTrackLaps(v); prefSet('laps', v); }} />}
              />
              {type !== 'WORKOUT' ? (
                <OptionRow
                  icon={Volume2}
                  label="Audio cues"
                  hint="Spoken split every kilometre"
                  right={<Toggle on={audioCues} onChange={(v) => { setAudioCues(v); prefSet('cues', v); }} />}
                />
              ) : null}
              <OptionRow
                icon={Radio}
                label="Share live location"
                hint={liveCopied ? 'Demo link copied — anyone with it can watch' : 'Send friends a live beacon link'}
                right={
                  <Toggle
                    on={liveShare}
                    onChange={(v) => {
                      setLiveShare(v);
                      if (v) {
                        void navigator.clipboard
                          ?.writeText(`${location.origin}/beacon/demo-${Date.now().toString(36)}`)
                          .catch(() => {});
                        setLiveCopied(true);
                      } else {
                        setLiveCopied(false);
                      }
                    }}
                  />
                }
              />
              <OptionRow
                icon={Bluetooth}
                label="Add a sensor"
                hint="Heart rate, cadence"
                right={<ChevronRight size={15} className="text-muted" />}
                onClick={() => setSensorOpen(true)}
              />
              <OptionRow
                icon={Settings}
                label="Settings"
                hint="Units, reminders, language"
                right={<ChevronRight size={15} className="text-muted" />}
                onClick={() => navigate('/profile/settings')}
              />
            </div>
          ) : null}
        </>
      ) : null}

      {sensorOpen ? <SensorSheet onClose={() => setSensorOpen(false)} /> : null}

      {phase === 'recording' || phase === 'paused' ? (
        <>
          <div className="card flex flex-col items-center gap-1 !py-8">
            <p className="label !mb-0">{t('Time')}</p>
            <p className="display text-7xl leading-none">{formatDuration(elapsedSec)}</p>
            {type !== 'WORKOUT' ? (
              <div className="mt-4 flex gap-8 text-center">
                <div>
                  <p className="label !mb-0">{t('Distance')}</p>
                  <p className="display text-3xl">{formatDistanceM(distanceM, units)}</p>
                </div>
                <div>
                  <p className="label !mb-0">Avg pace</p>
                  <p className="display text-3xl">{formatPace(avgPace, units)}</p>
                </div>
              </div>
            ) : null}
            {phase === 'paused' ? (
              <p className="mt-3 text-xs font-black uppercase tracking-widest text-warn">Paused</p>
            ) : null}
          </div>
          {gpsError ? <p className="text-sm font-bold text-danger">{gpsError}</p> : null}
          {trackLaps ? (
            <div className="card flex items-center gap-3 !py-3">
              <button
                onClick={() => setLaps((prev) => [...prev, elapsedSec])}
                disabled={phase === 'paused'}
                className="btn-ghost shrink-0 !px-5 !py-2.5 text-sm disabled:opacity-40"
              >
                Lap {laps.length + 1}
              </button>
              <div className="flex min-w-0 flex-1 gap-1.5 overflow-x-auto">
                {laps.map((at, i) => (
                  <span key={i} className="chip shrink-0 bg-surface-raised text-muted">
                    {i + 1} · {formatDuration(at - (laps[i - 1] ?? 0))}
                  </span>
                ))}
                {laps.length === 0 ? (
                  <span className="text-xs text-muted">Tap to mark a lap.</span>
                ) : null}
              </div>
            </div>
          ) : null}
          {points.length > 1 ? <RouteMap points={points} height={150} /> : null}
          <div className="grid grid-cols-2 gap-3">
            <button onClick={togglePause} className="btn-ghost flex items-center justify-center gap-2">
              {phase === 'paused' ? <Play size={18} /> : <Pause size={18} />}
              {phase === 'paused' ? t('Resume') : t('Pause')}
            </button>
            <button onClick={finish} className="btn-brand flex items-center justify-center gap-2">
              <Square size={16} fill="currentColor" /> {t('Finish')}
            </button>
          </div>
        </>
      ) : null}

      {phase === 'saving' ? (
        <SaveForm
          type={type === 'HYROX' ? 'WORKOUT' : type}
          focus={focus}
          laps={laps}
          points={points}
          elapsedSec={elapsedSec}
          distanceM={distanceM}
          gear={(stats?.gear ?? []).filter((g) => !g.retired)}
          onDiscard={discard}
          onSaved={(id) => {
            invalidate();
            navigate(`/train/activities/${id}`);
          }}
        />
      ) : null}
    </div>
  );
}

function SaveForm({
  type,
  focus,
  laps,
  points,
  elapsedSec,
  distanceM,
  gear,
  onDiscard,
  onSaved,
}: {
  type: ActivityType;
  focus: string[];
  laps: number[];
  points: TrackPoint[];
  elapsedSec: number;
  distanceM: number;
  gear: { id: string; name: string; kind: string }[];
  onDiscard: () => void;
  onSaved: (activityId: string) => void;
}) {
  const t = useT();
  const units = useUnits();
  const [title, setTitle] = useState(() =>
    type === 'WORKOUT' && focus.length > 0
      ? `${focus.slice(0, 2).join(' & ')} Workout`
      : defaultTitle(type),
  );
  const [description, setDescription] = useState(() =>
    [
      focus.length > 0 ? `Focus: ${focus.join(', ')}` : null,
      laps.length > 0
        ? `Laps: ${laps.map((at, i) => `${i + 1}) ${formatDuration(at - (laps[i - 1] ?? 0))}`).join('  ')}`
        : null,
    ]
      .filter(Boolean)
      .join('\n'),
  );
  const [gearId, setGearId] = useState('');
  const [visibility, setVisibility] = useState<ActivityVisibility>('EVERYONE');
  const [photos, setPhotos] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const addPhoto = async (file: File | undefined) => {
    if (!file || photos.length >= 2) return;
    try {
      const dataUrl = await resizeImageToDataUrl(file, 640);
      setPhotos((p) => [...p, dataUrl]);
    } catch {
      setError('Could not read that image.');
    }
  };

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      const saved = await api.athlete.save({
        type,
        title,
        description,
        startedAt: new Date(Date.now() - elapsedSec * 1000).toISOString(),
        points,
        manualElapsedSec: type === 'WORKOUT' ? Math.max(1, elapsedSec) : null,
        gearId: gearId || null,
        visibility,
        photos,
      });
      onSaved(saved.id);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Save failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <div className="card flex gap-6 text-sm">
        <div>
          <p className="label !mb-0">{t('Time')}</p>
          <p className="display text-xl">{formatDuration(elapsedSec)}</p>
        </div>
        {type !== 'WORKOUT' ? (
          <div>
            <p className="label !mb-0">{t('Distance')}</p>
            <p className="display text-xl">{formatDistanceM(distanceM, units)}</p>
          </div>
        ) : null}
      </div>
      {points.length > 1 ? <GeoMap tracks={[{ points }]} height={180} interactive={false} /> : null}
      <div>
        <label className="label">{t('Title')}</label>
        <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <div>
        <label className="label">{t('Description')}</label>
        <textarea
          className="input min-h-20"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="How did it go?"
        />
      </div>
      <div>
        <label className="label">Photos ({photos.length}/2)</label>
        <div className="flex gap-2">
          {photos.map((p, i) => (
            <div key={i} className="relative">
              <img src={p} alt="" className="h-20 w-20 rounded-xl object-cover" />
              <button
                onClick={() => setPhotos((arr) => arr.filter((_, j) => j !== i))}
                className="absolute -right-1.5 -top-1.5 rounded-full bg-ink p-1 text-white"
                aria-label="Remove photo"
              >
                <X size={12} />
              </button>
            </div>
          ))}
          {photos.length < 2 ? (
            <label className="flex h-20 w-20 cursor-pointer items-center justify-center rounded-xl border-2 border-dashed border-line text-muted">
              <ImagePlus size={22} />
              <input
                type="file"
                accept="image/*"
                className="hidden"
                onChange={(e) => void addPhoto(e.target.files?.[0])}
              />
            </label>
          ) : null}
        </div>
      </div>
      {gear.length > 0 && type !== 'WORKOUT' ? (
        <div>
          <label className="label">{t('Gear')}</label>
          <select className="input" value={gearId} onChange={(e) => setGearId(e.target.value)}>
            <option value="">None</option>
            {gear
              .filter((g) => (type === 'RIDE' ? g.kind === 'BIKE' : g.kind === 'SHOES'))
              .map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
          </select>
        </div>
      ) : null}
      <div>
        <label className="label">{t('Who can see this')}</label>
        <div className="grid grid-cols-3 gap-2">
          {(['EVERYONE', 'FOLLOWERS', 'PRIVATE'] as const).map((v) => (
            <button
              key={v}
              onClick={() => setVisibility(v)}
              className={`rounded-xl border px-2 py-2 text-xs font-black uppercase ${
                visibility === v ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
              }`}
            >
              {v.toLowerCase()}
            </button>
          ))}
        </div>
      </div>
      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
      <button className="btn-brand" disabled={busy || title.length < 1} onClick={() => void save()}>
        {t('Save activity')}
      </button>
      <button className="btn-ghost text-danger" onClick={onDiscard}>
        {t('Discard')}
      </button>
    </div>
  );
}


/** Mock sensor pairing — scans, finds nothing (demo build has no Bluetooth). */
function SensorSheet({ onClose }: { onClose: () => void }) {
  const [scanning, setScanning] = useState(true);
  useEffect(() => {
    const id = window.setTimeout(() => setScanning(false), 2200);
    return () => clearTimeout(id);
  }, []);
  return (
    <div className="sheet-backdrop fixed inset-0 z-40 flex items-end justify-center bg-black/50" onClick={onClose}>
      <div className="sheet-panel w-full max-w-md rounded-t-3xl bg-surface p-5 pb-8" onClick={(e) => e.stopPropagation()}>
        <h2 className="display mb-1 text-xl">Add a sensor</h2>
        <p className="mb-4 text-sm text-muted">Heart rate straps, cadence and power meters.</p>
        {scanning ? (
          <div className="flex items-center gap-3 rounded-2xl bg-surface-raised px-4 py-4 text-sm font-bold">
            <span className="h-5 w-5 animate-spin rounded-full border-2 border-line border-t-[#ed1c24]" />
            Scanning for nearby sensors…
          </div>
        ) : (
          <div className="rounded-2xl bg-surface-raised px-4 py-4 text-sm">
            <p className="font-bold">No sensors nearby</p>
            <p className="mt-0.5 text-muted">
              Pairing needs Bluetooth on a real device — this demo build stops here.
            </p>
          </div>
        )}
        <button className="btn-ghost mt-4 w-full" onClick={onClose}>
          Close
        </button>
      </div>
    </div>
  );
}
