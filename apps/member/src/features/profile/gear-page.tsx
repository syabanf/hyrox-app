import { ApiError } from '@hyrox/api-client';
import { Spinner, formatDistanceM } from '@hyrox/ui';
import { ArrowLeft, Bike, Check, Footprints, Pencil, Plus, X } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useAthleteStats, useUnits } from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';

export function GearPage() {
  const navigate = useNavigate();
  const units = useUnits();
  const invalidate = useInvalidateAll();
  const { data: stats, isLoading } = useAthleteStats();
  const [addOpen, setAddOpen] = useState(false);
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [error, setError] = useState('');

  if (isLoading || !stats) return <Spinner label="Loading gear…" />;
  const gear = stats.gear;
  const active = gear.filter((g) => !g.retired);
  const retired = gear.filter((g) => g.retired);

  const setRetired = async (g: (typeof gear)[number], value: boolean) => {
    setError('');
    try {
      await api.athlete.updateGear(g.id, { name: g.name, kind: g.kind, retired: value });
      invalidate();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Update failed.');
    }
  };

  const rename = async (g: (typeof gear)[number]) => {
    setError('');
    try {
      await api.athlete.updateGear(g.id, { name: renameValue, kind: g.kind, retired: g.retired });
      setRenamingId(null);
      invalidate();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Rename failed.');
    }
  };

  const GearRow = ({ g }: { g: (typeof gear)[number] }) => (
    <div className={`card flex items-center gap-3 !py-4 ${g.retired ? 'opacity-60' : ''}`}>
      <span
        className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${
          g.kind === 'SHOES' ? 'bg-brand/10 text-brand' : 'bg-[#2563eb]/10 text-[#2563eb]'
        }`}
      >
        {g.kind === 'SHOES' ? <Footprints size={19} /> : <Bike size={19} />}
      </span>
      <div className="min-w-0 flex-1">
        {renamingId === g.id ? (
          <div className="flex items-center gap-2">
            <input
              autoFocus
              className="input !py-1.5 text-sm"
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && renameValue.trim().length >= 2) void rename(g);
                if (e.key === 'Escape') setRenamingId(null);
              }}
            />
            <button
              className="text-ok"
              aria-label="Save name"
              onClick={() => void rename(g)}
              disabled={renameValue.trim().length < 2}
            >
              <Check size={18} />
            </button>
            <button className="text-muted" aria-label="Cancel" onClick={() => setRenamingId(null)}>
              <X size={18} />
            </button>
          </div>
        ) : (
          <>
            <p className="flex items-center gap-1.5 truncate text-sm font-extrabold">
              {g.name}
              <button
                className="text-muted"
                aria-label="Rename"
                onClick={() => {
                  setRenamingId(g.id);
                  setRenameValue(g.name);
                }}
              >
                <Pencil size={13} />
              </button>
            </p>
            <p className="text-xs text-muted">
              {g.kind === 'SHOES' ? 'Shoes' : 'Bike'} · {formatDistanceM(g.distanceM, units)} logged
            </p>
          </>
        )}
      </div>
      <button
        className={`chip shrink-0 ${g.retired ? 'bg-ok/10 text-ok' : 'bg-surface-raised text-muted'}`}
        onClick={() => void setRetired(g, !g.retired)}
      >
        {g.retired ? 'Reactivate' : 'Retire'}
      </button>
    </div>
  );

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="display text-3xl">My gear</h1>
          <p className="mt-1 text-sm text-muted">
            Mileage adds up automatically from your activities.
          </p>
        </div>
        <button
          className="btn-brand mt-1 flex shrink-0 items-center gap-1.5 !px-4 !py-2.5 text-sm"
          onClick={() => setAddOpen(true)}
        >
          <Plus size={16} /> Add
        </button>
      </div>

      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}

      {gear.length === 0 ? (
        <div className="card text-sm text-muted">
          Nothing here yet — add your shoes or bike and every run and ride keeps its mileage up to
          date.
        </div>
      ) : (
        <>
          <div className="flex flex-col gap-2">
            {active.map((g) => (
              <GearRow key={g.id} g={g} />
            ))}
          </div>
          {retired.length > 0 ? (
            <div>
              <p className="mb-2 px-1 text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
                Retired
              </p>
              <div className="flex flex-col gap-2">
                {retired.map((g) => (
                  <GearRow key={g.id} g={g} />
                ))}
              </div>
            </div>
          ) : null}
        </>
      )}

      {addOpen ? (
        <AddGearSheet
          onClose={() => setAddOpen(false)}
          onDone={() => {
            setAddOpen(false);
            invalidate();
          }}
        />
      ) : null}
    </div>
  );
}

function AddGearSheet({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('');
  const [kind, setKind] = useState<'SHOES' | 'BIKE'>('SHOES');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      await api.athlete.createGear({ name, kind, retired: false });
      onDone();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Save failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-40 flex items-end justify-center bg-black/50" onClick={onClose}>
      <div className="w-full max-w-md rounded-t-3xl bg-surface p-5 pb-8" onClick={(e) => e.stopPropagation()}>
        <h2 className="display mb-4 text-xl">Add gear</h2>
        <div className="flex flex-col gap-3">
          <div>
            <label className="label">Name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Novablast 4" />
          </div>
          <div className="grid grid-cols-2 gap-2">
            {(['SHOES', 'BIKE'] as const).map((k) => (
              <button
                key={k}
                onClick={() => setKind(k)}
                className={`rounded-xl border px-3 py-2.5 text-sm font-black uppercase ${
                  kind === k ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                }`}
              >
                {k}
              </button>
            ))}
          </div>
          {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
          <button className="btn-brand" disabled={busy || name.length < 2} onClick={() => void save()}>
            Add gear
          </button>
        </div>
      </div>
    </div>
  );
}
