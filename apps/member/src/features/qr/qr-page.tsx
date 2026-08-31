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
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="display text-3xl">Gate access</h1>
        <p className="mt-1 text-sm text-muted">Show this at the scanner to check in.</p>
      </div>

      <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-20 -top-28 h-64 w-64 rounded-full bg-brand/30 blur-3xl" />
        <div className="pointer-events-none absolute -bottom-24 -left-16 h-48 w-48 rounded-full bg-white/[0.05] blur-2xl" />

        <div className="relative mx-auto w-fit rounded-2xl bg-white p-4">
          <QRCodeSVG value={qr.data.token} size={212} level="M" marginSize={0} />
        </div>

        <div className="relative mt-6 flex items-center gap-4">
          <svg width="56" height="56" viewBox="0 0 64 64" aria-hidden className="shrink-0">
            <circle cx="32" cy="32" r={R} fill="none" stroke="rgba(255,255,255,0.15)" strokeWidth="5" />
            <circle
              cx="32"
              cy="32"
              r={R}
              fill="none"
              stroke="#f5333a"
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
              fill="#ffffff"
              fontSize="16"
              fontWeight="800"
              fontFamily="inherit"
            >
              {secondsLeft}
            </text>
          </svg>
          <div className="min-w-0 flex-1 text-sm text-white/55">
            <p className="truncate font-extrabold text-white">{member?.fullName}</p>
            <p className="text-xs">Refreshes automatically every {qr.data.ttlSeconds}s.</p>
          </div>
          <div className="shrink-0 text-right">
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/40">Credits</p>
            <p className="display text-2xl leading-none text-[#ff4348]">{me?.balance ?? '…'}</p>
          </div>
        </div>

        <div className="relative mt-5 flex items-center justify-between border-t border-white/10 pt-4">
          <Link to="/visits" className="chip bg-white/10 text-white/80">
            Visit history →
          </Link>
          <Link to="/wallet/topup" className="chip bg-white/10 text-white/80">
            Top up
          </Link>
        </div>
      </div>
    </div>
  );
}
