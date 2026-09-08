'use client';

import type { PaymentMethod, ReceiptSettings } from '@nuhabit/domain';
import { METHOD_KINDS } from '@nuhabit/domain';
import { formatDayTime, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Printer } from 'lucide-react';
import { useEffect, useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  SearchSelect,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * How the counter takes money, and what it prints.
 *
 * Both are configuration a shop should be able to change on a Tuesday. What
 * cannot be configured stays a rule enforced by the server — change comes out
 * of the drawer, so a method that gives it has to be counted in it.
 */
export default function CounterSettingsPage() {
  const { can } = usePermissions();
  const [branch, setBranch] = useState('');

  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.admin.branches.list });

  // Settings are per branch, so a branch has to be chosen before there is
  // anything to edit.
  useEffect(() => {
    const first = branches?.[0];
    if (!branch && first) setBranch(first.id);
  }, [branch, branches]);

  return (
    <div>
      <PageTitle title="Counter settings" subtitle="How the till takes money, and what it prints" />

      <div className="grid gap-4">
        <PaymentMethods canManage={can('pos.manage')} />
        {branch ? (
          <ReceiptCard branchId={branch} canManage={can('pos.manage')} branches={branches ?? []} onBranch={setBranch} />
        ) : null}
        <PrintQueue branchId={branch} />
      </div>
    </div>
  );
}

