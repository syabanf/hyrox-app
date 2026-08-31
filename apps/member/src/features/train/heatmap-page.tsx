import type { ActivityCardView } from '@hyrox/contracts';
import { Spinner, formatDayTime, formatDistanceM, formatDuration } from '@hyrox/ui';
import { ArrowLeft, Bike, Footprints, PersonStanding } from 'lucide-react';
import { Link, useNavigate } from 'react-router';
import { GeoMap } from '../../components/geo-map';
import { RouteMap } from '../../components/route-map';
import { useHeatmap, useMyActivities, useUnits } from '../../lib/athlete-queries';

const SPORT_META: Record<string, { label: string; icon: typeof Footprints }> = {
  RUN: { label: 'Run', icon: Footprints },
  RIDE: { label: 'Ride', icon: Bike },
  WALK: { label: 'Walk', icon: PersonStanding },
};

export function HeatmapPage() {
  const navigate = useNavigate();
  const units = useUnits();
  const { data, isLoading } = useHeatmap();
  const { data: mine } = useMyActivities();

  // Only GPS activities paint the map — the same set feeds the stats below.
  const gps = (mine ?? []).filter((a) => a.thumbnail.length > 1);
  const totalM = gps.reduce((sum, a) => sum + a.distanceM, 0);
  const totalSec = gps.reduce((sum, a) => sum + a.movingSec, 0);
  const bySport = Object.entries(SPORT_META)
    .map(([type, meta]) => {
      const list = gps.filter((a) => a.type === type);
      return {
        type,
        ...meta,
        count: list.length,
        distanceM: list.reduce((sum, a) => sum + a.distanceM, 0),
      };
    })
    .filter((row) => row.count > 0);

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <div>
        <h1 className="display text-3xl">Personal heatmap</h1>
        <p className="text-sm text-muted">Every GPS track you have recorded, on one map.</p>
      </div>
      {isLoading || !data ? (
        <Spinner label="Painting your tracks…" />
      ) : data.tracks.length === 0 ? (
        <p className="card text-sm text-muted">No GPS activities yet.</p>
      ) : (
        <GeoMap
          tracks={data.tracks.map((points) => ({
            points,
            opacity: 0.4,
            width: 3.5,
            markers: false,
          }))}
          height={380}
        />
      )}

      {/* Coverage totals on the shared black card */}
      <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
        <div className="relative grid grid-cols-3 gap-3 text-center text-sm">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Tracks</p>
            <p className="display mt-1 text-3xl">{gps.length}</p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Painted</p>
            <p className="display mt-1 text-3xl">{(totalM / 1000).toFixed(0)} km</p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Moving</p>
            <p className="display mt-1 text-3xl">{formatDuration(totalSec)}</p>
          </div>
        </div>
      </div>

      {/* Split per sport */}
      {bySport.length > 0 ? (
        <section>
          <p className="mb-2 px-1 text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
            By sport
          </p>
          <div className="card flex flex-col gap-3">
            {bySport.map(({ type, label, icon: Icon, count, distanceM }) => (
              <div key={type} className="flex items-center gap-3">
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[#1b1b1f] text-white">
                  <Icon size={16} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline justify-between text-sm">
                    <span className="font-extrabold">{label}</span>
                    <span className="font-bold text-muted">
                      {count} · {formatDistanceM(distanceM, units)}
                    </span>
                  </div>
                  <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-surface-raised">
                    <div
                      className="h-full rounded-full bg-brand"
                      style={{ width: `${totalM > 0 ? (distanceM / totalM) * 100 : 0}%` }}
                    />
                  </div>
                </div>
              </div>
            ))}
          </div>
        </section>
      ) : null}

      {/* The tracks behind the paint */}
      {gps.length > 0 ? (
        <section>
          <p className="mb-2 px-1 text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
            Recent tracks
          </p>
          <div className="flex flex-col gap-2">
            {gps.slice(0, 5).map((a: ActivityCardView) => (
              <Link key={a.id} to={`/train/activities/${a.id}`} className="card flex items-center gap-3 !p-3">
                <div className="h-14 w-20 shrink-0 overflow-hidden rounded-xl">
                  <RouteMap points={a.thumbnail} height={56} />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-extrabold">{a.title}</p>
                  <p className="text-xs text-muted">
                    {formatDayTime(a.startedAt)} · {formatDistanceM(a.distanceM, units)} ·{' '}
                    {formatDuration(a.movingSec)}
                  </p>
                </div>
              </Link>
            ))}
          </div>
        </section>
      ) : null}
    </div>
  );
}
