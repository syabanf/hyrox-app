import { installFetchMock } from '@hyrox/mock-api/fetch';
import { QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router/dom';
import { router } from './app/router';
import { queryClient } from './lib/query-client';
import './styles.css';

// Plain in-process mock — must be live before anything fetches.
installFetchMock();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