function PaymentMethods({ canManage }: { canManage: boolean }) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<PaymentMethod | 'new' | null>(null);

  const { data: methods, isLoading, error } = useQuery({
    queryKey: ['pos', 'payment-methods'],
    queryFn: () => api.admin.pos.paymentMethods.list(),
  });

  return (
    <div className="a-card !p-0">
      <div className="flex items-start justify-between gap-3 px-4 pt-4">
        <div>
          <h2 className="font-black">How the counter is paid</h2>
          <p className="text-xs text-muted">
            Rows, not code: signing up with a new QRIS provider on Tuesday should not need a
            release on Wednesday.
          </p>
        </div>
        {canManage ? (
          <button className="a-btn-ghost !px-3 !py-1 text-xs" onClick={() => setEditing('new')}>
            + Method
          </button>
        ) : null}
      </div>
      <QueryError error={error} />

      {isLoading ? (
        <div className="p-4">
          <Spinner label="Loading methods…" />
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="a-table mt-2">
            <thead>
              <tr>
                <th>Method</th>
                <th>Behaves like</th>
                <th>Gives change</th>
                <th>In the drawer</th>
                <th>Needs a reference</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(methods ?? []).map((method) => (
                <tr key={method.id}>
                  <td>
                    <p className="font-bold">{method.name}</p>
                    <p className="font-mono text-xs text-muted">{method.code}</p>
                  </td>
                  <td className="text-sm text-muted">{method.kind.toLowerCase().replace('_', ' ')}</td>
                  <td className="text-sm">{method.givesChange ? 'yes' : '—'}</td>
                  <td className="text-sm">{method.countsInDrawer ? 'yes' : '—'}</td>
                  <td className="text-sm">{method.needsReference ? 'yes' : '—'}</td>
                  <td>
                    <StatusBadge status={method.active ? 'ACTIVE' : 'INACTIVE'} />
                  </td>
                  <td className="text-right">
                    {canManage ? (
                      <button
                        className="a-btn-ghost !px-2 !py-1 text-xs"
                        onClick={() => setEditing(method)}
                      >
                        <Pencil size={13} />
                      </button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {editing ? (
        <MethodModal
          method={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['pos', 'payment-methods'] });
          }}
        />
      ) : null}
    </div>
  );
}

function MethodModal({
  method,
  onClose,
  onDone,
}: {
  method: PaymentMethod | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    code: method?.code ?? '',
    name: method?.name ?? '',
    kind: method?.kind ?? 'QR',
    givesChange: method?.givesChange ?? false,
    needsReference: method?.needsReference ?? true,
    countsInDrawer: method?.countsInDrawer ?? false,
    sortOrder: method?.sortOrder ?? 0,
    active: method?.active ?? true,
  });

  const save = useMutation({
    mutationFn: () => api.admin.pos.paymentMethods.save(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That method did not save.'),
  });

  return (
    <Modal title={method ? `Edit ${method.name}` : 'New payment method'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Code" hint="What a receipt and a report call it.">
            <input
              className="a-input font-mono"
              value={form.code}
              onChange={(e) => setForm((f) => ({ ...f, code: e.target.value.toUpperCase() }))}
            />
          </Field>
          <Field label="Name">
            <input
              className="a-input"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </Field>
        </div>
        <Field label="Behaves like">
          <select
            className="a-input"
            value={form.kind}
            onChange={(e) => setForm((f) => ({ ...f, kind: e.target.value as PaymentMethod['kind'] }))}
          >
            {METHOD_KINDS.map((k) => (
              <option key={k} value={k}>
                {k.toLowerCase().replace('_', ' ')}
              </option>
            ))}
          </select>
        </Field>
        <label className="flex items-start gap-2 text-sm font-bold">
          <input
            type="checkbox"
            className="mt-1"
            checked={form.countsInDrawer}
            onChange={(e) =>
              setForm((f) => ({
                ...f,
                countsInDrawer: e.target.checked,
                // Change comes out of the drawer, so it cannot give change
                // without being in it. The server refuses the contradiction;
                // the form should not offer it.
                givesChange: e.target.checked ? f.givesChange : false,
              }))
            }
          />
          <span>
            Counted in the drawer at cash-up
            <span className="block text-xs font-normal text-muted">
              A card sale never touches the drawer, and counting it would make every till look
              short by exactly the card takings.
            </span>
          </span>
        </label>
        <label className="flex items-start gap-2 text-sm font-bold">
          <input
            type="checkbox"
            className="mt-1"
            disabled={!form.countsInDrawer}
            checked={form.givesChange}
            onChange={(e) => setForm((f) => ({ ...f, givesChange: e.target.checked }))}
          />
          <span>
            Can give change
            <span className="block text-xs font-normal text-muted">
              Only possible for something counted in the drawer, because that is where change
              comes from.
            </span>
          </span>
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.needsReference}
            onChange={(e) => setForm((f) => ({ ...f, needsReference: e.target.checked }))}
          />
          Needs a reference (approval code, transfer id)
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active}
            onChange={(e) => setForm((f) => ({ ...f, active: e.target.checked }))}
          />
          Offered at the till
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !form.code || !form.name}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function ReceiptCard({
  branchId,
  canManage,
  branches,
  onBranch,
}: {
  branchId: string;
  canManage: boolean;
  branches: Array<{ id: string; name: string }>;
  onBranch: (id: string) => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState<ReceiptSettings | null>(null);

  const { data: settings, isLoading } = useQuery({
    queryKey: ['pos', 'receipt-settings', branchId],
    queryFn: () => api.admin.pos.receipts.settings(branchId),
  });

  // The form follows whichever branch is being edited, so switching branch
  // does not leave the previous one's wording on screen.
  useEffect(() => {
    if (settings) setForm(settings);
  }, [settings]);

  const save = useMutation({
    mutationFn: () => api.admin.pos.receipts.saveSettings({ ...form!, branchId }),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['pos', 'receipt-settings'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Those settings did not save.'),
  });

  if (isLoading || !form) {
    return (
      <div className="a-card">
        <Spinner label="Loading receipt settings…" />
      </div>
    );
  }

  const set = <K extends keyof ReceiptSettings>(k: K, v: ReceiptSettings[K]) =>
    setForm((f) => (f ? { ...f, [k]: v } : f));

  return (
    <div className="a-card">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-black">What the receipt says</h2>
          <p className="text-xs text-muted">
            Per branch. Thermal paper is 58mm or 80mm, and a line longer than the paper comes out
            cut in half.
          </p>
        </div>
        <div className="w-52">
          <SearchSelect
            value={branchId}
            onChange={onBranch}
            placeholder="Search branch…"
            options={branches.map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
      </div>
      <ErrorNote message={error} />

      <div className="grid gap-3 lg:grid-cols-2">
        <div className="grid gap-3">
          <Field label="Business name">
            <input
              className="a-input"
              value={form.businessName}
              onChange={(e) => set('businessName', e.target.value)}
            />
          </Field>
          <Field label="Address">
            <input className="a-input" value={form.address} onChange={(e) => set('address', e.target.value)} />
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Phone">
              <input
                className="a-input"
                value={form.phone ?? ''}
                onChange={(e) => set('phone', e.target.value || null)}
              />
            </Field>
            <Field label="Tax number">
              <input
                className="a-input"
                value={form.taxNumber ?? ''}
                onChange={(e) => set('taxNumber', e.target.value || null)}
              />
            </Field>
          </div>
          <Field label="Header line">
            <input className="a-input" value={form.header} onChange={(e) => set('header', e.target.value)} />
          </Field>
          <Field label="Footer line">
            <input className="a-input" value={form.footer} onChange={(e) => set('footer', e.target.value)} />
          </Field>
          <Field label="Paper">
            <select
              className="a-input max-w-[10rem]"
              value={form.paperWidth}
              onChange={(e) => set('paperWidth', Number(e.target.value) as 58 | 80)}
            >
              <option value={58}>58mm (32 columns)</option>
              <option value={80}>80mm (48 columns)</option>
            </select>
          </Field>
          <label className="flex items-center gap-2 text-sm font-bold">
            <input
              type="checkbox"
              checked={form.showCashier}
              onChange={(e) => set('showCashier', e.target.checked)}
            />
            Print who served
          </label>
          <label className="flex items-center gap-2 text-sm font-bold">
            <input
              type="checkbox"
              checked={form.autoPrint}
              onChange={(e) => set('autoPrint', e.target.checked)}
            />
            Print automatically on completion
          </label>
        </div>

        <div>
          <p className="a-label">Roughly how it will look</p>
          <pre className="overflow-x-auto rounded-xl bg-ink p-4 font-mono text-[11px] leading-tight text-white">
{sampleReceipt(form)}
          </pre>
        </div>
      </div>

      {canManage ? (
        <div className="mt-3 flex justify-end">
          <button className="a-btn" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      ) : null}
    </div>
  );
}

/**
 * A rough preview.
 *
 * Deliberately marked as roughly: the real receipt is rendered on the server
 * so a reprint months later is identical to the original, and reimplementing
 * that here would give two answers that could drift.
 */
function sampleReceipt(settings: ReceiptSettings): string {
  const width = settings.paperWidth >= 80 ? 48 : 32;
  const centre = (text: string) => {
    const pad = Math.max(0, Math.floor((width - text.length) / 2));
    return ' '.repeat(pad) + text;
  };
  const row = (label: string, value: string) => {
    const gap = Math.max(1, width - label.length - value.length);
    return label + ' '.repeat(gap) + value;
  };
  const rule = '-'.repeat(width);

  return [
    settings.businessName ? centre(settings.businessName.toUpperCase()) : null,
    settings.address ? centre(settings.address) : null,
    settings.phone ? centre(settings.phone) : null,
    settings.header ? centre(settings.header) : null,
    rule,
    row('SAL-20260908-A1B2', '08/09/26 14:22'),
    settings.showCashier ? row('Cashier', 'Nadia') : null,
    rule,
    'Protein Bar',
    row('  2 pcs x 35.000', '70.000'),
    rule,
    row('TOTAL', '70.000'),
    row('CASH', '100.000'),
    row('Change', '30.000'),
    settings.footer ? rule : null,
    settings.footer ? centre(settings.footer) : null,
  ]
    .filter((line): line is string => line !== null)
    .join('\n');
}

function PrintQueue({ branchId }: { branchId: string }) {
  const { data: jobs, isLoading } = useQuery({
    queryKey: ['pos', 'print-jobs', branchId],
    queryFn: () => api.admin.pos.printJobs.list({ branchId: branchId || undefined, limit: 30 }),
    refetchInterval: 15_000,
  });

  const rows = jobs ?? [];

  return (
    <div className="a-card !p-0">
      <div className="px-4 pt-4">
        <h2 className="flex items-center gap-2 font-black">
          <Printer size={16} /> Print queue
        </h2>
        <p className="text-xs text-muted">
          The server cannot reach a printer on the shop's network, so it writes the job and the
          counter's agent picks it up. A receipt that never printed is visible here as one.
        </p>
      </div>

      {isLoading ? (
        <div className="p-4">
          <Spinner label="Loading the queue…" />
        </div>
      ) : (
        <table className="a-table mt-2">
          <thead>
            <tr>
              <th>Job</th>
              <th>Kind</th>
              <th>Queued</th>
              <th className="text-right">Attempts</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((job) => (
              <tr key={job.id}>
                <td className="font-mono text-xs">{job.id.slice(-8)}</td>
                <td className="text-sm">{job.kind.toLowerCase().replace('_', ' ')}</td>
                <td className="text-sm text-muted">{formatDayTime(job.createdAt)}</td>
                <td className="text-right tabular-nums text-sm">{job.attempts}</td>
                <td>
                  <StatusBadge status={job.status} />
                  {job.error ? <p className="text-xs text-danger">{job.error}</p> : null}
                </td>
              </tr>
            ))}
            {rows.length === 0 ? (
              <tr>
                <td colSpan={5} className="py-8 text-center text-sm text-muted">
                  Nothing waiting to print.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      )}
    </div>
  );
}
