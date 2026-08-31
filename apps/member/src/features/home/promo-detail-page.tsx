import { Spinner, formatDay } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, Copy } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';

export function PromoDetailPage() {
  const { code = '' } = useParams();
  const navigate = useNavigate();
  const [copied, setCopied] = useState(false);
  const { data: p, isLoading } = useQuery({
    queryKey: ['promo', code],
    queryFn: () => api.me.promo(code),
  });

  if (isLoading || !p) return <Spinner label="Loading promo…" />;

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>

      <div className="card surface-ink relative overflow-hidden !border-0 !p-7 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-64 w-64 rounded-full bg-brand/25 blur-3xl" />
        <div className="relative">
          <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
            {p.live ? 'Live promo' : 'Promo'}
          </p>
          <p className="display mt-1 text-5xl leading-none">{p.label}</p>
          <button
            className="mt-5 inline-flex items-center gap-2 rounded-full bg-white/10 px-4 py-2 font-mono text-sm font-bold tracking-[0.14em]"
            onClick={() => {
              void navigator.clipboard?.writeText(p.code);
              setCopied(true);
            }}
          >
            {p.code} <Copy size={14} className="text-white/60" />
            {copied ? <span className="text-[10px] uppercase text-ok">Copied</span> : null}
          </button>
        </div>
      </div>

      <div className="card flex flex-col gap-3 text-sm">
        <div className="flex justify-between">
          <span className="text-muted">Valid</span>
          <span className="font-bold">
            {formatDay(p.startsAt)} – {formatDay(p.endsAt)}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-muted">Applies to</span>
          <span className="max-w-[60%] text-right font-bold">
            {p.packageNames ? p.packageNames.join(', ') : 'Every credit package'}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-muted">Per member</span>
          <span className="font-bold">{p.perMemberLimit ?? 'Unlimited'} use{p.perMemberLimit === 1 ? '' : 's'}</span>
        </div>
        {p.newMembersOnly ? (
          <p className="rounded-xl bg-brand/10 px-3 py-2 text-xs font-bold text-brand">
            New members only — valid within your first 14 days.
          </p>
        ) : null}
      </div>

      <Link
        to={`/wallet/topup?voucher=${encodeURIComponent(p.code)}`}
        className={`btn-brand ${p.live ? '' : 'pointer-events-none opacity-40'}`}
      >
        Use it at top up
      </Link>
    </div>
  );
}
