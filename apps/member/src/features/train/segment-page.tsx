import { Spinner, formatDay, formatDistanceM, formatDuration } from '@hyrox/ui';
import { ArrowLeft, Crown } from 'lucide-react';
import { useNavigate, useParams } from 'react-router';
import { useSegment, useUnits } from '../../lib/athlete-queries';

export function SegmentPage() {
  const { segmentId = '' } = useParams();
  const navigate = useNavigate();
  const units = useUnits();
  const { data: v, isLoading } = useSegment(segmentId);

  if (isLoading || !v) return <Spinner label="Loading segment…" />;

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <div>
        <h1 className="display text-3xl">{v.segment.name}</h1>
        <p className="text-muted">
          {formatDistanceM(v.segment.distanceM, units)} · {v.segment.type[0] + v.segment.type.slice(1).toLowerCase()} ·{' '}
          {v.segment.location}
        </p>
        {v.myRank ? <p className="mt-1 text-sm font-black text-brand">Your rank: #{v.myRank}</p> : null}
      </div>
      <div className="card !p-0">
        <p className="label px-4 pt-4">Leaderboard (best effort per athlete)</p>
        <div className="flex flex-col">
          {v.leaderboard.map((row) => (
            <div
              key={row.memberId}
              className={`flex items-center justify-between border-t border-line px-4 py-2.5 text-sm ${
                row.isMe ? 'bg-brand/5 font-black text-brand' : ''
              }`}
            >
              <div className="flex items-center gap-3">
                <span className="w-6 font-black">{row.rank}</span>
                {row.rank === 1 ? <Crown size={14} className="text-brand" /> : null}
                <span className="font-bold">{row.memberName}</span>
              </div>
              <div className="text-right">
                <p className="font-black">{formatDuration(row.elapsedSec)}</p>
                <p className="text-xs text-muted">{formatDay(row.createdAt)}</p>
              </div>
            </div>
          ))}
          {v.leaderboard.length === 0 ? (
            <p className="px-4 py-6 text-center text-sm text-muted">No efforts yet — be the first!</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}
