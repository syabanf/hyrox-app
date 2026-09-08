'use client';

import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Undo2 } from 'lucide-react';
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
 * Sending goods back.
 *
 * Somebody's signature rather than the receiving bay's decision: it costs the
 * relationship something, and posting one raises a credit note at the moment
 * the stock leaves — which is how a supplier stops being paid in full for
 * goods they took back.
 */
export default function ReturnsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [status, setStatus] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const [raising, setRaising] = useState(false);

  const { data: returns, isLoading, error } = useQuery({
    queryKey: ['purchasing', 'returns', status],
    queryFn: () => api.admin.purchasing.returns.list({ status: status || undefined, limit: 100 }),
  });

  const rows = returns ?? [];
  const waiting = rows.filter((r) => r.status === 'PENDING_APPROVAL').length;

  return (
    <div>
      <PageTitle
        title="Returns"
        subtitle="Goods going back, and who signed for it"
        actions={
          can('purchasing.receive') ? (
            <button className="a-btn" onClick={() => setRaising(true)}>
              <Undo2 size={16} /> Send something back
            </button>
          ) : null
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink" label="Returns" value={rows.length} icon={Undo2} />
        <StatCard
          label="Waiting on a signature"
          value={waiting}
          tone={waiting > 0 ? 'brand' : undefined}
        />
        <StatCard tone="ok" label="Approved, not yet sent" value={rows.filter((r) => r.status === 'APPROVED').length} />
        <StatCard tone="brand"
          label="Gone back"
          value={formatIdr(
            rows.filter((r) => r.status === 'POSTED').reduce((s, r) => s + r.totalIdr, 0),
          )}
          hint="Each one raised a credit note"
        />
      </div>

      <div className="mb-3 flex flex-wrap gap-1 rounded-xl border border-line p-1">
        {[
          { value: '', label: 'Everything' },
          { value: 'DRAFT', label: 'Draft' },
          { value: 'PENDING_APPROVAL', label: 'Waiting' },
          { value: 'APPROVED', label: 'Approved' },
          { value: 'REJECTED', label: 'Rejected' },
          { value: 'POSTED', label: 'Sent back' },
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
        <Spinner label="Loading returns…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Return</th>
                  <th>Why</th>
                  <th>Returned on</th>
                  <th className="text-right">Worth</th>
                  <th>Status</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rows.map((ret) => (
                  <tr key={ret.id}>
                    <td className="font-bold">{ret.returnNumber}</td>
                    <td className="text-sm">
                      {ret.reasonType.toLowerCase().replace('_', ' ')}
                      {ret.reasonNote ? (
                        <span className="block text-xs text-muted">{ret.reasonNote}</span>
                      ) : null}
                    </td>
                    <td className="text-sm">{ret.returnedOn}</td>
                    <td className="text-right tabular-nums">{formatIdr(ret.totalIdr)}</td>
                    <td>
                      <StatusBadge status={ret.status} />
                    </td>
                    <td className="text-right">
                      <button
                        className="a-btn-ghost !px-3 !py-1 text-xs"
                        onClick={() => setOpen(ret.id)}
                      >
                        Open
                      </button>
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      Nothing has gone back.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {raising ? (
        <RaiseModal
          onClose={() => setRaising(false)}
          onDone={(id) => {
            setRaising(false);
            setOpen(id);
            void qc.invalidateQueries({ queryKey: ['purchasing'] });
          }}
        />
      ) : null}

      {open ? (
        <ReturnSheet
          returnId={open}
          canReceive={can('purchasing.receive')}
          canApprove={can('purchasing.approve')}
          onClose={() => setOpen(null)}
          onChanged={() => void qc.invalidateQueries({ queryKey: ['purchasing'] })}
        />
      ) : null}
    </div>
  );
}

function RaiseModal({
  onClose,
  onDone,
}: {
  onClose: () => void;
  onDone: (id: string) => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({ receiptId: '', reasonType: 'DAMAGED', reasonNote: '' });

  // Only goods actually taken into stock can go back out of it.
  const { data: receipts } = useQuery({
    queryKey: ['purchasing', 'receipts', 'posted'],
    queryFn: () => api.admin.purchasing.receipts.list({ status: 'POSTED', limit: 200 }),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.purchasing.returns.open({
        receiptId: form.receiptId,
        reasonType: form.reasonType,
        reasonNote: form.reasonNote || null,
      }),
    onSuccess: (ret) => onDone(ret.id),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That return did not open.'),
  });

  return (
    <Modal title="Send goods back" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Against which delivery" hint="Only what was actually taken into stock can go back.">
          <SearchSelect
            value={form.receiptId}
            onChange={(v) => setForm((f) => ({ ...f, receiptId: v }))}
            placeholder="Search goods receipt…"
            options={(receipts ?? []).map((r) => ({
              value: r.id,
              label: r.grnNumber,
              hint: r.receivedOn,
            }))}
          />
        </Field>
        <Field label="Why">
          <select
            className="a-input"
            value={form.reasonType}
            onChange={(e) => setForm((f) => ({ ...f, reasonType: e.target.value }))}
          >
            {['DAMAGED', 'WRONG_ITEM', 'EXPIRED', 'OVERSTOCK', 'SPEC_MISMATCH', 'OTHER'].map((r) => (
              <option key={r} value={r}>
                {r.toLowerCase().replace('_', ' ')}
              </option>
            ))}
          </select>
        </Field>
        <Field label="What to tell the supplier">
          <input
            className="a-input"
            value={form.reasonNote}
            onChange={(e) => setForm((f) => ({ ...f, reasonNote: e.target.value }))}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={!form.receiptId || save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Opening…' : 'Open it'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function ReturnSheet({
  returnId,
  canReceive,
  canApprove,
  onClose,
  onChanged,
}: {
  returnId: string;
  canReceive: boolean;
  canApprove: boolean;
  onClose: () => void;
  onChanged: () => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [line, setLine] = useState('');
  const [qty, setQty] = useState('');
  const [note, setNote] = useState('');

  const { data: ret, isLoading } = useQuery({
    queryKey: ['purchasing', 'return', returnId],
    queryFn: () => api.admin.purchasing.returns.get(returnId),
  });
  const { data: receipt } = useQuery({
    queryKey: ['purchasing', 'receipt', ret?.receiptId],
    queryFn: () => api.admin.purchasing.receipts.get(ret!.receiptId),
    enabled: !!ret,
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['purchasing', 'return', returnId] });
    onChanged();
  };

  const add = useMutation({
    mutationFn: () =>
      api.admin.purchasing.returns.addLine(returnId, {
        receiptItemId: line,
        qty: Number(qty),
      }),
    onSuccess: () => {
      setError(null);
      setLine('');
      setQty('');
      refresh();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That line did not save.'),
  });

  const act = useMutation({
    mutationFn: (action: 'submit' | 'approve' | 'reject' | 'revise' | 'post') => {
      switch (action) {
        case 'submit':
          return api.admin.purchasing.returns.submit(returnId);
        case 'approve':
          return api.admin.purchasing.returns.approve(returnId, note || undefined);
        case 'reject':
          return api.admin.purchasing.returns.reject(returnId, note);
        case 'revise':
          return api.admin.purchasing.returns.revise(returnId);
        case 'post':
          return api.admin.purchasing.returns.post(returnId);
      }
    },
    onSuccess: () => {
      setError(null);
      setNote('');
      refresh();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That did not go through.'),
  });

  if (isLoading || !ret) {
    return (
      <Modal title="Return" onClose={onClose}>
        <Spinner label="Loading…" />
      </Modal>
    );
  }

  return (
    <Modal title={`${ret.returnNumber} · ${ret.supplierName}`} onClose={onClose} wide>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="flex items-center justify-between gap-2">
          <StatusBadge status={ret.status} />
          <span className="tabular-nums font-bold">{formatIdr(ret.totalIdr)}</span>
        </div>

        <p className="rounded-lg bg-surface-raised px-3 py-2 text-xs text-muted">
          Nothing leaves on the receiving bay's say-so. Once approved and posted, the stock goes
          out and a credit note is raised for what it was worth — so the supplier's next invoice
          is smaller rather than somebody having to remember.
        </p>

        {ret.decisionNote ? (
          <p className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
            <span className="font-bold">Decision: </span>
            {ret.decisionNote}
          </p>
        ) : null}

        {canReceive && ret.status === 'DRAFT' ? (
          <div className="grid gap-2 sm:grid-cols-[1fr_auto_auto]">
            <SearchSelect
              value={line}
              onChange={setLine}
              placeholder="Search a received line…"
              options={(receipt?.items ?? [])
                .filter((i) => i.qtyReturnable > 0)
                .map((i) => ({
                  value: i.id,
                  label: i.itemName,
                  hint: `${i.qtyReturnable} can still go back`,
                }))}
            />
            <input
              className="a-input w-28"
              placeholder="How many"
              value={qty}
              onChange={(e) => setQty(e.target.value)}
            />
            <button
              className="a-btn"
              disabled={add.isPending || !line || Number(qty) <= 0}
              onClick={() => add.mutate()}
            >
              Add
            </button>
          </div>
        ) : null}

        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Item</th>
                <th className="text-right">Going back</th>
                <th className="text-right">Worth</th>
              </tr>
            </thead>
            <tbody>
              {ret.items.map((item) => (
                <tr key={item.id}>
                  <td className="font-bold">{item.itemName}</td>
                  <td className="text-right tabular-nums">{item.qty}</td>
                  <td className="text-right tabular-nums">
                    {formatIdr(item.qty * item.unitPriceIdr)}
                  </td>
                </tr>
              ))}
              {ret.items.length === 0 ? (
                <tr>
                  <td colSpan={3} className="py-6 text-center text-sm text-muted">
                    Nothing on it yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        {canApprove && ret.status === 'PENDING_APPROVAL' ? (
          <Field label="Note on the decision" hint="Required to reject: a refusal without a reason is a dead end.">
            <input className="a-input" value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
        ) : null}

        <div className="mt-1 flex flex-wrap justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Close
          </button>
          {canReceive && ret.status === 'DRAFT' && ret.items.length > 0 ? (
            <button className="a-btn" disabled={act.isPending} onClick={() => act.mutate('submit')}>
              Send for approval
            </button>
          ) : null}
          {canReceive && ret.status === 'REJECTED' ? (
            <button className="a-btn" disabled={act.isPending} onClick={() => act.mutate('revise')}>
              Take it back to draft
            </button>
          ) : null}
          {canApprove && ret.status === 'PENDING_APPROVAL' ? (
            <>
              <button
                className="a-btn-ghost"
                disabled={act.isPending || !note.trim()}
                title={note.trim() ? undefined : 'Say why first'}
                onClick={() => act.mutate('reject')}
              >
                Reject
              </button>
              <button className="a-btn" disabled={act.isPending} onClick={() => act.mutate('approve')}>
                Approve
              </button>
            </>
          ) : null}
          {canReceive && ret.status === 'APPROVED' ? (
            <button className="a-btn" disabled={act.isPending} onClick={() => act.mutate('post')}>
              Send it back
            </button>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}
