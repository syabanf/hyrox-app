'use client';

import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Gift, Plus } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  Pager,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../components/ui';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';

const PAGE_SIZE = 25;

export default function LoyaltyPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [tier, setTier] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const [adjusting, setAdjusting] = useState<string | null>(null);
  const [redeeming, setRedeeming] = useState<string | null>(null);
  const [openMember, setOpenMember] = useState<string | null>(null);

  const { data: overview } = useQuery({
    queryKey: ['crm', 'overview'],
    queryFn: api.admin.crm.overview,
  });
  const { data: members, isLoading, error } = useQuery({
    queryKey: ['crm', 'members', tier],
    queryFn: () => api.admin.crm.members.list({ tierCode: tier || undefined }),
  });
  const { data: tiers } = useQuery({ queryKey: ['crm', 'tiers'], queryFn: api.admin.crm.tiers.list });

  const q = query.trim().toLowerCase();
  const rows = (members ?? []).filter(
    (m) => !q || m.memberName.toLowerCase().includes(q) || m.memberCode.toLowerCase().includes(q),
  );
  const shown = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));

  return (
    <div>
      <PageTitle
        title="Loyalty"
        subtitle="Points are not credits: they cannot be topped up and they never pay for a class"
        actions={
          <>
            <Link href="/loyalty/rewards" className="a-btn-ghost">
              Rewards
            </Link>
            <Link href="/loyalty/scheme" className="a-btn-ghost">
              Tiers &amp; rules
            </Link>
          </>
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="Members earning" value={overview?.members ?? '—'} tone="brand" />
        <StatCard tone="lime"
          label="Points outstanding"
          value={overview?.outstandingXp.toLocaleString() ?? '—'}
          hint="What members could still spend"
        />
        <StatCard tone="ok"
          label="Earned this month"
          value={overview?.earnedThisMonth.toLocaleString() ?? '—'}
        />
        <StatCard
          label="Claims waiting"
          value={overview?.pendingClaims ?? '—'}
          tone={overview && overview.pendingClaims > 0 ? 'danger' : undefined}
        />
      </div>

      {overview && Object.keys(overview.byTier).length > 0 ? (
        <div className="a-card mb-4">
          <p className="mb-2 text-[11px] font-bold uppercase tracking-wider text-muted">By tier</p>
          <div className="flex flex-wrap gap-2">
            {(tiers ?? []).map((t) => (
              <span
                key={t.code}
                className="rounded-xl border border-line px-3 py-1.5 text-sm font-bold"
                style={{ borderColor: t.colour }}
              >
                {t.name}
                <span className="ml-2 text-muted">{overview.byTier[t.code] ?? 0}</span>
              </span>
            ))}
          </div>
        </div>
      ) : null}

      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search member…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-44">
          <SearchSelect
            value={tier}
            onChange={(v) => {
              setTier(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All tiers"
            placeholder="Search tier…"
            options={(tiers ?? []).map((t) => ({ value: t.code, label: t.name }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading members…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Member</th>
                  <th>Tier</th>
                  <th className="text-right">Points</th>
                  <th className="text-right">Lifetime</th>
                  <th className="text-right">Spent with us</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((profile) => (
                  <tr key={profile.id}>
                    <td>
                      <button
                        className="font-bold hover:text-brand"
                        onClick={() => setOpenMember(profile.memberId)}
                      >
                        {profile.memberName || profile.memberId}
                      </button>
                      <p className="text-xs text-muted">{profile.memberCode}</p>
                    </td>
                    <td>
                      {profile.tier ? (
                        <span
                          className="rounded-lg px-2 py-0.5 text-xs font-bold"
                          style={{ background: `${profile.tier.colour}22`, color: profile.tier.colour }}
                        >
                          {profile.tier.name}
                        </span>
                      ) : (
                        <StatusBadge status={profile.tierCode} />
                      )}
                    </td>
                    <td className="text-right tabular-nums font-bold">
                      {profile.currentXp.toLocaleString()}
                    </td>
                    <td className="text-right tabular-nums text-sm text-muted">
                      {profile.lifetimeXp.toLocaleString()}
                    </td>
                    <td className="text-right tabular-nums text-sm">
                      {formatIdr(profile.lifetimeSpendIdr)}
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Adjust points',
                            icon: Plus,
                            disabled: !can('crm.adjust'),
                            onClick: () => setAdjusting(profile.memberId),
                          },
                          {
                            label: 'Redeem a reward',
                            icon: Gift,
                            disabled: !can('crm.approve'),
                            onClick: () => setRedeeming(profile.memberId),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-sm text-muted">
                      Nobody has earned anything yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}

      {adjusting ? (
        <AdjustModal
          memberId={adjusting}
          onClose={() => setAdjusting(null)}
          onDone={() => {
            setAdjusting(null);
            void qc.invalidateQueries({ queryKey: ['crm'] });
          }}
        />
      ) : null}
      {redeeming ? (
        <RedeemModal
          memberId={redeeming}
          onClose={() => setRedeeming(null)}
          onDone={() => {
            setRedeeming(null);
            void qc.invalidateQueries({ queryKey: ['crm'] });
          }}
        />
      ) : null}
      {openMember ? (
        <MemberSheet memberId={openMember} onClose={() => setOpenMember(null)} />
      ) : null}
    </div>
  );
}

function AdjustModal({
  memberId,
  onClose,
  onDone,
}: {
  memberId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [delta, setDelta] = useState('');
  const [reason, setReason] = useState('');

  const save = useMutation({
    mutationFn: () => api.admin.crm.members.adjust(memberId, { xpDelta: Number(delta), reason }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That adjustment did not save.'),
  });

  return (
    <Modal title="Adjust points" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Change" hint="A signed number: negative takes points away.">
          <input className="a-input" value={delta} onChange={(e) => setDelta(e.target.value)} />
        </Field>
        <Field label="Reason" hint="Points appearing without one look like a member being favoured.">
          <input className="a-input" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !reason.trim() || !Number(delta)}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Adjust'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function RedeemModal({
  memberId,
  onClose,
  onDone,
}: {
  memberId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [reward, setReward] = useState('');

  const { data: rewards } = useQuery({
    queryKey: ['crm', 'rewards'],
    queryFn: () => api.admin.crm.rewards.list(true),
  });
  const { data: detail } = useQuery({
    queryKey: ['crm', 'member', memberId],
    queryFn: () => api.admin.crm.members.get(memberId),
  });

  const save = useMutation({
    mutationFn: () => api.admin.crm.members.redeem(memberId, reward),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That reward could not be claimed.'),
  });

  return (
    <Modal title="Redeem a reward" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        {detail ? (
          <p className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
            <span className="font-bold">{detail.profile.currentXp.toLocaleString()} points</span>{' '}
            available · {detail.profile.tier?.name ?? detail.profile.tierCode}
          </p>
        ) : null}
        <Field label="Reward">
          <SearchSelect
            value={reward}
            onChange={setReward}
            placeholder="Search reward…"
            options={(rewards ?? []).map((r) => ({
              value: r.id,
              label: r.name,
              hint: `${r.xpCost.toLocaleString()} XP${r.requiredTierCode ? ` · ${r.requiredTierCode} and up` : ''}`,
            }))}
          />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !reward} onClick={() => save.mutate()}>
            {save.isPending ? 'Claiming…' : 'Redeem'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

/** One member's standing, their ledger and what they have claimed. */
function MemberSheet({ memberId, onClose }: { memberId: string; onClose: () => void }) {
  const { data } = useQuery({
    queryKey: ['crm', 'member', memberId],
    queryFn: () => api.admin.crm.members.get(memberId),
  });

  return (
    <Modal title={data?.profile.memberName ?? 'Member'} onClose={onClose}>
      <div className="grid gap-3">
        {data ? (
          <>
            <div className="grid gap-2 sm:grid-cols-3">
              <div className="rounded-xl bg-surface-raised px-3 py-2">
                <p className="text-[11px] font-bold uppercase tracking-wider text-muted">Points</p>
                <p className="display text-2xl font-black">{data.profile.currentXp.toLocaleString()}</p>
              </div>
              <div className="rounded-xl bg-surface-raised px-3 py-2">
                <p className="text-[11px] font-bold uppercase tracking-wider text-muted">Lifetime</p>
                <p className="display text-2xl font-black">{data.profile.lifetimeXp.toLocaleString()}</p>
              </div>
              <div className="rounded-xl bg-surface-raised px-3 py-2">
                <p className="text-[11px] font-bold uppercase tracking-wider text-muted">Tier</p>
                <p className="display text-2xl font-black">
                  {data.profile.tier?.name ?? data.profile.tierCode}
                </p>
              </div>
            </div>
            {data.profile.nextTier ? (
              <p className="text-sm text-muted">
                {data.profile.xpToNext.toLocaleString()} more lifetime points to reach{' '}
                <span className="font-bold text-ink">{data.profile.nextTier.name}</span>. Spending
                points never costs a tier — only lifetime earnings count.
              </p>
            ) : null}

            <div className="a-card !p-0 max-h-64 overflow-y-auto">
              <table className="a-table">
                <thead>
                  <tr>
                    <th>What</th>
                    <th className="text-right">Points</th>
                    <th className="text-right">Balance</th>
                  </tr>
                </thead>
                <tbody>
                  {data.ledger.map((entry) => (
                    <tr key={entry.id}>
                      <td className="text-sm">
                        <span className="font-bold">{entry.direction.toLowerCase()}</span>
                        {entry.description ? (
                          <p className="text-xs text-muted">{entry.description}</p>
                        ) : null}
                      </td>
                      <td
                        className={`text-right tabular-nums font-bold ${
                          entry.xpDelta < 0 ? 'text-danger' : 'text-brand'
                        }`}
                      >
                        {entry.xpDelta > 0 ? '+' : ''}
                        {entry.xpDelta}
                      </td>
                      <td className="text-right tabular-nums text-sm text-muted">
                        {entry.balanceAfter}
                      </td>
                    </tr>
                  ))}
                  {data.ledger.length === 0 ? (
                    <tr>
                      <td colSpan={3} className="py-6 text-center text-sm text-muted">
                        Nothing earned yet.
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </>
        ) : (
          <Spinner label="Loading…" />
        )}
        <div className="mt-2 flex justify-end">
          <button className="a-btn-ghost" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </Modal>
  );
}
