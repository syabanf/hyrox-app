import { formatDayTime } from '@hyrox/ui';
import { Spinner } from '@hyrox/ui';
import { CalendarDays, Dumbbell, Flag, QrCode, Wallet } from 'lucide-react';
import { Link } from 'react-router';
import { useT } from '../../lib/i18n';
import { useMe, useMyBookings } from '../../lib/queries';

export function HomePage() {
  const t = useT();
  const { data: me, isLoading } = useMe();
  const { data: bookings } = useMyBookings();

  if (isLoading || !me) return <Spinner label="Loading…" />;

  const upcoming = (bookings ?? [])
    .filter(
      (b) =>
        ['CONFIRMED', 'WAITLIST'].includes(b.booking.status) &&
        new Date(b.session.startsAt).getTime() > Date.now(),
    )
    .sort((a, b) => new Date(a.session.startsAt).getTime() - new Date(b.session.startsAt).getTime())
    .slice(0, 3);

  return (
    <div className="flex flex-col gap-4">
      <p className="text-lg text-muted">
        Hey, <span className="font-black text-ink">{me.member.fullName.split(' ')[0]}</span>
      </p>

      {/* Credit balance card */}
      <Link to="/wallet" className="card block !bg-brand !border-brand">
        <div className="flex items-center justify-between text-white">
          <div>
            <p className="text-xs font-black uppercase tracking-wider opacity-70">Credit balance</p>
            <p className="display text-6xl font-black leading-none">{me.balance}</p>
          </div>
          <Wallet size={40} className="opacity-70" />
        </div>
        {me.expiringCredits > 0 ? (
          <p className="mt-2 rounded-lg bg-black/25 px-3 py-1.5 text-xs font-bold text-white">
            {me.expiringCredits} credit{me.expiringCredits === 1 ? '' : 's'} expiring soon
          </p>
        ) : null}
        {me.lowBalance ? (
          <p className="mt-2 text-sm font-black uppercase text-white">Low balance — top up now</p>
        ) : null}
      </Link>

      <div className="grid grid-cols-2 gap-3">
        {[
          { to: '/qr', icon: QrCode, label: t('Check in') },
          { to: '/classes', icon: CalendarDays, label: t('Book a class') },
          { to: '/workout', icon: Dumbbell, label: t('Generate workout') },
          { to: '/races', icon: Flag, label: t('Races') },
        ].map(({ to, icon: Icon, label }) => (
          <Link key={to} to={to} className="card flex flex-col items-center gap-2 py-5">
            <Icon size={26} className="text-brand" />
            <span className="text-center text-sm font-black uppercase tracking-wide">{label}</span>
          </Link>
        ))}
      </div>

      <section>
        <div className="mb-2 flex items-center justify-between">
          <h2 className="display text-xl font-black">{t('Upcoming')}</h2>
          <Link to="/bookings" className="text-sm font-bold text-brand">
            {t('All bookings')}
          </Link>
        </div>
        {upcoming.length === 0 ? (
          <div className="card text-sm text-muted">
            Nothing booked yet.{' '}
            <Link to="/classes" className="font-bold text-brand">
              Browse the schedule →
            </Link>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {upcoming.map((b) => (
              <Link key={b.booking.id} to={`/classes/${b.session.id}`} className="card flex items-center justify-between">
                <div>
                  <p className="font-black">{b.classTypeName}</p>
                  <p className="text-sm text-muted">
                    {formatDayTime(b.session.startsAt)} · {b.branchName} · {b.coachName}
                  </p>
                </div>
                <span
                  className={`text-xs font-black uppercase ${
                    b.booking.status === 'CONFIRMED' ? 'text-ok' : 'text-warn'
                  }`}
                >
                  {b.booking.status === 'WAITLIST'
                    ? `WL #${b.booking.waitlistPosition}`
                    : 'Booked'}
                </span>
              </Link>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
