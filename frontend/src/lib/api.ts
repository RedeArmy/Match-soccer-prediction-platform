// Typed API client — all calls route through the BFF proxy (Next.js API routes).
// Server components use BACKEND_INTERNAL_URL directly; client components hit /api/v1/*.
import type {
  PublicExchangeRate,
  AdminExchangeRate,
  RateHistoryEntry,
  UserResponse,
  BalanceResponse,
  LedgerEntry,
  QuinielaResponse,
  GroupResponse,
  GroupDetailResponse,
  MemberResponse,
  LeaderboardEntry,
  MatchResponse,
  PredictionResponse,
  KYCProfileResponse,
  KYCDocumentResponse,
  KYCRequirementsResponse,
  KYCEventResponse,
  BankResponse,
  BankAccountTypeResponse,
  AdminBankResponse,
  BankTransferResponse,
  PaymentIntentResponse,
  PaymentIntentSummary,
  PaymentIntentAdminResponse,
  PagedWithTotal,
  PageMeta,
  PayPalOrderResponse,
  WithdrawalResponse,
  WithdrawalLimits,
  InboxResponse,
  PreferenceResponse,
  DashboardStatsResponse,
  SSEStatsResponse,
  SystemParamResponse,
  SystemParamHistoryResponse,
  ScoringRuleResponse,
  CircuitBreakerResponse,
  MetricsSummaryResponse,
  MetricsQueryResponse,
  LogsSearchResponse,
  TournamentModeRequest,
  CursorPaged,
  AdminUserResponse,
  AdminUserProfileResponse,
  LivePredictionsResponse,
  TournamentSlotResponse,
} from "./api-types";

// ── Base fetch ────────────────────────────────────────────────────────────────

class APIClient {
  private readonly base: string;

  constructor(base = "") {
    this.base = base;
  }

