import { Crosshair, LoaderCircle, MapPinOff, ShieldAlert } from 'lucide-react';
import type { LocationPermission } from '../lib/geolocation';

/**
 * The ask, and what to do when the answer was no.
 *
 * A browser gives an app one chance at the location prompt. Spending it the
 * moment a screen mounts — before the person has any idea what the app wants
 * it for — is how an app ends up permanently blocked by somebody who would
 * happily have said yes to "so we can draw your run on the map". So the
 * prompt is always behind a button, and the button says why.
 */
export function LocationGate({
  permission,
  error,
  locating,
  onRequest,
  reason,
  compact = false,
}: {
  permission: LocationPermission;
  error: string | null;
  locating: boolean;
  onRequest: () => void;
  /** Why this screen wants it, in the user's terms. */
  reason: string;
  compact?: boolean;
}) {
  if (permission === 'granted' && !error) return null;

  const blocked = permission === 'denied' || permission === 'unsupported' || permission === 'insecure';
  const Icon = blocked ? (permission === 'denied' ? ShieldAlert : MapPinOff) : Crosshair;

  return (
    <div
      className={`flex items-start gap-3 rounded-xl border ${
        blocked ? 'border-warn/30 bg-warn/[0.07]' : 'border-line bg-surface'
      } ${compact ? 'p-3' : 'p-4'}`}
    >
      <Icon size={18} className={`mt-0.5 shrink-0 ${blocked ? 'text-warn' : 'text-brand'}`} />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-bold">{blocked ? headline(permission) : 'Use your location?'}</p>
        {/* Blocked, the useful sentence is how to unblock it — the reason we
            wanted it is water under the bridge by then. */}
        <p className="mt-0.5 text-xs text-muted">{error ?? (blocked ? remedy(permission) : reason)}</p>
        {!blocked ? (
          <button
            type="button"
            onClick={onRequest}
            disabled={locating}
            className="btn mt-3 !py-2 !text-xs"
          >
            {locating ? (
              <>
                <LoaderCircle size={14} className="animate-spin" /> Finding you…
              </>
            ) : (
              <>
                <Crosshair size={14} /> Allow location
              </>
            )}
          </button>
        ) : null}
      </div>
    </div>
  );
}

function remedy(permission: LocationPermission): string {
  switch (permission) {
    case 'denied':
      return 'Allow location for this site in your browser settings, then reload.';
    case 'insecure':
      return 'Browsers only share location over https. Open the app on its https address.';
    default:
      return 'Timer-based workouts still record; anything with a route needs location.';
  }
}

function headline(permission: LocationPermission): string {
  switch (permission) {
    case 'denied':
      return 'Location is blocked';
    case 'insecure':
      return 'Location needs https';
    default:
      return 'This device cannot share its location';
  }
}
