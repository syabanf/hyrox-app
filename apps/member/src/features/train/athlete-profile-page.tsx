import { Spinner, formatDuration } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, UserCheck, UserPlus } from 'lucide-react';
import { useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';
import { useInvalidateAll } from '../../lib/queries';
import { ActivityCard } from './feed-page';

export function AthleteProfilePage() {
  const { memberId = '' } = useParams();
  const navigate = useNavigate();
  const invalidate = useInvalidateAll();
  const { data: p, isLoading, refetch } = useQuery({
    queryKey: ['athlete-profile', memberId],
    queryFn: () => api.athlete.profile(memberId),
  });

  if (isLoading || !p) return <Spinner label="Loading athlete…" />;

  const toggleFollow = async () => {
    await api.athlete.toggleFollow(p.member.id);
    invalidate();
    void refetch();
  };

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>

      <div className="flex items-center gap-4">
        {p.member.avatarUrl ? (
          <img src={p.member.avatarUrl} alt="" className="h-16 w-16 rounded-full object-cover" />
        ) : (
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-[#1b1b1f] text-xl font-black text-white">
            {p.member.fullName
              .split(' ')
              .slice(0, 2)
              .map((x) => x[0])
              .join('')}
          </div>
        )}
        <div className="min-w-0 flex-1">
          <h1 className="display truncate text-2xl leading-tight">{p.member.fullName}</h1>
          <p className="text-sm text-muted">
            {p.followerCount} followers · {p.followingCount} following
          </p>
        </div>
        {!p.isMe ? (
          <button
            onClick={() => void toggleFollow()}
            className={`${p.isFollowing ? 'btn-ghost' : 'btn-brand'} flex shrink-0 items-center gap-1.5 !px-4 !py-2.5 text-sm`}
          >
            {p.isFollowing ? <UserCheck size={15} /> : <UserPlus size={15} />}
            {p.isFollowing ? 'Following' : 'Follow'}
          </button>
        ) : null}
      </div>

      <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
        <div className="relative grid grid-cols-3 gap-3 text-center text-sm">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Activities</p>
            <p className="display mt-1 text-3xl">{p.totals.activities}</p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Distance</p>
            <p className="display mt-1 text-3xl">{p.totals.distanceKm.toFixed(0)} km</p>
          </div>
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.18em] text-white/50">Time</p>
            <p className="display mt-1 text-3xl">{formatDuration(p.totals.movingSec)}</p>
          </div>
        </div>
      </div>

      <section>
        <p className="mb-2 px-1 text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
          Recent activities
        </p>
        <div className="flex flex-col gap-3">
          {p.activities.map((a) => (
            <ActivityCard key={a.id} a={a} />
          ))}
          {p.activities.length === 0 ? (
            <p className="card text-sm text-muted">No visible activities yet.</p>
          ) : null}
        </div>
      </section>
    </div>
  );
}
