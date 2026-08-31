import type {
  AccessLogView,
  AdminSessionView,
  ApiErrorBody,
  BookResultView,
  BookingView,
  CancelResultView,
  CreditsReportView,
  DashboardStatsView,
  MeView,
  MemberDetailView,
  MemberSessionView,
  MemberSummaryView,
  OtpChallengeView,
  PackageStatsView,
  PaymentView,
  QrView,
  RulesView,
  SalesReportView,
  ScanResultView,
  SessionDetailAdminView,
  SessionView,
  TopUpView,
  VisitsReportView,
  VoucherQuoteView,
  VoucherView,
  WalletView,
} from '@hyrox/contracts';
import type {
  AdjustCreditsInput,
  AdminBookInput,
  CreateSessionInput,
  GateScanInput,
  RegisterMemberInput,
  TopUpRequest,
  UpdateBranchInput,
  UpdateMemberAdminInput,
  UpdateProfileInput,
  UpdateRulesInput,
  UpdateSessionInput,
  UpsertCampaignInput,
  UpsertClassTypeInput,
  UpsertCoachInput,
  UpsertPackageInput,
  UpsertVoucherInput,
} from '@hyrox/contracts';
import type {
  ActivityCardView,
  ActivityCommentView,
  ActivityDetailView,
  AthleteStatsView,
  ChallengeView,
  ClassesReportView,
  ClubView,
  CreateBranchInput,
  GenerateWorkoutInput,
  HeatmapView,
  HomeView,
  MyRaceView,
  RaceEventView,
  RegisterRaceInput,
  ResolveConflictInput,
  RouteView,
  SaveActivityInput,
  SegmentDetailView,
  SegmentListView,
  SegmentPreviewInput,
  SegmentPreviewView,
  SocialView,
  UpdateActivityInput,
  UpdateAthleteSettingsInput,
  UpdateUserRaceInput,
  UpsertAdminUserInput,
  UpsertGateInput,
  UpsertGearInput,
  WorkoutHistoryItemView,
  WorkoutSessionView,
} from '@hyrox/contracts';
import type {
  AdminUser,
  AthleteSettings,
  AuditEvent,
  Booking,
  Branch,
  BusinessRules,
  Campaign,
  ClassType,
  Coach,
  CreditLedgerEntry,
  CreditPackage,
  Exercise,
  Gate,
  Gear,
  GeneratedWorkout,
  Member,
  MemberNotification,
  Payment,
  Route,
  SubstitutionRule,
  UserRace,
  VoucherStatus,
} from '@hyrox/domain';

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export interface ApiClientOptions {
  baseUrl?: string;
  getToken: () => string | null;
}

type Query = Record<string, string | number | null | undefined>;

const qs = (query?: Query): string => {
  if (!query) return '';
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== null && v !== undefined && v !== '') params.set(k, String(v));
  }
  const s = params.toString();
  return s ? `?${s}` : '';
};

