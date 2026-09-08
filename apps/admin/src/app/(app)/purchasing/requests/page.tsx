'use client';

import { APPROVAL_ROLES } from '@nuhabit/domain';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, Eye, FileOutput, X } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { useAdminAuth, usePermissions } from '../../../../lib/auth';

export default function RequestsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const role = useAdminAuth((s) => s.user?.role);
  const [status, setStatus] = useState('');
  const [branch, setBranch] = useState('');
  const [creating, setCreating] = useState(false);
  const [openRequest, setOpenRequest] = useState<string | null>(null);
  const [converting, setConverting] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: requests, isLoading, error: listError } = useQuery({
    queryKey: ['purchasing', 'requests', status, branch],
    queryFn: () =>
      api.admin.purchasing.requests.list({
        status: status || undefined,
        branchId: branch || undefined,
      }),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const decide = useMutation({
    mutationFn: ({ id, approve }: { id: string; approve: boolean }) => {
      if (approve) return api.admin.purchasing.requests.approve(id);
      const reason = prompt('Why is this being turned down?') ?? '';
      if (!reason.trim()) throw new ApiError(422, 'VALIDATION_FAILED', 'A rejection needs a reason.');
      return api.admin.purchasing.requests.reject(id, reason);
    },
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That decision did not save.'),
  });

  const rows = requests ?? [];
  const pending = rows.filter((r) => r.status.startsWith('PENDING'));

  /** The panel greys out what the server would refuse anyway. */
  const canSignNow = (requestStatus: string) => {
    if (!role || !can('purchasing.approve')) return false;
    const level = requestStatus.replace('PENDING_', '') as keyof typeof APPROVAL_ROLES;
    return APPROVAL_ROLES[level]?.includes(role) ?? false;
  };

  return (
    <div>
      <PageTitle
        title="Purchase requests"
        subtitle="Every request needs a signature from somebody other than the person asking"
        actions={
          <>
            <Link href="/purchasing" className="a-btn-ghost">
              Orders
            </Link>
            {can('purchasing.manage') ? (
              <button className="a-btn" onClick={() => setCreating(true)}>
                + New request
              </button>
            ) : null}
          </>
        }
      />
      <ErrorNote message={error} />
      <QueryError error={listError} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink" label="Requests" value={rows.length} />
        <StatCard label="Awaiting a signature" value={pending.length} tone="brand" />
        <StatCard
          label="Waiting on you"
          value={pending.filter((r) => canSignNow(r.status)).length}
          tone={pending.some((r) => canSignNow(r.status)) ? 'danger' : undefined}
        />
        <StatCard tone="ok" label="Value requested" value={formatIdr(rows.reduce((s, r) => s + r.totalIdr, 0))} />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <div className="w-52">
          <SearchSelect
            value={status}
            onChange={setStatus}
            allowEmpty
            emptyLabel="Any status"
            placeholder="Search status…"
            options={[
              'DRAFT',
              'PENDING_HEAD',
              'PENDING_FINANCE',
              'PENDING_DIRECTOR',
              'APPROVED',
              'REJECTED',
              'CONVERTED',
            ].map((s) => ({ value: s, label: s.replaceAll('_', ' ').toLowerCase() }))}
          />
        </div>
        <div className="w-44">
          <SearchSelect
            value={branch}
            onChange={setBranch}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading requests…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Request</th>
                  <th>Requested by</th>
                  <th>Needed by</th>
                  <th className="text-right">Total</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((request) => (
                  <tr key={request.id}>
                    <td>
                      <button
                        className="font-bold hover:text-brand"
                        onClick={() => setOpenRequest(request.id)}
                      >
                        {request.prNumber}
                      </button>
                      <p className="text-xs text-muted">{request.priority.toLowerCase()}</p>
                    </td>
                    <td className="text-sm">{request.requesterName}</td>
                    <td className="tabular-nums text-sm">
                      {request.requiredOn ?? <span className="text-muted">—</span>}
                    </td>
                    <td className="text-right tabular-nums">{formatIdr(request.totalIdr)}</td>
                    <td>
                      <StatusBadge status={request.status} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          { label: 'Open', icon: Eye, onClick: () => setOpenRequest(request.id) },
                          {
                            label: 'Approve',
                            icon: Check,
                            disabled: !canSignNow(request.status),
                            onClick: () => decide.mutate({ id: request.id, approve: true }),
                          },
                          {
                            label: 'Turn down',
                            icon: X,
                            tone: 'danger' as const,
                            disabled: !can('purchasing.approve') || !request.status.startsWith('PENDING'),
                            onClick: () => decide.mutate({ id: request.id, approve: false }),
                          },
                          {
                            label: 'Make an order',
                            icon: FileOutput,
                            disabled: !can('purchasing.manage') || request.status !== 'APPROVED',
                            onClick: () => setConverting(request.id),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      No requests match those filters.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {creating ? (
        <NewRequestModal
          branches={(branches ?? []).map((b) => ({ id: b.id, name: b.name }))}
          onClose={() => setCreating(false)}
          onDone={(id) => {
            setCreating(false);
            setOpenRequest(id);
            void qc.invalidateQueries({ queryKey: ['purchasing'] });
          }}
        />
      ) : null}
      {openRequest ? (
        <RequestSheet
          requestId={openRequest}
          onClose={() => {
            setOpenRequest(null);
            void qc.invalidateQueries({ queryKey: ['purchasing'] });
          }}
        />
      ) : null}
      {converting ? (
        <ConvertModal
          requestId={converting}
          onClose={() => setConverting(null)}
          onDone={(orderId) => {
            setConverting(null);
            location.href = `/admin/purchasing/orders/${orderId}`;
          }}
        />
      ) : null}
    </div>
  );
}

function NewRequestModal({
  branches,
  onClose,
  onDone,
}: {
  branches: { id: string; name: string }[];
  onClose: () => void;
  onDone: (id: string) => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [branch, setBranch] = useState(branches[0]?.id ?? '');
  const [priority, setPriority] = useState('NORMAL');
  const [requiredOn, setRequiredOn] = useState('');
  const [note, setNote] = useState('');

  const save = useMutation({
    mutationFn: () =>
      api.admin.purchasing.requests.create({
        branchId: branch,
        priority,
        requiredOn: requiredOn || null,
        note: note || null,
      }),
    onSuccess: (request) => onDone(request.id),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That request did not open.'),
  });

  return (
    <Modal title="New purchase request" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Branch">
          <SearchSelect
            value={branch}
            onChange={setBranch}
            placeholder="Search branch…"
            options={branches.map((b) => ({ value: b.id, label: b.name }))}
          />
        </Field>
        <Field label="Priority">
          <SearchSelect
            value={priority}
            onChange={setPriority}
            placeholder="Search priority…"
            options={['LOW', 'NORMAL', 'HIGH', 'URGENT'].map((p) => ({
              value: p,
              label: p[0]! + p.slice(1).toLowerCase(),
            }))}
          />
        </Field>
        <Field label="Needed by">
          <input
            type="date"
            className="a-input"
            value={requiredOn}
            onChange={(e) => setRequiredOn(e.target.value)}
          />
        </Field>
        <Field label="Note">
          <input className="a-input" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !branch} onClick={() => save.mutate()}>
            {save.isPending ? 'Opening…' : 'Open'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

/** The request itself: its lines, and the chain of signatures it needs. */
function RequestSheet({ requestId, onClose }: { requestId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [description, setDescription] = useState('');
  const [item, setItem] = useState('');
  const [qty, setQty] = useState('');
  const [price, setPrice] = useState('');

  const { data: request } = useQuery({
    queryKey: ['purchasing', 'request', requestId],
    queryFn: () => api.admin.purchasing.requests.get(requestId),
  });
  const { data: items } = useQuery({
    queryKey: ['inventory', 'items', 'all'],
    queryFn: () => api.admin.inventory.items.list(),
  });

  const addLine = useMutation({
    mutationFn: () =>
      api.admin.purchasing.requests.addLine(requestId, {
        itemId: item || null,
        description: description || (items ?? []).find((i) => i.id === item)?.name || 'Item',
        qty: Number(qty),
        estimatedPriceIdr: Number(price || 0),
      }),
    onSuccess: () => {
      setError(null);
      setDescription('');
      setItem('');
      setQty('');
      setPrice('');
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That line did not save.'),
  });

  const submit = useMutation({
    mutationFn: () => api.admin.purchasing.requests.submit(requestId),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That request did not submit.'),
  });

  return (
    <Modal title={request ? request.prNumber : 'Request'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />

        {request ? (
          <div className="rounded-lg bg-surface-raised px-3 py-2">
            <p className="text-[11px] font-bold uppercase tracking-wider text-muted">
              Signatures this amount needs
            </p>
            <div className="mt-1.5 flex flex-wrap gap-2">
              {request.chain.map((step) => (
                <span
                  key={step.level}
                  className={`rounded-lg px-2.5 py-1 text-xs font-bold ${
                    step.signed
                      ? 'bg-brand/20 text-ink'
                      : step.level === request.awaitingLevel
                        ? 'bg-danger/10 text-danger'
                        : 'bg-line/60 text-muted'
                  }`}
                >
                  {step.level.toLowerCase()}
                  {step.signed ? ' ✓' : step.level === request.awaitingLevel ? ' · waiting' : ''}
                </span>
              ))}
              {request.chain.length === 0 ? (
                <span className="text-xs text-muted">Add a line to see the chain.</span>
              ) : null}
            </div>
          </div>
        ) : null}

        {request?.status === 'DRAFT' ? (
          <div className="grid gap-2">
            <SearchSelect
              value={item}
              onChange={setItem}
              allowEmpty
              emptyLabel="Not in the catalogue"
              placeholder="Search item…"
              options={(items ?? []).map((i) => ({ value: i.id, label: i.name, hint: i.sku }))}
            />
            <input
              className="a-input"
              placeholder="What is being asked for"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
            <div className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
              <input className="a-input" placeholder="Qty" value={qty} onChange={(e) => setQty(e.target.value)} />
              <input
                className="a-input"
                placeholder="Est. price"
                value={price}
                onChange={(e) => setPrice(e.target.value)}
              />
              <button
                className="a-btn"
                disabled={addLine.isPending || Number(qty) <= 0 || (!item && !description)}
                onClick={() => addLine.mutate()}
              >
                Add
              </button>
            </div>
          </div>
        ) : null}

        <div className="a-card !p-0 max-h-56 overflow-y-auto">
          <table className="a-table">
            <thead>
              <tr>
                <th>What</th>
                <th className="text-right">Qty</th>
                <th className="text-right">Est. price</th>
                <th className="text-right">Total</th>
              </tr>
            </thead>
            <tbody>
              {(request?.items ?? []).map((line) => (
                <tr key={line.id}>
                  <td className="text-sm font-bold">{line.description}</td>
                  <td className="text-right tabular-nums text-sm">
                    {line.qty} {line.unit.toLowerCase()}
                  </td>
                  <td className="text-right tabular-nums text-sm">{formatIdr(line.estimatedPriceIdr)}</td>
                  <td className="text-right tabular-nums">{formatIdr(line.totalIdr)}</td>
                </tr>
              ))}
              {(request?.items ?? []).length === 0 ? (
                <tr>
                  <td colSpan={4} className="py-6 text-center text-sm text-muted">
                    Nothing asked for yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        {request?.rejectionReason ? (
          <ErrorNote message={`Turned down: ${request.rejectionReason}`} />
        ) : null}

        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Close
          </button>
          {request?.status === 'DRAFT' ? (
            <button
              className="a-btn"
              disabled={submit.isPending || (request?.items ?? []).length === 0}
              onClick={() => submit.mutate()}
            >
              {submit.isPending ? 'Submitting…' : 'Submit for approval'}
            </button>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}

function ConvertModal({
  requestId,
  onClose,
  onDone,
}: {
  requestId: string;
  onClose: () => void;
  onDone: (orderId: string) => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [supplier, setSupplier] = useState('');
  const { data: suppliers } = useQuery({
    queryKey: ['purchasing', 'suppliers'],
    queryFn: () => api.admin.purchasing.suppliers.list(),
  });

  const save = useMutation({
    mutationFn: () => api.admin.purchasing.requests.convert(requestId, supplier),
    onSuccess: (order) => onDone(order.id),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That did not become an order.'),
  });

  return (
    <Modal title="Turn into a purchase order" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="text-sm text-muted">
          Every line carries across at its estimated price. A line that is not a catalogue item
          cannot be ordered as stock.
        </p>
        <Field label="Supplier">
          <SearchSelect
            value={supplier}
            onChange={setSupplier}
            placeholder="Search supplier…"
            options={(suppliers ?? [])
              .filter((s) => s.status === 'ACTIVE' || s.status === 'PROBATION')
              .map((s) => ({ value: s.id, label: s.name, hint: s.code }))}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !supplier} onClick={() => save.mutate()}>
            {save.isPending ? 'Creating…' : 'Create order'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
