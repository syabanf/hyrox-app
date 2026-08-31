'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useEffect, useState, type ReactNode } from 'react';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5_000, retry: 1, refetchOnWindowFocus: false },
  },
});

/**
 * MSW only exists in the browser, so this gate blocks the whole app until the
 * worker is running — and is exactly why every admin page fetches client-side.
 */
export function Providers({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void import('@hyrox/mock-api/browser').then(async ({ startMockWorker }) => {
      await startMockWorker();
      if (!cancelled) setReady(true);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!ready) {
    return (
      <div className="flex min-h-dvh items-center justify-center text-muted">
        <p className="text-sm font-bold uppercase tracking-widest">Starting mock backend…</p>
      </div>
    );
  }
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
