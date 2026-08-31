import type { TrackPoint } from '@hyrox/domain';

/**
 * Dependency-free route renderer: projects the GPS polyline into an SVG.
 * (A tile map can replace this later without touching callers.)
 */
export function RouteMap({
  points,
  height = 160,
  className = '',
}: {
  points: TrackPoint[];
  height?: number;
  className?: string;
}) {
  if (points.length < 2) {
    return (
      <div
        className={`flex items-center justify-center rounded-xl bg-surface-raised text-xs font-bold uppercase tracking-wider text-muted ${className}`}
        style={{ height }}
      >
        No GPS track
      </div>
    );
  }

  const W = 400;
  const H = 200;
  const PAD = 14;
  const lats = points.map((p) => p.lat);
  const lngs = points.map((p) => p.lng);
  const minLat = Math.min(...lats);
  const maxLat = Math.max(...lats);
  const minLng = Math.min(...lngs);
  const maxLng = Math.max(...lngs);
  // Meters-per-degree correction so the shape isn't stretched.
  const latScale = 1;
  const lngScale = Math.cos(((minLat + maxLat) / 2) * (Math.PI / 180));
  const spanLat = Math.max(maxLat - minLat, 1e-6) * latScale;
  const spanLng = Math.max(maxLng - minLng, 1e-6) * lngScale;
  const scale = Math.min((W - PAD * 2) / spanLng, (H - PAD * 2) / spanLat);
  const offsetX = (W - spanLng * scale) / 2;
  const offsetY = (H - spanLat * scale) / 2;

  const toXY = (p: TrackPoint): [number, number] => [
    offsetX + (p.lng - minLng) * lngScale * scale,
    H - (offsetY + (p.lat - minLat) * latScale * scale),
  ];
  const path = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${toXY(p)[0].toFixed(1)},${toXY(p)[1].toFixed(1)}`).join(' ');
  const [sx, sy] = toXY(points[0]!);
  const [ex, ey] = toXY(points[points.length - 1]!);

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className={`w-full rounded-xl bg-surface-raised ${className}`}
      style={{ height }}
      role="img"
      aria-label="Route map"
    >
      <path d={path} fill="none" stroke="#ed1c24" strokeWidth={3.5} strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={sx} cy={sy} r={5} fill="#34d27b" stroke="#fff" strokeWidth={1.5} />
      <circle cx={ex} cy={ey} r={5} fill="#191919" stroke="#fff" strokeWidth={1.5} />
    </svg>
  );
}
