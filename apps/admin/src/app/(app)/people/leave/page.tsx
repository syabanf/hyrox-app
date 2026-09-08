'use client';

import { LEAVE_TYPES, shortTime } from '@nuhabit/domain';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Ban, Check, X } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

function studioDate(offsetDays = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  return d.toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
}

export default function LeavePage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [tab, setTab] = useState<'leave' | 'overtime'>('leave');
  const [status, setStatus] = useState('PENDING');
  const [employeeId, setEmployeeId] = useState('');
  const [query, setQuery] = useState('');
  const [filing, setFiling] = useState<'leave' | 'overtime' | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: leaves, isLoading } = useQuery({
    queryKey: ['hris', 'leaves', status, employeeId],
    queryFn: () =>
      api.admin.hris.leaves.list({
        status: status || undefined,
        employeeId: employeeId || undefined,
        limit: 500,
      }),
  });
  const { data: overtime } = useQuery({
    queryKey: ['hris', 'overtime', status, employeeId],
    queryFn: () =>
      api.admin.hris.overtime.list({
        status: status || undefined,
        employeeId: employeeId || undefined,
        limit: 500,
      }),
  });
  const { data: employees } = useQuery({
    queryKey: ['hris', 'employees', 'all'],
    queryFn: () => api.admin.hris.employees.list(),
  });

  const decideLeave = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'approve' | 'reject' | 'cancel' }) =>
      api.admin.hris.leaves.decide(
        id,
        action,
        action === 'reject' ? (prompt('Why is this being turned down?') ?? '') : '',
      ),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['hris'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That decision did not save.'),
  });

  const decideOvertime = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'approve' | 'reject' | 'cancel' }) =>
      api.admin.hris.overtime.decide(
        id,
        action,
        action === 'reject' ? (prompt('Why is this being turned down?') ?? '') : '',
      ),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['hris'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That decision did not save.'),
  });

  const q = query.trim().toLowerCase();
  const leaveRows = (leaves ?? []).filter((l) => !q || l.employeeName.toLowerCase().includes(q));
  const overtimeRows = (overtime ?? []).filter((o) => !q || o.employeeName.toLowerCase().includes(q));
  const canDecide = can('hris.approve');

  return (
    <div>
      <PageTitle
        title="Leave and overtime"
        subtitle="A public holiday is free; collective leave comes out of the allowance"
        actions={
          can('hris.attendance') ? (
            <>
              <button className="a-btn-ghost" onClick={() => setFiling('overtime')}>
                Claim overtime
              </button>
              <button className="a-btn" onClick={() => setFiling('leave')}>
                File leave
              </button>
            </>
          ) : undefined
        }
      />
      <ErrorNote message={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard
          label="Leave awaiting a decision"
          value={(leaves ?? []).filter((l) => l.status === 'PENDING').length}
          tone="brand"
        />
        <StatCard tone="lime"
          label="Overtime awaiting a decision"
          value={(overtime ?? []).filter((o) => o.status === 'PENDING').length}
        />
        <StatCard tone="ok"
          label="Days requested"
          value={leaveRows.reduce((sum, l) => sum + l.totalDays, 0).toFixed(1)}
        />
        <StatCard tone="danger"
          label="Overtime hours"
          value={overtimeRows.reduce((sum, o) => sum + o.hours, 0).toFixed(1)}
        />
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <div className="flex gap-1.5">
          {(['leave', 'overtime'] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={tab === t ? 'a-btn !px-3 !py-1.5 text-xs' : 'a-btn-ghost !px-3 !py-1.5 text-xs'}
            >
              {t === 'leave' ? 'Leave' : 'Overtime'}
            </button>
          ))}
        </div>
        <input
          className="a-input max-w-xs"
          placeholder="Search employee…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="w-40">
          <SearchSelect
            value={status}
            onChange={setStatus}
            allowEmpty
            emptyLabel="Any status"
            placeholder="Search status…"
            options={['PENDING', 'APPROVED', 'REJECTED', 'CANCELLED'].map((s) => ({
              value: s,
              label: s[0]! + s.slice(1).toLowerCase(),
            }))}
          />
        </div>
        <div className="w-52">
          <SearchSelect
            value={employeeId}
            onChange={setEmployeeId}
            allowEmpty
            emptyLabel="Everyone"
            placeholder="Search employee…"
            options={(employees ?? []).map((e) => ({
              value: e.id,
              label: e.fullName,
              hint: e.employeeNumber,
            }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading requests…" />
      ) : tab === 'leave' ? (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Employee</th>
                  <th>Type</th>
                  <th>From</th>
                  <th>To</th>
                  <th className="text-right">Days</th>
                  <th>Reason</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {leaveRows.map((l) => (
                  <tr key={l.id}>
                    <td>
                      <Link href={`/people/employees/${l.employeeId}`} className="font-bold hover:text-brand">
                        {l.employeeName}
                      </Link>
                      <p className="text-xs text-muted">{l.employeeNumber}</p>
                    </td>
                    <td className="text-sm font-bold">{l.type.replaceAll('_', ' ')}</td>
                    <td className="tabular-nums">{l.startDate}</td>
                    <td className="tabular-nums">{l.endDate}</td>
                    <td className="text-right tabular-nums">{l.totalDays.toFixed(1)}</td>
                    <td className="max-w-xs truncate text-sm text-muted">
                      {l.reason}
                      {l.rejectionReason ? (
                        <span className="block text-xs text-danger">{l.rejectionReason}</span>
                      ) : null}
                    </td>
                    <td>
                      <StatusBadge status={l.status} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Approve',
                            icon: Check,
                            disabled: !canDecide || l.status !== 'PENDING',
                            onClick: () => decideLeave.mutate({ id: l.id, action: 'approve' }),
                          },
                          {
                            label: 'Turn down',
                            icon: X,
                            tone: 'danger' as const,
                            disabled: !canDecide || l.status !== 'PENDING',
                            onClick: () => decideLeave.mutate({ id: l.id, action: 'reject' }),
                          },
                          {
                            label: 'Withdraw',
                            icon: Ban,
                            disabled:
                              !canDecide || (l.status !== 'PENDING' && l.status !== 'APPROVED'),
                            onClick: () => {
                              if (confirm('Withdraw this request and return the days?'))
                                decideLeave.mutate({ id: l.id, action: 'cancel' });
                            },
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {leaveRows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="py-8 text-center text-sm text-muted">
                      Nothing to decide.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Employee</th>
                  <th>Date</th>
                  <th>Window</th>
                  <th className="text-right">Hours</th>
                  <th>Asked by</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {overtimeRows.map((o) => (
                  <tr key={o.id}>
                    <td>
                      <Link href={`/people/employees/${o.employeeId}`} className="font-bold hover:text-brand">
                        {o.employeeName}
                      </Link>
                      <p className="text-xs text-muted">{o.employeeNumber}</p>
                    </td>
                    <td className="tabular-nums">{o.date}</td>
                    <td className="text-sm">
                      {shortTime(o.startTime)}–{shortTime(o.endTime)}
                    </td>
                    <td className="text-right tabular-nums">{o.hours.toFixed(2)}</td>
                    <td className="text-sm">{o.source === 'COMPANY' ? 'The studio' : 'Themselves'}</td>
                    <td>
                      <StatusBadge status={o.status} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Approve',
                            icon: Check,
                            disabled: !canDecide || o.status !== 'PENDING',
                            onClick: () => decideOvertime.mutate({ id: o.id, action: 'approve' }),
                          },
                          {
                            label: 'Turn down',
                            icon: X,
                            tone: 'danger' as const,
                            disabled: !canDecide || o.status !== 'PENDING',
                            onClick: () => decideOvertime.mutate({ id: o.id, action: 'reject' }),
                          },
                          {
                            label: 'Withdraw',
                            icon: Ban,
                            disabled:
                              !canDecide || (o.status !== 'PENDING' && o.status !== 'APPROVED'),
                            onClick: () => decideOvertime.mutate({ id: o.id, action: 'cancel' }),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {overtimeRows.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      No overtime claimed.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {filing === 'leave' ? (
        <FileLeaveModal
          onClose={() => setFiling(null)}
          onDone={() => {
            setFiling(null);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
      {filing === 'overtime' ? (
        <ClaimOvertimeModal
          onClose={() => setFiling(null)}
          onDone={() => {
            setFiling(null);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
    </div>
  );
}

function FileLeaveModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [employeeId, setEmployeeId] = useState('');
  const [type, setType] = useState('ANNUAL');
  const [startDate, setStartDate] = useState(studioDate(1));
  const [endDate, setEndDate] = useState(studioDate(1));
  const [reason, setReason] = useState('');

  const { data: employees } = useQuery({
    queryKey: ['hris', 'employees', 'all'],
    queryFn: () => api.admin.hris.employees.list(),
  });
  const { data: balance } = useQuery({
    queryKey: ['hris', 'balance', employeeId],
    queryFn: () => api.admin.hris.employees.balance(employeeId),
    enabled: employeeId !== '',
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.hris.leaves.request({
        employeeId,
        type: type as (typeof LEAVE_TYPES)[number],
        startDate,
        endDate,
        reason,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That request did not save.'),
  });

  return (
    <Modal title="File leave" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Employee">
          <SearchSelect
            value={employeeId}
            onChange={setEmployeeId}
            placeholder="Search employee…"
            options={(employees ?? []).map((e) => ({
              value: e.id,
              label: e.fullName,
              hint: e.employeeNumber,
            }))}
          />
        </Field>
        {balance ? (
          <p className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
            <span className="font-bold">
              {(balance.annualTotal - balance.annualUsed).toFixed(1)} days
            </span>{' '}
            of annual leave left in {balance.year}.
          </p>
        ) : null}
        <Field label="Type">
          <SearchSelect
            value={type}
            onChange={setType}
            placeholder="Search type…"
            options={LEAVE_TYPES.map((t) => ({ value: t, label: t[0]! + t.slice(1).toLowerCase() }))}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="From">
            <input
              type="date"
              className="a-input"
              value={startDate}
              onChange={(e) => {
                setStartDate(e.target.value);
                if (e.target.value > endDate) setEndDate(e.target.value);
              }}
            />
          </Field>
          <Field label="To">
            <input type="date" className="a-input" value={endDate} onChange={(e) => setEndDate(e.target.value)} />
          </Field>
        </div>
        <Field
          label="Reason"
          hint="Weekends and public holidays inside the range are worked out by the backend and do not cost a day."
        >
          <input className="a-input" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !employeeId || !reason}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Filing…' : 'File'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function ClaimOvertimeModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [employeeId, setEmployeeId] = useState('');
  const [date, setDate] = useState(studioDate());
  const [startTime, setStartTime] = useState('18:00');
  const [endTime, setEndTime] = useState('20:00');
  const [source, setSource] = useState('COMPANY');
  const [reason, setReason] = useState('');

  const { data: employees } = useQuery({
    queryKey: ['hris', 'employees', 'all'],
    queryFn: () => api.admin.hris.employees.list(),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.hris.overtime.request({
        employeeId,
        date,
        startTime,
        endTime,
        source: source as 'COMPANY' | 'EMPLOYEE',
        reason: reason || null,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That claim did not save.'),
  });

  return (
    <Modal title="Claim overtime" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Employee">
          <SearchSelect
            value={employeeId}
            onChange={setEmployeeId}
            placeholder="Search employee…"
            options={(employees ?? []).map((e) => ({
              value: e.id,
              label: e.fullName,
              hint: e.employeeNumber,
            }))}
          />
        </Field>
        <Field label="Date">
          <input type="date" className="a-input" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="From">
            <input type="time" className="a-input" value={startTime} onChange={(e) => setStartTime(e.target.value)} />
          </Field>
          <Field label="To" hint="An end before the start rolls past midnight.">
            <input type="time" className="a-input" value={endTime} onChange={(e) => setEndTime(e.target.value)} />
          </Field>
        </div>
        <Field label="Asked by" hint="Company-directed overtime is payable on different terms.">
          <SearchSelect
            value={source}
            onChange={setSource}
            placeholder="Search…"
            options={[
              { value: 'COMPANY', label: 'The studio' },
              { value: 'EMPLOYEE', label: 'The employee' },
            ]}
          />
        </Field>
        <Field label="Reason">
          <input className="a-input" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !employeeId} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Claim'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
