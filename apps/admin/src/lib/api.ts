'use client';

import { createApiClient } from '@nuhabit/api-client';
import { inProcessTransport } from '@nuhabit/mock-api/in-process';
import { useAdminAuth } from './auth';

/**
 * Where the panel gets its data.
 *
 * The API, by default. Unset, requests go to `/api` on the panel's own origin,
 * which is what the nginx front door and the dev server's rewrite both serve;
 * set `NEXT_PUBLIC_API_BASE_URL` to a full origin when the API lives
 * somewhere else.
 *
 * `NEXT_PUBLIC_OFFLINE_DEMO=1` takes the server out of the picture entirely
 * and answers every request in-process from the bundled seed. That is the
 * demo, not the product: the four ERP modules have no in-process
 * implementation and say so, and passwords are not kept at all.
 */
const offlineDemo = (process.env.NEXT_PUBLIC_OFFLINE_DEMO ?? '') === '1';

/** True when the panel is talking to the real backend rather than the mock. */
export const usingRealBackend = !offlineDemo;

// Request paths already begin with /api, so a configured "/api" prefix would
// double it. Strip the suffix and any trailing slash, leaving an origin (or
// nothing at all, which means same-origin).
const baseUrl = (process.env.NEXT_PUBLIC_API_BASE_URL ?? '')
  .trim()
  .replace(/\/api\/?$/, '')
  .replace(/\/$/, '');

export const api = createApiClient({
  getToken: () => useAdminAuth.getState().token,
  ...(offlineDemo ? { transport: inProcessTransport } : { baseUrl }),
});

export { ApiError } from '@nuhabit/api-client';
