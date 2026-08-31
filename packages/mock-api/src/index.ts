import type { HttpHandler } from 'msw';
import { createEmptyDb, SEED_VERSION } from './db';
import type { MockDb } from './db';
import { createHandlers, type MockApiState } from './http/handlers';
import { clearSnapshot, loadSnapshot, saveSnapshot } from './persistence';
import { createDeps } from './repos';
import { createSeedDb } from './seed';

export interface MockApi {
  state: MockApiState;
  handlers: HttpHandler[];
  reset(): void;
  persist(): void;
}

export interface CreateMockApiOptions {
  /** Load/save localStorage snapshots (default true; tests turn it off). */
  persistence?: boolean;
  /**
   * Source of a fresh database (boot with no localStorage snapshot, and on
   * demo reset). Defaults to generating one with faker; the browser fetch
   * mock passes the bundled seed snapshot instead.
   */
  freshDb?: () => MockDb;
}

export function createMockApi(options: CreateMockApiOptions = {}): MockApi {
  const persistence = options.persistence ?? true;
  const makeDb = () => options.freshDb?.() ?? createSeedDb(new Date().toISOString());
  const snapshot = persistence ? loadSnapshot() : null;
  const db = snapshot ?? makeDb();

  const state: MockApiState = { db, deps: createDeps(db) };

  const persist = () => {
    if (persistence) saveSnapshot(state.db);
  };
  const reset = () => {
    clearSnapshot();
    state.db = makeDb();
    state.deps = createDeps(state.db);
    persist();
  };

  const handlers = createHandlers(state, reset);
  if (!snapshot) persist();
  return { state, handlers, reset, persist };
}

export { SEED_VERSION, createEmptyDb, createSeedDb, createDeps };
export type { MockDb, MockApiState };
