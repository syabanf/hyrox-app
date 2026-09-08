'use client';

import { formatDay, formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Banknote, ReceiptText } from 'lucide-react';
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
 * What we owe, and what has actually left the bank.
 *
 * Three numbers a supplier conversation needs: committed, arrived, paid. A
 * credit note settles a debt without money moving, so it counts here as paid —
 * a payables list that still shows a credited debt sends somebody to pay twice.
 */
export default function PayablesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [supplier, setSupplier] = useState('');
  const [paying, setPaying] = useState(false);
  const [crediting, setCrediting] = useState(false);
  const [voiding, setVoiding] = useState<string | null>(null);

  const { data: suppliers } = useQuery({
    queryKey: ['purchasing', 'suppliers', 'all'],
    queryFn: () => api.admin.purchasing.suppliers.list({ limit: 500 }),
  });
  const { data: payments, isLoading, error } = useQuery({
    queryKey: ['purchasing', 'payments', supplier],
    queryFn: () =>
      api.admin.purchasing.payments.list({ supplierId: supplier || undefined, limit: 200 }),
  });
  const { data: credits } = useQuery({
    queryKey: ['purchasing', 'credits', supplier],
    queryFn: () =>
      api.admin.purchasing.credits.list({ supplierId: supplier || undefined, limit: 200 }),
  });

  const names = new Map((suppliers ?? []).map((s) => [s.id, s.name]));
  const rows = payments ?? [];
  const posted = rows.filter((p) => p.status === 'POSTED');
  const openCredits = (credits ?? []).filter((c) =>
    ['OPEN', 'PARTIALLY_APPLIED'].includes(c.status),
  );

  const refresh = () => void qc.invalidateQueries({ queryKey: ['purchasing'] });

  const postPayment = useMutation({
    mutationFn: (id: string) => api.admin.purchasing.payments.post(id),
    onSuccess: refresh,
  });

  return (
    <div>
      <PageTitle
        title="Payables"
        subtitle="What we owe suppliers, and what has actually left the bank"
        actions={
          can('purchasing.pay') ? (
            <>
              <button className="a-btn-ghost" onClick={() => setCrediting(true)}>
                <ReceiptText size={16} /> Credit note
              </button>
              <button className="a-btn" onClick={() => setPaying(true)}>
                <Banknote size={16} /> Pay a supplier
              </button>
            </>
          ) : null
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink"
          label="Paid"
          value={formatIdr(posted.reduce((s, p) => s + p.amountIdr - p.creditIdr, 0))}
          hint="Cash that actually left"
          icon={Banknote}
        />
        <StatCard tone="warn"
          label="Settled by credit"
          value={formatIdr(posted.reduce((s, p) => s + p.creditIdr, 0))}
          hint="Debt gone, no money moved"
        />
        <StatCard tone="danger" label="Drafts" value={rows.filter((p) => p.status === 'DRAFT').length} />
        <StatCard
          label="Credit notes open"
          value={formatIdr(openCredits.reduce((s, c) => s + (c.amountIdr - c.appliedIdr), 0))}
          tone={openCredits.length > 0 ? 'brand' : undefined}
          hint="Spend these before they lapse"
          icon={ReceiptText}
        />
      </div>

      <div className="mb-4 max-w-xs">
        <SearchSelect
          value={supplier}
          onChange={setSupplier}
          allowEmpty
          emptyLabel="Every supplier"
          placeholder="Search supplier…"
          options={(suppliers ?? []).map((s) => ({ value: s.id, label: s.name, hint: s.code }))}
        />
      </div>

      {isLoading ? (
        <Spinner label="Loading payments…" />
      ) : (
        <>
          <div className="a-card mb-4 !p-0">
            <div className="px-4 pt-4">
              <h2 className="font-black">Payments</h2>
              <p className="text-xs text-muted">
                A draft has moved no money. Posting settles it against its instalment.
              </p>
            </div>
            <div className="overflow-x-auto">
              <table className="a-table mt-2">
                <thead>
                  <tr>
                    <th>Payment</th>
                    <th>Supplier</th>
                    <th>Paid on</th>
                    <th>Method</th>
                    <th className="text-right">Amount</th>
                    <th className="text-right">By credit</th>
                    <th>Status</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((payment) => (
                    <tr key={payment.id}>
                      <td className="font-bold">{payment.paymentNumber}</td>
                      <td className="text-sm">{names.get(payment.supplierId) ?? '—'}</td>
                      <td className="text-sm">{payment.paidOn}</td>
                      <td className="text-xs text-muted">{payment.method.toLowerCase()}</td>
                      <td className="text-right tabular-nums">{formatIdr(payment.amountIdr)}</td>
                      <td className="text-right tabular-nums text-sm text-muted">
                        {payment.creditIdr > 0 ? formatIdr(payment.creditIdr) : '—'}
                      </td>
                      <td>
                        <StatusBadge status={payment.status} />
                      </td>
                      <td className="text-right">
                        {can('purchasing.pay') && payment.status === 'DRAFT' ? (
                          <button
                            className="a-btn-ghost !px-3 !py-1 text-xs"
                            disabled={postPayment.isPending}
                            onClick={() => postPayment.mutate(payment.id)}
                          >
                            Post
                          </button>
                        ) : null}
                        {can('purchasing.pay') && payment.status === 'POSTED' ? (
                          <button
                            className="a-btn-ghost !px-3 !py-1 text-xs"
                            onClick={() => setVoiding(payment.id)}
                          >
                            Void
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  ))}
                  {rows.length === 0 ? (
                    <tr>
                      <td colSpan={8} className="py-8 text-center text-sm text-muted">
                        Nothing paid yet.
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>

          <div className="a-card !p-0">
            <div className="px-4 pt-4">
              <h2 className="font-black">Credit notes</h2>
              <p className="text-xs text-muted">
                What a supplier owes back, usually from a return. The money does not move — the
                next invoice is smaller. Oldest are spent first, so none lapse unused.
              </p>
            </div>
            <div className="overflow-x-auto">
              <table className="a-table mt-2">
                <thead>
                  <tr>
                    <th>Credit note</th>
                    <th>Supplier</th>
                    <th>Issued</th>
                    <th>Reason</th>
                    <th className="text-right">Worth</th>
                    <th className="text-right">Left</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {(credits ?? []).map((credit) => (
                    <tr key={credit.id}>
                      <td className="font-bold">{credit.creditNumber}</td>
                      <td className="text-sm">{names.get(credit.supplierId) ?? '—'}</td>
                      <td className="text-sm">{credit.issuedOn}</td>
                      <td className="text-sm text-muted">{credit.reason ?? '—'}</td>
                      <td className="text-right tabular-nums">{formatIdr(credit.amountIdr)}</td>
                      <td className="text-right tabular-nums font-bold">
                        {formatIdr(credit.amountIdr - credit.appliedIdr)}
                      </td>
                      <td>
                        <StatusBadge status={credit.status} />
                      </td>
                    </tr>
                  ))}
                  {(credits ?? []).length === 0 ? (
                    <tr>
                      <td colSpan={7} className="py-8 text-center text-sm text-muted">
                        No credit notes.
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}

      {paying ? (
        <PaymentModal
          suppliers={(suppliers ?? []).map((s) => ({ id: s.id, name: s.name, code: s.code }))}
          onClose={() => setPaying(false)}
          onDone={() => {
            setPaying(false);
            refresh();
          }}
        />
      ) : null}

      {crediting ? (
        <CreditModal
          suppliers={(suppliers ?? []).map((s) => ({ id: s.id, name: s.name, code: s.code }))}
          onClose={() => setCrediting(false)}
          onDone={() => {
            setCrediting(false);
            refresh();
          }}
        />
      ) : null}

      {voiding ? (
        <VoidModal
          paymentId={voiding}
          onClose={() => setVoiding(null)}
          onDone={() => {
            setVoiding(null);
            refresh();
          }}
        />
      ) : null}
    </div>
  );
}

function PaymentModal({
  suppliers,
  onClose,
  onDone,
}: {
  suppliers: Array<{ id: string; name: string; code: string }>;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    supplierId: '',
    orderId: '',
    termId: '',
    amountIdr: '',
    method: 'TRANSFER',
    reference: '',
    useCredits: true,
  });

  // Only orders belonging to the chosen supplier, so a payment cannot be
  // pointed at somebody else's invoice.
  const { data: orders } = useQuery({
    queryKey: ['purchasing', 'orders', form.supplierId],
    queryFn: () => api.admin.purchasing.orders.list({ supplierId: form.supplierId, limit: 200 }),
    enabled: Boolean(form.supplierId),
  });
  const { data: payables } = useQuery({
    queryKey: ['purchasing', 'payables', form.orderId],
    queryFn: () => api.admin.purchasing.payables.get(form.orderId),
    enabled: Boolean(form.orderId),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.purchasing.payments.record({
        supplierId: form.supplierId,
        orderId: form.orderId || null,
        termId: form.termId || null,
        amountIdr: Number(form.amountIdr),
        method: form.method,
        reference: form.reference || null,
        useCredits: form.useCredits,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That payment did not save.'),
  });

  return (
    <Modal title="Pay a supplier" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Supplier">
          <SearchSelect
            value={form.supplierId}
            onChange={(v) => setForm((f) => ({ ...f, supplierId: v, orderId: '', termId: '' }))}
            placeholder="Search supplier…"
            options={suppliers.map((s) => ({ value: s.id, label: s.name, hint: s.code }))}
          />
        </Field>
        <Field label="Against which order" hint="Optional: a supplier paid off in a lump names none.">
          <SearchSelect
            value={form.orderId}
            onChange={(v) => setForm((f) => ({ ...f, orderId: v, termId: '' }))}
            allowEmpty
            emptyLabel="No particular order"
            placeholder="Search order…"
            options={(orders ?? []).map((o) => ({ value: o.id, label: o.poNumber }))}
            disabled={!form.supplierId}
          />
        </Field>
        {payables && payables.terms.length > 0 ? (
          <Field label="Which instalment">
            <SearchSelect
              value={form.termId}
              onChange={(v) => {
                const term = payables.terms.find((t) => t.id === v);
                setForm((f) => ({
                  ...f,
                  termId: v,
                  // Prefilled with exactly what is owed, because paying more
                  // than an instalment needs is refused.
                  amountIdr: term ? String(term.amountIdr - term.paidIdr) : f.amountIdr,
                }));
              }}
              allowEmpty
              emptyLabel="No instalment — pay against the order"
              placeholder="Search instalment…"
              options={payables.terms.map((t) => ({
                value: t.id,
                label: t.label,
                hint: `${formatIdr(t.amountIdr - t.paidIdr)} outstanding · due ${formatDay(t.dueOn)}`,
              }))}
            />
          </Field>
        ) : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Amount">
            <input
              className="a-input"
              value={form.amountIdr}
              onChange={(e) => setForm((f) => ({ ...f, amountIdr: e.target.value }))}
            />
          </Field>
          <Field label="Method">
            <SearchSelect
              value={form.method}
              onChange={(v) => setForm((f) => ({ ...f, method: v }))}
              placeholder="Search…"
              options={['TRANSFER', 'CASH', 'CHEQUE', 'CARD'].map((m) => ({
                value: m,
                label: m.toLowerCase(),
              }))}
            />
          </Field>
        </div>
        <Field label="Reference" hint="The transfer id or cheque number.">
          <input
            className="a-input font-mono"
            value={form.reference}
            onChange={(e) => setForm((f) => ({ ...f, reference: e.target.value }))}
          />
        </Field>
        <label className="flex items-start gap-2 text-sm font-bold">
          <input
            type="checkbox"
            className="mt-1"
            checked={form.useCredits}
            onChange={(e) => setForm((f) => ({ ...f, useCredits: e.target.checked }))}
          />
          <span>
            Spend open credit notes first
            <span className="block text-xs font-normal text-muted">
              Oldest first. Paying cash while a credit note lapses unused is the same as throwing
              the money away.
            </span>
          </span>
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !form.supplierId || Number(form.amountIdr) <= 0}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Record it'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function CreditModal({
  suppliers,
  onClose,
  onDone,
}: {
  suppliers: Array<{ id: string; name: string; code: string }>;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({ supplierId: '', amountIdr: '', reason: '', expiresOn: '' });

  const save = useMutation({
    mutationFn: () =>
      api.admin.purchasing.credits.raise({
        supplierId: form.supplierId,
        amountIdr: Number(form.amountIdr),
        reason: form.reason || null,
        expiresOn: form.expiresOn || null,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That credit note did not save.'),
  });

  return (
    <Modal title="Raise a credit note" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="text-sm text-muted">
          Returns raise these by themselves. This is for the rest: a price correction, a rebate, a
          goodwill gesture.
        </p>
        <Field label="Supplier">
          <SearchSelect
            value={form.supplierId}
            onChange={(v) => setForm((f) => ({ ...f, supplierId: v }))}
            placeholder="Search supplier…"
            options={suppliers.map((s) => ({ value: s.id, label: s.name, hint: s.code }))}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Worth">
            <input
              className="a-input"
              value={form.amountIdr}
              onChange={(e) => setForm((f) => ({ ...f, amountIdr: e.target.value }))}
            />
          </Field>
          <Field label="Expires" hint="Leave empty if it does not lapse.">
            <input
              className="a-input"
              type="date"
              value={form.expiresOn}
              onChange={(e) => setForm((f) => ({ ...f, expiresOn: e.target.value }))}
            />
          </Field>
        </div>
        <Field label="Why">
          <input
            className="a-input"
            value={form.reason}
            onChange={(e) => setForm((f) => ({ ...f, reason: e.target.value }))}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !form.supplierId || Number(form.amountIdr) <= 0}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Raise it'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function VoidModal({
  paymentId,
  onClose,
  onDone,
}: {
  paymentId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [reason, setReason] = useState('');

  const save = useMutation({
    mutationFn: () => api.admin.purchasing.payments.void(paymentId, reason),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That payment did not void.'),
  });

  return (
    <Modal title="Void this payment" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="text-sm text-muted">
          The instalment gets its money back and the debt shows again, rather than quietly staying
          settled.
        </p>
        <Field label="Why">
          <input className="a-input" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <div className="flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={!reason.trim() || save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Voiding…' : 'Void it'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
