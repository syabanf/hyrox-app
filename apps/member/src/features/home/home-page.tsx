import { Spinner, formatDay, formatDayTime, formatDuration, formatTime } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { CalendarDays, Dumbbell, Flag, QrCode, Trophy } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { api } from '../../lib/api';
import { useT } from '../../lib/i18n';
import { classImage } from '../../lib/images';
import { useMe, useMyBookings } from '../../lib/queries';

const useHome = () => useQuery({ queryKey: ['home'], queryFn: api.me.home });

/** Quiet, uniform section header: tiny caps label + optional trailing link. */
function SectionHeader({ label, action }: { label: string; action?: ReactNode }) {
  return (
    <div className="mb-3 flex items-baseline justify-between px-1">
      <p className="text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">{label}</p>
      {action}
    </div>
  );
}

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
    <div className="flex flex-col gap-8 pt-2">
      <div>
        <p className="text-sm font-semibold text-muted">Hey,</p>
        <p className="display text-3xl leading-tight">{me.member.fullName.split(' ')[0]}</p>
      </div>

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

      {/* Race spotlight — photo card */}
      {home?.spotlightRace ? (
        <Link
          to="/races"
          className="card relative block overflow-hidden !border-0 !p-0 text-white"
        >
          {home.spotlightRace.imageUrl ? (
            <img
              src={home.spotlightRace.imageUrl}
              alt=""
              className="h-52 w-full object-cover"
              loading="lazy"
            />
          ) : (
            <div className="surface-brand h-52 w-full" />
          )}
          <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/25 to-black/10" />
          <div className="absolute inset-x-0 bottom-0 p-5">
            <p className="text-[11px] font-extrabold uppercase tracking-[0.14em] text-brand" style={{ color: '#ff5a5f' }}>
              {home.spotlightRace.joined ? t('Race day') : t('Next race near you')} ·{' '}
              {formatDay(home.spotlightRace.startsAt)}
            </p>
            <p className="display text-3xl leading-tight">{home.spotlightRace.name}</p>
            <div className="mt-2 flex items-center gap-2">
              <span className="rounded-full bg-white/20 px-3 py-1 text-xs font-extrabold backdrop-blur">
                {home.spotlightRace.daysToRace} {t('days away')}
              </span>
              {home.spotlightRace.joined && home.spotlightRace.goalSec ? (
                <span className="rounded-full bg-white/20 px-3 py-1 text-xs font-extrabold backdrop-blur">
                  {t('Goal')} {formatDuration(home.spotlightRace.goalSec)}
                </span>
              ) : !home.spotlightRace.joined ? (
                <span className="rounded-full bg-brand px-3 py-1 text-xs font-extrabold">
                  {t('Add to my races')}
                </span>
              ) : null}
            </div>
          </div>
        </Link>
      ) : null}

      {/* Promos */}
      {home && home.promos.length > 0 ? (
        <section>
          <SectionHeader label={t('Promos')} />
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

      {/* Announcements — one calm card, hairline-divided rows */}
      {home && home.announcements.length > 0 ? (
        <section>
          <SectionHeader label={t('Announcements')} />
          <div className="card divide-y divide-line !py-1">
            {home.announcements.slice(0, 3).map((a) => {
              const inner = (
                <>
                  <div className="flex items-baseline justify-between gap-3">
                    <p className="text-sm font-extrabold">{a.title}</p>
                    <p className="shrink-0 text-xs text-muted/70">{formatDay(a.createdAt)}</p>
                  </div>
                  <p className="mt-0.5 text-sm text-muted">{a.message}</p>
                </>
              );
              return a.deepLink ? (
                <Link key={a.id} to={a.deepLink} className="block py-3.5">
                  {inner}
                </Link>
              ) : (
                <div key={a.id} className="py-3.5">
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
          <SectionHeader
            label={home.railDay === 'TODAY' ? t('Today at the studio') : t('Tomorrow at the studio')}
            action={
              <Link to="/classes" className="text-xs font-extrabold text-brand">
                {t('Schedule')}
              </Link>
            }
          />
          <div className="-mx-4 flex snap-x gap-3 overflow-x-auto px-4 pb-1">
            {home.todaySessions.map((v) => {
              const image = classImage(v.session.classTypeId);
              return (
                <Link
                  key={v.session.id}
                  to={`/classes/${v.session.id}`}
                  className="card min-w-48 shrink-0 snap-start overflow-hidden !p-0"
                >
                  {image ? (
                    <div className="relative h-24 w-full">
                      <img src={image} alt="" className="h-full w-full object-cover" loading="lazy" />
                      <span className="display absolute bottom-2 left-3 text-xl text-white drop-shadow-[0_1px_4px_rgb(0_0_0/0.6)]">
                        {formatTime(v.session.startsAt)}
                      </span>
                    </div>
                  ) : (
                    <p className="display px-3.5 pt-3 text-xl">{formatTime(v.session.startsAt)}</p>
                  )}
                  <div className="p-3.5 pt-2.5">
                    <p className="truncate text-sm font-extrabold">{v.classTypeName}</p>
                    <p className="truncate text-xs text-muted">
                      {v.branchName} · {v.coachName}
                    </p>
                    <p
                      className={`mt-1.5 text-xs font-extrabold ${
                        v.myBooking ? 'text-ok' : v.spotsLeft > 0 ? 'text-brand' : 'text-warn'
                      }`}
                    >
                      {v.myBooking ? t('Booked') : v.spotsLeft > 0 ? `${v.spotsLeft} ${t('left')}` : t('Full · WL')}
                    </p>
                  </div>
                </Link>
              );
            })}
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

      {/* Upcoming bookings — one calm card, hairline-divided rows */}
      <section>
        <SectionHeader
          label={t('Upcoming')}
          action={
            <Link to="/bookings" className="text-xs font-extrabold text-brand">
              {t('All bookings')}
            </Link>
          }
        />
        {upcoming.length === 0 ? (
          <div className="card text-sm text-muted">
            {t('Nothing booked yet.')}{' '}
            <Link to="/classes" className="font-bold text-brand">
              {t('Browse the schedule →')}
            </Link>
          </div>
        ) : (
          <div className="card divide-y divide-line !py-1">
            {upcoming.map((b) => (
              <Link
                key={b.booking.id}
                to={`/classes/${b.session.id}`}
                className="flex items-center justify-between gap-3 py-3.5"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-extrabold">{b.classTypeName}</p>
                  <p className="truncate text-sm text-muted">
                    {formatDayTime(b.session.startsAt)} · {b.branchName}
                  </p>
                </div>
                <span
                  className={`shrink-0 text-xs font-extrabold ${
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
