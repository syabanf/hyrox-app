import { Spinner } from '@hyrox/ui';
import { ArrowLeft } from 'lucide-react';
import { useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useAthleteSettings } from '../../lib/athlete-queries';
import { useInvalidateAll } from '../../lib/queries';

export function SettingsPage() {
  const navigate = useNavigate();
  const invalidate = useInvalidateAll();
  const { data: settings, isLoading } = useAthleteSettings();

  if (isLoading || !settings) return <Spinner label="Loading settings…" />;

  const update = async (patch: Parameters<typeof api.athlete.updateSettings>[0]) => {
    await api.athlete.updateSettings(patch);
    invalidate();
  };

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <h1 className="display text-3xl">Settings</h1>

      <div className="card flex flex-col gap-4 text-sm">
        <div>
          <p className="label">Language / Bahasa</p>
          <div className="grid grid-cols-2 gap-2">
            {(['EN', 'ID'] as const).map((lang) => (
              <button
                key={lang}
                onClick={() => void update({ language: lang })}
                className={`rounded-xl border px-3 py-2.5 text-sm font-black uppercase ${
                  settings.language === lang
                    ? 'border-brand bg-brand/10 text-brand'
                    : 'border-line bg-surface text-muted'
                }`}
              >
                {lang === 'EN' ? 'English' : 'Bahasa Indonesia'}
              </button>
            ))}
          </div>
        </div>

        <div>
          <p className="label">Units</p>
          <div className="grid grid-cols-2 gap-2">
            {(['METRIC', 'IMPERIAL'] as const).map((u) => (
              <button
                key={u}
                onClick={() => void update({ units: u })}
                className={`rounded-xl border px-3 py-2.5 text-sm font-black uppercase ${
                  settings.units === u ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                }`}
              >
                {u === 'METRIC' ? 'Kilometers' : 'Miles'}
              </button>
            ))}
          </div>
        </div>

        <label className="flex items-center justify-between font-bold">
          <span>
            Booking reminders
            <span className="block text-xs font-medium text-muted">
              Get notified before a booked class starts
            </span>
          </span>
          <input
            type="checkbox"
            checked={settings.bookingReminders}
            onChange={(e) => void update({ bookingReminders: e.target.checked })}
            className="h-5 w-5 accent-[var(--color-brand)]"
          />
        </label>
      </div>

      <div className="card text-sm text-muted">
        <p className="label">About</p>
        <p>
          HYROX Studio App — demo build. All data lives in this browser; use the dev tools (flask
          button) to reset it.
        </p>
      </div>
    </div>
  );
}
