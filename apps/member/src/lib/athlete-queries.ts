import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from './api';

export const athleteKeys = {
  feed: (scope: string) => ['athlete', 'feed', scope] as const,
  mine: ['athlete', 'mine'] as const,
  activity: (id: string) => ['athlete', 'activity', id] as const,
  stats: ['athlete', 'stats'] as const,
  segments: ['athlete', 'segments'] as const,
  segment: (id: string) => ['athlete', 'segment', id] as const,
  challenges: ['athlete', 'challenges'] as const,
  clubs: ['athlete', 'clubs'] as const,
  social: ['athlete', 'social'] as const,
  settings: ['athlete', 'settings'] as const,
};

export const useFeed = (scope: 'everyone' | 'following') =>
  useQuery({ queryKey: athleteKeys.feed(scope), queryFn: () => api.athlete.feed(scope) });
export const useMyActivities = () =>
  useQuery({ queryKey: athleteKeys.mine, queryFn: api.athlete.myActivities });
export const useActivity = (id: string) =>
  useQuery({ queryKey: athleteKeys.activity(id), queryFn: () => api.athlete.activity(id) });
export const useAthleteStats = () =>
  useQuery({ queryKey: athleteKeys.stats, queryFn: api.athlete.stats });
export const useSegments = () =>
  useQuery({ queryKey: athleteKeys.segments, queryFn: api.athlete.segments });
export const useSegment = (id: string) =>
  useQuery({ queryKey: athleteKeys.segment(id), queryFn: () => api.athlete.segment(id) });
export const useChallenges = () =>
  useQuery({ queryKey: athleteKeys.challenges, queryFn: api.athlete.challenges });
export const useClubs = () => useQuery({ queryKey: athleteKeys.clubs, queryFn: api.athlete.clubs });
export const useSocial = () =>
  useQuery({ queryKey: athleteKeys.social, queryFn: api.athlete.social });
export const useAthleteSettings = () =>
  useQuery({ queryKey: athleteKeys.settings, queryFn: api.athlete.settings });
export const useRoutes = () =>
  useQuery({ queryKey: ['athlete', 'routes'], queryFn: api.athlete.routes });
export const useHeatmap = () =>
  useQuery({ queryKey: ['athlete', 'heatmap'], queryFn: api.athlete.heatmap });
export const useExerciseLibrary = () =>
  useQuery({ queryKey: ['workout', 'exercises'], queryFn: api.workout.exercises, staleTime: Infinity });
export const useWorkout = (id: string) =>
  useQuery({ queryKey: ['workout', id], queryFn: () => api.workout.get(id) });
export const useWorkoutSessions = () =>
  useQuery({ queryKey: ['workout', 'sessions'], queryFn: api.workout.sessions });
export const useRaces = (query?: { region?: string; scope?: 'upcoming' | 'results' }) =>
  useQuery({ queryKey: ['races', query?.region ?? '', query?.scope ?? 'upcoming'], queryFn: () => api.races.list(query) });
export const useMyRaces = () => useQuery({ queryKey: ['races', 'mine'], queryFn: api.races.mine });

/** Current unit preference with a safe default while loading. */
export function useUnits(): 'METRIC' | 'IMPERIAL' {
  const { data } = useAthleteSettings();
  return data?.units ?? 'METRIC';
}

export function useKudosMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (activityId: string) => api.athlete.toggleKudos(activityId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['athlete'] }),
  });
}

export function useFollowMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (memberId: string) => api.athlete.toggleFollow(memberId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['athlete'] }),
  });
}
