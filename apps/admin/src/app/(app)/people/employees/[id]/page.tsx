'use client';

import { WEEKDAYS, annualRemaining, shortTime } from '@nuhabit/domain';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, Pencil, Plus, Trash2 } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import {
  ErrorNote,
  Modal,
  PageTitle,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../../components/ui';
import { api, ApiError } from '../../../../../lib/api';
import { usePermissions } from '../../../../../lib/auth';
import { EmployeeModal, Field } from '../../../../../components/people';

function clockLabel(iso: string | null) {
  if (!iso) return '—';
  return new Date(iso).toLocaleTimeString('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Asia/Jakarta',
  });
}

export default function EmployeePage() {
  const { id } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [tab, setTab] = useState<'overview' | 'schedule' | 'attendance' | 'leave'>('overview');
  const [editing, setEditing] = useState(false);
  const [assigning, setAssigning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['hris', 'employee', id],
    queryFn: () => api.admin.hris.employees.get(id),
  });

  const removeRow = useMutation({
    mutationFn: (rowId: string) => api.admin.hris.employees.removeScheduleRow(id, rowId),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['hris'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That row did not delete.'),
  });

  if (isLoading || !data) return <Spinner label="Loading the record…" />;
  const { employee, balance, schedule, attendance, leaves, overtime, directReports } = data;

  return (
    <div>
      <Link href="/people/employees" className="mb-3 inline-flex items-center gap-1.5 text-sm font-bold text-muted hover:text-ink">
        <ArrowLeft size={14} /> Staff directory
      </Link>
      <PageTitle
        title={employee.fullName}
        subtitle={[employee.positionTitle, employee.departmentName, employee.branchName]
          .filter(Boolean)
          .join(' · ')}
        actions={
          can('hris.manage') ? (
            <button className="a-btn-ghost" onClick={() => setEditing(true)}>
              <Pencil size={14} className="mr-1.5 inline" /> Edit record
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard label="Employee number" value={employee.employeeNumber} />
        <StatCard
          label="Annual leave left"
          value={annualRemaining(balance).toFixed(1)}
          hint={`of ${balance.annualTotal} days in ${balance.year}`}
          tone="brand"
        />
        <StatCard label="Sick days taken" value={balance.sickUsed.toFixed(1)} />
        <StatCard
          label="Employment"
          value={<StatusBadge status={employee.active ? employee.employmentStatusCode : 'INACTIVE'} />}
          hint={employee.endDate ? `Left ${employee.endDate}` : `Joined ${employee.joinDate}`}
        />
      </div>

      <div className="mb-4 flex flex-wrap gap-1.5">
        {(['overview', 'schedule', 'attendance', 'leave'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={tab === t ? 'a-btn !px-3 !py-1.5 text-xs' : 'a-btn-ghost !px-3 !py-1.5 text-xs'}
          >
            {t[0]!.toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {tab === 'overview' ? (
        <div className="grid gap-3 lg:grid-cols-2">
          <div className="a-card">
            <h2 className="mb-3 font-black">Contact</h2>
            <Detail label="Email" value={employee.email} />
            <Detail label="Phone" value={employee.phone} />
            <Detail label="Address" value={employee.address} />
            <Detail label="Born" value={employee.birthDate} />
          </div>
          <div className="a-card">
            <h2 className="mb-3 font-black">Employment</h2>
            <Detail label="Reports to" value={employee.reportingToName} />
            <Detail label="Teaches as" value={employee.coachName} />
            <Detail label="Joined" value={employee.joinDate} />
            <Detail label="Left" value={employee.endDate} />
          </div>
          <div className="a-card">
            <h2 className="mb-3 font-black">Payroll</h2>
            <Detail label="Bank" value={employee.bankName} />
            <Detail label="Account" value={employee.bankAccount} />
          </div>
          <div className="a-card">
            <h2 className="mb-3 font-black">In an emergency</h2>
            <Detail label="Contact" value={employee.emergencyContactName} />
            <Detail label="Phone" value={employee.emergencyContactPhone} />
            <Detail label="Relation" value={employee.emergencyContactRelation} />
          </div>
          {directReports.length > 0 ? (
            <div className="a-card lg:col-span-2">
              <h2 className="mb-3 font-black">Direct reports</h2>
              <div className="flex flex-wrap gap-2">
                {directReports.map((r) => (
                  <Link
                    key={r.id}
                    href={`/people/employees/${r.id}`}
                    className="rounded-xl border border-line px-3 py-1.5 text-sm font-bold hover:border-brand hover:text-brand"
                  >
                    {r.fullName}
                    <span className="ml-1.5 text-xs font-normal text-muted">{r.positionTitle}</span>
                  </Link>
                ))}
              </div>
            </div>
          ) : null}
          {overtime.length > 0 ? (
            <div className="a-card lg:col-span-2 !p-0">
              <h2 className="px-4 pt-4 font-black">Overtime</h2>
              <table className="a-table">
                <thead>
                  <tr>
                    <th>Date</th>
                    <th>Window</th>
                    <th className="text-right">Hours</th>
                    <th>Asked by</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {overtime.map((o) => (
                    <tr key={o.id}>
                      <td className="tabular-nums">{o.date}</td>
                      <td className="text-sm">
                        {shortTime(o.startTime)}–{shortTime(o.endTime)}
                      </td>
                      <td className="text-right tabular-nums">{o.hours.toFixed(2)}</td>
                      <td className="text-sm">{o.source === 'COMPANY' ? 'The studio' : 'Themselves'}</td>
                      <td>
                        <StatusBadge status={o.status} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </div>
      ) : null}

      {tab === 'schedule' ? (
        <div className="a-card !p-0">
          <div className="flex items-center justify-between px-4 py-3">
            <div>
              <h2 className="font-black">Weekly pattern</h2>
              <p className="text-xs text-muted">
                Re-rostering is a new row with a later start date, never an edit — so past
                attendance can always be explained against the pattern that was in force.
              </p>
            </div>
            {can('hris.manage') ? (
              <button className="a-btn !px-3 !py-1.5 text-xs" onClick={() => setAssigning(true)}>
                <Plus size={13} className="mr-1 inline" /> Assign a day
              </button>
            ) : null}
          </div>
          <table className="a-table">
            <thead>
              <tr>
                <th>Day</th>
                <th>Shift</th>
                <th>In force from</th>
                <th>Until</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {schedule.map((row) => (
                <tr key={row.id}>
                  <td className="font-bold">{row.dayName}</td>
                  <td className="text-sm">
                    {row.shiftName ?? <span className="text-muted">Rest day</span>}
                  </td>
                  <td className="text-sm tabular-nums">{row.effectiveFrom}</td>
                  <td className="text-sm tabular-nums">
                    {row.effectiveTo ?? <span className="text-muted">open-ended</span>}
                  </td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        {
                          label: 'Remove',
                          icon: Trash2,
                          tone: 'danger' as const,
                          disabled: !can('hris.manage'),
                          onClick: () => {
                            if (confirm(`Remove the ${row.dayName} pattern row?`)) removeRow.mutate(row.id);
                          },
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
              {schedule.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-sm text-muted">
                    No pattern yet — this person shows as unscheduled on the roster.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'attendance' ? (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>In</th>
                <th>Out</th>
                <th className="text-right">Hours</th>
                <th className="text-right">Overtime</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {attendance.map((a) => (
                <tr key={a.id}>
                  <td className="tabular-nums">{a.date}</td>
                  <td className="text-sm">
                    {clockLabel(a.clockIn)}
                    {a.isLate ? (
                      <span className="ml-1.5 text-xs font-bold text-danger">+{a.lateMinutes}m</span>
                    ) : null}
                  </td>
                  <td className="text-sm">{clockLabel(a.clockOut)}</td>
                  <td className="text-right tabular-nums">{a.workHours.toFixed(2)}</td>
                  <td className="text-right tabular-nums">
                    {a.overtimeHours > 0 ? a.overtimeHours.toFixed(2) : '—'}
                  </td>
                  <td>
                    <StatusBadge status={a.status} />
                  </td>
                </tr>
              ))}
              {attendance.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-sm text-muted">
                    Nothing on the timesheet yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'leave' ? (
        <div className="grid gap-3">
          <div className="a-card">
            <h2 className="mb-3 font-black">Allowance for {balance.year}</h2>
            <div className="grid gap-3 sm:grid-cols-4">
              <Detail label="Annual total" value={balance.annualTotal.toFixed(1)} />
              <Detail label="Annual used" value={balance.annualUsed.toFixed(1)} />
              <Detail label="Sick" value={balance.sickUsed.toFixed(1)} />
              <Detail label="Unpaid" value={balance.unpaidUsed.toFixed(1)} />
            </div>
          </div>
          <div className="a-card !p-0">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Type</th>
                  <th>From</th>
                  <th>To</th>
                  <th className="text-right">Days</th>
                  <th>Reason</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {leaves.map((l) => (
                  <tr key={l.id}>
                    <td className="font-bold">{l.type.replaceAll('_', ' ')}</td>
                    <td className="tabular-nums">{l.startDate}</td>
                    <td className="tabular-nums">{l.endDate}</td>
                    <td className="text-right tabular-nums">{l.totalDays.toFixed(1)}</td>
                    <td className="max-w-xs truncate text-sm text-muted">{l.reason}</td>
                    <td>
                      <StatusBadge status={l.status} />
                    </td>
                  </tr>
                ))}
                {leaves.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      No leave on file.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {editing ? (
        <EmployeeModal
          employee={employee}
          onClose={() => setEditing(false)}
          onDone={() => {
            setEditing(false);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
      {assigning ? (
        <AssignShiftModal
          employeeId={id}
          onClose={() => setAssigning(false)}
          onDone={() => {
            setAssigning(false);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string | null | undefined }) {
  return (
    <div className="mb-2 last:mb-0">
      <p className="text-[11px] font-bold uppercase tracking-wider text-muted">{label}</p>
      <p className="text-sm">{value || <span className="text-muted">—</span>}</p>
    </div>
  );
}

function AssignShiftModal({
  employeeId,
  onClose,
  onDone,
}: {
  employeeId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [dayOfWeek, setDayOfWeek] = useState('1');
  const [shiftId, setShiftId] = useState('');
  const [effectiveFrom, setEffectiveFrom] = useState(new Date().toISOString().slice(0, 10));

  const { data: shifts } = useQuery({
    queryKey: ['hris', 'shifts'],
    queryFn: () => api.admin.hris.shifts.list(),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.hris.employees.assignShift(employeeId, {
        dayOfWeek: Number(dayOfWeek),
        shiftId: shiftId || null,
        effectiveFrom,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That day did not save.'),
  });

  return (
    <Modal title="Assign a day" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Weekday">
          <SearchSelect
            value={dayOfWeek}
            onChange={setDayOfWeek}
            placeholder="Search day…"
            options={WEEKDAYS.slice(1).map((name, i) => ({ value: String(i + 1), label: name }))}
          />
        </Field>
        <Field label="Shift" hint="Leaving this empty makes the day an explicit rest day.">
          <SearchSelect
            value={shiftId}
            onChange={setShiftId}
            allowEmpty
            emptyLabel="Rest day"
            placeholder="Search shift…"
            options={(shifts ?? []).map((s) => ({
              value: s.id,
              label: s.name,
              hint: `${shortTime(s.startTime)}–${shortTime(s.endTime)}`,
            }))}
          />
        </Field>
        <Field label="In force from">
          <input
            type="date"
            className="a-input"
            value={effectiveFrom}
            onChange={(e) => setEffectiveFrom(e.target.value)}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Assign'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
