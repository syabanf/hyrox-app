'use client';

import type { UpsertRewardInput } from '@nuhabit/contracts';
import { REWARD_TYPES, type Reward } from '@nuhabit/domain';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Ban, Check, PackageCheck, Pencil } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard, StatRow,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

export default function RewardsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [tab, setTab] = useState<'catalogue' | 'claims'>('claims');
  const [status, setStatus] = useState('PENDING');
  const [editing, setEditing] = useState<Reward | 'new' | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: rewards, error: rewardsError } = useQuery({
    queryKey: ['crm', 'rewards'],
    queryFn: () => api.admin.crm.rewards.list(),
  });
  const { data: claims, isLoading } = useQuery({
    queryKey: ['crm', 'redemptions', status],
    queryFn: () => api.admin.crm.redemptions.list({ status: status || undefined }),
  });

  const decide = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'approve' | 'fulfil' | 'cancel' }) =>
      api.admin.crm.redemptions.decide(id, action),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['crm'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That claim did not change.'),
  });

  const catalogue = rewards ?? [];
  const rows = claims ?? [];

  return (
    <div>
      <PageTitle
        title="Rewards"
        subtitle="What points buy, and who has claimed what"
        actions={
          <>
            <Link href="/loyalty" className="a-btn-ghost">
              Members
            </Link>
            {can('crm.manage') ? (
              <button className="a-btn" onClick={() => setEditing('new')}>
                + New reward
              </button>
            ) : null}
          </>
        }
      />
      <ErrorNote message={error} />
      <QueryError error={rewardsError} />

      <StatRow>
        <StatCard label="Rewards offered" value={catalogue.filter((r) => r.active).length} tone="brand" />
        <StatCard tone="ok" label="Claims waiting" value={rows.filter((c) => c.status === 'PENDING').length} />
        <StatCard tone="warn" label="Fulfilled" value={rows.filter((c) => c.status === 'FULFILLED').length} />
        <StatCard
          tone="info"
          label="Points committed"
          value={rows
            .filter((c) => c.status !== 'CANCELLED')
            .reduce((sum, c) => sum + c.xpCost, 0)
            .toLocaleString()}
        />
      </StatRow>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <div className="flex gap-1.5">
          {(['claims', 'catalogue'] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={tab === t ? 'a-btn !px-3 !py-1.5 text-xs' : 'a-btn-ghost !px-3 !py-1.5 text-xs'}
            >
              {t === 'claims' ? 'Claims' : 'Catalogue'}
            </button>
          ))}
        </div>
        {tab === 'claims' ? (
          <div className="w-44">
            <SearchSelect
              value={status}
              onChange={setStatus}
              allowEmpty
              emptyLabel="Any status"
              placeholder="Search status…"
              options={['PENDING', 'APPROVED', 'FULFILLED', 'CANCELLED'].map((s) => ({
                value: s,
                label: s[0]! + s.slice(1).toLowerCase(),
              }))}
            />
          </div>
        ) : null}
      </div>

      {tab === 'claims' ? (
        isLoading ? (
          <Spinner label="Loading claims…" />
        ) : (
          <div className="a-card !p-0">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Claim</th>
                  <th>Member</th>
                  <th>Reward</th>
                  <th className="text-right">Points</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((claim) => (
                  <tr key={claim.id}>
                    <td className="font-bold">{claim.redemptionNumber}</td>
                    <td className="text-sm">{claim.memberName}</td>
                    <td className="text-sm">{claim.rewardName}</td>
                    <td className="text-right tabular-nums">{claim.xpCost.toLocaleString()}</td>
                    <td>
                      <StatusBadge status={claim.status} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Approve',
                            icon: Check,
                            disabled: !can('crm.approve') || claim.status !== 'PENDING',
                            onClick: () => decide.mutate({ id: claim.id, action: 'approve' }),
                          },
                          {
                            label: 'Hand over',
                            icon: PackageCheck,
                            disabled: !can('crm.approve') || claim.status !== 'APPROVED',
                            onClick: () => decide.mutate({ id: claim.id, action: 'fulfil' }),
                          },
                          {
                            label: 'Cancel',
                            icon: Ban,
                            tone: 'danger' as const,
                            disabled:
                              !can('crm.approve') ||
                              !['PENDING', 'APPROVED'].includes(claim.status),
                            onClick: () => {
                              if (confirm('Cancel this claim and give the points back?'))
                                decide.mutate({ id: claim.id, action: 'cancel' });
                            },
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      Nothing to hand out.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        )
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Reward</th>
                <th>Kind</th>
                <th className="text-right">Cost</th>
                <th>Requires</th>
                <th className="text-right">Stock</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {catalogue.map((reward) => (
                <tr key={reward.id}>
                  <td>
                    <p className="font-bold">{reward.name}</p>
                    <p className="text-xs text-muted">{reward.code}</p>
                  </td>
                  <td>
                    <StatusBadge status={reward.active ? reward.rewardType : 'INACTIVE'} />
                  </td>
                  <td className="text-right tabular-nums">{reward.xpCost.toLocaleString()} XP</td>
                  <td className="text-sm">
                    {reward.requiredTierCode ?? <span className="text-muted">any tier</span>}
                  </td>
                  <td className="text-right tabular-nums text-sm">
                    {reward.stockTotal == null ? (
                      <span className="text-muted">unlimited</span>
                    ) : (
                      `${reward.stockTotal - reward.stockRedeemed} left`
                    )}
                  </td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        {
                          label: 'Edit',
                          icon: Pencil,
                          disabled: !can('crm.manage'),
                          onClick: () => setEditing(reward),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
              {catalogue.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-sm text-muted">
                    Nothing to spend points on yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      )}

      {editing ? (
        <RewardModal
          reward={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['crm'] });
          }}
        />
      ) : null}
    </div>
  );
}

function RewardModal({
  reward,
  onClose,
  onDone,
}: {
  reward: Reward | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const { data: tiers } = useQuery({ queryKey: ['crm', 'tiers'], queryFn: api.admin.crm.tiers.list });

  const [form, setForm] = useState<UpsertRewardInput>(() => ({
    code: reward?.code ?? '',
    name: reward?.name ?? '',
    description: reward?.description ?? '',
    rewardType: reward?.rewardType ?? 'MERCHANDISE',
    xpCost: reward?.xpCost ?? 0,
    requiredTierCode: reward?.requiredTierCode ?? null,
    stockTotal: reward?.stockTotal ?? null,
    maxPerMember: reward?.maxPerMember ?? null,
    active: reward?.active ?? true,
  }));
  const set = <K extends keyof UpsertRewardInput>(k: K, v: UpsertRewardInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const save = useMutation({
    mutationFn: () => api.admin.crm.rewards.save(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That reward did not save.'),
  });

  return (
    <Modal title={reward ? `Edit ${reward.name}` : 'New reward'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Code">
            <input className="a-input" value={form.code} onChange={(e) => set('code', e.target.value)} />
          </Field>
          <Field label="Points to claim">
            <input
              className="a-input"
              type="number"
              value={form.xpCost}
              onChange={(e) => set('xpCost', Number(e.target.value))}
            />
          </Field>
        </div>
        <Field label="Name">
          <input className="a-input" value={form.name} onChange={(e) => set('name', e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Kind">
            <SearchSelect
              value={form.rewardType}
              onChange={(v) => set('rewardType', v)}
              placeholder="Search kind…"
              options={REWARD_TYPES.map((t) => ({ value: t, label: t[0]! + t.slice(1).toLowerCase() }))}
            />
          </Field>
          <Field label="Minimum tier" hint="Checked before affordability, so nobody is told to save up for something they cannot have.">
            <SearchSelect
              value={form.requiredTierCode ?? ''}
              onChange={(v) => set('requiredTierCode', v || null)}
              allowEmpty
              emptyLabel="Any tier"
              placeholder="Search tier…"
              options={(tiers ?? []).map((t) => ({ value: t.code, label: t.name }))}
            />
          </Field>
          <Field label="Total stock" hint="Empty for unlimited.">
            <input
              className="a-input"
              value={form.stockTotal ?? ''}
              onChange={(e) => set('stockTotal', e.target.value === '' ? null : Number(e.target.value))}
            />
          </Field>
          <Field label="Limit per member" hint="Empty for no limit.">
            <input
              className="a-input"
              value={form.maxPerMember ?? ''}
              onChange={(e) => set('maxPerMember', e.target.value === '' ? null : Number(e.target.value))}
            />
          </Field>
        </div>
        <Field label="Description">
          <textarea
            className="a-input"
            rows={2}
            value={form.description ?? ''}
            onChange={(e) => set('description', e.target.value)}
          />
        </Field>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active ?? true}
            onChange={(e) => set('active', e.target.checked)}
          />
          Currently offered
        </label>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !form.code || !form.name} onClick={() => save.mutate()}>
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
