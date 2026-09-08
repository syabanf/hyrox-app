'use client';

import { useMutation, useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../lib/api';
import { ErrorNote, Modal, SearchSelect } from './ui';

/**
 * Scheduling a class.
 *
 * `coachId` opens it with the trainer already chosen, which is how the Coaches
 * page and a coach-filtered schedule create one: the answer to "who is
 * teaching this" is the thing they already had in mind.
 */
export function CreateSessionModal({
  coachId: initialCoachId,
  onClose,
  onDone,
}: {
  coachId?: string | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const { data: classTypes } = useQuery({ queryKey: ['class-types'], queryFn: api.admin.classTypes.list });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data: coaches } = useQuery({ queryKey: ['coaches'], queryFn: api.admin.coaches.list });
  const [classTypeId, setClassTypeId] = useState('');
  const [coachId, setCoachId] = useState(initialCoachId ?? '');
  // A coach belongs to a branch, so choosing one answers the branch too. It
  // stays editable: a coach can cover a class at the other site.
  const coachBranchId = (coaches ?? []).find((c) => c.id === coachId)?.branchId ?? '';
  const [branchOverride, setBranchOverride] = useState('');
  const branchId = branchOverride || coachBranchId;
  const setBranchId = setBranchOverride;
  const [startsAt, setStartsAt] = useState('');
  const [capacity, setCapacity] = useState('');
  const [publish, setPublish] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      api.admin.sessions.create({
        classTypeId,
        branchId,
        coachId,
        startsAt: new Date(startsAt).toISOString(),
        capacity: capacity ? Number(capacity) : undefined,
        area: null,
        publish,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Create failed.'),
  });

  return (
    <Modal title="New class session" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Class type</label>
          <SearchSelect
            value={classTypeId}
            onChange={setClassTypeId}
            placeholder="Search class type…"
            options={(classTypes ?? []).map((t) => ({
              value: t.id,
              label: t.name,
              hint: `${t.defaultCreditCost} cr · cap ${t.defaultCapacity}`,
            }))}
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Branch</label>
            <SearchSelect
              value={branchId}
              onChange={setBranchId}
              placeholder="Search branch…"
              options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
            />
          </div>
          <div>
            <label className="a-label">Coach</label>
            <SearchSelect
              value={coachId}
              onChange={setCoachId}
              placeholder="Search coach…"
              options={(coaches ?? [])
                .filter((c) => !branchId || c.branchId === branchId)
                .map((c) => ({ value: c.id, label: c.name, hint: c.specialization }))}
            />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Starts at</label>
            <input
              type="datetime-local"
              className="a-input"
              value={startsAt}
              onChange={(e) => setStartsAt(e.target.value)}
            />
          </div>
          <div>
            <label className="a-label">Capacity (blank = type default)</label>
            <input className="a-input" value={capacity} onChange={(e) => setCapacity(e.target.value)} />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={publish} onChange={(e) => setPublish(e.target.checked)} />
          Publish immediately (bookable)
        </label>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || !classTypeId || !branchId || !coachId || !startsAt}
          onClick={() => mutation.mutate()}
        >
          Create session
        </button>
      </div>
    </Modal>
  );
}
