import { Spinner, formatDay, formatDayTime, formatTime } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { CalendarDays, Dumbbell, Flag, Megaphone, QrCode, TicketPercent, Trophy } from 'lucide-react';
import { Link } from 'react-router';
import { api } from '../../lib/api';
import { useT } from '../../lib/i18n';
import { useMe, useMyBookings } from '../../lib/queries';

const useHome = () => useQuery({ queryKey: ['home'], queryFn: api.me.home });

export function HomePage() {
  const t = useT();
  const { data: me, isLoading } = useMe();
  const { data: bookings } = useMyBookings();
  const { data: home } = useHome();

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
    <div className="flex flex-col gap-5">
      <p className="text-lg text-muted">
        Hey, <span className="font-black text-ink">{me.member.fullName.split(' ')[0]}</span>
      </p>

      {/* Credit balance card */}
      <Link to="/wallet" className="card surface-brand relative block overflow-hidden !border-0">
        <div className="pointer-events-none absolute -right-10 -top-14 h-44 w-44 rounded-full bg-white/10" />
        <div className="pointer-events-none absolute -bottom-20 right-14 h-40 w-40 rounded-full bg-black/10" />
        <div className="relative flex items-center justify-between text-white">
          <div>
            <p className="text-[11px] font-extrabold uppercase tracking-[0.14em] opacity-80">
              {t('Credit balance')}
            </p>
            <p className="display text-6xl leading-none">{me.balance}</p>
          </div>
          <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-white/15">
            <QrCode size={26} />
          </span>
        </div>
        {me.expiringCredits > 0 ? (
          <p className="mt-2 rounded-lg bg-black/25 px-3 py-1.5 text-xs font-bold text-white">
            {me.expiringCredits} credit{me.expiringCredits === 1 ? '' : 's'} {t('expiring soon')}
          </p>
        ) : null}
        {me.lowBalance ? (
          <p className="mt-2 text-sm font-black uppercase text-white">{t('Low balance — top up now')}</p>
        ) : null}
      </Link>

      {/* Quick actions */}
      <div className="grid grid-cols-2 gap-3">
        {[
          { to: '/qr', icon: QrCode, label: t('Check in') },
          { to: '/classes', icon: CalendarDays, label: t('Book a class') },
          { to: '/workout', icon: Dumbbell, label: t('Generate workout') },
          { to: '/races', icon: Flag, label: t('Races') },
        ].map(({ to, icon: Icon, label }) => (
          <Link key={to} to={to} className="card flex items-center gap-3 !p-3.5 active:scale-[0.98]">
            <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-brand/10 text-brand">
              <Icon size={22} />
            </span>
            <span className="text-sm font-extrabold leading-tight">{label}</span>
          </Link>
        ))}
      </div>

      {/* Promos */}
      {home && home.promos.length > 0 ? (
        <section>
          <h2 className="display mb-2 flex items-center gap-2 text-xl font-black">
            <TicketPercent size={20} className="text-brand" /> {t('Promos')}
          </h2>
          <div className="-mx-4 flex snap-x gap-3 overflow-x-auto px-4 pb-1">
            {home.promos.map((p) => (
              <Link
                key={p.voucherId}
                to={`/wallet/topup?voucher=${encodeURIComponent(p.code)}`}
                className="card surface-brand min-w-64 shrink-0 snap-start !border-0 text-white"
              >
                <div className="flex items-start justify-between gap-2">
                  <p className="display text-3xl leading-none">{p.label}</p>
                  <span className="rounded-full bg-black/25 px-2.5 py-1 font-mono text-xs font-black tracking-wider">
                    {p.code}
                  </span>
                </div>
                <p className="mt-2 text-sm font-bold opacity-90">{p.description}</p>
                <div className="mt-2 flex items-center justify-between text-xs font-bold">
                  <span className="opacity-80">
                    {t('Until')} {formatDay(p.endsAt)}
                    {p.newMembersOnly ? ` · ${t('new members')}` : ''}
                  </span>
                  <span className="rounded-lg bg-white px-2.5 py-1 font-black uppercase text-brand">
                    {t('Use it')}
                  </span>
                </div>
              </Link>
            ))}
          </div>
        </section>
      ) : null}

      {/* Announcements */}
      {home && home.announcements.length > 0 ? (
        <section>
          <h2 className="display mb-2 flex items-center gap-2 text-xl font-black">
            <Megaphone size={20} className="text-brand" /> {t('Announcements')}
          </h2>
          <div className="flex flex-col gap-2">
            {home.announcements.map((a) => {
              const inner = (
                <>
                  <p className="text-sm font-black">{a.title}</p>
                  <p className="text-sm text-muted">{a.message}</p>
                  <p className="mt-1 text-xs text-muted/60">{formatDay(a.createdAt)}</p>
                </>
              );
              return a.deepLink ? (
                <Link key={a.id} to={a.deepLink} className="card block !py-3 active:bg-surface-raised">
                  {inner}
                </Link>
              ) : (
                <div key={a.id} className="card !py-3">
                  {inner}
                </div>
              );
            })}
          </div>
        </section>
      ) : null}

      {/* Today's classes rail */}
      {home && home.todaySessions.length > 0 ? (
        <section>
          <div className="mb-2 flex items-center justify-between">
            <h2 className="display text-xl font-black">
              {home.railDay === 'TODAY' ? t('Today at the studio') : t('Tomorrow at the studio')}
            </h2>
            <Link to="/classes" className="text-sm font-bold text-brand">
              {t('Schedule')}
            </Link>
          </div>
          <div className="-mx-4 flex snap-x gap-3 overflow-x-auto px-4 pb-1">
            {home.todaySessions.map((v) => (
              <Link
                key={v.session.id}
                to={`/classes/${v.session.id}`}
                className="card min-w-44 shrink-0 snap-start !py-3"
              >
                <p className="display text-xl">{formatTime(v.session.startsAt)}</p>
                <p className="truncate text-sm font-black">{v.classTypeName}</p>
                <p className="truncate text-xs text-muted">
                  {v.branchName} · {v.coachName}
                </p>
                <p
                  className={`mt-1.5 text-xs font-black uppercase ${
                    v.myBooking ? 'text-ok' : v.spotsLeft > 0 ? 'text-brand' : 'text-warn'
                  }`}
                >
                  {v.myBooking ? t('Booked') : v.spotsLeft > 0 ? `${v.spotsLeft} ${t('left')}` : t('Full · WL')}
                </p>
              </Link>
            ))}
          </div>
        </section>
      ) : null}

      {/* Challenge progress */}
      {home?.challenge ? (
        <Link to="/train/explore" className="card flex items-center gap-4 !py-3">
          <Trophy size={26} className="shrink-0 text-brand" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-black">{home.challenge.name}</p>
            <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-surface-raised">
              <div
                className="h-full rounded-full bg-brand"
                style={{
                  width: `${Math.min(100, (home.challenge.progressKm / home.challenge.targetKm) * 100)}%`,
                }}
              />
            </div>
          </div>
          <p className="shrink-0 text-right text-xs font-black">
            {home.challenge.progressKm.toFixed(1)}
            <span className="text-muted"> / {home.challenge.targetKm} km</span>
          </p>
        </Link>
      ) : null}

      {/* Upcoming bookings */}
      <section>
        <div className="mb-2 flex items-center justify-between">
          <h2 className="display text-xl font-black">{t('Upcoming')}</h2>
          <Link to="/bookings" className="text-sm font-bold text-brand">
            {t('All bookings')}
          </Link>
        </div>
        {upcoming.length === 0 ? (
          <div className="card text-sm text-muted">
            {t('Nothing booked yet.')}{' '}
            <Link to="/classes" className="font-bold text-brand">
              {t('Browse the schedule →')}
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
                  {b.booking.status === 'WAITLIST' ? `WL #${b.booking.waitlistPosition}` : t('Booked')}
                </span>
              </Link>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
