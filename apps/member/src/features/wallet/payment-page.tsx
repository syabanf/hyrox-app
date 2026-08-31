import { ApiError } from '@hyrox/api-client';
import type { PaymentChannel } from '@hyrox/domain';
import { Spinner, formatIdr } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { CheckCircle2, Copy, ShieldCheck, Smartphone } from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router';
import { api } from '../../lib/api';
import { useInvalidateAll } from '../../lib/queries';

const CHANNEL_TITLE: Record<PaymentChannel, string> = {
  QRIS: 'Scan to pay with QRIS',
  EWALLET: 'Pay with your e-wallet',
  VIRTUAL_ACCOUNT: 'Bank transfer — Virtual Account',
  CARD: 'Pay with card',
};

/** Deterministic fake VA number derived from the payment id. */
const vaNumber = (paymentId: string): string => {
  let h = 0;
  for (const ch of paymentId) h = (h * 31 + ch.charCodeAt(0)) % 1_0000_0000;
  return `8808 ${String(h).padStart(8, '0').replace(/(\d{4})(\d{4})/, '$1 $2')}`;
};

const WALLETS = ['GoPay', 'OVO', 'DANA', 'ShopeePay'];

/**
 * Mock checkout standing in for the Xendit-hosted payment page — looks and
 * flows like the real thing per channel, but "paying" just fires the fake
 * webhook. No gateway integration yet by design.
 */
