'use client';

import type { ClassType } from '@hyrox/domain';
import { Spinner, StatusBadge } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { Pencil, Trash2 } from 'lucide-react';
import { ErrorNote, Modal, PageTitle, RowActions, StatCard } from '../../../../components/ui';

export default function ClassTypesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<ClassType | 'new' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const { data, isLoading } = useQuery({ queryKey: ['class-types'], queryFn: api.admin.classTypes.list });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.classTypes.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  return (
    <div>
      <PageTitle
        title="Class Types"
        subtitle="Reusable templates — sessions are scheduled from these"
        actions={
          can('class_types.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New class type
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Class types" value={(data ?? []).length} />
        <StatCard label="Active" value={(data ?? []).filter((t) => t.active).length} />
        <StatCard
          label="Avg credit cost"
          value={(data ?? []).length > 0 ? ((data ?? []).reduce((sum, t) => sum + t.defaultCreditCost, 0) / (data ?? []).length).toFixed(1) : '—'}
        />
      </div>
      <div className="mb-4">
        <input
          className="a-input max-w-xs"
          placeholder="Search class type…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>
      {isLoading ? (
        <Spinner label="Loading…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Duration</th>
                <th>Credit cost</th>
                <th>Capacity</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(data ?? [])
                .filter(
                  (t) =>
                    !query ||
                    t.name.toLowerCase().includes(query.toLowerCase()) ||
                    t.description.toLowerCase().includes(query.toLowerCase()),
                )
                .map((t) => (
                <tr key={t.id}>
                  <td>
                    <p className="font-bold">{t.name}</p>
                    <p className="max-w-md truncate text-xs text-muted">{t.description}</p>
                  </td>
                  <td>{t.defaultDurationMin} min</td>
                  <td className="font-bold text-brand">{t.defaultCreditCost}</td>
                  <td>{t.defaultCapacity}</td>
                  <td>
                    <StatusBadge status={t.active ? 'ACTIVE' : 'INACTIVE'} />
                  </td>
                  <td className="text-right">
                    {can('class_types.manage') ? (
                      <RowActions
                        items={[
                          { label: 'Edit', icon: Pencil, onClick: () => setEditing(t) },
                          {
                            label: 'Delete',
                            icon: Trash2,
                            tone: 'danger' as const,
                            onClick: () => {
                              if (confirm(`Delete class type "${t.name}"?`)) remove.mutate(t.id);
                            },
                          },
                        ]}
                      />
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing ? (
        <ClassTypeModal
          classType={editing === 'new' ? null : editing}
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

function ClassTypeModal({
  classType,
  onClose,
  onDone,
}: {
  classType: ClassType | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(classType?.name ?? '');
  const [description, setDescription] = useState(classType?.description ?? '');
  const [duration, setDuration] = useState(String(classType?.defaultDurationMin ?? 60));
  const [cost, setCost] = useState(String(classType?.defaultCreditCost ?? 1));
  const [capacity, setCapacity] = useState(String(classType?.defaultCapacity ?? 16));
  const [active, setActive] = useState(classType?.active ?? true);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const body = {
        name,
        description,
        defaultDurationMin: Number(duration),
        defaultCreditCost: Number(cost),
        defaultCapacity: Number(capacity),
        active,
      };
      return classType ? api.admin.classTypes.update(classType.id, body) : api.admin.classTypes.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={classType ? `Edit ${classType.name}` : 'New class type'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="a-label">Description</label>
          <input className="a-input" value={description} onChange={(e) => setDescription(e.target.value)} />
        </div>
        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className="a-label">Duration (min)</label>
            <input className="a-input" value={duration} onChange={(e) => setDuration(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Credit cost</label>
            <input className="a-input" value={cost} onChange={(e) => setCost(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Capacity</label>
            <input className="a-input" value={capacity} onChange={(e) => setCapacity(e.target.value)} />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
          Active (available for scheduling)
        </label>
        <ErrorNote message={error} />
        <button className="a-btn" disabled={mutation.isPending || name.length < 2} onClick={() => mutation.mutate()}>
          {classType ? 'Save changes' : 'Create class type'}
        </button>
      </div>
    </Modal>
  );
}
