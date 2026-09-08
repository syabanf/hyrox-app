'use client';

import type { POSProductView, UpsertPOSProductInput } from '@nuhabit/contracts';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil } from 'lucide-react';
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
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

export default function MenuPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [query, setQuery] = useState('');
  const [editing, setEditing] = useState<POSProductView | 'new' | null>(null);

  const { data: products, isLoading, error } = useQuery({
    queryKey: ['pos', 'products'],
    queryFn: () => api.admin.pos.products.list(),
  });

  const q = query.trim().toLowerCase();
  const rows = (products ?? []).filter(
    (p) => !q || p.name.toLowerCase().includes(q) || p.sku.toLowerCase().includes(q),
  );

  return (
    <div>
      <PageTitle
        title="Counter menu"
        subtitle="What the till can sell, and what it comes off"
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
        <StatCard label="Products" value={rows.length} />
        <StatCard label="On the menu" value={rows.filter((p) => p.available && p.active).length} tone="brand" />
        <StatCard
          label="Services"
          value={rows.filter((p) => !p.inventoryItemId).length}
          hint="Priced, but move no stock"
        />
        <StatCard
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
          placeholder="Search product…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {isLoading ? (
        <Spinner label="Loading the menu…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Product</th>
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
                      <p className="font-bold">{product.name}</p>
                      <p className="text-xs text-muted">
                        {product.sku}
                        {product.bonusXp > 0 ? ` · +${product.bonusXp} bonus points` : ''}
                      </p>
                    </td>
                    <td className="text-sm">
                      {product.inventoryItemId ? (
                        product.onHand != null ? (
                          <span>{product.onHand} in stock</span>
                        ) : (
                          <span className="text-muted">stock item</span>
                        )
                      ) : (
                        <span className="text-muted">a service</span>
                      )}
                    </td>
                    <td className="text-right tabular-nums">{formatIdr(product.priceIdr)}</td>
                    <td className="text-right tabular-nums text-sm text-muted">
                      {formatIdr(product.costIdr)}
                    </td>
                    <td className="text-right tabular-nums text-sm">
                      {product.priceIdr > 0
                        ? `${Math.round(((product.priceIdr - product.costIdr) / product.priceIdr) * 100)}%`
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
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      Nothing on the menu yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

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
    bonusXp: product?.bonusXp ?? 0,
    active: product?.active ?? true,
    available: product?.available ?? true,
  }));
  const set = <K extends keyof UpsertPOSProductInput>(k: K, v: UpsertPOSProductInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

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
          On the menu today
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
