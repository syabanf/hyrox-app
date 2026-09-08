'use client';

import type { UpsertBadgeInput } from '@nuhabit/contracts';
import { BADGE_METRIC_LABELS, BADGE_METRICS, type Badge, type BadgeMetric } from '@nuhabit/domain';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil } from 'lucide-react';
import { useState } from 'react';
import { ErrorNote, Field, Modal, PageTitle, QueryError, RowActions, SearchSelect, StatCard } from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * Badges: what a member has done, as opposed to what they have spent.
 *
 * Deliberately not tiers. A tier is a standing that moves both ways with
 * activity; a badge is a fact about the past that never becomes untrue.
 */
export default function BadgesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<Badge | 'new' | null>(null);

  const { data: badges, isLoading, error } = useQuery({
    queryKey: ['crm', 'badges'],
    queryFn: () => api.admin.crm.badges.list(),
  });

  const rows = badges ?? [];

  return (
    <div>
      <PageTitle
        title="Badges"
        subtitle="What a member has done, as opposed to what they have spent"
        actions={
          can('crm.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New badge
            </button>
          ) : null
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard tone="ink" label="Badges" value={rows.length} />
        <StatCard
          label="Earned automatically"
          value={rows.filter((b) => b.metric !== 'MANUAL').length}
          tone="brand"
        />
        <StatCard
          label="Given by hand"
          value={rows.filter((b) => b.metric === 'MANUAL').length}
          hint="For what no counter can see"
        />
      </div>

      {isLoading ? (
        <Spinner label="Loading badges…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Badge</th>
                <th>Earned by</th>
                <th className="text-right">Worth</th>
                <th>Status</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((badge) => (
                <tr key={badge.id}>
                  <td>
                    <p className="font-bold">{badge.name}</p>
                    <p className="text-xs text-muted">{badge.description || badge.code}</p>
                  </td>
                  <td className="text-sm">
                    {badge.metric === 'MANUAL' ? (
                      <span className="text-muted">a person decides</span>
                    ) : (
                      <>
                        {BADGE_METRIC_LABELS[badge.metric]}{' '}
                        <span className="font-bold">
                          {badge.metric === 'SPEND_IDR'
                            ? formatIdr(badge.threshold)
                            : badge.threshold}
                        </span>
                      </>
                    )}
                  </td>
                  <td className="text-right tabular-nums text-sm">
                    {badge.bonusXp > 0 ? `${badge.bonusXp} points` : '—'}
                  </td>
                  <td>
                    <StatusBadge status={badge.active ? 'ACTIVE' : 'INACTIVE'} />
                  </td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        {
                          label: 'Edit',
                          icon: Pencil,
                          disabled: !can('crm.manage'),
                          onClick: () => setEditing(badge),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-sm text-muted">
                    No badges yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      )}

      {editing ? (
        <BadgeModal
          badge={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['crm', 'badges'] });
          }}
        />
      ) : null}
    </div>
  );
}

function BadgeModal({
  badge,
  onClose,
  onDone,
}: {
  badge: Badge | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState<UpsertBadgeInput>(() => ({
    code: badge?.code ?? '',
    name: badge?.name ?? '',
    description: badge?.description ?? '',
    metric: badge?.metric ?? 'VISITS',
    threshold: badge?.threshold ?? 10,
    bonusXp: badge?.bonusXp ?? 0,
    sortOrder: badge?.sortOrder ?? 0,
    active: badge?.active ?? true,
  }));
  const set = <K extends keyof UpsertBadgeInput>(k: K, v: UpsertBadgeInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const save = useMutation({
    mutationFn: () => api.admin.crm.badges.save(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That badge did not save.'),
  });

  return (
    <Modal title={badge ? `Edit ${badge.name}` : 'New badge'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Code">
            <input
              className="a-input font-mono"
              value={form.code}
              onChange={(e) => set('code', e.target.value)}
            />
          </Field>
          <Field label="Name">
            <input className="a-input" value={form.name} onChange={(e) => set('name', e.target.value)} />
          </Field>
        </div>
        <Field label="Description" hint="What the member sees when they earn it.">
          <input
            className="a-input"
            value={form.description ?? ''}
            onChange={(e) => set('description', e.target.value)}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Earned by">
            <SearchSelect
              value={form.metric}
              onChange={(v) => set('metric', v as BadgeMetric)}
              placeholder="Search…"
              options={[
                ...BADGE_METRICS.map((metric) => ({ value: metric, label: BADGE_METRIC_LABELS[metric] })),
              ]}
            />
          </Field>
          {form.metric !== 'MANUAL' ? (
            <Field label="At this many">
              <input
                className="a-input"
                type="number"
                min={0}
                value={form.threshold}
                onChange={(e) => set('threshold', Number(e.target.value))}
              />
            </Field>
          ) : null}
        </div>
        <Field
          label="Points it carries"
          hint="Paid through the ordinary ledger the first time it is earned, and never twice."
        >
          <input
            className="a-input max-w-[10rem]"
            type="number"
            min={0}
            value={form.bonusXp ?? 0}
            onChange={(e) => set('bonusXp', Number(e.target.value))}
          />
        </Field>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active ?? true}
            onChange={(e) => set('active', e.target.checked)}
          />
          Still awarded
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !form.code || !form.name}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
