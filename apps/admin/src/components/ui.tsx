'use client';

import { Check, ChevronsUpDown, type LucideIcon } from 'lucide-react';
import { useEffect, useRef, useState, type ComponentType, type ReactNode } from 'react';
import { usePublishHeader } from './page-header';

export interface SearchSelectOption {
  value: string;
  label: string;
  /** Extra text shown under the label and included in search matching. */
  hint?: string;
}

/** Searchable dropdown - the admin default wherever a native select would list entities. */
export function SearchSelect({
  value,
  onChange,
  options,
  placeholder = 'Search…',
  allowEmpty = false,
  emptyLabel = 'All',
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  options: SearchSelectOption[];
  placeholder?: string;
  /** Adds a pinned option with value '' (for "All …" filters or optional fields). */
  allowEmpty?: boolean;
  emptyLabel?: string;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, []);

  const selected = options.find((o) => o.value === value) ?? null;
  const q = query.trim().toLowerCase();
  const filtered = q
    ? options.filter((o) => `${o.label} ${o.hint ?? ''}`.toLowerCase().includes(q))
    : options;

  const pick = (next: string) => {
    onChange(next);
    setOpen(false);
    setQuery('');
  };

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        disabled={disabled}
        onClick={() => {
          setOpen((v) => !v);
          setQuery('');
        }}
        className="a-input flex items-center justify-between gap-2 text-left disabled:opacity-40"
      >
        <span className={`truncate ${selected || (allowEmpty && value === '') ? '' : 'text-muted'}`}>
          {selected ? selected.label : allowEmpty ? emptyLabel : placeholder}
        </span>
        <ChevronsUpDown size={14} className="shrink-0 text-muted" />
      </button>
      {open ? (
        <div className="menu-pop absolute inset-x-0 z-30 mt-1 overflow-hidden rounded-xl border border-line bg-surface shadow-[0_16px_40px_rgb(0_40_26/0.18)]">
          <input
            autoFocus
            className="w-full border-b border-line bg-surface px-3.5 py-2.5 text-sm focus:outline-none"
            placeholder={placeholder}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') setOpen(false);
              if (e.key === 'Enter' && filtered.length > 0) pick(filtered[0]!.value);
            }}
          />
          <div className="max-h-56 overflow-y-auto py-1">
            {allowEmpty && q === '' ? (
              <button
                type="button"
                onClick={() => pick('')}
                className="flex w-full items-center justify-between px-3.5 py-2 text-left text-sm font-bold hover:bg-surface-raised"
              >
                {emptyLabel}
                {value === '' ? <Check size={14} className="text-brand" /> : null}
              </button>
            ) : null}
            {filtered.map((o) => (
              <button
                key={o.value}
                type="button"
                onClick={() => pick(o.value)}
                className="flex w-full items-center justify-between gap-2 px-3.5 py-2 text-left text-sm hover:bg-surface-raised"
              >
                <span className="min-w-0">
                  <span className="block truncate font-bold">{o.label}</span>
                  {o.hint ? <span className="block truncate text-xs text-muted">{o.hint}</span> : null}
                </span>
                {o.value === value ? <Check size={14} className="shrink-0 text-brand" /> : null}
              </button>
            ))}
            {filtered.length === 0 ? (
              <p className="px-3.5 py-2.5 text-sm text-muted">No matches.</p>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

export interface RowAction {
  label: string;
  icon?: ComponentType<{ size?: number | string; className?: string }>;
  tone?: 'danger';
  /** Rendered but inert - for actions the entity's state machine forbids right now. */
  disabled?: boolean;
  onClick: () => void;
}

/**
 * Row actions as a quiet strip of icon buttons (classic backoffice tables).
 * Each button carries a native tooltip; destructive actions turn red on hover.
 */
export function RowActions({ items }: { items: RowAction[] }) {
  return (
    <div className="inline-flex items-center justify-end gap-0.5">
      {items.map((item) => (
        <button
          key={item.label}
          type="button"
          title={item.label}
          aria-label={item.label}
          disabled={item.disabled}
          onClick={item.onClick}
          className={`inline-flex h-8 w-8 items-center justify-center rounded-lg transition disabled:pointer-events-none disabled:opacity-30 ${
            item.tone === 'danger'
              ? 'text-muted hover:bg-danger/10 hover:text-danger'
              : 'text-muted hover:bg-surface-raised hover:text-ink'
          }`}
        >
          {item.icon ? <item.icon size={15} /> : <span className="text-[10px] font-black">{item.label.slice(0, 2)}</span>}
        </button>
      ))}
    </div>
  );
}

/** Prev/next pagination footer for tables. */
export function Pager({
  page,
  pageCount,
  onPage,
}: {
  page: number;
  pageCount: number;
  onPage: (page: number) => void;
}) {
  if (pageCount <= 1) return null;
  return (
    <div className="flex items-center justify-between border-t border-line px-4 py-2.5">
      <p className="text-xs font-bold text-muted">
        Page {page + 1} of {pageCount}
      </p>
      <div className="flex gap-1.5">
        <button
          className="a-btn-ghost !px-3 !py-1 text-xs"
          disabled={page === 0}
          onClick={() => onPage(page - 1)}
        >
          Prev
        </button>
        <button
          className="a-btn-ghost !px-3 !py-1 text-xs"
          disabled={page >= pageCount - 1}
          onClick={() => onPage(page + 1)}
        >
          Next
        </button>
      </div>
    </div>
  );
}

/** A labelled form field with an optional explanation underneath. */
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-[11px] font-bold uppercase tracking-wider text-muted">
        {label}
      </span>
      {children}
      {hint ? <span className="mt-1 block text-xs text-muted">{hint}</span> : null}
    </label>
  );
}

/**
 * A page's title.
 *
 * It renders nothing itself: the shell's top bar is where a title belongs, so
 * this publishes upwards and the bar draws it. Pages carry on calling it the
 * way they always did.
 */
export function PageTitle({ title, subtitle, actions }: { title: string; subtitle?: string; actions?: ReactNode }) {
  return usePublishHeader(title, subtitle, actions);
}

/**
 * One number, with the thing it measures above it.
 *
 * The icon sits in its own circle in the corner rather than beside the label,
 * so a row of these reads as a row of numbers — the eye lands on the figures
 * and the icons stay decoration.
 */
export function StatCard({
  label,
  value,
  hint,
  tone,
  icon: Icon,
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  tone?: 'brand' | 'danger';
  icon?: LucideIcon;
}) {
  return (
    <div className="a-card relative">
      {Icon ? (
        <span
          className={`absolute right-4 top-4 flex h-9 w-9 items-center justify-center rounded-full ${
            tone === 'danger'
              ? 'bg-danger/10 text-danger'
              : tone === 'brand'
                ? 'bg-lime text-ink'
                : 'bg-brand/[0.06] text-brand'
          }`}
        >
          <Icon size={16} />
        </span>
      ) : null}
      <p className="pr-12 text-sm font-bold text-muted">{label}</p>
      {/* A rupiah figure runs long. Sizing down at the breakpoint where these
          cards get narrow keeps the number inside its card rather than
          against the edge of it. */}
      <p
        className={`display mt-2 text-3xl font-black tabular-nums xl:text-[1.75rem] 2xl:text-4xl ${
          tone === 'danger' ? 'text-danger' : ''
        }`}
      >
        {value}
      </p>
      {hint ? <p className="mt-1.5 text-xs text-muted">{hint}</p> : null}
    </div>
  );
}

export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  /**
   * For a sheet that carries a table. A five-column table in a form-width
   * panel loses its last column off the edge, which is the one nobody knew
   * was there.
   */
  wide?: boolean;
}) {
  return (
    <div className="modal-backdrop fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className={`modal-panel max-h-[85dvh] w-full overflow-y-auto rounded-3xl bg-surface p-6 shadow-[0_24px_60px_rgb(0_40_26/0.3)] ${
          wide ? 'max-w-3xl' : 'max-w-lg'
        }`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="display text-lg font-black">{title}</h2>
          <button onClick={onClose} className="text-muted hover:text-ink" aria-label="Close">
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function ErrorNote({ message }: { message: string | null }) {
  if (!message) return null;
  return <p className="rounded-lg bg-danger/10 px-3 py-2 text-sm font-bold text-danger">{message}</p>;
}

/**
 * What a refused query looks like.
 *
 * A 403 that renders as an empty table reads as "there is nothing here", which
 * is a different and worse answer than "you are not allowed to see this".
 */
export function QueryError({ error }: { error: unknown }) {
  if (!error) return null;
  const status = (error as { status?: number }).status;
  const message =
    status === 403
      ? 'Your role does not include access to this. Ask an administrator if you need it.'
      : ((error as { message?: string }).message ?? 'That did not load.');
  return <ErrorNote message={message} />;
}
