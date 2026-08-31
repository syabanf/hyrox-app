import { Spinner, formatDay, formatDistanceM, formatDuration } from '@hyrox/ui';
import { Check, Play, Trash2, Trophy, Users } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { RouteMap } from '../../components/route-map';
import { api } from '../../lib/api';
import {
  useChallenges,
  useClubs,
  useFollowMutation,
  useRoutes,
  useSegments,
  useSocial,
  useUnits,
} from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';
import { TrainTabs } from './train-tabs';

const TABS = ['Segments', 'Routes', 'Challenges', 'Clubs', 'Athletes'] as const;
type Tab = (typeof TABS)[number];

export function ExplorePage() {
  const [tab, setTab] = useState<Tab>('Segments');
  return (
    <div className="flex flex-col gap-4">
      <h1 className="display text-2xl">Explore</h1>
      <TrainTabs />
      <div className="-mt-1 flex gap-2 overflow-x-auto pb-1">
        {TABS.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`whitespace-nowrap rounded-full px-4 py-1.5 text-sm font-bold ${
              tab === t ? 'bg-ink text-white' : 'bg-surface text-muted'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      {tab === 'Segments' ? <SegmentsTab /> : null}
      {tab === 'Routes' ? <RoutesTab /> : null}
      {tab === 'Challenges' ? <ChallengesTab /> : null}
      {tab === 'Clubs' ? <ClubsTab /> : null}
      {tab === 'Athletes' ? <AthletesTab /> : null}
    </div>
  );
}

function SegmentsTab() {
  const units = useUnits();
  const { data, isLoading } = useSegments();
  if (isLoading) return <Spinner label="Loading segments…" />;
  return (
    <div className="flex flex-col gap-2">
      {(data ?? []).map((v) => (
        <Link key={v.segment.id} to={`/train/segments/${v.segment.id}`} className="card flex items-center justify-between">
          <div>
            <p className="font-black">{v.segment.name}</p>
            <p className="text-sm text-muted">
              {formatDistanceM(v.segment.distanceM, units)} · {v.segment.location} · {v.effortCount} efforts
            </p>
            {v.myRank ? (
              <p className="text-xs font-bold text-brand">
                Your rank #{v.myRank} · {v.myBestElapsedSec ? formatDuration(v.myBestElapsedSec) : ''}
              </p>
            ) : null}
          </div>
          <div className="text-right text-sm">
            <p className="label !mb-0">Record</p>
            <p className="font-black">{v.bestElapsedSec ? formatDuration(v.bestElapsedSec) : '—'}</p>
          </div>
        </Link>
      ))}
    </div>
  );
}

function RoutesTab() {
  const units = useUnits();
  const navigate = useNavigate();
  const invalidate = useInvalidateAll();
  const { data: routes, isLoading } = useRoutes();
  if (isLoading) return <Spinner label="Loading routes…" />;
  if (!routes || routes.length === 0) {
    return (
      <p className="card text-sm text-muted">
        No saved routes yet. Open one of your activities and choose "Save as route".
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-3">
      {routes.map((route) => (
        <div key={route.id} className="card">
          <div className="flex items-start justify-between gap-2">
            <div>
              <p className="font-black">{route.name}</p>
              <p className="text-sm text-muted">
                {formatDistanceM(route.distanceM, units)} · saved {formatDay(route.createdAt)}
              </p>
            </div>
            <button
              className="text-muted hover:text-danger"
              aria-label="Delete route"
              onClick={async () => {
                if (!window.confirm(`Delete route "${route.name}"?`)) return;
                await api.athlete.deleteRoute(route.id);
                invalidate();
              }}
            >
              <Trash2 size={16} />
            </button>
          </div>
          <div className="mt-2">
            <RouteMap points={route.points} height={110} />
          </div>
          <button
            className="btn-brand mt-3 flex w-full items-center justify-center gap-2 !py-2 text-sm"
            onClick={() => navigate(`/train/record?route=${route.id}`)}
          >
            <Play size={14} fill="currentColor" /> Use this route
          </button>
        </div>
      ))}
    </div>
  );
}

function ChallengesTab() {
  const { data, isLoading } = useChallenges();
  const invalidate = useInvalidateAll();
  if (isLoading) return <Spinner label="Loading challenges…" />;
  return (
    <div className="flex flex-col gap-3">
      {(data ?? []).map((v) => {
        const pct = Math.min(1, v.progressKm / v.challenge.targetKm);
        return (
          <div key={v.challenge.id} className="card">
            <div className="flex items-start justify-between gap-2">
              <div>
                <p className="font-black">{v.challenge.name}</p>
                <p className="text-sm text-muted">{v.challenge.description}</p>
              </div>
              <Trophy size={20} className="shrink-0 text-brand" />
            </div>
            {v.joined ? (
              <>
                <div className="mt-3 h-2.5 overflow-hidden rounded-full bg-surface-raised">
                  <div className="h-full rounded-full bg-brand" style={{ width: `${pct * 100}%` }} />
                </div>
                <p className="mt-1 text-xs font-bold text-muted">
                  {v.progressKm.toFixed(1)} / {v.challenge.targetKm} km · {v.participantCount} athletes
                </p>
                {v.leaderboard.length > 0 ? (
                  <div className="mt-2 flex flex-col gap-1 text-sm">
                    {v.leaderboard.map((r, i) => (
                      <div key={r.memberName} className={`flex justify-between ${r.isMe ? 'font-black text-brand' : ''}`}>
                        <span>
                          {i + 1}. {r.memberName}
                        </span>
                        <span>{r.km.toFixed(1)} km</span>
                      </div>
                    ))}
                  </div>
                ) : null}
              </>
            ) : (
              <button
                className="btn-brand mt-3 !py-2 text-sm"
                onClick={async () => {
                  await api.athlete.joinChallenge(v.challenge.id);
                  invalidate();
                }}
              >
                Join challenge
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ClubsTab() {
  const { data, isLoading } = useClubs();
  const invalidate = useInvalidateAll();
  if (isLoading) return <Spinner label="Loading clubs…" />;
  return (
    <div className="flex flex-col gap-3">
      {(data ?? []).map((v) => (
        <div key={v.club.id} className="card">
          <div className="flex items-start justify-between gap-2">
            <div>
              <p className="font-black">{v.club.name}</p>
              <p className="text-sm text-muted">{v.club.description}</p>
              <p className="mt-1 flex items-center gap-1 text-xs font-bold text-muted">
                <Users size={12} /> {v.memberCount} members · {v.club.location}
              </p>
            </div>
            <button
              className={`shrink-0 rounded-full px-4 py-1.5 text-xs font-black uppercase ${
                v.joined ? 'bg-surface-raised text-muted' : 'bg-brand text-white'
              }`}
              onClick={async () => {
                await api.athlete.toggleClub(v.club.id);
                invalidate();
              }}
            >
              {v.joined ? 'Leave' : 'Join'}
            </button>
          </div>
          {v.joined && v.weeklyLeaderboard.length > 0 ? (
            <div className="mt-3 border-t border-line pt-2">
              <p className="label">This week</p>
              <div className="flex flex-col gap-1 text-sm">
                {v.weeklyLeaderboard.map((r, i) => (
                  <div key={r.memberName} className={`flex justify-between ${r.isMe ? 'font-black text-brand' : ''}`}>
                    <span>
                      {i + 1}. {r.memberName}
                    </span>
                    <span>{r.km.toFixed(1)} km</span>
                  </div>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function AthletesTab() {
  const { data, isLoading } = useSocial();
  const follow = useFollowMutation();
  if (isLoading || !data) return <Spinner label="Loading athletes…" />;

  const Row = ({ memberId, name, weeklyKm, isFollowing }: (typeof data.suggestions)[number]) => (
    <div className="card flex items-center justify-between !py-3">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-full bg-brand text-xs font-black text-white">
          {name
            .split(' ')
            .slice(0, 2)
            .map((p) => p[0])
            .join('')}
        </div>
        <div>
          <p className="text-sm font-black">{name}</p>
          <p className="text-xs text-muted">{weeklyKm.toFixed(1)} km this week</p>
        </div>
      </div>
      <button
        className={`flex items-center gap-1 rounded-full px-4 py-1.5 text-xs font-black uppercase ${
          isFollowing ? 'bg-surface-raised text-muted' : 'bg-brand text-white'
        }`}
        disabled={follow.isPending}
        onClick={() => follow.mutate(memberId)}
      >
        {isFollowing ? (
          <>
            <Check size={12} /> Following
          </>
        ) : (
          'Follow'
        )}
      </button>
    </div>
  );

  return (
    <div className="flex flex-col gap-4">
      {data.suggestions.length > 0 ? (
        <section>
          <p className="label">Suggested</p>
          <div className="flex flex-col gap-2">
            {data.suggestions.map((a) => (
              <Row key={a.memberId} {...a} />
            ))}
          </div>
        </section>
      ) : null}
      <section>
        <p className="label">Following ({data.following.length})</p>
        <div className="flex flex-col gap-2">
          {data.following.map((a) => (
            <Row key={a.memberId} {...a} />
          ))}
        </div>
      </section>
      <section>
        <p className="label">Followers ({data.followers.length})</p>
        <div className="flex flex-col gap-2">
          {data.followers.map((a) => (
            <Row key={a.memberId} {...a} />
          ))}
        </div>
      </section>
    </div>
  );
}
