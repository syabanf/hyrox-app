import type { Exercise } from '@hyrox/domain';
import { Spinner } from '@hyrox/ui';
import { CirclePlay } from 'lucide-react';
import { useState } from 'react';
import { VideoSheet } from '../../components/video-sheet';
import { useExerciseLibrary } from '../../lib/athlete-queries';
import { TrainTabs } from './train-tabs';

const youtubeId = (url: string): string | null => url.split('v=')[1]?.split('&')[0] ?? null;

function TutorialRow({ ex, onPlay }: { ex: Exercise; onPlay: () => void }) {
  const id = ex.videoUrl ? youtubeId(ex.videoUrl) : null;
  const spec = [
    ex.defaultSpec.distanceM ? `${ex.defaultSpec.distanceM} m` : null,
    ex.defaultSpec.reps ? `${ex.defaultSpec.reps} reps` : null,
    ...ex.equipment,
  ]
    .filter(Boolean)
    .join(' · ');
  return (
    <button onClick={onPlay} className="card flex items-center gap-3 !p-3 text-left active:scale-[0.99]">
      <div className="relative h-16 w-24 shrink-0 overflow-hidden rounded-xl bg-ink">
        {id ? (
          <img
            src={`https://i.ytimg.com/vi/${id}/hqdefault.jpg`}
            alt=""
            className="h-full w-full object-cover"
            loading="lazy"
          />
        ) : null}
        <span className="absolute inset-0 flex items-center justify-center bg-black/25 text-white">
          <CirclePlay size={22} />
        </span>
        {ex.hyroxStationOrder ? (
          <span className="absolute left-1.5 top-1.5 flex h-5 w-5 items-center justify-center rounded-md bg-brand text-[10px] font-black text-white">
            {ex.hyroxStationOrder}
          </span>
        ) : null}
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-extrabold">{ex.name}</p>
        <p className="truncate text-xs text-muted">{spec || 'Technique guide'}</p>
      </div>
    </button>
  );
}

/** Dedicated technique library: one how-to video per exercise. */
export function TutorialsPage() {
  const { data: library, isLoading } = useExerciseLibrary();
  const [playing, setPlaying] = useState<Exercise | null>(null);

  if (isLoading || !library) return <Spinner label="Loading guides…" />;

  const withVideo = library.exercises.filter((e) => e.videoUrl);
  const stations = withVideo
    .filter((e) => e.hyroxStationOrder !== null || e.id === 'ex_run')
    .sort((a, b) => (a.hyroxStationOrder ?? 0) - (b.hyroxStationOrder ?? 0));
  const alternatives = withVideo.filter(
    (e) => e.hyroxStationOrder === null && e.id !== 'ex_run',
  );

  return (
    <div className="flex flex-col gap-5">
      <h1 className="display text-3xl">Guides</h1>
      <TrainTabs />

      <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
        <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
        <div className="relative">
          <p className="text-[10px] font-bold uppercase tracking-[0.22em] text-white/50">
            Technique library
          </p>
          <p className="display mt-1 text-2xl leading-tight">Master all 8 stations.</p>
          <p className="mt-1.5 text-sm text-white/60">
            One short video per movement — watch before you race the clock.
          </p>
        </div>
      </div>

      <section>
        <p className="mb-2 px-1 text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
          Race stations
        </p>
        <div className="flex flex-col gap-2">
          {stations.map((ex) => (
            <TutorialRow key={ex.id} ex={ex} onPlay={() => setPlaying(ex)} />
          ))}
        </div>
      </section>

      {alternatives.length > 0 ? (
        <section>
          <p className="mb-2 px-1 text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
            Substitutes
          </p>
          <div className="flex flex-col gap-2">
            {alternatives.map((ex) => (
              <TutorialRow key={ex.id} ex={ex} onPlay={() => setPlaying(ex)} />
            ))}
          </div>
        </section>
      ) : null}

      {playing?.videoUrl ? (
        <VideoSheet title={playing.name} videoUrl={playing.videoUrl} onClose={() => setPlaying(null)} />
      ) : null}
    </div>
  );
}
