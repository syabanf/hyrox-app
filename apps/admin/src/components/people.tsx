'use client';

import type { EmployeeView, UpsertEmployeeInput } from '@nuhabit/contracts';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useMemo, useState, type ReactNode } from 'react';
import { ErrorNote, Modal, SearchSelect } from './ui';
import { api, ApiError } from '../lib/api';

/**
 * The whole record in one form. It is a PUT rather than a PATCH on purpose:
 * an employee carries bank details and next of kin, and a partial write of
 * that is a good way to silently drop one of them.
 */
export function EmployeeModal({
  employee,
  onClose,
  onDone,
}: {
  employee: EmployeeView | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const { data: departments } = useQuery({
    queryKey: ['hris', 'departments'],
    queryFn: api.admin.hris.departments.list,
  });
  const { data: positions } = useQuery({
    queryKey: ['hris', 'positions'],
    queryFn: api.admin.hris.positions.list,
  });
  const { data: statuses } = useQuery({
    queryKey: ['hris', 'employment-statuses'],
    queryFn: api.admin.hris.employmentStatuses,
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data: coaches } = useQuery({ queryKey: ['coaches'], queryFn: api.admin.coaches.list });
  const { data: peers } = useQuery({
    queryKey: ['hris', 'employees', 'all'],
    queryFn: () => api.admin.hris.employees.list(),
  });

  const [form, setForm] = useState<UpsertEmployeeInput>(() => ({
    fullName: employee?.fullName ?? '',
    employeeNumber: employee?.employeeNumber ?? '',
    email: employee?.email ?? '',
    phone: employee?.phone ?? '',
    birthDate: employee?.birthDate ?? null,
    address: employee?.address ?? null,
    joinDate: employee?.joinDate ?? new Date().toISOString().slice(0, 10),
    endDate: employee?.endDate ?? null,
    employmentStatusCode: employee?.employmentStatusCode ?? 'PERMANENT',
    active: employee?.active ?? true,
    departmentId: employee?.departmentId ?? null,
    positionId: employee?.positionId ?? null,
    branchId: employee?.branchId ?? null,
    reportingTo: employee?.reportingTo ?? null,
    coachId: employee?.coachId ?? null,
    bankName: employee?.bankName ?? null,
    bankAccount: employee?.bankAccount ?? null,
    emergencyContactName: employee?.emergencyContactName ?? null,
    emergencyContactPhone: employee?.emergencyContactPhone ?? null,
    emergencyContactRelation: employee?.emergencyContactRelation ?? null,
    notes: employee?.notes ?? null,
  }));

  const set = <K extends keyof UpsertEmployeeInput>(key: K, value: UpsertEmployeeInput[K]) =>
    setForm((f) => ({ ...f, [key]: value }));

  const save = useMutation({
    mutationFn: () =>
      employee
        ? api.admin.hris.employees.update(employee.id, form)
        : api.admin.hris.employees.create(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That record did not save.'),
  });

  const peerOptions = useMemo(
    () =>
      (peers ?? [])
        .filter((p) => p.id !== employee?.id)
        .map((p) => ({ value: p.id, label: p.fullName, hint: p.positionTitle ?? undefined })),
    [peers, employee?.id],
  );

  return (
    <Modal title={employee ? `Edit ${employee.fullName}` : 'New employee'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />

        <Field label="Full name">
          <input
            className="a-input"
            value={form.fullName}
            onChange={(e) => set('fullName', e.target.value)}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Employee number">
            <input
              className="a-input"
              placeholder="NH-0001"
              value={form.employeeNumber}
              onChange={(e) => set('employeeNumber', e.target.value)}
            />
          </Field>
          <Field label="Employment status">
            <SearchSelect
              value={form.employmentStatusCode}
              onChange={(v) => set('employmentStatusCode', v)}
              placeholder="Search status…"
              options={(statuses ?? []).map((s) => ({ value: s.code, label: s.name, hint: s.description }))}
            />
          </Field>
          <Field label="Email">
            <input
              className="a-input"
              value={form.email}
              onChange={(e) => set('email', e.target.value)}
            />
          </Field>
          <Field label="Phone">
            <input
              className="a-input"
              value={form.phone}
              onChange={(e) => set('phone', e.target.value)}
            />
          </Field>
          <Field label="Joined">
            <input
              type="date"
              className="a-input"
              value={form.joinDate}
              onChange={(e) => set('joinDate', e.target.value)}
            />
          </Field>
          <Field label="Left" hint="Leave empty while they are still here">
            <input
              type="date"
              className="a-input"
              value={form.endDate ?? ''}
              onChange={(e) => set('endDate', e.target.value || null)}
            />
          </Field>
          <Field label="Department">
            <SearchSelect
              value={form.departmentId ?? ''}
              onChange={(v) => set('departmentId', v || null)}
              allowEmpty
              emptyLabel="None"
              placeholder="Search department…"
              options={(departments ?? []).map((d) => ({ value: d.id, label: d.name, hint: d.code }))}
            />
          </Field>
          <Field label="Position">
            <SearchSelect
              value={form.positionId ?? ''}
              onChange={(v) => set('positionId', v || null)}
              allowEmpty
              emptyLabel="None"
              placeholder="Search position…"
              options={(positions ?? []).map((p) => ({ value: p.id, label: p.title, hint: p.level }))}
            />
          </Field>
          <Field label="Branch">
            <SearchSelect
              value={form.branchId ?? ''}
              onChange={(v) => set('branchId', v || null)}
              allowEmpty
              emptyLabel="None"
              placeholder="Search branch…"
              options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
            />
          </Field>
          <Field label="Reports to">
            <SearchSelect
              value={form.reportingTo ?? ''}
              onChange={(v) => set('reportingTo', v || null)}
              allowEmpty
              emptyLabel="Nobody"
              placeholder="Search employee…"
              options={peerOptions}
            />
          </Field>
          <Field label="Teaches as" hint="Links their classes to this record">
            <SearchSelect
              value={form.coachId ?? ''}
              onChange={(v) => set('coachId', v || null)}
              allowEmpty
              emptyLabel="Not a coach"
              placeholder="Search coach…"
              options={(coaches ?? []).map((c) => ({ value: c.id, label: c.name, hint: c.specialization }))}
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
          <Field label="Emergency contact">
            <input
              className="a-input"
              value={form.emergencyContactName ?? ''}
              onChange={(e) => set('emergencyContactName', e.target.value || null)}
            />
          </Field>
          <Field label="Their phone">
            <input
              className="a-input"
              value={form.emergencyContactPhone ?? ''}
              onChange={(e) => set('emergencyContactPhone', e.target.value || null)}
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
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active ?? true}
            onChange={(e) => set('active', e.target.checked)}
          />
          Currently employed
        </label>

        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !form.fullName || !form.employeeNumber}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-[11px] font-bold uppercase tracking-wider text-muted">
        {label}
      </span>
      {children}
      {hint ? <span className="mt-1 block text-xs text-muted">{hint}</span> : null}
    </label>
  );
}
