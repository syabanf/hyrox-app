'use client';

import type { Voucher, VoucherStatus } from '@hyrox/domain';
import { Spinner, StatusBadge, formatDay, formatIdr } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle, Pager, SearchSelect, StatCard } from '../../../../components/ui';

const NEXT_ACTIONS: Partial<Record<VoucherStatus, VoucherStatus[]>> = {
  DRAFT: ['ACTIVE', 'SCHEDULED', 'DISABLED'],
  SCHEDULED: ['ACTIVE', 'DISABLED'],
  ACTIVE: ['DISABLED', 'EXPIRED'],
  DISABLED: ['ACTIVE'],
};

export default function VouchersPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [createOpen, setCreateOpen] = useState(false);
  const [statusView, setStatusView] = useState('');
  const [page, setPage] = useState(0);
  const [editTarget, setEditTarget] = useState<Voucher | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { data, isLoading } = useQuery({ queryKey: ['admin-vouchers'], queryFn: api.admin.vouchers.list });

  const setStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: VoucherStatus }) =>
      api.admin.vouchers.setStatus(id, status),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Status change failed.'),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.vouchers.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  return (
    <div>
      <PageTitle
        title="Vouchers"
        subtitle="Redemptions are separate records linked to payments"
        actions={
          can('vouchers.manage') ? (
            <button className="a-btn" onClick={() => setCreateOpen(true)}>
              + New voucher
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Live now" value={(data ?? []).filter((v) => v.voucher.status === 'ACTIVE').length} />
        <StatCard
          label="Draft / scheduled"
          value={(data ?? []).filter((v) => ['DRAFT', 'SCHEDULED'].includes(v.voucher.status)).length}
        />
        <StatCard label="Redemptions" value={(data ?? []).reduce((sum, v) => sum + v.redemptionCount, 0)} />
      </div>
      <div className="mb-4 w-44">
        <SearchSelect
          value={statusView}
          onChange={(v) => {
            setStatusView(v);
            setPage(0);
          }}
          allowEmpty
          emptyLabel="All statuses"
          placeholder="Search status…"
          options={['DRAFT', 'SCHEDULED', 'ACTIVE', 'EXPIRED', 'DISABLED'].map((s) => ({ value: s, label: s }))}
        />
      </div>
      {isLoading ? (
        <Spinner label="Loading vouchers…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Code</th>
                <th>Value</th>
                <th>Window</th>
                <th>Limits</th>
                <th className="text-right">Redeemed</th>
                <th>Status</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {(data ?? [])
                .filter((row) => !statusView || row.voucher.status === statusView)
                .slice(page * 8, page * 8 + 8)
                .map(({ voucher: v, redemptionCount }) => (
                <tr key={v.id}>
                  <td className="font-mono font-black text-brand">{v.code}</td>
                  <td>{v.type === 'PERCENT' ? `${v.value}%` : formatIdr(v.value)}</td>
                  <td className="text-muted">
                    {formatDay(v.startsAt)} → {formatDay(v.endsAt)}
                  </td>
                  <td className="text-muted">
                    {v.usageLimit ?? '∞'} total · {v.perMemberLimit ?? '∞'}/member
                    {v.eligibleSegment === 'NEW_MEMBERS' ? (
                      <span className="block text-xs text-info">New members only</span>
                    ) : null}
                  </td>
                  <td className="text-right font-bold">{redemptionCount}</td>
                  <td>
                    <StatusBadge status={v.status} />
                  </td>
                  <td className="text-right">
                    {can('vouchers.manage') ? (
                      <div className="flex justify-end gap-2">
                        <button
                          className="text-xs font-bold text-brand"
                          onClick={() => setEditTarget(v)}
                        >
                          Edit
                        </button>
                        {(NEXT_ACTIONS[v.status] ?? []).map((s) => (
                          <button
                            key={s}
                            className="text-xs font-bold text-muted hover:text-brand"
                            onClick={() => setStatus.mutate({ id: v.id, status: s })}
                          >
                            → {s}
                          </button>
                        ))}
                        <button
                          className="text-xs font-bold text-muted hover:text-danger"
                          onClick={() => {
                            if (confirm(`Delete voucher ${v.code}? Only possible while unredeemed.`))
                              remove.mutate(v.id);
                          }}
                        >
                          Delete
                        </button>
                      </div>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pager
            page={page}
            pageCount={Math.max(1, Math.ceil((data ?? []).filter((row) => !statusView || row.voucher.status === statusView).length / 8))}
            onPage={setPage}
          />
        </div>
      )}
      {createOpen ? (
        <VoucherModal
          onClose={() => setCreateOpen(false)}
          onDone={() => {
            setCreateOpen(false);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
      {editTarget ? (
        <VoucherModal
          voucher={editTarget}
          onClose={() => setEditTarget(null)}
          onDone={() => {
            setEditTarget(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}

function VoucherModal({
  voucher,
  onClose,
  onDone,
}: {
  voucher?: Voucher;
  onClose: () => void;
  onDone: () => void;
}) {
  const [code, setCode] = useState(voucher?.code ?? '');
  const [type, setType] = useState<'FIXED_IDR' | 'PERCENT'>(voucher?.type ?? 'PERCENT');
  const [value, setValue] = useState(String(voucher?.value ?? 10));
  const [startsAt, setStartsAt] = useState(voucher ? voucher.startsAt.slice(0, 10) : '');
  const [endsAt, setEndsAt] = useState(voucher ? voucher.endsAt.slice(0, 10) : '');
  const [segment, setSegment] = useState<'ALL' | 'NEW_MEMBERS'>(voucher?.eligibleSegment ?? 'ALL');
  const [usageLimit, setUsageLimit] = useState(voucher?.usageLimit ? String(voucher.usageLimit) : '');
  const [perMemberLimit, setPerMemberLimit] = useState(
    voucher?.perMemberLimit ? String(voucher.perMemberLimit) : '1',
  );
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const body = {
        code,
        type,
        value: Number(value),
        startsAt: new Date(startsAt).toISOString(),
        endsAt: new Date(endsAt).toISOString(),
        usageLimit: usageLimit ? Number(usageLimit) : null,
        perMemberLimit: perMemberLimit ? Number(perMemberLimit) : null,
        eligibleSegment: segment,
        applicablePackageIds: null,
      };
      return voucher ? api.admin.vouchers.update(voucher.id, body) : api.admin.vouchers.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={voucher ? `Edit ${voucher.code}` : 'New voucher (created as DRAFT)'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Code</label>
            <input className="a-input uppercase" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} />
          </div>
          <div>
            <label className="a-label">Segment</label>
            <select className="a-input" value={segment} onChange={(e) => setSegment(e.target.value as typeof segment)}>
              <option value="ALL">All members</option>
              <option value="NEW_MEMBERS">New members</option>
            </select>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Type</label>
            <select className="a-input" value={type} onChange={(e) => setType(e.target.value as typeof type)}>
              <option value="PERCENT">Percent discount</option>
              <option value="FIXED_IDR">Fixed IDR discount</option>
            </select>
          </div>
          <div>
            <label className="a-label">{type === 'PERCENT' ? 'Percent' : 'Amount (IDR)'}</label>
            <input className="a-input" value={value} onChange={(e) => setValue(e.target.value)} />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Starts</label>
            <input type="date" className="a-input" value={startsAt} onChange={(e) => setStartsAt(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Ends</label>
            <input type="date" className="a-input" value={endsAt} onChange={(e) => setEndsAt(e.target.value)} />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Usage limit (blank = unlimited)</label>
            <input className="a-input" value={usageLimit} onChange={(e) => setUsageLimit(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Per-member limit</label>
            <input className="a-input" value={perMemberLimit} onChange={(e) => setPerMemberLimit(e.target.value)} />
          </div>
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || code.length < 3 || !startsAt || !endsAt}
          onClick={() => mutation.mutate()}
        >
          {voucher ? 'Save changes' : 'Create voucher'}
        </button>
      </div>
    </Modal>
  );
}
