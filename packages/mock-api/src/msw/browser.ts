import { setupWorker } from 'msw/browser';
import { createMockApi, type MockApi } from '../index';

export interface StartedMockWorker {
  api: MockApi;
  worker: ReturnType<typeof setupWorker>;
}

export interface StartMockWorkerOptions {
  /**
   * Where the app serves mockServiceWorker.js. Defaults to the domain root —
   * apps deployed under a subpath (e.g. /admin) must pass
   * `${basePath}/mockServiceWorker.js` or registration 404s.
   */
  serviceWorkerUrl?: string;
  /** Registration scope; defaults to the script's directory. */
  serviceWorkerScope?: string;
}

/**
 * Boot the browser mock API. Apps must await this BEFORE rendering anything
 * that fetches, so no request escapes to the network.
 */
export async function startMockWorker(
  options: StartMockWorkerOptions = {},
): Promise<StartedMockWorker> {
  const api = createMockApi();
  const worker = setupWorker(...api.handlers);
  await worker.start({
    onUnhandledRequest: 'bypass',
    quiet: true,
    serviceWorker: {
      url: options.serviceWorkerUrl ?? '/mockServiceWorker.js',
      ...(options.serviceWorkerScope
        ? { options: { scope: options.serviceWorkerScope } }
        : {}),
    },
  });
  // Snapshot the db periodically + on unload so state survives reloads.
  setInterval(() => api.persist(), 1500);
  window.addEventListener('beforeunload', () => api.persist());
  return { api, worker };
}
