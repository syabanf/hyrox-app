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

let installed: MockApi | null = null;

/**
 * Boot the browser mock synchronously: patch window.fetch to answer /api/*
 * from the bundled seed snapshot in-process. No service worker, no async
 * registration, nothing to hang or fail — localStorage still persists demo
 * state between reloads.
 */
export function installFetchMock(): MockApi {
  if (installed) return installed;
  const api = createMockApi({ freshDb: reanchoredSnapshot });

  const originalFetch = window.fetch.bind(window);
  window.fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const href =
      typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    const url = new URL(href, location.href);
    if (url.origin === location.origin && url.pathname.startsWith('/api/')) {
      const request = new Request(input, init);
      return getResponse(api.handlers, request).then(
        (response) => response ?? originalFetch(request),
      );
    }
    return originalFetch(input, init);
  };

  // Older builds registered an MSW service worker; unregister any leftovers.
  void navigator.serviceWorker
    ?.getRegistrations?.()
    .then((regs) => {
      for (const reg of regs) {
        if (reg.active?.scriptURL.includes('mockServiceWorker')) void reg.unregister();
      }
    })
    .catch(() => {});

  setInterval(() => api.persist(), 1500);
  window.addEventListener('beforeunload', () => api.persist());
  installed = api;
  return api;
}
