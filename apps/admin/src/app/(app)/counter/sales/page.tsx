'use client';

import { formatDayTime, formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Ban, Eye } from 'lucide-react';
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
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

const PAGE_SIZE = 25;

export default function SalesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branch, setBranch] = useState('');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(0);
  const [opened, setOpened] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: overview } = useQuery({
    queryKey: ['pos', 'overview', branch],
    queryFn: () => api.admin.pos.overview(branch || undefined),
  });
  const { data: orders, isLoading, error: listError } = useQuery({
    queryKey: ['pos', 'orders', branch, status],
    queryFn: () =>
      api.admin.pos.orders.list({ branchId: branch || undefined, status: status || undefined }),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const voidSale = useMutation({
    mutationFn: (id: string) => {
      const reason = prompt('Why is this sale being voided?') ?? '';
      if (!reason.trim()) throw new ApiError(422, 'VALIDATION_FAILED', 'Voiding needs a reason.');
      return api.admin.pos.orders.void(id, reason);
    },
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['pos'] });
      void qc.invalidateQueries({ queryKey: ['inventory'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That sale did not void.'),
  });

  const rows = orders ?? [];
  const shown = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));

  return (
    <div>
      <PageTitle
        title="Counter sales"
        subtitle="What the till took, and what it made on it"
        actions={
          <>
            <Link href="/counter" className="a-btn-ghost">
              Till
            </Link>
            <Link href="/counter/products" className="a-btn-ghost">
              Menu
            </Link>
          </>
        }
      />
      <ErrorNote message={error} />
      <QueryError error={listError} />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Today"
          value={overview ? formatIdr(overview.today.salesIdr) : '—'}
          hint={overview ? `${overview.today.orders} sales` : undefined}
          tone="brand"
        />
        <StatCard tone="ok"
          label="This month"
          value={overview ? formatIdr(overview.month.salesIdr) : '—'}
          hint={overview ? `${overview.month.orders} sales` : undefined}
        />
        <StatCard tone="brand"
          label="Gross profit this month"
          value={overview ? formatIdr(overview.month.grossProfitIdr) : '—'}
          hint={
            overview && overview.month.salesIdr > 0
              ? `${Math.round((overview.month.grossProfitIdr / overview.month.salesIdr) * 100)}% margin`
              : undefined
          }
        />
        <StatCard
          label="Voided this month"
          value={overview?.month.voidedOrders ?? '—'}
          tone={overview && overview.month.voidedOrders > 0 ? 'danger' : undefined}
        />
      </div>

      {overview && overview.topProducts.length > 0 ? (
        <div className="a-card mb-4">
          <p className="mb-2 text-[11px] font-bold uppercase tracking-wider text-muted">
            Best sellers this month
          </p>
          <div className="flex flex-wrap gap-2">
            {overview.topProducts.slice(0, 6).map((p) => (
              <span key={p.productId} className="rounded-xl border border-line px-3 py-1.5 text-sm">
                <span className="font-bold">{p.productName}</span>
                <span className="ml-2 text-muted">
                  {p.qty} · {formatIdr(p.salesIdr)}
                </span>
              </span>
            ))}
          </div>
        </div>
      ) : null}

      <div className="mb-4 flex flex-wrap gap-2">
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
        <div className="w-40">
          <SearchSelect
            value={status}
            onChange={(v) => {
              setStatus(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="Any status"
            placeholder="Search status…"
            options={['OPEN', 'COMPLETED', 'CANCELLED', 'VOIDED'].map((s) => ({
              value: s,
              label: s[0]! + s.slice(1).toLowerCase(),
            }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading sales…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Sale</th>
                  <th>When</th>
                  <th>Cashier</th>
                  <th className="text-right">Total</th>
                  <th className="text-right">Gross</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((order) => (
                  <tr key={order.id}>
                    <td>
                      <button className="font-bold hover:text-brand" onClick={() => setOpened(order.id)}>
                        {order.orderNumber}
                      </button>
                      {order.xpEarned > 0 ? (
                        <p className="text-xs text-muted">{order.xpEarned} points earned</p>
                      ) : null}
                    </td>
                    <td className="text-xs text-muted">{formatDayTime(order.openedAt)}</td>
                    <td className="text-sm">{order.cashierName}</td>
                    <td className="text-right tabular-nums">{formatIdr(order.totalIdr)}</td>
                    <td className="text-right tabular-nums text-sm">
                      {order.status === 'COMPLETED' ? formatIdr(order.grossProfitIdr) : '—'}
                    </td>
                    <td>
                      <StatusBadge status={order.status} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          { label: 'Open', icon: Eye, onClick: () => setOpened(order.id) },
                          {
                            label: 'Void',
                            icon: Ban,
                            tone: 'danger' as const,
                            disabled: !can('pos.void') || order.status !== 'COMPLETED',
                            onClick: () => voidSale.mutate(order.id),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      No sales match those filters.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}

      {opened ? <ReceiptSheet orderId={opened} onClose={() => setOpened(null)} /> : null}
    </div>
  );
}

function ReceiptSheet({ orderId, onClose }: { orderId: string; onClose: () => void }) {
  const { data: order } = useQuery({
    queryKey: ['pos', 'order', orderId],
    queryFn: () => api.admin.pos.orders.get(orderId),
  });

  return (
    <Modal title={order?.orderNumber ?? 'Sale'} onClose={onClose}>
      {!order ? (
        <Spinner label="Loading…" />
      ) : (
        <div className="grid gap-3">
          <p className="text-sm text-muted">
            {formatDayTime(order.openedAt)} · {order.cashierName}
            {order.memberName ? ` · ${order.memberName}` : ''}
          </p>
          <div className="a-card !p-0">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Item</th>
                  <th className="text-right">Qty</th>
                  <th className="text-right">Total</th>
                </tr>
              </thead>
              <tbody>
                {order.items.map((line) => (
                  <tr key={line.id}>
                    <td className="text-sm font-bold">{line.productName}</td>
                    <td className="text-right tabular-nums text-sm">{line.qty}</td>
                    <td className="text-right tabular-nums text-sm">{formatIdr(line.lineTotalIdr)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="grid gap-1 text-sm">
            <div className="flex justify-between">
              <span className="text-muted">Subtotal</span>
              <span className="tabular-nums">{formatIdr(order.subtotalIdr)}</span>
            </div>
            {order.tierDiscountIdr > 0 ? (
              <div className="flex justify-between">
                <span className="text-muted">Member discount</span>
                <span className="tabular-nums">− {formatIdr(order.tierDiscountIdr)}</span>
              </div>
            ) : null}
            <div className="flex justify-between border-t border-line pt-1 font-black">
              <span>Total</span>
              <span className="tabular-nums">{formatIdr(order.totalIdr)}</span>
            </div>
            {order.payments.map((payment) => (
              <div key={payment.id} className="flex justify-between text-muted">
                <span>{payment.method.replaceAll('_', ' ').toLowerCase()}</span>
                <span className="tabular-nums">{formatIdr(payment.amountIdr)}</span>
              </div>
            ))}
            {order.changeIdr > 0 ? (
              <div className="flex justify-between">
                <span className="text-muted">Change</span>
                <span className="tabular-nums">{formatIdr(order.changeIdr)}</span>
              </div>
            ) : null}
          </div>
          {order.voidReason ? <ErrorNote message={`Voided: ${order.voidReason}`} /> : null}
          <div className="mt-2 flex justify-end">
            <button className="a-btn-ghost" onClick={onClose}>
              Close
            </button>
          </div>
        </div>
      )}
    </Modal>
  );
}
