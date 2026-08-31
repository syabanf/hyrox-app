import { ApiError } from '@hyrox/api-client';
import type { ActivityVisibility } from '@hyrox/domain';
import {
  Spinner,
  formatDayTime,
  formatDistanceM,
  formatDuration,
  formatPace,
  formatSpeedKmh,
} from '@hyrox/ui';
import { ArrowLeft, Award, MessageCircle, MoreHorizontal, ThumbsUp, Users } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { GeoMap } from '../../components/geo-map';
import { api } from '../../lib/api';
import { useActivity, useKudosMutation, useUnits } from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';

export function ActivityDetailPage() {
  const { activityId = '' } = useParams();
  const navigate = useNavigate();
  const units = useUnits();
  const { data: a, isLoading } = useActivity(activityId);
  const kudos = useKudosMutation();
  const invalidate = useInvalidateAll();
  const [comment, setComment] = useState('');
  const [busy, setBusy] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [notice, setNotice] = useState('');

  if (isLoading || !a) return <Spinner label="Loading activity…" />;

  const sendComment = async () => {
    if (!comment.trim()) return;
    setBusy(true);
    try {
      await api.athlete.comment(a.id, comment.trim());
      setComment('');
      invalidate();
    } finally {
      setBusy(false);
    }
  };

  const saveAsRoute = async () => {
    const name = window.prompt('Route name?', `${a.title} route`);
    if (!name) return;
    try {
      await api.athlete.saveRoute(a.id, name);
      setNotice(`Route "${name}" saved — find it in Explore › Routes.`);
      invalidate();
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : 'Could not save route.');
    }
    setMenuOpen(false);
  };

  const deleteActivity = async () => {
    if (!window.confirm('Delete this activity? This cannot be undone.')) return;
    await api.athlete.deleteActivity(a.id);
    invalidate();
    navigate('/train/you', { replace: true });
  };

  const maxSplitPace = Math.max(...a.splits.map((s) => s.paceSecPerKm), 1);

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center justify-between">
        <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
          <ArrowLeft size={16} /> Back
        </button>
        {a.isOwn ? (
          <div className="relative">
            <button
              onClick={() => setMenuOpen((o) => !o)}
              className="rounded-full border border-line bg-surface p-2"
              aria-label="Activity menu"
            >
              <MoreHorizontal size={16} />
            </button>
            {menuOpen ? (
              <div className="absolute right-0 top-10 z-20 w-44 rounded-xl border border-line bg-surface p-1 shadow-lg">
                <button
                  className="w-full rounded-lg px-3 py-2 text-left text-sm font-bold hover:bg-surface-raised"
                  onClick={() => {
                    setMenuOpen(false);
                    setEditOpen(true);
                  }}
                >
                  Edit activity
                </button>
                {a.points.length > 1 ? (
                  <button
                    className="w-full rounded-lg px-3 py-2 text-left text-sm font-bold hover:bg-surface-raised"
                    onClick={() => void saveAsRoute()}
                  >
                    Save as route
                  </button>
                ) : null}
                <button
                  className="w-full rounded-lg px-3 py-2 text-left text-sm font-bold text-danger hover:bg-surface-raised"
                  onClick={() => void deleteActivity()}
                >
                  Delete
                </button>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>

      <div>
        <p className="text-xs font-bold uppercase tracking-wider text-muted">
          {a.memberName} · {formatDayTime(a.startedAt)} · {a.type[0] + a.type.slice(1).toLowerCase()}
          {a.visibility !== 'EVERYONE' ? ` · ${a.visibility.toLowerCase()}` : ''}
        </p>
        <h1 className="display text-3xl">{a.title}</h1>
        {a.description ? <p className="mt-1 text-sm text-muted">{a.description}</p> : null}
      </div>

      {notice ? <p className="rounded-xl bg-ok/10 px-3 py-2 text-sm font-bold text-ok">{notice}</p> : null}

      {a.points.length > 1 ? <GeoMap tracks={[{ points: a.points }]} height={240} /> : null}

      {a.photos.length > 0 ? (
        <div className="grid grid-cols-2 gap-2">
          {a.photos.map((p, i) => (
            <img key={i} src={p} alt="" className="h-40 w-full rounded-xl object-cover" />
          ))}
        </div>
      ) : null}

      <div className="card grid grid-cols-3 gap-3 text-center">
        <div>
          <p className="label !mb-0">Distance</p>
          <p className="display text-2xl">{a.type === 'WORKOUT' ? '—' : formatDistanceM(a.distanceM, units)}</p>
        </div>
        <div>
          <p className="label !mb-0">Moving time</p>
          <p className="display text-2xl">{formatDuration(a.movingSec)}</p>
        </div>
        <div>
          <p className="label !mb-0">{a.type === 'RIDE' ? 'Avg speed' : 'Avg pace'}</p>
          <p className="display text-2xl">
            {a.type === 'RIDE'
              ? formatSpeedKmh(a.distanceM, a.movingSec, units)
              : formatPace(a.avgPaceSecPerKm, units)}
          </p>
        </div>
        <div>
          <p className="label !mb-0">Elapsed</p>
          <p className="font-bold">{formatDuration(a.elapsedSec)}</p>
        </div>
        <div>
          <p className="label !mb-0">Elev gain</p>
          <p className="font-bold">{a.elevationGainM > 0 ? `${a.elevationGainM} m` : '—'}</p>
        </div>
        <div>
          <p className="label !mb-0">Gear</p>
          <p className="truncate font-bold">{a.gearName ?? '—'}</p>
        </div>
      </div>

      {a.groupedWith.length > 0 ? (
        <div className="card !py-3">
          <p className="flex items-center gap-2 text-sm font-bold">
            <Users size={16} className="text-brand" />
            Trained together with{' '}
            {a.groupedWith.map((g, i) => (
              <span key={g.activityId}>
                {i > 0 ? ', ' : ''}
                <Link to={`/train/activities/${g.activityId}`} className="text-brand">
                  {g.memberName}
                </Link>
              </span>
            ))}
          </p>
        </div>
      ) : null}

      {a.splits.length > 0 ? (
        <div className="card">
          <p className="label">Splits</p>
          <div className="flex flex-col gap-1.5">
            {a.splits.map((s) => (
              <div key={s.km} className="flex items-center gap-3 text-sm">
                <span className="w-6 font-black text-muted">{s.km}</span>
                <div className="h-4 flex-1 overflow-hidden rounded bg-surface-raised">
                  <div
                    className="h-full rounded bg-brand"
                    style={{ width: `${Math.max(12, (1 - (s.paceSecPerKm - maxSplitPace * 0.6) / (maxSplitPace * 0.6)) * 100)}%` }}
                  />
                </div>
                <span className="w-20 text-right font-bold">
                  {formatPace(s.paceSecPerKm, units)}
                  {!s.full ? <span className="text-muted"> *</span> : null}
                </span>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {a.efforts.length > 0 ? (
        <div className="card">
          <p className="label">Segment efforts</p>
          <div className="flex flex-col gap-2">
            {a.efforts.map((e) => (
              <Link
                key={e.segmentId}
                to={`/train/segments/${e.segmentId}`}
                className="flex items-center justify-between gap-2 rounded-lg bg-surface-raised px-3 py-2"
              >
                <div>
                  <p className="text-sm font-black">
                    {e.segmentName}
                    {e.isPersonalBest ? (
                      <span className="ml-2 inline-flex items-center gap-1 text-xs font-black uppercase text-brand">
                        <Award size={12} /> PR
                      </span>
                    ) : null}
                  </p>
                  <p className="text-xs text-muted">{formatDistanceM(e.distanceM, units)}</p>
                </div>
                <div className="text-right">
                  <p className="font-black">{formatDuration(e.elapsedSec)}</p>
                  <p className="text-xs text-muted">
                    #{e.rank} of {e.totalEfforts}
                  </p>
                </div>
              </Link>
            ))}
          </div>
        </div>
      ) : null}

      <div className="card">
        <div className="mb-3 flex items-center gap-4">
          <button
            onClick={() => kudos.mutate(a.id)}
            className={`flex items-center gap-1.5 text-sm font-bold ${a.hasKudoed ? 'text-brand' : 'text-muted'}`}
          >
            <ThumbsUp size={18} fill={a.hasKudoed ? 'currentColor' : 'none'} />
            {a.kudosCount} kudos
          </button>
          <span className="flex items-center gap-1.5 text-sm font-bold text-muted">
            <MessageCircle size={18} /> {a.comments.length}
          </span>
        </div>
        <div className="flex flex-col gap-3">
          {a.comments.map((c) => (
            <div key={c.id} className="text-sm">
              <p>
                <span className="font-black">{c.memberName}</span>{' '}
                <span className="text-xs text-muted">{formatDayTime(c.createdAt)}</span>
              </p>
              <p className="text-muted">{c.text}</p>
            </div>
          ))}
          <div className="flex gap-2">
            <input
              className="input flex-1 !py-2"
              placeholder="Add a comment…"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void sendComment();
              }}
            />
            <button className="btn-brand !px-4 !py-2 text-sm" disabled={busy || !comment.trim()} onClick={() => void sendComment()}>
              Send
            </button>
          </div>
        </div>
      </div>

      {editOpen ? (
        <EditSheet
          activityId={a.id}
          initial={{ title: a.title, description: a.description, visibility: a.visibility }}
          onClose={() => setEditOpen(false)}
          onDone={() => {
            setEditOpen(false);
            invalidate();
          }}
        />
      ) : null}
    </div>
  );
}

function EditSheet({
  activityId,
  initial,
  onClose,
  onDone,
}: {
  activityId: string;
  initial: { title: string; description: string; visibility: ActivityVisibility };
  onClose: () => void;
  onDone: () => void;
}) {
  const [title, setTitle] = useState(initial.title);
  const [description, setDescription] = useState(initial.description);
  const [visibility, setVisibility] = useState<ActivityVisibility>(initial.visibility);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      await api.athlete.updateActivity(activityId, { title, description, visibility });
      onDone();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Save failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/50" onClick={onClose}>
      <div className="w-full max-w-md rounded-t-3xl bg-surface p-5" onClick={(e) => e.stopPropagation()}>
        <h2 className="display mb-4 text-xl">Edit activity</h2>
        <div className="flex flex-col gap-3">
          <div>
            <label className="label">Title</label>
            <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div>
            <label className="label">Description</label>
            <textarea className="input min-h-20" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div className="grid grid-cols-3 gap-2">
            {(['EVERYONE', 'FOLLOWERS', 'PRIVATE'] as const).map((v) => (
              <button
                key={v}
                onClick={() => setVisibility(v)}
                className={`rounded-xl border px-2 py-2 text-xs font-black uppercase ${
                  visibility === v ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                }`}
              >
                {v.toLowerCase()}
              </button>
            ))}
          </div>
          {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
          <button className="btn-brand" disabled={busy || title.length < 1} onClick={() => void save()}>
            Save changes
          </button>
        </div>
      </div>
    </div>
  );
}
