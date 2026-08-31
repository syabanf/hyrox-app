'use client';

import type { Permission } from '@hyrox/domain';
import {
  BarChart3,
  CalendarDays,
  CreditCard,
  DoorOpen,
  Dumbbell,
  Flag,
  LayoutDashboard,
  LogOut,
  Megaphone,
  RotateCcw,
  Settings,
  Ticket,
  Users,
  Wallet,
  ClipboardList,
  ScanLine,
} from 'lucide-react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useEffect, type ReactNode } from 'react';
import { api } from '../../lib/api';
import { useAdminAuth, usePermissions } from '../../lib/auth';

interface NavItem {
  href: string;
  label: string;
  icon: typeof Users;
  permission: Permission;
}
interface NavGroup {
  label: string | null;
  items: NavItem[];
}

const NAV: NavGroup[] = [
  {
    label: null,
    items: [{ href: '/dashboard', label: 'Dashboard', icon: LayoutDashboard, permission: 'dashboard.view' }],
  },
  {
    label: 'Members',
    items: [{ href: '/members', label: 'Members', icon: Users, permission: 'members.view' }],
  },
  {
    label: 'Operations',
    items: [
      { href: '/operations/schedule', label: 'Schedule', icon: CalendarDays, permission: 'operations.view' },
      { href: '/operations/sessions', label: 'Class Sessions', icon: ClipboardList, permission: 'operations.view' },
      { href: '/operations/class-types', label: 'Class Types', icon: Dumbbell, permission: 'operations.view' },
      { href: '/operations/bookings', label: 'Bookings', icon: ClipboardList, permission: 'operations.view' },
      { href: '/operations/coaches', label: 'Coaches', icon: Users, permission: 'operations.view' },
    ],
  },
  {
    label: 'Access',
    items: [
      { href: '/access/monitor', label: 'Live Check-in', icon: ScanLine, permission: 'access.view' },
      { href: '/access/logs', label: 'Access Logs', icon: DoorOpen, permission: 'access.view' },
    ],
  },
  {
    label: 'Commercial',
    items: [
      { href: '/commercial/packages', label: 'Credit Packages', icon: Wallet, permission: 'commercial.view' },
      { href: '/commercial/payments', label: 'Payments', icon: CreditCard, permission: 'payments.view' },
      { href: '/commercial/vouchers', label: 'Vouchers', icon: Ticket, permission: 'commercial.view' },
    ],
  },
  {
    label: 'Engagement',
    items: [
      { href: '/engagement', label: 'Campaigns', icon: Megaphone, permission: 'engagement.view' },
      { href: '/engagement/races', label: 'Race Events', icon: Flag, permission: 'engagement.view' },
    ],
  },
  {
    label: 'Insights',
    items: [
      { href: '/reports', label: 'Reports', icon: BarChart3, permission: 'reports.view' },
      { href: '/config', label: 'Configuration', icon: Settings, permission: 'config.view' },
    ],
  },
];

export default function AppLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, token, clear } = useAdminAuth();
  const { can } = usePermissions();

  useEffect(() => {
    if (!token) router.replace('/login');
  }, [token, router]);
  if (!token || !user) return null;

  const logout = () => {
    clear();
    router.replace('/login');
  };

  const resetDemo = async () => {
    await api.dev.reset();
    clear();
    location.href = '/login';
  };

  // Highlight only the deepest matching nav item (so /engagement/races doesn't
  // also light up /engagement).
  const allHrefs = NAV.flatMap((g) => g.items.map((i) => i.href));
  const activeHref = allHrefs
    .filter((h) => pathname === h || pathname.startsWith(`${h}/`))
    .sort((a, b) => b.length - a.length)[0];

  return (
    <div className="flex min-h-dvh">
      <aside className="surface-ink fixed inset-y-0 left-0 flex w-60 flex-col text-white">
        <div className="px-6 pb-4 pt-6">
          <Link href="/dashboard" className="display text-lg font-black">
            HYROX<span className="text-[#ff4348]">STUDIO</span>
          </Link>
          <p className="mt-0.5 text-[10px] font-bold uppercase tracking-[0.22em] text-white/35">
            Admin Panel
          </p>
        </div>
        <nav className="flex-1 overflow-y-auto px-3 pb-4">
          {NAV.map((group) => {
            const visible = group.items.filter((i) => can(i.permission));
            if (visible.length === 0) return null;
            return (
              <div key={group.label ?? 'root'} className="mb-5">
                {group.label ? (
                  <p className="px-3 pb-1.5 text-[10px] font-extrabold uppercase tracking-[0.18em] text-white/30">
                    {group.label}
                  </p>
                ) : null}
                {visible.map(({ href, label, icon: Icon }) => {
                  const active = href === activeHref;
                  return (
                    <Link
                      key={href}
                      href={href}
                      className={`mb-0.5 flex items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-bold transition ${
                        active
                          ? 'bg-white/10 text-white'
                          : 'text-white/45 hover:bg-white/5 hover:text-white/80'
                      }`}
                    >
                      <Icon size={16} className={active ? 'text-[#ff4348]' : undefined} />
                      {label}
                    </Link>
                  );
                })}
              </div>
            );
          })}
        </nav>
        <div className="border-t border-white/10 p-4 text-sm">
          <p className="font-black">{user.name}</p>
          <p className="mb-3 text-[11px] font-bold uppercase tracking-wide text-[#ff4348]">
            {user.role.replaceAll('_', ' ')}
          </p>
          <div className="flex gap-2">
            <button
              onClick={logout}
              className="flex flex-1 items-center justify-center gap-1.5 rounded-xl bg-white/10 px-2 py-2 text-xs font-bold text-white/80 transition hover:bg-white/15"
            >
              <LogOut size={13} /> Sign out
            </button>
            <button
              onClick={() => void resetDemo()}
              className="flex items-center justify-center rounded-xl bg-white/10 px-2.5 py-2 text-white/80 transition hover:bg-white/15"
              title="Reset demo data"
            >
              <RotateCcw size={13} />
            </button>
          </div>
        </div>
      </aside>
      <main className="ml-60 min-w-0 flex-1 px-8 py-7">{children}</main>
    </div>
  );
}