export function createApiClient(options: ApiClientOptions) {
  const base = options.baseUrl ?? '';

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const token = options.getToken();
    const res = await fetch(`${base}${path}`, {
      method,
      headers: {
        ...(body !== undefined ? { 'content-type': 'application/json' } : {}),
        ...(token ? { authorization: `Bearer ${token}` } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) {
      const parsed = (await res.json().catch(() => null)) as ApiErrorBody | null;
      throw new ApiError(
        res.status,
        parsed?.error.code ?? 'UNKNOWN',
        parsed?.error.message ?? `Request failed (${res.status}).`,
      );
    }
    return (await res.json()) as T;
  }

  const get = <T>(path: string, query?: Query) => request<T>('GET', `${path}${qs(query)}`);
  const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {});
  const patch = <T>(path: string, body: unknown) => request<T>('PATCH', path, body);
  const put = <T>(path: string, body: unknown) => request<T>('PUT', path, body);

  return {
    auth: {
      requestOtp: (identifier: string) =>
        post<OtpChallengeView>('/api/auth/otp/request', { identifier }),
      verifyOtp: (challengeId: string, code: string) =>
        post<MemberSessionView>('/api/auth/otp/verify', { challengeId, code }),
      register: (input: RegisterMemberInput) =>
        post<MemberSessionView>('/api/auth/register', input),
      adminUsers: () => get<AdminSessionView['user'][]>('/api/admin/auth/users'),
      adminLogin: (userId: string) => post<AdminSessionView>('/api/admin/auth/login', { userId }),
    },
    me: {
      get: () => get<MeView>('/api/me'),
      home: () => get<HomeView>('/api/home'),
      update: (input: UpdateProfileInput) => patch<Member>('/api/me', input),
      wallet: () => get<WalletView>('/api/me/wallet'),
      topUp: (input: TopUpRequest) => post<TopUpView>('/api/me/topup', input),
      qr: () => post<QrView>('/api/me/qr'),
      bookings: () => get<BookingView[]>('/api/me/bookings'),
      visits: () => get<AccessLogView[]>('/api/me/visits'),
      notifications: () => get<MemberNotification[]>('/api/me/notifications'),
      readAllNotifications: () => post<{ ok: boolean }>('/api/me/notifications/read-all'),
    },
    catalog: {
      branches: () => get<Branch[]>('/api/branches'),
      packages: () => get<CreditPackage[]>('/api/packages'),
      sessions: (query?: { branchId?: string; from?: string; to?: string }) =>
        get<SessionView[]>('/api/sessions', query),
      session: (id: string) => get<SessionView>(`/api/sessions/${id}`),
      validateVoucher: (code: string, packageId: string) =>
        post<VoucherQuoteView>('/api/vouchers/validate', { code, packageId }),
    },
    bookings: {
      book: (sessionId: string) => post<BookResultView>(`/api/sessions/${sessionId}/book`),
      cancel: (bookingId: string) => post<CancelResultView>(`/api/bookings/${bookingId}/cancel`),
      confirmSpot: (bookingId: string) => post<Booking>(`/api/bookings/${bookingId}/confirm-spot`),
    },
    payments: {
      simulate: (paymentId: string) =>
        post<{ payment: Payment; entry: CreditLedgerEntry }>(`/api/payments/${paymentId}/simulate`),
    },
    gate: {
      scan: (gateId: string, input: GateScanInput) =>
        post<ScanResultView>(`/api/gates/${gateId}/scan`, input),
    },
    admin: {
      members: {
        list: (query?: { query?: string; status?: string }) =>
          get<MemberSummaryView[]>('/api/admin/members', query),
        get: (id: string) => get<MemberDetailView>(`/api/admin/members/${id}`),
        update: (id: string, input: UpdateMemberAdminInput) =>
          patch<MemberDetailView>(`/api/admin/members/${id}`, input),
        adjust: (id: string, input: AdjustCreditsInput) =>
          post<CreditLedgerEntry>(`/api/admin/members/${id}/adjust`, input),
      },
      ledger: {
        reverse: (entryId: string, reason: string) =>
          post<CreditLedgerEntry>(`/api/admin/ledger/${entryId}/reverse`, { reason }),
      },
      classTypes: {
        list: () => get<ClassType[]>('/api/admin/class-types'),
        create: (input: UpsertClassTypeInput) => post<ClassType>('/api/admin/class-types', input),
        update: (id: string, input: Partial<UpsertClassTypeInput>) =>
          patch<ClassType>(`/api/admin/class-types/${id}`, input),
      },
      sessions: {
        list: (query?: { branchId?: string; from?: string; to?: string }) =>
          get<SessionView[]>('/api/admin/sessions', query),
        get: (id: string) => get<SessionDetailAdminView>(`/api/admin/sessions/${id}`),
        create: (input: CreateSessionInput) => post<SessionView>('/api/admin/sessions', input),
        update: (id: string, input: UpdateSessionInput) =>
          patch<SessionView>(`/api/admin/sessions/${id}`, input),
        action: (id: string, action: 'publish' | 'cancel' | 'complete') =>
          post<SessionView>(`/api/admin/sessions/${id}/${action}`),
      },
      bookings: {
        book: (input: AdminBookInput) => post<BookResultView>('/api/admin/bookings', input),
        noShow: (bookingId: string) => post(`/api/admin/bookings/${bookingId}/no-show`),
        checkIn: (bookingId: string) => post(`/api/admin/bookings/${bookingId}/check-in`),
      },
      coaches: {
        list: () => get<Coach[]>('/api/admin/coaches'),
        create: (input: UpsertCoachInput) => post<Coach>('/api/admin/coaches', input),
        update: (id: string, input: Partial<UpsertCoachInput>) =>
          patch<Coach>(`/api/admin/coaches/${id}`, input),
      },
      gates: {
        list: () => get<Gate[]>('/api/admin/gates'),
        create: (input: UpsertGateInput) => post<Gate>('/api/admin/gates', input),
        update: (id: string, input: Partial<UpsertGateInput>) =>
          patch<Gate>(`/api/admin/gates/${id}`, input),
      },
      users: {
        create: (input: UpsertAdminUserInput) => post<AdminUser>('/api/admin/users', input),
        update: (id: string, input: Partial<UpsertAdminUserInput>) =>
          patch<AdminUser>(`/api/admin/users/${id}`, input),
      },
      segments: {
        preview: (input: SegmentPreviewInput) =>
          post<SegmentPreviewView>('/api/admin/segments/preview', input),
      },
      accessLogs: {
        list: (query?: {
          branchId?: string;
          gateId?: string;
          result?: string;
          mode?: string;
          limit?: number;
        }) => get<AccessLogView[]>('/api/admin/access-logs', query),
        resolve: (logId: string, input: ResolveConflictInput) =>
          post<AccessLogView>(`/api/admin/access-logs/${logId}/resolve`, input),
      },
      packages: {
        list: () => get<PackageStatsView[]>('/api/admin/packages'),
        create: (input: UpsertPackageInput) => post<CreditPackage>('/api/admin/packages', input),
        update: (id: string, input: Partial<UpsertPackageInput>) =>
          patch<CreditPackage>(`/api/admin/packages/${id}`, input),
      },
      payments: {
        list: () => get<PaymentView[]>('/api/admin/payments'),
        refund: (id: string, reason: string) =>
          post<{ payment: Payment }>(`/api/admin/payments/${id}/refund`, { reason }),
        simulate: (id: string) =>
          post<{ payment: Payment }>(`/api/payments/${id}/simulate`),
      },
      vouchers: {
        list: () => get<VoucherView[]>('/api/admin/vouchers'),
        create: (input: UpsertVoucherInput) => post<VoucherView>('/api/admin/vouchers', input),
        update: (id: string, input: Partial<UpsertVoucherInput>) =>
          patch<VoucherView>(`/api/admin/vouchers/${id}`, input),
        setStatus: (id: string, status: VoucherStatus) =>
          post<VoucherView>(`/api/admin/vouchers/${id}/status`, { status }),
      },
      campaigns: {
        list: () => get<Campaign[]>('/api/admin/campaigns'),
        create: (input: UpsertCampaignInput) => post<Campaign>('/api/admin/campaigns', input),
        send: (id: string) => post<Campaign>(`/api/admin/campaigns/${id}/send`),
      },
      reports: {
        dashboard: () => get<DashboardStatsView>('/api/admin/reports/dashboard'),
        sales: (days = 30) => get<SalesReportView>('/api/admin/reports/sales', { days }),
        visits: (days = 30) => get<VisitsReportView>('/api/admin/reports/visits', { days }),
        credits: () => get<CreditsReportView>('/api/admin/reports/credits'),
        classes: () => get<ClassesReportView>('/api/admin/reports/classes'),
      },
      audit: { list: (limit = 100) => get<AuditEvent[]>('/api/admin/audit', { limit }) },
      branches: {
        list: () => get<Branch[]>('/api/admin/branches'),
        create: (input: CreateBranchInput) => post<Branch>('/api/admin/branches', input),
        update: (id: string, input: UpdateBranchInput) =>
          patch<Branch>(`/api/admin/branches/${id}`, input),
      },
      rules: {
        get: () => get<RulesView>('/api/admin/rules'),
        update: (input: UpdateRulesInput) => put<BusinessRules>('/api/admin/rules', input),
      },
    },
    athlete: {
      feed: (scope: 'everyone' | 'following' = 'everyone') =>
        get<ActivityCardView[]>('/api/athlete/feed', { scope }),
      myActivities: () => get<ActivityCardView[]>('/api/athlete/activities'),
      save: (input: SaveActivityInput) => post<ActivityCardView>('/api/athlete/activities', input),
      activity: (id: string) => get<ActivityDetailView>(`/api/athlete/activities/${id}`),
      updateActivity: (id: string, input: UpdateActivityInput) =>
        patch<ActivityCardView>(`/api/athlete/activities/${id}`, input),
      deleteActivity: (id: string) => request<{ deleted: true }>('DELETE', `/api/athlete/activities/${id}`),
      routes: () => get<RouteView[]>('/api/athlete/routes'),
      route: (id: string) => get<Route>(`/api/athlete/routes/${id}`),
      saveRoute: (activityId: string, name: string) =>
        post<Route>('/api/athlete/routes', { activityId, name }),
      deleteRoute: (id: string) => request<{ ok: true }>('DELETE', `/api/athlete/routes/${id}`),
      heatmap: () => get<HeatmapView>('/api/athlete/heatmap'),
      toggleKudos: (id: string) =>
        post<{ kudoed: boolean; count: number }>(`/api/athlete/activities/${id}/kudos`),
      comment: (id: string, text: string) =>
        post<ActivityCommentView>(`/api/athlete/activities/${id}/comments`, { text }),
      stats: () => get<AthleteStatsView>('/api/athlete/stats'),
      segments: () => get<SegmentListView[]>('/api/athlete/segments'),
      segment: (id: string) => get<SegmentDetailView>(`/api/athlete/segments/${id}`),
      challenges: () => get<ChallengeView[]>('/api/athlete/challenges'),
      joinChallenge: (id: string) => post<{ joined: true }>(`/api/athlete/challenges/${id}/join`),
      clubs: () => get<ClubView[]>('/api/athlete/clubs'),
      toggleClub: (id: string) => post<{ joined: boolean }>(`/api/athlete/clubs/${id}/toggle`),
      social: () => get<SocialView>('/api/athlete/social'),
      toggleFollow: (memberId: string) =>
        post<{ following: boolean }>(`/api/athlete/follow/${memberId}`),
      createGear: (input: UpsertGearInput) => post<Gear>('/api/athlete/gear', input),
      updateGear: (id: string, input: UpsertGearInput) =>
        patch<Gear>(`/api/athlete/gear/${id}`, input),
      settings: () => get<AthleteSettings>('/api/me/settings'),
      updateSettings: (input: UpdateAthleteSettingsInput) =>
        put<AthleteSettings>('/api/me/settings', input),
    },
    workout: {
      exercises: () =>
        get<{ exercises: Exercise[]; substitutions: SubstitutionRule[] }>('/api/exercises'),
      generate: (input: GenerateWorkoutInput) =>
        post<GeneratedWorkout>('/api/workouts/generate', input),
      get: (id: string) => get<GeneratedWorkout>(`/api/workouts/${id}`),
      replaceBlock: (id: string, order: number, exerciseId: string) =>
        post<GeneratedWorkout>(`/api/workouts/${id}/replace`, { order, exerciseId }),
      start: (id: string) => post<WorkoutSessionView>(`/api/workouts/${id}/start`),
      sessions: () => get<WorkoutHistoryItemView[]>('/api/workout-sessions'),
      session: (id: string) => get<WorkoutSessionView>(`/api/workout-sessions/${id}`),
      completeBlock: (sessionId: string, order: number, durationSec: number) =>
        post<WorkoutSessionView>(`/api/workout-sessions/${sessionId}/block`, { order, durationSec }),
      pause: (sessionId: string) => post<WorkoutSessionView>(`/api/workout-sessions/${sessionId}/pause`),
      resume: (sessionId: string, pausedSec: number) =>
        post<WorkoutSessionView>(`/api/workout-sessions/${sessionId}/resume`, { pausedSec }),
      finish: (sessionId: string, partial: boolean) =>
        post<WorkoutSessionView & { activityId: string | null }>(
          `/api/workout-sessions/${sessionId}/finish`,
          { partial },
        ),
    },
    races: {
      list: (query?: { region?: string; scope?: 'upcoming' | 'results' }) =>
        get<RaceEventView[]>('/api/races', query),
      register: (raceEventId: string, input: RegisterRaceInput) =>
        post<UserRace>(`/api/races/${raceEventId}/register`, input),
      mine: () => get<MyRaceView[]>('/api/me/races'),
      update: (userRaceId: string, input: UpdateUserRaceInput) =>
        patch<UserRace>(`/api/me/races/${userRaceId}`, input),
    },
    dev: {
      reset: () => post<{ ok: boolean }>('/api/dev/reset'),
      expirySweep: () => post<{ affectedMembers: number; entries: number }>('/api/dev/expiry-sweep'),
    },
  };
}

export type ApiClient = ReturnType<typeof createApiClient>;
