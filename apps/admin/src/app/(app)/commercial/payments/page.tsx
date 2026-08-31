'use client';

import { Spinner, StatusBadge, formatDayTime, formatIdr } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle, Pager, SearchSelect, StatCard } from '../../../../components/ui';

export default function PaymentsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [statusFilter, setStatusFilter] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const [refundTarget, setRefundTarget] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data, isLoading } = useQuery({ queryKey: ['admin-payments'], queryFn: api.admin.payments.list });

  const simulate = useMutation({
    mutationFn: (id: string) => api.admin.payments.simulate(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Simulation failed.'),
  });

  const q = query.trim().toLowerCase();
  const rows = (data ?? [])
    .filter((p) => !statusFilter || p.payment.status === statusFilter)
    .filter(
      (p) =>
        !q ||
        p.memberName.toLowerCase().includes(q) ||
        p.packageName.toLowerCase().includes(q) ||
        p.payment.id.toLowerCase().includes(q),
    );
  const pageCount = Math.max(1, Math.ceil(rows.length / 10));
  const safePage = Math.min(page, pageCount - 1);
  const paged = rows.slice(safePage * 10, safePage * 10 + 10);
  const paidTotal = (data ?? [])
    .filter((p) => p.payment.status === 'PAID')
    .reduce((sum, p) => sum + p.payment.totalIdr, 0);

  return (
    <div>
      <PageTitle
        title="Payments"
        subtitle="Top-ups via mock Xendit — PAYMENT ≠ CREDIT LEDGER; a paid payment produces the TOP_UP entry"
      />
      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Collected" value={formatIdr(paidTotal)} hint="Paid payments" />
        <StatCard label="Pending" value={(data ?? []).filter((p) => p.payment.status === 'PENDING').length} />
        <StatCard label="Refunded" value={(data ?? []).filter((p) => p.payment.status === 'REFUNDED').length} />
        <StatCard
          label="Failed / expired"
          value={(data ?? []).filter((p) => ['FAILED', 'EXPIRED'].includes(p.payment.status)).length}
        />
      </div>
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search member, package, id…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-44">
        <SearchSelect
          value={statusFilter}
          onChange={(v) => {
            setStatusFilter(v);
            setPage(0);
          }}
          allowEmpty
          emptyLabel="All statuses"
          placeholder="Search status…"
          options={['PENDING', 'PAID', 'FAILED', 'EXPIRED', 'REFUNDED'].map((s) => ({ value: s, label: s }))}
        />
        </div>
      </div>
      <ErrorNote message={error} />
      {isLoading ? (
        <Spinner label="Loading payments…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Payment</th>
                <th>Member</th>
                <th>Package</th>
                <th className="text-right">Total</th>
                <th>Channel</th>
                <th>Status</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {paged.map((p) => (
                <tr key={p.payment.id}>
                  <td className="whitespace-nowrap text-muted">{formatDayTime(p.payment.createdAt)}</td>
                  <td className="font-mono text-xs">{p.payment.id}</td>
                  <td className="font-bold">{p.memberName}</td>
                  <td>
                    {p.packageName}
                    {p.payment.voucherCode ? (
                      <span className="block text-xs text-ok">
                        {p.payment.voucherCode} (−{formatIdr(p.payment.discountIdr)})
                      </span>
                    ) : null}
                  </td>
                  <td className="text-right font-bold">{formatIdr(p.payment.totalIdr)}</td>
                  <td className="text-muted">{p.payment.channel}</td>
                  <td>
                    <StatusBadge status={p.payment.status} />
                  </td>
                  <td className="text-right">
                    <div className="flex justify-end gap-2">
                      {can('payments.simulate') && p.payment.status === 'PENDING' ? (
                        <button
                          className="a-btn !px-2.5 !py-1 text-xs"
                          onClick={() => simulate.mutate(p.payment.id)}
                        >
                          Simulate webhook: PAID
                        </button>
                      ) : null}
                      {can('refunds.manage') && p.payment.status === 'PAID' ? (
                        <button
                          className="a-btn-danger !px-2.5 !py-1 text-xs"
                          onClick={() => setRefundTarget(p.payment.id)}
                        >
                          Refund
                        </button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pager page={safePage} pageCount={pageCount} onPage={setPage} />
        </div>
      )}
      {refundTarget ? (
        <RefundModal
          paymentId={refundTarget}
          onClose={() => setRefundTarget(null)}
          onDone={() => {
            setRefundTarget(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}

function RefundModal({ paymentId, onClose, onDone }: { paymentId: string; onClose: () => void; onDone: () => void }) {
  const [reason, setReason] = useState('');
  const [error, setError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => api.admin.payments.refund(paymentId, reason),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Refund failed.'),
  });
  return (
    <Modal title="Refund payment" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <p className="text-sm text-muted">
          Marks the payment REFUNDED and reverses its TOP_UP ledger entry (credits are taken back).
          Audited.
        </p>
        <div>
          <label className="a-label">Reason (required)</label>
          <input className="a-input" value={reason} onChange={(e) => setReason(e.target.value)} />
        </div>
        <ErrorNote message={error} />
        <button className="a-btn-danger" disabled={mutation.isPending || reason.length < 3} onClick={() => mutation.mutate()}>
          Refund payment
        </button>
      </div>
    </Modal>
  );
}
