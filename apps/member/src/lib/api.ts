import { createApiClient } from '@hyrox/api-client';
import { useAuthStore } from './auth';

export const api = createApiClient({
  getToken: () => useAuthStore.getState().token,
});

export { ApiError } from '@hyrox/api-client';
