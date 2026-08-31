export function formatIdr(amount: number): string {
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(amount);
}

export function formatCredits(amount: number, opts: { signed?: boolean } = {}): string {
  const sign = opts.signed && amount > 0 ? '+' : '';
  return `${sign}${amount}`;
}

export function formatDayTime(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function formatDay(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
  });
}

export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

// ── Athlete formatting ────────────────────────────────────────────────────────
export type Units = 'METRIC' | 'IMPERIAL';
const KM_PER_MI = 1.60934;

export function formatDistanceM(meters: number, units: Units = 'METRIC'): string {
  if (units === 'IMPERIAL') {
    const mi = meters / 1000 / KM_PER_MI;
    return `${mi >= 10 ? mi.toFixed(1) : mi.toFixed(2)} mi`;
  }
  const km = meters / 1000;
  return `${km >= 10 ? km.toFixed(1) : km.toFixed(2)} km`;
}

export function formatDuration(totalSec: number): string {
  const sec = Math.max(0, Math.round(totalSec));
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  return `${m}:${String(s).padStart(2, '0')}`;
}

export function formatPace(secPerKm: number | null, units: Units = 'METRIC'): string {
  if (secPerKm === null || !Number.isFinite(secPerKm)) return '—';
  const sec = units === 'IMPERIAL' ? secPerKm * KM_PER_MI : secPerKm;
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  return `${m}:${String(s).padStart(2, '0')} /${units === 'IMPERIAL' ? 'mi' : 'km'}`;
}

export function formatSpeedKmh(distanceM: number, movingSec: number, units: Units = 'METRIC'): string {
  if (movingSec <= 0) return '—';
  const kmh = distanceM / 1000 / (movingSec / 3600);
  if (units === 'IMPERIAL') return `${(kmh / KM_PER_MI).toFixed(1)} mph`;
  return `${kmh.toFixed(1)} km/h`;
}
