'use client';

import type { Permission } from '@nuhabit/domain';
import {
  BarChart3,
  CalendarDays,
  CreditCard,
  DoorOpen,
  Dumbbell,
  Flag,
  HandCoins,
  Trophy,
  Video,
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
  CalendarCheck,
  CalendarClock,
  Clock,
  IdCard,
  Award,
  Boxes,
  Undo2,
  Link2,
  Banknote,
  PackageCheck,
  Settings2,
  Tag,
  ChartColumn,
  Star,
  MessagesSquare,
  Building2,
  ClipboardCheck,
  Gift,
  Layers,
  Medal,
  Package,
  Receipt,
  ScrollText,
  ShoppingCart,
  Store,
  Truck,
  Warehouse,
} from 'lucide-react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { ChevronLeft, ChevronRight, Menu, Search, X } from 'lucide-react';
import {
  PageHeaderProvider,
  usePageActionSlot,
  usePageHeader,
} from '../../components/page-header';
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
      { href: '/operations/exercises', label: 'Exercise Guides', icon: Video, permission: 'operations.view' },
      { href: '/operations/incentives', label: 'Coach Incentives', icon: HandCoins, permission: 'incentives.view' },
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
      { href: '/engagement/challenges', label: 'Challenges', icon: Trophy, permission: 'engagement.view' },
    ],
  },
  {
    label: 'Counter',
    items: [
      { href: '/counter', label: 'Till', icon: Store, permission: 'pos.sell' },
      { href: '/counter/sales', label: 'Sales', icon: Receipt, permission: 'pos.view' },
      { href: '/counter/products', label: 'Catalogue', icon: ShoppingCart, permission: 'pos.view' },
      { href: '/counter/shifts', label: 'Till Shifts', icon: ClipboardCheck, permission: 'pos.view' },
      { href: '/counter/offers', label: 'Offers', icon: Tag, permission: 'pos.view' },
      { href: '/counter/gift-cards', label: 'Gift Cards', icon: CreditCard, permission: 'pos.view' },
      { href: '/counter/reports', label: 'Till Reports', icon: ChartColumn, permission: 'pos.view' },
      { href: '/counter/settings', label: 'Counter Setup', icon: Settings2, permission: 'pos.view' },
    ],
  },
  {
    label: 'Stock',
    items: [
      { href: '/inventory', label: 'Stock Levels', icon: Warehouse, permission: 'inventory.view' },
      { href: '/inventory/items', label: 'Catalogue', icon: Package, permission: 'inventory.view' },
      { href: '/inventory/movements', label: 'Stock Ledger', icon: ScrollText, permission: 'inventory.view' },
      { href: '/inventory/counts', label: 'Stock Takes', icon: Boxes, permission: 'inventory.view' },
      { href: '/inventory/expiry', label: 'Dates', icon: CalendarClock, permission: 'inventory.view' },
    ],
  },
  {
    label: 'Purchasing',
    items: [
      { href: '/purchasing', label: 'Purchase Orders', icon: Truck, permission: 'purchasing.view' },
      { href: '/purchasing/requests', label: 'Requests', icon: Layers, permission: 'purchasing.view' },
      { href: '/purchasing/deliveries', label: 'Receiving Bay', icon: PackageCheck, permission: 'purchasing.view' },
      { href: '/purchasing/returns', label: 'Returns', icon: Undo2, permission: 'purchasing.view' },
      { href: '/purchasing/payables', label: 'Payables', icon: Banknote, permission: 'purchasing.view' },
      { href: '/purchasing/suppliers', label: 'Suppliers', icon: Building2, permission: 'purchasing.view' },
    ],
  },
  {
    label: 'Loyalty',
    items: [
      { href: '/loyalty', label: 'Members', icon: Medal, permission: 'crm.view' },
      { href: '/loyalty/rewards', label: 'Rewards', icon: Gift, permission: 'crm.view' },
      { href: '/loyalty/scheme', label: 'Tiers & Rules', icon: Trophy, permission: 'crm.view' },
      { href: '/loyalty/badges', label: 'Badges', icon: Award, permission: 'crm.view' },
      { href: '/loyalty/inbox', label: 'Inbox', icon: MessagesSquare, permission: 'crm.view' },
      { href: '/loyalty/reviews', label: 'Reviews', icon: Star, permission: 'crm.view' },
      { href: '/loyalty/partners', label: 'Partners', icon: Link2, permission: 'crm.view' },
    ],
  },
  {
    label: 'People',
    items: [
      { href: '/people', label: 'Roster', icon: CalendarCheck, permission: 'hris.view' },
      { href: '/people/employees', label: 'Staff Directory', icon: IdCard, permission: 'hris.view' },
      { href: '/people/attendance', label: 'Timesheet', icon: Clock, permission: 'hris.view' },
      { href: '/people/leave', label: 'Leave & Overtime', icon: CalendarClock, permission: 'hris.view' },
      { href: '/people/shifts', label: 'Shifts & Calendar', icon: CalendarDays, permission: 'hris.view' },
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
  const [navOpen, setNavOpen] = useState(false);

  useEffect(() => {
    if (!token) router.replace('/login');
  }, [token, router]);
  // Close the mobile drawer whenever navigation happens.
  useEffect(() => {
    setNavOpen(false);
  }, [pathname]);
  if (!token || !user) return null;

  const logout = () => {
    clear();
    router.replace('/login');
  };

  const resetDemo = async () => {
    await api.dev.reset();
    clear();
    // A full reload, not a client navigation, so the mock backend is rebuilt
    // from the fresh seed. The base path is explicit because location.href
    // does not carry Next's basePath the way the router does.
    location.href = `${process.env.NEXT_PUBLIC_BASE_PATH ?? '/admin'}/login`;
  };

  // Highlight only the deepest matching nav item (so /engagement/races doesn't
  // also light up /engagement).
  const allHrefs = NAV.flatMap((g) => g.items.map((i) => i.href));
  const activeHref = allHrefs
    .filter((h) => pathname === h || pathname.startsWith(`${h}/`))
    .sort((a, b) => b.length - a.length)[0];

  return (
    <PageHeaderProvider>
      <Shell
        user={user}
        navGroups={NAV}
        can={can}
        activeHref={activeHref}
        navOpen={navOpen}
        setNavOpen={setNavOpen}
        onLogout={logout}
        onReset={() => void resetDemo()}
      >
        {children}
      </Shell>
    </PageHeaderProvider>
  );
}

/**
 * The shell: a floating dark rail on the left, a utility bar across the top,
 * and the page underneath.
 *
 * The rail collapses to icons. That is not decoration — this panel has nine
 * groups and forty-odd destinations, and somebody working a till all day wants
 * the width back once they know where things are. The choice is remembered,
 * because being asked again every morning is the same as not offering it.
 */
function Shell({
  user,
  navGroups,
  can,
  activeHref,
  navOpen,
  setNavOpen,
  onLogout,
  onReset,
  children,
}: {
  user: { id: string; name: string; role: string };
  navGroups: NavGroup[];
  can: (permission: Permission) => boolean;
  activeHref?: string;
  navOpen: boolean;
  setNavOpen: (open: boolean) => void;
  onLogout: () => void;
  onReset: () => void;
  children: ReactNode;
}) {
  const header = usePageHeader();
  const setActionSlot = usePageActionSlot();
  const [collapsed, setCollapsed] = useState(false);

  // Restored on load rather than defaulted, so the rail is where it was left.
  useEffect(() => {
    try {
      setCollapsed(localStorage.getItem('nuhabit.admin.rail') === 'collapsed');
    } catch {
      // A browser refusing storage is not a reason to fail to render a menu.
    }
  }, []);
  const toggleRail = () => {
    setCollapsed((was) => {
      const next = !was;
      try {
        localStorage.setItem('nuhabit.admin.rail', next ? 'collapsed' : 'open');
      } catch {
        // Same again: the preference is a convenience, not a requirement.
      }
      return next;
    });
  };

  // The nav's own label stands in until the page publishes its own, so the bar
  // is never briefly blank on the way to being right.
  const navLabel = navGroups.flatMap((g) => g.items).find((i) => i.href === activeHref)?.label;
  const title = header?.title ?? navLabel ?? 'Dashboard';
  const initials = user.name
    .split(' ')
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();

  return (
    <div className="min-h-dvh">
      {/* Mobile bar. The rail becomes a drawer below the large breakpoint. */}
      <header className="fixed inset-x-0 top-0 z-30 flex items-center justify-between bg-beige/90 px-4 py-3 backdrop-blur lg:hidden">
        <Link href="/dashboard" className="flex items-center gap-2.5">
          <img src="/admin/brand/nuhabit-logo.png" alt="NüHabit" className="h-[22px] w-auto" />
        </Link>
        <button
          onClick={() => setNavOpen(!navOpen)}
          className="flex h-9 w-9 items-center justify-center rounded-xl bg-brand text-white"
          aria-label="Toggle menu"
        >
          {navOpen ? <X size={18} /> : <Menu size={18} />}
        </button>
      </header>
      {navOpen ? (
        <div className="fixed inset-0 z-30 bg-black/50 lg:hidden" onClick={() => setNavOpen(false)} />
      ) : null}

      <aside
        className={`surface-ink fixed inset-y-0 left-0 z-40 flex flex-col text-white transition-all duration-200 max-lg:w-64 lg:inset-y-3 lg:left-3 lg:rounded-3xl lg:shadow-[0_18px_50px_rgb(0_40_26/0.25)] ${
          collapsed ? 'lg:w-[76px]' : 'lg:w-[248px]'
        } ${navOpen ? 'translate-x-0' : 'max-lg:-translate-x-full'}`}
      >
        <div className={`pb-3 pt-5 ${collapsed ? 'px-3' : 'px-4'}`}>
          <Link
            href="/dashboard"
            className={`flex items-center gap-2.5 rounded-2xl bg-white/[0.07] py-3 ${
              collapsed ? 'justify-center px-2' : 'px-3.5'
            }`}
            title="NüHabit Admin"
          >
            <img
              src="/admin/brand/nuhabit-mark-white.png"
              alt="NüHabit"
              className="h-6 w-6 shrink-0 object-contain"
              onError={(e) => {
                // The square mark is optional artwork; the wordmark always
                // exists, so fall back to it rather than showing a broken box.
                (e.currentTarget as HTMLImageElement).src = '/admin/brand/nuhabit-logo-white.png';
              }}
            />
            {!collapsed ? (
              <span className="text-[11px] font-bold uppercase tracking-[0.2em] text-white/45">
                Admin
              </span>
            ) : null}
          </Link>
        </div>

        <nav className={`flex-1 overflow-y-auto pb-3 ${collapsed ? 'px-3' : 'px-3'}`}>
          {navGroups.map((group) => {
            const visible = group.items.filter((i) => can(i.permission));
            if (visible.length === 0) return null;
            return (
              <div key={group.label ?? 'root'} className="mb-4">
                {group.label && !collapsed ? (
                  <p className="px-3 pb-1.5 text-[10px] font-extrabold uppercase tracking-[0.18em] text-white/25">
                    {group.label}
                  </p>
                ) : null}
                {collapsed && group.label ? (
                  <div className="mx-auto mb-2 h-px w-6 bg-white/10" />
                ) : null}
                {visible.map(({ href, label, icon: Icon }) => {
                  const active = href === activeHref;
                  return (
                    <Link
                      key={href}
                      href={href}
                      title={collapsed ? label : undefined}
                      className={`mb-1 flex items-center gap-3 rounded-2xl text-sm font-bold transition ${
                        collapsed ? 'justify-center px-2 py-2.5' : 'px-3 py-2.5'
                      } ${
                        active
                          ? 'bg-lime text-ink shadow-[0_6px_18px_rgb(218_255_89/0.18)]'
                          : 'text-white/50 hover:bg-white/[0.07] hover:text-white'
                      }`}
                    >
                      <Icon size={17} className="shrink-0" />
                      {!collapsed ? <span className="truncate">{label}</span> : null}
                    </Link>
                  );
                })}
              </div>
            );
          })}
        </nav>

        <div className={`pb-4 ${collapsed ? 'px-3' : 'px-3'}`}>
          {/* Who is signed in lives in the top bar, once. Repeating it here
              costs the rail a row and tells nobody anything new. */}
          <button
            onClick={onLogout}
            title={collapsed ? 'Sign out' : undefined}
            className={`mb-1 flex w-full items-center gap-3 rounded-2xl py-2.5 text-sm font-bold text-white/50 transition hover:bg-white/[0.07] hover:text-white ${
              collapsed ? 'justify-center px-2' : 'px-3'
            }`}
          >
            <LogOut size={17} className="shrink-0" />
            {!collapsed ? 'Sign out' : null}
          </button>
          <button
            onClick={onReset}
            title={collapsed ? 'Reset demo data' : undefined}
            className={`mb-2 flex w-full items-center gap-3 rounded-2xl py-2.5 text-sm font-bold text-white/50 transition hover:bg-white/[0.07] hover:text-white ${
              collapsed ? 'justify-center px-2' : 'px-3'
            }`}
          >
            <RotateCcw size={17} className="shrink-0" />
            {!collapsed ? 'Reset demo' : null}
          </button>
          <button
            onClick={toggleRail}
            className={`hidden w-full items-center gap-3 rounded-2xl bg-white/[0.07] py-2.5 text-sm font-bold text-white/60 transition hover:bg-white/10 hover:text-white lg:flex ${
              collapsed ? 'justify-center px-2' : 'px-3'
            }`}
            title={collapsed ? 'Expand' : 'Collapse'}
          >
            {collapsed ? <ChevronRight size={17} /> : <ChevronLeft size={17} />}
            {!collapsed ? 'Collapse' : null}
          </button>
        </div>
      </aside>

      <div
        className={`min-w-0 transition-all duration-200 ${
          collapsed ? 'lg:pl-[92px]' : 'lg:pl-[264px]'
        }`}
      >
        {/* The utility bar: what page this is, a way to get anywhere, and who
            you are signed in as. */}
        <header className="sticky top-0 z-20 flex flex-wrap items-center gap-3 bg-beige/85 px-4 pb-3 pt-16 backdrop-blur sm:px-6 lg:px-7 lg:pt-5">
          <div className="min-w-0 shrink-0">
            <h1 className="display truncate text-2xl font-black leading-tight">{title}</h1>
            {header?.subtitle ? (
              <p className="truncate text-[13px] text-muted">{header.subtitle}</p>
            ) : null}
          </div>

          <NavSearch groups={navGroups} can={can} />
          {/* The page's own buttons land here, portalled from wherever the
              page happens to render its PageTitle. */}
          <div ref={setActionSlot} className="flex flex-wrap items-center gap-2" />
          <div
            className="flex items-center gap-2.5 rounded-full bg-surface py-1.5 pl-1.5 pr-4 shadow-[0_1px_2px_rgb(0_40_26/0.04)]"
            title={user.id}
          >
            <span className="flex h-8 w-8 items-center justify-center rounded-full bg-brand text-[11px] font-black text-lime">
              {initials}
            </span>
            <span className="hidden leading-tight sm:block">
              <span className="block text-[13px] font-black">{user.name}</span>
              <span className="block text-[10px] font-bold uppercase tracking-wider text-muted">
                {user.role.replaceAll('_', ' ')}
              </span>
            </span>
          </div>
        </header>

        <main className="min-w-0 px-4 pb-8 pt-2 sm:px-6 lg:px-7">{children}</main>
      </div>
    </div>
  );
}

/**
 * Getting anywhere from anywhere.
 *
 * A panel with nine groups and forty destinations is faster to type at than to
 * hunt through, particularly once the rail is collapsed to icons. It searches
 * what it can actually take you to — the destinations — rather than pretending
 * to search the data behind them, because a box that finds nothing useful is
 * worse than no box at all.
 */
function NavSearch({
  groups,
  can,
}: {
  groups: NavGroup[];
  can: (permission: Permission) => boolean;
}) {
  const router = useRouter();
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [highlighted, setHighlighted] = useState(0);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const away = (event: MouseEvent) => {
      if (box.current && !box.current.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', away);
    return () => document.removeEventListener('mousedown', away);
  }, []);

  const needle = query.trim().toLowerCase();
  const matches = needle
    ? groups
        .flatMap((group) =>
          group.items
            .filter((item) => can(item.permission))
            .map((item) => ({ ...item, group: group.label ?? '' })),
        )
        .filter(
          (item) =>
            item.label.toLowerCase().includes(needle) ||
            item.group.toLowerCase().includes(needle),
        )
        .slice(0, 8)
    : [];

  const go = (href: string) => {
    setQuery('');
    setOpen(false);
    router.push(href);
  };

  return (
    <div ref={box} className="relative min-w-0 flex-1 basis-64">
      <span className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-muted">
        <Search size={16} />
      </span>
      <input
        className="w-full rounded-full border-none bg-surface py-2.5 pl-11 pr-4 text-sm shadow-[0_1px_2px_rgb(0_40_26/0.04)] outline-none placeholder:text-muted focus:ring-2 focus:ring-lime"
        placeholder="Search for a page…"
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
          setHighlighted(0);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown') {
            e.preventDefault();
            setHighlighted((i) => Math.min(i + 1, matches.length - 1));
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            setHighlighted((i) => Math.max(i - 1, 0));
          } else if (e.key === 'Enter' && matches[highlighted]) {
            go(matches[highlighted].href);
          } else if (e.key === 'Escape') {
            setOpen(false);
          }
        }}
      />
      {open && matches.length > 0 ? (
        <div className="absolute left-0 right-0 top-full z-30 mt-1.5 overflow-hidden rounded-2xl bg-surface py-1.5 shadow-[0_18px_40px_rgb(0_40_26/0.18)]">
          {matches.map((item, index) => (
            <button
              key={item.href}
              onMouseEnter={() => setHighlighted(index)}
              onClick={() => go(item.href)}
              className={`flex w-full items-center gap-2.5 px-4 py-2 text-left text-sm transition ${
                index === highlighted ? 'bg-surface-raised' : ''
              }`}
            >
              <item.icon size={15} className="shrink-0 text-muted" />
              <span className="font-bold">{item.label}</span>
              {item.group ? (
                <span className="ml-auto text-[11px] font-bold uppercase tracking-wider text-muted">
                  {item.group}
                </span>
              ) : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
