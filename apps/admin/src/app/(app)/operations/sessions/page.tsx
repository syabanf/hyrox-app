'use client';

import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle, Pager, SearchSelect, StatCard } from '../../../../components/ui';

export default function SessionsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branchId, setBranchId] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [showPast, setShowPast] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');
  const [page, setPage] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data: sessions, isLoading } = useQuery({
    queryKey: ['admin-sessions', branchId],
    queryFn: () => api.admin.sessions.list(branchId ? { branchId } : undefined),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.sessions.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  const visible = (sessions ?? [])
    .filter((v) => showPast || new Date(v.session.endsAt).getTime() > Date.now() - 3600_000)
    .filter((v) => !statusFilter || v.session.status === statusFilter);
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
            <button className="a-btn" onClick={() => setCreateOpen(true)}>
              + New session
            </button>
          ) : undefined
        }
      />
      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Upcoming"
          value={(sessions ?? []).filter((v) => new Date(v.session.startsAt).getTime() > Date.now() && ['PUBLISHED', 'FULL', 'DRAFT'].includes(v.session.status)).length}
        />
        <StatCard label="Published" value={(sessions ?? []).filter((v) => v.session.status === 'PUBLISHED').length} />
        <StatCard label="Full" value={(sessions ?? []).filter((v) => v.session.status === 'FULL').length} />
        <StatCard label="Draft" value={(sessions ?? []).filter((v) => v.session.status === 'DRAFT').length} />
      </div>
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <div className="w-44">
          <SearchSelect
            value={branchId}
            onChange={(v) => {
              setBranchId(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
        <div className="w-40">
          <SearchSelect
            value={statusFilter}
            onChange={(v) => {
              setStatusFilter(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All statuses"
            placeholder="Search status…"
            options={[...new Set((sessions ?? []).map((v) => v.session.status))].map((s) => ({ value: s, label: s }))}
          />
        </div>
        <label className="flex items-center gap-2 text-sm text-muted">
          <input type="checkbox" checked={showPast} onChange={(e) => setShowPast(e.target.checked)} />
          Show past sessions
        </label>
      </div>
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
                    {can('sessions.manage') && v.confirmedCount === 0 && v.waitlistCount === 0 ? (
                      <button
                        className="text-xs font-bold text-muted hover:text-danger"
                        onClick={() => {
                          if (confirm('Delete this session? Only possible while nobody is booked.'))
                            remove.mutate(v.session.id);
                        }}
                      >
                        Delete
                      </button>
                    ) : null}
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

function CreateSessionModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { data: classTypes } = useQuery({ queryKey: ['class-types'], queryFn: api.admin.classTypes.list });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data: coaches } = useQuery({ queryKey: ['coaches'], queryFn: api.admin.coaches.list });
  const [classTypeId, setClassTypeId] = useState('');
  const [branchId, setBranchId] = useState('');
  const [coachId, setCoachId] = useState('');
  const [startsAt, setStartsAt] = useState('');
  const [capacity, setCapacity] = useState('');
  const [publish, setPublish] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      api.admin.sessions.create({
        classTypeId,
        branchId,
        coachId,
        startsAt: new Date(startsAt).toISOString(),
        capacity: capacity ? Number(capacity) : undefined,
        area: null,
        publish,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Create failed.'),
  });

  return (
    <Modal title="New class session" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Class type</label>
          <SearchSelect
            value={classTypeId}
            onChange={setClassTypeId}
            placeholder="Search class type…"
            options={(classTypes ?? []).map((t) => ({
              value: t.id,
              label: t.name,
              hint: `${t.defaultCreditCost} cr · cap ${t.defaultCapacity}`,
            }))}
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Branch</label>
            <SearchSelect
              value={branchId}
              onChange={setBranchId}
              placeholder="Search branch…"
              options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
            />
          </div>
          <div>
            <label className="a-label">Coach</label>
            <SearchSelect
              value={coachId}
              onChange={setCoachId}
              placeholder="Search coach…"
              options={(coaches ?? [])
                .filter((c) => !branchId || c.branchId === branchId)
                .map((c) => ({ value: c.id, label: c.name, hint: c.specialization }))}
            />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Starts at</label>
            <input
              type="datetime-local"
              className="a-input"
              value={startsAt}
              onChange={(e) => setStartsAt(e.target.value)}
            />
          </div>
          <div>
            <label className="a-label">Capacity (blank = type default)</label>
            <input className="a-input" value={capacity} onChange={(e) => setCapacity(e.target.value)} />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={publish} onChange={(e) => setPublish(e.target.checked)} />
          Publish immediately (bookable)
        </label>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || !classTypeId || !branchId || !coachId || !startsAt}
          onClick={() => mutation.mutate()}
        >
          Create session
        </button>
      </div>
    </Modal>
  );
}
