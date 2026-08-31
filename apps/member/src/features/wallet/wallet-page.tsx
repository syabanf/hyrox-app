import { Spinner, StatusBadge, formatDay, formatDayTime } from '@hyrox/ui';
import { Link } from 'react-router';
import { useWallet } from '../../lib/queries';

const ENTRY_LABEL: Record<string, string> = {
  TOP_UP: 'Top up',
  VISIT_DEDUCTION: 'Visit',
  REFUND: 'Refund',
  BONUS: 'Bonus',
  PROMO: 'Promo',
  EXPIRATION: 'Expired',
  ADJUSTMENT: 'Adjustment',
  REVERSAL: 'Reversal',
};

export function WalletPage() {
  const { data: wallet, isLoading } = useWallet();
  if (isLoading || !wallet) return <Spinner label="Loading wallet…" />;

  return (
    <div className="flex flex-col gap-5">
      <h1 className="display text-3xl font-black">Wallet</h1>

      <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
        <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">Credit balance</p>
        <p className="display mt-1 text-7xl leading-none">{wallet.balance}</p>
        {wallet.expiringCredits > 0 ? (
          <p className="mt-2 rounded-lg bg-black/25 px-3 py-1.5 text-xs font-bold">
            {wallet.expiringCredits} expiring within the reminder window
          </p>
        ) : null}
      </div>

      <Link to="/wallet/topup" className="btn-brand">
        Top up credits
      </Link>

      {wallet.myPackages.length > 0 ? (
        <section>
          <h2 className="display mb-2 text-xl font-black">My packages</h2>
          <div className="flex flex-col gap-2">
            {wallet.myPackages.map((p) => (
              <div key={p.lotId} className={`card !py-4 ${p.active ? '' : 'opacity-60'}`}>
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-extrabold">{p.name}</p>
                    <p className="text-xs text-muted">
                      {p.credits} credits · bought {formatDay(p.purchasedAt)}
                    </p>
                  </div>
                  <span
                    className={`chip shrink-0 ${p.active ? 'bg-ok/10 text-ok' : 'bg-surface-raised text-muted'}`}
                  >
                    {p.active ? `Valid until ${formatDay(p.expiresAt)}` : 'Expired'}
                  </span>
                </div>
                <div className="mt-2.5 flex flex-wrap gap-1.5">
                  {p.coverageNames ? (
                    p.coverageNames.map((n) => (
                      <span key={n} className="chip bg-brand/10 text-brand">
                        {n}
                      </span>
                    ))
                  ) : (
                    <span className="chip bg-surface-raised text-muted">All classes</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        </section>
      ) : null}

      <section>
        <h2 className="display mb-2 text-xl font-black">Transaction history</h2>
        <div className="flex flex-col gap-2">
          {wallet.entries.map((e) => (
            <div key={e.id} className="card flex items-center justify-between gap-3 !p-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-bold">{e.description}</p>
                <p className="text-xs text-muted">
                  {formatDayTime(e.createdAt)} · <StatusBadge status={ENTRY_LABEL[e.type] ?? e.type} tone={e.amount >= 0 ? 'ok' : 'neutral'} />
                </p>
              </div>
              <span
                className={`display text-xl font-black ${e.amount >= 0 ? 'text-ok' : 'text-danger'}`}
              >
                {e.amount > 0 ? `+${e.amount}` : e.amount}
              </span>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
