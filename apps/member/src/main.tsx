import { QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router/dom';
import { router } from './app/router';
import { queryClient } from './lib/query-client';
import './styles.css';

async function boot() {
  // The mock API worker MUST be running before anything fetches.
  const { startMockWorker } = await import('@hyrox/mock-api/browser');
  await startMockWorker();

  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </StrictMode>,
  );
}

void boot();
