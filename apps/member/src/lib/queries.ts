import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from './api';

export const keys = {
  me: ['me'] as const,
  wallet: ['wallet'] as const,
  sessions: (branchId?: string) => ['sessions', branchId ?? 'all'] as const,
  session: (id: string) => ['session', id] as const,
  bookings: ['bookings'] as const,
  visits: ['visits'] as const,
  notifications: ['notifications'] as const,
  branches: ['branches'] as const,
  packages: ['packages'] as const,
};

export const useMe = () => useQuery({ queryKey: keys.me, queryFn: api.me.get });
export const useWallet = () => useQuery({ queryKey: keys.wallet, queryFn: api.me.wallet });
export const useBranches = () =>
  useQuery({ queryKey: keys.branches, queryFn: api.catalog.branches, staleTime: Infinity });
export const usePackages = () =>
  useQuery({ queryKey: keys.packages, queryFn: api.catalog.packages });
export const useSessions = (branchId?: string) =>
  useQuery({
    queryKey: keys.sessions(branchId),
    queryFn: () => api.catalog.sessions(branchId ? { branchId } : undefined),
  });
export const useSession = (id: string) =>
  useQuery({ queryKey: keys.session(id), queryFn: () => api.catalog.session(id) });
export const useMyBookings = () => useQuery({ queryKey: keys.bookings, queryFn: api.me.bookings });
export const useMyVisits = () => useQuery({ queryKey: keys.visits, queryFn: api.me.visits });
export const useNotifications = () =>
  useQuery({ queryKey: keys.notifications, queryFn: api.me.notifications });

/** Everything that changes bookings/credits touches several views at once. */
export function useInvalidateAll() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries();
}

export function useBookMutation() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (sessionId: string) => api.bookings.book(sessionId),
    onSuccess: invalidate,
  });
}

export function useCancelMutation() {
  const invalidate = useInvalidateAll();
  return useMutation({
    mutationFn: (bookingId: string) => api.bookings.cancel(bookingId),
    onSuccess: invalidate,
  });
}
