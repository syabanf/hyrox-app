'use client';

import { MOVEMENT_LABELS } from '@nuhabit/domain';
import { formatDayTime, formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, Pencil } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import { ErrorNote, Field, Modal, PageTitle, SearchSelect, StatCard, StatRow } from '../../../../../components/ui';
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

      <StatRow>
        <StatCard
          label="On hand"
          value={item.trackStock ? `${item.totalOnHand} ${item.unit.toLowerCase()}` : 'not tracked'}
          tone="brand"
        />
        <StatCard
          tone="info"
          label="Average cost"
          value={formatIdr(item.unitCostIdr)}
          hint="What the stock on hand actually cost"
        />
        <StatCard
          tone="ok"
          label="Stock value"
          value={formatIdr(item.totalOnHand * item.unitCostIdr)}
        />
        <StatCard label="Kind" value={<StatusBadge status={item.active ? item.kind : 'INACTIVE'} />} />
      </StatRow>

      <PacksCard itemId={id} baseUnit={item.unit} canManage={can('inventory.manage')} />

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

/**
 * How this item may be handed over.
 *
 * Stock is counted in the base unit throughout; every other pack says how many
 * of those it holds, and carries its own barcode. This is the table that makes
 * "ten cartons" and "240 pieces" the same fact rather than two.
 */
function PacksCard({
  itemId,
  baseUnit,
  canManage,
}: {
  itemId: string;
  baseUnit: string;
  canManage: boolean;
}) {
  const qc = useQueryClient();
  const [adding, setAdding] = useState(false);

  const { data: packs, isLoading } = useQuery({
    queryKey: ['inventory', 'packs', itemId],
    queryFn: () => api.admin.inventory.items.packs(itemId),
  });

  const remove = useMutation({
    mutationFn: (packId: string) => api.admin.inventory.items.deletePack(itemId, packId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['inventory', 'packs', itemId] }),
  });

  return (
    <div className="a-card mb-4 !p-0">
      <div className="flex items-start justify-between gap-3 px-4 pt-4">
        <div>
          <h2 className="font-black">How it is packed</h2>
          <p className="text-xs text-muted">
            Bought by the carton, counted by the {baseUnit.toLowerCase()}. Each pack scans differently.
          </p>
        </div>
        {canManage ? (
          <button className="a-btn-ghost !px-3 !py-1 text-xs" onClick={() => setAdding(true)}>
            + Pack
          </button>
        ) : null}
      </div>

      {isLoading ? (
        <div className="p-4">
          <Spinner label="Loading packs…" />
        </div>
      ) : (
        <table className="a-table mt-2">
          <thead>
            <tr>
              <th>Pack</th>
              <th className="text-right">Holds</th>
              <th>Barcode</th>
              <th>Used for</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(packs ?? []).map((pack) => (
              <tr key={pack.id}>
                <td className="font-bold">
                  {pack.unitCode}
                  {pack.isBase ? <span className="ml-2 text-xs font-normal text-muted">base</span> : null}
                </td>
                <td className="text-right tabular-nums">
                  {pack.factor} {baseUnit.toLowerCase()}
                </td>
                <td className="font-mono text-xs text-muted">{pack.barcode ?? '—'}</td>
                <td className="text-xs text-muted">
                  {[pack.purchaseDefault ? 'buying' : null, pack.saleDefault ? 'selling' : null]
                    .filter(Boolean)
                    .join(' · ') || '—'}
                </td>
                <td className="text-right">
                  {canManage && !pack.isBase ? (
                    <button
                      className="a-btn-ghost !px-2 !py-1 text-xs"
                      onClick={() => remove.mutate(pack.id)}
                    >
                      Remove
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {adding ? (
        <PackModal
          itemId={itemId}
          baseUnit={baseUnit}
          onClose={() => setAdding(false)}
          onDone={() => {
            setAdding(false);
            void qc.invalidateQueries({ queryKey: ['inventory', 'packs', itemId] });
          }}
        />
      ) : null}
    </div>
  );
}

function PackModal({
  itemId,
  baseUnit,
  onClose,
  onDone,
}: {
  itemId: string;
  baseUnit: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    unitCode: '',
    factor: 24,
    barcode: '',
    purchaseDefault: true,
    saleDefault: false,
  });

  const { data: units } = useQuery({
    queryKey: ['inventory', 'units'],
    queryFn: () => api.admin.inventory.units.list(true),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.inventory.items.savePack(itemId, {
        unitCode: form.unitCode,
        factor: form.factor,
        barcode: form.barcode || null,
        purchaseDefault: form.purchaseDefault,
        saleDefault: form.saleDefault,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That pack did not save.'),
  });

  return (
    <Modal title="Add a pack" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Unit">
            <SearchSelect
              value={form.unitCode}
              onChange={(v) => setForm((f) => ({ ...f, unitCode: v }))}
              allowEmpty
              emptyLabel="Choose…"
              placeholder="Search unit…"
              options={(units ?? []).map((unit) => ({
                value: unit.code,
                label: unit.code,
                hint: unit.name,
              }))}
            />
          </Field>
          <Field label={`Holds how many ${baseUnit.toLowerCase()}`}>
            <input
              className="a-input"
              type="number"
              min={1}
              value={form.factor}
              onChange={(e) => setForm((f) => ({ ...f, factor: Number(e.target.value) }))}
            />
          </Field>
        </div>
        <Field label="Barcode" hint="The outer case has its own code, distinct from the single.">
          <input
            className="a-input font-mono"
            value={form.barcode}
            onChange={(e) => setForm((f) => ({ ...f, barcode: e.target.value }))}
          />
        </Field>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.purchaseDefault}
            onChange={(e) => setForm((f) => ({ ...f, purchaseDefault: e.target.checked }))}
          />
          What we normally buy in
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.saleDefault}
            onChange={(e) => setForm((f) => ({ ...f, saleDefault: e.target.checked }))}
          />
          What we normally sell in
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !form.unitCode || form.factor <= 0}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Add pack'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
