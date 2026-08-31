import { Activity, Bell, CalendarDays, Home, QrCode, User } from 'lucide-react';
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
      <header className="flex items-center justify-between px-5 pt-5 pb-3">
        <Link to="/" className="display text-xl font-black">
          HYROX<span className="text-brand">STUDIO</span>
        </Link>
        <Link
          to="/notifications"
          className="relative rounded-full border border-line bg-surface p-2.5"
          aria-label="Notifications"
        >
          <Bell size={18} />
          {me && me.unreadNotifications > 0 ? (
            <span className="absolute -top-1 -right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-brand px-1 text-[10px] font-black text-white">
              {me.unreadNotifications}
            </span>
          ) : null}
        </Link>
      </header>
      <main className="flex-1 px-5 pt-1 pb-28">
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
