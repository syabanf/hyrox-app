'use client';

import type { RaceEvent, RaceRegion, RaceStatus } from '@hyrox/domain';
import { RACE_REGIONS, RACE_STATUSES } from '@hyrox/domain';
import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle } from '../../../../components/ui';

type AdminRace = RaceEvent & { participants: number };

const toLocalInput = (iso: string | null): string => {
  if (!iso) return '';
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

export default function RaceEventsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<AdminRace | 'new' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { data: races, isLoading } = useQuery({ queryKey: ['admin-races'], queryFn: api.admin.races.list });

  const removeMutation = useMutation({
    mutationFn: (id: string) => api.admin.races.remove(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin-races'] }),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  if (isLoading) return <Spinner label="Loading race events…" />;
  const manage = can('campaigns.manage');

  return (
    <div>
      <PageTitle
        title="Race Events"
        subtitle="The HYROX calendar shown in the member app"
        actions={
          manage ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New race
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      <div className="mt-3 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {(races ?? []).map((r) => (
          <div key={r.id} className="a-card overflow-hidden !p-0">
            <div className="relative h-32 w-full">
              {r.imageUrl ? (
                <img src={r.imageUrl} alt="" className="h-full w-full object-cover" loading="lazy" />
              ) : (
                <div className="surface-ink h-full w-full" />
              )}
              <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/20 to-transparent" />
              <div className="absolute inset-x-0 bottom-0 flex items-end justify-between p-4">
                <p className="display text-xl leading-tight text-white">{r.name}</p>
                <span className="chip bg-white/15 text-white backdrop-blur">
                  {r.participants} joined
                </span>
              </div>
            </div>
            <div className="p-4">
              <div className="flex items-center justify-between gap-2">
                <p className="truncate text-sm font-bold">
                  {r.city}, {r.country} · {r.venue}
                </p>
                <StatusBadge status={r.status} />
              </div>
              <p className="mt-1 text-sm text-muted">{formatDayTime(r.startsAt)}</p>
              {manage ? (
                <div className="mt-3 flex gap-2">
                  <button className="a-btn-ghost flex-1 !py-1.5 text-xs" onClick={() => setEditing(r)}>
                    Edit
                  </button>
                  <button
                    className="a-btn-danger !py-1.5 text-xs"
                    disabled={removeMutation.isPending}
                    onClick={() => {
                      setError(null);
                      if (confirm(`Delete "${r.name}"? This cannot be undone.`))
                        removeMutation.mutate(r.id);
                    }}
                  >
                    Delete
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {editing ? (
        <RaceModal
          race={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['admin-races'] });
          }}
        />
      ) : null}
    </div>
  );
}

function RaceModal({
  race,
  onClose,
  onDone,
}: {
  race: AdminRace | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(race?.name ?? '');
  const [country, setCountry] = useState(race?.country ?? 'Indonesia');
  const [region, setRegion] = useState<RaceRegion>(race?.region ?? 'ASIA');
  const [city, setCity] = useState(race?.city ?? '');
  const [venue, setVenue] = useState(race?.venue ?? '');
  const [startsAt, setStartsAt] = useState(toLocalInput(race?.startsAt ?? null));
  const [registrationUrl, setRegistrationUrl] = useState(race?.registrationUrl ?? '');
  const [imageUrl, setImageUrl] = useState(race?.imageUrl ?? '');
  const [status, setStatus] = useState<RaceStatus>(race?.status ?? 'ANNOUNCED');
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const body = {
        name,
        country,
        region,
        city,
        venue,
        startsAt: new Date(startsAt).toISOString(),
        endsAt: null,
        registrationUrl,
        imageUrl: imageUrl.trim() === '' ? null : imageUrl.trim(),
        status,
      };
      return race ? api.admin.races.update(race.id, body) : api.admin.races.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={race ? `Edit ${race.name}` : 'New race event'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="HYROX Jakarta" />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">City</label>
            <input className="a-input" value={city} onChange={(e) => setCity(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Venue</label>
            <input className="a-input" value={venue} onChange={(e) => setVenue(e.target.value)} />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Country</label>
            <input className="a-input" value={country} onChange={(e) => setCountry(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Region</label>
            <select className="a-input" value={region} onChange={(e) => setRegion(e.target.value as RaceRegion)}>
              {RACE_REGIONS.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Race day</label>
            <input
              type="datetime-local"
              className="a-input"
              value={startsAt}
              onChange={(e) => setStartsAt(e.target.value)}
            />
          </div>
          <div>
            <label className="a-label">Status</label>
            <select className="a-input" value={status} onChange={(e) => setStatus(e.target.value as RaceStatus)}>
              {RACE_STATUSES.map((s) => (
                <option key={s}>{s}</option>
              ))}
            </select>
          </div>
        </div>
        <div>
          <label className="a-label">Registration URL</label>
          <input className="a-input" value={registrationUrl} onChange={(e) => setRegistrationUrl(e.target.value)} placeholder="https://…" />
        </div>
        <div>
          <label className="a-label">Image URL (optional)</label>
          <input className="a-input" value={imageUrl} onChange={(e) => setImageUrl(e.target.value)} placeholder="https://images.unsplash.com/…" />
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || name.length < 2 || city.length < 2 || venue.length < 2 || !startsAt}
          onClick={() => mutation.mutate()}
        >
          {race ? 'Save changes' : 'Create race'}
        </button>
      </div>
    </Modal>
  );
}
