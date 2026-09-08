import type {
  AccessLogView,
  AdminSessionView,
  ApiErrorBody,
  AuthModeView,
  CoachFeeView,
  TrainerCardView,
  TrainerProfileView,
  BookResultView,
  BookingView,
  CancelResultView,
  CoachStatementView,
  CreditsReportView,
  DashboardStatsView,
  IncentivePayoutView,
  IncentiveSchemeView,
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
} from '@nuhabit/contracts';
import type {
  AdjustCreditsInput,
  AdminBookInput,
  CreateMemberAdminInput,
  CreatePayoutInput,
  CreateSessionInput,
  GateScanInput,
  PayoutActionInput,
  RegisterMemberInput,
  TopUpRequest,
  UpdateBranchInput,
  UpdateMemberAdminInput,
  UpdateProfileInput,
  UpdateRulesInput,
  UpdateSessionInput,
  UpdateExerciseInput,
  UpsertCampaignInput,
  UpsertChallengeInput,
  UpsertClassTypeInput,
  UpsertCoachInput,
  UpsertIncentiveSchemeInput,
  UpsertPackageInput,
  UpsertVoucherInput,
} from '@nuhabit/contracts';
import type {
  AdjustStockInput,
  AdjustXPInput,
  CashierShiftView,
  CreatePurchaseOrderInput,
  CreatePurchaseRequestInput,
  GoodsReceiptView,
  InventoryItemDetailView,
  InventoryItemView,
  BatchView,
  CampaignPreviewView,
  CampaignReportView,
  ClosingReportView,
  DeliveryLineInput,
  DeliveryView,
  ConversationView,
  ExpiryReportView,
  InboxOverviewView,
  InventoryOverviewView,
  ExternalEventView,
  GiftCardView,
  IssueGiftCardInput,
  ItemPackView,
  OpenDeliveryInput,
  LoyaltyMemberDetailView,
  MemberBadgeView,
  LoyaltyProfileView,
  LoyaltySummaryView,
  POSOrderView,
  POSOverviewView,
  POSProductView,
  PurchaseOrderLineInput,
  OpenConversationInput,
  PostMessageInput,
  ReviewsForView,
  ReviewView,
  PayablesView,
  PriceHistoryEntryView,
  PromotionView,
  ProfitLine,
  PurchaseSummaryView,
  RaiseCreditInput,
  RecordPaymentInput,
  SaveTermsInput,
  RevenueCompositionView,
  RushHour,
  ScanView,
  SetConsentInput,
  StockCardEntry,
  SupplierPerformance,
  TemplatePreviewView,
  PurchaseOrderView,
  PurchaseRequestLineInput,
  PurchaseRequestView,
  PurchaseReturnView,
  PurchasingSummaryView,
  ReceiveLineInput,
  RedemptionView,
  ReorderPointInput,
  StockRowView,
  StockTakeView,
  TenderInput,
  TransferStockInput,
  UpsertBadgeInput,
  UpsertMethodInput,
  UpsertPartnerInput,
  UpsertPromotionInput,
  UpsertInventoryItemInput,
  UpsertPackInput,
  UpsertPOSProductInput,
  UpsertRewardInput,
  UpsertSupplierInput,
  UpsertTemplateInput,
  ValuationReport,
  UpsertTierInput,
  XPAwardView,
} from '@nuhabit/contracts';
import type {
  AssignShiftInput,
  AttendanceView,
  ClockInput,
  EmployeeDetailView,
  EmployeeView,
  HrOverviewView,
  HrSelfView,
  LeaveView,
  MarkAttendanceInput,
  OvertimeView,
  RequestLeaveInput,
  RequestOvertimeInput,
  ScheduleRowView,
  SetLeaveAllowanceInput,
  StaffRosterEntryView,
  UpsertDepartmentInput,
  UpsertEmployeeInput,
  UpsertHolidayInput,
  UpsertPositionInput,
  UpsertShiftInput,
} from '@nuhabit/contracts';
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
  UpsertRaceEventInput,
  UpsertGearInput,
  WorkoutHistoryItemView,
  WorkoutSessionView,
} from '@nuhabit/contracts';
import type {
  AdminUser,
  AthleteSettings,
  CashierShift,
  GoodsReceipt,
  InventoryCategory,
  LoyaltyTier,
  POSCategory,
  POSOrder,
  POSProduct,
  PurchaseOrder,
  PurchaseRequest,
  PurchaseReturn,
  Reward,
  StockMovement,
  StockTake,
  StockTransfer,
  Supplier,
  SupplierPrice,
  XPEntry,
  XPRule,
  AuditEvent,
  Booking,
  Branch,
  BusinessRules,
  Campaign,
  Challenge,
  ClassType,
  Coach,
  CreditLedgerEntry,
  CreditPackage,
  Exercise,
  Gate,
  Badge,
  ContactPreference,
  Delivery,
  Gear,
  GeneratedWorkout,
  ItemPack,
  Member,
  ExternalEvent,
  GiftCard,
  ImportResult,
  IntegrationPartner,
  MemberBadge,
  MessageTemplate,
  PaymentMethod,
  PrintJob,
  MemberNotification,
  Department,
  EmploymentStatus,
  Holiday,
  LeaveBalance,
  Payment,
  Position,
  ProductPrice,
  ReceiptSettings,
  Review,
  RaceEvent,
  SalesChannel,
  Shift,
  Route,
  SubstitutionRule,
  Unit,
  UnitKind,
  VendorCredit,
  VendorPayment,
  UserRace,
  VoucherStatus,
} from '@nuhabit/domain';

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
  /**
   * How requests reach the backend - defaults to the network (global fetch).
   * The demo passes the in-process mock transport here; a real deployment
   * simply leaves it unset.
   */
  transport?: (request: Request) => Promise<Response>;
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
    const send = options.transport ?? ((req: Request) => fetch(req));
    const res = await send(
      new Request(`${base}${path}`, {
        method,
        headers: {
          ...(body !== undefined ? { 'content-type': 'application/json' } : {}),
          ...(token ? { authorization: `Bearer ${token}` } : {}),
        },
        body: body === undefined ? undefined : JSON.stringify(body),
      }),
    );
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
  const del = (path: string) => request<{ ok: boolean }>('DELETE', path);

  /**
   * Posts a spreadsheet as its own bytes.
   *
   * A CSV is not JSON and wrapping it in a string field would double its size
   * and its escaping. The server reads either shape; this is the honest one.
   */
  async function postText<T>(path: string, body: string): Promise<T> {
    const token = options.getToken();
    const send = options.transport ?? ((req: Request) => fetch(req));
    const res = await send(
      new Request(`${base}${path}`, {
        method: 'POST',
        headers: {
          'content-type': 'text/csv',
          ...(token ? { authorization: `Bearer ${token}` } : {}),
        },
        body,
      }),
    );
    if (!res.ok) {
      const parsed = (await res.json().catch(() => null)) as ApiErrorBody | null;
      throw new ApiError(
        res.status,
        parsed?.error.code ?? 'UNKNOWN',
        parsed?.error.message ?? `Upload failed (${res.status}).`,
      );
    }
    return (await res.json()) as T;
  }

  /**
   * Fetches an export as a file.
   *
   * Deliberately not a plain link with the token in the query string: a URL
   * ends up in server logs, browser history and any Referer that follows it.
   * Fetching with the header and handing back the bytes keeps the credential
   * where it belongs, and the caller turns it into a download.
   */
  async function download(path: string, query?: Query): Promise<Blob> {
    const token = options.getToken();
    const send = options.transport ?? ((req: Request) => fetch(req));
    const res = await send(
      new Request(`${base}${path}${qs(query)}`, {
        method: 'GET',
        headers: token ? { authorization: `Bearer ${token}` } : {},
      }),
    );
    if (!res.ok) {
      const parsed = (await res.json().catch(() => null)) as ApiErrorBody | null;
      throw new ApiError(
        res.status,
        parsed?.error.code ?? 'UNKNOWN',
        parsed?.error.message ?? `Export failed (${res.status}).`,
      );
    }
    return res.blob();
  }

  return {
    auth: {
      requestOtp: (identifier: string) =>
        post<OtpChallengeView>('/api/auth/otp/request', { identifier }),
      verifyOtp: (challengeId: string, code: string) =>
        post<MemberSessionView>('/api/auth/otp/verify', { challengeId, code }),
      register: (input: RegisterMemberInput) =>
        post<MemberSessionView>('/api/auth/register', input),
      adminUsers: () => get<AdminSessionView['user'][]>('/api/admin/auth/users'),
      /** What the login screen may offer: read before anybody has a token. */
      adminAuthMode: () => get<AuthModeView>('/api/admin/auth/mode'),
      adminLogin: (input: { userId?: string; email?: string; password?: string }) =>
        post<AdminSessionView>('/api/admin/auth/login', input),
      /** Replacing your own password. */
      changePassword: (currentPassword: string, newPassword: string) =>
        post<{ changed: boolean }>('/api/admin/auth/password', { currentPassword, newPassword }),
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
      announcement: (id: string) =>
        get<{
          id: string;
          title: string;
          message: string;
          deepLink: string | null;
          imageUrl: string | null;
          createdAt: string;
        }>(`/api/announcements/${id}`),
      promo: (code: string) =>
        get<{
          code: string;
          label: string;
          live: boolean;
          startsAt: string;
          endsAt: string;
          perMemberLimit: number | null;
          usageLimit: number | null;
          newMembersOnly: boolean;
          packageNames: string[] | null;
        }>(`/api/promos/${encodeURIComponent(code)}`),
      readAllNotifications: () => post<{ ok: boolean }>('/api/me/notifications/read-all'),
    },
    catalog: {
      branches: () => get<Branch[]>('/api/branches'),
      classTypes: () => get<ClassType[]>('/api/class-types'),
      packages: () => get<(CreditPackage & { coverageNames: string[] | null })[]>('/api/packages'),
      sessions: (query?: { branchId?: string; coachId?: string; from?: string; to?: string }) =>
        get<SessionView[]>('/api/sessions', query),
      session: (id: string) => get<SessionView>(`/api/sessions/${id}`),
      /** Coaches members can book, with what each has coming up. */
      trainers: (branchId?: string) =>
        get<TrainerCardView[]>('/api/coaches', branchId ? { branchId } : undefined),
      trainer: (id: string) => get<TrainerProfileView>(`/api/coaches/${id}`),
      validateVoucher: (code: string, packageId: string) =>
        post<VoucherQuoteView>('/api/vouchers/validate', { code, packageId }),
    },
    bookings: {
      book: (sessionId: string) => post<BookResultView>(`/api/sessions/${sessionId}/book`),
      cancel: (bookingId: string) => post<CancelResultView>(`/api/bookings/${bookingId}/cancel`),
      confirmSpot: (bookingId: string) => post<Booking>(`/api/bookings/${bookingId}/confirm-spot`),
    },
    payments: {
      get: (paymentId: string) =>
        get<{ payment: Payment; packageName: string }>(`/api/payments/${paymentId}`),
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
        create: (input: CreateMemberAdminInput) =>
          post<MemberDetailView>('/api/admin/members', input),
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
        remove: (id: string) => del(`/api/admin/class-types/${id}`),
      },
      sessions: {
        list: (query?: { branchId?: string; coachId?: string; from?: string; to?: string }) =>
          get<SessionView[]>('/api/admin/sessions', query),
        get: (id: string) => get<SessionDetailAdminView>(`/api/admin/sessions/${id}`),
        create: (input: CreateSessionInput) => post<SessionView>('/api/admin/sessions', input),
        update: (id: string, input: UpdateSessionInput) =>
          patch<SessionView>(`/api/admin/sessions/${id}`, input),
        action: (id: string, action: 'publish' | 'cancel' | 'complete') =>
          post<SessionView>(`/api/admin/sessions/${id}/${action}`),
        remove: (id: string) => del(`/api/admin/sessions/${id}`),
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
        remove: (id: string) => del(`/api/admin/coaches/${id}`),
      },
      gates: {
        list: () => get<Gate[]>('/api/admin/gates'),
        create: (input: UpsertGateInput) => post<Gate>('/api/admin/gates', input),
        update: (id: string, input: Partial<UpsertGateInput>) =>
          patch<Gate>(`/api/admin/gates/${id}`, input),
        remove: (id: string) => del(`/api/admin/gates/${id}`),
      },
      users: {
        create: (input: UpsertAdminUserInput) => post<AdminUser>('/api/admin/users', input),
        update: (id: string, input: Partial<UpsertAdminUserInput>) =>
          patch<AdminUser>(`/api/admin/users/${id}`, input),
        remove: (id: string) => del(`/api/admin/users/${id}`),
        /** Hand somebody a password; they must replace it at first use. */
        setPassword: (id: string, password: string) =>
          put<{ set: boolean }>(`/api/admin/users/${id}/password`, { password }),
        /** A PIN for authorising a void at a till. Empty takes it away. */
        setSupervisorPin: (id: string, pin: string) =>
          put<{ set: boolean }>(`/api/admin/users/${id}/supervisor-pin`, { pin }),
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
        remove: (id: string) => del(`/api/admin/packages/${id}`),
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
        remove: (id: string) => del(`/api/admin/vouchers/${id}`),
      },
      campaigns: {
        list: () => get<Campaign[]>('/api/admin/campaigns'),
        create: (input: UpsertCampaignInput) => post<Campaign>('/api/admin/campaigns', input),
        update: (id: string, input: Partial<UpsertCampaignInput>) =>
          patch<Campaign>(`/api/admin/campaigns/${id}`, input),
        send: (id: string) => post<Campaign>(`/api/admin/campaigns/${id}/send`),
        remove: (id: string) => del(`/api/admin/campaigns/${id}`),
      },
      challenges: {
        list: () =>
          get<{ challenge: Challenge; participantCount: number }[]>('/api/admin/challenges'),
        create: (input: UpsertChallengeInput) =>
          post<{ challenge: Challenge; participantCount: number }>('/api/admin/challenges', input),
        update: (id: string, input: Partial<UpsertChallengeInput>) =>
          patch<{ challenge: Challenge; participantCount: number }>(`/api/admin/challenges/${id}`, input),
        remove: (id: string) => del(`/api/admin/challenges/${id}`),
      },
      exercises: {
        list: () => get<Exercise[]>('/api/admin/exercises'),
        update: (id: string, input: UpdateExerciseInput) =>
          patch<Exercise>(`/api/admin/exercises/${id}`, input),
      },
      races: {
        list: () => get<(RaceEvent & { participants: number })[]>('/api/admin/races'),
        create: (input: UpsertRaceEventInput) =>
          post<RaceEvent & { participants: number }>('/api/admin/races', input),
        update: (id: string, input: Partial<UpsertRaceEventInput>) =>
          patch<RaceEvent & { participants: number }>(`/api/admin/races/${id}`, input),
        remove: (id: string) => del(`/api/admin/races/${id}`),
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
        remove: (id: string) => del(`/api/admin/branches/${id}`),
      },
      rules: {
        get: () => get<RulesView>('/api/admin/rules'),
        update: (input: UpdateRulesInput) => put<BusinessRules>('/api/admin/rules', input),
      },
      incentives: {
        schemes: {
          list: () => get<IncentiveSchemeView[]>('/api/admin/incentives/schemes'),
          create: (input: UpsertIncentiveSchemeInput) =>
            post<IncentiveSchemeView>('/api/admin/incentives/schemes', input),
          update: (id: string, input: UpsertIncentiveSchemeInput) =>
            put<IncentiveSchemeView>(`/api/admin/incentives/schemes/${id}`, input),
        },
        /** What each coach is actually paid, with the scheme resolved. */
        coachFees: () => get<CoachFeeView[]>('/api/admin/incentives/coach-fees'),
        statements: (query: { period: string; branchId?: string }) =>
          get<CoachStatementView[]>('/api/admin/incentives/statements', query),
        payouts: {
          list: (query?: { period?: string; coachId?: string; status?: string }) =>
            get<IncentivePayoutView[]>('/api/admin/incentives/payouts', query),
          create: (input: CreatePayoutInput) =>
            post<IncentivePayoutView>('/api/admin/incentives/payouts', input),
          action: (
            id: string,
            action: 'approve' | 'pay' | 'void',
            input: Partial<PayoutActionInput> = {},
          ) =>
            post<IncentivePayoutView>(`/api/admin/incentives/payouts/${id}/${action}`, input),
        },
      },
      /** Stock: what is on the shelves, and every movement that changed it. */
      inventory: {
        overview: (branchId?: string) =>
          get<InventoryOverviewView>('/api/admin/inventory/overview', { branchId }),
        categories: {
          list: () => get<InventoryCategory[]>('/api/admin/inventory/categories'),
          create: (input: { name: string; code: string; sortOrder?: number; active?: boolean }) =>
            post<InventoryCategory>('/api/admin/inventory/categories', input),
          update: (id: string, input: { name: string; code: string; sortOrder?: number; active?: boolean }) =>
            put<InventoryCategory>(`/api/admin/inventory/categories/${id}`, input),
          remove: (id: string) => del(`/api/admin/inventory/categories/${id}`),
        },
        items: {
          list: (query?: { query?: string; categoryId?: string; kind?: string; activeOnly?: string; limit?: number }) =>
            get<InventoryItemView[]>('/api/admin/inventory/items', query),
          get: (id: string) => get<InventoryItemDetailView>(`/api/admin/inventory/items/${id}`),
          create: (input: UpsertInventoryItemInput) =>
            post<InventoryItemView>('/api/admin/inventory/items', input),
          update: (id: string, input: UpsertInventoryItemInput) =>
            put<InventoryItemView>(`/api/admin/inventory/items/${id}`, input),
          setReorderPoint: (id: string, input: ReorderPointInput) =>
            put<StockRowView['level']>(`/api/admin/inventory/items/${id}/reorder`, input),
          /**
           * How this item may be handed over. The base pack is the unit stock
           * is counted in; the rest are the cartons it is bought by.
           */
          packs: (id: string, branchId?: string) =>
            get<ItemPackView[]>(`/api/admin/inventory/items/${id}/packs`, { branchId }),
          savePack: (id: string, input: UpsertPackInput) =>
            put<ItemPack>(`/api/admin/inventory/items/${id}/packs`, input),
          deletePack: (id: string, packId: string) =>
            del(`/api/admin/inventory/items/${id}/packs/${packId}`),
        },
        /**
         * Dated stock. The list comes back in FEFO order — soonest expiry
         * first, undated last — which is the order it has to leave in.
         */
        batches: (query?: { itemId?: string; branchId?: string; state?: string; limit?: number }) =>
          get<BatchView[]>('/api/admin/inventory/batches', query),
        expiry: (branchId?: string) =>
          get<ExpiryReportView>('/api/admin/inventory/expiry', { branchId }),
        /** Takes an expired batch off the shelf, as an adjustment with a reason. */
        writeOffBatch: (batchId: string) =>
          post<StockMovement>(`/api/admin/inventory/batches/${batchId}/write-off`, {}),
        /** The unit master: a word for an amount, shared across the catalogue. */
        units: {
          list: (activeOnly?: boolean) =>
            get<Unit[]>('/api/admin/inventory/units', { activeOnly: activeOnly ? 'true' : undefined }),
          save: (input: { code: string; name: string; kind?: UnitKind; sortOrder?: number; active?: boolean }) =>
            put<Unit>('/api/admin/inventory/units', input),
        },
        stock: (query?: { branchId?: string; categoryId?: string; query?: string; lowOnly?: string; limit?: number }) =>
          get<StockRowView[]>('/api/admin/inventory/stock', query),
        movements: (query?: { itemId?: string; branchId?: string; kind?: string; referenceType?: string; referenceId?: string; limit?: number }) =>
          get<StockMovement[]>('/api/admin/inventory/movements', query),
        /**
         * Spreadsheets. An import is a dry run until `apply` is set, so a
         * three-hundred-row mistake is visible before it happens.
         */
        importItems: (csv: string, apply = false) =>
          postText<ImportResult>(`/api/admin/inventory/items/import?apply=${apply}`, csv),
        exportItems: () => download('/api/admin/inventory/items/export'),
        exportStock: (branchId?: string) =>
          download('/api/admin/inventory/stock/export', { branchId }),
        reports: {
          /** At weighted-average cost: what the stock on hand actually cost. */
          valuation: (branchId?: string) =>
            get<ValuationReport>('/api/admin/inventory/reports/valuation', { branchId }),
          /** Every movement of one item, with the ledger's own running balance. */
          stockCard: (query: { itemId: string; branchId?: string; from?: string; to?: string }) =>
            get<StockCardEntry[]>('/api/admin/inventory/reports/stock-card', query),
        },
        /** A quantity never changes without a reason attached. */
        adjust: (input: AdjustStockInput) =>
          post<StockMovement>('/api/admin/inventory/adjust', input),
        transfer: (input: TransferStockInput) =>
          post<StockTransfer>('/api/admin/inventory/transfer', input),
        transfers: (query?: { itemId?: string; limit?: number }) =>
          get<StockTransfer[]>('/api/admin/inventory/transfers', query),
        stockTakes: {
          list: (query?: { branchId?: string; status?: string; limit?: number }) =>
            get<StockTake[]>('/api/admin/inventory/stock-takes', query),
          get: (id: string) => get<StockTakeView>(`/api/admin/inventory/stock-takes/${id}`),
          open: (input: { branchId: string; countedOn?: string; note?: string | null }) =>
            post<StockTakeView>('/api/admin/inventory/stock-takes', input),
          count: (id: string, input: { itemId: string; qtyCounted: number; note?: string | null }) =>
            post<StockTakeView>(`/api/admin/inventory/stock-takes/${id}/count`, input),
          removeLine: (id: string, lineId: string) =>
            del(`/api/admin/inventory/stock-takes/${id}/lines/${lineId}`),
          decide: (id: string, action: 'apply' | 'cancel') =>
            post<StockTakeView>(`/api/admin/inventory/stock-takes/${id}/${action}`, {}),
        },
      },

      /** Buying: request, order, receipt — three separate pairs of hands. */
      purchasing: {
        overview: (branchId?: string) =>
          get<PurchasingSummaryView>('/api/admin/purchasing/overview', { branchId }),
        /** What came off the truck, before anybody judged it. */
        deliveries: {
          list: (query?: { orderId?: string; supplierId?: string; branchId?: string; status?: string; limit?: number }) =>
            get<Delivery[]>('/api/admin/purchasing/deliveries', query),
          get: (id: string) => get<DeliveryView>(`/api/admin/purchasing/deliveries/${id}`),
          open: (input: OpenDeliveryInput) =>
            post<DeliveryView>('/api/admin/purchasing/deliveries', input),
          addLine: (id: string, input: DeliveryLineInput) =>
            post<DeliveryView>(`/api/admin/purchasing/deliveries/${id}/lines`, input),
          /** Only once every unit has been accepted or rejected. */
          close: (id: string) =>
            post<DeliveryView>(`/api/admin/purchasing/deliveries/${id}/close`, {}),
        },
        /** What an order owes, and what has actually been paid against it. */
        payables: {
          get: (orderId: string) =>
            get<PayablesView>(`/api/admin/purchasing/orders/${orderId}/payables`),
          /** Build the schedule from the supplier's own terms. */
          schedule: (orderId: string) =>
            post<PayablesView>(`/api/admin/purchasing/orders/${orderId}/payables/schedule`, {}),
          saveTerms: (orderId: string, input: SaveTermsInput) =>
            put<PayablesView>(`/api/admin/purchasing/orders/${orderId}/payables/terms`, input),
        },
        payments: {
          list: (query?: { supplierId?: string; orderId?: string; status?: string; limit?: number }) =>
            get<VendorPayment[]>('/api/admin/purchasing/payments', query),
          record: (input: RecordPaymentInput) =>
            post<VendorPayment>('/api/admin/purchasing/payments', input),
          post: (id: string) => post<VendorPayment>(`/api/admin/purchasing/payments/${id}/post`, {}),
          void: (id: string, reason: string) =>
            post<VendorPayment>(`/api/admin/purchasing/payments/${id}/void`, { reason }),
        },
        credits: {
          list: (query?: { supplierId?: string; status?: string; openOnly?: string; limit?: number }) =>
            get<VendorCredit[]>('/api/admin/purchasing/credits', query),
          raise: (input: RaiseCreditInput) =>
            post<VendorCredit>('/api/admin/purchasing/credits', input),
        },
        reports: {
          orders: (query?: ReportWindow & { supplierId?: string }) =>
            get<PurchaseSummaryView>('/api/admin/purchasing/reports/orders', query),
          /**
           * How suppliers actually behave, as opposed to what their price
           * lists say. Fill rate is weighted heaviest: goods that never
           * arrived cannot be sold at any price.
           */
          suppliers: (query?: ReportWindow & { supplierId?: string }) =>
            get<SupplierPerformance[]>('/api/admin/purchasing/reports/suppliers', query),
          /** Read from receipts: a quoted price is a promise, a received one a fact. */
          priceHistory: (itemId: string, limit?: number) =>
            get<PriceHistoryEntryView[]>('/api/admin/purchasing/reports/price-history', { itemId, limit }),
        },
        importSuppliers: (csv: string, apply = false) =>
          postText<ImportResult>(`/api/admin/purchasing/suppliers/import?apply=${apply}`, csv),
        importPrices: (supplierId: string, csv: string, apply = false) =>
          postText<ImportResult>(
            `/api/admin/purchasing/suppliers/${supplierId}/prices/import?apply=${apply}`, csv),
        exportSuppliers: () => download('/api/admin/purchasing/suppliers/export'),
        exportOrders: (query?: ReportWindow & { supplierId?: string }) =>
          download('/api/admin/purchasing/orders/export', query),
        suppliers: {
          list: (query?: { query?: string; status?: string; limit?: number }) =>
            get<Supplier[]>('/api/admin/purchasing/suppliers', query),
          get: (id: string) => get<Supplier>(`/api/admin/purchasing/suppliers/${id}`),
          create: (input: UpsertSupplierInput) =>
            post<Supplier>('/api/admin/purchasing/suppliers', input),
          update: (id: string, input: UpsertSupplierInput) =>
            put<Supplier>(`/api/admin/purchasing/suppliers/${id}`, input),
          remove: (id: string) => del(`/api/admin/purchasing/suppliers/${id}`),
          prices: (id: string, itemId?: string) =>
            get<SupplierPrice[]>(`/api/admin/purchasing/suppliers/${id}/prices`, { itemId }),
          setPrice: (id: string, input: { itemId: string; unitPriceIdr: number; minOrderQty?: number; leadTimeDays?: number; effectiveFrom?: string }) =>
            post<SupplierPrice>(`/api/admin/purchasing/suppliers/${id}/prices`, input),
        },
        requests: {
          list: (query?: { branchId?: string; status?: string; priority?: string; limit?: number }) =>
            get<PurchaseRequest[]>('/api/admin/purchasing/requests', query),
          get: (id: string) => get<PurchaseRequestView>(`/api/admin/purchasing/requests/${id}`),
          create: (input: CreatePurchaseRequestInput) =>
            post<PurchaseRequestView>('/api/admin/purchasing/requests', input),
          addLine: (id: string, input: PurchaseRequestLineInput) =>
            post<PurchaseRequestView>(`/api/admin/purchasing/requests/${id}/lines`, input),
          removeLine: (id: string, lineId: string) =>
            del(`/api/admin/purchasing/requests/${id}/lines/${lineId}`),
          submit: (id: string) =>
            post<PurchaseRequestView>(`/api/admin/purchasing/requests/${id}/submit`, {}),
          /** Signs whichever level the request is waiting on, if the role reaches it. */
          approve: (id: string) =>
            post<PurchaseRequestView>(`/api/admin/purchasing/requests/${id}/approve`, {}),
          reject: (id: string, reason: string) =>
            post<PurchaseRequestView>(`/api/admin/purchasing/requests/${id}/reject`, { reason }),
          convert: (id: string, supplierId: string) =>
            post<PurchaseOrderView>(`/api/admin/purchasing/requests/${id}/convert`, { supplierId }),
        },
        orders: {
          list: (query?: { branchId?: string; supplierId?: string; status?: string; limit?: number }) =>
            get<PurchaseOrder[]>('/api/admin/purchasing/orders', query),
          get: (id: string) => get<PurchaseOrderView>(`/api/admin/purchasing/orders/${id}`),
          create: (input: CreatePurchaseOrderInput) =>
            post<PurchaseOrderView>('/api/admin/purchasing/orders', input),
          setTerms: (id: string, input: { discountIdr: number; taxPercent: number; expectedOn?: string | null; terms?: string | null; shipTo?: string | null; note?: string | null }) =>
            put<PurchaseOrderView>(`/api/admin/purchasing/orders/${id}/terms`, input),
          addLine: (id: string, input: PurchaseOrderLineInput) =>
            post<PurchaseOrderView>(`/api/admin/purchasing/orders/${id}/lines`, input),
          removeLine: (id: string, lineId: string) =>
            del(`/api/admin/purchasing/orders/${id}/lines/${lineId}`),
          approve: (id: string) =>
            post<PurchaseOrderView>(`/api/admin/purchasing/orders/${id}/approve`, {}),
          send: (id: string) => post<PurchaseOrderView>(`/api/admin/purchasing/orders/${id}/send`, {}),
          cancel: (id: string, reason: string) =>
            post<PurchaseOrderView>(`/api/admin/purchasing/orders/${id}/cancel`, { reason }),
        },
        receipts: {
          list: (query?: { orderId?: string; branchId?: string; status?: string; limit?: number }) =>
            get<GoodsReceipt[]>('/api/admin/purchasing/receipts', query),
          get: (id: string) => get<GoodsReceiptView>(`/api/admin/purchasing/receipts/${id}`),
          open: (input: { orderId: string; deliveryNoteNumber?: string | null; note?: string | null }) =>
            post<GoodsReceiptView>('/api/admin/purchasing/receipts', input),
          addLine: (id: string, input: ReceiveLineInput) =>
            post<GoodsReceiptView>(`/api/admin/purchasing/receipts/${id}/lines`, input),
          removeLine: (id: string, lineId: string) =>
            del(`/api/admin/purchasing/receipts/${id}/lines/${lineId}`),
          /** Where the goods actually enter stock. */
          post: (id: string) =>
            post<GoodsReceiptView>(`/api/admin/purchasing/receipts/${id}/post`, {}),
          cancel: (id: string) =>
            post<GoodsReceiptView>(`/api/admin/purchasing/receipts/${id}/cancel`, {}),
        },
        returns: {
          list: (query?: { receiptId?: string; status?: string; limit?: number }) =>
            get<PurchaseReturn[]>('/api/admin/purchasing/returns', query),
          get: (id: string) => get<PurchaseReturnView>(`/api/admin/purchasing/returns/${id}`),
          open: (input: { receiptId: string; reasonType: string; reasonNote?: string | null }) =>
            post<PurchaseReturnView>('/api/admin/purchasing/returns', input),
          addLine: (id: string, input: { receiptItemId: string; qty: number; note?: string | null }) =>
            post<PurchaseReturnView>(`/api/admin/purchasing/returns/${id}/lines`, input),
          /**
           * Sending goods back is somebody's signature. Nothing leaves the
           * building on the receiving bay's say-so, and a rejected return goes
           * back to draft to be corrected rather than dying there.
           */
          submit: (id: string) =>
            post<PurchaseReturnView>(`/api/admin/purchasing/returns/${id}/submit`, {}),
          approve: (id: string, note?: string) =>
            post<PurchaseReturnView>(`/api/admin/purchasing/returns/${id}/approve`, { note: note ?? null }),
          reject: (id: string, note: string) =>
            post<PurchaseReturnView>(`/api/admin/purchasing/returns/${id}/reject`, { note }),
          revise: (id: string) =>
            post<PurchaseReturnView>(`/api/admin/purchasing/returns/${id}/revise`, {}),
          post: (id: string) => post<PurchaseReturnView>(`/api/admin/purchasing/returns/${id}/post`, {}),
        },
      },

      /** Loyalty: points, tiers and what they buy. */
      crm: {
        overview: () => get<LoyaltySummaryView>('/api/admin/crm/overview'),
        members: {
          list: (query?: { tierCode?: string; status?: string; limit?: number }) =>
            get<LoyaltyProfileView[]>('/api/admin/crm/members', query),
          get: (id: string) => get<LoyaltyMemberDetailView>(`/api/admin/crm/members/${id}`),
          /** Handing out points by hand always carries a reason. */
          adjust: (id: string, input: AdjustXPInput) =>
            post<XPAwardView>(`/api/admin/crm/members/${id}/adjust`, input),
          redeem: (id: string, rewardId: string) =>
            post<RedemptionView>(`/api/admin/crm/members/${id}/redeem`, { rewardId }),
          /** What this member has done, as opposed to what they have spent. */
          badges: (id: string) => get<MemberBadgeView[]>(`/api/admin/crm/members/${id}/badges`),
          awardBadge: (id: string, badgeId: string, note?: string) =>
            post<MemberBadge>(`/api/admin/crm/members/${id}/badges`, { badgeId, note: note ?? null }),
          revokeBadge: (id: string, badgeId: string) =>
            del(`/api/admin/crm/members/${id}/badges/${badgeId}`),
          /** Awards everything newly qualified for. Idempotent, so it is cheap to run. */
          evaluateBadges: (id: string) =>
            post<Badge[]>(`/api/admin/crm/members/${id}/badges/evaluate`, {}),
          /** Whether we may contact them, per channel. */
          consent: (id: string) =>
            get<ContactPreference[]>(`/api/admin/crm/members/${id}/consent`),
          setConsent: (id: string, input: SetConsentInput) =>
            put<ContactPreference>(`/api/admin/crm/members/${id}/consent`, input),
        },
        badges: {
          list: (activeOnly = false) =>
            get<Badge[]>('/api/admin/crm/badges', { activeOnly: activeOnly ? 'true' : '' }),
          save: (input: UpsertBadgeInput) => put<Badge>('/api/admin/crm/badges', input),
        },
        /**
         * The inbox. The one number that matters is the gap between a member
         * speaking and somebody answering, and it is computed rather than
         * maintained by hand.
         */
        inbox: () => get<InboxOverviewView>('/api/admin/crm/inbox'),
        conversations: {
          list: (query?: { status?: string; channel?: string; assignedTo?: string; memberId?: string; query?: string; limit?: number }) =>
            get<ConversationView[]>('/api/admin/crm/conversations', query),
          get: (id: string) => get<ConversationView>(`/api/admin/crm/conversations/${id}`),
          open: (input: OpenConversationInput) =>
            post<ConversationView>('/api/admin/crm/conversations', input),
          update: (id: string, input: { status?: string; priority?: string; assignedTo?: string | null; assignedName?: string | null; tags?: string[]; memberId?: string | null; setMember?: boolean }) =>
            put<ConversationView>(`/api/admin/crm/conversations/${id}`, input),
          reply: (id: string, input: PostMessageInput) =>
            post<ConversationView>(`/api/admin/crm/conversations/${id}/messages`, input),
        },
        templates: {
          list: (channel?: string) =>
            get<MessageTemplate[]>('/api/admin/crm/templates', { channel }),
          save: (input: UpsertTemplateInput) =>
            put<MessageTemplate>('/api/admin/crm/templates', input),
          /** Renders against real values and says which placeholders had none. */
          preview: (id: string, values: Record<string, string>) =>
            post<TemplatePreviewView>(`/api/admin/crm/templates/${id}/preview`, { values }),
        },
        reviews: {
          list: (query?: { subjectType?: string; subjectId?: string; memberId?: string; status?: string; unansweredOnly?: string; maxRating?: number; limit?: number }) =>
            get<ReviewView[]>('/api/admin/crm/reviews', query),
          summary: (subjectType: string, subjectId: string) =>
            get<ReviewsForView>('/api/admin/crm/reviews/summary', { subjectType, subjectId }),
          reply: (id: string, reply: string) =>
            post<Review>(`/api/admin/crm/reviews/${id}/reply`, { reply }),
          /** Hidden, never deleted: it leaves the average and stays in the list. */
          setStatus: (id: string, status: string) =>
            put<Review>(`/api/admin/crm/reviews/${id}/status`, { status }),
        },
        partners: {
          list: (activeOnly = false) =>
            get<IntegrationPartner[]>('/api/admin/crm/partners', { activeOnly: activeOnly ? 'true' : '' }),
          save: (input: UpsertPartnerInput) =>
            put<IntegrationPartner>('/api/admin/crm/partners', input),
        },
        /** What partners have told us, and who we matched it to. */
        events: {
          list: (query?: { partnerId?: string; memberId?: string; status?: string; eventType?: string; limit?: number }) =>
            get<ExternalEventView[]>('/api/admin/crm/events', query),
          /** Point an unmatched event at a member by hand. */
          rematch: (id: string, memberId: string) =>
            post<ExternalEvent>(`/api/admin/crm/events/${id}/rematch`, { memberId }),
        },
        campaigns: {
          /** How it will actually read to particular members, consent included. */
          preview: (input: { message: string; deepLink?: string; memberIds: string[]; channel?: string }) =>
            post<CampaignPreviewView[]>('/api/admin/crm/campaigns/preview', input),
          report: (id: string) =>
            get<CampaignReportView>(`/api/admin/crm/campaigns/${id}/report`),
          mark: (id: string, memberId: string, event: 'OPENED' | 'CLICKED') =>
            post<{ marked: boolean }>(`/api/admin/crm/campaigns/${id}/mark`, { memberId, event }),
        },
        ledger: (query?: { memberId?: string; direction?: string; limit?: number }) =>
          get<XPEntry[]>('/api/admin/crm/ledger', query),
        tiers: {
          list: () => get<LoyaltyTier[]>('/api/admin/crm/tiers'),
          save: (input: UpsertTierInput) => put<LoyaltyTier>('/api/admin/crm/tiers', input),
        },
        rules: {
          list: (query?: { channel?: string; activeOnly?: string }) =>
            get<XPRule[]>('/api/admin/crm/rules', query),
          save: (input: Record<string, unknown>) => put<XPRule>('/api/admin/crm/rules', input),
        },
        rewards: {
          list: (activeOnly = false) =>
            get<Reward[]>('/api/admin/crm/rewards', { activeOnly: activeOnly ? 'true' : '' }),
          save: (input: UpsertRewardInput) => put<Reward>('/api/admin/crm/rewards', input),
        },
        redemptions: {
          list: (query?: { memberId?: string; rewardId?: string; status?: string; limit?: number }) =>
            get<RedemptionView[]>('/api/admin/crm/redemptions', query),
          decide: (id: string, action: 'approve' | 'fulfil' | 'cancel', note?: string) =>
            post<RedemptionView>(`/api/admin/crm/redemptions/${id}/${action}`, { note: note ?? null }),
        },
      },

      /** The till. */
      pos: {
        overview: (branchId?: string) => get<POSOverviewView>('/api/admin/pos/overview', { branchId }),
        categories: {
          list: () => get<POSCategory[]>('/api/admin/pos/categories'),
          save: (input: { id?: string; name: string; sortOrder?: number; active?: boolean }) =>
            put<POSCategory>('/api/admin/pos/categories', input),
        },
        products: {
          list: (query?: { query?: string; categoryId?: string; sellableOnly?: string; branchId?: string; limit?: number }) =>
            get<POSProductView[]>('/api/admin/pos/products', query),
          save: (input: UpsertPOSProductInput) => put<POSProduct>('/api/admin/pos/products', input),
          /** Every price break defined for one product, in every channel. */
          prices: (id: string) => get<ProductPrice[]>(`/api/admin/pos/products/${id}/prices`),
          savePrice: (id: string, input: { channel: SalesChannel; minQty: number; priceIdr: number; active?: boolean }) =>
            put<ProductPrice>(`/api/admin/pos/products/${id}/prices`, input),
          deletePrice: (id: string, priceId: string) =>
            del(`/api/admin/pos/products/${id}/prices/${priceId}`),
        },
        /**
         * The scanner's endpoint. A barcode resolves to exactly one product or
         * to nothing at all, so a miss is a 404 rather than an empty list —
         * the cashier's next move depends on knowing which.
         */
        scan: (barcode: string, branchId?: string) =>
          get<ScanView>('/api/admin/pos/scan', { barcode, branchId }),
        promotions: {
          list: (activeOnly = false) =>
            get<PromotionView[]>('/api/admin/pos/promotions', { activeOnly: activeOnly ? 'true' : '' }),
          save: (input: UpsertPromotionInput) =>
            put<PromotionView>('/api/admin/pos/promotions', input),
        },
        giftCards: {
          list: (query?: { memberId?: string; status?: string; query?: string; limit?: number }) =>
            get<GiftCard[]>('/api/admin/pos/gift-cards', query),
          get: (code: string) => get<GiftCardView>(`/api/admin/pos/gift-cards/${code}`),
          issue: (input: IssueGiftCardInput) =>
            post<GiftCardView>('/api/admin/pos/gift-cards', input),
          topUp: (code: string, amountIdr: number) =>
            post<GiftCardView>(`/api/admin/pos/gift-cards/${code}/top-up`, { amountIdr }),
          setStatus: (code: string, status: string) =>
            put<GiftCardView>(`/api/admin/pos/gift-cards/${code}/status`, { status }),
        },
        paymentMethods: {
          list: (activeOnly = false) =>
            get<PaymentMethod[]>('/api/admin/pos/payment-methods', { activeOnly: activeOnly ? 'true' : '' }),
          save: (input: UpsertMethodInput) =>
            put<PaymentMethod>('/api/admin/pos/payment-methods', input),
        },
        receipts: {
          settings: (branchId?: string) =>
            get<ReceiptSettings>('/api/admin/pos/receipt-settings', { branchId }),
          saveSettings: (input: ReceiptSettings) =>
            put<ReceiptSettings>('/api/admin/pos/receipt-settings', input),
          /** The rendered paper, exactly as it will print. */
          preview: (orderId: string) =>
            get<{ body: string }>(`/api/admin/pos/orders/${orderId}/receipt`),
          print: (orderId: string) =>
            post<PrintJob>(`/api/admin/pos/orders/${orderId}/print`, {}),
          send: (orderId: string, channel: string, destination: string) =>
            post<{ id: string; status: string }>(`/api/admin/pos/orders/${orderId}/send`, { channel, destination }),
        },
        printJobs: {
          list: (query?: { branchId?: string; status?: string; limit?: number }) =>
            get<PrintJob[]>('/api/admin/pos/print-jobs', query),
          finish: (id: string, status: string, error?: string) =>
            put<PrintJob>(`/api/admin/pos/print-jobs/${id}`, { status, error: error ?? null }),
        },
        /**
         * What the counter did. Every one of these reads completed sales only
         * — a voided sale is money that came in and went out again.
         */
        reports: {
          transactions: (query?: ReportWindow) =>
            get<POSOrder[]>('/api/admin/pos/reports/transactions', query),
          productSales: (query?: ReportWindow) =>
            get<ProfitLine[]>('/api/admin/pos/reports/product-sales', query),
          revenue: (query?: ReportWindow) =>
            get<RevenueCompositionView>('/api/admin/pos/reports/revenue-composition', query),
          rushHour: (query?: ReportWindow) =>
            get<RushHour>('/api/admin/pos/reports/rush-hour', query),
          /** The one action that makes money disappear, looked at together. */
          voids: (query?: ReportWindow) =>
            get<POSOrder[]>('/api/admin/pos/reports/voids', query),
          closing: (shiftId: string) =>
            get<ClosingReportView>(`/api/admin/pos/reports/closing/${shiftId}`),
        },
        shifts: {
          list: (query?: { branchId?: string; cashierId?: string; status?: string; limit?: number }) =>
            get<CashierShift[]>('/api/admin/pos/shifts', query),
          get: (id: string) => get<CashierShiftView>(`/api/admin/pos/shifts/${id}`),
          open: (input: { branchId: string; openingCashIdr: number; note?: string | null }) =>
            post<CashierShiftView>('/api/admin/pos/shifts', input),
          close: (id: string, input: { countedCashIdr: number; note?: string | null }) =>
            post<CashierShiftView>(`/api/admin/pos/shifts/${id}/close`, input),
        },
        orders: {
          list: (query?: { branchId?: string; shiftId?: string; memberId?: string; status?: string; limit?: number }) =>
            get<POSOrder[]>('/api/admin/pos/orders', query),
          get: (id: string) => get<POSOrderView>(`/api/admin/pos/orders/${id}`),
          open: (input: { branchId: string; memberId?: string | null; channel?: SalesChannel; note?: string | null }) =>
            post<POSOrderView>('/api/admin/pos/orders', input),
          setDetails: (id: string, input: { memberId?: string | null; setMember?: boolean; discountIdr: number; discountReason?: string | null; note?: string | null }) =>
            put<POSOrderView>(`/api/admin/pos/orders/${id}`, input),
          addLine: (id: string, input: { productId: string; qty: number; discountIdr?: number; note?: string | null }) =>
            post<POSOrderView>(`/api/admin/pos/orders/${id}/lines`, input),
          removeLine: (id: string, lineId: string) =>
            del(`/api/admin/pos/orders/${id}/lines/${lineId}`),
          tender: (id: string, input: TenderInput) =>
            post<POSOrderView>(`/api/admin/pos/orders/${id}/tender`, input),
          /** Where the stock moves, the points land and the sale closes. */
          complete: (id: string) => post<POSOrderView>(`/api/admin/pos/orders/${id}/complete`, {}),
          cancel: (id: string) => post<POSOrderView>(`/api/admin/pos/orders/${id}/cancel`, {}),
          /** Unwinding a paid sale: puts the stock back, needs a reason. */
          /** A code somebody typed. Refused with a reason when it saves nothing. */
          applyCode: (id: string, code: string) =>
            post<POSOrderView>(`/api/admin/pos/orders/${id}/code`, { code }),
          /**
           * Unwinding a paid sale. A cashier without the grant may still do it
           * with a manager's PIN, and the manager is recorded on the sale.
           */
          void: (id: string, reason: string, supervisorPin?: string) =>
            post<POSOrderView>(`/api/admin/pos/orders/${id}/void`, {
              reason, supervisorPin: supervisorPin ?? '',
            }),
        },
      },

      /**
       * The people side. Everything lives under /api/admin/hris because an
       * employee record carries a home address, a bank account and next of
       * kin — `hris.view` is a narrower grant than `members.view`.
       */
      hris: {
        overview: () => get<HrOverviewView>('/api/admin/hris/overview'),
        /** Your own working day. Any staff login may read and punch this. */
        me: () => get<HrSelfView>('/api/admin/hris/me'),
        clockInSelf: (notes?: string) =>
          post<AttendanceView>('/api/admin/hris/me/clock-in', { notes: notes ?? null }),
        clockOutSelf: (notes?: string) =>
          post<AttendanceView>('/api/admin/hris/me/clock-out', { notes: notes ?? null }),

        departments: {
          list: () => get<Department[]>('/api/admin/hris/departments'),
          create: (input: UpsertDepartmentInput) =>
            post<Department>('/api/admin/hris/departments', input),
          update: (id: string, input: UpsertDepartmentInput) =>
            put<Department>(`/api/admin/hris/departments/${id}`, input),
          remove: (id: string) => del(`/api/admin/hris/departments/${id}`),
        },
        positions: {
          list: () => get<Position[]>('/api/admin/hris/positions'),
          create: (input: UpsertPositionInput) =>
            post<Position>('/api/admin/hris/positions', input),
          update: (id: string, input: UpsertPositionInput) =>
            put<Position>(`/api/admin/hris/positions/${id}`, input),
          remove: (id: string) => del(`/api/admin/hris/positions/${id}`),
        },
        employmentStatuses: () =>
          get<EmploymentStatus[]>('/api/admin/hris/employment-statuses'),

        employees: {
          list: (query?: {
            query?: string;
            departmentId?: string;
            branchId?: string;
            activeOnly?: string;
            limit?: number;
          }) => get<EmployeeView[]>('/api/admin/hris/employees', query),
          get: (id: string) => get<EmployeeDetailView>(`/api/admin/hris/employees/${id}`),
          create: (input: UpsertEmployeeInput) =>
            post<EmployeeView>('/api/admin/hris/employees', input),
          /** A whole record, not a patch: see UpsertEmployeeInput. */
          update: (id: string, input: UpsertEmployeeInput) =>
            put<EmployeeView>(`/api/admin/hris/employees/${id}`, input),
          schedule: (id: string) =>
            get<ScheduleRowView[]>(`/api/admin/hris/employees/${id}/schedule`),
          assignShift: (id: string, input: AssignShiftInput) =>
            post<ScheduleRowView>(`/api/admin/hris/employees/${id}/schedule`, input),
          removeScheduleRow: (id: string, rowId: string) =>
            del(`/api/admin/hris/employees/${id}/schedule/${rowId}`),
          balance: (id: string, year?: number) =>
            get<LeaveBalance>(`/api/admin/hris/employees/${id}/balance`, { year }),
          setAllowance: (id: string, input: SetLeaveAllowanceInput) =>
            put<LeaveBalance>(`/api/admin/hris/employees/${id}/balance`, input),
        },

        shifts: {
          list: (activeOnly = false) =>
            get<Shift[]>('/api/admin/hris/shifts', { activeOnly: activeOnly ? 'true' : '' }),
          create: (input: UpsertShiftInput) => post<Shift>('/api/admin/hris/shifts', input),
          update: (id: string, input: UpsertShiftInput) =>
            put<Shift>(`/api/admin/hris/shifts/${id}`, input),
        },

        roster: (query?: { date?: string; branchId?: string }) =>
          get<StaffRosterEntryView[]>('/api/admin/hris/roster', query),
        attendance: {
          list: (query?: {
            employeeId?: string;
            from?: string;
            to?: string;
            status?: string;
            limit?: number;
          }) => get<AttendanceView[]>('/api/admin/hris/attendance', query),
          clockIn: (input: ClockInput) =>
            post<AttendanceView>('/api/admin/hris/attendance/clock-in', input),
          clockOut: (input: ClockInput) =>
            post<AttendanceView>('/api/admin/hris/attendance/clock-out', input),
          /** The escape hatch for everything a clock cannot express. */
          mark: (input: MarkAttendanceInput) =>
            post<AttendanceView>('/api/admin/hris/attendance/mark', input),
        },

        leaves: {
          list: (query?: {
            employeeId?: string;
            status?: string;
            type?: string;
            from?: string;
            to?: string;
            limit?: number;
          }) => get<LeaveView[]>('/api/admin/hris/leaves', query),
          request: (input: RequestLeaveInput) => post<LeaveView>('/api/admin/hris/leaves', input),
          decide: (id: string, action: 'approve' | 'reject' | 'cancel', reason = '') =>
            post<LeaveView>(`/api/admin/hris/leaves/${id}/${action}`, { reason }),
        },

        overtime: {
          list: (query?: { employeeId?: string; status?: string; limit?: number }) =>
            get<OvertimeView[]>('/api/admin/hris/overtime', query),
          request: (input: RequestOvertimeInput) =>
            post<OvertimeView>('/api/admin/hris/overtime', input),
          decide: (id: string, action: 'approve' | 'reject' | 'cancel', reason = '') =>
            post<OvertimeView>(`/api/admin/hris/overtime/${id}/${action}`, { reason }),
        },

        holidays: {
          list: (query?: { from?: string; to?: string; includeDrafts?: string }) =>
            get<Holiday[]>('/api/admin/hris/holidays', query),
          create: (input: UpsertHolidayInput) => post<Holiday>('/api/admin/hris/holidays', input),
          remove: (id: string) => del(`/api/admin/hris/holidays/${id}`),
        },
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
      profile: (memberId: string) =>
        get<{
          member: { id: string; fullName: string; avatarUrl: string | null };
          isMe: boolean;
          isFollowing: boolean;
          followerCount: number;
          followingCount: number;
          totals: { activities: number; distanceKm: number; movingSec: number };
          activities: ActivityCardView[];
        }>(`/api/athlete/profile/${memberId}`),
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
      get: (id: string) =>
        get<{ view: RaceEventView; myRace: MyRaceView | null }>(`/api/races/${id}`),
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

/**
 * The window a report covers.
 *
 * Both ends are inclusive calendar days in the studio's timezone, because
 * "yesterday" is a calendar question rather than an instant. Omitting them
 * takes a sensible recent window rather than everything ever.
 */
export type ReportWindow = {
  branchId?: string;
  from?: string;
  to?: string;
};
