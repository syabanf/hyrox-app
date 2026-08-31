import { ApiError } from '@hyrox/api-client';
import { formatIdr } from '@hyrox/ui';
import { CheckCircle2 } from 'lucide-react';
import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { api } from '../../lib/api';
import { useInvalidateAll } from '../../lib/queries';

/** Stand-in for the Xendit-hosted payment page. */
export function PaymentPage() {
  const { paymentId = '' } = useParams();
  const invalidate = useInvalidateAll();
  const [state, setState] = useState<'pending' | 'paid'>('pending');
  const [credits, setCredits] = useState<number | null>(null);
  const [totalIdr, setTotalIdr] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const pay = async () => {
    setBusy(true);
    setError('');
    try {
      const res = await api.payments.simulate(paymentId);
      setCredits(res.payment.credits);
      setTotalIdr(res.payment.totalIdr);
      setState('paid');
      invalidate();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Payment failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col items-center gap-6 pt-8 text-center">
      {state === 'pending' ? (
        <>
          <div className="card w-full">
            <p className="label">Mock payment gateway</p>
            <p className="text-sm text-muted">
              In production this screen is Xendit's checkout (QRIS / e-wallet / VA / card). Here you
              simulate the payment callback.
            </p>
          </div>
          {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
          <button className="btn-brand w-full" disabled={busy} onClick={() => void pay()}>
            Pay now (simulate success)
          </button>
          <Link to="/wallet" className="text-sm font-bold text-muted">
            Cancel and go back
          </Link>
        </>
      ) : (
        <>
          <CheckCircle2 size={72} className="text-ok" />
          <div>
            <h1 className="display text-3xl font-black">Payment successful</h1>
            <p className="mt-1 text-muted">
              {credits} credits added{totalIdr !== null ? ` · ${formatIdr(totalIdr)}` : ''}
            </p>
          </div>
          <Link to="/wallet" className="btn-brand w-full">
            Back to wallet
          </Link>
          <Link to="/classes" className="text-sm font-bold text-brand">
            Book a class →
          </Link>
        </>
      )}
    </div>
  );
}
