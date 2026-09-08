'use client';

import type { LoyaltyTier } from '@nuhabit/domain';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  RowActions,
  StatCard,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

export default function SchemePage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editingTier, setEditingTier] = useState<LoyaltyTier | 'new' | null>(null);

  const { data: tiers, isLoading, error } = useQuery({
    queryKey: ['crm', 'tiers'],
    queryFn: api.admin.crm.tiers.list,
  });
  const { data: rules } = useQuery({
    queryKey: ['crm', 'rules'],
    queryFn: () => api.admin.crm.rules.list(),
  });

  return (
    <div>
      <PageTitle
        title="Tiers &amp; earning rules"
        subtitle="How standing is earned, and what it is worth"
        actions={
          <>
            <Link href="/loyalty" className="a-btn-ghost">
              Members
            </Link>
            {can('crm.manage') ? (
              <button className="a-btn" onClick={() => setEditingTier('new')}>
                + New tier
              </button>
            ) : null}
          </>
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Tiers" value={(tiers ?? []).length} tone="brand" />
        <StatCard tone="info" label="Earning rules" value={(rules ?? []).filter((r) => r.active).length} />
        <StatCard
          label="Channels covered"
          value={new Set((rules ?? []).filter((r) => r.active).map((r) => r.sourceChannel)).size}
        />
      </div>

      {isLoading ? (
        <Spinner label="Loading the scheme…" />
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="a-card !p-0">
            <h2 className="px-4 pt-4 font-black">Tiers</h2>
            <p className="px-4 pb-2 text-xs text-muted">
              Both thresholds must be met. Because lifetime points only ever grow, spending them
              never costs a member their tier.
            </p>
            <table className="a-table">
              <thead>
                <tr>
                  <th>Tier</th>
                  <th className="text-right">From</th>
                  <th className="text-right">Multiplier</th>
                  <th className="text-right">Discount</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {(tiers ?? []).map((tier) => (
                  <tr key={tier.id}>
                    <td>
                      <span
                        className="rounded-lg px-2 py-0.5 text-xs font-bold"
                        style={{ background: `${tier.colour}22`, color: tier.colour }}
                      >
                        {tier.name}
                      </span>
                      <p className="mt-0.5 text-xs text-muted">rank {tier.rank}</p>
                    </td>
                    <td className="text-right text-sm tabular-nums">
                      {tier.minLifetimeXp.toLocaleString()} XP
                      {tier.minSpendIdr > 0 ? (
                        <p className="text-xs text-muted">and {formatIdr(tier.minSpendIdr)}</p>
                      ) : null}
                    </td>
                    <td className="text-right tabular-nums text-sm">{tier.xpMultiplier}×</td>
                    <td className="text-right tabular-nums text-sm">{tier.discountPercent}%</td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Edit',
                            icon: Pencil,
                            disabled: !can('crm.manage'),
                            onClick: () => setEditingTier(tier),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="a-card !p-0">
            <h2 className="px-4 pt-4 font-black">Earning rules</h2>
            <p className="px-4 pb-2 text-xs text-muted">
              Highest priority wins; a tie goes to the more specific rule, so a campaign aimed at
              one class beats the studio-wide default without anybody disabling it.
            </p>
            <table className="a-table">
              <thead>
                <tr>
                  <th>Rule</th>
                  <th>From</th>
                  <th className="text-right">Worth</th>
                  <th>Tier bonus</th>
                </tr>
              </thead>
              <tbody>
                {(rules ?? []).map((rule) => (
                  <tr key={rule.id}>
                    <td>
                      <p className="font-bold">{rule.name}</p>
                      <p className="text-xs text-muted">
                        {rule.code} · priority {rule.priority}
                      </p>
                    </td>
                    <td>
                      <StatusBadge status={rule.active ? rule.sourceChannel : 'INACTIVE'} />
                    </td>
                    <td className="text-right text-sm tabular-nums">
                      {rule.xpMode === 'PER_AMOUNT'
                        ? `${rule.xpValue} XP / ${formatIdr(rule.amountStep)}`
                        : rule.xpMode === 'PERCENTAGE'
                          ? `${rule.xpValue}% of spend`
                          : `${rule.xpValue} XP`}
                      {rule.maxXpPerEvent != null ? (
                        <p className="text-xs text-muted">capped at {rule.maxXpPerEvent}</p>
                      ) : null}
                    </td>
                    <td className="text-sm">
                      {rule.tierMultiplierEnabled ? (
                        <span className="text-brand">applies</span>
                      ) : (
                        <span className="text-muted">flat</span>
                      )}
                    </td>
                  </tr>
                ))}
                {(rules ?? []).length === 0 ? (
                  <tr>
                    <td colSpan={4} className="py-8 text-center text-sm text-muted">
                      Nothing earns points yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {editingTier ? (
        <TierModal
          tier={editingTier === 'new' ? null : editingTier}
          onClose={() => setEditingTier(null)}
          onDone={() => {
            setEditingTier(null);
            void qc.invalidateQueries({ queryKey: ['crm'] });
          }}
        />
      ) : null}
    </div>
  );
}

function TierModal({
  tier,
  onClose,
  onDone,
}: {
  tier: LoyaltyTier | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    code: tier?.code ?? '',
    name: tier?.name ?? '',
    rank: tier?.rank ?? 1,
    minLifetimeXp: tier?.minLifetimeXp ?? 0,
    minSpendIdr: tier?.minSpendIdr ?? 0,
    xpMultiplier: tier?.xpMultiplier ?? 1,
    discountPercent: tier?.discountPercent ?? 0,
    colour: tier?.colour ?? '#5F6B62',
    active: tier?.active ?? true,
  });
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const save = useMutation({
    mutationFn: () => api.admin.crm.tiers.save({ ...form, benefits: tier?.benefits ?? [] }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That tier did not save.'),
  });

  return (
    <Modal title={tier ? `Edit ${tier.name}` : 'New tier'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Code">
            <input className="a-input" value={form.code} onChange={(e) => set('code', e.target.value)} />
          </Field>
          <Field label="Rank" hint="1 is the entry tier.">
            <input
              className="a-input"
              type="number"
              value={form.rank}
              onChange={(e) => set('rank', Number(e.target.value))}
            />
          </Field>
        </div>
        <Field label="Name">
          <input className="a-input" value={form.name} onChange={(e) => set('name', e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Lifetime points needed">
            <input
              className="a-input"
              type="number"
              value={form.minLifetimeXp}
              onChange={(e) => set('minLifetimeXp', Number(e.target.value))}
            />
          </Field>
          <Field label="Lifetime spend needed" hint="Both thresholds must be met.">
            <input
              className="a-input"
              type="number"
              value={form.minSpendIdr}
              onChange={(e) => set('minSpendIdr', Number(e.target.value))}
            />
          </Field>
          <Field label="Points multiplier">
            <input
              className="a-input"
              type="number"
              step="0.05"
              value={form.xpMultiplier}
              onChange={(e) => set('xpMultiplier', Number(e.target.value))}
            />
          </Field>
          <Field label="Counter discount %">
            <input
              className="a-input"
              type="number"
              value={form.discountPercent}
              onChange={(e) => set('discountPercent', Number(e.target.value))}
            />
          </Field>
        </div>
        <Field label="Colour">
          <input className="a-input" value={form.colour} onChange={(e) => set('colour', e.target.value)} />
        </Field>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input type="checkbox" checked={form.active} onChange={(e) => set('active', e.target.checked)} />
          In use
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