  private async request<T>(
    path: string,
    init: RequestInit = {},
    token?: string,
  ): Promise<T> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...(init.headers as Record<string, string>),
    };
    if (token) headers["Authorization"] = `Bearer ${token}`;

    const res = await fetch(`${this.base}${path}`, {
      ...init,
      headers,
      cache: "no-store",
    });

    if (!res.ok) {
      if (res.status === 401 && !!token && globalThis.window !== undefined) {
        globalThis.dispatchEvent(new CustomEvent("wcq:session-expired"));
      }
      // Read the body as text first so we can attempt JSON parsing without
      // consuming the stream twice. If the upstream returns an HTML error page
      // (reverse proxy, CDN) json() would throw and silently produce {}, masking
      // the actual response content.
      const text = await res.text().catch(() => "");
      let body: Record<string, unknown> = {};
      try {
        body = JSON.parse(text) as Record<string, unknown>;
      } catch {
        // Non-JSON body (HTML error page, plain text, etc.). The status code is
        // used as the fallback message; raw text is available in debug tooling.
      }
      const msg =
        (body?.error as Record<string, unknown> | undefined)?.message ??
        `HTTP ${res.status}`;
      const code =
        (body?.error as Record<string, unknown> | undefined)?.code ??
        "ERR_UNKNOWN";
      throw Object.assign(new Error(String(msg)), {
        code: String(code),
        status: res.status,
      });
    }

    // 204 No Content — return empty object
    if (res.status === 204) return {} as T;
    return res.json();
  }

  // ── Public endpoints (no auth needed) ──────────────────────────────────────

  getExchangeRate(): Promise<PublicExchangeRate> {
    return this.request("/api/exchange-rate");
  }

  // ── Auth ──────────────────────────────────────────────────────────────────

  logout(token: string): Promise<void> {
    return this.request("/api/v1/auth/logout", { method: "POST" }, token);
  }

  // ── User ──────────────────────────────────────────────────────────────────

  getMe(token: string): Promise<UserResponse> {
    return this.request("/api/v1/users/me", {}, token);
  }

  updateMe(
    token: string,
    data: Partial<Pick<UserResponse, "display_name" | "timezone">>,
  ): Promise<UserResponse> {
    return this.request(
      "/api/v1/users/me",
      { method: "PATCH", body: JSON.stringify(data) },
      token,
    );
  }

  getBalance(token: string): Promise<BalanceResponse> {
    return this.request("/api/v1/users/me/balance", {}, token);
  }

  getLedger(
    token: string,
    cursor?: string,
    limit = 50,
  ): Promise<LedgerEntry[]> {
    const q = new URLSearchParams({ limit: String(limit) });
    if (cursor) q.set("cursor", cursor);
    return this.request(`/api/v1/users/me/balance/ledger?${q}`, {}, token);
  }

  // ── Groups ────────────────────────────────────────────────────────────────

  getMyGroups(token: string): Promise<GroupResponse[]> {
    return this.request("/api/v1/groups/me", {}, token);
  }

  getGroup(token: string, id: number): Promise<GroupDetailResponse> {
    return this.request(`/api/v1/groups/${id}`, {}, token);
  }

  getGroupLeaderboard(
    token: string,
    id: number,
    breakdown = false,
  ): Promise<{
    entries: LeaderboardEntry[];
    active_paid_members: number;
    winner_count: number;
    eligible_for_prizes: boolean;
  }> {
    const q = new URLSearchParams();
    if (breakdown) q.set("breakdown", "true");
    const qs = q.toString();
    const suffix = qs ? `?${qs}` : "";
    return this.request(`/api/v1/groups/${id}/leaderboard${suffix}`, {}, token);
  }

  setTournamentMode(
    token: string,
    id: number,
    data: TournamentModeRequest,
  ): Promise<GroupDetailResponse> {
    return this.request(
      `/api/v1/groups/${id}/tournament-mode`,
      { method: "PATCH", body: JSON.stringify(data) },
      token,
    );
  }

  updateRequireApproval(
    token: string,
    id: number,
    requireApproval: boolean,
  ): Promise<GroupDetailResponse> {
    return this.request(
      `/api/v1/groups/${id}/require-approval`,
      {
        method: "PATCH",
        body: JSON.stringify({ require_approval: requireApproval }),
      },
      token,
    );
  }

  updateScoreFromZero(
    token: string,
    id: number,
    scoreFromZero: boolean,
  ): Promise<GroupDetailResponse> {
    return this.request(
      `/api/v1/groups/${id}/score-from-zero`,
      {
        method: "PATCH",
        body: JSON.stringify({ score_from_zero: scoreFromZero }),
      },
      token,
    );
  }

  getGroupMembers(token: string, id: number): Promise<MemberResponse[]> {
    return this.request<{ data: MemberResponse[] }>(
      `/api/v1/groups/${id}/members`,
      {},
      token,
    ).then((r) => r.data);
  }

  approveGroupMember(
    token: string,
    groupId: number,
    membershipId: number,
  ): Promise<MemberResponse> {
    return this.request(
      `/api/v1/groups/${groupId}/members/${membershipId}/approve`,
      { method: "POST" },
      token,
    );
  }

  rejectGroupMember(
    token: string,
    groupId: number,
    membershipId: number,
  ): Promise<void> {
    return this.request(
      `/api/v1/groups/${groupId}/members/${membershipId}`,
      { method: "DELETE" },
      token,
    );
  }

  checkGroupName(token: string, name: string): Promise<{ available: boolean }> {
    return this.request(
      `/api/v1/groups/check-name?name=${encodeURIComponent(name)}`,
      {},
      token,
    );
  }

  createGroup(
    token: string,
    data: { name: string },
  ): Promise<QuinielaResponse> {
    return this.request(
      "/api/v1/groups",
      { method: "POST", body: JSON.stringify(data) },
      token,
    );
  }

  joinGroup(token: string, invite_code: string): Promise<void> {
    return this.request(
      "/api/v1/groups/join",
      { method: "POST", body: JSON.stringify({ invite_code }) },
      token,
    );
  }

  joinGroupWithBalance(token: string, invite_code: string): Promise<void> {
    return this.request(
      "/api/v1/groups/join-with-balance",
      { method: "POST", body: JSON.stringify({ invite_code }) },
      token,
    );
  }

  leaveGroup(token: string, id: number): Promise<void> {
    return this.request(
      `/api/v1/groups/${id}/members/me`,
      { method: "DELETE" },
      token,
    );
  }

  getGroupLivePredictions(
    token: string,
    id: number,
  ): Promise<LivePredictionsResponse> {
    return this.request(`/api/v1/groups/${id}/live-predictions`, {}, token);
  }

  // ── Matches ───────────────────────────────────────────────────────────────

  getMatches(token: string): Promise<MatchResponse[]> {
    return this.request("/api/v1/matches", {}, token);
  }

  // ── Teams ─────────────────────────────────────────────────────────────────

  getTeams(): Promise<string[]> {
    return this.request<string[]>("/api/v1/teams");
  }

  // ── Tournament slots ──────────────────────────────────────────────────────

  getSlots(token?: string | null): Promise<TournamentSlotResponse[]> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    return fetch("/api/v1/tournament/slots", { headers }).then((res) => {
      if (!res.ok) throw new Error("Failed to fetch slots");
      return res.json();
    });
  }

  adminConfirmSlot(
    token: string,
    slotId: number,
    team: string,
  ): Promise<TournamentSlotResponse> {
    return this.request(
      `/api/v1/tournament/slots/${slotId}`,
      { method: "PATCH", body: JSON.stringify({ team }) },
      token,
    );
  }

  adminUpdateMatchSlots(
    token: string,
    matchId: number,
    homeSlotId: number | null,
    awaySlotId: number | null,
  ): Promise<MatchResponse> {
    return this.request(
      `/api/v1/matches/${matchId}/slots`,
      {
        method: "PATCH",
        body: JSON.stringify({
          home_slot_id: homeSlotId,
          away_slot_id: awaySlotId,
        }),
      },
      token,
    );
  }

  // ── Predictions ───────────────────────────────────────────────────────────

  getMyPredictions(token: string): Promise<PredictionResponse[]> {
    return this.request<{ data: PredictionResponse[] }>(
      "/api/v1/predictions/me",
      {},
      token,
    ).then((r) => r.data);
  }

  submitPrediction(
    token: string,
    data: { match_id: number; home_score: number; away_score: number },
  ): Promise<PredictionResponse> {
    return this.request(
      "/api/v1/predictions",
      { method: "POST", body: JSON.stringify(data) },
      token,
    );
  }

  updatePrediction(
    token: string,
    id: number,
    data: { home_score: number; away_score: number },
  ): Promise<PredictionResponse> {
    return this.request(
      `/api/v1/predictions/${id}`,
      { method: "PATCH", body: JSON.stringify(data) },
      token,
    );
  }

  // ── KYC ───────────────────────────────────────────────────────────────────

  getKYCStatus(token: string): Promise<KYCProfileResponse> {
    return this.request("/api/v1/kyc/status", {}, token);
  }

  getKYCRequirements(token: string): Promise<KYCRequirementsResponse> {
    return this.request("/api/v1/kyc/requirements", {}, token);
  }

  getKYCDocuments(token: string): Promise<KYCDocumentResponse[]> {
    return this.request("/api/v1/kyc/documents", {}, token);
  }

  getKYCEvents(
    token: string,
    cursor?: string,
  ): Promise<{ events: KYCEventResponse[]; next_cursor: string }> {
    const q = cursor ? `?cursor=${cursor}` : "";
    return this.request(`/api/v1/kyc/events${q}`, {}, token);
  }

  submitKYC(
    token: string,
    data: {
      full_name: string;
      date_of_birth: string;
      nationality: string;
      document_type: string;
      document_number: string;
      address_line: string;
      city: string;
      country: string;
      postal_code: string;
    },
  ): Promise<KYCProfileResponse> {
    return this.request(
      "/api/v1/kyc/submit",
      { method: "POST", body: JSON.stringify(data) },
      token,
    );
  }

  uploadKYCDocument(
    token: string,
    formData: FormData,
  ): Promise<KYCDocumentResponse> {
    return this.requestFormData("/api/v1/kyc/documents", formData, token);
  }

  deleteKYCDocument(token: string, docID: number): Promise<void> {
    return this.request(
      `/api/v1/kyc/documents/${docID}`,
      { method: "DELETE" },
      token,
    );
  }

  // ── Payments ──────────────────────────────────────────────────────────────

  createPaymentIntent(
    token: string,
    data: { amount_cents: number; currency: string; provider: string },
    idempotencyKey: string,
  ): Promise<PaymentIntentResponse> {
    return this.request(
      "/api/v1/payment-intents",
      {
        method: "POST",
        body: JSON.stringify(data),
        headers: { "Idempotency-Key": idempotencyKey } as HeadersInit,
      },
      token,
    );
  }

  createPayPalOrder(
    token: string,
    data: { amount_cents: number; currency: "USD" },
  ): Promise<PayPalOrderResponse> {
    return this.request(
      "/api/v1/paypal/create-order",
      {
        method: "POST",
        body: JSON.stringify(data),
      },
      token,
    );
  }

  uploadBankTransfer(
    token: string,
    formData: FormData,
    idempotencyKey: string,
  ): Promise<BankTransferResponse> {
    return this.requestFormData(
      "/api/v1/bank-transfers",
      formData,
      token,
      idempotencyKey,
    );
  }

  getMyBankTransfers(token: string): Promise<BankTransferResponse[]> {
    return this.request("/api/v1/bank-transfers", {}, token);
  }

  listMyPendingIntents(token: string): Promise<PaymentIntentSummary[]> {
    return this.request("/api/v1/payment-intents/my", {}, token);
  }

  listMyIntents(token: string): Promise<PaymentIntentSummary[]> {
    return this.request("/api/v1/payment-intents/my/all", {}, token);
  }

  uploadComprobante(
    token: string,
    intentToken: string,
    formData: FormData,
  ): Promise<void> {
    return this.requestFormData(
      `/api/v1/payment-intents/${encodeURIComponent(intentToken)}/comprobante`,
      formData,
      token,
    );
  }

  resubmitForReview(
    token: string,
    intentToken: string,
    formData: FormData,
  ): Promise<PaymentIntentSummary> {
    return this.requestFormData(
      `/api/v1/payment-intents/${encodeURIComponent(intentToken)}/resubmit`,
      formData,
      token,
    );
  }

  cancelIntent(authToken: string, intentToken: string): Promise<void> {
    return this.request(
      `/api/v1/payment-intents/${encodeURIComponent(intentToken)}/cancel`,
      { method: "POST" },
      authToken,
    );
  }

  adminRequestComprobante(
    token: string,
    id: number,
  ): Promise<PaymentIntentAdminResponse> {
    return this.request(
      `/api/v1/admin/payment-intents/${id}/request-comprobante`,
      { method: "POST" },
      token,
    );
  }

  // ── Admin: payment intents ─────────────────────────────────────────────────

  adminListPaymentIntents(
    token: string,
    params?: {
      provider?: string;
      status?: string;
      page?: number;
      limit?: number;
    },
  ): Promise<PagedWithTotal<PaymentIntentAdminResponse> & { page: PageMeta }> {
    const q = new URLSearchParams();
    if (params?.provider) q.set("provider", params.provider);
    if (params?.status) q.set("status", params.status);
    if (params?.page) q.set("page", String(params.page));
    if (params?.limit) q.set("limit", String(params.limit ?? 15));
    return this.request(`/api/v1/admin/payment-intents?${q}`, {}, token);
  }

  adminCreditPaymentIntent(
    token: string,
    id: number,
    notes?: string,
  ): Promise<PaymentIntentAdminResponse> {
    return this.request(
      `/api/v1/admin/payment-intents/${id}/credit`,
      { method: "POST", body: JSON.stringify({ notes: notes ?? "" }) },
      token,
    );
  }

  adminRejectPaymentIntent(
    token: string,
    id: number,
    notes: string,
  ): Promise<PaymentIntentAdminResponse> {
    return this.request(
      `/api/v1/admin/payment-intents/${id}/reject`,
      { method: "POST", body: JSON.stringify({ notes }) },
      token,
    );
  }

  adminPaymentIntentComprobanteUrl(id: number): string {
    return `/api/v1/admin/payment-intents/${id}/comprobante`;
  }

  // ── Withdrawals ───────────────────────────────────────────────────────────

  getWithdrawalLimits(token: string): Promise<WithdrawalLimits> {
    return this.request("/api/v1/withdrawals/limits", {}, token);
  }

  createWithdrawal(
    token: string,
    data: {
      amount_cents: number;
      currency: string;
      method: string;
      payout_details: Record<string, string>;
    },
    idempotencyKey: string,
  ): Promise<WithdrawalResponse> {
    return this.request(
      "/api/v1/withdrawals",
      {
        method: "POST",
        body: JSON.stringify(data),
        headers: { "Idempotency-Key": idempotencyKey } as HeadersInit,
      },
      token,
    );
  }

  getMyWithdrawals(token: string): Promise<WithdrawalResponse[]> {
    return this.request("/api/v1/withdrawals", {}, token);
  }

  getBanks(token: string): Promise<BankResponse[]> {
    return this.request("/api/v1/banks", {}, token);
  }

  getBankAccountTypes(token: string): Promise<BankAccountTypeResponse[]> {
    return this.request("/api/v1/bank-account-types", {}, token);
  }

  // ── Admin: matches ───────────────────────────────────────────────────────

  adminStartMatch(token: string, id: number): Promise<MatchResponse> {
    return this.request(
      `/api/v1/matches/${id}/start`,
      { method: "POST" },
      token,
    );
  }

  adminUpdateMatchResult(
    token: string,
    id: number,
    data: { home_score: number; away_score: number; win_method?: string },
  ): Promise<MatchResponse> {
    return this.request(
      `/api/v1/matches/${id}`,
      { method: "PATCH", body: JSON.stringify(data) },
      token,
    );
  }

  adminCorrectMatchResult(
    token: string,
    id: number,
    data: { home_score: number; away_score: number; win_method?: string },
  ): Promise<MatchResponse> {
    return this.request(
      `/api/v1/matches/${id}/correct-result`,
      { method: "POST", body: JSON.stringify(data) },
      token,
    );
  }

  adminCancelMatch(token: string, id: number): Promise<MatchResponse> {
    return this.request(
      `/api/v1/matches/${id}/cancel`,
      { method: "POST" },
      token,
    );
  }

  adminTriggerDailySync(
    token: string,
    startDate?: string,
    endDate?: string,
  ): Promise<{
    total: number;
    linked: number;
    kickoffs_updated: number;
    scores_corrected: number;
  }> {
    const params = new URLSearchParams();
    if (startDate) params.set("start_date", startDate);
    if (endDate) params.set("end_date", endDate);
    const qs = params.toString();
    const url = "/api/v1/admin/match-sync/today" + (qs ? "?" + qs : "");
    return this.request(url, { method: "POST" }, token);
  }

  // ── Admin: bank transfers ─────────────────────────────────────────────────

  adminListBankTransfers(token: string): Promise<BankTransferResponse[]> {
    return this.request("/api/v1/admin/bank-transfers", {}, token);
  }

  adminApproveBankTransfer(
    token: string,
    id: number,
    data: { notes?: string; approved_amount_cents?: number },
  ): Promise<BankTransferResponse> {
    return this.request(
      `/api/v1/admin/bank-transfers/${id}/approve`,
      {
        method: "POST",
        body: JSON.stringify(data),
      },
      token,
    );
  }

  adminRejectBankTransfer(
    token: string,
    id: number,
    notes: string,
  ): Promise<BankTransferResponse> {
    return this.request(
      `/api/v1/admin/bank-transfers/${id}/reject`,
      {
        method: "POST",
        body: JSON.stringify({ notes }),
      },
      token,
    );
  }

  // ── Admin: withdrawals ────────────────────────────────────────────────────

  adminListWithdrawals(
    token: string,
    status?: string,
  ): Promise<WithdrawalResponse[]> {
    const qs = status ? `?status=${encodeURIComponent(status)}` : "";
    return this.request(`/api/v1/admin/withdrawals${qs}`, {}, token);
  }

  adminApproveWithdrawal(
    token: string,
    id: number,
    notes?: string,
  ): Promise<WithdrawalResponse> {
    return this.request(
      `/api/v1/admin/withdrawals/${id}/approve`,
      {
        method: "POST",
        body: JSON.stringify({ notes: notes ?? "" }),
      },
      token,
    );
  }

  adminRejectWithdrawal(
    token: string,
    id: number,
    notes: string,
  ): Promise<WithdrawalResponse> {
    return this.request(
      `/api/v1/admin/withdrawals/${id}/reject`,
      {
        method: "POST",
        body: JSON.stringify({ notes }),
      },
      token,
    );
  }

  adminProcessWithdrawal(
    token: string,
    id: number,
  ): Promise<WithdrawalResponse> {
    return this.request(
      `/api/v1/admin/withdrawals/${id}/process`,
      { method: "POST" },
      token,
    );
  }

  // ── Admin: banks ──────────────────────────────────────────────────────────

  adminListBanks(
    token: string,
    onlyActive?: boolean,
  ): Promise<AdminBankResponse[]> {
    const qs = onlyActive ? "?active=true" : "";
    return this.request(`/api/v1/admin/banks${qs}`, {}, token);
  }

  adminCreateBank(token: string, name: string): Promise<AdminBankResponse> {
    return this.request(
      "/api/v1/admin/banks",
      { method: "POST", body: JSON.stringify({ name }) },
      token,
    );
  }

  adminSetBankActive(
    token: string,
    id: number,
    active: boolean,
  ): Promise<AdminBankResponse> {
    return this.request(
      `/api/v1/admin/banks/${id}/active`,
      { method: "PATCH", body: JSON.stringify({ active }) },
      token,
    );
  }

  // ── Admin: account types ──────────────────────────────────────────────────

  adminListBankAccountTypes(
    token: string,
    onlyActive?: boolean,
  ): Promise<AdminBankResponse[]> {
    const qs = onlyActive ? "?active=true" : "";
    return this.request(`/api/v1/admin/bank-account-types${qs}`, {}, token);
  }

  adminCreateBankAccountType(
    token: string,
    name: string,
  ): Promise<AdminBankResponse> {
    return this.request(
      "/api/v1/admin/bank-account-types",
      { method: "POST", body: JSON.stringify({ name }) },
      token,
    );
  }

  adminSetBankAccountTypeActive(
    token: string,
    id: number,
    active: boolean,
  ): Promise<AdminBankResponse> {
    return this.request(
      `/api/v1/admin/bank-account-types/${id}/active`,
      { method: "PATCH", body: JSON.stringify({ active }) },
      token,
    );
  }

  // ── Notifications ─────────────────────────────────────────────────────────

  getInbox(token: string, limit = 50, offset = 0): Promise<InboxResponse> {
    return this.request(
      `/api/v1/notifications?limit=${limit}&offset=${offset}`,
      {},
      token,
    );
  }

  markRead(token: string, ids: number[]): Promise<void> {
    return this.request(
      "/api/v1/notifications/mark-read",
      { method: "POST", body: JSON.stringify({ ids }) },
      token,
    );
  }

  markAllRead(token: string): Promise<void> {
    return this.request(
      "/api/v1/notifications/mark-read",
      { method: "POST", body: JSON.stringify({ mark_all: true }) },
      token,
    );
  }

  getPreferences(token: string): Promise<PreferenceResponse[]> {
    return this.request("/api/v1/notifications/preferences", {}, token);
  }

  // ── Admin ─────────────────────────────────────────────────────────────────

  adminGetStats(token: string): Promise<DashboardStatsResponse> {
    return this.request("/api/v1/admin/stats", {}, token);
  }

  adminListUsers(
    token: string,
    params?: {
      search?: string;
      banned?: boolean;
      role?: string;
      cursor?: string;
    },
  ): Promise<CursorPaged<AdminUserResponse>> {
    const q = new URLSearchParams();
    if (params?.search) q.set("search", params.search);
    if (params?.banned !== undefined) q.set("banned", String(params.banned));
    if (params?.role) q.set("role", params.role);
    if (params?.cursor) q.set("cursor", params.cursor);
    q.set("limit", "50");
    return this.request(`/api/v1/admin/users?${q}`, {}, token);
  }

  adminGetUserProfile(
    token: string,
    id: number,
  ): Promise<AdminUserProfileResponse> {
    return this.request(`/api/v1/admin/users/${id}`, {}, token);
  }

  adminBanUser(
    token: string,
    id: number,
    reason: string,
  ): Promise<AdminUserResponse> {
    return this.request(
      `/api/v1/admin/users/${id}/ban`,
      { method: "POST", body: JSON.stringify({ reason }) },
      token,
    );
  }

  adminUnbanUser(token: string, id: number): Promise<AdminUserResponse> {
    return this.request(
      `/api/v1/admin/users/${id}/ban`,
      { method: "DELETE" },
      token,
    );
  }

  adminSetUserRole(
    token: string,
    id: number,
    role: string,
  ): Promise<AdminUserResponse> {
    return this.request(
      `/api/v1/admin/users/${id}/role`,
      { method: "PATCH", body: JSON.stringify({ role }) },
      token,
    );
  }

  adminGetKYCQueue(
    token: string,
    status?: string,
  ): Promise<KYCProfileResponse[]> {
    const q = new URLSearchParams();
    if (status) q.set("status", status);
    return this.request(`/api/v1/admin/kyc/queue?${q}`, {}, token);
  }

  adminApproveKYC(
    token: string,
    profileID: number,
    tier = 2,
  ): Promise<KYCProfileResponse> {
    return this.request(
      `/api/v1/admin/kyc/profiles/${profileID}/approve`,
      { method: "POST", body: JSON.stringify({ tier }) },
      token,
    );
  }

  adminRejectKYC(
    token: string,
    profileID: number,
    reason: string,
  ): Promise<KYCProfileResponse> {
    return this.request(
      `/api/v1/admin/kyc/profiles/${profileID}/reject`,
      { method: "POST", body: JSON.stringify({ reason }) },
      token,
    );
  }

  adminGetExchangeRate(token: string): Promise<AdminExchangeRate> {
    return this.request("/api/v1/admin/exchange-rate/current", {}, token);
  }

  adminGetExchangeRateHistory(
    token: string,
    cursor?: string,
  ): Promise<RateHistoryEntry[]> {
    const q = cursor ? `?cursor=${cursor}` : "";
    return this.request(`/api/v1/admin/exchange-rate/history${q}`, {}, token);
  }

  adminOverrideExchangeRate(
    token: string,
    data: { reference_rate: string; reason: string },
  ): Promise<AdminExchangeRate> {
    return this.request(
      "/api/v1/admin/exchange-rate/override",
      { method: "POST", body: JSON.stringify(data) },
      token,
    );
  }

  adminRefreshExchangeRate(token: string): Promise<AdminExchangeRate> {
    return this.request(
      "/api/v1/admin/exchange-rate/refresh",
      { method: "POST" },
      token,
    );
  }

  adminGetSSEStats(token: string): Promise<SSEStatsResponse> {
    return this.request("/api/v1/admin/notifications/sse/stats", {}, token);
  }

  adminGetSystemParams(token: string): Promise<SystemParamResponse[]> {
    return this.request("/api/v1/admin/system-params", {}, token);
  }

  adminSetSystemParam(
    token: string,
    key: string,
    value: string,
  ): Promise<SystemParamResponse> {
    return this.request(
      `/api/v1/admin/system-params/${encodeURIComponent(key)}`,
      {
        method: "PATCH",
        body: JSON.stringify({ value }),
      },
      token,
    );
  }

  adminResetSystemParam(
    token: string,
    key: string,
  ): Promise<SystemParamResponse> {
    return this.request(
      `/api/v1/admin/system-params/${encodeURIComponent(key)}/reset`,
      {
        method: "POST",
      },
      token,
    );
  }

  adminGetSystemParamHistory(
    token: string,
    key: string,
    cursor?: string,
  ): Promise<CursorPaged<SystemParamHistoryResponse>> {
    const q = cursor ? `?cursor=${encodeURIComponent(cursor)}` : "";
    return this.request(
      `/api/v1/admin/system-params/${encodeURIComponent(key)}/history${q}`,
      {},
      token,
    );
  }

  adminGetScoringRules(token: string): Promise<ScoringRuleResponse[]> {
    return this.request("/api/v1/admin/scoring-rules", {}, token);
  }

  adminGetCircuitBreakers(token: string): Promise<CircuitBreakerResponse[]> {
    return this.request(
      "/api/v1/admin/observability/circuit-breakers",
      {},
      token,
    );
  }

  adminGetMetricsSummary(token: string): Promise<MetricsSummaryResponse> {
    return this.request(
      "/api/v1/admin/observability/metrics/summary",
      {},
      token,
    );
  }

  adminQueryMetrics(token: string, q: string): Promise<MetricsQueryResponse> {
    return this.request(
      `/api/v1/admin/observability/metrics/query?q=${encodeURIComponent(q)}`,
      {},
      token,
    );
  }

  adminSearchLogs(
    token: string,
    params: { q?: string; since?: number; limit?: number },
  ): Promise<LogsSearchResponse> {
    const qs = new URLSearchParams();
    if (params.q) qs.set("q", params.q);
    if (params.since) qs.set("since", String(params.since));
    if (params.limit) qs.set("limit", String(params.limit));
    return this.request(
      `/api/v1/admin/observability/logs?${qs.toString()}`,
      {},
      token,
    );
  }

  // ── Helpers ───────────────────────────────────────────────────────────────

  private async requestFormData<T>(
    path: string,
    formData: FormData,
    token?: string,
    idempotencyKey?: string,
  ): Promise<T> {
    const headers: Record<string, string> = {};
    if (token) headers["Authorization"] = `Bearer ${token}`;
    if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

    const res = await fetch(`${this.base}${path}`, {
      method: "POST",
      headers,
      body: formData,
      cache: "no-store",
    });

    if (!res.ok) {
      if (res.status === 401 && !!token && globalThis.window !== undefined) {
        globalThis.dispatchEvent(new CustomEvent("wcq:session-expired"));
      }
      const body = await res.json().catch(() => ({}));
      const msg = body?.error?.message ?? `HTTP ${res.status}`;
      throw Object.assign(new Error(msg), {
        code: body?.error?.code,
        status: res.status,
      });
    }
    return res.json();
  }
}

// Browser-side client hits the BFF proxy — no backend URL needed
export const api = new APIClient();

// Server-side client (Server Components / Route Handlers) hits backend directly
export function serverAPI() {
  const url = process.env.BACKEND_INTERNAL_URL;
  if (!url) throw new Error("BACKEND_INTERNAL_URL is not set");
  return new APIClient(url);
}
