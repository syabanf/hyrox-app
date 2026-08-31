'use client';

import { createApiClient } from '@hyrox/api-client';
import { useAdminAuth } from './auth';

export const api = createApiClient({
  getToken: () => useAdminAuth.getState().token,
});

export { ApiError } from '@hyrox/api-client';
