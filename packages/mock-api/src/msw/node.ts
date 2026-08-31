import { setupServer } from 'msw/node';
import { createMockApi, type MockApi } from '../index';

export interface StartedMockServer {
  api: MockApi;
  server: ReturnType<typeof setupServer>;
}

/** Node-side mock API for integration tests. */
export function createMockServer(): StartedMockServer {
  const api = createMockApi({ persistence: false });
  const server = setupServer(...api.handlers);
  return { api, server };
}
