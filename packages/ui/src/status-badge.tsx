export type Tone = 'ok' | 'warn' | 'danger' | 'info' | 'neutral' | 'brand';

/* NüHabit palette: Lettuce / Golden Ochre tints for fills, deepened for text on beige;
   the brand tone is Pale Lime with Deep Forest Green. */
const TONE_STYLES: Record<Tone, { background: string; color: string }> = {
  ok: { background: 'rgb(171 222 103 / 0.28)', color: '#3e7314' },
  warn: { background: 'rgb(201 162 39 / 0.2)', color: '#8a6a10' },
  danger: { background: 'rgb(209 59 64 / 0.14)', color: '#c2363b' },
  info: { background: 'rgb(32 59 50 / 0.12)', color: '#203b32' },
  neutral: { background: 'rgb(95 107 98 / 0.15)', color: '#5f6b62' },
  brand: { background: 'rgb(218 255 89 / 0.5)', color: '#00281a' },
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
    case 'PRESENT':
    case 'REMOTE':
    case 'APPROVED':
    case 'WORKING':
    case 'DONE':
      return 'ok';
    case 'PENDING':
    case 'WAITLIST':
    case 'SCHEDULED':
    case 'FULL':
    case 'SUSPENDED':
    case 'OFFLINE_ALLOWED':
    case 'PROCESSING':
    case 'DRAFT':
    case 'LATE':
    case 'HALF_DAY':
    case 'EXPECTED':
      return 'warn';
    case 'FAILED':
    case 'DENIED':
    case 'CANCELLED':
    case 'NO_SHOW':
    case 'CONFLICT':
    case 'EXPIRED':
    case 'ARCHIVED':
    case 'OFFLINE':
    case 'ABSENT':
    case 'MISSING':
    case 'REJECTED':
      return 'danger';
    case 'REFUNDED':
    case 'INACTIVE':
    case 'DISABLED':
    case 'VOID':
    case 'REST_DAY':
    case 'UNSCHEDULED':
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
