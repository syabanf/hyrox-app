import type { WorkoutSessionView } from '@hyrox/contracts';
import { Spinner, formatDuration } from '@hyrox/ui';
import { CheckCircle2, Pause, Play, Square } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router';
import { api } from '../../lib/api';
import { useInvalidateAll } from '../../lib/queries';
import { BlockLine } from './preview-page';

/** The active workout screen (blueprint §48): huge timer, one block at a time. */
export function WorkoutActivePage() {
  const { sessionId = '' } = useParams();
  const invalidate = useInvalidateAll();
  const [view, setView] = useState<(WorkoutSessionView & { activityId?: string | null }) | null>(null);
  const [totalSec, setTotalSec] = useState(0);
  const [blockSec, setBlockSec] = useState(0);
  const [paused, setPaused] = useState(false);
  const pauseStartedRef = useRef(0);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void api.workout.session(sessionId).then(setView);
  }, [sessionId]);

  useEffect(() => {
    const id = window.setInterval(() => {
      if (!paused && view && ['STARTED', 'PAUSED'].includes(view.session.status)) {
        setTotalSec((s) => s + 1);
        setBlockSec((s) => s + 1);
      }
    }, 1000);
    return () => clearInterval(id);
  }, [paused, view]);

  if (!view) return <Spinner label="Loading session…" />;
  const { session, workout } = view;
  const done = session.status === 'COMPLETED' || session.status === 'PARTIAL';
  const currentBlock = workout.blocks.find((b) => b.order === session.currentBlock);
  const completedCount = session.blockResults.length;
  const pct = Math.round((completedCount / workout.blocks.length) * 100);

  const completeBlock = async () => {
    if (!currentBlock || busy) return;
    setBusy(true);
    try {
      const updated = await api.workout.completeBlock(
        session.id,
        currentBlock.order,
        Math.max(1, blockSec),
      );
      setBlockSec(0);
      if (updated.session.blockResults.length >= workout.blocks.length) {
        const finished = await api.workout.finish(session.id, false);
        setView(finished);
        invalidate();
      } else {
        setView(updated);
      }
    } finally {
      setBusy(false);
    }
  };

  const togglePause = async () => {
    if (busy) return;
    setBusy(true);
    try {
      if (paused) {
        const pausedSec = Math.max(1, Math.round((Date.now() - pauseStartedRef.current) / 1000));
        setView(await api.workout.resume(session.id, pausedSec));
        setPaused(false);
      } else {
        pauseStartedRef.current = Date.now();
        setView(await api.workout.pause(session.id));
        setPaused(true);
      }
    } finally {
      setBusy(false);
    }
  };

  const stopAndSave = async () => {
    if (!window.confirm('Stop and save this workout as partial?')) return;
    setBusy(true);
    try {
      const finished = await api.workout.finish(session.id, true);
      setView(finished);
      invalidate();
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return (
      <div className="flex flex-col items-center gap-5 pt-6 text-center">
        <CheckCircle2 size={64} className={session.status === 'COMPLETED' ? 'text-ok' : 'text-warn'} />
        <div>
          <h1 className="display text-3xl">
            {session.status === 'COMPLETED' ? 'Workout complete' : 'Saved as partial'}
          </h1>
          <p className="mt-1 text-muted">
            {completedCount} / {workout.blocks.length} blocks · {view.completionPct}% ·{' '}
            {formatDuration(view.activeSec)} active
          </p>
        </div>
        <div className="card w-full text-left">
          <p className="label">Block results</p>
          <div className="flex flex-col gap-1.5">
            {session.blockResults.map((r) => {
              const block = workout.blocks.find((b) => b.order === r.order)!;
              return (
                <div key={r.order} className="flex justify-between text-sm">
                  <span className="font-bold">
                    {r.order}. {block.exerciseName}
                  </span>
                  <span className={r.durationSec <= block.targetSec ? 'font-black text-ok' : 'font-black text-warn'}>
                    {formatDuration(r.durationSec)}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
        {view.activityId ? (
          <Link to={`/train/activities/${view.activityId}`} className="btn-brand w-full">
            View in training log
          </Link>
        ) : null}
        <Link to="/workout" className="text-sm font-bold text-brand">
          Back to workouts
        </Link>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="card flex flex-col items-center !py-6">
        <p className="label !mb-0">Total time</p>
        <p className="display text-6xl leading-none">{formatDuration(totalSec)}</p>
        <p className="mt-2 text-xs font-black uppercase tracking-widest text-muted">
          Block {completedCount + 1} / {workout.blocks.length}
        </p>
      </div>

      {currentBlock ? (
        <div className="card flex flex-col items-center gap-1 !border-brand !py-6 text-center">
          <p className="display text-3xl">{currentBlock.exerciseName}</p>
          {currentBlock.originalExerciseName ? (
            <p className="text-xs font-bold text-muted">substituting {currentBlock.originalExerciseName}</p>
          ) : null}
          <p className="text-sm font-bold text-muted">
            {currentBlock.distanceM ? `${currentBlock.distanceM} m` : null}
            {currentBlock.reps ? `${currentBlock.reps} reps` : null}
            {currentBlock.weightNote ? ` · ${currentBlock.weightNote}` : ''}
            {' · target '}
            {formatDuration(currentBlock.targetSec)}
          </p>
          <p className="display mt-2 text-5xl text-brand">{formatDuration(blockSec)}</p>
        </div>
      ) : null}

      <button className="btn-brand !py-4 text-lg" disabled={busy || paused} onClick={() => void completeBlock()}>
        Complete block
      </button>
      <div className="grid grid-cols-2 gap-3">
        <button className="btn-ghost flex items-center justify-center gap-2" disabled={busy} onClick={() => void togglePause()}>
          {paused ? <Play size={18} /> : <Pause size={18} />}
          {paused ? 'Resume' : 'Pause'}
        </button>
        <button className="btn-ghost flex items-center justify-center gap-2 text-danger" disabled={busy} onClick={() => void stopAndSave()}>
          <Square size={16} fill="currentColor" /> Stop &amp; save
        </button>
      </div>

      <div className="h-2.5 overflow-hidden rounded-full bg-surface-raised">
        <div className="h-full rounded-full bg-brand transition-all" style={{ width: `${pct}%` }} />
      </div>
      <p className="text-center text-xs font-black uppercase tracking-widest text-muted">{pct}% complete</p>

      <div className="card">
        <p className="label">Up next</p>
        <div className="flex flex-col gap-2 opacity-70">
          {workout.blocks
            .filter((b) => b.order > session.currentBlock)
            .slice(0, 4)
            .map((b) => (
              <BlockLine key={b.order} block={b} />
            ))}
        </div>
      </div>
    </div>
  );
}
