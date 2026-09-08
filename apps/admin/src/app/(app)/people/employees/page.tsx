'use client';

import type { EmployeeView } from '@nuhabit/contracts';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Eye, Pencil } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import { EmployeeModal } from '../../../../components/people';
import {
  Pager,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { api } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

const PAGE_SIZE = 20;

export default function EmployeesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [query, setQuery] = useState('');
  const [department, setDepartment] = useState('');
  const [branch, setBranch] = useState('');
  const [activeOnly, setActiveOnly] = useState(true);
  const [page, setPage] = useState(0);
  const [editing, setEditing] = useState<EmployeeView | 'new' | null>(null);

  const { data: employees, isLoading, error } = useQuery({
    queryKey: ['hris', 'employees', query, department, branch, activeOnly],
    queryFn: () =>
      api.admin.hris.employees.list({
        query: query || undefined,
        departmentId: department || undefined,
        branchId: branch || undefined,
        activeOnly: activeOnly ? 'true' : undefined,
      }),
  });
  const { data: departments } = useQuery({
    queryKey: ['hris', 'departments'],
    queryFn: api.admin.hris.departments.list,
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const rows = employees ?? [];
  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const shown = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  return (
    <div>
      <PageTitle
        title="Staff directory"
        subtitle="Everybody on the payroll"
        actions={
          can('hris.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New employee
            </button>
          ) : undefined
        }
      />

      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink" label="Records" value={rows.length} />
        <StatCard label="Active" value={rows.filter((e) => e.active).length} tone="brand" />
        <StatCard
          tone="warn"
          label="Departments"
          value={new Set(rows.map((e) => e.departmentId).filter(Boolean)).size}
        />
        <StatCard tone="info" label="Coaching staff" value={rows.filter((e) => e.coachId).length} />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search name, number or email…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-48">
          <SearchSelect
            value={department}
            onChange={(v) => {
              setDepartment(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All departments"
            placeholder="Search department…"
            options={(departments ?? []).map((d) => ({ value: d.id, label: d.name, hint: d.code }))}
          />
        </div>
        <div className="w-44">
          <SearchSelect
            value={branch}
            onChange={(v) => {
              setBranch(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={activeOnly}
            onChange={(e) => setActiveOnly(e.target.checked)}
          />
          Current staff only
        </label>
      </div>

      {isLoading ? (
        <Spinner label="Loading the directory…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Employee</th>
                  <th>Position</th>
                  <th>Department</th>
                  <th>Branch</th>
                  <th>Joined</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((e) => (
                  <tr key={e.id}>
                    <td>
                      <Link href={`/people/employees/${e.id}`} className="font-bold hover:text-brand">
                        {e.fullName}
                      </Link>
                      <p className="text-xs text-muted">
                        {e.employeeNumber}
                        {e.coachName ? ` · coaches as ${e.coachName}` : ''}
                      </p>
                    </td>
                    <td className="text-sm">{e.positionTitle ?? <span className="text-muted">—</span>}</td>
                    <td className="text-sm">{e.departmentName ?? <span className="text-muted">—</span>}</td>
                    <td className="text-sm">{e.branchName ?? <span className="text-muted">—</span>}</td>
                    <td className="text-sm tabular-nums">{e.joinDate}</td>
                    <td>
                      <StatusBadge status={e.active ? e.employmentStatusCode : 'INACTIVE'} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Open',
                            icon: Eye,
                            onClick: () => {
                              location.href = `/people/employees/${e.id}`;
                            },
                          },
                          {
                            label: 'Edit',
                            icon: Pencil,
                            disabled: !can('hris.manage'),
                            onClick: () => setEditing(e),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      No employees match those filters.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}

      {editing ? (
        <EmployeeModal
          employee={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['hris'] });
          }}
        />
      ) : null}
    </div>
  );
}
