'use client';

import type { GoodsReceiptView } from '@nuhabit/contracts';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, Ban, PackageCheck, Send, Trash2 } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../../components/ui';
import { api, ApiError } from '../../../../../lib/api';
import { usePermissions } from '../../../../../lib/auth';

export default function OrderPage() {
  const { id } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [receiving, setReceiving] = useState<GoodsReceiptView | null>(null);

  const { data: order, isLoading } = useQuery({
    queryKey: ['purchasing', 'order', id],
    queryFn: () => api.admin.purchasing.orders.get(id),
  });

  const act = useMutation({
    mutationFn: async (action: 'approve' | 'send' | 'cancel') => {
      if (action === 'cancel') {
        const reason = prompt('Why is this order being cancelled?') ?? '';
        if (!reason.trim()) throw new ApiError(422, 'VALIDATION_FAILED', 'A cancellation needs a reason.');
        return api.admin.purchasing.orders.cancel(id, reason);
      }
      return action === 'approve'
        ? api.admin.purchasing.orders.approve(id)
        : api.admin.purchasing.orders.send(id);
    },
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That did not go through.'),
  });

  const removeLine = useMutation({
    mutationFn: (lineId: string) => api.admin.purchasing.orders.removeLine(id, lineId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['purchasing'] }),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That line did not delete.'),
  });

  const openReceipt = useMutation({
    mutationFn: () => api.admin.purchasing.receipts.open({ orderId: id }),
    onSuccess: (receipt) => {
      setError(null);
      setReceiving(receipt);
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That delivery did not open.'),
  });

  if (isLoading || !order) return <Spinner label="Loading the order…" />;
  const isDraft = order.status === 'DRAFT';
  const canReceive = ['APPROVED', 'SENT', 'PARTIALLY_RECEIVED'].includes(order.status);

  return (
    <div>
      <Link
        href="/purchasing"
        className="mb-3 inline-flex items-center gap-1.5 text-sm font-bold text-muted hover:text-ink"
      >
        <ArrowLeft size={14} /> Purchase orders
      </Link>
      <PageTitle
        title={order.poNumber}
        subtitle={`${order.supplierName} · ${order.branchName}`}
        actions={
          <>
            {can('purchasing.approve') && isDraft ? (
              <button className="a-btn-ghost" onClick={() => act.mutate('approve')}>
                Approve
              </button>
            ) : null}
            {can('purchasing.manage') && order.status === 'APPROVED' ? (
              <button className="a-btn" onClick={() => act.mutate('send')}>
                <Send size={14} className="mr-1.5 inline" /> Send to supplier
              </button>
            ) : null}
            {can('purchasing.receive') && canReceive ? (
              <button className="a-btn" onClick={() => openReceipt.mutate()}>
                <PackageCheck size={14} className="mr-1.5 inline" /> Record a delivery
              </button>
            ) : null}
            {can('purchasing.manage') && !['RECEIVED', 'PARTIALLY_RECEIVED', 'CANCELLED'].includes(order.status) ? (
              <button className="a-btn-ghost" onClick={() => act.mutate('cancel')}>
                <Ban size={14} className="mr-1.5 inline" /> Cancel
              </button>
            ) : null}
          </>
        }
      />
      <ErrorNote message={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard label="Status" value={<StatusBadge status={order.status} />} />
        <StatCard label="Subtotal" value={formatIdr(order.subtotalIdr)} />
        <StatCard label={`Tax (${order.taxPercent}%)`} value={formatIdr(order.taxIdr)} />
        <StatCard label="Total" value={formatIdr(order.totalIdr)} tone="brand" />
      </div>

      <div className="a-card !p-0">
        <div className="flex items-center justify-between px-4 py-3">
          <div>
            <h2 className="font-black">Lines</h2>
            <p className="text-xs text-muted">
              More cannot be delivered than was ordered — over-delivery is a conversation with the
              supplier, not a quantity the system invents.
            </p>
          </div>
          {can('purchasing.manage') && isDraft ? (
            <button className="a-btn !px-3 !py-1.5 text-xs" onClick={() => setAdding(true)}>
              + Line
            </button>
          ) : null}
        </div>
        <table className="a-table">
          <thead>
            <tr>
              <th>Item</th>
              <th className="text-right">Ordered</th>
              <th className="text-right">Received</th>
              <th className="text-right">Outstanding</th>
              <th className="text-right">Unit price</th>
              <th className="text-right">Subtotal</th>
              <th className="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {order.items.map((line) => (
              <tr key={line.id}>
                <td>
                  <p className="font-bold">{line.itemName || line.description}</p>
                  <p className="text-xs text-muted">{line.unit.toLowerCase()}</p>
                </td>
                <td className="text-right tabular-nums">
                  {line.qtyOrdered}
                  {line.packFactor > 1 ? (
                    <span className="block text-xs font-normal text-muted">
                      {line.qtyOrderedBase} into stock
                    </span>
                  ) : null}
                </td>
                <td className="text-right tabular-nums text-sm">{line.qtyReceived}</td>
                <td className="text-right tabular-nums text-sm">
                  {line.qtyOutstanding > 0 ? (
                    <span className="font-bold">{line.qtyOutstanding}</span>
                  ) : (
                    <span className="text-muted">—</span>
                  )}
                </td>
                <td className="text-right tabular-nums text-sm">{formatIdr(line.unitPriceIdr)}</td>
                <td className="text-right tabular-nums">{formatIdr(line.subtotalIdr)}</td>
                <td className="text-right">
                  <RowActions
                    items={[
                      {
                        label: 'Remove',
                        icon: Trash2,
                        tone: 'danger' as const,
                        disabled: !can('purchasing.manage') || !isDraft,
                        onClick: () => removeLine.mutate(line.id),
                      },
                    ]}
                  />
                </td>
              </tr>
            ))}
            {order.items.length === 0 ? (
              <tr>
                <td colSpan={7} className="py-8 text-center text-sm text-muted">
                  Nothing on this order yet.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      {(order.receipts ?? []).length > 0 ? (
        <div className="a-card mt-4 !p-0">
          <h2 className="px-4 pt-4 font-black">Deliveries</h2>
          <table className="a-table">
            <thead>
              <tr>
                <th>Receipt</th>
                <th>Received on</th>
                <th>Delivery note</th>
                <th>Status</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {(order.receipts ?? []).map((receipt) => (
                <tr key={receipt.id}>
                  <td className="font-bold">{receipt.grnNumber}</td>
                  <td className="tabular-nums text-sm">{receipt.receivedOn}</td>
                  <td className="text-sm">{receipt.deliveryNoteNumber ?? <span className="text-muted">—</span>}</td>
                  <td>
                    <StatusBadge status={receipt.status} />
                  </td>
                  <td className="text-right">
                    <button
                      className="a-btn-ghost !px-3 !py-1 text-xs"
                      onClick={async () => {
                        const full = await api.admin.purchasing.receipts.get(receipt.id);
                        setReceiving(full);
                      }}
                    >
                      Open
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {adding ? (
        <AddLineModal
          orderId={id}
          onClose={() => setAdding(false)}
          onDone={() => {
            setAdding(false);
            void qc.invalidateQueries({ queryKey: ['purchasing'] });
          }}
        />
      ) : null}
      {receiving ? (
        <ReceiveSheet
          receiptId={receiving.id}
          onClose={() => {
            setReceiving(null);
            void qc.invalidateQueries({ queryKey: ['purchasing', 'inventory'] });
            void qc.invalidateQueries({ queryKey: ['inventory'] });
          }}
        />
      ) : null}
    </div>
  );
}

function AddLineModal({
  orderId,
  onClose,
  onDone,
}: {
  orderId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [item, setItem] = useState('');
  const [unit, setUnit] = useState('');
  const [qty, setQty] = useState('');
  const [price, setPrice] = useState('');

  const { data: items } = useQuery({
    queryKey: ['inventory', 'items', 'all'],
    queryFn: () => api.admin.inventory.items.list(),
  });

  // The packs this item is stocked in. A buyer orders cartons, so the form
  // asks which pack rather than assuming the unit the ledger counts in.
  const { data: packs } = useQuery({
    queryKey: ['inventory', 'packs', item],
    queryFn: () => api.admin.inventory.items.packs(item),
    enabled: Boolean(item),
  });

  // Whatever this item is normally bought by, unless the buyer says otherwise.
  const chosenPack = (packs ?? []).find((p) => p.unitCode === unit)
    ?? (packs ?? []).find((p) => p.purchaseDefault)
    ?? (packs ?? []).find((p) => p.isBase);

  const save = useMutation({
    mutationFn: () => {
      const chosen = (items ?? []).find((i) => i.id === item);
      return api.admin.purchasing.orders.addLine(orderId, {
        itemId: item,
        description: chosen?.name ?? item,
        qty: Number(qty),
        unit: chosenPack?.unitCode,
        unitPriceIdr: Number(price),
      });
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That line did not save.'),
  });

  return (
    <Modal title="Add a line" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Item">
          <SearchSelect
            value={item}
            onChange={(v) => {
              setItem(v);
              setUnit('');
            }}
            placeholder="Search item…"
            options={(items ?? []).map((i) => ({ value: i.id, label: i.name, hint: i.sku }))}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label="Quantity">
            <input className="a-input" value={qty} onChange={(e) => setQty(e.target.value)} />
          </Field>
          <Field label="Ordered in">
            <select
              className="a-input"
              value={chosenPack?.unitCode ?? ''}
              disabled={!item}
              onChange={(e) => setUnit(e.target.value)}
            >
              {(packs ?? []).map((pack) => (
                <option key={pack.id} value={pack.unitCode}>
                  {pack.label}
                </option>
              ))}
            </select>
          </Field>
          <Field label={`Price per ${chosenPack?.unitCode.toLowerCase() ?? 'unit'}`}>
            <input className="a-input" value={price} onChange={(e) => setPrice(e.target.value)} />
          </Field>
        </div>

        {/*
          The conversion, stated before it is committed. This line is where a
          purchase order for ten cartons quietly becomes ten pieces, so the
          form says what it is about to mean.
        */}
        {chosenPack && chosenPack.factor > 1 && Number(qty) > 0 ? (
          <p className="rounded-xl bg-subtle px-3 py-2 text-sm text-muted">
            {qty} × {chosenPack.unitCode} is{' '}
            <span className="font-bold text-ink">
              {Number(qty) * chosenPack.factor} into stock
            </span>
            {Number(price) > 0
              ? ` at ${formatIdr(Math.round((Number(price) / chosenPack.factor) * 100) / 100)} each`
              : ''}
            .
          </p>
        ) : null}
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !item || Number(qty) <= 0}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Add'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

/** Recording what actually turned up, and booking it into stock. */
function ReceiveSheet({ receiptId, onClose }: { receiptId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [line, setLine] = useState('');
  const [accepted, setAccepted] = useState('');
  const [rejected, setRejected] = useState('');

  const { data: receipt } = useQuery({
    queryKey: ['purchasing', 'receipt', receiptId],
    queryFn: () => api.admin.purchasing.receipts.get(receiptId),
  });
  const { data: order } = useQuery({
    queryKey: ['purchasing', 'order', receipt?.orderId],
    queryFn: () => api.admin.purchasing.orders.get(receipt!.orderId),
    enabled: !!receipt,
  });

  const add = useMutation({
    mutationFn: () =>
      api.admin.purchasing.receipts.addLine(receiptId, {
        orderItemId: line,
        qtyAccepted: Number(accepted || 0),
        qtyRejected: Number(rejected || 0),
      }),
    onSuccess: () => {
      setError(null);
      setLine('');
      setAccepted('');
      setRejected('');
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That line did not save.'),
  });

  const post = useMutation({
    mutationFn: () => api.admin.purchasing.receipts.post(receiptId),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
      void qc.invalidateQueries({ queryKey: ['inventory'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That delivery did not post.'),
  });

  return (
    <Modal title={receipt ? `${receipt.grnNumber} · ${receipt.supplierName}` : 'Delivery'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="rounded-lg bg-surface-raised px-3 py-2 text-xs text-muted">
          Accepted goods enter stock at the price on the order, which moves the item's average
          cost. Rejected goods are recorded and never enter it.
        </p>

        {receipt?.status === 'DRAFT' ? (
          <div className="grid gap-2">
            <SearchSelect
              value={line}
              onChange={setLine}
              placeholder="Search an ordered line…"
              options={(order?.items ?? [])
                .filter((i) => i.qtyOutstanding > 0)
                .map((i) => ({
                  value: i.id,
                  label: i.itemName || i.description,
                  hint: `${i.qtyOutstanding} still outstanding`,
                }))}
            />
            <div className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
              <input
                className="a-input"
                placeholder="Accepted"
                value={accepted}
                onChange={(e) => setAccepted(e.target.value)}
              />
              <input
                className="a-input"
                placeholder="Rejected"
                value={rejected}
                onChange={(e) => setRejected(e.target.value)}
              />
              <button
                className="a-btn"
                disabled={add.isPending || !line || (!accepted && !rejected)}
                onClick={() => add.mutate()}
              >
                Add
              </button>
            </div>
          </div>
        ) : null}

        <div className="a-card !p-0 max-h-64 overflow-y-auto">
          <table className="a-table">
            <thead>
              <tr>
                <th>Item</th>
                <th className="text-right">Accepted</th>
                <th className="text-right">Rejected</th>
                <th>QC</th>
              </tr>
            </thead>
            <tbody>
              {(receipt?.items ?? []).map((item) => (
                <tr key={item.id}>
                  <td className="text-sm font-bold">{item.itemName}</td>
                  <td className="text-right tabular-nums">{item.qtyAccepted}</td>
                  <td className="text-right tabular-nums text-sm text-danger">
                    {item.qtyRejected > 0 ? item.qtyRejected : '—'}
                  </td>
                  <td>
                    <StatusBadge status={item.qcStatus} />
                  </td>
                </tr>
              ))}
              {(receipt?.items ?? []).length === 0 ? (
                <tr>
                  <td colSpan={4} className="py-6 text-center text-sm text-muted">
                    Nothing recorded yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Close
          </button>
          {receipt?.status === 'DRAFT' ? (
            <button
              className="a-btn"
              disabled={post.isPending || (receipt?.items ?? []).length === 0}
              onClick={() => post.mutate()}
            >
              {post.isPending ? 'Posting…' : 'Post into stock'}
            </button>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}
