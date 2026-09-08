'use client';

import type { UpsertSupplierInput } from '@nuhabit/contracts';
import { PAYMENT_TERMS, SUPPLIER_STATUSES, type Supplier } from '@nuhabit/domain';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Trash2 } from 'lucide-react';
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
import { usePermissions } from '../../../../lib/auth';

export default function SuppliersPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('');
  const [editing, setEditing] = useState<Supplier | 'new' | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: suppliers, isLoading, error: listError } = useQuery({
    queryKey: ['purchasing', 'suppliers', query, status],
    queryFn: () =>
      api.admin.purchasing.suppliers.list({
        query: query || undefined,
        status: status || undefined,
      }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.purchasing.suppliers.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['purchasing'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That supplier did not delete.'),
  });

  const rows = suppliers ?? [];

  return (
    <div>
      <PageTitle
        title="Suppliers"
        subtitle="Who the studio buys from, and on what terms"
        actions={
          <>
            <Link href="/purchasing" className="a-btn-ghost">
              Orders
            </Link>
            {can('purchasing.manage') ? (
              <button className="a-btn" onClick={() => setEditing('new')}>
                + New supplier
              </button>
            ) : null}
          </>
        }
      />
      <ErrorNote message={error} />
      <QueryError error={listError} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard label="Suppliers" value={rows.length} />
        <StatCard label="Active" value={rows.filter((s) => s.status === 'ACTIVE').length} tone="brand" />
        <StatCard label="On probation" value={rows.filter((s) => s.status === 'PROBATION').length} />
        <StatCard
          label="Blocked"
          value={rows.filter((s) => s.status === 'BLOCKED').length}
          hint="No order may name them"
        />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search name, code or contact…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="w-44">
          <SearchSelect
            value={status}
            onChange={setStatus}
            allowEmpty
            emptyLabel="Any status"
            placeholder="Search status…"
            options={SUPPLIER_STATUSES.map((s) => ({ value: s, label: s[0]! + s.slice(1).toLowerCase() }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading suppliers…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Supplier</th>
                  <th>Contact</th>
                  <th>City</th>
                  <th>Terms</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((supplier) => (
                  <tr key={supplier.id}>
                    <td>
                      <p className="font-bold">{supplier.name}</p>
                      <p className="text-xs text-muted">
                        {supplier.code}
                        {supplier.category ? ` · ${supplier.category}` : ''}
                      </p>
                    </td>
                    <td className="text-sm">
                      {supplier.contactName ?? <span className="text-muted">—</span>}
                      {supplier.contactPhone ? (
                        <p className="text-xs text-muted">{supplier.contactPhone}</p>
                      ) : null}
                    </td>
                    <td className="text-sm">{supplier.city ?? <span className="text-muted">—</span>}</td>
                    <td className="text-sm">{supplier.paymentTerms}</td>
                    <td>
                      <StatusBadge status={supplier.status} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Edit',
                            icon: Pencil,
                            disabled: !can('purchasing.manage'),
                            onClick: () => setEditing(supplier),
                          },
                          {
                            label: 'Delete',
                            icon: Trash2,
                            tone: 'danger' as const,
                            disabled: !can('purchasing.manage'),
                            onClick: () => {
                              if (confirm(`Delete ${supplier.name}? Blocking is usually better.`))
                                remove.mutate(supplier.id);
                            },
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      No suppliers match those filters.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {editing ? (
        <SupplierModal
          supplier={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['purchasing'] });
          }}
        />
      ) : null}
    </div>
  );
}

function SupplierModal({
  supplier,
  onClose,
  onDone,
}: {
  supplier: Supplier | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState<UpsertSupplierInput>(() => ({
    code: supplier?.code ?? '',
    name: supplier?.name ?? '',
    contactName: supplier?.contactName ?? null,
    contactPhone: supplier?.contactPhone ?? null,
    email: supplier?.email ?? null,
    address: supplier?.address ?? null,
    city: supplier?.city ?? null,
    taxNumber: supplier?.taxNumber ?? null,
    paymentTerms: supplier?.paymentTerms ?? 'NET30',
    bankName: supplier?.bankName ?? null,
    bankAccount: supplier?.bankAccount ?? null,
    bankHolder: supplier?.bankHolder ?? null,
    category: supplier?.category ?? null,
    status: supplier?.status ?? 'ACTIVE',
    note: supplier?.note ?? null,
  }));
  const set = <K extends keyof UpsertSupplierInput>(k: K, v: UpsertSupplierInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const save = useMutation({
    mutationFn: () =>
      supplier
        ? api.admin.purchasing.suppliers.update(supplier.id, form)
        : api.admin.purchasing.suppliers.create(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That supplier did not save.'),
  });

  return (
    <Modal title={supplier ? `Edit ${supplier.name}` : 'New supplier'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Code">
            <input className="a-input" value={form.code} onChange={(e) => set('code', e.target.value)} />
          </Field>
          <Field label="Category">
            <input
              className="a-input"
              value={form.category ?? ''}
              onChange={(e) => set('category', e.target.value || null)}
            />
          </Field>
        </div>
        <Field label="Name">
          <input className="a-input" value={form.name} onChange={(e) => set('name', e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Contact">
            <input
              className="a-input"
              value={form.contactName ?? ''}
              onChange={(e) => set('contactName', e.target.value || null)}
            />
          </Field>
          <Field label="Phone">
            <input
              className="a-input"
              value={form.contactPhone ?? ''}
              onChange={(e) => set('contactPhone', e.target.value || null)}
            />
          </Field>
          <Field label="Email">
            <input
              className="a-input"
              value={form.email ?? ''}
              onChange={(e) => set('email', e.target.value || null)}
            />
          </Field>
          <Field label="City">
            <input
              className="a-input"
              value={form.city ?? ''}
              onChange={(e) => set('city', e.target.value || null)}
            />
          </Field>
          <Field label="Payment terms" hint="How long after delivery the invoice falls due.">
            <SearchSelect
              value={form.paymentTerms ?? 'NET30'}
              onChange={(v) => set('paymentTerms', v)}
              placeholder="Search terms…"
              options={PAYMENT_TERMS.map((t) => ({ value: t, label: t }))}
            />
          </Field>
          <Field label="Status" hint="Blocked means no order may name them again.">
            <SearchSelect
              value={form.status ?? 'ACTIVE'}
              onChange={(v) => set('status', v)}
              placeholder="Search status…"
              options={SUPPLIER_STATUSES.map((s) => ({ value: s, label: s[0]! + s.slice(1).toLowerCase() }))}
            />
          </Field>
          <Field label="Tax number (NPWP)">
            <input
              className="a-input"
              value={form.taxNumber ?? ''}
              onChange={(e) => set('taxNumber', e.target.value || null)}
            />
          </Field>
          <Field label="Bank">
            <input
              className="a-input"
              value={form.bankName ?? ''}
              onChange={(e) => set('bankName', e.target.value || null)}
            />
          </Field>
          <Field label="Account number">
            <input
              className="a-input"
              value={form.bankAccount ?? ''}
              onChange={(e) => set('bankAccount', e.target.value || null)}
            />
          </Field>
          <Field label="Account holder">
            <input
              className="a-input"
              value={form.bankHolder ?? ''}
              onChange={(e) => set('bankHolder', e.target.value || null)}
            />
          </Field>
        </div>
        <Field label="Address">
          <textarea
            className="a-input"
            rows={2}
            value={form.address ?? ''}
            onChange={(e) => set('address', e.target.value || null)}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !form.code || !form.name} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
