'use client';

import type { HOLIDAY_TYPES, Shift } from '@nuhabit/domain';
import { shortTime } from '@nuhabit/domain';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Trash2 } from 'lucide-react';
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

export default function ShiftsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<Shift | 'new' | null>(null);
  const [addingHoliday, setAddingHoliday] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const year = new Date().getFullYear();

  const { data: shifts, isLoading } = useQuery({
    queryKey: ['hris', 'shifts'],
    queryFn: () => api.admin.hris.shifts.list(),
  });
  const { data: holidays } = useQuery({
    queryKey: ['hris', 'holidays', year],
    queryFn: () => api.admin.hris.holidays.list({ from: `${year}-01-01`, to: `${year}-12-31` }),
  });

  const removeHoliday = useMutation({
    mutationFn: (id: string) => api.admin.hris.holidays.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['hris'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That holiday did not delete.'),
  });

  if (isLoading) return <Spinner label="Loading shifts…" />;
  const rows = shifts ?? [];
  const calendar = holidays ?? [];

  return (
    <div>
      <PageTitle
        title="Shifts and calendar"
        subtitle="The windows people work, and the days the studio does not"
        actions={
          can('hris.manage') ? (
            <>
              <button className="a-btn-ghost" onClick={() => setAddingHoliday(true)}>
                + Holiday
              </button>
              <button className="a-btn" onClick={() => setEditing('new')}>
                + New shift
              </button>
            </>
          ) : undefined
        }
      />
      <ErrorNote message={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard tone="ink" label="Shifts" value={rows.length} hint={`${rows.filter((s) => s.active).length} active`} />
        <StatCard label="Holidays this year" value={calendar.length} tone="brand" />
        <StatCard
          label="Collective leave days"
          value={calendar.filter((h) => h.deductsLeave).length}
          hint="Drawn from the annual allowance"
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="a-card !p-0">
          <h2 className="px-4 pt-4 font-black">Shifts</h2>
          <p className="px-4 pb-2 text-xs text-muted">
            The tolerance decides whether an arrival counts as late; the minutes are still measured
            from the start time.
          </p>
          <table className="a-table">
            <thead>
              <tr>
                <th>Shift</th>
                <th>Window</th>
                <th className="text-right">Break</th>
                <th className="text-right">Grace</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => (
                <tr key={s.id}>
                  <td>
                    <p className="font-bold">{s.name}</p>
                    {!s.active ? <StatusBadge status="INACTIVE" /> : null}
                  </td>
                  <td className="text-sm tabular-nums">
                    {shortTime(s.startTime)}–{shortTime(s.endTime)}
                    {s.isOvernight ? <span className="ml-1 text-xs text-muted">+1d</span> : null}
                  </td>
                  <td className="text-right tabular-nums">{s.breakMinutes}m</td>
                  <td className="text-right tabular-nums">{s.lateToleranceMinutes}m</td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        {
                          label: 'Edit',
                          icon: Pencil,
                          disabled: !can('hris.manage'),
                          onClick: () => setEditing(s),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-sm text-muted">
                    No shifts yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <div className="a-card !p-0">
          <h2 className="px-4 pt-4 font-black">Public holidays {year}</h2>
          <p className="px-4 pb-2 text-xs text-muted">
            A national holiday inside a leave request is free. Collective leave (<em>cuti bersama</em>)
            comes out of the annual allowance — that is what the badge marks.
          </p>
          <table className="a-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>Holiday</th>
                <th>Kind</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {calendar.map((h) => (
                <tr key={h.id}>
                  <td className="tabular-nums">{h.date}</td>
                  <td>
                    <p className="font-bold">{h.name}</p>
                    {h.deductsLeave ? (
                      <p className="text-xs text-muted">Costs a leave day</p>
                    ) : (
                      <p className="text-xs text-muted">Free</p>
                    )}
                  </td>
                  <td>
                    <StatusBadge status={h.type} />
                  </td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        {
                          label: 'Delete',
                          icon: Trash2,
                          tone: 'danger' as const,
                          disabled: !can('hris.manage'),
                          onClick: () => {
                            if (confirm(`Remove "${h.name}" from the calendar?`)) removeHoliday.mutate(h.id);
                          },
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
              {calendar.length === 0 ? (
                <tr>
                  <td colSpan={4} className="py-8 text-center text-sm text-muted">
                    No holidays on the calendar for {year}.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>

      {editing ? (
        <ShiftModal
          shift={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
      {addingHoliday ? (
        <HolidayModal
          onClose={() => setAddingHoliday(false)}
          onDone={() => {
            setAddingHoliday(false);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
    </div>
  );
}

function ShiftModal({
  shift,
  onClose,
  onDone,
}: {
  shift: Shift | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState(shift?.name ?? '');
  const [startTime, setStartTime] = useState(shift ? shortTime(shift.startTime) : '08:00');
  const [endTime, setEndTime] = useState(shift ? shortTime(shift.endTime) : '16:00');
  const [breakMinutes, setBreakMinutes] = useState(shift?.breakMinutes ?? 60);
  const [tolerance, setTolerance] = useState(shift?.lateToleranceMinutes ?? 10);
  const [isOvernight, setIsOvernight] = useState(shift?.isOvernight ?? false);
  const [active, setActive] = useState(shift?.active ?? true);

  const save = useMutation({
    mutationFn: () => {
      const input = {
        name,
        startTime,
        endTime,
        breakMinutes,
        lateToleranceMinutes: tolerance,
        isOvernight,
        active,
        sortOrder: shift?.sortOrder ?? 0,
      };
      return shift ? api.admin.hris.shifts.update(shift.id, input) : api.admin.hris.shifts.create(input);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That shift did not save.'),
  });

  return (
    <Modal title={shift ? `Edit ${shift.name}` : 'New shift'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Name">
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Starts">
            <input type="time" className="a-input" value={startTime} onChange={(e) => setStartTime(e.target.value)} />
          </Field>
          <Field label="Ends">
            <input type="time" className="a-input" value={endTime} onChange={(e) => setEndTime(e.target.value)} />
          </Field>
          <Field label="Unpaid break" hint="Minutes, taken off the hours worked.">
            <input
              type="number"
              className="a-input"
              value={breakMinutes}
              onChange={(e) => setBreakMinutes(Number(e.target.value))}
            />
          </Field>
          <Field label="Grace period" hint="Minutes before an arrival counts as late.">
            <input
              type="number"
              className="a-input"
              value={tolerance}
              onChange={(e) => setTolerance(Number(e.target.value))}
            />
          </Field>
        </div>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input type="checkbox" checked={isOvernight} onChange={(e) => setIsOvernight(e.target.checked)} />
          Ends the following day
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
          Available to roster
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !name} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function HolidayModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10));
  const [name, setName] = useState('');
  const [type, setType] = useState<(typeof HOLIDAY_TYPES)[number]>('NATIONAL');
  const [deductsLeave, setDeductsLeave] = useState(false);

  const save = useMutation({
    mutationFn: () => api.admin.hris.holidays.create({ date, name, type, deductsLeave }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That holiday did not save.'),
  });

  return (
    <Modal title="Add a holiday" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Date">
          <input type="date" className="a-input" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
        <Field label="Name">
          <input
            className="a-input"
            placeholder="Hari Kemerdekaan RI"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
        <Field label="Kind">
          <SearchSelect
            value={type}
            onChange={(v) => {
              const next = v as (typeof HOLIDAY_TYPES)[number];
              setType(next);
              // Collective leave is the one kind that costs the employee a day.
              setDeductsLeave(next === 'COLLECTIVE');
            }}
            placeholder="Search kind…"
            options={[
              { value: 'NATIONAL', label: 'National holiday', hint: 'Free — costs nobody a leave day' },
              { value: 'COLLECTIVE', label: 'Collective leave', hint: 'Cuti bersama — comes out of the allowance' },
              { value: 'COMPANY', label: 'Studio closure' },
            ]}
          />
        </Field>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input type="checkbox" checked={deductsLeave} onChange={(e) => setDeductsLeave(e.target.checked)} />
          Comes out of the annual allowance
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !name} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Add'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
