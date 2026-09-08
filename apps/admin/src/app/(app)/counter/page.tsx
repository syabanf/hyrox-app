'use client';

import type { POSOrderView } from '@nuhabit/contracts';
import { POS_PAYMENT_METHODS } from '@nuhabit/domain';
import { formatIdr, Spinner } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Trash2 } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  SearchSelect,
} from '../../../components/ui';
import { api, ApiError } from '../../../lib/api';
import { useAdminAuth, usePermissions } from '../../../lib/auth';

/**
 * The till.
 *
 * One screen: pick a branch, open a shift, scan products onto a sale, take the
 * money, complete it. Completing is where the stock moves and the points land,
 * so the button says so.
 */
export default function TillPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const user = useAdminAuth((s) => s.user);
  const [branch, setBranch] = useState('');
  const [orderId, setOrderId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [openingTill, setOpeningTill] = useState(false);
  const [tendering, setTendering] = useState(false);

  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const effectiveBranch = branch || user?.branchId || branches?.[0]?.id || '';

  const { data: shifts, error: shiftsError } = useQuery({
    queryKey: ['pos', 'shifts', effectiveBranch],
    queryFn: () =>
      api.admin.pos.shifts.list({ branchId: effectiveBranch, status: 'OPEN', limit: 20 }),
    enabled: !!effectiveBranch,
  });
  const myShift = (shifts ?? []).find((s) => s.cashierId === user?.id);

  const { data: products } = useQuery({
    queryKey: ['pos', 'products', effectiveBranch],
    queryFn: () =>
      api.admin.pos.products.list({ sellableOnly: 'true', branchId: effectiveBranch }),
    enabled: !!effectiveBranch,
  });
  const { data: members } = useQuery({
    queryKey: ['members', 'all'],
    queryFn: () => api.admin.members.list(),
  });
  const { data: order } = useQuery({
    queryKey: ['pos', 'order', orderId],
    queryFn: () => api.admin.pos.orders.get(orderId!),
    enabled: !!orderId,
  });

  const openOrder = useMutation({
    mutationFn: () => api.admin.pos.orders.open({ branchId: effectiveBranch }),
    onSuccess: (created) => {
      setError(null);
      setOrderId(created.id);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That sale did not open.'),
  });

  const addLine = useMutation({
    mutationFn: (productId: string) =>
      api.admin.pos.orders.addLine(orderId!, { productId, qty: 1 }),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['pos', 'order', orderId] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That line did not add.'),
  });

  const removeLine = useMutation({
    mutationFn: (lineId: string) => api.admin.pos.orders.removeLine(orderId!, lineId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['pos', 'order', orderId] }),
  });

  const setMember = useMutation({
    mutationFn: (memberId: string | null) =>
      api.admin.pos.orders.setDetails(orderId!, {
        memberId,
        setMember: true,
        discountIdr: order?.discountIdr ?? 0,
      }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['pos', 'order', orderId] }),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That customer did not save.'),
  });

  const complete = useMutation({
    mutationFn: () => api.admin.pos.orders.complete(orderId!),
    onSuccess: (done) => {
      setError(null);
      setOrderId(null);
      void qc.invalidateQueries({ queryKey: ['pos'] });
      void qc.invalidateQueries({ queryKey: ['inventory'] });
      void qc.invalidateQueries({ queryKey: ['crm'] });
      alert(
        `${done.orderNumber} completed. ${formatIdr(done.totalIdr)}` +
          (done.changeIdr > 0 ? ` · change ${formatIdr(done.changeIdr)}` : '') +
          (done.xpEarned > 0 ? ` · ${done.xpEarned} points earned` : ''),
      );
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That sale did not complete.'),
  });

  const cancel = useMutation({
    mutationFn: () => api.admin.pos.orders.cancel(orderId!),
    onSuccess: () => {
      setOrderId(null);
      void qc.invalidateQueries({ queryKey: ['pos'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That sale did not cancel.'),
  });

  return (
    <div>
      <PageTitle
        title="Till"
        subtitle="Completing a sale moves the stock and awards the points in one act"
        actions={
          <>
            <Link href="/counter/sales" className="a-btn-ghost">
              Sales
            </Link>
            <Link href="/counter/shifts" className="a-btn-ghost">
              Shifts
            </Link>
          </>
        }
      />
      <ErrorNote message={error} />
      <QueryError error={shiftsError} />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <div className="w-44">
          <SearchSelect
            value={effectiveBranch}
            onChange={(v) => {
              setBranch(v);
              setOrderId(null);
            }}
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
        {myShift ? (
          <span className="rounded-xl bg-brand/15 px-3 py-1.5 text-sm font-bold">
            {myShift.shiftNumber} open · float {formatIdr(myShift.openingCashIdr)}
          </span>
        ) : can('pos.sell') ? (
          <button className="a-btn" onClick={() => setOpeningTill(true)}>
            Open a till
          </button>
        ) : null}
      </div>

      {!myShift ? (
        <div className="a-card text-center">
          <p className="text-sm text-muted">
            A sale needs a till behind it, or it has nowhere to be counted at the end of the day.
          </p>
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
          <div className="a-card">
            <h2 className="mb-3 font-black">What is for sale</h2>
            <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
              {(products ?? []).map((product) => (
                <button
                  key={product.id}
                  disabled={!orderId || addLine.isPending}
                  onClick={() => addLine.mutate(product.id)}
                  className="rounded-xl border border-line p-3 text-left transition hover:border-brand disabled:opacity-40"
                >
                  <p className="font-bold">{product.name}</p>
                  <p className="text-sm text-brand">{formatIdr(product.priceIdr)}</p>
                  <p className="text-xs text-muted">
                    {product.onHand == null ? 'service' : `${product.onHand} in stock`}
                  </p>
                </button>
              ))}
              {(products ?? []).length === 0 ? (
                <p className="text-sm text-muted">Nothing is on the menu yet.</p>
              ) : null}
            </div>
          </div>

          <div className="a-card">
            {!orderId ? (
              <div className="text-center">
                <p className="mb-3 text-sm text-muted">No sale open.</p>
                <button className="a-btn" onClick={() => openOrder.mutate()}>
                  Start a sale
                </button>
              </div>
            ) : !order ? (
              <Spinner label="Loading…" />
            ) : (
              <div className="grid gap-3">
                <div className="flex items-center justify-between">
                  <p className="font-black">{order.orderNumber}</p>
                  <button className="text-xs font-bold text-muted hover:text-danger" onClick={() => cancel.mutate()}>
                    Cancel
                  </button>
                </div>

                <SearchSelect
                  value={order.memberId ?? ''}
                  onChange={(v) => setMember.mutate(v || null)}
                  allowEmpty
                  emptyLabel="No member"
                  placeholder="Search member…"
                  options={(members ?? []).map((m) => ({
                    value: m.member.id,
                    label: m.member.fullName,
                    hint: `${m.balance} credits`,
                  }))}
                />

                <div className="max-h-64 overflow-y-auto">
                  {order.items.map((line) => (
                    <div key={line.id} className="flex items-center justify-between border-b border-line py-2">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-bold">{line.productName}</p>
                        <p className="text-xs text-muted">
                          {line.qty} × {formatIdr(line.unitPriceIdr)}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="tabular-nums text-sm">{formatIdr(line.lineTotalIdr)}</span>
                        <button
                          className="text-muted hover:text-danger"
                          onClick={() => removeLine.mutate(line.id)}
                        >
                          <Trash2 size={14} />
                        </button>
                      </div>
                    </div>
                  ))}
                  {order.items.length === 0 ? (
                    <p className="py-4 text-center text-sm text-muted">Nothing on the sale yet.</p>
                  ) : null}
                </div>

                <div className="grid gap-1 border-t border-line pt-2 text-sm">
                  <Row label="Subtotal" value={formatIdr(order.subtotalIdr)} />
                  {order.tierDiscountIdr > 0 ? (
                    <Row label="Member discount" value={`− ${formatIdr(order.tierDiscountIdr)}`} />
                  ) : null}
                  {order.discountIdr > 0 ? (
                    <Row label="Discount" value={`− ${formatIdr(order.discountIdr)}`} />
                  ) : null}
                  {order.taxIdr > 0 ? <Row label="Tax" value={formatIdr(order.taxIdr)} /> : null}
                  <div className="mt-1 flex items-center justify-between border-t border-line pt-2">
                    <span className="font-black">Total</span>
                    <span className="display text-2xl font-black">{formatIdr(order.totalIdr)}</span>
                  </div>
                  {order.paidIdr > 0 ? (
                    <>
                      <Row label="Paid" value={formatIdr(order.paidIdr)} />
                      {order.dueIdr > 0 ? (
                        <Row label="Still owed" value={formatIdr(order.dueIdr)} />
                      ) : order.changeIdr > 0 ? (
                        <Row label="Change" value={formatIdr(order.changeIdr)} />
                      ) : null}
                    </>
                  ) : null}
                </div>

                <div className="grid gap-2">
                  <button
                    className="a-btn-ghost"
                    disabled={order.items.length === 0}
                    onClick={() => setTendering(true)}
                  >
                    Take payment
                  </button>
                  <button
                    className="a-btn"
                    disabled={complete.isPending || order.paymentStatus !== 'PAID'}
                    onClick={() => complete.mutate()}
                  >
                    {complete.isPending ? 'Completing…' : 'Complete sale'}
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {openingTill ? (
        <OpenTillModal
          branchId={effectiveBranch}
          onClose={() => setOpeningTill(false)}
          onDone={() => {
            setOpeningTill(false);
            void qc.invalidateQueries({ queryKey: ['pos'] });
          }}
        />
      ) : null}
      {tendering && order ? (
        <TenderModal
          order={order}
          onClose={() => setTendering(false)}
          onDone={() => {
            setTendering(false);
            void qc.invalidateQueries({ queryKey: ['pos', 'order', orderId] });
          }}
        />
      ) : null}
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-muted">{label}</span>
      <span className="tabular-nums">{value}</span>
    </div>
  );
}

function OpenTillModal({
  branchId,
  onClose,
  onDone,
}: {
  branchId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [float, setFloat] = useState('500000');

  const save = useMutation({
    mutationFn: () => api.admin.pos.shifts.open({ branchId, openingCashIdr: Number(float) }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That till did not open.'),
  });

  return (
    <Modal title="Open a till" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Opening float" hint="What is in the drawer before the first sale.">
          <input className="a-input" value={float} onChange={(e) => setFloat(e.target.value)} />
        </Field>
        <p className="text-xs text-muted">
          One till per cashier per branch. Two under one name is how cash goes missing with nobody
          accountable for it.
        </p>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Opening…' : 'Open'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function TenderModal({
  order,
  onClose,
  onDone,
}: {
  order: POSOrderView;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [method, setMethod] = useState('CASH');
  const [amount, setAmount] = useState(String(order.dueIdr));

  const save = useMutation({
    mutationFn: () =>
      api.admin.pos.orders.tender(order.id, { method, amountIdr: Number(amount) }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That payment was not taken.'),
  });

  return (
    <Modal title="Take payment" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
          <span className="font-bold">{formatIdr(order.dueIdr)}</span> still owed on{' '}
          {order.orderNumber}.
        </p>
        <Field label="Method" hint="Only cash gives change; a card cannot overpay.">
          <SearchSelect
            value={method}
            onChange={setMethod}
            placeholder="Search method…"
            options={POS_PAYMENT_METHODS.map((m) => ({
              value: m,
              label: m.replaceAll('_', ' ').toLowerCase(),
            }))}
          />
        </Field>
        <Field label="Amount">
          <input className="a-input" value={amount} onChange={(e) => setAmount(e.target.value)} />
        </Field>
        {method === 'CASH' && Number(amount) > order.dueIdr ? (
          <p className="text-sm text-brand">
            Change: {formatIdr(Number(amount) - order.dueIdr)}
          </p>
        ) : null}
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || Number(amount) <= 0} onClick={() => save.mutate()}>
            {save.isPending ? 'Taking…' : 'Take payment'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
