import { createApiClient } from '@hyrox/api-client';
import { inProcessTransport } from '@hyrox/mock-api/in-process';
import { useAuthStore } from './auth';

// Demo backend runs in-process; swap `transport` for a baseUrl when real.
export const api = createApiClient({
  getToken: () => useAuthStore.getState().token,
  transport: inProcessTransport,
});

export { ApiError } from '@hyrox/api-client';
