'use client';

import type { Coach } from '@nuhabit/domain';
import { Spinner, StatusBadge, formatIdr } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { Banknote, CalendarPlus, Pencil, Trash2 } from 'lucide-react';
import { CreateSessionModal } from '../../../../components/create-session';
import { ErrorNote, Modal, PageTitle, RowActions, SearchSelect, StatCard } from '../../../../components/ui';

export default function CoachesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<Coach | 'new' | null>(null);
  // Scheduling a class for the coach whose card it was clicked from.
  const [schedulingFor, setSchedulingFor] = useState<Coach | null>(null);
  const [branchView, setBranchView] = useState('');
  const [query, setQuery] = useState('');
  const [error, setError] = useState<string | null>(null);
  const { data: coaches, isLoading } = useQuery({ queryKey: ['coaches'], queryFn: api.admin.coaches.list });
  // What each coach is paid, shown beside them. It needs the incentives grant,
  // so a Branch Manager sees the roster without the payroll.
  const { data: fees } = useQuery({
    queryKey: ['coach-fees'],
    queryFn: api.admin.incentives.coachFees,
    enabled: can('incentives.view'),
    retry: false,
  });
  const feeOf = (coachId: string) => (fees ?? []).find((f) => f.coachId === coachId);
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.coaches.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  if (isLoading) return <Spinner label="Loading coaches…" />;

  return (
    <div>
      <PageTitle
        title="Coaches"
        subtitle="Assignable to class sessions"
        actions={
          can('coaches.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New coach
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Coaches" value={(coaches ?? []).length} />
        <StatCard label="Active" value={(coaches ?? []).filter((c) => c.status === 'ACTIVE').length} />
        <StatCard label="Branches covered" value={[...new Set((coaches ?? []).map((c) => c.branchId))].length} />
      </div>
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search coach…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="w-44">
          <SearchSelect
            value={branchView}
            onChange={setBranchView}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {(coaches ?? [])
          .filter((c) => !branchView || c.branchId === branchView)
          .filter(
            (c) =>
              !query ||
              c.name.toLowerCase().includes(query.toLowerCase()) ||
              c.specialization.toLowerCase().includes(query.toLowerCase()),
          )
          .map((c) => (
          <div key={c.id} className="a-card">
            <div className="flex items-center justify-between">
              <p className="font-black">{c.name}</p>
              <StatusBadge status={c.status} />
            </div>
            <p className="text-sm text-brand">{c.specialization}</p>
            <p className="mt-1 text-sm text-muted">{c.bio}</p>
            {(() => {
              const fee = feeOf(c.id);
              if (!fee) return null;
              return (
                <p className="mt-2 flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs">
                  <span className="font-black text-brand">{formatIdr(fee.sessionFeeIdr)}</span>
                  <span className="text-muted">per class</span>
                  <span className="text-muted">
                    · {formatIdr(fee.perAttendeeIdr)} per attendee
                  </span>
                  <span
                    className={`rounded-full px-2 py-0.5 text-[10px] font-black uppercase ${
                      fee.ownScheme ? 'bg-brand/10 text-brand' : 'bg-line text-muted'
                    }`}
                  >
                    {fee.ownScheme ? 'Own fee' : 'Studio default'}
                  </span>
                  {fee.classRates > 0 ? (
                    <span className="text-muted">
                      · {fee.classRates} class rate{fee.classRates === 1 ? '' : 's'}
                    </span>
                  ) : null}
                </p>
              );
            })()}
            <div className="mt-2 flex items-center justify-between">
              <p className="text-xs font-bold uppercase tracking-wide text-muted">
                {(branches ?? []).find((b) => b.id === c.branchId)?.name ?? c.branchId}
              </p>
              {can('coaches.manage') ? (
                <RowActions
                  items={[
                    { label: 'Edit', icon: Pencil, onClick: () => setEditing(c) },
                    ...(can('incentives.manage')
                      ? [
                          {
                            label: 'Set fee',
                            icon: Banknote,
                            onClick: () => {
                              location.href = '/admin/operations/incentives';
                            },
                          },
                        ]
                      : []),
                    ...(can('sessions.manage')
                      ? [
                          {
                            label: 'Schedule a class',
                            icon: CalendarPlus,
                            onClick: () => setSchedulingFor(c),
                          },
                        ]
                      : []),
                    {
                      label: 'Delete',
                      icon: Trash2,
                      tone: 'danger' as const,
                      onClick: () => {
                        if (confirm(`Delete coach "${c.name}"?`)) remove.mutate(c.id);
                      },
                    },
                  ]}
                />
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {schedulingFor ? (
        <CreateSessionModal
          coachId={schedulingFor.id}
          onClose={() => setSchedulingFor(null)}
          onDone={() => {
            setSchedulingFor(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
      {editing ? (
        <CoachModal
          coach={editing === 'new' ? null : editing}
          branches={(branches ?? []).map((b) => ({ id: b.id, name: b.name }))}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}

function CoachModal({
  coach,
  branches,
  onClose,
  onDone,
}: {
  coach: Coach | null;
  branches: { id: string; name: string }[];
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(coach?.name ?? '');
  const [bio, setBio] = useState(coach?.bio ?? '');
  const [specialization, setSpecialization] = useState(coach?.specialization ?? '');
  const [branchId, setBranchId] = useState(coach?.branchId ?? branches[0]?.id ?? '');
  const [status, setStatus] = useState<'ACTIVE' | 'INACTIVE'>(coach?.status ?? 'ACTIVE');
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const body = { name, bio, specialization, branchId, status };
      return coach ? api.admin.coaches.update(coach.id, body) : api.admin.coaches.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={coach ? `Edit ${coach.name}` : 'New coach'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="a-label">Specialization</label>
          <input className="a-input" value={specialization} onChange={(e) => setSpecialization(e.target.value)} />
        </div>
        <div>
          <label className="a-label">Bio</label>
          <input className="a-input" value={bio} onChange={(e) => setBio(e.target.value)} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Branch</label>
            <SearchSelect
              value={branchId}
              onChange={setBranchId}
              placeholder="Search branch…"
              options={branches.map((b) => ({ value: b.id, label: b.name }))}
            />
          </div>
          <div>
            <label className="a-label">Status</label>
            <select className="a-input" value={status} onChange={(e) => setStatus(e.target.value as typeof status)}>
              <option>ACTIVE</option>
              <option>INACTIVE</option>
            </select>
          </div>
        </div>
        <ErrorNote message={error} />
        <button className="a-btn" disabled={mutation.isPending || name.length < 2 || !branchId} onClick={() => mutation.mutate()}>
          {coach ? 'Save changes' : 'Create coach'}
        </button>
      </div>
    </Modal>
  );
}
