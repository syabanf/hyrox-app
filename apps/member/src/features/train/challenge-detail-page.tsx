import { Spinner, formatDay } from '@hyrox/ui';
import { ArrowLeft, Trophy } from 'lucide-react';
import { useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';
import { useChallenges } from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';

export function ChallengeDetailPage() {
  const { challengeId = '' } = useParams();
  const navigate = useNavigate();
  const invalidate = useInvalidateAll();
  const { data: challenges, isLoading } = useChallenges();

  if (isLoading || !challenges) return <Spinner label="Loading challenge…" />;
  const view = challenges.find((c) => c.challenge.id === challengeId);
  if (!view) return <p className="card text-sm text-muted">Challenge not found.</p>;

  const { challenge: c } = view;
  const pct = Math.min(100, (view.progressKm / c.targetKm) * 100);

  const join = async () => {
    await api.athlete.joinChallenge(c.id);
    invalidate();
  };

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>

      <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
        <div className="relative">
          <p className="flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
            <Trophy size={13} className="text-[#ff4348]" /> Challenge ·{' '}
            {formatDay(c.startsAt)} – {formatDay(c.endsAt)}
          </p>
          <p className="display mt-1 text-3xl leading-tight">{c.name}</p>
          <p className="mt-1.5 text-sm text-white/60">{c.description}</p>
          <div className="mt-5">
            <div className="flex items-baseline justify-between text-sm">
              <span className="display text-2xl">
                {view.progressKm.toFixed(1)}
                <span className="text-sm font-bold text-white/50"> / {c.targetKm} km</span>
              </span>
              <span className="font-bold text-white/60">{Math.round(pct)}%</span>
            </div>
            <div className="mt-2 h-2 overflow-hidden rounded-full bg-white/10">
              <div
                className="h-full rounded-full bg-gradient-to-r from-brand to-[#ff7a45]"
                style={{ width: `${pct}%` }}
              />
            </div>
          </div>
        </div>
      </div>

      {!view.joined ? (
        <button className="btn-brand" onClick={() => void join()}>
          Join challenge
        </button>
      ) : (
        <p className="chip self-start bg-ok/10 text-ok">You're in — {view.participantCount} athletes joined</p>
      )}

      <section>
        <p className="mb-2 px-1 text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
          Leaderboard
        </p>
        <div className="card divide-y divide-line !py-1">
          {view.leaderboard.map((row, i) => (
            <div key={`${row.memberName}-${i}`} className="flex items-center gap-3 py-3">
              <span
                className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-black ${
                  i === 0 ? 'bg-brand text-white' : 'bg-surface-raised text-muted'
                }`}
              >
                {i + 1}
              </span>
              <p className={`min-w-0 flex-1 truncate text-sm ${row.isMe ? 'font-extrabold text-brand' : 'font-bold'}`}>
                {row.memberName}
                {row.isMe ? ' (you)' : ''}
              </p>
              <span className="display text-lg">{row.km.toFixed(1)} km</span>
            </div>
          ))}
          {view.leaderboard.length === 0 ? (
            <p className="py-3 text-sm text-muted">Nobody has logged kilometres yet.</p>
          ) : null}
        </div>
      </section>
    </div>
  );
}
