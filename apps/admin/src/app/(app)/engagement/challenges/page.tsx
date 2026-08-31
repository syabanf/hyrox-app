'use client';

import type { Challenge } from '@hyrox/domain';
import { Spinner, formatDay } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle, SearchSelect, StatCard } from '../../../../components/ui';

const toLocalInput = (iso: string | null): string => {
  if (!iso) return '';
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

export default function ChallengesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<Challenge | 'new' | null>(null);
  const [typeView, setTypeView] = useState('');
  const [query, setQuery] = useState('');
  const [error, setError] = useState<string | null>(null);
  const { data, isLoading } = useQuery({
    queryKey: ['admin-challenges'],
    queryFn: api.admin.challenges.list,
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.challenges.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  if (isLoading) return <Spinner label="Loading challenges…" />;
  const rows = data ?? [];
  const now = Date.now();
  const live = rows.filter(
    (r) =>
      new Date(r.challenge.startsAt).getTime() <= now &&
      new Date(r.challenge.endsAt).getTime() >= now,
  );
  const manage = can('campaigns.manage');
  const visible = rows
    .filter((r) => !typeView || r.challenge.type === typeView)
    .filter((r) => !query || r.challenge.name.toLowerCase().includes(query.toLowerCase()));

  return (
    <div>
      <PageTitle
        title="Challenges"
        subtitle="Community distance goals shown in the member Train tab"
        actions={
          manage ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New challenge
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Challenges" value={rows.length} />
        <StatCard label="Live now" value={live.length} />
        <StatCard
          label="Participants"
          value={rows.reduce((sum, r) => sum + r.participantCount, 0)}
          hint="Joins across all challenges"
        />
      </div>
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search challenge…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="w-44">
          <SearchSelect
            value={typeView}
            onChange={setTypeView}
            allowEmpty
            emptyLabel="All types"
            placeholder="Search type…"
            options={['ANY', 'RUN', 'RIDE', 'WALK'].map((t) => ({ value: t, label: t }))}
          />
        </div>
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        {visible.map(({ challenge: c, participantCount }) => {
          const activeNow =
            new Date(c.startsAt).getTime() <= now && new Date(c.endsAt).getTime() >= now;
          return (
            <div key={c.id} className="a-card">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-black">{c.name}</p>
                  <p className="text-sm text-muted">{c.description}</p>
                </div>
                <span className={`chip shrink-0 ${activeNow ? 'bg-ok/10 text-ok' : 'bg-surface-raised text-muted'}`}>
                  {activeNow ? 'Live' : 'Off-window'}
                </span>
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5 text-xs">
                <span className="chip bg-surface-raised text-muted">{c.type}</span>
                <span className="chip bg-surface-raised text-muted">{c.targetKm} km target</span>
                <span className="chip bg-surface-raised text-muted">
                  {formatDay(c.startsAt)} – {formatDay(c.endsAt)}
                </span>
                <span className="chip bg-brand/10 text-brand">{participantCount} joined</span>
              </div>
              {manage ? (
                <div className="mt-3 flex gap-2">
                  <button className="a-btn-ghost flex-1 !py-1.5 text-xs" onClick={() => setEditing(c)}>
                    Edit
                  </button>
                  <button
                    className="a-btn-danger !py-1.5 text-xs"
                    onClick={() => {
                      if (confirm(`Delete challenge "${c.name}"? Joins are removed too.`))
                        remove.mutate(c.id);
                    }}
                  >
                    Delete
                  </button>
                </div>
              ) : null}
            </div>
          );
        })}
        {visible.length === 0 ? (
          <p className="a-card text-sm text-muted">No challenges match.</p>
        ) : null}
      </div>
      {editing ? (
        <ChallengeModal
          challenge={editing === 'new' ? null : editing}
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

function ChallengeModal({
  challenge,
  onClose,
  onDone,
}: {
  challenge: Challenge | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(challenge?.name ?? '');
  const [description, setDescription] = useState(challenge?.description ?? '');
  const [type, setType] = useState<Challenge['type']>(challenge?.type ?? 'ANY');
  const [targetKm, setTargetKm] = useState(String(challenge?.targetKm ?? 50));
  const [startsAt, setStartsAt] = useState(toLocalInput(challenge?.startsAt ?? null));
  const [endsAt, setEndsAt] = useState(toLocalInput(challenge?.endsAt ?? null));
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const body = {
        name,
        description,
        type,
        targetKm: Number(targetKm),
        startsAt: new Date(startsAt).toISOString(),
        endsAt: new Date(endsAt).toISOString(),
      };
      return challenge
        ? api.admin.challenges.update(challenge.id, body)
        : api.admin.challenges.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={challenge ? `Edit ${challenge.name}` : 'New challenge'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Monthly Run 50K" />
        </div>
        <div>
          <label className="a-label">Description</label>
          <input className="a-input" value={description} onChange={(e) => setDescription(e.target.value)} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Counts</label>
            <select className="a-input" value={type} onChange={(e) => setType(e.target.value as Challenge['type'])}>
              {['ANY', 'RUN', 'RIDE', 'WALK'].map((t) => (
                <option key={t}>{t}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="a-label">Target (km)</label>
            <input className="a-input" inputMode="decimal" value={targetKm} onChange={(e) => setTargetKm(e.target.value)} />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Starts</label>
            <input type="datetime-local" className="a-input" value={startsAt} onChange={(e) => setStartsAt(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Ends</label>
            <input type="datetime-local" className="a-input" value={endsAt} onChange={(e) => setEndsAt(e.target.value)} />
          </div>
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={
            mutation.isPending || name.length < 2 || !startsAt || !endsAt || Number(targetKm) <= 0
          }
          onClick={() => mutation.mutate()}
        >
          {challenge ? 'Save changes' : 'Create challenge'}
        </button>
      </div>
    </Modal>
  );
}
