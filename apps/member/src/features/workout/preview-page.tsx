import { ApiError } from '@hyrox/api-client';
import type { WorkoutBlock } from '@hyrox/domain';
import { listSubstitutes } from '@hyrox/domain';
import { Spinner, formatDuration } from '@hyrox/ui';
import { ArrowLeft, CirclePlay, Footprints, Play, RefreshCcw } from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';
import { VideoSheet } from '../../components/video-sheet';
import { useExerciseLibrary, useWorkout } from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';

export function BlockLine({ block }: { block: WorkoutBlock }) {
  return (
    <div className="flex items-center gap-3">
      {block.kind === 'RUN' ? (
        <Footprints size={18} className="shrink-0 text-muted" />
      ) : (
        <span className="w-[18px] shrink-0 text-center font-black text-brand">{block.order}</span>
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-black">
          {block.exerciseName}
          {block.originalExerciseName ? (
            <span className="ml-1.5 text-xs font-bold text-muted">(for {block.originalExerciseName})</span>
          ) : null}
        </p>
        <p className="text-xs text-muted">
          {block.distanceM ? `${block.distanceM} m` : null}
          {block.reps ? `${block.reps} reps` : null}
          {block.weightNote ? ` · ${block.weightNote}` : ''}
        </p>
      </div>
      <span className="text-sm font-bold text-muted">{formatDuration(block.targetSec)}</span>
    </div>
  );
}

export function WorkoutPreviewPage() {
  const { workoutId = '' } = useParams();
  const navigate = useNavigate();
  const invalidate = useInvalidateAll();
  const { data: workout, isLoading } = useWorkout(workoutId);
  const { data: library } = useExerciseLibrary();
  const [swapTarget, setSwapTarget] = useState<WorkoutBlock | null>(null);
  const [videoTarget, setVideoTarget] = useState<{ title: string; url: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  if (isLoading || !workout) return <Spinner label="Loading workout…" />;

  const start = async () => {
    setBusy(true);
    setError('');
    try {
      const session = await api.workout.start(workout.id);
      navigate(`/workout/active/${session.session.id}`, { replace: true });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Could not start.');
    } finally {
      setBusy(false);
    }
  };

  const videoFor = (block: WorkoutBlock) =>
    library?.exercises.find((e) => e.id === block.exerciseId)?.videoUrl ?? null;

  const substitutesFor = (block: WorkoutBlock) =>
    library
      ? listSubstitutes(block.originalExerciseId ?? block.exerciseId, library.substitutions, library.exercises)
      : [];

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate('/workout')} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <div>
        <p className="text-xs font-bold uppercase tracking-wider text-muted">
          {workout.division.replaceAll('_', ' ')}
        </p>
        <h1 className="display text-3xl">{workout.type.replaceAll('_', ' ')}</h1>
        <p className="text-sm text-muted">
          {workout.blocks.length} blocks · target {formatDuration(workout.totalTargetSec)}
        </p>
      </div>

      <div className="card flex flex-col gap-3">
        {workout.blocks.map((block) => (
          <div key={block.order} className="flex items-center gap-2">
            <div className="min-w-0 flex-1">
              <BlockLine block={block} />
            </div>
            {videoFor(block) ? (
              <button
                className="shrink-0 text-muted hover:text-brand"
                aria-label="How to perform"
                onClick={() => setVideoTarget({ title: block.exerciseName, url: videoFor(block)! })}
              >
                <CirclePlay size={16} />
              </button>
            ) : null}
            {block.kind === 'STATION' && substitutesFor(block).length > 0 ? (
              <button
                className="shrink-0 text-muted hover:text-brand"
                aria-label="Swap exercise"
                onClick={() => setSwapTarget(block)}
              >
                <RefreshCcw size={15} />
              </button>
            ) : null}
          </div>
        ))}
      </div>

      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
      <button className="btn-brand flex items-center justify-center gap-2 !py-4 text-lg" disabled={busy} onClick={() => void start()}>
        <Play size={20} fill="currentColor" /> Start workout
      </button>

      {videoTarget ? (
        <VideoSheet title={videoTarget.title} videoUrl={videoTarget.url} onClose={() => setVideoTarget(null)} />
      ) : null}
      {swapTarget ? (
        <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/50" onClick={() => setSwapTarget(null)}>
          <div className="w-full max-w-md rounded-t-3xl bg-surface p-5" onClick={(e) => e.stopPropagation()}>
            <h2 className="display mb-1 text-xl">Replace {swapTarget.exerciseName}</h2>
            <p className="mb-3 text-sm text-muted">Substitutes ranked by similarity.</p>
            <div className="flex flex-col gap-2">
              {substitutesFor(swapTarget).map(({ exercise, rule }) => (
                <button
                  key={exercise.id}
                  className="card flex items-center justify-between !py-3 text-left"
                  onClick={async () => {
                    await api.workout.replaceBlock(workout.id, swapTarget.order, exercise.id);
                    setSwapTarget(null);
                    invalidate();
                  }}
                >
                  <div>
                    <p className="font-black">{exercise.name}</p>
                    <p className="text-xs text-muted">{rule.conversionNote}</p>
                  </div>
                  <span className="text-xs font-black text-brand">
                    {Math.round(rule.similarity * 100)}%
                  </span>
                </button>
              ))}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
