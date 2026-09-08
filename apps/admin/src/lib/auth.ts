'use client';

import type { AdminUser, Permission } from '@nuhabit/domain';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AdminAuthState {
  token: string | null;
  user: AdminUser | null;
  permissions: Permission[];
  /**
   * The password was chosen by somebody else — a new starter, or a reset — so
   * the panel keeps the user on the change-password screen until it is gone.
   */
  mustChangePassword: boolean;
  setSession: (
    token: string,
    user: AdminUser,
    permissions: readonly Permission[],
    mustChangePassword?: boolean,
  ) => void;
  passwordChanged: () => void;
  clear: () => void;
}

export const useAdminAuth = create<AdminAuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      permissions: [],
      mustChangePassword: false,
      setSession: (token, user, permissions, mustChangePassword = false) =>
        set({ token, user, permissions: [...permissions], mustChangePassword }),
      passwordChanged: () => set({ mustChangePassword: false }),
      clear: () => set({ token: null, user: null, permissions: [], mustChangePassword: false }),
    }),
    { name: 'nuhabit.admin.session' },
  ),
);

export function usePermissions() {
  const permissions = useAdminAuth((s) => s.permissions);
  return {
    permissions,
    can: (permission: Permission) => permissions.includes(permission),
  };
}
