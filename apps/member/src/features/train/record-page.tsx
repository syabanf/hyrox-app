import { ApiError } from '@hyrox/api-client';
import type { ActivityType, ActivityVisibility, Route, TrackPoint } from '@hyrox/domain';
import { haversineM } from '@hyrox/domain';
import { formatDistanceM, formatDuration, formatPace } from '@hyrox/ui';
import { ImagePlus, Pause, Play, Square, X } from 'lucide-react';
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

const SIM_SPEED: Record<ActivityType, number> = { RUN: 3.2, RIDE: 7.5, WALK: 1.5, WORKOUT: 0 };
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
  const [type, setType] = useState<ActivityType>('RUN');
  const [useDemoGps, setUseDemoGps] = useState(true);
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

    if (type === 'WORKOUT') return; // timer only

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
          const v = SIM_SPEED[type] * (0.9 + Math.random() * 0.2);
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

  const avgPace = distanceM > 50 ? (elapsedSec / distanceM) * 1000 : null;

  return (
    <div className="flex flex-col gap-4">
      <h1 className="display text-2xl">{t('Record')}</h1>
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
            <div className="grid grid-cols-4 gap-2">
              {(['RUN', 'RIDE', 'WALK', 'WORKOUT'] as const).map((option) => (
                <button
                  key={option}
                  onClick={() => setType(option)}
                  className={`rounded-xl border px-2 py-2.5 text-xs font-black uppercase ${
                    type === option ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                  }`}
                >
                  {option}
                </button>
              ))}
            </div>
          </div>
          {type !== 'WORKOUT' ? (
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
            <p className="card text-sm text-muted">Workouts record time only — no GPS.</p>
          )}
          <button onClick={start} className="btn-brand flex items-center justify-center gap-2 !py-5 text-lg">
            <Play size={22} fill="currentColor" /> {t('Start')}
          </button>
        </>
      ) : null}

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
          type={type}
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
  points,
  elapsedSec,
  distanceM,
  gear,
  onDiscard,
  onSaved,
}: {
  type: ActivityType;
  points: TrackPoint[];
  elapsedSec: number;
  distanceM: number;
  gear: { id: string; name: string; kind: string }[];
  onDiscard: () => void;
  onSaved: (activityId: string) => void;
}) {
  const t = useT();
  const units = useUnits();
  const [title, setTitle] = useState(defaultTitle(type));
  const [description, setDescription] = useState('');
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
    <div className="flex flex-col gap-4">
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
