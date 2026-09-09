'use client';

import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eye } from 'lucide-react';
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
  StatCard, StatRow,
} from '../../../components/ui';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';

const PAGE_SIZE = 20;

export default function OrdersPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branch, setBranch] = useState('');
  const [supplier, setSupplier] = useState('');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(0);
  const [creating, setCreating] = useState(false);

  const { data: overview } = useQuery({
    queryKey: ['purchasing', 'overview', branch],
    queryFn: () => api.admin.purchasing.overview(branch || undefined),
  });
  const { data: orders, isLoading, error } = useQuery({
    queryKey: ['purchasing', 'orders', branch, supplier, status],
    queryFn: () =>
      api.admin.purchasing.orders.list({
        branchId: branch || undefined,
        supplierId: supplier || undefined,
        status: status || undefined,
      }),
  });
  const { data: suppliers } = useQuery({
    queryKey: ['purchasing', 'suppliers'],
    queryFn: () => api.admin.purchasing.suppliers.list(),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const rows = orders ?? [];
  const shown = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const supplierNames = new Map((suppliers ?? []).map((s) => [s.id, s.name]));

  return (
    <div>
      <PageTitle
        title="Purchase orders"
        subtitle="What the studio has committed to buy"
        actions={
          <>
            <Link href="/purchasing/requests" className="a-btn-ghost">
              Requests
            </Link>
            <Link href="/purchasing/suppliers" className="a-btn-ghost">
              Suppliers
            </Link>
            {can('purchasing.manage') ? (
              <button className="a-btn" onClick={() => setCreating(true)}>
                + New order
              </button>
            ) : null}
          </>
        }
      />
      <QueryError error={error} />

      <StatRow>
        <StatCard
          tone="ink"
          label="Awaiting a signature"
          value={overview?.pendingRequests ?? '—'}
          hint="Purchase requests in the chain"
        />
        <StatCard label="Open orders" value={overview?.openOrders ?? '—'} tone="brand" />
        <StatCard
          tone="warn"
          label="Committed"
          value={overview ? formatIdr(overview.committedIdr) : '—'}
          hint="Approved but not yet delivered"
        />
        <StatCard
          tone="ok"
          label="Received this month"
          value={overview ? formatIdr(overview.receivedMonthIdr) : '—'}
        />
      </StatRow>

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
        <div className="w-52">
          <SearchSelect
            value={supplier}
            onChange={(v) => {
              setSupplier(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All suppliers"
            placeholder="Search supplier…"
            options={(suppliers ?? []).map((s) => ({ value: s.id, label: s.name, hint: s.code }))}
          />
        </div>
        <div className="w-48">
          <SearchSelect
            value={status}
            onChange={(v) => {
              setStatus(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="Any status"
            placeholder="Search status…"
            options={['DRAFT', 'APPROVED', 'SENT', 'PARTIALLY_RECEIVED', 'RECEIVED', 'CANCELLED'].map(
              (s) => ({ value: s, label: s.replaceAll('_', ' ').toLowerCase() }),
            )}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading orders…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Order</th>
                  <th>Supplier</th>
                  <th>Ordered</th>
                  <th>Expected</th>
                  <th className="text-right">Total</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((order) => (
                  <tr key={order.id}>
                    <td>
                      <Link href={`/purchasing/orders/${order.id}`} className="font-bold hover:text-brand">
                        {order.poNumber}
                      </Link>
                    </td>
                    <td className="text-sm">{supplierNames.get(order.supplierId) ?? order.supplierId}</td>
                    <td className="tabular-nums text-sm">{order.orderedOn}</td>
                    <td className="tabular-nums text-sm">
                      {order.expectedOn ?? <span className="text-muted">—</span>}
                    </td>
                    <td className="text-right tabular-nums">{formatIdr(order.totalIdr)}</td>
                    <td>
                      <StatusBadge status={order.status} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Open',
                            icon: Eye,
                            onClick: () => {
                              location.href = `/admin/purchasing/orders/${order.id}`;
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
                      No orders match those filters.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}

      {creating ? (
        <NewOrderModal
          suppliers={(suppliers ?? []).map((s) => ({ id: s.id, name: s.name, status: s.status }))}
          branches={(branches ?? []).map((b) => ({ id: b.id, name: b.name }))}
          onClose={() => setCreating(false)}
          onDone={(id) => {
            setCreating(false);
            void qc.invalidateQueries({ queryKey: ['purchasing'] });
            location.href = `/admin/purchasing/orders/${id}`;
          }}
        />
      ) : null}
    </div>
  );
}

function NewOrderModal({
  suppliers,
  branches,
  onClose,
  onDone,
}: {
  suppliers: { id: string; name: string; status: string }[];
  branches: { id: string; name: string }[];
  onClose: () => void;
  onDone: (id: string) => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [supplier, setSupplier] = useState('');
  const [branch, setBranch] = useState(branches[0]?.id ?? '');
  const [expected, setExpected] = useState('');

  const save = useMutation({
    mutationFn: () =>
      api.admin.purchasing.orders.create({
        supplierId: supplier,
        branchId: branch,
        expectedOn: expected || null,
        taxPercent: 11,
      }),
    onSuccess: (order) => onDone(order.id),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That order did not open.'),
  });

  return (
    <Modal title="New purchase order" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Supplier" hint="Blocked suppliers cannot be ordered from.">
          <SearchSelect
            value={supplier}
            onChange={setSupplier}
            placeholder="Search supplier…"
            options={suppliers
              .filter((s) => s.status === 'ACTIVE' || s.status === 'PROBATION')
              .map((s) => ({ value: s.id, label: s.name }))}
          />
        </Field>
        <Field label="Deliver to">
          <SearchSelect
            value={branch}
            onChange={setBranch}
            placeholder="Search branch…"
            options={branches.map((b) => ({ value: b.id, label: b.name }))}
          />
        </Field>
        <Field label="Expected on">
          <input
            type="date"
            className="a-input"
            value={expected}
            onChange={(e) => setExpected(e.target.value)}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !supplier || !branch}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Opening…' : 'Open'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
