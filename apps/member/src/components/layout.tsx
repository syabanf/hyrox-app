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
      <main className="flex-1 px-4 pb-32">
        <Outlet />
      </main>
      <nav className="fixed inset-x-0 bottom-0 z-20 mx-auto max-w-md px-3 pb-[max(env(safe-area-inset-bottom),0.75rem)]">
        <div className="flex items-end justify-around rounded-[1.75rem] border border-black/5 bg-white/90 px-2 pb-2 pt-2 shadow-[0_10px_36px_rgb(0_0_0/0.14)] backdrop-blur-xl">
          {NAV.map(({ to, label, icon: Icon, emphasized }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                emphasized
                  ? 'flex -translate-y-4 flex-col items-center gap-1'
                  : `flex flex-col items-center gap-0.5 px-2 py-1 text-[11px] font-bold ${
                      isActive ? 'text-brand' : 'text-muted'
                    }`
              }
            >
              {({ isActive }) =>
                emphasized ? (
                  <>
                    <span
                      className={`flex h-14 w-14 items-center justify-center rounded-[1.35rem] shadow-[0_8px_20px_rgb(237_28_36/0.35)] ${
                        isActive ? 'surface-brand text-white' : 'bg-brand text-white'
                      }`}
                    >
                      <Icon size={26} strokeWidth={2.5} />
                    </span>
                    <span className="text-[11px] font-bold text-muted">{t(label)}</span>
                  </>
                ) : (
                  <>
                    <span
                      className={`flex h-9 w-9 items-center justify-center rounded-xl transition ${
                        isActive ? 'bg-brand/10' : ''
                      }`}
                    >
                      <Icon size={20} strokeWidth={isActive ? 2.5 : 2} />
                    </span>
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
