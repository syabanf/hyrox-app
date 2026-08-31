import { getResponse } from 'msw';
import { createMockApi, type MockApi } from '../index';
import type { MockDb } from '../db';
import snapshotJson from '../../seed-snapshot.json';

const DAY_MS = 86_400_000;
const ISO_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/;

/**
 * The committed snapshot froze its dates the day it was dumped. Shift every
 * timestamp forward by whole days so "today" in the data is always today —
 * sessions keep their time of day, windows keep their durations.
 */
function reanchoredSnapshot(): MockDb {
  const source = snapshotJson as unknown as MockDb;
  const delta =
    Math.round((Date.now() - new Date(source.seededAt).getTime()) / DAY_MS) * DAY_MS;
  const walk = (value: unknown): unknown => {
    if (typeof value === 'string' && ISO_RE.test(value))
      return new Date(new Date(value).getTime() + delta).toISOString();
    if (Array.isArray(value)) return value.map(walk);
    if (value && typeof value === 'object')
      return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, walk(v)]));
    return value;
  };
  // walk() also deep-copies, so mutations never touch the imported module.
  return walk(source) as MockDb;
}

let backend: MockApi | null = null;

/** The in-process backend, created on first use from the bundled snapshot. */
export function ensureInProcessBackend(): MockApi {
  if (backend) return backend;
  backend = createMockApi({ freshDb: reanchoredSnapshot });
  if (typeof window !== 'undefined') {
    const api = backend;
    setInterval(() => api.persist(), 1500);
    window.addEventListener('beforeunload', () => api.persist());
    // Older builds registered an MSW service worker; unregister any leftovers.
    void navigator.serviceWorker
      ?.getRegistrations?.()
      .then((regs) => {
        for (const reg of regs) {
          if (reg.active?.scriptURL.includes('mockServiceWorker')) void reg.unregister();
        }
      })
      .catch(() => {});
  }
  return backend;
}

/**
 * Transport for `createApiClient`: answers requests straight from the mock
 * handlers as ordinary function calls — no service worker, no fetch patching,
 * nothing global. Requests no handler matches fall through to the network.
 */
export function inProcessTransport(request: Request): Promise<Response> {
  const { handlers } = ensureInProcessBackend();
  return getResponse(handlers, request).then((response) => response ?? fetch(request));
}
