import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * Where the browser stands on letting us see the device's location.
 *
 * `prompt` is the important one: it means we may ask, and asking is a thing
 * you get to do properly once. A browser that has been refused will not ask
 * again on our behalf, so a screen that fires getCurrentPosition on mount and
 * hopes for the best spends the user's single chance on a moment when they
 * had no idea what the app wanted it for.
 */
export type LocationPermission = 'unsupported' | 'insecure' | 'prompt' | 'granted' | 'denied';

export interface Fix {
  lat: number;
  lng: number;
  /** Metres. The radius the browser is prepared to stand behind. */
  accuracyM: number;
  altitude?: number;
  heading?: number;
  speed?: number;
  at: number;
}

function initialState(): LocationPermission {
  if (typeof navigator === 'undefined' || !navigator.geolocation) return 'unsupported';
  // Geolocation is refused outright off a secure origin, and the error it
  // gives back is the same PERMISSION_DENIED a user refusal produces — which
  // sends you looking in browser settings for a switch that was never the
  // problem.
  if (typeof window !== 'undefined' && !window.isSecureContext) return 'insecure';
  return 'prompt';
}

/**
 * The permission, read without asking for it.
 *
 * The Permissions API tells us where we stand without showing anybody a
 * dialog, which is what lets a screen say "Location is off" instead of
 * springing a browser prompt on somebody who only wanted to look at a map.
 * Safari was late to it, so a missing API means "we may ask", not "no".
 */
export function useLocationPermission() {
  const [state, setState] = useState<LocationPermission>(initialState);

  useEffect(() => {
    if (state === 'unsupported' || state === 'insecure') return;
    if (!navigator.permissions?.query) return;

    let status: PermissionStatus | undefined;
    let live = true;
    const onChange = () => {
      if (status && live) setState(status.state as LocationPermission);
    };
    void navigator.permissions
      .query({ name: 'geolocation' as PermissionName })
      .then((result) => {
        if (!live) return;
        status = result;
        setState(result.state as LocationPermission);
        result.addEventListener('change', onChange);
      })
      // An older browser that cannot answer leaves us at 'prompt', which is
      // the honest answer: we do not know, and asking will find out.
      .catch(() => undefined);

    return () => {
      live = false;
      status?.removeEventListener('change', onChange);
    };
  }, [state]);

  return [state, setState] as const;
}

export interface UseMyLocation {
  fix: Fix | null;
  permission: LocationPermission;
  error: string | null;
  /** True between asking and the first answer. */
  locating: boolean;
  /** Ask. Call this from a click — never on mount. */
  request: () => Promise<Fix | null>;
  stop: () => void;
}

/**
 * The device's location, once or continuously.
 *
 * `watch` follows the device; without it this is a single fix, which is all a
 * "centre the map on me" button needs and a great deal cheaper on a phone
 * than leaving the GPS on.
 */
export function useMyLocation({ watch = false }: { watch?: boolean } = {}): UseMyLocation {
  const [permission, setPermission] = useLocationPermission();
  const [fix, setFix] = useState<Fix | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [locating, setLocating] = useState(false);
  const watchIdRef = useRef<number | null>(null);

  const stop = useCallback(() => {
    if (watchIdRef.current !== null) {
      navigator.geolocation.clearWatch(watchIdRef.current);
      watchIdRef.current = null;
    }
  }, []);

  useEffect(() => stop, [stop]);

  const request = useCallback(async () => {
    if (permission === 'unsupported') {
      setError('This device cannot share its location.');
      return null;
    }
    if (permission === 'insecure') {
      setError('Location needs a secure connection (https). Open the app over https and try again.');
      return null;
    }

    setLocating(true);
    setError(null);
    try {
      const first = await new Promise<Fix>((resolve, reject) => {
        navigator.geolocation.getCurrentPosition(
          (p) => resolve(toFix(p)),
          reject,
          { enableHighAccuracy: true, timeout: 15000, maximumAge: 30000 },
        );
      });
      setFix(first);
      setPermission('granted');

      if (watch && watchIdRef.current === null) {
        watchIdRef.current = navigator.geolocation.watchPosition(
          (p) => setFix(toFix(p)),
          (err) => setError(describe(err)),
          { enableHighAccuracy: true, maximumAge: 1000 },
        );
      }
      return first;
    } catch (err) {
      const positionError = err as GeolocationPositionError;
      setError(describe(positionError));
      if (positionError?.code === 1) setPermission('denied');
      return null;
    } finally {
      setLocating(false);
    }
  }, [permission, setPermission, watch]);

  return { fix, permission, error, locating, request, stop };
}

function toFix(p: GeolocationPosition): Fix {
  return {
    lat: p.coords.latitude,
    lng: p.coords.longitude,
    accuracyM: p.coords.accuracy,
    altitude: typeof p.coords.altitude === 'number' ? p.coords.altitude : undefined,
    heading: typeof p.coords.heading === 'number' ? p.coords.heading : undefined,
    speed: typeof p.coords.speed === 'number' ? p.coords.speed : undefined,
    at: p.timestamp,
  };
}

/**
 * What went wrong, in words that suggest what to do about it.
 *
 * The browser's own messages are written for developers: "User denied
 * Geolocation" tells somebody standing in a car park nothing they can act on.
 */
function describe(err: GeolocationPositionError): string {
  switch (err?.code) {
    case 1:
      return 'Location is blocked for this site. Turn it on in your browser settings for nuhabit, then try again.';
    case 2:
      return 'Your device could not get a fix. Step outside or check that location services are on.';
    case 3:
      return 'Finding you took too long. Try again somewhere with a clearer view of the sky.';
    default:
      return err?.message || 'Could not read your location.';
  }
}
