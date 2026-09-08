'use client';

import type { StaffRosterEntryView } from '@nuhabit/contracts';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CalendarOff, LogIn, LogOut, UserX } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../components/ui';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';

/** Today, in the studio's own terms rather than the browser's. */
function todayISO() {
  return new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
}

function clockLabel(iso: string | null) {
  if (!iso) return '—';
  return new Date(iso).toLocaleTimeString('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Asia/Jakarta',
  });
}

export default function PeoplePage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [date, setDate] = useState(todayISO());
  const [branch, setBranch] = useState('');
  const [query, setQuery] = useState('');
  const [error, setError] = useState<string | null>(null);

  const { data: overview } = useQuery({
    queryKey: ['hris', 'overview'],
    queryFn: api.admin.hris.overview,
  });
  const {
    data: roster,
    isLoading,
    error: rosterError,
  } = useQuery({
    queryKey: ['hris', 'roster', date, branch],
    queryFn: () => api.admin.hris.roster({ date, branchId: branch || undefined }),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const punch = useMutation({
    mutationFn: ({ employeeId, out }: { employeeId: string; out: boolean }) =>
      out
        ? api.admin.hris.attendance.clockOut({ employeeId })
        : api.admin.hris.attendance.clockIn({ employeeId }),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['hris'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That punch did not go through.'),
  });

  const mark = useMutation({
    mutationFn: ({ employeeId, status }: { employeeId: string; status: string }) =>
      api.admin.hris.attendance.mark({ employeeId, date, status }),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['hris'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That change did not save.'),
  });

  const q = query.trim().toLowerCase();
  const rows = (roster ?? []).filter(
    (r) =>
      !q ||
      r.employeeName.toLowerCase().includes(q) ||
      r.employeeNumber.toLowerCase().includes(q),
  );
  const isToday = date === todayISO();

  return (
    <div>
      <PageTitle
        title="People"
        subtitle="Who is in today, and who is not"
        actions={
          <Link href="/people/employees" className="a-btn-ghost">
            Staff directory
          </Link>
        }
      />
      <ErrorNote message={error} />
      <QueryError error={rosterError} />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <StatCard tone="ink"
          label="On the payroll"
          value={overview?.activeEmployees ?? '—'}
          hint={overview ? `${overview.totalEmployees} records in total` : undefined}
        />
        <StatCard label="Expected today" value={overview?.expectedToday ?? '—'} tone="brand" />
        <StatCard
          label="Late arrivals"
          value={overview?.today.late ?? '—'}
          hint={overview ? `${overview.today.lateMinutes} minutes in total` : undefined}
          tone={overview && overview.today.late > 0 ? 'danger' : undefined}
        />
        <StatCard tone="warn" label="On leave" value={overview?.onLeaveToday ?? '—'} />
        <StatCard
          label="Waiting on you"
          value={(overview?.pendingLeave ?? 0) + (overview?.pendingOvertime ?? 0)}
          hint={
            overview
              ? `${overview.pendingLeave} leave, ${overview.pendingOvertime} overtime`
              : undefined
          }
        />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <input
          type="date"
          className="a-input max-w-[11rem]"
          value={date}
          onChange={(e) => setDate(e.target.value)}
        />
        <input
          className="a-input max-w-xs"
          placeholder="Search name or number…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
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
        <Spinner label="Reading the roster…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Employee</th>
                  <th>Shift</th>
                  <th>In</th>
                  <th>Out</th>
                  <th className="text-right">Hours</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((entry) => (
                  <RosterRow
                    key={entry.employeeId}
                    entry={entry}
                    canPunch={can('hris.attendance') && isToday}
                    canMark={can('hris.attendance')}
                    onPunch={(out) => punch.mutate({ employeeId: entry.employeeId, out })}
                    onMark={(status) => mark.mutate({ employeeId: entry.employeeId, status })}
                  />
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      Nobody is rostered for this day.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

function RosterRow({
  entry,
  canPunch,
  canMark,
  onPunch,
  onMark,
}: {
  entry: StaffRosterEntryView;
  canPunch: boolean;
  canMark: boolean;
  onPunch: (out: boolean) => void;
  onMark: (status: string) => void;
}) {
  const day = entry.attendance;
  const rostered = entry.status !== 'REST_DAY' && entry.status !== 'UNSCHEDULED';

  return (
    <tr>
      <td>
        <Link href={`/people/employees/${entry.employeeId}`} className="font-bold hover:text-brand">
          {entry.employeeName}
        </Link>
        <p className="text-xs text-muted">{entry.employeeNumber}</p>
      </td>
      <td className="text-sm">
        {entry.shiftName ?? <span className="text-muted">—</span>}
        {entry.scheduledStart && entry.scheduledEnd ? (
          <p className="text-xs text-muted">
            {clockLabel(entry.scheduledStart)}–{clockLabel(entry.scheduledEnd)}
          </p>
        ) : null}
      </td>
      <td className="text-sm">
        {clockLabel(day?.clockIn ?? null)}
        {day?.isLate ? (
          <p className="text-xs font-bold text-danger">{day.lateMinutes} min late</p>
        ) : null}
      </td>
      <td className="text-sm">{clockLabel(day?.clockOut ?? null)}</td>
      <td className="text-right text-sm tabular-nums">{day ? day.workHours.toFixed(2) : '—'}</td>
      <td>
        <StatusBadge status={entry.status} />
        {entry.leaveType ? (
          <p className="mt-0.5 text-xs text-muted">{entry.leaveType.toLowerCase()}</p>
        ) : null}
        {entry.holiday ? <p className="mt-0.5 text-xs text-muted">{entry.holiday.name}</p> : null}
      </td>
      <td className="text-right">
        <RowActions
          items={[
            {
              label: 'Clock in',
              icon: LogIn,
              disabled: !canPunch || !!day?.clockIn,
              onClick: () => onPunch(false),
            },
            {
              label: 'Clock out',
              icon: LogOut,
              disabled: !canPunch || !day?.clockIn || !!day?.clockOut,
              onClick: () => onPunch(true),
            },
            {
              label: 'Mark absent',
              icon: UserX,
              tone: 'danger' as const,
              disabled: !canMark || !rostered,
              onClick: () => onMark('ABSENT'),
            },
            {
              label: 'Mark on leave',
              icon: CalendarOff,
              disabled: !canMark || !rostered,
              onClick: () => onMark('ON_LEAVE'),
            },
          ]}
        />
      </td>
    </tr>
  );
}
