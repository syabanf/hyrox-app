import type { Member } from '@hyrox/domain';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AuthState {
  token: string | null;
  member: Member | null;
  setSession: (token: string, member: Member) => void;
  clear: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      member: null,
      setSession: (token, member) => set({ token, member }),
      clear: () => set({ token: null, member: null }),
    }),
    { name: 'hyrox.member.session' },
  ),
);
