import type { MockDb } from './db';
import { SEED_VERSION } from './db';

const storageKey = () => `hyrox.mockdb.v${SEED_VERSION}`;

const hasStorage = (): boolean => {
  try {
    return typeof localStorage !== 'undefined';
  } catch {
    return false;
  }
};

export function loadSnapshot(): MockDb | null {
  if (!hasStorage()) return null;
  try {
    const raw = localStorage.getItem(storageKey());
    if (!raw) return null;
    const parsed = JSON.parse(raw) as MockDb;
    return parsed.seedVersion === SEED_VERSION ? parsed : null;
  } catch {
    return null;
  }
}

export function saveSnapshot(db: MockDb): void {
  if (!hasStorage()) return;
  try {
    localStorage.setItem(storageKey(), JSON.stringify(db));
  } catch {
    // Storage full or unavailable — the demo keeps running in memory.
  }
}

export function clearSnapshot(): void {
  if (!hasStorage()) return;
  try {
    localStorage.removeItem(storageKey());
  } catch {
    /* ignore */
  }
}
