'use client';

import type { Campaign, MemberSegment, SegmentFilter } from '@hyrox/domain';
import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';
import { ErrorNote, Modal, PageTitle } from '../../../components/ui';

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
        subtitle="Segmented campaigns — sending creates in-app notifications for every member in the segment"
        actions={
          can('campaigns.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New campaign
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
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
              {(data ?? []).map((c) => (
                <tr key={c.id}>
                  <td className="font-bold">{c.name}</td>
                  <td>
                    <span className="rounded-full bg-info/15 px-2 py-0.5 text-xs font-bold text-info">
                      {SEGMENT_LABEL[c.segment]}
                    </span>
                  </td>
                  <td className="max-w-xs truncate text-muted">{c.message}</td>
                  <td className="text-muted">{formatDayTime(c.createdAt)}</td>
                  <td className="text-right font-bold">{c.sentCount ?? '—'}</td>
                  <td>
                    <StatusBadge status={c.status} />
                  </td>
                  <td className="text-right">
                    {can('campaigns.manage') ? (
                      <div className="flex justify-end gap-1.5">
                        {['DRAFT', 'SCHEDULED'].includes(c.status) ? (
                          <button className="a-btn !px-2.5 !py-1 text-xs" onClick={() => send.mutate(c.id)}>
                            Send now
                          </button>
                        ) : null}
                        <button className="a-btn-ghost !px-2.5 !py-1 text-xs" onClick={() => setEditing(c)}>
                          Edit
                        </button>
                        <button
                          className="a-btn-danger !px-2.5 !py-1 text-xs"
                          onClick={() => {
                            if (confirm(`Delete campaign "${c.name}"?`)) remove.mutate(c.id);
                          }}
                        >
                          Delete
                        </button>
                      </div>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
      const body = { name, segment, customFilter, message };
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
          <select className="a-input" value={segment} onChange={(e) => setSegment(e.target.value as MemberSegment)}>
            {Object.entries(SEGMENT_LABEL).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </select>
        </div>
        {segment === 'CUSTOM' ? (
          <div className="rounded-xl border border-line bg-surface-raised p-3">
            <p className="a-label">Custom criteria (all must match)</p>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="a-label">Preferred branch</label>
                <select className="a-input" value={branchId} onChange={(e) => setBranchId(e.target.value)}>
                  <option value="">Any</option>
                  {(branches ?? []).map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.name}
                    </option>
                  ))}
                </select>
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
            {preview.sample.length > 0 ? ` — ${preview.sample.join(', ')}${preview.count > preview.sample.length ? '…' : ''}` : ''}
          </p>
        ) : null}
        <div>
          <label className="a-label">Message</label>
          <textarea
            className="a-input min-h-24"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
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
