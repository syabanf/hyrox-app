'use client';

import type { AdminUser, Permission } from '@hyrox/domain';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AdminAuthState {
  token: string | null;
  user: AdminUser | null;
  permissions: Permission[];
  setSession: (token: string, user: AdminUser, permissions: readonly Permission[]) => void;
  clear: () => void;
}

export const useAdminAuth = create<AdminAuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      permissions: [],
      setSession: (token, user, permissions) => set({ token, user, permissions: [...permissions] }),
      clear: () => set({ token: null, user: null, permissions: [] }),
    }),
    { name: 'hyrox.admin.session' },
  ),
);

export function usePermissions() {
  const permissions = useAdminAuth((s) => s.permissions);
  return {
    permissions,
    can: (permission: Permission) => permissions.includes(permission),
  };
}
