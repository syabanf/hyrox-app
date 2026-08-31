import { Activity, Bell, CalendarDays, Home, QrCode, Search, User } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link, NavLink, Outlet, useLocation, Navigate } from 'react-router';
import { useAuthStore } from '../lib/auth';
import { useT } from '../lib/i18n';
import { useMe } from '../lib/queries';
import { DevDrawer } from './dev-drawer';
import { OfflineBanner } from './offline-banner';

export function RequireAuth({ children }: { children: ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const location = useLocation();
  if (!token) return <Navigate to="/auth/login" replace state={{ from: location.pathname }} />;
  return <>{children}</>;
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
  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col">
      <OfflineBanner />
      <header className="pointer-events-none fixed inset-x-0 top-0 z-20 mx-auto flex max-w-md items-center justify-between bg-gradient-to-b from-[#f6f6f2] via-[#f6f6f2]/75 to-transparent px-5 pb-4 pt-[max(env(safe-area-inset-top),1.1rem)] [&_a]:pointer-events-auto">
        <div className="flex items-center gap-2.5">
          <Link
            to="/profile"
            aria-label="Profile"
            className="block h-10 w-10 shrink-0 overflow-hidden rounded-full bg-[#1b1b1f] shadow-[0_4px_14px_rgb(17_17_20/0.18)]"
          >
            {me?.member.avatarUrl ? (
              <img src={me.member.avatarUrl} alt="" className="h-full w-full object-cover" />
            ) : (
              <span className="flex h-full w-full items-center justify-center text-sm font-black text-white">
                {me?.member.fullName
                  .split(' ')
                  .slice(0, 2)
                  .map((p) => p[0])
                  .join('') ?? ''}
              </span>
            )}
          </Link>
          <Link
            to="/train/explore"
            aria-label="Explore"
            className="flex h-10 w-10 items-center justify-center rounded-full bg-[#1b1b1f] text-white shadow-[0_4px_14px_rgb(17_17_20/0.18)]"
          >
            <Search size={17} strokeWidth={2.4} />
          </Link>
        </div>
        <Link
          to="/notifications"
          className="relative flex h-10 w-10 items-center justify-center rounded-full bg-[#1b1b1f] text-white shadow-[0_4px_14px_rgb(17_17_20/0.18)]"
          aria-label="Notifications"
        >
          <Bell size={17} strokeWidth={2.4} />
          {me && me.unreadNotifications > 0 ? (
            <span className="absolute -right-1 -top-1 flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-brand px-1 text-[10px] font-black text-white ring-2 ring-[#f6f6f2]">
              {me.unreadNotifications}
            </span>
          ) : null}
        </Link>
      </header>
      <main className="flex-1 px-5 pb-28 pt-[4.75rem]">
        <Outlet />
      </main>
      <nav className="fixed inset-x-0 bottom-0 z-20 mx-auto max-w-md px-5 pb-[max(env(safe-area-inset-bottom),1.25rem)]">
        <div className="surface-ink flex items-center justify-between rounded-full px-3 py-2.5 shadow-[0_18px_40px_rgb(13_13_16/0.35)]">
          {NAV.map(({ to, label, icon: Icon, emphasized }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              aria-label={t(label)}
              title={t(label)}
              className={({ isActive }) =>
                emphasized
                  ? 'surface-brand flex h-12 w-12 items-center justify-center rounded-full text-white shadow-[0_6px_18px_rgb(237_28_36/0.45)] transition active:scale-95'
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
