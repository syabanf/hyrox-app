import { Activity, Bell, CalendarDays, Home, QrCode, Search, User } from 'lucide-react';
import { Spinner } from '@nuhabit/ui';
import { useEffect, useState, type ReactNode } from 'react';
import { Link, NavLink, Outlet, useLocation, Navigate } from 'react-router';
import { api } from '../lib/api';
import { DEMO_IDENTIFIER, useAuthStore } from '../lib/auth';
import { useT } from '../lib/i18n';
import { useMe } from '../lib/queries';
import { DevDrawer } from './dev-drawer';
import { OfflineBanner } from './offline-banner';

/**
 * Opens the app straight on the main page: with no session (and no explicit
 * sign-out before), a demo session is established silently through the same
 * OTP endpoints the login screen uses. The login screen is only shown after
 * Sign out, or if the silent sign-in fails.
 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const signedOut = useAuthStore((s) => s.signedOut);
  const setSession = useAuthStore((s) => s.setSession);
  const location = useLocation();
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (token || signedOut) return;
    let cancelled = false;
    (async () => {
      try {
        const challenge = await api.auth.requestOtp(DEMO_IDENTIFIER);
        const session = await api.auth.verifyOtp(challenge.challengeId, '123456');
        if (!cancelled) setSession(session.token, session.member);
      } catch {
        if (!cancelled) setFailed(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token, signedOut, setSession]);

  if (token) return <>{children}</>;
  if (signedOut || failed)
    return <Navigate to="/auth/login" replace state={{ from: location.pathname }} />;
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <Spinner label="Opening NüHabit…" />
    </div>
  );
}

const NAV = [
  { to: '/', label: 'Home', icon: Home },
  { to: '/classes', label: 'Classes', icon: CalendarDays },
  { to: '/qr', label: 'QR', icon: QrCode, emphasized: true },
  { to: '/train', label: 'Train', icon: Activity },
  { to: '/profile', label: 'Profile', icon: User },
];

export function AppLayout() {
  const { data: me } = useMe();
  const t = useT();
  const location = useLocation();
  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col">
      <OfflineBanner />
      <header className="pointer-events-none fixed inset-x-0 top-0 z-20 mx-auto flex max-w-md items-center justify-between bg-gradient-to-b from-[#f3ece2] via-[#f3ece2]/75 to-transparent px-5 pb-4 pt-[max(env(safe-area-inset-top),1.1rem)] [&_a]:pointer-events-auto">
        <div className="flex items-center gap-2.5">
          <Link
            to="/profile"
            aria-label="Profile"
            className="block h-10 w-10 shrink-0 overflow-hidden rounded-full bg-ink-soft shadow-[0_4px_14px_rgb(0_40_26/0.18)]"
          >
            {me?.member.avatarUrl ? (
              <img src={me.member.avatarUrl} alt="" className="h-full w-full object-cover" />
            ) : me ? (
              <span className="flex h-full w-full items-center justify-center text-sm font-black text-white">
                {me.member.fullName
                  .split(' ')
                  .slice(0, 2)
                  .map((p) => p[0])
                  .join('')}
              </span>
            ) : (
              <span className="flex h-full w-full items-center justify-center">
                <img src="/brand/nuhabit-logo-alt-white.png" alt="" className="h-3.5 w-auto" />
              </span>
            )}
          </Link>
          <Link
            to="/train/explore"
            aria-label="Explore"
            className="flex h-10 w-10 items-center justify-center rounded-full bg-ink-soft text-white shadow-[0_4px_14px_rgb(0_40_26/0.18)]"
          >
            <Search size={17} strokeWidth={2.4} />
          </Link>
        </div>
        {/* Wordmark - black logo on the beige ground. */}
        <img
          src="/brand/nuhabit-logo-black.png"
          alt="NüHabit"
          className="pointer-events-none absolute left-1/2 top-[max(env(safe-area-inset-top),1.1rem)] h-[18px] w-auto -translate-x-1/2 translate-y-[11px]"
        />
        <Link
          to="/notifications"
          className="relative flex h-10 w-10 items-center justify-center rounded-full bg-ink-soft text-white shadow-[0_4px_14px_rgb(0_40_26/0.18)]"
          aria-label="Notifications"
        >
          <Bell size={17} strokeWidth={2.4} />
          {me && me.unreadNotifications > 0 ? (
            <span className="absolute -right-1 -top-1 flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-brand px-1 text-[10px] font-black text-white ring-2 ring-[#f3ece2]">
              {me.unreadNotifications}
            </span>
          ) : null}
        </Link>
      </header>
      <main className="flex-1 px-5 pb-28 pt-[4.75rem]">
        <div key={location.pathname} className="page-enter">
          <Outlet />
        </div>
      </main>
      <nav className="fixed inset-x-0 bottom-0 z-20 mx-auto max-w-md px-5 pb-[max(env(safe-area-inset-bottom),1.25rem)]">
        <div className="surface-ink flex items-center justify-between rounded-full px-3 py-2.5 shadow-[0_18px_40px_rgb(0_40_26/0.35)]">
          {NAV.map(({ to, label, icon: Icon, emphasized }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              aria-label={t(label)}
              title={t(label)}
              className={({ isActive }) =>
                emphasized
                  ? 'surface-brand flex h-12 w-12 items-center justify-center rounded-full text-ink shadow-[0_6px_18px_rgb(218_255_89/0.35)] transition active:scale-95'
                  : `flex h-12 w-12 items-center justify-center rounded-full transition active:scale-95 ${
                      isActive ? 'bg-white/12 text-white' : 'text-white/45'
                    }`
              }
            >
              {({ isActive }) => (
                <Icon
                  size={emphasized ? 22 : 21}
                  strokeWidth={emphasized || isActive ? 2.4 : 2}
                />
              )}
            </NavLink>
          ))}
        </div>
      </nav>
      {import.meta.env.DEV ? <DevDrawer /> : null}
    </div>
  );
}
