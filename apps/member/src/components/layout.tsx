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
      <header className="flex items-center justify-between px-4 pt-4 pb-2">
        <Link to="/" className="display text-xl font-black">
          HYROX<span className="text-brand">STUDIO</span>
        </Link>
        <Link
          to="/notifications"
          className="relative rounded-full border border-line bg-surface p-2"
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
      <main className="flex-1 px-4 pb-28">
        <Outlet />
      </main>
      <nav className="fixed inset-x-0 bottom-0 z-20 mx-auto max-w-md border-t border-line bg-white/95 backdrop-blur">
        <div className="flex items-end justify-around px-2 pb-[max(env(safe-area-inset-bottom),0.5rem)] pt-2">
          {NAV.map(({ to, label, icon: Icon, emphasized }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                emphasized
                  ? 'flex -translate-y-3 flex-col items-center gap-1'
                  : `flex flex-col items-center gap-1 px-3 py-1 text-[11px] font-bold uppercase tracking-wide ${
                      isActive ? 'text-brand' : 'text-muted'
                    }`
              }
            >
              {({ isActive }) =>
                emphasized ? (
                  <>
                    <span
                      className={`flex h-14 w-14 items-center justify-center rounded-full border-4 border-white shadow-[0_2px_8px_rgb(0_0_0/0.12)] ${
                        isActive ? 'bg-brand text-white' : 'bg-surface text-brand'
                      }`}
                    >
                      <Icon size={26} strokeWidth={2.5} />
                    </span>
                    <span className="text-[11px] font-bold uppercase tracking-wide text-muted">
                      {t(label)}
                    </span>
                  </>
                ) : (
                  <>
                    <Icon size={20} strokeWidth={isActive ? 2.5 : 2} />
                    <span>{t(label)}</span>
                  </>
                )
              }
            </NavLink>
          ))}
        </div>
      </nav>
      {import.meta.env.DEV ? <DevDrawer /> : null}
    </div>
  );
}
