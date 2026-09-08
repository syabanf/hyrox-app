'use client';

import type { POSProductView, UpsertPOSProductInput } from '@nuhabit/contracts';
import { CHANNEL_LABELS, SALES_CHANNELS, type SalesChannel } from '@nuhabit/domain';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Tags } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { Thumb } from '../../../../components/thumb';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

export default function CatalogPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [query, setQuery] = useState('');
  const [editing, setEditing] = useState<POSProductView | 'new' | null>(null);
  const [pricing, setPricing] = useState<POSProductView | null>(null);

  const { data: products, isLoading, error } = useQuery({
    queryKey: ['pos', 'products'],
    queryFn: () => api.admin.pos.products.list(),
  });

  const q = query.trim().toLowerCase();
  // A barcode is searchable text like any other: typing or scanning one into
  // the box is the fastest way to find the row you mean.
  const rows = (products ?? []).filter(
    (p) =>
      !q ||
      p.name.toLowerCase().includes(q) ||
      p.sku.toLowerCase().includes(q) ||
      (p.barcode ?? '').includes(q),
  );

  return (
    <div>
      <PageTitle
        title="Catalogue"
        subtitle="What the till can sell, in what pack, and what it comes off"
        actions={
          <>
            <Link href="/counter" className="a-btn-ghost">
              Till
            </Link>
            {can('pos.manage') ? (
              <button className="a-btn" onClick={() => setEditing('new')}>
                + New product
              </button>
            ) : null}
          </>
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink" label="Products" value={rows.length} />
        <StatCard label="On sale today" value={rows.filter((p) => p.available && p.active).length} tone="brand" />
        <StatCard
          tone="brand"
          label="Multipacks"
          value={rows.filter((p) => p.packFactor > 1).length}
          hint="Sold by the carton, counted by the piece"
        />
        <StatCard
          tone="info"
          label="Average margin"
          value={
            rows.length
              ? `${Math.round(
                  (rows.reduce((s, p) => s + (p.priceIdr - p.costIdr), 0) /
                    Math.max(1, rows.reduce((s, p) => s + p.priceIdr, 0))) * 100,
                )}%`
              : '—'
          }
        />
      </div>

      <div className="mb-4">
        <input
          className="a-input max-w-xs"
          placeholder="Search or scan a product…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {isLoading ? (
        <Spinner label="Loading the catalogue…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Product</th>
                  <th>Sold as</th>
                  <th>Comes off</th>
                  <th className="text-right">Price</th>
                  <th className="text-right">Cost</th>
                  <th className="text-right">Margin</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((product) => (
                  <tr key={product.id}>
                    <td>
                      <div className="flex items-center gap-3">
                        <Thumb src={product.imageUrl} name={product.name} />
                        <div className="min-w-0">
                      <p className="font-bold">{product.name}</p>
                      <p className="text-xs text-muted">
                        {product.sku}
                        {product.barcode ? ` · ${product.barcode}` : ''}
                        {product.bonusXp > 0 ? ` · +${product.bonusXp} bonus points` : ''}
                      </p>
                        </div>
                      </div>
                    </td>
                    <td className="text-sm">
                      {product.packFactor > 1 ? (
                        <span className="font-bold">
                          {product.packUnit}{' '}
                          <span className="font-normal text-muted">of {product.packFactor}</span>
                        </span>
                      ) : (
                        <span className="text-muted">{product.packUnit}</span>
                      )}
                    </td>
                    <td className="text-sm">
                      {product.inventoryItemId ? (
                        product.onHand != null ? (
                          <span>
                            {product.onHand} {product.packUnit.toLowerCase()} in stock
                          </span>
                        ) : (
                          <span className="text-muted">stock item</span>
                        )
                      ) : (
                        <span className="text-muted">a service</span>
                      )}
                    </td>
                    <td className="text-right tabular-nums">{formatIdr(product.priceIdr)}</td>
                    {/* Cost is per base unit, so a carton costs its factor
                        times as much — showing the piece cost against the
                        carton price would make every multipack look like a
                        miracle. */}
                    <td className="text-right tabular-nums text-sm text-muted">
                      {formatIdr(product.costIdr * product.packFactor)}
                    </td>
                    <td className="text-right tabular-nums text-sm">
                      {product.priceIdr > 0
                        ? `${Math.round(
                            ((product.priceIdr - product.costIdr * product.packFactor) / product.priceIdr) * 100,
                          )}%`
                        : '—'}
                    </td>
                    <td>
                      <StatusBadge
                        status={!product.active ? 'INACTIVE' : product.available ? 'ACTIVE' : 'PAUSED'}
                      />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Edit',
                            icon: Pencil,
                            disabled: !can('pos.manage'),
                            onClick: () => setEditing(product),
                          },
                          {
                            label: 'Price breaks',
                            icon: Tags,
                            disabled: !can('pos.manage'),
                            onClick: () => setPricing(product),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="py-8 text-center text-sm text-muted">
                      Nothing in the catalogue yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {pricing ? (
        <PriceBreakModal product={pricing} onClose={() => setPricing(null)} />
      ) : null}

      {editing ? (
        <ProductModal
          product={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['pos'] });
          }}
        />
      ) : null}
    </div>
  );
}

