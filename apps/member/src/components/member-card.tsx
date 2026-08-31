import { useQuery } from '@tanstack/react-query';
import { X } from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import { useEffect, useState } from 'react';
import { api } from '../lib/api';
import { useMe } from '../lib/queries';

/** Full-screen digital membership card with the live gate QR. */
export function MemberCardSheet({ onClose }: { onClose: () => void }) {
  const { data: me } = useMe();

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
    const id = setInterval(tick, 500);
    return () => clearInterval(id);
  }, [qr.data]);

  if (!me) return null;
  const m = me.member;
  const memberNo = `NO. ${m.id.replace('mem_', '').toUpperCase().padStart(6, '0')}`;

  return (
    <div
      className="sheet-backdrop fixed inset-0 z-40 flex items-center justify-center bg-black/70 p-5 backdrop-blur-sm"
      onClick={onClose}
    >
      <div className="sheet-panel w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
        <div className="surface-ink relative overflow-hidden rounded-3xl p-6 text-white shadow-[0_30px_80px_rgb(0_0_0/0.5)]">
          <div className="pointer-events-none absolute -right-20 -top-28 h-64 w-64 rounded-full bg-brand/30 blur-3xl" />
          <div className="pointer-events-none absolute -bottom-24 -left-16 h-48 w-48 rounded-full bg-white/[0.05] blur-2xl" />

          <div className="relative flex items-start justify-between">
            <div>
              <p className="display text-lg">
                HYROX<span className="text-[#ff4348]">STUDIO</span>
              </p>
              <p className="mt-0.5 text-[10px] font-bold uppercase tracking-[0.24em] text-white/40">
                Member card
              </p>
            </div>
            <span
              className={`chip ${m.status === 'ACTIVE' ? 'bg-ok/20 text-ok' : 'bg-white/10 text-white/70'}`}
            >
              {m.status}
            </span>
          </div>

          <div className="relative mt-6 flex items-center justify-center">
            <div className="rounded-2xl bg-white p-3.5">
              {qr.data ? (
                <QRCodeSVG value={qr.data.token} size={168} level="M" marginSize={0} />
              ) : (
                <div className="h-[168px] w-[168px]" />
              )}
            </div>
          </div>
          <p className="relative mt-2 text-center text-[11px] font-bold text-white/40">
            Scan at the gate · refreshes in {secondsLeft}s
          </p>

          <div className="relative mt-6 flex items-end justify-between gap-3">
            <div className="min-w-0">
              <p className="display truncate text-xl leading-tight">{m.fullName}</p>
              <p className="mt-0.5 font-mono text-[11px] tracking-[0.18em] text-white/45">
                {memberNo}
              </p>
            </div>
            <div className="shrink-0 text-right">
              <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/40">
                Credits
              </p>
              <p className="display text-2xl leading-none text-[#ff4348]">{me.balance}</p>
            </div>
          </div>
        </div>

        <button
          onClick={onClose}
          className="mx-auto mt-5 flex h-11 w-11 items-center justify-center rounded-full bg-white/15 text-white"
          aria-label="Close"
        >
          <X size={18} />
        </button>
      </div>
    </div>
  );
}
