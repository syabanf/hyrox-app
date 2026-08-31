export type Tone = 'ok' | 'warn' | 'danger' | 'info' | 'neutral' | 'brand';

const TONE_STYLES: Record<Tone, { background: string; color: string }> = {
  ok: { background: 'rgb(52 210 123 / 0.15)', color: '#34d27b' },
  warn: { background: 'rgb(255 176 32 / 0.15)', color: '#ffb020' },
  danger: { background: 'rgb(255 77 79 / 0.15)', color: '#ff4d4f' },
  info: { background: 'rgb(77 171 247 / 0.15)', color: '#4dabf7' },
  neutral: { background: 'rgb(154 154 154 / 0.18)', color: '#9a9a9a' },
  brand: { background: 'rgb(237 28 36 / 0.14)', color: '#ed1c24' },
};

/** Maps every entity state in the system to a visual tone. */
export function statusTone(status: string): Tone {
  switch (status) {
    case 'ACTIVE':
    case 'CONFIRMED':
    case 'PAID':
    case 'ALLOWED':
    case 'PUBLISHED':
    case 'SENT':
    case 'CHECKED_IN':
    case 'ONLINE':
    case 'SYNCED':
    case 'COMPLETED':
      return 'ok';
    case 'PENDING':
    case 'WAITLIST':
    case 'SCHEDULED':
    case 'FULL':
    case 'SUSPENDED':
    case 'OFFLINE_ALLOWED':
    case 'PROCESSING':
    case 'DRAFT':
      return 'warn';
    case 'FAILED':
    case 'DENIED':
    case 'CANCELLED':
    case 'NO_SHOW':
    case 'CONFLICT':
    case 'EXPIRED':
    case 'ARCHIVED':
    case 'OFFLINE':
      return 'danger';
    case 'REFUNDED':
    case 'INACTIVE':
    case 'DISABLED':
      return 'neutral';
    default:
      return 'info';
  }
}

export function StatusBadge({ status, tone }: { status: string; tone?: Tone }) {
  const style = TONE_STYLES[tone ?? statusTone(status)];
  return (
    <span className="hx-badge" style={style}>
      {status.replaceAll('_', ' ')}
    </span>
  );
}
