import { ApiError } from '@hyrox/api-client';
import { Spinner, formatDayTime } from '@hyrox/ui';
import { ArrowLeft } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';
import { useBookMutation, useCancelMutation, useInvalidateAll, useSession } from '../../lib/queries';

export function SessionDetailPage() {
  const { sessionId = '' } = useParams();
  const navigate = useNavigate();
  const { data: v, isLoading } = useSession(sessionId);
  const book = useBookMutation();
  const cancel = useCancelMutation();
  const [message, setMessage] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null);
  const [confirmBusy, setConfirmBusy] = useState(false);
  const invalidate = useInvalidateAll();

  const onConfirmSpot = async (bookingId: string) => {
    setConfirmBusy(true);
    setMessage(null);
    try {
      await api.bookings.confirmSpot(bookingId);
      setMessage({ kind: 'ok', text: "You're in — spot confirmed!" });
      invalidate();
    } catch (e) {
      setMessage({ kind: 'err', text: e instanceof ApiError ? e.message : 'Could not confirm.' });
      invalidate();
    } finally {
      setConfirmBusy(false);
    }
  };

  if (isLoading || !v) return <Spinner label="Loading class…" />;

  const mine = v.myBooking;
  const bookable = ['PUBLISHED', 'FULL'].includes(v.session.status);

  const onBook = async () => {
    setMessage(null);
    try {
      const res = await book.mutateAsync(v.session.id);
      setMessage({
        kind: 'ok',
        text:
          res.decision === 'CONFIRMED'
            ? "You're in! See you at the studio."
            : `Class is full — you're #${res.booking.waitlistPosition} on the waitlist.`,
      });
    } catch (e) {
      setMessage({ kind: 'err', text: e instanceof ApiError ? e.message : 'Booking failed.' });
    }
  };

  const onCancel = async () => {
    if (!mine) return;
    setMessage(null);
    try {
      const res = await cancel.mutateAsync(mine.id);
      setMessage({
        kind: 'ok',
        text:
          res.outcome === 'LATE' && res.penaltyCredits > 0
            ? `Cancelled after the deadline — ${res.penaltyCredits} credit forfeited.`
            : 'Booking cancelled.',
      });
    } catch (e) {
      setMessage({ kind: 'err', text: e instanceof ApiError ? e.message : 'Cancel failed.' });
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>

      <div>
        <h1 className="display text-3xl font-black">{v.classTypeName}</h1>
        <p className="mt-1 text-muted">{formatDayTime(v.session.startsAt)}</p>
      </div>

      <div className="card grid grid-cols-2 gap-3 text-sm">
        <div>
          <p className="label !mb-0.5">Branch</p>
          <p className="font-bold">{v.branchName}</p>
        </div>
        <div>
          <p className="label !mb-0.5">Coach</p>
          <p className="font-bold">{v.coachName}</p>
        </div>
        <div>
          <p className="label !mb-0.5">Cost</p>
          <p className="font-bold text-brand">{v.session.creditCost} credit{v.session.creditCost === 1 ? '' : 's'}</p>
        </div>
        <div>
          <p className="label !mb-0.5">Capacity</p>
          <p className="font-bold">
            {v.confirmedCount}/{v.session.capacity}
            {v.waitlistCount > 0 ? ` · ${v.waitlistCount} waiting` : ''}
          </p>
        </div>
      </div>

      {message ? (
        <div
          className={`rounded-xl p-3 text-sm font-bold ${
            message.kind === 'ok' ? 'bg-ok/15 text-ok' : 'bg-danger/15 text-danger'
          }`}
        >
          {message.text}
        </div>
      ) : null}

      {mine ? (
        <div className="flex flex-col gap-2">
          <div className="card text-sm">
            {mine.status === 'CONFIRMED' ? (
              <p className="font-bold text-ok">You're booked. Scan your QR at the gate to check in.</p>
            ) : mine.status === 'CHECKED_IN' ? (
              <p className="font-bold text-ok">Checked in — enjoy the session!</p>
            ) : mine.promotionOfferedAt ? (
              <p className="font-bold text-brand">
                A spot opened up — confirm it before someone else takes it.
              </p>
            ) : (
              <p className="font-bold text-warn">Waitlist position #{mine.waitlistPosition}.</p>
            )}
          </div>
          {mine.status === 'WAITLIST' && mine.promotionOfferedAt ? (
            <button
              className="btn-brand"
              disabled={confirmBusy}
              onClick={() => void onConfirmSpot(mine.id)}
            >
              Confirm spot
            </button>
          ) : null}
          {['CONFIRMED', 'WAITLIST'].includes(mine.status) ? (
            <button className="btn-ghost text-danger" disabled={cancel.isPending} onClick={() => void onCancel()}>
              {mine.status === 'WAITLIST' ? 'Leave waitlist' : 'Cancel booking'}
            </button>
          ) : null}
        </div>
      ) : bookable ? (
        <button className="btn-brand" disabled={book.isPending} onClick={() => void onBook()}>
          {v.spotsLeft > 0 ? 'Book this class' : 'Join waitlist'}
        </button>
      ) : (
        <div className="card text-sm text-muted">This class is {v.session.status.toLowerCase()}.</div>
      )}

      <Link to="/bookings" className="text-center text-sm font-bold text-brand">
        My bookings →
      </Link>
    </div>
  );
}
