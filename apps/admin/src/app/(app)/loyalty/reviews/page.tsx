'use client';

import type { ReviewView } from '@nuhabit/contracts';
import { Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { EyeOff, Star } from 'lucide-react';
import { useState } from 'react';
import { ErrorNote, Modal, PageTitle, QueryError, StatCard, StatRow } from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * What members said.
 *
 * The default view is the list somebody should actually work through: poorly
 * rated and nobody has answered. Everything else is browsing.
 */
export default function ReviewsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [unansweredOnly, setUnansweredOnly] = useState(true);
  const [replying, setReplying] = useState<ReviewView | null>(null);

  const { data: reviews, isLoading, error } = useQuery({
    queryKey: ['crm', 'reviews', unansweredOnly],
    queryFn: () =>
      api.admin.crm.reviews.list({
        unansweredOnly: unansweredOnly ? 'true' : undefined,
        limit: 200,
      }),
  });

  const hide = useMutation({
    mutationFn: (id: string) => api.admin.crm.reviews.setStatus(id, 'HIDDEN'),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['crm', 'reviews'] }),
  });

  const rows = reviews ?? [];
  const published = rows.filter((r) => r.status === 'PUBLISHED');
  const average = published.length
    ? Math.round((published.reduce((s, r) => s + r.rating, 0) / published.length) * 100) / 100
    : 0;

  return (
    <div>
      <PageTitle title="Reviews" subtitle="What members said, and what we said back" />
      <QueryError error={error} />

      <StatRow>
        <StatCard tone="ink" label="Showing" value={rows.length} />
        <StatCard label="Average" value={average || '—'} tone="brand" />
        <StatCard
          label="Unanswered"
          value={rows.filter((r) => !r.reply).length}
          tone={rows.some((r) => !r.reply && r.rating <= 3) ? 'danger' : undefined}
        />
        <StatCard
          tone="danger"
          label="Backed by a visit"
          value={rows.filter((r) => r.verified).length}
          hint="An average built from unverified reviews means something else"
        />
      </StatRow>

      <label className="mb-4 flex items-center gap-2 text-sm font-bold">
        <input
          type="checkbox"
          checked={unansweredOnly}
          onChange={(e) => setUnansweredOnly(e.target.checked)}
        />
        Only the ones nobody has answered
      </label>

      {isLoading ? (
        <Spinner label="Loading reviews…" />
      ) : (
        <div className="grid gap-3">
          {rows.map((review) => (
            <div key={review.id} className="a-card">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <Stars rating={review.rating} />
                    <p className="font-bold">{review.memberName}</p>
                    {review.verified ? (
                      <span className="text-xs font-bold text-muted">was there</span>
                    ) : null}
                  </div>
                  <p className="text-xs text-muted">
                    {review.subjectType.toLowerCase()} · {new Date(review.createdAt).toLocaleDateString()}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <StatusBadge status={review.status} />
                  {can('crm.manage') ? (
                    <>
                      {!review.reply ? (
                        <button className="a-btn-ghost !px-3 !py-1 text-xs" onClick={() => setReplying(review)}>
                          Reply
                        </button>
                      ) : null}
                      {review.status === 'PUBLISHED' ? (
                        <button
                          className="a-btn-ghost !px-2 !py-1 text-xs"
                          title="Hidden, not deleted: it leaves the average and stays here"
                          onClick={() => hide.mutate(review.id)}
                        >
                          <EyeOff size={14} />
                        </button>
                      ) : null}
                    </>
                  ) : null}
                </div>
              </div>

              {review.comment ? <p className="mt-2 text-sm">{review.comment}</p> : null}
              {review.reply ? (
                <div className="mt-3 rounded-xl bg-surface-raised px-3 py-2">
                  <p className="text-xs font-bold text-muted">We replied</p>
                  <p className="text-sm">{review.reply}</p>
                </div>
              ) : null}
            </div>
          ))}
          {rows.length === 0 ? (
            <div className="a-card text-center text-sm text-muted">
              {unansweredOnly ? 'Everything has been answered.' : 'No reviews yet.'}
            </div>
          ) : null}
        </div>
      )}

      {replying ? (
        <ReplyModal
          review={replying}
          onClose={() => setReplying(null)}
          onDone={() => {
            setReplying(null);
            void qc.invalidateQueries({ queryKey: ['crm', 'reviews'] });
          }}
        />
      ) : null}
    </div>
  );
}

function Stars({ rating }: { rating: number }) {
  return (
    <span className="flex items-center gap-0.5">
      {[1, 2, 3, 4, 5].map((star) => (
        <Star
          key={star}
          size={14}
          className={star <= rating ? 'fill-brand text-brand' : 'text-line'}
        />
      ))}
    </span>
  );
}

function ReplyModal({
  review,
  onClose,
  onDone,
}: {
  review: ReviewView;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [reply, setReply] = useState('');

  const save = useMutation({
    mutationFn: () => api.admin.crm.reviews.reply(review.id, reply),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That reply did not save.'),
  });

  return (
    <Modal title={`Reply to ${review.memberName}`} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="rounded-xl bg-surface-raised px-3 py-2">
          <Stars rating={review.rating} />
          <p className="mt-1 text-sm">{review.comment ?? 'No comment.'}</p>
        </div>
        <textarea
          className="a-input min-h-[6rem]"
          placeholder="Write a reply…"
          value={reply}
          onChange={(e) => setReply(e.target.value)}
        />
        <div className="flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={!reply.trim() || save.isPending} onClick={() => save.mutate()}>
            {save.isPending ? 'Sending…' : 'Reply'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
