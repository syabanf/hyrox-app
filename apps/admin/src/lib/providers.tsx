'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useEffect, useState, type ReactNode } from 'react';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5_000, retry: 1, refetchOnWindowFocus: false },
  },
});

/** Matches next.config basePath — /admin unless overridden at build time. */
const BASE_PATH = process.env.NEXT_PUBLIC_BASE_PATH ?? '/admin';

/**
 * MSW only exists in the browser, so this gate blocks the whole app until the
 * worker is running — and is exactly why every admin page fetches client-side.
 */
export function Providers({ children }: { children: ReactNode }) {
  const [state, setState] = useState<{ status: 'starting' | 'ready' | 'error'; message?: string }>(
    { status: 'starting' },
  );

  useEffect(() => {
    let cancelled = false;
    void import('@hyrox/mock-api/browser')
      .then(async ({ startMockWorker }) => {
        try {
          // Normal case: the worker script the app itself serves under its
          // base path.
          return await startMockWorker({
            serviceWorkerUrl: `${BASE_PATH}/mockServiceWorker.js`,
          });
        } catch (first) {
          // Some embedded browsers refuse subpath-hosted SW scripts. Behind
          // the shared-domain proxy the member app serves an identical copy
          // at the root - register that with our scope instead.
          if (!BASE_PATH) throw first;
          return await startMockWorker({
            serviceWorkerUrl: '/mockServiceWorker.js',
            serviceWorkerScope: `${BASE_PATH}/`,
          });
        }
      })
      .then(() => {
        if (!cancelled) setState({ status: 'ready' });
      })
      .catch((e: unknown) => {
        if (!cancelled)
          setState({
            status: 'error',
            message: e instanceof Error ? e.message : 'Service worker registration failed.',
          });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (state.status === 'error') {
    return (
      <div className="flex min-h-dvh items-center justify-center p-6">
        <div className="max-w-md text-center">
          <p className="display text-2xl font-black">Mock backend failed to start</p>
          <p className="mt-2 text-sm text-muted">
            The demo runs entirely in your browser via a service worker, and this one could not
            register. Serve the app over HTTPS (or localhost) and make sure{' '}
            <code className="font-mono">/mockServiceWorker.js</code> is reachable.
          </p>
          <p className="mt-3 rounded-xl bg-danger/10 px-3 py-2 text-xs font-bold text-danger">
            {state.message}
          </p>
          <button className="a-btn mt-4" onClick={() => location.reload()}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (state.status === 'starting') {
    return (
      <div className="flex min-h-dvh items-center justify-center text-muted">
        <p className="text-sm font-bold uppercase tracking-widest">Starting mock backend…</p>
      </div>
    );
  }
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
