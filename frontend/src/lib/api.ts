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
  BankTransferResponse,
  PaymentIntentResponse,
  WithdrawalResponse,
  InboxResponse,
  PreferenceResponse,
  CursorPaged,
  DashboardStatsResponse,
  SSEStatsResponse,
  SystemParamResponse,
  ScoringRuleResponse,
  CircuitBreakerResponse,
} from './api-types'

// ── Base fetch ────────────────────────────────────────────────────────────────

class APIClient {
  private readonly base: string

  constructor(base = '') {
    this.base = base
  }

  private async request<T>(
    path: string,
    init: RequestInit = {},
    token?: string,
  ): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(init.headers as Record<string, string>),
    }
    if (token) headers['Authorization'] = `Bearer ${token}`

    const res = await fetch(`${this.base}${path}`, {
      ...init,
      headers,
      cache: 'no-store',
    })

    if (!res.ok) {
      const body = await res.json().catch(() => ({}))
      const msg = body?.error?.message ?? `HTTP ${res.status}`
      const code = body?.error?.code ?? 'ERR_UNKNOWN'
      throw Object.assign(new Error(msg), { code, status: res.status })
    }

    // 204 No Content — return empty object
    if (res.status === 204) return {} as T
    return res.json()
  }

  // ── Public endpoints (no auth needed) ──────────────────────────────────────

  getExchangeRate(): Promise<PublicExchangeRate> {
    return this.request('/api/exchange-rate')
  }

  // ── User ──────────────────────────────────────────────────────────────────

  getMe(token: string): Promise<UserResponse> {
    return this.request('/api/v1/users/me', {}, token)
  }

  updateMe(token: string, data: Partial<Pick<UserResponse, 'display_name'>>): Promise<UserResponse> {
    return this.request('/api/v1/users/me', { method: 'PATCH', body: JSON.stringify(data) }, token)
  }

  getBalance(token: string): Promise<BalanceResponse> {
    return this.request('/api/v1/users/me/balance', {}, token)
  }

  getLedger(token: string, cursor?: string, limit = 50): Promise<LedgerEntry[]> {
    const q = new URLSearchParams({ limit: String(limit) })
    if (cursor) q.set('cursor', cursor)
    return this.request(`/api/v1/users/me/balance/ledger?${q}`, {}, token)
  }

  // ── Groups ────────────────────────────────────────────────────────────────

  getMyGroups(token: string): Promise<GroupResponse[]> {
    return this.request('/api/v1/groups/me', {}, token)
  }

  getGroup(token: string, id: number): Promise<GroupDetailResponse> {
    return this.request(`/api/v1/groups/${id}`, {}, token)
  }

  getGroupLeaderboard(token: string, id: number, cursor?: string, limit = 50): Promise<CursorPaged<LeaderboardEntry>> {
    const q = new URLSearchParams({ limit: String(limit) })
    if (cursor) q.set('cursor', cursor)
    return this.request(`/api/v1/groups/${id}/leaderboard?${q}`, {}, token)
  }

  getGroupMembers(token: string, id: number): Promise<MemberResponse[]> {
    return this.request<{ data: MemberResponse[] }>(`/api/v1/groups/${id}/members`, {}, token)
      .then((r) => r.data)
  }

  approveGroupMember(token: string, groupId: number, membershipId: number): Promise<MemberResponse> {
    return this.request(`/api/v1/groups/${groupId}/members/${membershipId}/approve`, { method: 'POST' }, token)
  }

  rejectGroupMember(token: string, groupId: number, membershipId: number): Promise<void> {
    return this.request(`/api/v1/groups/${groupId}/members/${membershipId}`, { method: 'DELETE' }, token)
  }

  createGroup(token: string, data: { name: string }): Promise<QuinielaResponse> {
    return this.request('/api/v1/groups', { method: 'POST', body: JSON.stringify(data) }, token)
  }

  joinGroup(token: string, invite_code: string): Promise<void> {
    return this.request('/api/v1/groups/join', { method: 'POST', body: JSON.stringify({ invite_code }) }, token)
  }

  joinGroupWithBalance(token: string, invite_code: string): Promise<void> {
    return this.request('/api/v1/groups/join-with-balance', { method: 'POST', body: JSON.stringify({ invite_code }) }, token)
  }

  leaveGroup(token: string, id: number): Promise<void> {
    return this.request(`/api/v1/groups/${id}/members/me`, { method: 'DELETE' }, token)
  }

  // ── Matches ───────────────────────────────────────────────────────────────

  getMatches(token: string): Promise<MatchResponse[]> {
    return this.request('/api/v1/matches', {}, token)
  }

  // ── Predictions ───────────────────────────────────────────────────────────

  getMyPredictions(token: string): Promise<PredictionResponse[]> {
    return this.request<{ data: PredictionResponse[] }>('/api/v1/predictions/me', {}, token)
      .then((r) => r.data)
  }

  submitPrediction(token: string, data: { match_id: number; home_score: number; away_score: number }): Promise<PredictionResponse> {
    return this.request('/api/v1/predictions', { method: 'POST', body: JSON.stringify(data) }, token)
  }

  updatePrediction(token: string, id: number, data: { home_score: number; away_score: number }): Promise<PredictionResponse> {
    return this.request(`/api/v1/predictions/${id}`, { method: 'PATCH', body: JSON.stringify(data) }, token)
  }

  // ── KYC ───────────────────────────────────────────────────────────────────

  getKYCStatus(token: string): Promise<KYCProfileResponse> {
    return this.request('/api/v1/kyc/status', {}, token)
  }

  getKYCRequirements(token: string): Promise<KYCRequirementsResponse> {
    return this.request('/api/v1/kyc/requirements', {}, token)
  }

  getKYCDocuments(token: string): Promise<KYCDocumentResponse[]> {
    return this.request('/api/v1/kyc/documents', {}, token)
  }

  getKYCEvents(token: string, cursor?: string): Promise<{ events: KYCEventResponse[]; next_cursor: string }> {
    const q = cursor ? `?cursor=${cursor}` : ''
    return this.request(`/api/v1/kyc/events${q}`, {}, token)
  }

  submitKYC(token: string, data: {
    full_name: string; date_of_birth: string; nationality: string;
    document_type: string; document_number: string; address_line: string;
    city: string; country: string; postal_code: string;
  }): Promise<KYCProfileResponse> {
    return this.request('/api/v1/kyc/submit', { method: 'POST', body: JSON.stringify(data) }, token)
  }

  uploadKYCDocument(token: string, formData: FormData): Promise<KYCDocumentResponse> {
    return this.requestFormData('/api/v1/kyc/documents', formData, token)
  }

  // ── Payments ──────────────────────────────────────────────────────────────

  createPaymentIntent(token: string, data: { amount_cents: number; currency: string; provider: string }, idempotencyKey: string): Promise<PaymentIntentResponse> {
    return this.request('/api/v1/payment-intents', {
      method: 'POST',
      body: JSON.stringify(data),
      headers: { 'Idempotency-Key': idempotencyKey } as HeadersInit,
    }, token)
  }

  uploadBankTransfer(token: string, formData: FormData, idempotencyKey: string): Promise<BankTransferResponse> {
    return this.requestFormData('/api/v1/bank-transfers', formData, token, idempotencyKey)
  }

  getMyBankTransfers(token: string): Promise<BankTransferResponse[]> {
    return this.request('/api/v1/bank-transfers', {}, token)
  }

  // ── Withdrawals ───────────────────────────────────────────────────────────

  createWithdrawal(token: string, data: { amount_cents: number; method: string; [key: string]: unknown }, idempotencyKey: string): Promise<WithdrawalResponse> {
    return this.request('/api/v1/withdrawals', {
      method: 'POST',
      body: JSON.stringify(data),
      headers: { 'Idempotency-Key': idempotencyKey } as HeadersInit,
    }, token)
  }

  getMyWithdrawals(token: string): Promise<WithdrawalResponse[]> {
    return this.request('/api/v1/withdrawals', {}, token)
  }

  // ── Notifications ─────────────────────────────────────────────────────────

  getInbox(token: string, limit = 50, offset = 0): Promise<InboxResponse> {
    return this.request(`/api/v1/notifications?limit=${limit}&offset=${offset}`, {}, token)
  }

  markRead(token: string, ids: number[]): Promise<void> {
    return this.request('/api/v1/notifications/mark-read', { method: 'POST', body: JSON.stringify({ ids }) }, token)
  }

  markAllRead(token: string): Promise<void> {
    return this.request('/api/v1/notifications/mark-read', { method: 'POST', body: JSON.stringify({ mark_all: true }) }, token)
  }

  getPreferences(token: string): Promise<PreferenceResponse[]> {
    return this.request('/api/v1/notifications/preferences', {}, token)
  }

  // ── Admin ─────────────────────────────────────────────────────────────────

  adminGetStats(token: string): Promise<DashboardStatsResponse> {
    return this.request('/api/v1/admin/stats', {}, token)
  }

  adminGetKYCQueue(token: string, cursor?: string, status?: string): Promise<CursorPaged<import('./api-types').KYCProfileResponse>> {
    const q = new URLSearchParams()
    if (cursor) q.set('cursor', cursor)
    if (status) q.set('status', status)
    return this.request(`/api/v1/admin/kyc/queue?${q}`, {}, token)
  }

  adminApproveKYC(token: string, profileID: number): Promise<KYCProfileResponse> {
    return this.request(`/api/v1/admin/kyc/profiles/${profileID}/approve`, { method: 'POST' }, token)
  }

  adminRejectKYC(token: string, profileID: number, reason: string): Promise<KYCProfileResponse> {
    return this.request(`/api/v1/admin/kyc/profiles/${profileID}/reject`, { method: 'POST', body: JSON.stringify({ reason }) }, token)
  }

  adminGetExchangeRate(token: string): Promise<AdminExchangeRate> {
    return this.request('/api/v1/admin/exchange-rate/current', {}, token)
  }

  adminGetExchangeRateHistory(token: string, cursor?: string): Promise<CursorPaged<RateHistoryEntry>> {
    const q = cursor ? `?cursor=${cursor}` : ''
    return this.request(`/api/v1/admin/exchange-rate/history${q}`, {}, token)
  }

  adminOverrideExchangeRate(token: string, data: { reference_rate: string; reason: string }): Promise<AdminExchangeRate> {
    return this.request('/api/v1/admin/exchange-rate/override', { method: 'POST', body: JSON.stringify(data) }, token)
  }

  adminRefreshExchangeRate(token: string): Promise<AdminExchangeRate> {
    return this.request('/api/v1/admin/exchange-rate/refresh', { method: 'POST' }, token)
  }

  adminGetSSEStats(token: string): Promise<SSEStatsResponse> {
    return this.request('/api/v1/admin/notifications/sse/stats', {}, token)
  }

  adminGetSystemParams(token: string): Promise<SystemParamResponse[]> {
    return this.request('/api/v1/admin/system-params', {}, token)
  }

  adminGetScoringRules(token: string): Promise<ScoringRuleResponse[]> {
    return this.request('/api/v1/admin/scoring-rules', {}, token)
  }

  adminGetCircuitBreakers(token: string): Promise<CircuitBreakerResponse[]> {
    return this.request('/api/v1/admin/observability/circuit-breakers', {}, token)
  }

  // ── Helpers ───────────────────────────────────────────────────────────────

  private async requestFormData<T>(path: string, formData: FormData, token?: string, idempotencyKey?: string): Promise<T> {
    const headers: Record<string, string> = {}
    if (token) headers['Authorization'] = `Bearer ${token}`
    if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey

    const res = await fetch(`${this.base}${path}`, {
      method: 'POST',
      headers,
      body: formData,
      cache: 'no-store',
    })

    if (!res.ok) {
      const body = await res.json().catch(() => ({}))
      const msg = body?.error?.message ?? `HTTP ${res.status}`
      throw Object.assign(new Error(msg), { code: body?.error?.code, status: res.status })
    }
    return res.json()
  }
}

// Browser-side client hits the BFF proxy — no backend URL needed
export const api = new APIClient()

// Server-side client (Server Components / Route Handlers) hits backend directly
export function serverAPI() {
  const url = process.env.BACKEND_INTERNAL_URL
  if (!url) throw new Error('BACKEND_INTERNAL_URL is not set')
  return new APIClient(url)
}
