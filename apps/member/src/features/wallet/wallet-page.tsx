import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
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
    <div className="flex flex-col gap-4">
      <h1 className="display text-2xl font-black">Wallet</h1>

      <div className="card !bg-brand !border-brand text-white">
        <p className="text-xs font-black uppercase tracking-wider opacity-80">Credit balance</p>
        <p className="display text-6xl font-black leading-none">{wallet.balance}</p>
        {wallet.expiringCredits > 0 ? (
          <p className="mt-2 rounded-lg bg-black/25 px-3 py-1.5 text-xs font-bold">
            {wallet.expiringCredits} expiring within the reminder window
          </p>
        ) : null}
      </div>

      <Link to="/wallet/topup" className="btn-brand">
        Top up credits
      </Link>

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
