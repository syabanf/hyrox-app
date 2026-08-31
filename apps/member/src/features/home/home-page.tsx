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

      {/* Credit balance — premium "black card" */}
      <Link to="/wallet" className="card surface-ink relative block overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
        <div className="pointer-events-none absolute -bottom-28 -left-10 h-48 w-48 rounded-full bg-white/[0.04] blur-2xl" />
        <div className="relative flex items-start justify-between">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
              {t('Credit balance')}
            </p>
            <p className="display mt-1 text-7xl leading-none">{me.balance}</p>
          </div>
          <span className="flex h-11 w-11 items-center justify-center rounded-2xl bg-white/10">
            <QrCode size={22} className="text-white/80" />
          </span>
        </div>
        <div className="relative mt-5 flex items-center justify-between">
          <p className="text-xs font-semibold text-white/45">{me.member.fullName}</p>
          {me.lowBalance ? (
            <span className="chip bg-brand text-white">{t('Top up')}</span>
          ) : me.expiringCredits > 0 ? (
            <span className="chip bg-white/10 text-white/80">
              {me.expiringCredits} {t('expiring soon')}
            </span>
          ) : (
            <span className="chip bg-white/10 text-white/70">{t('Wallet')}</span>
          )}
        </div>
      </Link>

      {/* Quick actions — each with its own soft accent tint */}
      <div className="grid grid-cols-2 gap-3">
        {[
          { to: '/qr', icon: QrCode, label: t('Check in'), tint: 'surface-brand text-white' },
          { to: '/classes', icon: CalendarDays, label: t('Book a class'), tint: 'bg-[#1b1b1f] text-white' },
          { to: '/workout', icon: Dumbbell, label: t('Generate workout'), tint: 'bg-[#1b1b1f] text-white' },
          { to: '/races', icon: Flag, label: t('Races'), tint: 'bg-[#1b1b1f] text-white' },
        ].map(({ to, icon: Icon, label, tint }) => (
          <Link key={to} to={to} className="card flex items-center gap-3 !p-4 active:scale-[0.98]">
            <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${tint}`}>
              <Icon size={19} strokeWidth={2.2} />
            </span>
            <span className="text-sm font-bold leading-tight">{label}</span>
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
          <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/30 to-black/5" />
          <div className="absolute inset-x-0 bottom-0 p-6">
            <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/60">
              {home.spotlightRace.joined ? t('Race day') : t('Next race near you')} ·{' '}
              {formatDay(home.spotlightRace.startsAt)}
            </p>
            <p className="display mt-0.5 text-3xl leading-tight">{home.spotlightRace.name}</p>
            <div className="mt-3 flex items-center gap-2">
              <span className="chip bg-white/15 text-white backdrop-blur">
                {home.spotlightRace.daysToRace} {t('days away')}
              </span>
              {home.spotlightRace.joined && home.spotlightRace.goalSec ? (
                <span className="chip bg-white/15 text-white backdrop-blur">
                  {t('Goal')} {formatDuration(home.spotlightRace.goalSec)}
                </span>
              ) : !home.spotlightRace.joined ? (
                <span className="chip bg-brand text-white">{t('Add to my races')}</span>
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
                className="card surface-ink relative min-w-64 shrink-0 snap-start overflow-hidden !border-0 text-white"
              >
                <div className="pointer-events-none absolute -right-12 -top-16 h-40 w-40 rounded-full bg-brand/20 blur-3xl" />
                <div className="relative flex items-start justify-between gap-2">
                  <p className="display text-3xl leading-none">{p.label}</p>
                  <span className="rounded-full bg-white/10 px-2.5 py-1 font-mono text-[11px] font-bold tracking-wider text-white/80">
                    {p.code}
                  </span>
                </div>
                <p className="relative mt-2 text-sm font-medium text-white/60">{p.description}</p>
                <div className="relative mt-4 flex items-center justify-between text-xs">
                  <span className="font-semibold text-white/40">
                    {t('Until')} {formatDay(p.endsAt)}
                    {p.newMembersOnly ? ` · ${t('new members')}` : ''}
                  </span>
                  <span className="chip bg-brand text-white">{t('Use it')}</span>
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
            {home.announcements.slice(0, 4).map((a, i) => {
              const dot = i === 0 ? 'bg-brand' : 'bg-ink/25';
              const inner = (
                <div className="flex gap-3">
                  <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${dot}`} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-baseline justify-between gap-3">
                      <p className="text-sm font-extrabold">{a.title}</p>
                      <p className="shrink-0 text-xs text-muted/70">{formatDay(a.createdAt)}</p>
                    </div>
                    <p className="mt-0.5 text-sm text-muted">{a.message}</p>
                  </div>
                  {a.imageUrl ? (
                    <img
                      src={a.imageUrl}
                      alt=""
                      className="h-14 w-14 shrink-0 self-center rounded-xl object-cover"
                      loading="lazy"
                    />
                  ) : null}
                </div>
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
              <Link to="/classes" className="text-xs font-bold text-ink/50">
                {t('Schedule')} →
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
                      className={`mt-1.5 text-xs font-bold ${
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
        <Link to="/train/explore" className="card flex items-center gap-4 !py-4">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[#1b1b1f] text-white">
            <Trophy size={19} strokeWidth={2.2} />
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-bold">{home.challenge.name}</p>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-surface-raised">
              <div
                className="h-full rounded-full bg-gradient-to-r from-brand to-[#ff7a45]"
                style={{
                  width: `${Math.min(100, (home.challenge.progressKm / home.challenge.targetKm) * 100)}%`,
                }}
              />
            </div>
          </div>
          <p className="shrink-0 text-right text-xs font-bold">
            {home.challenge.progressKm.toFixed(1)}
            <span className="font-semibold text-muted"> / {home.challenge.targetKm} km</span>
          </p>
        </Link>
      ) : null}

      {/* Upcoming bookings — one calm card, hairline-divided rows */}
      <section>
        <SectionHeader
          label={t('Upcoming')}
          action={
            <Link to="/bookings" className="text-xs font-bold text-ink/50">
              {t('All bookings')} →
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
            {upcoming.map((b) => {
              const d = new Date(b.session.startsAt);
              return (
                <Link
                  key={b.booking.id}
                  to={`/classes/${b.session.id}`}
                  className="flex items-center gap-3 py-3.5"
                >
                  <span className="flex h-12 w-12 shrink-0 flex-col items-center justify-center rounded-2xl bg-brand/[0.08]">
                    <span className="display text-lg leading-none text-brand">{d.getDate()}</span>
                    <span className="text-[9px] font-extrabold uppercase tracking-wide text-brand/70">
                      {d.toLocaleDateString(undefined, { month: 'short' })}
                    </span>
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-extrabold">{b.classTypeName}</p>
                    <p className="truncate text-sm text-muted">
                      {formatDayTime(b.session.startsAt)} · {b.branchName}
                    </p>
                  </div>
                  <span
                    className={`chip shrink-0 ${
                      b.booking.status === 'CONFIRMED'
                        ? 'bg-ok/10 text-ok'
                        : 'bg-warn/10 text-warn'
                    }`}
                  >
                    {b.booking.status === 'WAITLIST' ? `WL #${b.booking.waitlistPosition}` : t('Booked')}
                  </span>
                </Link>
              );
            })}
          </div>
        )}
      </section>

      {/* Community finisher — a splash of brand at the end of the page */}
      <Link
        to="/train"
        className="card surface-brand relative block overflow-hidden !border-0 !p-6 text-white"
      >
        <div className="pointer-events-none absolute -right-14 -top-20 h-48 w-48 rounded-full bg-white/15 blur-3xl" />
        <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/60">
          {t('Community')}
        </p>
        <p className="display mt-1 text-2xl leading-tight">{t('See what your crew is training')}</p>
        <span className="chip mt-4 bg-white/15 text-white backdrop-blur">{t('Open Train')} →</span>
      </Link>

      <p className="display pb-2 text-center text-4xl text-ink/[0.06]">HYROXSTUDIO</p>
    </div>
  );
}
