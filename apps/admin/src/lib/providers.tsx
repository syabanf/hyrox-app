'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useEffect, useState, type ReactNode } from 'react';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5_000, retry: 1, refetchOnWindowFocus: false },
  },
});

/**
 * The demo backend is plain in-process code wired into the API client (see
 * lib/api.ts) — nothing to boot. The one-tick mount gate keeps the app
 * client-only: it skips the hydration render, where persisted stores still
 * report their empty server snapshot and auth guards would misfire.
 */
export function Providers({ children }: { children: ReactNode }) {
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  if (!mounted) return null;
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
