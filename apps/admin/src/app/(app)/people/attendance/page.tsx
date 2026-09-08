'use client';

import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  Pager,
  PageTitle,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

const PAGE_SIZE = 25;

function studioDate(offsetDays = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  return d.toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
}

function clockLabel(iso: string | null) {
  if (!iso) return '—';
  return new Date(iso).toLocaleTimeString('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Asia/Jakarta',
  });
}

const STATUS_OPTIONS = [
  'PRESENT',
  'LATE',
  'ABSENT',
  'HALF_DAY',
  'REMOTE',
  'ON_LEAVE',
  'HOLIDAY',
  'REST_DAY',
];

export default function AttendancePage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [from, setFrom] = useState(studioDate(-13));
  const [to, setTo] = useState(studioDate());
  const [employeeId, setEmployeeId] = useState('');
  const [status, setStatus] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const [marking, setMarking] = useState(false);

  const { data: rows, isLoading } = useQuery({
    queryKey: ['hris', 'attendance', from, to, employeeId, status],
    queryFn: () =>
      api.admin.hris.attendance.list({
        from,
        to,
        employeeId: employeeId || undefined,
        status: status || undefined,
        limit: 500,
      }),
  });
  const { data: employees } = useQuery({
    queryKey: ['hris', 'employees', 'all'],
    queryFn: () => api.admin.hris.employees.list(),
  });

  const q = query.trim().toLowerCase();
  const filtered = (rows ?? []).filter(
    (r) => !q || r.employeeName.toLowerCase().includes(q) || r.employeeNumber.toLowerCase().includes(q),
  );
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const shown = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  const totalHours = filtered.reduce((sum, r) => sum + r.workHours, 0);
  const lateMinutes = filtered.reduce((sum, r) => sum + r.lateMinutes, 0);

  return (
    <div>
      <PageTitle
        title="Timesheet"
        subtitle="Lateness is counted from the shift start, not from the end of the tolerance"
        actions={
          can('hris.attendance') ? (
            <button className="a-btn" onClick={() => setMarking(true)}>
              Record a day
            </button>
          ) : undefined
        }
      />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink" label="Days on file" value={filtered.length} />
        <StatCard label="Hours worked" value={totalHours.toFixed(1)} tone="brand" />
        <StatCard
          label="Late arrivals"
          value={filtered.filter((r) => r.isLate).length}
          hint={`${lateMinutes} minutes in total`}
          tone={lateMinutes > 0 ? 'danger' : undefined}
        />
        <StatCard tone="danger" label="Absences" value={filtered.filter((r) => r.status === 'ABSENT').length} />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <input type="date" className="a-input max-w-[11rem]" value={from} onChange={(e) => setFrom(e.target.value)} />
        <input type="date" className="a-input max-w-[11rem]" value={to} onChange={(e) => setTo(e.target.value)} />
        <input
          className="a-input max-w-xs"
          placeholder="Search name or number…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-52">
          <SearchSelect
            value={employeeId}
            onChange={(v) => {
              setEmployeeId(v);
              setPage(0);
            }}
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
        <div className="w-40">
          <SearchSelect
            value={status}
            onChange={(v) => {
              setStatus(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="Any status"
            placeholder="Search status…"
            options={STATUS_OPTIONS.map((s) => ({ value: s, label: s.replaceAll('_', ' ') }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Reading the timesheet…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Date</th>
                  <th>Employee</th>
                  <th>Shift</th>
                  <th>In</th>
                  <th>Out</th>
                  <th className="text-right">Hours</th>
                  <th className="text-right">Overtime</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((r) => (
                  <tr key={r.id}>
                    <td className="tabular-nums">{r.date}</td>
                    <td>
                      <Link href={`/people/employees/${r.employeeId}`} className="font-bold hover:text-brand">
                        {r.employeeName}
                      </Link>
                      <p className="text-xs text-muted">{r.employeeNumber}</p>
                    </td>
                    <td className="text-sm">{r.shiftName ?? <span className="text-muted">—</span>}</td>
                    <td className="text-sm">
                      {clockLabel(r.clockIn)}
                      {r.isLate ? (
                        <span className="ml-1.5 text-xs font-bold text-danger">+{r.lateMinutes}m</span>
                      ) : null}
                    </td>
                    <td className="text-sm">{clockLabel(r.clockOut)}</td>
                    <td className="text-right tabular-nums">{r.workHours.toFixed(2)}</td>
                    <td className="text-right tabular-nums">
                      {r.overtimeHours > 0 ? r.overtimeHours.toFixed(2) : '—'}
                    </td>
                    <td>
                      <StatusBadge status={r.status} />
                    </td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="py-8 text-center text-sm text-muted">
                      Nothing on the timesheet for that window.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}

      {marking ? (
        <MarkDayModal
          onClose={() => setMarking(false)}
          onDone={() => {
            setMarking(false);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * Recording a day by hand. It is the escape hatch for everything the clock
 * cannot express — a missed punch, a day someone was simply not there — and it
 * is audited for exactly that reason.
 */
function MarkDayModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [employeeId, setEmployeeId] = useState('');
  const [date, setDate] = useState(studioDate());
  const [status, setStatus] = useState('ABSENT');
  const [notes, setNotes] = useState('');

  const { data: employees } = useQuery({
    queryKey: ['hris', 'employees', 'all'],
    queryFn: () => api.admin.hris.employees.list(),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.hris.attendance.mark({
        employeeId,
        date,
        status,
        notes: notes || null,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That day did not save.'),
  });

  return (
    <Modal title="Record a day" onClose={onClose}>
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
        <Field label="Status">
          <SearchSelect
            value={status}
            onChange={setStatus}
            placeholder="Search status…"
            options={STATUS_OPTIONS.map((s) => ({ value: s, label: s.replaceAll('_', ' ') }))}
          />
        </Field>
        <Field label="Note" hint="Why this was recorded by hand.">
          <input className="a-input" value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !employeeId} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Record'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
