'use client';

import type { StockRowView } from '@nuhabit/contracts';
import { formatIdr, Spinner } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeftRight, PackagePlus, Pencil } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Modal,
  Pager,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../components/ui';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';

const PAGE_SIZE = 25;

export default function StockPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branch, setBranch] = useState('');
  const [category, setCategory] = useState('');
  const [query, setQuery] = useState('');
  const [lowOnly, setLowOnly] = useState(false);
  const [page, setPage] = useState(0);
  const [adjusting, setAdjusting] = useState<StockRowView | null>(null);
  const [transferring, setTransferring] = useState<StockRowView | null>(null);

  const { data: overview } = useQuery({
    queryKey: ['inventory', 'overview', branch],
    queryFn: () => api.admin.inventory.overview(branch || undefined),
  });
  const {
    data: rows,
    isLoading,
    error,
  } = useQuery({
    queryKey: ['inventory', 'stock', branch, category, query, lowOnly],
    queryFn: () =>
      api.admin.inventory.stock({
        branchId: branch || undefined,
        categoryId: category || undefined,
        query: query || undefined,
        lowOnly: lowOnly ? 'true' : undefined,
      }),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data: categories } = useQuery({
    queryKey: ['inventory', 'categories'],
    queryFn: api.admin.inventory.categories.list,
  });

  const shown = (rows ?? []).slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const pageCount = Math.max(1, Math.ceil((rows ?? []).length / PAGE_SIZE));

  return (
    <div>
      <PageTitle
        title="Stock"
        subtitle="What is on the shelves, and where"
        actions={
          <Link href="/inventory/items" className="a-btn-ghost">
            Catalogue
          </Link>
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Stock on hand"
          value={overview ? formatIdr(overview.valuation.valueIdr) : '—'}
          hint={overview ? `${overview.valuation.items} shelves · ${overview.valuation.units} units` : undefined}
          tone="brand"
        />
        <StatCard
          label="Below reorder point"
          value={overview?.valuation.lowStock ?? '—'}
          tone={overview && overview.valuation.lowStock > 0 ? 'danger' : undefined}
        />
        <StatCard tone="danger" label="Out of stock" value={overview?.valuation.outOfStock ?? '—'} />
        <StatCard tone="ok" label="Items tracked" value={overview?.valuation.items ?? '—'} />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search item or SKU…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-44">
          <SearchSelect
            value={branch}
            onChange={(v) => {
              setBranch(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
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
        <label className="flex items-center gap-2 text-sm font-bold">
          <input type="checkbox" checked={lowOnly} onChange={(e) => setLowOnly(e.target.checked)} />
          Needs reordering
        </label>
      </div>

      {isLoading ? (
        <Spinner label="Counting the shelves…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Item</th>
                  <th>Branch</th>
                  <th className="text-right">On hand</th>
                  <th className="text-right">On order</th>
                  <th className="text-right">Minimum</th>
                  <th className="text-right">Value</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((row) => (
                  <tr key={`${row.item.id}-${row.level.branchId}`}>
                    <td>
                      <Link href={`/inventory/items/${row.item.id}`} className="font-bold hover:text-brand">
                        {row.item.name}
                      </Link>
                      <p className="text-xs text-muted">
                        {row.item.sku} · {formatIdr(row.item.unitCostIdr)} avg
                      </p>
                    </td>
                    <td className="text-sm">{row.level.branchName}</td>
                    <td className="text-right tabular-nums">
                      <span className={row.level.lowStock ? 'font-bold text-danger' : ''}>
                        {row.level.qtyOnHand} {row.item.unit.toLowerCase()}
                      </span>
                      {row.level.lowStock ? (
                        <p className="text-xs text-danger">reorder {row.level.reorderQty}</p>
                      ) : null}
                    </td>
                    <td className="text-right tabular-nums text-sm">
                      {row.level.qtyOnOrder > 0 ? row.level.qtyOnOrder : '—'}
                    </td>
                    <td className="text-right tabular-nums text-sm">{row.level.qtyMinimum}</td>
                    <td className="text-right tabular-nums text-sm">{formatIdr(row.level.valueIdr)}</td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Adjust',
                            icon: PackagePlus,
                            disabled: !can('inventory.count'),
                            onClick: () => setAdjusting(row),
                          },
                          {
                            label: 'Transfer',
                            icon: ArrowLeftRight,
                            disabled: !can('inventory.count'),
                            onClick: () => setTransferring(row),
                          },
                          {
                            label: 'Edit item',
                            icon: Pencil,
                            disabled: !can('inventory.manage'),
                            onClick: () => {
                              location.href = `/admin/inventory/items/${row.item.id}`;
                            },
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      Nothing on the shelves matches those filters.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}

      {adjusting ? (
        <AdjustModal
          row={adjusting}
          onClose={() => setAdjusting(null)}
          onDone={() => {
            setAdjusting(null);
            void qc.invalidateQueries({ queryKey: ['inventory'] });
          }}
        />
      ) : null}
      {transferring ? (
        <TransferModal
          row={transferring}
          branches={(branches ?? []).map((b) => ({ id: b.id, name: b.name }))}
          onClose={() => setTransferring(null)}
          onDone={() => {
            setTransferring(null);
            void qc.invalidateQueries({ queryKey: ['inventory'] });
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * A manual correction. The reason is required by the server as well as here,
 * because an unexplained change to a quantity is indistinguishable from theft.
 */
function AdjustModal({
  row,
  onClose,
  onDone,
}: {
  row: StockRowView;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [qty, setQty] = useState('');
  const [reason, setReason] = useState('');

  const save = useMutation({
    mutationFn: () =>
      api.admin.inventory.adjust({
        itemId: row.item.id,
        branchId: row.level.branchId,
        qty: Number(qty),
        reason,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That adjustment did not save.'),
  });

  const parsed = Number(qty);
  const after = row.level.qtyOnHand + (Number.isFinite(parsed) ? parsed : 0);

  return (
    <Modal title={`Adjust ${row.item.name}`} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
          <span className="font-bold">
            {row.level.qtyOnHand} {row.item.unit.toLowerCase()}
          </span>{' '}
          on hand at {row.level.branchName}.
        </p>
        <label className="block">
          <span className="mb-1 block text-[11px] font-bold uppercase tracking-wider text-muted">
            Change
          </span>
          <input
            className="a-input"
            placeholder="-5 to take away, 12 to add"
            value={qty}
            onChange={(e) => setQty(e.target.value)}
          />
          <span className="mt-1 block text-xs text-muted">
            {qty && Number.isFinite(parsed) && parsed !== 0
              ? `Leaves ${after} ${row.item.unit.toLowerCase()}.`
              : 'A signed number: negative takes stock away.'}
          </span>
        </label>
        <label className="block">
          <span className="mb-1 block text-[11px] font-bold uppercase tracking-wider text-muted">
            Reason
          </span>
          <input
            className="a-input"
            placeholder="Damaged in the stockroom"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !reason.trim() || !parsed || after < 0}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Adjust'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function TransferModal({
  row,
  branches,
  onClose,
  onDone,
}: {
  row: StockRowView;
  branches: { id: string; name: string }[];
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [to, setTo] = useState('');
  const [qty, setQty] = useState('');
  const [note, setNote] = useState('');

  const save = useMutation({
    mutationFn: () =>
      api.admin.inventory.transfer({
        itemId: row.item.id,
        fromBranchId: row.level.branchId,
        toBranchId: to,
        qty: Number(qty),
        note: note || null,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That transfer did not save.'),
  });

  return (
    <Modal title={`Move ${row.item.name}`} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
          Moving out of <span className="font-bold">{row.level.branchName}</span>, which has{' '}
          {row.level.qtyOnHand} {row.item.unit.toLowerCase()}.
        </p>
        <label className="block">
          <span className="mb-1 block text-[11px] font-bold uppercase tracking-wider text-muted">
            To branch
          </span>
          <SearchSelect
            value={to}
            onChange={setTo}
            placeholder="Search branch…"
            options={branches
              .filter((b) => b.id !== row.level.branchId)
              .map((b) => ({ value: b.id, label: b.name }))}
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-[11px] font-bold uppercase tracking-wider text-muted">
            Quantity
          </span>
          <input className="a-input" value={qty} onChange={(e) => setQty(e.target.value)} />
        </label>
        <label className="block">
          <span className="mb-1 block text-[11px] font-bold uppercase tracking-wider text-muted">
            Note
          </span>
          <input className="a-input" value={note} onChange={(e) => setNote(e.target.value)} />
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !to || Number(qty) <= 0}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Moving…' : 'Transfer'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
