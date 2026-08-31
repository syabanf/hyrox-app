import { ApiError } from '@hyrox/api-client';
import { Spinner, formatDayTime } from '@hyrox/ui';
import { ArrowLeft } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';
import { sessionRundown } from '../../lib/rundown';
import { useBookMutation, useCancelMutation, useInvalidateAll, useSession , useWallet } from '../../lib/queries';

export function SessionDetailPage() {
  const { sessionId = '' } = useParams();
  const navigate = useNavigate();
  const { data: v, isLoading } = useSession(sessionId);
  const { data: wallet } = useWallet();
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
  // Package coverage indicator: null = no package credits (no restriction).
  const activePkgs = (wallet?.myPackages ?? []).filter((p) => p.active);
  const covered =
    activePkgs.length === 0
      ? null
      : activePkgs.some(
          (p) => p.coverageIds === null || p.coverageIds.includes(v.session.classTypeId),
        );

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

      {covered === false ? (
        <div className="rounded-xl bg-warn/15 p-3 text-sm font-bold text-warn">
          None of your packages cover this class — top up with a package that includes it.
        </div>
      ) : covered === true ? (
        <p className="chip self-start bg-ok/10 text-ok">Covered by your package</p>
      ) : null}

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

      <section className="card">
        <p className="label">Rundown</p>
        <div className="flex flex-col">
          {sessionRundown(v.session.classTypeId, v.session.startsAt, v.session.endsAt).map(
            (item, i, arr) => (
              <div key={item.label} className="flex gap-3">
                <p className="w-14 shrink-0 pt-0.5 text-right font-mono text-xs font-bold text-muted">
                  {item.startsAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                </p>
                <div className="flex flex-col items-center">
                  <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${i === 0 ? 'bg-brand' : 'bg-ink/25'}`} />
                  {i < arr.length - 1 ? <span className="w-px flex-1 bg-line" /> : null}
                </div>
                <div className="pb-4">
                  <p className="text-sm font-extrabold leading-tight">{item.label}</p>
                  <p className="text-xs text-muted">{item.minutes} min</p>
                </div>
              </div>
            ),
          )}
        </div>
      </section>

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
