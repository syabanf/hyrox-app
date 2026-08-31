'use client';

import { installFetchMock } from '@hyrox/mock-api/fetch';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useEffect, useState, type ReactNode } from 'react';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5_000, retry: 1, refetchOnWindowFocus: false },
  },
});

/**
 * The mock backend is a synchronous in-process fetch patch (no service
 * worker), so the only gate needed is one client-side render pass to keep
 * hydration consistent — pages must still fetch client-side only.
 */
export function Providers({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    installFetchMock();
    setReady(true);
  }, []);

  if (!ready) return null;
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