export function PaymentPage() {
  const { paymentId = '' } = useParams();
  const invalidate = useInvalidateAll();
  const [state, setState] = useState<'pending' | 'paid'>('pending');
  const [credits, setCredits] = useState<number | null>(null);
  const [wallet, setWallet] = useState(WALLETS[0]);
  const [copied, setCopied] = useState(false);
  const [secondsLeft, setSecondsLeft] = useState(15 * 60);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const { data } = useQuery({
    queryKey: ['payment', paymentId],
    queryFn: () => api.payments.get(paymentId),
  });

  useEffect(() => {
    const t = setInterval(() => setSecondsLeft((s) => Math.max(0, s - 1)), 1000);
    return () => clearInterval(t);
  }, []);

  const pay = async () => {
    setBusy(true);
    setError('');
    try {
      const res = await api.payments.simulate(paymentId);
      setCredits(res.payment.credits);
      setState('paid');
      invalidate();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Payment failed.');
    } finally {
      setBusy(false);
    }
  };

  if (state === 'paid') {
    return (
      <div className="flex flex-col items-center gap-6 pt-10 text-center">
        <CheckCircle2 size={72} className="text-ok" />
        <div>
          <h1 className="display text-3xl font-black">Payment successful</h1>
          <p className="mt-1 text-muted">
            {credits} credits added
            {data ? ` · ${formatIdr(data.payment.totalIdr)}` : ''}
          </p>
        </div>
        <Link to="/wallet" className="btn-brand w-full">
          Back to wallet
        </Link>
        <Link to="/classes" className="text-sm font-bold text-brand">
          Book a class →
        </Link>
      </div>
    );
  }

  if (!data) return <Spinner label="Preparing checkout…" />;
  const { payment, packageName } = data;
  const mm = Math.floor(secondsLeft / 60);
  const ss = String(secondsLeft % 60).padStart(2, '0');

  return (
    <div className="flex flex-col gap-5 pt-2">
      {/* Gateway-style header */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
            Secure checkout
          </p>
          <p className="display text-2xl">{CHANNEL_TITLE[payment.channel]}</p>
        </div>
        <span className="chip bg-surface-raised text-muted">
          <ShieldCheck size={12} /> Demo mode
        </span>
      </div>

      <div className="card flex items-center justify-between !py-4 text-sm">
        <div>
          <p className="font-extrabold">{packageName}</p>
          <p className="text-muted">{payment.credits} credits</p>
        </div>
        <div className="text-right">
          <p className="display text-xl text-brand">{formatIdr(payment.totalIdr)}</p>
          <p className="text-xs text-muted">expires in {mm}:{ss}</p>
        </div>
      </div>

      {payment.channel === 'QRIS' ? (
        <div className="card flex flex-col items-center gap-3 !p-6">
          <div className="rounded-2xl bg-white p-4 shadow-[0_1px_2px_rgb(0_0_0/0.06)]">
            <QRCodeSVG value={`QRIS.DEMO.${payment.id}.${payment.totalIdr}`} size={190} />
          </div>
          <p className="text-center text-sm text-muted">
            Scan with any banking or e-wallet app that supports QRIS.
          </p>
        </div>
      ) : null}

      {payment.channel === 'VIRTUAL_ACCOUNT' ? (
        <div className="card flex flex-col gap-3">
          <div>
            <p className="label !mb-1">BCA Virtual Account</p>
            <div className="flex items-center justify-between gap-2 rounded-xl bg-surface-raised px-4 py-3">
              <span className="font-mono text-lg font-extrabold tracking-wider">
                {vaNumber(payment.id)}
              </span>
              <button
                className="flex items-center gap-1 text-xs font-bold text-brand"
                onClick={() => {
                  void navigator.clipboard?.writeText(vaNumber(payment.id).replaceAll(' ', ''));
                  setCopied(true);
                }}
              >
                <Copy size={13} /> {copied ? 'Copied' : 'Copy'}
              </button>
            </div>
          </div>
          <ol className="list-decimal pl-4 text-sm text-muted">
            <li>Open your mobile banking app.</li>
            <li>Choose transfer to BCA Virtual Account.</li>
            <li>Paste the number and confirm the exact amount.</li>
          </ol>
        </div>
      ) : null}

      {payment.channel === 'EWALLET' ? (
        <div className="card flex flex-col gap-3">
          <p className="label !mb-0">Choose your wallet</p>
          <div className="grid grid-cols-2 gap-2">
            {WALLETS.map((w) => (
              <button
                key={w}
                onClick={() => setWallet(w)}
                className={`rounded-xl border px-3 py-2.5 text-sm font-bold ${
                  wallet === w
                    ? 'border-brand bg-brand/10 text-brand'
                    : 'border-line bg-surface text-muted'
                }`}
              >
                {w}
              </button>
            ))}
          </div>
          <p className="flex items-center gap-2 text-sm text-muted">
            <Smartphone size={15} /> You'll be asked to approve the charge in {wallet}.
          </p>
        </div>
      ) : null}

      {payment.channel === 'CARD' ? (
        <div className="surface-ink card relative overflow-hidden !border-0 !p-5 text-white">
          <div className="pointer-events-none absolute -right-14 -top-20 h-44 w-44 rounded-full bg-brand/25 blur-3xl" />
          <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
            Demo card on file
          </p>
          <p className="font-mono mt-3 text-xl tracking-[0.18em]">4242 4242 4242 4242</p>
          <div className="mt-3 flex justify-between text-xs text-white/60">
            <span>HYROX STUDIO MEMBER</span>
            <span>12/29 · CVV ···</span>
          </div>
        </div>
      ) : null}

      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
      <button className="btn-brand" disabled={busy || secondsLeft === 0} onClick={() => void pay()}>
        {payment.channel === 'QRIS'
          ? "I've paid — check status"
          : payment.channel === 'VIRTUAL_ACCOUNT'
            ? "I've transferred — check status"
            : payment.channel === 'EWALLET'
              ? `Approve in ${wallet}`
              : `Pay ${formatIdr(payment.totalIdr)}`}
      </button>
      <p className="text-center text-xs text-muted">
        Demo checkout — no real money moves. Xendit replaces this screen in production.
      </p>
      <Link to="/wallet" className="text-center text-sm font-bold text-muted">
        Cancel and go back
      </Link>
    </div>
  );
}
