'use client';

import type { InventoryItemView, UpsertInventoryItemInput } from '@nuhabit/contracts';
import { ITEM_KINDS } from '@nuhabit/domain';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eye, Pencil } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  Pager,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { Thumb } from '../../../../components/thumb';
import { ExportButton, ImportButton } from '../../../../components/spreadsheet';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

const PAGE_SIZE = 25;

export default function ItemsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('');
  const [kind, setKind] = useState('');
  const [page, setPage] = useState(0);
  const [editing, setEditing] = useState<InventoryItemView | 'new' | null>(null);

  const {
    data: items,
    isLoading,
    error,
  } = useQuery({
    queryKey: ['inventory', 'items', query, category, kind],
    queryFn: () =>
      api.admin.inventory.items.list({
        query: query || undefined,
        categoryId: category || undefined,
        kind: kind || undefined,
      }),
  });
  const { data: categories } = useQuery({
    queryKey: ['inventory', 'categories'],
    queryFn: api.admin.inventory.categories.list,
  });

  const rows = items ?? [];
  const shown = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));

  return (
    <div>
      <PageTitle
        title="Catalogue"
        subtitle="Everything the studio counts"
        actions={
          <>
            <Link href="/inventory" className="a-btn-ghost">
              Stock
            </Link>
            <ExportButton
              filename="items"
              fetcher={() => api.admin.inventory.exportItems()}
            />
            {can('inventory.manage') ? (
              <ImportButton
                title="Import a catalogue"
                hint="However many rows somebody exported from whatever they used before. Typing them in is not a migration plan."
                columns={['sku', 'name', 'category', 'unit', 'cost', 'kind', 'barcode']}
                importer={(csv, apply) => api.admin.inventory.importItems(csv, apply)}
                onDone={() => void qc.invalidateQueries({ queryKey: ['inventory'] })}
              />
            ) : null}
            {can('inventory.manage') ? (
              <button className="a-btn" onClick={() => setEditing('new')}>
                + New item
              </button>
            ) : null}
          </>
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink" label="Items" value={rows.length} />
        <StatCard label="For sale" value={rows.filter((i) => i.kind === 'RETAIL').length} tone="brand" />
        <StatCard tone="danger" label="Studio supplies" value={rows.filter((i) => i.kind === 'SUPPLY').length} />
        <StatCard
          label="Stock value"
          value={formatIdr(rows.reduce((sum, i) => sum + i.totalOnHand * i.unitCostIdr, 0))}
        />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search name, SKU or barcode…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-44">
          <SearchSelect
            value={category}
            onChange={(v) => {
              setCategory(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All categories"
            placeholder="Search category…"
            options={(categories ?? []).map((c) => ({ value: c.id, label: c.name, hint: c.code }))}
          />
        </div>
        <div className="w-40">
          <SearchSelect
            value={kind}
            onChange={(v) => {
              setKind(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="Any kind"
            placeholder="Search kind…"
            options={ITEM_KINDS.map((k) => ({ value: k, label: k[0]! + k.slice(1).toLowerCase() }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading the catalogue…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Item</th>
                  <th>Category</th>
                  <th>Kind</th>
                  <th className="text-right">On hand</th>
                  <th className="text-right">Average cost</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((item) => (
                  <tr key={item.id}>
                    <td>
                      <div className="flex items-center gap-3">
                        <Thumb src={item.imageUrl} name={item.name} />
                        <div className="min-w-0">
                          <Link
                            href={`/inventory/items/${item.id}`}
                            className="font-bold hover:text-brand"
                          >
                            {item.name}
                          </Link>
                          <p className="text-xs text-muted">{item.sku}</p>
                        </div>
                      </div>
                    </td>
                    <td className="text-sm">{item.categoryName ?? <span className="text-muted">—</span>}</td>
                    <td>
                      <StatusBadge status={item.active ? item.kind : 'INACTIVE'} />
                    </td>
                    <td className="text-right tabular-nums">
                      {item.trackStock ? (
                        `${item.totalOnHand} ${item.unit.toLowerCase()}`
                      ) : (
                        <span className="text-muted">not tracked</span>
                      )}
                    </td>
                    <td className="text-right tabular-nums text-sm">{formatIdr(item.unitCostIdr)}</td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Open',
                            icon: Eye,
                            onClick: () => {
                              location.href = `/admin/inventory/items/${item.id}`;
                            },
                          },
                          {
                            label: 'Edit',
                            icon: Pencil,
                            disabled: !can('inventory.manage'),
                            onClick: () => setEditing(item),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      No items match those filters.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}

      {editing ? (
        <ItemModal
          item={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['inventory'] });
          }}
        />
      ) : null}
    </div>
  );
}

function ItemModal({
  item,
  onClose,
  onDone,
}: {
  item: InventoryItemView | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const { data: categories } = useQuery({
    queryKey: ['inventory', 'categories'],
    queryFn: api.admin.inventory.categories.list,
  });

  const [form, setForm] = useState<UpsertInventoryItemInput>(() => ({
    sku: item?.sku ?? '',
    name: item?.name ?? '',
    description: item?.description ?? '',
    categoryId: item?.categoryId ?? null,
    unit: item?.unit ?? 'PCS',
    kind: item?.kind ?? 'RETAIL',
    trackStock: item?.trackStock ?? true,
    trackBatches: item?.trackBatches ?? false,
    expiryWarningDays: item?.expiryWarningDays ?? 30,
    barcode: item?.barcode ?? null,
    active: item?.active ?? true,
  }));
  const set = <K extends keyof UpsertInventoryItemInput>(k: K, v: UpsertInventoryItemInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const save = useMutation({
    mutationFn: () =>
      item
        ? api.admin.inventory.items.update(item.id, form)
        : api.admin.inventory.items.create(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That item did not save.'),
  });

  return (
    <Modal title={item ? `Edit ${item.name}` : 'New item'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="SKU">
            <input className="a-input" value={form.sku} onChange={(e) => set('sku', e.target.value)} />
          </Field>
          <Field label="Unit" hint="What stock is counted in.">
            <input className="a-input" value={form.unit ?? ''} onChange={(e) => set('unit', e.target.value)} />
          </Field>
        </div>
        <Field label="Name">
          <input className="a-input" value={form.name} onChange={(e) => set('name', e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Category">
            <SearchSelect
              value={form.categoryId ?? ''}
              onChange={(v) => set('categoryId', v || null)}
              allowEmpty
              emptyLabel="None"
              placeholder="Search category…"
              options={(categories ?? []).map((c) => ({ value: c.id, label: c.name, hint: c.code }))}
            />
          </Field>
          <Field label="Kind" hint="Retail is sold; supply is only consumed.">
            <SearchSelect
              value={form.kind ?? 'RETAIL'}
              onChange={(v) => set('kind', v)}
              placeholder="Search kind…"
              options={ITEM_KINDS.map((k) => ({ value: k, label: k[0]! + k.slice(1).toLowerCase() }))}
            />
          </Field>
        </div>
        <Field label="Barcode">
          <input
            className="a-input"
            value={form.barcode ?? ''}
            onChange={(e) => set('barcode', e.target.value || null)}
          />
        </Field>
        <Field label="Description">
          <textarea
            className="a-input"
            rows={2}
            value={form.description ?? ''}
            onChange={(e) => set('description', e.target.value)}
          />
        </Field>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.trackStock ?? true}
            onChange={(e) => set('trackStock', e.target.checked)}
          />
          Counted as stock
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.trackBatches ?? false}
            onChange={(e) => set('trackBatches', e.target.checked)}
          />
          Goes off
        </label>
        {form.trackBatches ? (
          <Field
            label="Warn this many days before"
            hint="Every delivery of this item will have to name its batch and date. Stock leaves in date order, soonest first."
          >
            <input
              className="a-input max-w-[10rem]"
              type="number"
              min={0}
              value={form.expiryWarningDays ?? 30}
              onChange={(e) => set('expiryWarningDays', Number(e.target.value))}
            />
          </Field>
        ) : null}
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active ?? true}
            onChange={(e) => set('active', e.target.checked)}
          />
          In the catalogue
        </label>
        <p className="text-xs text-muted">
          The average cost is not set here: it follows what deliveries actually cost.
        </p>
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