function ProductModal({
  product,
  onClose,
  onDone,
}: {
  product: POSProductView | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const { data: categories } = useQuery({
    queryKey: ['pos', 'categories'],
    queryFn: api.admin.pos.categories.list,
  });
  const { data: items } = useQuery({
    queryKey: ['inventory', 'items', 'all'],
    queryFn: () => api.admin.inventory.items.list(),
  });

  const [form, setForm] = useState<UpsertPOSProductInput>(() => ({
    sku: product?.sku ?? '',
    name: product?.name ?? '',
    description: product?.description ?? '',
    categoryId: product?.categoryId ?? null,
    inventoryItemId: product?.inventoryItemId ?? null,
    priceIdr: product?.priceIdr ?? 0,
    taxPercent: product?.taxPercent ?? 0,
    barcode: product?.barcode ?? null,
    packUnit: product?.packUnit ?? 'PCS',
    bonusXp: product?.bonusXp ?? 0,
    active: product?.active ?? true,
    available: product?.available ?? true,
  }));
  const set = <K extends keyof UpsertPOSProductInput>(k: K, v: UpsertPOSProductInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  // The packs of whichever item this product draws on: a product is one pack
  // of one item, and this is the list to choose from.
  const { data: packs } = useQuery({
    queryKey: ['inventory', 'packs', form.inventoryItemId],
    queryFn: () => api.admin.inventory.items.packs(form.inventoryItemId as string),
    enabled: Boolean(form.inventoryItemId),
  });

  const save = useMutation({
    mutationFn: () => api.admin.pos.products.save(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That product did not save.'),
  });

  return (
    <Modal title={product ? `Edit ${product.name}` : 'New product'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="SKU">
            <input className="a-input" value={form.sku} onChange={(e) => set('sku', e.target.value)} />
          </Field>
          <Field label="Price">
            <input
              className="a-input"
              type="number"
              value={form.priceIdr}
              onChange={(e) => set('priceIdr', Number(e.target.value))}
            />
          </Field>
        </div>
        <Field label="Name">
          <input className="a-input" value={form.name} onChange={(e) => set('name', e.target.value)} />
        </Field>
        <Field label="Category">
          <SearchSelect
            value={form.categoryId ?? ''}
            onChange={(v) => set('categoryId', v || null)}
            allowEmpty
            emptyLabel="None"
            placeholder="Search category…"
            options={(categories ?? []).map((c) => ({ value: c.id, label: c.name }))}
          />
        </Field>
        <Field
          label="Comes off stock"
          hint="Leave empty for a service. The cost follows the item's average, so the margin is not typed."
        >
          <SearchSelect
            value={form.inventoryItemId ?? ''}
            onChange={(v) => set('inventoryItemId', v || null)}
            allowEmpty
            emptyLabel="A service — no stock"
            placeholder="Search item…"
            options={(items ?? []).map((i) => ({ value: i.id, label: i.name, hint: i.sku }))}
          />
        </Field>
        {form.inventoryItemId ? (
          <Field
            label="Sold in"
            hint="One of the item's packs. Selling a carton of 24 takes 24 off the shelf, and the factor comes from the item rather than from here."
          >
            <SearchSelect
              value={form.packUnit ?? ''}
              onChange={(v) => set('packUnit', v || 'PCS')}
              placeholder="Search pack…"
              options={(packs ?? []).map((p) => ({ value: p.unitCode, label: p.label }))}
            />
          </Field>
        ) : null}
        <Field label="Barcode" hint="What the scanner reads. A single and a carton carry different codes.">
          <input
            className="a-input"
            value={form.barcode ?? ''}
            onChange={(e) => set('barcode', e.target.value || null)}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Tax %">
            <input
              className="a-input"
              type="number"
              value={form.taxPercent ?? 0}
              onChange={(e) => set('taxPercent', Number(e.target.value))}
            />
          </Field>
          <Field label="Bonus points" hint="On top of what the spend earns.">
            <input
              className="a-input"
              type="number"
              value={form.bonusXp ?? 0}
              onChange={(e) => set('bonusXp', Number(e.target.value))}
            />
          </Field>
        </div>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.available ?? true}
            onChange={(e) => set('available', e.target.checked)}
          />
          On sale today
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active ?? true}
            onChange={(e) => set('active', e.target.checked)}
          />
          Still sold at all
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !form.sku || !form.name} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

/**
 * Price breaks: what this product costs, by channel and by quantity.
 *
 * The deepest break a customer's quantity reaches wins, and a channel with no
 * list of its own falls back to retail — so an empty table here still sells at
 * the shelf price rather than failing.
 */
function PriceBreakModal({ product, onClose }: { product: POSProductView; onClose: () => void }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [draft, setDraft] = useState<{ channel: SalesChannel; minQty: number; priceIdr: number }>({
    channel: 'RETAIL',
    minQty: 1,
    priceIdr: product.priceIdr,
  });

  const { data: prices, isLoading } = useQuery({
    queryKey: ['pos', 'prices', product.id],
    queryFn: () => api.admin.pos.products.prices(product.id),
  });

  const refresh = () => qc.invalidateQueries({ queryKey: ['pos', 'prices', product.id] });

  const save = useMutation({
    mutationFn: () => api.admin.pos.products.savePrice(product.id, draft),
    onSuccess: () => void refresh(),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That price did not save.'),
  });
  const remove = useMutation({
    mutationFn: (priceId: string) => api.admin.pos.products.deletePrice(product.id, priceId),
    onSuccess: () => void refresh(),
  });

  const rows = [...(prices ?? [])].sort((a, b) =>
    a.channel === b.channel ? a.minQty - b.minQty : a.channel.localeCompare(b.channel),
  );

  return (
    <Modal title={`Price breaks — ${product.name}`} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="text-sm text-muted">
          The shelf price is {formatIdr(product.priceIdr)} per {product.packUnit.toLowerCase()}. A break
          applies from its quantity upwards; a channel with no break of its own pays the retail price.
        </p>

        {isLoading ? (
          <Spinner label="Loading prices…" />
        ) : (
          <div className="a-card !p-0">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Channel</th>
                  <th className="text-right">From</th>
                  <th className="text-right">Price</th>
                  <th className="text-right">Off shelf</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rows.map((price) => (
                  <tr key={price.id}>
                    <td className="font-bold">{CHANNEL_LABELS[price.channel]}</td>
                    <td className="text-right tabular-nums">{price.minQty}</td>
                    <td className="text-right tabular-nums">{formatIdr(price.priceIdr)}</td>
                    <td className="text-right tabular-nums text-sm text-muted">
                      {product.priceIdr > 0
                        ? `${Math.round(((product.priceIdr - price.priceIdr) / product.priceIdr) * 100)}%`
                        : '—'}
                    </td>
                    <td className="text-right">
                      <button
                        className="a-btn-ghost !px-2 !py-1 text-xs"
                        onClick={() => remove.mutate(price.id)}
                      >
                        Remove
                      </button>
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-6 text-center text-sm text-muted">
                      No breaks — everyone pays the shelf price.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        )}

        <div className="grid items-end gap-3 sm:grid-cols-4">
          <Field label="Channel">
            <SearchSelect
              value={draft.channel}
              onChange={(v) => setDraft((d) => ({ ...d, channel: v as SalesChannel }))}
              placeholder="Search…"
              options={[
                ...SALES_CHANNELS.map((channel) => ({ value: channel, label: CHANNEL_LABELS[channel] })),
              ]}
            />
          </Field>
          <Field label="From quantity">
            <input
              className="a-input"
              type="number"
              min={1}
              value={draft.minQty}
              onChange={(e) => setDraft((d) => ({ ...d, minQty: Number(e.target.value) }))}
            />
          </Field>
          <Field label="Price">
            <input
              className="a-input"
              type="number"
              value={draft.priceIdr}
              onChange={(e) => setDraft((d) => ({ ...d, priceIdr: Number(e.target.value) }))}
            />
          </Field>
          <button className="a-btn" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Add break'}
          </button>
        </div>

        <div className="mt-2 flex justify-end">
          <button className="a-btn-ghost" onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </Modal>
  );
}
