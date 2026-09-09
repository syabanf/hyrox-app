'use client';

import type { IntegrationPartner } from '@nuhabit/domain';
import { PARTNER_KINDS } from '@nuhabit/domain';
import { formatDayTime, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link2, Pencil } from 'lucide-react';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  SearchSelect,
  StatCard, StatRow,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * Things that happened somewhere else.
 *
 * A race timing system knows somebody finished; a partner gym knows they
 * trained. Each is a fact this system did not observe, held as what somebody
 * else said — and matched to a member separately, so a wrong match can be
 * corrected without losing what they actually sent.
 */
export default function PartnersPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<IntegrationPartner | 'new' | null>(null);
  const [status, setStatus] = useState('UNMATCHED');
  const [matching, setMatching] = useState<string | null>(null);

  const { data: partners, error } = useQuery({
    queryKey: ['crm', 'partners'],
    queryFn: () => api.admin.crm.partners.list(),
  });
  const { data: events, isLoading } = useQuery({
    queryKey: ['crm', 'events', status],
    queryFn: () => api.admin.crm.events.list({ status: status || undefined, limit: 100 }),
  });

  const rows = events ?? [];

  return (
    <div>
      <PageTitle
        title="Partners"
        subtitle="What other systems tell us, and who it turned out to be about"
        actions={
          can('crm.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              <Link2 size={16} /> New partner
            </button>
          ) : null
        }
      />
      <QueryError error={error} />

      <StatRow>
        <StatCard tone="ink" label="Partners" value={(partners ?? []).length} icon={Link2} />
        <StatCard
          tone="ok"
          label="Can post"
          value={(partners ?? []).filter((p) => p.hasSecret && p.active).length}
          hint="A partner without a secret can post nothing"
        />
        <StatCard
          label="Awarding points"
          value={(partners ?? []).filter((p) => p.awardsXp).length}
          hint="Recording a partner's word is not paying out on it"
          tone="brand"
        />
      </StatRow>

      <div className="a-card mb-4 !p-0">
        <h2 className="px-4 pt-4 font-black">Who sends us facts</h2>
        <table className="a-table mt-2">
          <thead>
            <tr>
              <th>Partner</th>
              <th>Kind</th>
              <th>Contact</th>
              <th>Can post</th>
              <th>Awards points</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(partners ?? []).map((partner) => (
              <tr key={partner.id}>
                <td>
                  <p className="font-bold">{partner.name}</p>
                  <p className="font-mono text-xs text-muted">{partner.code}</p>
                </td>
                <td className="text-sm text-muted">{partner.kind.toLowerCase()}</td>
                <td className="text-sm text-muted">
                  {partner.contactName ?? '—'}
                  {partner.contactEmail ? ` · ${partner.contactEmail}` : ''}
                </td>
                <td className="text-sm">{partner.hasSecret ? 'yes' : 'no secret set'}</td>
                <td className="text-sm">{partner.awardsXp ? 'yes' : '—'}</td>
                <td>
                  <StatusBadge status={partner.active ? 'ACTIVE' : 'INACTIVE'} />
                </td>
                <td className="text-right">
                  {can('crm.manage') ? (
                    <button
                      className="a-btn-ghost !px-2 !py-1 text-xs"
                      onClick={() => setEditing(partner)}
                    >
                      <Pencil size={13} />
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
            {(partners ?? []).length === 0 ? (
              <tr>
                <td colSpan={7} className="py-8 text-center text-sm text-muted">
                  No partners yet.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <div className="mb-3 flex gap-1 rounded-xl border border-line p-1">
        {[
          { value: 'UNMATCHED', label: 'Needs a person' },
          { value: 'PROCESSED', label: 'Acted on' },
          { value: '', label: 'Everything' },
        ].map((tab) => (
          <button
            key={tab.value}
            onClick={() => setStatus(tab.value)}
            className={`rounded-lg px-3 py-1 text-xs font-bold transition ${
              status === tab.value ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Spinner label="Loading events…" />
      ) : (
        <div className="a-card !p-0">
          <div className="px-4 pt-4">
            <h2 className="font-black">What they told us</h2>
            <p className="text-xs text-muted">
              An event about somebody we cannot identify waits here rather than failing — the
              person may join next week, and matching on a name would give one Budi Santoso the
              other's race result.
            </p>
          </div>
          <div className="overflow-x-auto">
            <table className="a-table mt-2">
              <thead>
                <tr>
                  <th>Partner</th>
                  <th>What happened</th>
                  <th>They called them</th>
                  <th>We matched</th>
                  <th className="text-right">Points</th>
                  <th>When</th>
                  <th>Status</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rows.map((event) => (
                  <tr key={event.id}>
                    <td className="text-sm font-bold">{event.partnerName}</td>
                    <td className="text-sm">{event.eventType.toLowerCase().replace('_', ' ')}</td>
                    <td className="font-mono text-xs text-muted">{event.subject || '—'}</td>
                    <td className="text-sm">{event.memberName || '—'}</td>
                    <td className="text-right tabular-nums text-sm">
                      {event.xpAwarded > 0 ? event.xpAwarded : '—'}
                    </td>
                    <td className="text-sm text-muted">{formatDayTime(event.occurredAt)}</td>
                    <td>
                      <StatusBadge status={event.status} />
                    </td>
                    <td className="text-right">
                      {can('crm.manage') && event.status === 'UNMATCHED' ? (
                        <button
                          className="a-btn-ghost !px-3 !py-1 text-xs"
                          onClick={() => setMatching(event.id)}
                        >
                          Match
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="py-8 text-center text-sm text-muted">
                      {status === 'UNMATCHED' ? 'Everything has been matched.' : 'Nothing yet.'}
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {editing ? (
        <PartnerModal
          partner={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['crm', 'partners'] });
          }}
        />
      ) : null}

      {matching ? (
        <MatchModal
          eventId={matching}
          onClose={() => setMatching(null)}
          onDone={() => {
            setMatching(null);
            void qc.invalidateQueries({ queryKey: ['crm', 'events'] });
          }}
        />
      ) : null}
    </div>
  );
}

function PartnerModal({
  partner,
  onClose,
  onDone,
}: {
  partner: IntegrationPartner | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [secret, setSecret] = useState('');
  const [form, setForm] = useState({
    code: partner?.code ?? '',
    name: partner?.name ?? '',
    kind: partner?.kind ?? 'OTHER',
    contactName: partner?.contactName ?? '',
    contactEmail: partner?.contactEmail ?? '',
    awardsXp: partner?.awardsXp ?? false,
    active: partner?.active ?? true,
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.crm.partners.save({
        ...form,
        contactName: form.contactName || null,
        contactEmail: form.contactEmail || null,
        // Omitted leaves the existing secret alone: rotating it should be
        // deliberate, not a side effect of renaming a partner.
        secret: secret || undefined,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That partner did not save.'),
  });

  return (
    <Modal title={partner ? `Edit ${partner.name}` : 'New partner'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Code" hint="They post to a URL with this in it.">
            <input
              className="a-input font-mono"
              value={form.code}
              onChange={(e) => setForm((f) => ({ ...f, code: e.target.value.toUpperCase() }))}
            />
          </Field>
          <Field label="Name">
            <input
              className="a-input"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </Field>
        </div>
        <Field label="Kind">
          <SearchSelect
            value={form.kind}
            onChange={(v) => setForm((f) => ({ ...f, kind: v as IntegrationPartner['kind'] }))}
            placeholder="Search…"
            options={[
              ...PARTNER_KINDS.map((k) => ({ value: k, label: k.toLowerCase() })),
            ]}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Who to ask">
            <input
              className="a-input"
              value={form.contactName}
              onChange={(e) => setForm((f) => ({ ...f, contactName: e.target.value }))}
            />
          </Field>
          <Field label="Their email">
            <input
              className="a-input"
              value={form.contactEmail}
              onChange={(e) => setForm((f) => ({ ...f, contactEmail: e.target.value }))}
            />
          </Field>
        </div>
        <Field
          label={partner?.hasSecret ? 'Replace the shared secret' : 'Shared secret'}
          hint={
            partner?.hasSecret
              ? 'Leave empty to keep the one they already have. At least 16 characters.'
              : 'They sign every call with this. At least 16 characters, and it never leaves the server again.'
          }
        >
          <input
            className="a-input font-mono"
            type="password"
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
        </Field>
        <label className="flex items-start gap-2 text-sm font-bold">
          <input
            type="checkbox"
            className="mt-1"
            checked={form.awardsXp}
            onChange={(e) => setForm((f) => ({ ...f, awardsXp: e.target.checked }))}
          />
          <span>
            Their events award points
            <span className="block text-xs font-normal text-muted">
              Recording somebody's word and paying out on it are different levels of trust. Their
              own figure is capped either way.
            </span>
          </span>
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active}
            onChange={(e) => setForm((f) => ({ ...f, active: e.target.checked }))}
          />
          Accepting their events
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

function MatchModal({
  eventId,
  onClose,
  onDone,
}: {
  eventId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [memberId, setMemberId] = useState('');

  const { data: members } = useQuery({
    queryKey: ['members', 'all'],
    queryFn: () => api.admin.members.list(),
  });

  const save = useMutation({
    mutationFn: () => api.admin.crm.events.rematch(eventId, memberId),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That match did not save.'),
  });

  return (
    <Modal title="Who was this about?" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <p className="text-sm text-muted">
          What the partner sent stays exactly as they sent it. Only our side of the match changes.
        </p>
        <Field label="Member">
          <SearchSelect
            value={memberId}
            onChange={setMemberId}
            placeholder="Search member…"
            options={(members ?? []).map((m) => ({
              value: m.member.id,
              label: m.member.fullName,
              hint: m.member.email,
            }))}
          />
        </Field>
        <div className="flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={!memberId || save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Matching…' : 'Match it'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
