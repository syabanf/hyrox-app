'use client';

import { MOVEMENT_LABELS } from '@nuhabit/domain';
import { formatDayTime, formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, Pencil } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import { ErrorNote, Field, Modal, PageTitle, StatCard } from '../../../../../components/ui';
import { api, ApiError } from '../../../../../lib/api';
import { usePermissions } from '../../../../../lib/auth';

export default function ItemPage() {
  const { id } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [reordering, setReordering] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['inventory', 'item', id],
    queryFn: () => api.admin.inventory.items.get(id),
  });

  if (isLoading || !data) return <Spinner label="Loading the item…" />;
  const { item, levels, movements } = data;

  return (
    <div>
      <Link
        href="/inventory/items"
        className="mb-3 inline-flex items-center gap-1.5 text-sm font-bold text-muted hover:text-ink"
      >
        <ArrowLeft size={14} /> Catalogue
      </Link>
      <PageTitle title={item.name} subtitle={`${item.sku} · ${item.categoryName ?? 'uncategorised'}`} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard
          label="On hand"
          value={item.trackStock ? `${item.totalOnHand} ${item.unit.toLowerCase()}` : 'not tracked'}
          tone="brand"
        />
        <StatCard
          label="Average cost"
          value={formatIdr(item.unitCostIdr)}
          hint="What the stock on hand actually cost"
        />
        <StatCard
          label="Stock value"
          value={formatIdr(item.totalOnHand * item.unitCostIdr)}
        />
        <StatCard label="Kind" value={<StatusBadge status={item.active ? item.kind : 'INACTIVE'} />} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="a-card !p-0">
          <h2 className="px-4 pt-4 font-black">Where it is</h2>
          <p className="px-4 pb-2 text-xs text-muted">
            Stock is held per branch. Reorder maths counts what is already on its way, so a
            purchase order already raised does not trigger a second one.
          </p>
          <table className="a-table">
            <thead>
              <tr>
                <th>Branch</th>
                <th className="text-right">On hand</th>
                <th className="text-right">On order</th>
                <th className="text-right">Minimum</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {levels.map((level) => (
                <tr key={level.branchId}>
                  <td className="font-bold">{level.branchName}</td>
                  <td className="text-right tabular-nums">
                    <span className={level.lowStock ? 'font-bold text-danger' : ''}>
                      {level.qtyOnHand}
                    </span>
                    {level.lowStock ? (
                      <p className="text-xs text-danger">reorder {level.reorderQty}</p>
                    ) : null}
                  </td>
                  <td className="text-right tabular-nums text-sm">{level.qtyOnOrder}</td>
                  <td className="text-right tabular-nums text-sm">
                    {level.qtyMinimum}
                    {level.qtyMaximum != null ? (
                      <span className="text-muted"> – {level.qtyMaximum}</span>
                    ) : null}
                  </td>
                  <td className="text-right">
                    <button
                      className="a-btn-ghost !px-2 !py-1 text-xs disabled:opacity-40"
                      disabled={!can('inventory.manage')}
                      onClick={() => setReordering(level.branchId)}
                    >
                      <Pencil size={12} className="mr-1 inline" /> Levels
                    </button>
                  </td>
                </tr>
              ))}
              {levels.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-sm text-muted">
                    Not stocked anywhere yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <div className="a-card !p-0">
          <h2 className="px-4 pt-4 font-black">Every movement</h2>
          <p className="px-4 pb-2 text-xs text-muted">
            The ledger is append-only. A miscount is corrected with an adjustment that says so,
            never by editing history.
          </p>
          <div className="max-h-[28rem] overflow-y-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>When</th>
                  <th>What</th>
                  <th className="text-right">Change</th>
                  <th className="text-right">After</th>
                </tr>
              </thead>
              <tbody>
                {movements.map((m) => (
                  <tr key={m.id}>
                    <td className="text-xs text-muted">{formatDayTime(m.createdAt)}</td>
                    <td className="text-sm">
                      <span className="font-bold">{MOVEMENT_LABELS[m.kind]}</span>
                      {m.referenceNumber ? (
                        <p className="text-xs text-muted">{m.referenceNumber}</p>
                      ) : m.reason ? (
                        <p className="text-xs text-muted">{m.reason}</p>
                      ) : null}
                    </td>
                    <td
                      className={`text-right tabular-nums font-bold ${
                        m.qty < 0 ? 'text-danger' : 'text-brand'
                      }`}
                    >
                      {m.qty > 0 ? '+' : ''}
                      {m.qty}
                    </td>
                    <td className="text-right tabular-nums text-sm">{m.qtyAfter}</td>
                  </tr>
                ))}
                {movements.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="py-8 text-center text-sm text-muted">
                      Nothing has moved yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {reordering ? (
        <ReorderModal
          itemId={id}
          level={levels.find((l) => l.branchId === reordering)!}
          onClose={() => setReordering(null)}
          onDone={() => {
            setReordering(null);
            void qc.invalidateQueries({ queryKey: ['inventory'] });
          }}
        />
      ) : null}
    </div>
  );
}

function ReorderModal({
  itemId,
  level,
  onClose,
  onDone,
}: {
  itemId: string;
  level: { branchId: string; branchName: string; qtyMinimum: number; qtyMaximum: number | null; binLocation: string | null };
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [minimum, setMinimum] = useState(String(level.qtyMinimum));
  const [maximum, setMaximum] = useState(level.qtyMaximum == null ? '' : String(level.qtyMaximum));
  const [bin, setBin] = useState(level.binLocation ?? '');

  const save = useMutation({
    mutationFn: () =>
      api.admin.inventory.items.setReorderPoint(itemId, {
        branchId: level.branchId,
        qtyMinimum: Number(minimum),
        qtyMaximum: maximum === '' ? null : Number(maximum),
        binLocation: bin || null,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Those levels did not save.'),
  });

  return (
    <Modal title={`Reorder levels at ${level.branchName}`} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Minimum" hint="Below this, the shelf shows as needing a reorder.">
          <input className="a-input" value={minimum} onChange={(e) => setMinimum(e.target.value)} />
        </Field>
        <Field label="Maximum" hint="What a reorder fills up to. Leave empty to fill to the minimum.">
          <input className="a-input" value={maximum} onChange={(e) => setMaximum(e.target.value)} />
        </Field>
        <Field label="Shelf location">
          <input className="a-input" value={bin} onChange={(e) => setBin(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
