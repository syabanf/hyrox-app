'use client';

import type { Permission } from '@hyrox/domain';
import {
  BarChart3,
  CalendarDays,
  CreditCard,
  DoorOpen,
  Dumbbell,
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
    items: [{ href: '/engagement', label: 'Campaigns', icon: Megaphone, permission: 'engagement.view' }],
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

  return (
    <div className="flex min-h-dvh">
      <aside className="fixed inset-y-0 left-0 flex w-60 flex-col border-r border-black/40 bg-ink-soft text-white">
        <div className="px-5 py-5">
          <Link href="/dashboard" className="display text-lg font-black">
            HYROX<span className="text-brand">STUDIO</span>
          </Link>
          <p className="text-[11px] font-bold uppercase tracking-widest text-muted">Admin Panel</p>
        </div>
        <nav className="flex-1 overflow-y-auto px-3 pb-4">
          {NAV.map((group) => {
            const visible = group.items.filter((i) => can(i.permission));
            if (visible.length === 0) return null;
            return (
              <div key={group.label ?? 'root'} className="mb-4">
                {group.label ? (
                  <p className="px-2 pb-1 text-[10px] font-black uppercase tracking-widest text-muted/60">
                    {group.label}
                  </p>
                ) : null}
                {visible.map(({ href, label, icon: Icon }) => {
                  const active = pathname === href || pathname.startsWith(`${href}/`);
                  return (
                    <Link
                      key={href}
                      href={href}
                      className={`mb-0.5 flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm font-bold ${
                        active ? 'bg-brand text-white' : 'text-[#c9c9c9] hover:bg-surface-raised'
                      }`}
                    >
                      <Icon size={16} />
                      {label}
                    </Link>
                  );
                })}
              </div>
            );
          })}
        </nav>
        <div className="border-t border-line p-4 text-sm">
          <p className="font-black">{user.name}</p>
          <p className="mb-3 text-xs font-bold uppercase tracking-wide text-brand">
            {user.role.replaceAll('_', ' ')}
          </p>
          <div className="flex gap-2">
            <button onClick={logout} className="a-btn-ghost flex-1 !px-2 !py-1.5 text-xs">
              <LogOut size={13} /> Sign out
            </button>
            <button
              onClick={() => void resetDemo()}
              className="a-btn-ghost !px-2 !py-1.5 text-xs"
              title="Reset demo data"
            >
              <RotateCcw size={13} />
            </button>
          </div>
        </div>
      </aside>
      <main className="ml-60 min-w-0 flex-1 px-8 py-6">{children}</main>
    </div>
  );
}
