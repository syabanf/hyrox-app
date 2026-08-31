'use client';

import type { MemberStatus } from '@hyrox/domain';
import { Spinner, StatusBadge, formatDay, formatDayTime, formatIdr } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle, StatCard } from '../../../../components/ui';

const TABS = ['Overview', 'Credit Ledger', 'Bookings', 'Visits', 'Payments', 'Waiver', 'Activity'] as const;
type Tab = (typeof TABS)[number];

export default function MemberDetailPage() {
  const { id } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [tab, setTab] = useState<Tab>('Overview');
  const [adjustOpen, setAdjustOpen] = useState(false);
  const [reverseTarget, setReverseTarget] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: m, isLoading } = useQuery({
    queryKey: ['member', id],
    queryFn: () => api.admin.members.get(id),
  });

  const invalidate = () => {
    void qc.invalidateQueries();
    setError(null);
  };
  const onError = (e: unknown) =>
    setError(e instanceof ApiError ? e.message : 'Something went wrong.');

  const statusMutation = useMutation({
    mutationFn: (status: MemberStatus) =>
      api.admin.members.update(id, { status, reason: 'Changed from member 360 view' }),
    onSuccess: invalidate,
    onError,
  });

  if (isLoading || !m) return <Spinner label="Loading member…" />;

  return (
    <div>
      <PageTitle
        title={m.member.fullName}
        subtitle={`${m.member.email} · ${m.member.phone}`}
        actions={
          can('members.manage') ? (
            <select
              className="a-input"
              value={m.member.status}
              onChange={(e) => statusMutation.mutate(e.target.value as MemberStatus)}
            >
              {['ACTIVE', 'SUSPENDED', 'INACTIVE', 'ARCHIVED'].map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          ) : (
            <StatusBadge status={m.member.status} />
          )
        }
      />

      <ErrorNote message={error} />

      <div className="mb-5 mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="Credit balance" value={m.balance} tone="brand" />
        <StatCard label="Expiring soon" value={m.expiringCredits} tone={m.expiringCredits > 0 ? 'danger' : undefined} />
        <StatCard label="Total visits" value={m.totalVisits} />
        <StatCard label="Last visit" value={m.lastVisitAt ? formatDay(m.lastVisitAt) : '—'} />
      </div>

      <div className="mb-4 flex flex-wrap gap-1 rounded-xl bg-surface p-1">
        {TABS.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-lg px-3.5 py-1.5 text-sm font-bold ${
              tab === t ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === 'Overview' ? (
        <div className="grid gap-3 lg:grid-cols-2">
          <div className="a-card text-sm">
            <p className="a-label">Profile</p>
            <dl className="grid grid-cols-2 gap-y-2">
              <dt className="text-muted">Member ID</dt>
              <dd className="font-mono text-xs">{m.member.id}</dd>
              <dt className="text-muted">Gender</dt>
              <dd>{m.member.gender ?? '—'}</dd>
              <dt className="text-muted">Date of birth</dt>
              <dd>{m.member.dateOfBirth ? formatDay(m.member.dateOfBirth) : '—'}</dd>
              <dt className="text-muted">Joined</dt>
              <dd>{formatDay(m.member.createdAt)}</dd>
              <dt className="text-muted">Emergency contact</dt>
              <dd>
                {m.member.emergencyContact
                  ? `${m.member.emergencyContact.name} · ${m.member.emergencyContact.phone}`
                  : '—'}
              </dd>
            </dl>
          </div>
          <div className="a-card">
            <p className="a-label">Purchased packages</p>
            {m.packages.length === 0 ? (
              <p className="text-sm text-muted">No packages purchased yet.</p>
            ) : (
              <div className="flex flex-col gap-2 text-sm">
                {m.packages.map((p) => (
                  <div key={p.lotId} className={`flex items-center justify-between gap-3 ${p.active ? '' : 'opacity-60'}`}>
                    <div className="min-w-0">
                      <p className="font-bold">{p.name}</p>
                      <p className="text-xs text-muted">
                        {p.credits} cr · bought {formatDay(p.purchasedAt)} ·{' '}
                        {p.coverageNames ? p.coverageNames.join(', ') : 'all classes'}
                      </p>
                    </div>
                    <span className={`chip shrink-0 ${p.active ? 'bg-ok/10 text-ok' : 'bg-surface-raised text-muted'}`}>
                      {p.active ? `until ${formatDay(p.expiresAt)}` : 'Expired'}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
          <div className="a-card !p-0">
            <p className="a-label px-4 pt-4">Upcoming bookings</p>
            <table className="a-table">
              <tbody>
                {m.upcomingBookings.map((b) => (
                  <tr key={b.booking.id}>
                    <td className="font-bold">{b.classTypeName}</td>
                    <td className="text-muted">{formatDayTime(b.session.startsAt)}</td>
                    <td>
                      <StatusBadge status={b.booking.status} />
                    </td>
                  </tr>
                ))}
                {m.upcomingBookings.length === 0 ? (
                  <tr>
                    <td className="py-4 text-center text-muted">Nothing upcoming.</td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {tab === 'Credit Ledger' ? (
        <div className="a-card !p-0">
          <div className="flex items-center justify-between px-4 pt-4">
            <p className="a-label !mb-0">Balance = sum of ledger entries. Corrections via reversal only.</p>
            {can('members.adjust_credits') ? (
              <button className="a-btn" onClick={() => setAdjustOpen(true)}>
                Manual adjustment
              </button>
            ) : null}
          </div>
          <table className="a-table mt-2">
            <thead>
              <tr>
                <th>When</th>
                <th>Type</th>
                <th>Description</th>
                <th className="text-right">Credits</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {m.entries.map((e) => (
                <tr key={e.id}>
                  <td className="whitespace-nowrap text-muted">{formatDayTime(e.createdAt)}</td>
                  <td>
                    <StatusBadge status={e.type} tone={e.amount >= 0 ? 'ok' : 'neutral'} />
                  </td>
                  <td>
                    {e.description}
                    {e.reason ? <span className="block text-xs text-muted">Reason: {e.reason}</span> : null}
                  </td>
                  <td className={`text-right font-black ${e.amount >= 0 ? 'text-ok' : 'text-danger'}`}>
                    {e.amount > 0 ? `+${e.amount}` : e.amount}
                  </td>
                  <td className="text-right">
                    {can('ledger.reverse') && e.type !== 'REVERSAL' ? (
                      <button
                        className="text-xs font-bold text-muted hover:text-danger"
                        onClick={() => setReverseTarget(e.id)}
                      >
                        Reverse
                      </button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'Bookings' ? (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Class</th>
                <th>When</th>
                <th>Branch</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {m.bookings.map((b) => (
                <tr key={b.booking.id}>
                  <td className="font-bold">{b.classTypeName}</td>
                  <td className="text-muted">{formatDayTime(b.session.startsAt)}</td>
                  <td>{b.branchName}</td>
                  <td>
                    <StatusBadge status={b.booking.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'Visits' ? (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Gate</th>
                <th>Result</th>
                <th className="text-right">Credits</th>
                <th>Mode</th>
              </tr>
            </thead>
            <tbody>
              {m.visits.map((v) => (
                <tr key={v.log.id}>
                  <td className="text-muted">{formatDayTime(v.log.createdAt)}</td>
                  <td>{v.gateName}</td>
                  <td>
                    <StatusBadge status={v.log.result} />
                    {v.log.reasonCode ? (
                      <span className="ml-1 text-xs text-danger">{v.log.reasonCode}</span>
                    ) : null}
                  </td>
                  <td className="text-right font-bold">{v.log.creditDelta || '—'}</td>
                  <td className="text-muted">{v.log.mode}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'Payments' ? (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Payment</th>
                <th className="text-right">Amount</th>
                <th>Channel</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {m.payments.map((p) => (
                <tr key={p.id}>
                  <td className="text-muted">{formatDayTime(p.createdAt)}</td>
                  <td className="font-mono text-xs">{p.id}</td>
                  <td className="text-right font-bold">{formatIdr(p.totalIdr)}</td>
                  <td>{p.channel}</td>
                  <td>
                    <StatusBadge status={p.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'Waiver' ? (
        <div className="a-card max-w-xl text-sm">
          <p className="a-label">Digital waiver</p>
          {m.member.waiverAcceptedAt ? (
            <p>
              Version <span className="font-bold">{m.member.waiverVersion}</span> signed on{' '}
              <span className="font-bold">{formatDayTime(m.member.waiverAcceptedAt)}</span>.
            </p>
          ) : (
            <p className="text-danger">No waiver on file.</p>
          )}
        </div>
      ) : null}

      {tab === 'Activity' ? (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Action</th>
                <th>Change</th>
                <th>Actor</th>
                <th>Reason</th>
              </tr>
            </thead>
            <tbody>
              {m.audit.map((a) => (
                <tr key={a.id}>
                  <td className="text-muted">{formatDayTime(a.createdAt)}</td>
                  <td className="font-bold">
                    {a.entityType} · {a.action}
                  </td>
                  <td className="text-muted">
                    {a.previousValue ?? '—'} → {a.newValue ?? '—'}
                  </td>
                  <td>{a.actorName}</td>
                  <td className="text-muted">{a.reason ?? '—'}</td>
                </tr>
              ))}
              {m.audit.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-4 text-center text-muted">
                    No audited actions for this member.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      ) : null}

      {adjustOpen ? (
        <AdjustModal
          memberId={id}
          onClose={() => setAdjustOpen(false)}
          onDone={() => {
            setAdjustOpen(false);
            invalidate();
          }}
        />
      ) : null}
      {reverseTarget ? (
        <ReverseModal
          entryId={reverseTarget}
          onClose={() => setReverseTarget(null)}
          onDone={() => {
            setReverseTarget(null);
            invalidate();
          }}
        />
      ) : null}
    </div>
  );
}

function AdjustModal({ memberId, onClose, onDone }: { memberId: string; onClose: () => void; onDone: () => void }) {
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [error, setError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => api.admin.members.adjust(memberId, { amount: Number(amount), reason }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Adjustment failed.'),
  });
  return (
    <Modal title="Manual credit adjustment" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <p className="text-sm text-muted">
          Creates an ADJUSTMENT ledger entry (audited). The balance itself is never edited.
        </p>
        <div>
          <label className="a-label">Amount (± credits)</label>
          <input className="a-input" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="-1 or 5" />
        </div>
        <div>
          <label className="a-label">Reason (required)</label>
          <input className="a-input" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Why?" />
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || !amount || Number.isNaN(Number(amount)) || reason.length < 3}
          onClick={() => mutation.mutate()}
        >
          Post adjustment
        </button>
      </div>
    </Modal>
  );
}

function ReverseModal({ entryId, onClose, onDone }: { entryId: string; onClose: () => void; onDone: () => void }) {
  const [reason, setReason] = useState('');
  const [error, setError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => api.admin.ledger.reverse(entryId, reason),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Reversal failed.'),
  });
  return (
    <Modal title="Reverse ledger entry" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <p className="text-sm text-muted">
          Financial entries are immutable — this posts a compensating REVERSAL entry referencing{' '}
          <span className="font-mono text-xs">{entryId}</span>.
        </p>
        <div>
          <label className="a-label">Reason (required)</label>
          <input className="a-input" value={reason} onChange={(e) => setReason(e.target.value)} />
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn-danger"
          disabled={mutation.isPending || reason.length < 3}
          onClick={() => mutation.mutate()}
        >
          Post reversal
        </button>
      </div>
    </Modal>
  );
}
