'use client';

import { formatDayTime, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Truck } from 'lucide-react';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * The receiving bay.
 *
 * What came off the truck, before anybody judged it. Nothing here moves stock:
 * that happens when a goods receipt inspects the delivery, which is a
 * different act by a different person, often on a different day.
 */
export default function DeliveriesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [status, setStatus] = useState('');
  const [opening, setOpening] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);

  const { data: deliveries, isLoading, error } = useQuery({
    queryKey: ['purchasing', 'deliveries', status],
    queryFn: () => api.admin.purchasing.deliveries.list({ status: status || undefined, limit: 100 }),
  });

  const rows = deliveries ?? [];
  const waiting = rows.filter((d) => d.status === 'ARRIVED').length;

  return (
    <div>
      <PageTitle
        title="Receiving bay"
        subtitle="What came off the truck, before anybody judged it"
        actions={
          can('purchasing.receive') ? (
            <button className="a-btn" onClick={() => setOpening(true)}>
              <Truck size={16} /> Record an arrival
            </button>
          ) : null
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard tone="ink" label="Arrivals" value={rows.length} icon={Truck} />
        <StatCard
          label="Waiting to be checked"
          value={waiting}
          tone={waiting > 0 ? 'brand' : undefined}
          hint="Counted at the bay, not yet in stock"
        />
        <StatCard
          tone="ok"
          label="Checked in"
          value={rows.filter((d) => d.status === 'INSPECTED').length}
        />
      </div>

      <div className="mb-3 flex gap-1 rounded-xl border border-line p-1">
        {[
          { value: '', label: 'Everything' },
          { value: 'ARRIVED', label: 'Waiting' },
          { value: 'INSPECTED', label: 'Checked in' },
        ].map((tab) => (
          <button
            key={tab.value}
            onClick={() => setStatus(tab.value)}
            className={`rounded-lg px-3 py-1 text-xs font-bold transition ${
              status === tab.value ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Spinner label="Loading arrivals…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Delivery</th>
                  <th>Delivery note</th>
                  <th>Driver</th>
                  <th>Arrived</th>
                  <th>Status</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rows.map((delivery) => (
                  <tr key={delivery.id}>
                    <td className="font-bold">{delivery.deliveryNumber}</td>
                    <td className="font-mono text-xs">{delivery.deliveryNoteNumber ?? '—'}</td>
                    <td className="text-sm text-muted">
                      {delivery.driverName ?? '—'}
                      {delivery.vehicle ? ` · ${delivery.vehicle}` : ''}
                    </td>
                    <td className="text-sm">{formatDayTime(delivery.arrivedAt)}</td>
                    <td>
                      <StatusBadge status={delivery.status} />
                    </td>
                    <td className="text-right">
                      <button
                        className="a-btn-ghost !px-3 !py-1 text-xs"
                        onClick={() => setSelected(delivery.id)}
                      >
                        Open
                      </button>
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      Nothing has arrived.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {opening ? (
        <OpenDeliveryModal
          onClose={() => setOpening(false)}
          onDone={(id) => {
            setOpening(false);
            setSelected(id);
            void qc.invalidateQueries({ queryKey: ['purchasing', 'deliveries'] });
          }}
        />
      ) : null}

      {selected ? (
        <DeliverySheet
          deliveryId={selected}
          canReceive={can('purchasing.receive')}
          onClose={() => setSelected(null)}
          onChanged={() => void qc.invalidateQueries({ queryKey: ['purchasing'] })}
        />
      ) : null}
    </div>
  );
}

function OpenDeliveryModal({
  onClose,
  onDone,
}: {
  onClose: () => void;
  onDone: (id: string) => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    orderId: '',
    deliveryNoteNumber: '',
    driverName: '',
    vehicle: '',
  });

  // Only orders that have been sent can have anything arrive against them.
  const { data: orders } = useQuery({
    queryKey: ['purchasing', 'orders', 'open'],
    queryFn: () => api.admin.purchasing.orders.list({ limit: 200 }),
  });
  // The list endpoint returns orders without their supplier's name, and a
  // picker showing bare PO numbers is a picker nobody can use.
  const { data: suppliers } = useQuery({
    queryKey: ['purchasing', 'suppliers', 'all'],
    queryFn: () => api.admin.purchasing.suppliers.list({ limit: 500 }),
  });
  const supplierNames = new Map((suppliers ?? []).map((s) => [s.id, s.name]));

  const open = (orders ?? []).filter((o) =>
    ['APPROVED', 'SENT', 'PARTIALLY_RECEIVED'].includes(o.status),
  );

  const save = useMutation({
    mutationFn: () =>
      api.admin.purchasing.deliveries.open({
        orderId: form.orderId,
        deliveryNoteNumber: form.deliveryNoteNumber || null,
        driverName: form.driverName || null,
        vehicle: form.vehicle || null,
      }),
    onSuccess: (delivery) => onDone(delivery.id),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That arrival did not save.'),
  });

  return (
    <Modal title="Record an arrival" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="text-sm text-muted">
          This records what was handed over. It moves no stock and makes no judgement — a goods
          receipt does that afterwards.
        </p>
        <Field label="Against which order">
          <SearchSelect
            value={form.orderId}
            onChange={(v) => setForm((f) => ({ ...f, orderId: v }))}
            placeholder="Search purchase order…"
            options={open.map((o) => ({
              value: o.id,
              label: o.poNumber,
              hint: `${supplierNames.get(o.supplierId) ?? 'supplier'} · ${o.status.toLowerCase().replace('_', ' ')}`,
            }))}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Their delivery note" hint="The number to quote back when the count disagrees.">
            <input
              className="a-input font-mono"
              value={form.deliveryNoteNumber}
              onChange={(e) => setForm((f) => ({ ...f, deliveryNoteNumber: e.target.value }))}
            />
          </Field>
          <Field label="Driver">
            <input
              className="a-input"
              value={form.driverName}
              onChange={(e) => setForm((f) => ({ ...f, driverName: e.target.value }))}
            />
          </Field>
        </div>
        <Field label="Vehicle">
          <input
            className="a-input"
            value={form.vehicle}
            onChange={(e) => setForm((f) => ({ ...f, vehicle: e.target.value }))}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={!form.orderId || save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Record it'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function DeliverySheet({
  deliveryId,
  canReceive,
  onClose,
  onChanged,
}: {
  deliveryId: string;
  canReceive: boolean;
  onClose: () => void;
  onChanged: () => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [line, setLine] = useState('');
  const [qty, setQty] = useState('');
  const [batch, setBatch] = useState('');
  const [expires, setExpires] = useState('');

  const { data: delivery, isLoading } = useQuery({
    queryKey: ['purchasing', 'delivery', deliveryId],
    queryFn: () => api.admin.purchasing.deliveries.get(deliveryId),
  });
  const { data: order } = useQuery({
    queryKey: ['purchasing', 'order', delivery?.orderId],
    queryFn: () => api.admin.purchasing.orders.get(delivery!.orderId),
    enabled: !!delivery,
  });
  const { data: items } = useQuery({
    queryKey: ['inventory', 'items', 'all'],
    queryFn: () => api.admin.inventory.items.list(),
  });

  const chosen = (order?.items ?? []).find((i) => i.id === line);
  const dated = Boolean(
    chosen && (items ?? []).find((i) => i.id === chosen.itemId)?.trackBatches,
  );

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['purchasing', 'delivery', deliveryId] });
    onChanged();
  };

  const add = useMutation({
    mutationFn: () =>
      api.admin.purchasing.deliveries.addLine(deliveryId, {
        orderItemId: line,
        qty: Number(qty),
        batchNumber: batch || null,
        expiresOn: expires || null,
      }),
    onSuccess: () => {
      setError(null);
      setLine('');
      setQty('');
      setBatch('');
      setExpires('');
      refresh();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That line did not save.'),
  });

  const close = useMutation({
    mutationFn: () => api.admin.purchasing.deliveries.close(deliveryId),
    onSuccess: refresh,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That delivery did not close.'),
  });

  if (isLoading || !delivery) return <Modal title="Delivery" onClose={onClose}><Spinner label="Loading…" /></Modal>;

  return (
    <Modal
      title={`${delivery.deliveryNumber} · ${delivery.supplierName}`}
      onClose={onClose}
      wide
    >
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="rounded-lg bg-surface-raised px-3 py-2 text-xs text-muted">
          Counted at the bay. Nothing enters stock until a goods receipt accepts or rejects it —
          which is why a short delivery and a rejection are different things here.
        </p>

        <div className="grid gap-3 sm:grid-cols-3">
          <StatCard tone="brand" label="Delivered" value={delivery.inspection.delivered} />
          <StatCard tone="info" label="Judged" value={delivery.inspection.inspected} />
          <StatCard
            label="Still to check"
            value={delivery.inspection.outstanding}
            tone={delivery.inspection.outstanding > 0 ? 'brand' : undefined}
          />
        </div>

        {canReceive && delivery.status === 'ARRIVED' ? (
          <div className="grid gap-2">
            <SearchSelect
              value={line}
              onChange={setLine}
              placeholder="Search an ordered line…"
              options={(order?.items ?? []).map((i) => ({
                value: i.id,
                label: i.itemName || i.description,
                hint: `${i.qtyOrdered} ${i.unit.toLowerCase()} ordered`,
              }))}
            />
            <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
              <input
                className="a-input"
                placeholder={`How many ${chosen?.unit.toLowerCase() ?? 'units'} arrived`}
                value={qty}
                onChange={(e) => setQty(e.target.value)}
              />
              <button
                className="a-btn"
                disabled={add.isPending || !line || Number(qty) <= 0 || (dated && !batch)}
                onClick={() => add.mutate()}
              >
                Add
              </button>
            </div>
            {dated ? (
              <div className="grid gap-2 sm:grid-cols-2">
                <input
                  className="a-input font-mono"
                  placeholder="Batch on the delivery note"
                  value={batch}
                  onChange={(e) => setBatch(e.target.value)}
                />
                <input
                  className="a-input"
                  type="date"
                  value={expires}
                  onChange={(e) => setExpires(e.target.value)}
                />
              </div>
            ) : null}
          </div>
        ) : null}

        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Item</th>
                <th className="text-right">Delivered</th>
                <th className="text-right">Accepted</th>
                <th className="text-right">Rejected</th>
                <th className="text-right">To check</th>
              </tr>
            </thead>
            <tbody>
              {delivery.items.map((item) => (
                <tr key={item.id}>
                  <td>
                    <p className="font-bold">{item.itemName}</p>
                    {item.batchNumber ? (
                      <p className="font-mono text-xs text-muted">
                        {item.batchNumber}
                        {item.expiresOn ? ` · ${item.expiresOn}` : ''}
                      </p>
                    ) : null}
                  </td>
                  <td className="text-right tabular-nums">
                    {item.qtyDelivered} {item.unit.toLowerCase()}
                  </td>
                  <td className="text-right tabular-nums">{item.qtyAccepted}</td>
                  <td className="text-right tabular-nums text-sm text-danger">
                    {item.qtyRejected > 0 ? item.qtyRejected : '—'}
                  </td>
                  <td className="text-right tabular-nums font-bold">
                    {item.outstanding > 0 ? item.outstanding : '—'}
                  </td>
                </tr>
              ))}
              {delivery.items.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-6 text-center text-sm text-muted">
                    Nothing counted yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <div className="mt-1 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Close
          </button>
          {canReceive && delivery.status === 'ARRIVED' ? (
            <button
              className="a-btn"
              disabled={close.isPending || !delivery.inspection.complete}
              title={
                delivery.inspection.complete
                  ? undefined
                  : 'Everything on it has to be accepted or rejected first'
              }
              onClick={() => close.mutate()}
            >
              {close.isPending ? 'Closing…' : 'Mark as checked in'}
            </button>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}
