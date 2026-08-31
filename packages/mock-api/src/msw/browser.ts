import { setupWorker } from 'msw/browser';
import { createMockApi, type MockApi } from '../index';

export interface StartedMockWorker {
  api: MockApi;
  worker: ReturnType<typeof setupWorker>;
}

/**
 * Boot the browser mock API. Apps must await this BEFORE rendering anything
 * that fetches, so no request escapes to the network.
 */
export async function startMockWorker(): Promise<StartedMockWorker> {
  const api = createMockApi();
  const worker = setupWorker(...api.handlers);
  await worker.start({
    onUnhandledRequest: 'bypass',
    quiet: true,
    serviceWorker: { url: '/mockServiceWorker.js' },
  });
  // Snapshot the db periodically + on unload so state survives reloads.
  setInterval(() => api.persist(), 1500);
  window.addEventListener('beforeunload', () => api.persist());
  return { api, worker };
}
