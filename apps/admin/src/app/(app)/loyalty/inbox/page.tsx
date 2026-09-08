'use client';

import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Send, StickyNote } from 'lucide-react';
import { useState } from 'react';
import { ErrorNote, PageTitle, QueryError, SearchSelect, StatCard } from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * The inbox.
 *
 * The queue is ordered by who has waited longest without an answer, not by
 * when a thread was opened — a conversation somebody replied to an hour ago is
 * not urgent, however old it is.
 */
export default function InboxPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [selected, setSelected] = useState<string | null>(null);
  const [filter, setFilter] = useState('');

  const { data: overview, error } = useQuery({
    queryKey: ['crm', 'inbox'],
    queryFn: api.admin.crm.inbox,
    refetchInterval: 30_000,
  });
  const { data: conversations, isLoading } = useQuery({
    queryKey: ['crm', 'conversations', filter],
    queryFn: () => api.admin.crm.conversations.list({ status: filter || undefined, limit: 100 }),
  });

  const active = selected ?? conversations?.[0]?.id ?? null;

  return (
    <div>
      <PageTitle title="Inbox" subtitle="Who is waiting, and how long they have waited" />
      <QueryError error={error} />

      {overview ? (
        <div className="mb-4 grid gap-3 sm:grid-cols-4">
          <StatCard label="Waiting" value={overview.metrics.waiting} tone="brand" />
          <StatCard
            label="Nobody has picked up"
            value={overview.metrics.unassigned}
            tone={overview.metrics.unassigned > 0 ? 'danger' : undefined}
          />
          <StatCard tone="ok"
            label="Typical first reply"
            value={duration(overview.metrics.medianFirstResponseSeconds)}
            hint="A median — one late thread should not sink a good day"
          />
          <StatCard
            label="Longest wait"
            value={duration(overview.metrics.longestWaitSeconds)}
          />
        </div>
      ) : null}

      <div className="mb-3 flex gap-1 rounded-xl border border-line p-1">
        {[
          { value: '', label: 'Everything' },
          { value: 'OPEN', label: 'Open' },
          { value: 'PENDING', label: 'Waiting on them' },
          { value: 'RESOLVED', label: 'Resolved' },
        ].map((tab) => (
          <button
            key={tab.value}
            onClick={() => setFilter(tab.value)}
            className={`rounded-lg px-3 py-1 text-xs font-bold transition ${
              filter === tab.value ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Spinner label="Loading the inbox…" />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[20rem_1fr]">
          <div className="a-card !p-0">
            <div className="max-h-[36rem] overflow-y-auto">
              {(conversations ?? []).map((conversation) => (
                <button
                  key={conversation.id}
                  onClick={() => setSelected(conversation.id)}
                  className={`block w-full border-b border-line px-3 py-3 text-left transition hover:bg-surface-raised ${
                    active === conversation.id ? 'bg-surface-raised' : ''
                  }`}
                >
                  <div className="flex items-start justify-between gap-2">
                    <p className="truncate font-bold">{conversation.memberName}</p>
                    {conversation.waiting ? (
                      <span className="shrink-0 text-xs font-bold text-danger">
                        {duration(conversation.waitedSeconds)}
                      </span>
                    ) : null}
                  </div>
                  <p className="truncate text-xs text-muted">
                    {conversation.channel.toLowerCase()} · {conversation.subject || 'no subject'}
                  </p>
                </button>
              ))}
              {(conversations ?? []).length === 0 ? (
                <p className="p-6 text-center text-sm text-muted">Nothing here.</p>
              ) : null}
            </div>
          </div>

          {active ? (
            <Thread
              conversationId={active}
              canReply={can('crm.manage')}
              onChanged={() => {
                void qc.invalidateQueries({ queryKey: ['crm', 'inbox'] });
                void qc.invalidateQueries({ queryKey: ['crm', 'conversations'] });
              }}
            />
          ) : (
            <div className="a-card text-center text-sm text-muted">
              Pick a conversation.
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function Thread({
  conversationId,
  canReply,
  onChanged,
}: {
  conversationId: string;
  canReply: boolean;
  onChanged: () => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [body, setBody] = useState('');

  const { data: conversation, isLoading } = useQuery({
    queryKey: ['crm', 'conversation', conversationId],
    queryFn: () => api.admin.crm.conversations.get(conversationId),
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['crm', 'conversation', conversationId] });
    onChanged();
  };

  const reply = useMutation({
    mutationFn: (internal: boolean) =>
      api.admin.crm.conversations.reply(conversationId, { body, internal }),
    onSuccess: () => {
      setError(null);
      setBody('');
      refresh();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That message did not send.'),
  });

  const setStatus = useMutation({
    mutationFn: (status: string) => api.admin.crm.conversations.update(conversationId, { status }),
    onSuccess: refresh,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That did not save.'),
  });

  if (isLoading || !conversation) return <Spinner label="Loading…" />;

  return (
    <div className="a-card">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-2 border-b border-line pb-3">
        <div>
          <p className="font-black">{conversation.memberName}</p>
          <p className="text-xs text-muted">
            {conversation.subject || 'no subject'} · {conversation.channel.toLowerCase()}
            {conversation.firstResponseSeconds != null
              ? ` · first reply in ${duration(conversation.firstResponseSeconds)}`
              : ''}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <StatusBadge status={conversation.status} />
          {canReply && conversation.status !== 'CLOSED' ? (
            <div className="!py-1 text-xs">
              <SearchSelect
                value=""
                onChange={(v) => v && setStatus.mutate(v)}
                allowEmpty
                emptyLabel="Move to…"
                placeholder="Search…"
                options={[
                  { value: 'OPEN', label: 'Open' },
                  { value: 'PENDING', label: 'Waiting on them' },
                  { value: 'RESOLVED', label: 'Resolved' },
                  { value: 'CLOSED', label: 'Closed' },
                ]}
              />
            </div>
          ) : null}
        </div>
      </div>

      <ErrorNote message={error} />

      <div className="mb-3 max-h-96 space-y-3 overflow-y-auto">
        {conversation.messages.map((message) => (
          <div
            key={message.id}
            className={`max-w-[80%] rounded-xl px-3 py-2 text-sm ${
              message.direction === 'INBOUND'
                ? 'bg-surface-raised'
                : message.internal
                  ? 'ml-auto border border-dashed border-line bg-transparent'
                  : 'ml-auto bg-brand text-white'
            }`}
          >
            {message.internal ? (
              <p className="mb-1 flex items-center gap-1 text-xs font-bold text-muted">
                <StickyNote size={12} /> internal note
              </p>
            ) : null}
            <p className="whitespace-pre-wrap">{message.body}</p>
            <p
              className={`mt-1 text-xs ${
                message.direction === 'OUTBOUND' && !message.internal ? 'text-white/70' : 'text-muted'
              }`}
            >
              {message.authorName ?? conversation.memberName} ·{' '}
              {new Date(message.createdAt).toLocaleString()}
            </p>
          </div>
        ))}
        {conversation.messages.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted">Nothing said yet.</p>
        ) : null}
      </div>

      {canReply && conversation.status !== 'CLOSED' ? (
        <div className="grid gap-2">
          <textarea
            className="a-input min-h-[5rem]"
            placeholder="Write a reply…"
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
          <div className="flex justify-end gap-2">
            {/* A note to colleagues is not an answer, and does not stop the
                clock. Making that a separate button is the only way it stays
                true in practice. */}
            <button
              className="a-btn-ghost"
              disabled={!body.trim() || reply.isPending}
              onClick={() => reply.mutate(true)}
            >
              <StickyNote size={14} /> Note
            </button>
            <button
              className="a-btn"
              disabled={!body.trim() || reply.isPending}
              onClick={() => reply.mutate(false)}
            >
              <Send size={14} /> Reply
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

/** Seconds as something a person reads: "4m", "2h", "3d". */
function duration(seconds: number): string {
  if (seconds <= 0) return '—';
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86_400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86_400)}d`;
}
