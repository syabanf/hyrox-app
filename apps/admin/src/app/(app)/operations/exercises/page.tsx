'use client';

import type { Exercise } from '@hyrox/domain';
import { Spinner } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle, SearchSelect, StatCard } from '../../../../components/ui';

/** The exercise library behind the member app's Guides tab and workout player. */
export default function ExercisesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<Exercise | null>(null);
  const [categoryView, setCategoryView] = useState('');
  const { data, isLoading } = useQuery({
    queryKey: ['admin-exercises'],
    queryFn: api.admin.exercises.list,
  });

  if (isLoading) return <Spinner label="Loading exercises…" />;
  const rows = (data ?? []).filter((e) => !categoryView || e.category === categoryView);
  const manage = can('class_types.manage');

  return (
    <div>
      <PageTitle
        title="Exercise Guides"
        subtitle="Names, difficulty, and the how-to videos members see in Guides"
      />
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Exercises" value={(data ?? []).length} />
        <StatCard label="Race stations" value={(data ?? []).filter((e) => e.hyroxStationOrder !== null).length} />
        <StatCard
          label="With how-to video"
          value={(data ?? []).filter((e) => e.videoUrl).length}
          hint="Shown in the member Guides tab"
        />
      </div>
      <div className="mb-4 w-44">
        <SearchSelect
          value={categoryView}
          onChange={setCategoryView}
          allowEmpty
          emptyLabel="All categories"
          placeholder="Search category…"
          options={[...new Set((data ?? []).map((e) => e.category))].map((c) => ({ value: c, label: c }))}
        />
      </div>
      <div className="a-card !p-0">
        <table className="a-table">
          <thead>
            <tr>
              <th>Exercise</th>
              <th>Category</th>
              <th>Station</th>
              <th>Difficulty</th>
              <th>How-to video</th>
              <th className="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((e) => (
              <tr key={e.id}>
                <td className="font-bold">{e.name}</td>
                <td className="text-muted">{e.category}</td>
                <td>{e.hyroxStationOrder ? <span className="chip bg-brand/10 text-brand">#{e.hyroxStationOrder}</span> : '—'}</td>
                <td>{'●'.repeat(e.difficulty)}</td>
                <td>
                  {e.videoUrl ? (
                    <a href={e.videoUrl} target="_blank" rel="noreferrer" className="text-sm font-bold text-brand">
                      Watch →
                    </a>
                  ) : (
                    <span className="chip bg-warn/10 text-warn">Missing</span>
                  )}
                </td>
                <td className="text-right">
                  {manage ? (
                    <button className="text-sm font-bold text-brand" onClick={() => setEditing(e)}>
                      Edit
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {editing ? (
        <ExerciseModal
          exercise={editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}

function ExerciseModal({
  exercise,
  onClose,
  onDone,
}: {
  exercise: Exercise;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(exercise.name);
  const [difficulty, setDifficulty] = useState<1 | 2 | 3>(exercise.difficulty);
  const [videoUrl, setVideoUrl] = useState(exercise.videoUrl ?? '');
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      api.admin.exercises.update(exercise.id, {
        name,
        difficulty,
        videoUrl: videoUrl.trim() === '' ? null : videoUrl.trim(),
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={`Edit ${exercise.name}`} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="a-label">Difficulty</label>
          <div className="flex gap-2">
            {([1, 2, 3] as const).map((d) => (
              <button
                key={d}
                type="button"
                onClick={() => setDifficulty(d)}
                className={`rounded-full px-4 py-1.5 text-xs font-bold transition ${
                  difficulty === d ? 'bg-brand/10 text-brand ring-1 ring-brand/20' : 'bg-surface-raised text-muted'
                }`}
              >
                {'●'.repeat(d)}
              </button>
            ))}
          </div>
        </div>
        <div>
          <label className="a-label">How-to video URL (YouTube)</label>
          <input
            className="a-input"
            value={videoUrl}
            onChange={(e) => setVideoUrl(e.target.value)}
            placeholder="https://www.youtube.com/watch?v=…"
          />
          <p className="mt-1 text-xs text-muted">
            Empty removes the video from the member Guides tab and workout player.
          </p>
        </div>
        <ErrorNote message={error} />
        <button className="a-btn" disabled={mutation.isPending || name.length < 2} onClick={() => mutation.mutate()}>
          Save changes
        </button>
      </div>
    </Modal>
  );
}
