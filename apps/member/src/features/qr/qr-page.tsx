import { Spinner } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { QRCodeSVG } from 'qrcode.react';
import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../lib/auth';
import { useMe } from '../../lib/queries';

export function QrPage() {
  const member = useAuthStore((s) => s.member);
  const { data: me } = useMe();

  // A fresh short-lived token, re-issued automatically the moment it expires.
  const qr = useQuery({
    queryKey: ['qr'],
    queryFn: api.me.qr,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return false;
      return Math.max(1_000, new Date(data.expiresAt).getTime() - Date.now());
    },
    refetchIntervalInBackground: false,
    staleTime: 0,
    gcTime: 0,
  });

  const [secondsLeft, setSecondsLeft] = useState(0);
  useEffect(() => {
    if (!qr.data) return;
    const tick = () =>
      setSecondsLeft(
        Math.max(0, Math.ceil((new Date(qr.data.expiresAt).getTime() - Date.now()) / 1000)),
      );
    tick();
    const id = setInterval(tick, 250);
    return () => clearInterval(id);
  }, [qr.data]);

  if (!qr.data) return <Spinner label="Generating your code…" />;

  const fraction = qr.data.ttlSeconds > 0 ? secondsLeft / qr.data.ttlSeconds : 0;
  const R = 26;
  const CIRC = 2 * Math.PI * R;

  return (
    <div className="flex flex-col items-center gap-5 pt-2">
      <div className="text-center">
        <h1 className="display text-2xl font-black">Gate access</h1>
        <p className="text-sm text-muted">Show this at the scanner</p>
      </div>

      <div className="rounded-3xl bg-white p-5 shadow-[0_0_60px_rgba(237,28,36,0.25)]">
        <QRCodeSVG value={qr.data.token} size={240} level="M" marginSize={0} />
      </div>

      <div className="flex items-center gap-3">
        <svg width="64" height="64" viewBox="0 0 64 64" aria-hidden>
          <circle cx="32" cy="32" r={R} fill="none" stroke="var(--color-line)" strokeWidth="5" />
          <circle
            cx="32"
            cy="32"
            r={R}
            fill="none"
            stroke="var(--color-brand)"
            strokeWidth="5"
            strokeLinecap="round"
            strokeDasharray={CIRC}
            strokeDashoffset={CIRC * (1 - fraction)}
            transform="rotate(-90 32 32)"
            style={{ transition: 'stroke-dashoffset 0.25s linear' }}
          />
          <text
            x="32"
            y="37"
            textAnchor="middle"
            fill="#191919"
            fontSize="16"
            fontWeight="800"
            fontFamily="inherit"
          >
            {secondsLeft}
          </text>
        </svg>
        <div className="text-sm text-muted">
          <p className="font-bold text-ink">{member?.fullName}</p>
          <p>
            Balance:{' '}
            <span className="font-black text-brand">{me?.balance ?? '…'} credits</span>
          </p>
          <p className="text-xs">Code refreshes automatically.</p>
        </div>
      </div>

      <Link to="/visits" className="text-sm font-bold text-brand">
        Visit history →
      </Link>
      <p className="max-w-64 text-center text-xs text-muted/70">
        Tip: use the flask button (dev tools) to simulate scanning this code at a gate.
      </p>
    </div>
  );
}
