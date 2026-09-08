import { monthPeriod, periodMonthOf } from '@nuhabit/domain';
import { getResponse } from 'msw';
import { createMockApi, type MockApi } from '../index';
import type { MockDb } from '../db';
import snapshotJson from '../../seed-snapshot.json';

const DAY_MS = 86_400_000;
const ISO_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/;

/**
 * The committed snapshot froze its dates the day it was dumped. Shift every
 * timestamp forward by whole days so "today" in the data is always today -
 * sessions keep their time of day, windows keep their durations. The one
 * exception is the "live" demo class (`ses_live`), which is re-anchored to
 * start 20 minutes from load time so the demo member's booking is inside the
 * gate's check-in window whenever the app is opened. Coach payout periods are
 * snapped back to calendar months after the shift (a day shift would otherwise
 * leave them straddling two months).
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
  const db = walk(source) as MockDb;
  const live = db.sessions.find((s) => s.id === 'ses_live');
  if (live) {
    const shift = Date.now() + 20 * 60_000 - new Date(live.startsAt).getTime();
    const move = (iso: string) => new Date(new Date(iso).getTime() + shift).toISOString();
    live.startsAt = move(live.startsAt);
    live.endsAt = move(live.endsAt);
    live.bookingOpensAt = move(live.bookingOpensAt);
    live.bookingClosesAt = move(live.bookingClosesAt);
  }
  for (const payout of db.incentivePayouts ?? []) {
    const midpoint = (new Date(payout.periodStart).getTime() + new Date(payout.periodEnd).getTime()) / 2;
    const period = monthPeriod(periodMonthOf(new Date(midpoint).toISOString()));
    payout.periodStart = period.start;
    payout.periodEnd = period.end;
  }
  return db;
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
 * handlers as ordinary function calls - no service worker, no fetch patching,
 * nothing global. Requests no handler matches fall through to the network.
 */
export function inProcessTransport(request: Request): Promise<Response> {
  const { handlers } = ensureInProcessBackend();
  return getResponse(handlers, request).then((response) => response ?? fetch(request));
}
