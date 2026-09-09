'use client';

import { Spinner, StatusBadge, formatDayTime } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { Eye, Trash2 } from 'lucide-react';
import { CreateSessionModal } from '../../../../components/create-session';
import { ErrorNote, PageTitle, Pager, RowActions, StatCard } from '../../../../components/ui';
import { FilterBar, FilterSelect, useFilters } from '../../../../components/filters';

const SESSION_FILTERS = { branchId: '', coachId: '', classTypeId: '', status: '', q: '', past: '' };

/** The class types actually present in the loaded window, named. */
function classTypeOptions(sessions: { session: { classTypeId: string }; classTypeName: string }[] | undefined) {
  const seen = new Map<string, string>();
  for (const v of sessions ?? []) seen.set(v.session.classTypeId, v.classTypeName);
  return [...seen].map(([value, label]) => ({ value, label }));
}

export default function SessionsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const { filters, set, setMany, clear, dirty } = useFilters(SESSION_FILTERS);
  const { branchId, coachId, classTypeId, status: statusFilter, q: query } = filters;
  const showPast = filters.past === '1';
  // The create modal opens either empty, or already set to a coach — from the
  // Coaches page, or from the coach this list is filtered to.
  const [createFor, setCreateFor] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [page, setPage] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data: coaches } = useQuery({ queryKey: ['coaches'], queryFn: api.admin.coaches.list });
  const { data: sessions, isLoading } = useQuery({
    queryKey: ['admin-sessions', branchId, coachId],
    queryFn: () =>
      api.admin.sessions.list({
        ...(branchId ? { branchId } : {}),
        ...(coachId ? { coachId } : {}),
      }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.sessions.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  const q = query.trim().toLowerCase();
  const visible = (sessions ?? [])
    .filter((v) => showPast || new Date(v.session.endsAt).getTime() > Date.now() - 3600_000)
    .filter((v) => !statusFilter || v.session.status === statusFilter)
    .filter((v) => !classTypeId || v.session.classTypeId === classTypeId)
    .filter(
      (v) => !q || v.classTypeName.toLowerCase().includes(q) || v.coachName.toLowerCase().includes(q),
    );
  const pageCount = Math.max(1, Math.ceil(visible.length / 10));
  const safePage = Math.min(page, pageCount - 1);
  const paged = visible.slice(safePage * 10, safePage * 10 + 10);

  return (
    <div>
      <PageTitle
        title="Class Sessions"
        subtitle="Scheduled occurrences (class type ≠ session)"
        actions={
          can('sessions.manage') ? (
            <button
              className="a-btn"
              onClick={() => {
                setCreateFor(coachId || null);
                setCreateOpen(true);
              }}
            >
              + New session
            </button>
          ) : undefined
        }
      />
      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {/* Each card filters the table to what it counts. */}
        <StatCard
          tone="ink"
          label="Upcoming"
          value={(sessions ?? []).filter((v) => new Date(v.session.startsAt).getTime() > Date.now() && ['PUBLISHED', 'FULL', 'DRAFT'].includes(v.session.status)).length}
          active={statusFilter === '' && !showPast}
          onClick={() => {
            setMany({ status: '', past: '' });
            setPage(0);
          }}
        />
        {(['PUBLISHED', 'FULL', 'DRAFT'] as const).map((status) => (
          <StatCard
            tone="ok"
            key={status}
            label={status === 'DRAFT' ? 'Draft' : status === 'FULL' ? 'Full' : 'Published'}
            value={(sessions ?? []).filter((v) => v.session.status === status).length}
            active={statusFilter === status}
            onClick={() => {
              set('status', statusFilter === status ? '' : status);
              setPage(0);
            }}
          />
        ))}
      </div>
      <FilterBar
        dirty={dirty}
        onClear={clear}
        chips={[
          ...(branchId
            ? [{ key: 'branchId', label: (branches ?? []).find((b) => b.id === branchId)?.name ?? branchId, onRemove: () => set('branchId', '') }]
            : []),
          ...(coachId
            ? [{ key: 'coachId', label: (coaches ?? []).find((c) => c.id === coachId)?.name ?? coachId, onRemove: () => set('coachId', '') }]
            : []),
          ...(classTypeId
            ? [{
                key: 'classTypeId',
                label:
                  (sessions ?? []).find((v) => v.session.classTypeId === classTypeId)?.classTypeName ?? classTypeId,
                onRemove: () => set('classTypeId', ''),
              }]
            : []),
          ...(statusFilter ? [{ key: 'status', label: statusFilter, onRemove: () => set('status', '') }] : []),
          ...(showPast ? [{ key: 'past', label: 'Including past', onRemove: () => set('past', '') }] : []),
          ...(query ? [{ key: 'q', label: `"${query}"`, onRemove: () => set('q', '') }] : []),
        ]}
      >
        <input
          className="a-input max-w-xs"
          placeholder="Search class or coach…"
          value={query}
          onChange={(e) => {
            set('q', e.target.value);
            setPage(0);
          }}
        />
        <FilterSelect
          value={branchId}
          onChange={(v) => {
            set('branchId', v);
            setPage(0);
          }}
          emptyLabel="All branches"
          options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
        />
        <FilterSelect
          value={coachId}
          onChange={(v) => {
            set('coachId', v);
            setPage(0);
          }}
          emptyLabel="All coaches"
          options={(coaches ?? [])
            .filter((c) => !branchId || c.branchId === branchId)
            .map((c) => ({ value: c.id, label: c.name, hint: c.specialization }))}
        />
        <FilterSelect
          value={classTypeId}
          onChange={(v) => {
            set('classTypeId', v);
            setPage(0);
          }}
          emptyLabel="All class types"
          options={classTypeOptions(sessions)}
        />
        <FilterSelect
          value={statusFilter}
          onChange={(v) => {
            set('status', v);
            setPage(0);
          }}
          emptyLabel="All statuses"
          width="w-40"
          options={[...new Set((sessions ?? []).map((v) => v.session.status))].map((s) => ({ value: s, label: s }))}
        />
        <label className="flex items-center gap-2 text-sm text-muted">
          <input type="checkbox" checked={showPast} onChange={(e) => set('past', e.target.checked ? '1' : '')} />
          Show past sessions
        </label>
      </FilterBar>
      <ErrorNote message={error} />
      {isLoading ? (
        <Spinner label="Loading sessions…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Class</th>
                <th>Branch</th>
                <th>Coach</th>
                <th>Cost</th>
                <th>Booked</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {paged.map((v) => (
                <tr key={v.session.id}>
                  <td className="whitespace-nowrap font-bold">{formatDayTime(v.session.startsAt)}</td>
                  <td>
                    <Link href={`/operations/sessions/${v.session.id}`} className="font-bold hover:text-brand">
                      {v.classTypeName}
                    </Link>
                  </td>
                  <td>{v.branchName}</td>
                  <td>{v.coachName}</td>
                  <td>{v.session.creditCost} cr</td>
                  <td>
                    {v.confirmedCount}/{v.session.capacity}
                    {v.waitlistCount > 0 ? <span className="text-warn"> +{v.waitlistCount} WL</span> : null}
                  </td>
                  <td>
                    <StatusBadge status={v.session.status} />
                  </td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        {
                          label: 'Open roster',
                          icon: Eye,
                          onClick: () => (location.href = `/admin/operations/sessions/${v.session.id}`),
                        },
                        ...(can('sessions.manage') && v.confirmedCount === 0 && v.waitlistCount === 0
                          ? [
                              {
                                label: 'Delete',
                                icon: Trash2,
                                tone: 'danger' as const,
                                onClick: () => {
                                  if (confirm('Delete this session? Only possible while nobody is booked.'))
                                    remove.mutate(v.session.id);
                                },
                              },
                            ]
                          : []),
                      ]}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pager page={safePage} pageCount={pageCount} onPage={setPage} />
        </div>
      )}
      {createOpen ? (
        <CreateSessionModal
          coachId={createFor}
          onClose={() => setCreateOpen(false)}
          onDone={() => {
            setCreateOpen(false);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}
