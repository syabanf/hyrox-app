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
}

export function createMockApi(options: CreateMockApiOptions = {}): MockApi {
  const persistence = options.persistence ?? true;
  const now = new Date().toISOString();
  const snapshot = persistence ? loadSnapshot() : null;
  const db = snapshot ?? createSeedDb(now);

  const state: MockApiState = { db, deps: createDeps(db) };

  const persist = () => {
    if (persistence) saveSnapshot(state.db);
  };
  const reset = () => {
    clearSnapshot();
    state.db = createSeedDb(new Date().toISOString());
    state.deps = createDeps(state.db);
    persist();
  };

  const handlers = createHandlers(state, reset);
  if (!snapshot) persist();
  return { state, handlers, reset, persist };
}

export { SEED_VERSION, createEmptyDb, createSeedDb, createDeps };
export type { MockDb, MockApiState };
