import type { Member } from '@nuhabit/domain';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

/** Demo account the app opens with when there is no session (no OTP at the start). */
export const DEMO_IDENTIFIER = 'demo@nuhabit.id';

interface AuthState {
  token: string | null;
  member: Member | null;
  /** True after an explicit sign-out: the app then shows the login screen instead of auto-signing in again. */
  signedOut: boolean;
  setSession: (token: string, member: Member) => void;
  clear: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      member: null,
      signedOut: false,
      setSession: (token, member) => set({ token, member, signedOut: false }),
      clear: () => set({ token: null, member: null, signedOut: true }),
    }),
    { name: 'nuhabit.member.session' },
  ),
);
