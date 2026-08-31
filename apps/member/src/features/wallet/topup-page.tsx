import { ApiError } from '@hyrox/api-client';
import type { PaymentChannel } from '@hyrox/domain';
import { Spinner, formatIdr } from '@hyrox/ui';
import { ArrowLeft, Check } from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { api } from '../../lib/api';
import { usePackages } from '../../lib/queries';

const CHANNELS: { id: PaymentChannel; label: string }[] = [
  { id: 'QRIS', label: 'QRIS' },
  { id: 'EWALLET', label: 'E-Wallet' },
  { id: 'VIRTUAL_ACCOUNT', label: 'Virtual Account' },
  { id: 'CARD', label: 'Card' },
];

export function TopUpPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { data: packages, isLoading } = usePackages();
  const [packageId, setPackageId] = useState('');
  const [channel, setChannel] = useState<PaymentChannel>('QRIS');
  // Promo cards on Home deep-link here with the code prefilled.
  const [voucherCode, setVoucherCode] = useState(
    (searchParams.get('voucher') ?? '').toUpperCase(),
  );
  const [voucherState, setVoucherState] = useState<
    { kind: 'idle' } | { kind: 'ok'; discountIdr: number } | { kind: 'err'; message: string }
  >({ kind: 'idle' });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  if (isLoading || !packages) return <Spinner label="Loading packages…" />;

  const selected = packages.find((p) => p.id === packageId);
  const discount = voucherState.kind === 'ok' ? voucherState.discountIdr : 0;

  const checkVoucher = async () => {
    if (!voucherCode || !packageId) return;
    try {
      const res = await api.catalog.validateVoucher(voucherCode, packageId);
      setVoucherState({ kind: 'ok', discountIdr: res.discountIdr });
    } catch (e) {
      setVoucherState({
        kind: 'err',
        message: e instanceof ApiError ? e.message : 'Voucher check failed.',
      });
    }
  };

  const checkout = async () => {
    if (!selected) return;
    setBusy(true);
    setError('');
    try {
      const res = await api.me.topUp({
        packageId: selected.id,
        voucherCode: voucherState.kind === 'ok' ? voucherCode : null,
        channel,
      });
      navigate(`/wallet/pay/${res.payment.id}`);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Checkout failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <h1 className="display text-3xl font-black">Top up</h1>

      <div className="flex flex-col gap-2">
        {packages.map((p) => (
          <button
            key={p.id}
            onClick={() => {
              setPackageId(p.id);
              setVoucherState({ kind: 'idle' });
            }}
            className={`card flex items-center justify-between text-left ${
              packageId === p.id ? '!border-brand' : ''
            }`}
          >
            <div>
              <p className="font-black">{p.name}</p>
              <p className="text-sm text-muted">
                {p.credits} credits · valid {p.validityDays} days
              </p>
            </div>
            <div className="flex items-center gap-2">
              <span className="font-black text-brand">{formatIdr(p.priceIdr)}</span>
              {packageId === p.id ? <Check size={18} className="text-brand" /> : null}
            </div>
          </button>
        ))}
      </div>

      {selected ? (
        <>
          <div>
            <label className="label">Voucher code</label>
            <div className="flex gap-2">
              <input
                className="input flex-1 uppercase"
                value={voucherCode}
                onChange={(e) => {
                  setVoucherCode(e.target.value.toUpperCase());
                  setVoucherState({ kind: 'idle' });
                }}
                placeholder="WELCOME10"
              />
              <button className="btn-ghost !px-4 !py-2 text-sm" onClick={() => void checkVoucher()}>
                Apply
              </button>
            </div>
            {voucherState.kind === 'ok' ? (
              <p className="mt-1.5 text-sm font-bold text-ok">
                Voucher applied: −{formatIdr(voucherState.discountIdr)}
              </p>
            ) : voucherState.kind === 'err' ? (
              <p className="mt-1.5 text-sm font-bold text-danger">{voucherState.message}</p>
            ) : null}
          </div>

          <div>
            <label className="label">Payment method</label>
            <div className="grid grid-cols-2 gap-2">
              {CHANNELS.map((c) => (
                <button
                  key={c.id}
                  onClick={() => setChannel(c.id)}
                  className={`rounded-xl border px-3 py-2.5 text-sm font-bold ${
                    channel === c.id
                      ? 'border-brand bg-brand/10 text-brand'
                      : 'border-line bg-surface text-muted'
                  }`}
                >
                  {c.label}
                </button>
              ))}
            </div>
          </div>

          <div className="card flex flex-col gap-1 text-sm">
            <div className="flex justify-between">
              <span className="text-muted">{selected.name}</span>
              <span>{formatIdr(selected.priceIdr)}</span>
            </div>
            {discount > 0 ? (
              <div className="flex justify-between text-ok">
                <span>Voucher</span>
                <span>−{formatIdr(discount)}</span>
              </div>
            ) : null}
            <div className="mt-1 flex justify-between border-t border-line pt-2 font-black">
              <span>Total</span>
              <span className="text-brand">{formatIdr(selected.priceIdr - discount)}</span>
            </div>
          </div>

          {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
          <button className="btn-brand" disabled={busy} onClick={() => void checkout()}>
            Checkout
          </button>
        </>
      ) : null}
    </div>
  );
}
