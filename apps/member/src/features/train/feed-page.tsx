import type { ActivityCardView } from '@hyrox/contracts';
import {
  EmptyState,
  Spinner,
  formatDayTime,
  formatDistanceM,
  formatDuration,
  formatPace,
  formatSpeedKmh,
} from '@hyrox/ui';
import { MessageCircle, ThumbsUp } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router';
import { RouteMap } from '../../components/route-map';
import { useFeed, useKudosMutation, useUnits } from '../../lib/athlete-queries';
import { TrainTabs } from './train-tabs';

export function ActivityStatsRow({ a }: { a: ActivityCardView }) {
  const units = useUnits();
  return (
    <div className="flex gap-5 text-sm">
      <div>
        <p className="label !mb-0">Distance</p>
        <p className="display text-lg">{a.type === 'WORKOUT' ? '—' : formatDistanceM(a.distanceM, units)}</p>
      </div>
      <div>
        <p className="label !mb-0">{a.type === 'RIDE' ? 'Speed' : 'Pace'}</p>
        <p className="display text-lg">
          {a.type === 'RIDE'
            ? formatSpeedKmh(a.distanceM, a.movingSec, units)
            : formatPace(a.avgPaceSecPerKm, units)}
        </p>
      </div>
      <div>
        <p className="label !mb-0">Time</p>
        <p className="display text-lg">{formatDuration(a.movingSec)}</p>
      </div>
    </div>
  );
}

export function ActivityCard({ a }: { a: ActivityCardView }) {
  const kudos = useKudosMutation();
  return (
    <div className="card !p-0">
      <Link to={`/train/athletes/${a.memberId}`} className="flex items-center gap-3 px-4 pt-4">
        {a.memberAvatarUrl ? (
          <img src={a.memberAvatarUrl} alt="" className="h-10 w-10 rounded-full object-cover" />
        ) : (
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-[#1b1b1f] text-sm font-black text-white">
            {a.memberName
              .split(' ')
              .slice(0, 2)
              .map((p) => p[0])
              .join('')}
          </div>
        )}
        <div className="min-w-0">
          <p className="truncate text-sm font-black">{a.memberName}</p>
          <p className="text-xs text-muted">
            {formatDayTime(a.startedAt)} · {a.type[0] + a.type.slice(1).toLowerCase()}
          </p>
        </div>
      </Link>
      <Link to={`/train/activities/${a.id}`} className="block px-4 pt-3">
        <p className="display text-xl">{a.title}</p>
        <div className="mt-2">
          <ActivityStatsRow a={a} />
        </div>
        {a.thumbnail.length > 1 ? (
          <div className="mt-3">
            <RouteMap points={a.thumbnail} height={140} />
          </div>
        ) : null}
      </Link>
      <div className="mt-2 flex items-center gap-4 border-t border-line px-4 py-2.5">
        <button
          onClick={() => kudos.mutate(a.id)}
          disabled={kudos.isPending}
          className={`flex items-center gap-1.5 text-sm font-bold ${
            a.hasKudoed ? 'text-brand' : 'text-muted'
          }`}
        >
          <ThumbsUp size={16} fill={a.hasKudoed ? 'currentColor' : 'none'} />
          {a.kudosCount}
        </button>
        <Link
          to={`/train/activities/${a.id}`}
          className="flex items-center gap-1.5 text-sm font-bold text-muted"
        >
          <MessageCircle size={16} />
          {a.commentCount}
        </Link>
      </div>
    </div>
  );
}

export function FeedPage() {
  const [scope, setScope] = useState<'following' | 'everyone'>('everyone');
  const { data: feed, isLoading } = useFeed(scope);

  return (
    <div className="flex flex-col gap-5">
      <h1 className="display text-3xl">Train</h1>
      <TrainTabs />
      <div className="-mt-1 flex gap-2">
        {(['everyone', 'following'] as const).map((s) => (
          <button
            key={s}
            onClick={() => setScope(s)}
            className={`rounded-full px-4 py-1.5 text-sm font-bold capitalize ${
              scope === s ? 'bg-ink text-white' : 'bg-surface text-muted'
            }`}
          >
            {s}
          </button>
        ))}
      </div>
      {isLoading ? (
        <Spinner label="Loading feed…" />
      ) : !feed || feed.length === 0 ? (
        <EmptyState
          title="Quiet in here"
          hint={scope === 'following' ? 'Follow athletes in Explore to fill your feed.' : 'Record your first activity!'}
          action={
            <Link to="/train/record" className="btn-brand mt-2 !py-2 text-sm">
              Record an activity
            </Link>
          }
        />
      ) : (
        <div className="flex flex-col gap-3">
          {feed.map((a) => (
            <ActivityCard key={a.id} a={a} />
          ))}
        </div>
      )}
    </div>
  );
}
