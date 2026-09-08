'use client';

import type { Campaign, MemberSegment, SegmentFilter } from '@nuhabit/domain';
import { Spinner, StatusBadge, formatDayTime } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';
import { Eye, Pencil, Send, Trash2 } from 'lucide-react';
import { ErrorNote, Modal, PageTitle, Pager, RowActions, SearchSelect, StatCard } from '../../../components/ui';

const SEGMENT_LABEL: Record<MemberSegment, string> = {
  ALL_ACTIVE: 'All active members',
  LOW_BALANCE: 'Low balance',
  EXPIRING_CREDITS: 'Expiring credits',
  NEW_MEMBERS: 'New members (14d)',
  NO_VISIT_14D: 'No visit in 14 days',
  CUSTOM: 'Custom audience',
};

export default function EngagementPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<Campaign | 'new' | null>(null);
  const [statusView, setStatusView] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const { data, isLoading } = useQuery({ queryKey: ['campaigns'], queryFn: api.admin.campaigns.list });

  const send = useMutation({
    mutationFn: (id: string) => api.admin.campaigns.send(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Send failed.'),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.campaigns.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  return (
    <div>
      <PageTitle
        title="Push Broadcast"
        subtitle="Segmented campaigns - sending creates in-app notifications for every member in the segment"
        actions={
          can('campaigns.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New campaign
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard tone="ink" label="Sent" value={(data ?? []).filter((c) => c.status === 'SENT').length} />
        <StatCard
          tone="lime"
          label="Draft / scheduled"
          value={(data ?? []).filter((c) => ['DRAFT', 'SCHEDULED'].includes(c.status)).length}
        />
        <StatCard
          tone="ok"
          label="Total reach"
          value={(data ?? []).reduce((sum, c) => sum + (c.sentCount ?? 0), 0)}
          hint="Notifications delivered"
        />
      </div>
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search campaign…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
        <div className="w-44">
        <SearchSelect
          value={statusView}
          onChange={(v) => {
            setStatusView(v);
            setPage(0);
          }}
          allowEmpty
          emptyLabel="All statuses"
          placeholder="Search status…"
          options={['DRAFT', 'SCHEDULED', 'SENT'].map((s) => ({ value: s, label: s }))}
        />
        </div>
      </div>
      {isLoading ? (
        <Spinner label="Loading campaigns…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Campaign</th>
                <th>Segment</th>
                <th>Message</th>
                <th>Created</th>
                <th className="text-right">Sent to</th>
                <th>Status</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {(data ?? [])
                .filter((c) => !statusView || c.status === statusView)
                .filter((c) => !query || c.name.toLowerCase().includes(query.toLowerCase()) || c.message.toLowerCase().includes(query.toLowerCase()))
                .slice(page * 8, page * 8 + 8)
                .map((c) => (
                <tr key={c.id}>
                  <td className="font-bold">{c.name}</td>
                  <td>
                    <span className="rounded-full bg-info/15 px-2 py-0.5 text-xs font-bold text-info">
                      {SEGMENT_LABEL[c.segment]}
                    </span>
                  </td>
                  <td className="max-w-xs truncate text-muted">{c.message}</td>
                  <td className="text-muted">{formatDayTime(c.createdAt)}</td>
                  <td className="text-right font-bold">{c.sentCount ?? '-'}</td>
                  <td>
                    <StatusBadge status={c.status} />
                  </td>
                  <td className="text-right">
                    {can('campaigns.manage') ? (
                      <div className="flex items-center justify-end gap-1.5">
                        {['DRAFT', 'SCHEDULED'].includes(c.status) ? (
                          <button className="a-btn !px-2.5 !py-1 text-xs" onClick={() => send.mutate(c.id)}>
                            Send now
                          </button>
                        ) : null}
                        <RowActions
                          items={[
                            { label: 'Edit', icon: Pencil, onClick: () => setEditing(c) },
                            ...(['DRAFT', 'SCHEDULED'].includes(c.status)
                              ? [{ label: 'Send now', icon: Send, onClick: () => send.mutate(c.id) }]
                              : []),
                            {
                              label: 'Delete',
                              icon: Trash2,
                              tone: 'danger' as const,
                              onClick: () => {
                                if (confirm(`Delete campaign "${c.name}"?`)) remove.mutate(c.id);
                              },
                            },
                          ]}
                        />
                      </div>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pager
            page={page}
            pageCount={Math.max(1, Math.ceil((data ?? []).filter((c) => !statusView || c.status === statusView).filter((c) => !query || c.name.toLowerCase().includes(query.toLowerCase()) || c.message.toLowerCase().includes(query.toLowerCase())).length / 8))}
            onPage={setPage}
          />
        </div>
      )}
      {editing ? (
        <CampaignModal
          campaign={editing === 'new' ? null : editing}
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

function CampaignModal({
  campaign,
  onClose,
  onDone,
}: {
  campaign: Campaign | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(campaign?.name ?? '');
  const [segment, setSegment] = useState<MemberSegment>(campaign?.segment ?? 'ALL_ACTIVE');
  const [message, setMessage] = useState(campaign?.message ?? '');
  const [branchId, setBranchId] = useState(campaign?.customFilter?.branchId ?? '');
  const [maxBalance, setMaxBalance] = useState(
    campaign?.customFilter?.maxBalance != null ? String(campaign.customFilter.maxBalance) : '',
  );
  const [minDaysSinceVisit, setMinDaysSinceVisit] = useState(
    campaign?.customFilter?.minDaysSinceLastVisit != null
      ? String(campaign.customFilter.minDaysSinceLastVisit)
      : '',
  );
  const [joinedWithinDays, setJoinedWithinDays] = useState(
    campaign?.customFilter?.joinedWithinDays != null
      ? String(campaign.customFilter.joinedWithinDays)
      : '',
  );
  const [imageUrl, setImageUrl] = useState(campaign?.imageUrl ?? '');
  const [error, setError] = useState<string | null>(null);

  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const customFilter: SegmentFilter | null =
    segment === 'CUSTOM'
      ? {
          branchId: branchId || null,
          maxBalance: maxBalance ? Number(maxBalance) : null,
          minDaysSinceLastVisit: minDaysSinceVisit ? Number(minDaysSinceVisit) : null,
          joinedWithinDays: joinedWithinDays ? Number(joinedWithinDays) : null,
        }
      : null;

  // Live audience preview while the segment is being built.
  const { data: preview } = useQuery({
    queryKey: ['segment-preview', segment, JSON.stringify(customFilter)],
    queryFn: () => api.admin.segments.preview({ segment, customFilter }),
  });

  const mutation = useMutation({
    mutationFn: () => {
      const body = {
        name,
        segment,
        customFilter,
        message,
        imageUrl: imageUrl.trim() === '' ? null : imageUrl.trim(),
      };
      return campaign
        ? api.admin.campaigns.update(campaign.id, body)
        : api.admin.campaigns.create({ ...body, deepLink: null, scheduledAt: null });
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={campaign ? `Edit ${campaign.name}` : 'New campaign'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="a-label">Audience segment</label>
          <SearchSelect
            value={segment}
            onChange={(v) => setSegment(v as MemberSegment)}
            placeholder="Search…"
            options={Object.entries(SEGMENT_LABEL).map(([k, v]) => ({ value: k, label: v }))}
          />
        </div>
        {segment === 'CUSTOM' ? (
          <div className="rounded-xl border border-line bg-surface-raised p-3">
            <p className="a-label">Custom criteria (all must match)</p>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="a-label">Preferred branch</label>
                <SearchSelect
                  value={branchId}
                  onChange={setBranchId}
                  allowEmpty
                  emptyLabel="Any"
                  placeholder="Search branch…"
                  options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
                />
              </div>
              <div>
                <label className="a-label">Balance at most</label>
                <input className="a-input" value={maxBalance} onChange={(e) => setMaxBalance(e.target.value)} placeholder="e.g. 3" />
              </div>
              <div>
                <label className="a-label">No visit for ≥ days</label>
                <input className="a-input" value={minDaysSinceVisit} onChange={(e) => setMinDaysSinceVisit(e.target.value)} placeholder="e.g. 14" />
              </div>
              <div>
                <label className="a-label">Joined within days</label>
                <input className="a-input" value={joinedWithinDays} onChange={(e) => setJoinedWithinDays(e.target.value)} placeholder="e.g. 30" />
              </div>
            </div>
          </div>
        ) : null}
        {preview ? (
          <p className="rounded-lg bg-info/10 px-3 py-2 text-sm font-bold text-info">
            Audience: {preview.count} member{preview.count === 1 ? '' : 's'}
            {preview.sample.length > 0 ? ` - ${preview.sample.join(', ')}${preview.count > preview.sample.length ? '…' : ''}` : ''}
          </p>
        ) : null}
        <div>
          <label className="a-label">Message</label>
          <textarea
            className="a-input min-h-24"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
          />
          <p className="mt-1 text-xs text-muted">
            {'{{firstName}}'}, {'{{name}}'}, {'{{email}}'} and {'{{phone}}'} are filled in per
            member. Preview it before sending — that is where you find out half the audience has
            no first name on file.
          </p>
          <CampaignPreview message={message} />
        </div>
        <div>
          <label className="a-label">Image URL (optional, shown on the member Home)</label>
          <input
            className="a-input"
            value={imageUrl}
            onChange={(e) => setImageUrl(e.target.value)}
            placeholder="/img/ann-equipment.jpg"
          />
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || name.length < 2 || message.length < 3}
          onClick={() => mutation.mutate()}
        >
          {campaign ? 'Save changes' : 'Create draft'}
        </button>
      </div>
    </Modal>
  );
}

/**
 * How a campaign will actually read, to real members.
 *
 * Not a template preview against values somebody typed: this renders against
 * particular members' real details, and it says who would not receive it at
 * all — consent is part of the question "who is getting this".
 */
function CampaignPreview({ message }: { message: string }) {
  const [memberIds, setMemberIds] = useState<string[]>([]);
  const [open, setOpen] = useState(false);

  const { data: members } = useQuery({
    queryKey: ['members', 'all'],
    queryFn: () => api.admin.members.list(),
  });

  // Three is enough to spot a missing placeholder without being a send.
  const sample = (members ?? []).slice(0, 3).map((m) => m.member.id);
  const chosen = memberIds.length > 0 ? memberIds : sample;

  const { data: previews, isFetching } = useQuery({
    queryKey: ['crm', 'campaign-preview', message, chosen],
    queryFn: () => api.admin.crm.campaigns.preview({ message, memberIds: chosen }),
    enabled: open && message.trim().length > 0 && chosen.length > 0,
  });

  if (!message.trim()) return null;

  return (
    <div className="mt-2">
      <button
        className="a-btn-ghost !px-3 !py-1 text-xs"
        onClick={() => setOpen((v) => !v)}
      >
        <Eye size={13} /> {open ? 'Hide preview' : 'Preview it'}
      </button>

      {open ? (
        <div className="mt-2 grid gap-2">
          <SearchSelect
            value={memberIds[0] ?? ''}
            onChange={(v) => setMemberIds(v ? [v] : [])}
            allowEmpty
            emptyLabel="A few members"
            placeholder="Preview as a particular member…"
            options={(members ?? []).map((m) => ({ value: m.member.id, label: m.member.fullName }))}
          />
          {isFetching ? (
            <p className="text-xs text-muted">Rendering…</p>
          ) : (
            (previews ?? []).map((preview) => (
              <div
                key={preview.memberId}
                className={`rounded-xl px-3 py-2 text-sm ${
                  preview.deliverable ? 'bg-surface-raised' : 'border border-dashed border-danger/40'
                }`}
              >
                <p className="mb-1 flex flex-wrap items-center gap-2 text-xs font-bold text-muted">
                  {preview.memberName}
                  {!preview.deliverable ? (
                    <span className="text-danger">will not receive it · {preview.skipReason}</span>
                  ) : null}
                  {preview.missing.length > 0 ? (
                    <span className="text-danger">
                      nothing to fill: {preview.missing.join(', ')}
                    </span>
                  ) : null}
                </p>
                <p>{preview.message}</p>
              </div>
            ))
          )}
        </div>
      ) : null}
    </div>
  );
}
