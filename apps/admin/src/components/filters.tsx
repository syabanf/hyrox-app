'use client';

import { X } from 'lucide-react';
import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useCallback, useMemo, type ReactNode } from 'react';
import { SearchSelect } from './ui';

/**
 * Filter state that lives in the URL.
 *
 * Keeping it in the address bar rather than in component state is what makes
 * a report drillable at all: "show me this branch's refunds last month" stops
 * being a sequence of clicks somebody has to repeat and becomes a link. It
 * also survives a reload, a back button, and being pasted to a colleague —
 * three things a `useState` filter loses every time.
 */
export function useFilters<T extends Record<string, string>>(defaults: T) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();

  const filters = useMemo(() => {
    const out = { ...defaults };
    for (const key of Object.keys(defaults) as (keyof T)[]) {
      const value = params.get(String(key));
      if (value !== null) out[key] = value as T[keyof T];
    }
    return out;
  }, [params, defaults]);

  const write = useCallback(
    (patch: Partial<Record<keyof T, string>>) => {
      const next = new URLSearchParams(params.toString());
      for (const [key, value] of Object.entries(patch)) {
        // A filter set back to its default is not a filter; leaving it in the
        // URL would put a chip on screen for "everything".
        if (!value || value === defaults[key as keyof T]) next.delete(key);
        else next.set(key, value);
      }
      // replace, not push: a filter change is a refinement of where you are,
      // and pushing would make the back button walk every keystroke.
      router.replace(next.toString() ? `${pathname}?${next}` : pathname, { scroll: false });
    },
    [params, pathname, router, defaults],
  );

  const set = useCallback(
    (key: keyof T, value: string) => write({ [key]: value } as Partial<Record<keyof T, string>>),
    [write],
  );

  const clear = useCallback(() => {
    const cleared = Object.fromEntries(Object.keys(defaults).map((k) => [k, ''])) as Partial<
      Record<keyof T, string>
    >;
    write(cleared);
  }, [write, defaults]);

  const dirty = useMemo(
    () => (Object.keys(defaults) as (keyof T)[]).some((k) => filters[k] !== defaults[k]),
    [filters, defaults],
  );

  return { filters, set, setMany: write, clear, dirty };
}

/**
 * The row of controls above a table, and the chips saying what is currently
 * narrowing it.
 *
 * The chips are the part that matters. A dropdown that has been left on a
 * value looks identical to one that has not, which is how somebody spends ten
 * minutes wondering where half the rows went.
 */
export function FilterBar({
  children,
  chips,
  onClear,
  dirty,
}: {
  children: ReactNode;
  chips?: { key: string; label: string; onRemove: () => void }[];
  onClear?: () => void;
  dirty?: boolean;
}) {
  return (
    <div className="mb-4 flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">{children}</div>
      {dirty && chips && chips.length > 0 ? (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-[11px] font-bold uppercase tracking-wider text-muted">
            Filtering
          </span>
          {chips.map((chip) => (
            <button
              key={chip.key}
              type="button"
              onClick={chip.onRemove}
              className="group inline-flex items-center gap-1 rounded-full border border-brand/25 bg-brand/[0.07] px-2.5 py-1 text-[11px] font-bold text-brand transition hover:border-brand/50"
            >
              {chip.label}
              <X size={11} className="opacity-50 transition group-hover:opacity-100" />
            </button>
          ))}
          {onClear ? (
            <button
              type="button"
              onClick={onClear}
              className="ml-1 text-[11px] font-bold text-muted underline underline-offset-2 hover:text-ink"
            >
              Clear all
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** The windows people actually ask for, in the order they ask for them. */
export const RANGE_PRESETS = [
  { value: '7', label: 'Last 7 days' },
  { value: '30', label: 'Last 30 days' },
  { value: '90', label: 'Last 90 days' },
  { value: '180', label: 'Last 6 months' },
  { value: '365', label: 'Last year' },
] as const;

export function rangeLabel(days: string): string {
  return RANGE_PRESETS.find((p) => p.value === days)?.label ?? `Last ${days} days`;
}

/**
 * A date window as a count of days.
 *
 * Not two date pickers: every report here is "the last N days" and asking for
 * two calendar dates to express that is three clicks and a mistake. Custom
 * start and end dates belong on the export, not on the screen people read
 * every morning.
 */
export function RangePicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (days: string) => void;
}) {
  return (
    <div className="inline-flex rounded-lg border border-line bg-surface p-0.5">
      {RANGE_PRESETS.map((preset) => (
        <button
          key={preset.value}
          type="button"
          onClick={() => onChange(preset.value)}
          className={`rounded-[6px] px-2.5 py-1.5 text-xs font-bold transition ${
            value === preset.value
              ? 'bg-brand text-white'
              : 'text-muted hover:bg-surface-raised hover:text-ink'
          }`}
        >
          {preset.label.replace('Last ', '')}
        </button>
      ))}
    </div>
  );
}

/** A labelled dropdown sized for the filter row. */
export function FilterSelect({
  value,
  onChange,
  options,
  emptyLabel,
  width = 'w-44',
}: {
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string; hint?: string }[];
  emptyLabel: string;
  width?: string;
}) {
  return (
    <div className={width}>
      <SearchSelect
        value={value}
        onChange={onChange}
        allowEmpty
        emptyLabel={emptyLabel}
        placeholder="Search…"
        options={options}
      />
    </div>
  );
}

/**
 * A row that drills one level down.
 *
 * The whole drill-down story is this component: a summary row is a link to
 * the screen that lists what the row counted, with the filters already
 * applied. No new endpoints, and the screen you land on is the one you
 * already know how to use.
 */
export function DrillRow({
  href,
  children,
  className = '',
}: {
  href: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Link
      href={href}
      className={`group flex items-center justify-between gap-3 rounded-lg px-3 py-2 transition hover:bg-surface-raised ${className}`}
    >
      {children}
    </Link>
  );
}

/** Build a link that lands on `path` with these filters already set. */
export function drillTo(path: string, params: Record<string, string | number | undefined>): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') query.set(key, String(value));
  }
  const q = query.toString();
  return q ? `${path}?${q}` : path;
}
