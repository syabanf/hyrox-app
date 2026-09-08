import { createApiClient } from '@nuhabit/api-client';
import { inProcessTransport } from '@nuhabit/mock-api/in-process';
import { useAuthStore } from './auth';

/**
 * Where the app gets its data.
 *
 * Unset, the app answers its own requests in-process from the bundled seed, so
 * the demo runs with no server behind it. Set, requests go to the Go backend:
 * `/api` for the proxied stack, or a full origin like `https://api.example.com`
 * when the API lives elsewhere.
 */
const configured = (import.meta.env.VITE_API_BASE_URL ?? '').trim();

/** True when the app is talking to the real backend rather than the mock. */
export const usingRealBackend = configured !== '';

// Request paths already begin with /api, so a configured "/api" prefix would
// double it. Strip the suffix and any trailing slash, leaving an origin (or
// nothing at all, which means same-origin).
const baseUrl = configured.replace(/\/api\/?$/, '').replace(/\/$/, '');

export const api = createApiClient({
  getToken: () => useAuthStore.getState().token,
  ...(usingRealBackend ? { baseUrl } : { transport: inProcessTransport }),
});

export { ApiError } from '@nuhabit/api-client';
