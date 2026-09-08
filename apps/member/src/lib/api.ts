import { createApiClient } from '@nuhabit/api-client';
import { inProcessTransport } from '@nuhabit/mock-api/in-process';
import { useAuthStore } from './auth';

/**
 * Where the app gets its data.
 *
 * The API, by default. Unset, requests go to `/api` on the app's own origin,
 * which is what the nginx front door serves and what the dev server proxies;
 * set `VITE_API_BASE_URL` to a full origin when the API lives somewhere else.
 *
 * `VITE_OFFLINE_DEMO=1` takes the server out of the picture entirely and
 * answers every request in-process from the bundled seed. That is the demo,
 * not the product.
 */
const offlineDemo = (import.meta.env.VITE_OFFLINE_DEMO ?? '') === '1';

/** True when the app is talking to the real backend rather than the mock. */
export const usingRealBackend = !offlineDemo;

// Request paths already begin with /api, so a configured "/api" prefix would
// double it. Strip the suffix and any trailing slash, leaving an origin (or
// nothing at all, which means same-origin).
const baseUrl = (import.meta.env.VITE_API_BASE_URL ?? '')
  .trim()
  .replace(/\/api\/?$/, '')
  .replace(/\/$/, '');

export const api = createApiClient({
  getToken: () => useAuthStore.getState().token,
  ...(offlineDemo ? { transport: inProcessTransport } : { baseUrl }),
});

export { ApiError } from '@nuhabit/api-client';
