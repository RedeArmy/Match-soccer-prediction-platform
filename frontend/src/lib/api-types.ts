// TypeScript types derived from Go response structs (Phase 1 analysis)

// ── Error envelope ───────────────────────────────────────────────────────────
export interface APIError {
  error: {
    schema_version: number
    code: string
    message: string
  }
}

// ── Pagination ───────────────────────────────────────────────────────────────
export interface PageMeta {
  limit: number
  offset: number
}

export interface Paged<T> {
  data: T[]
  page: PageMeta
}

export interface CursorPaged<T> {
  data: T[]
  next_cursor: string
  has_more: boolean
}

// ── Exchange Rate ─────────────────────────────────────────────────────────────
// GET /api/exchange-rate (public, outside /api/v1)
export interface PublicExchangeRate {
  buy_rate:     string   // decimal string, e.g. "7.72"
  sell_rate:    string   // decimal string, e.g. "7.80"
  effective_at: string   // RFC3339
  stale:        boolean
}

// GET /api/v1/admin/exchange-rate/current
export interface AdminExchangeRate extends PublicExchangeRate {
  reference_rate: string
  margin_bps:     number
  source:         string
  fetched_at:     string
}

export interface RateHistoryEntry {
  id:             number
  reference_rate: string
  buy_rate:       string
  sell_rate:      string
  source:         string
  is_override:    boolean
  override_reason: string | null
  effective_at:   string
  created_at:     string
}

// ── User ──────────────────────────────────────────────────────────────────────
export type UserRole = 'user' | 'admin' | 'organizer'

export interface UserResponse {
  id:           number
  clerk_id:     string
  email:        string
  display_name: string
  role:         UserRole
  kyc_tier:     number
  banned_at:    string | null
  created_at:   string
}

export interface UserStatsResponse {
  total_predictions: number
  correct_predictions: number
  groups_joined: number
  total_points: number
  best_rank: number | null
}

// ── Balance ───────────────────────────────────────────────────────────────────
export interface BalanceResponse {
  available_cents:  number
  reserved_cents:   number
  pending_cents:    number
  currency:         string
}

export interface LedgerEntry {
  id:           number
  delta_cents:  number   // positive = credit, negative = debit
  kind:         string   // e.g. "webhook_recurrente", "entry_fee", "prize"
  balance_after: number
  ref_id:       number | null
  ref_type:     string | null
  created_at:   string
}

// ── Match ─────────────────────────────────────────────────────────────────────
export type MatchStatus = 'scheduled' | 'in_progress' | 'finished' | 'cancelled'

export interface CountryInfo {
  id:   number
  name: string
  code: string
}

export interface StateInfo {
  id:      number
  name:    string
  code:    string
  country: CountryInfo
}

export interface CityInfo {
  id:    number
  name:  string
  state: StateInfo
}

export interface StadiumInfo {
  id:       number
  name:     string
  city:     CityInfo
  capacity: number
}

export interface MatchResponse {
  id:          number
  home_team:   string
  away_team:   string
  home_score:  number | null
  away_score:  number | null
  status:      MatchStatus
  kickoff_at:  string | null
  stadium:     StadiumInfo | null
  phase:       string | null
  group_label: string | null
}

// ── Prediction ────────────────────────────────────────────────────────────────
export interface PredictionResponse {
  id:          number
  match_id:    number
  home_score:  number
  away_score:  number
  points:      number | null
  scored_at:   string | null
  created_at:  string
}

// ── Quiniela (group created via API) ─────────────────────────────────────────
export interface QuinielaResponse {
  id:                     number
  name:                   string
  owner_user_id:          number
  invite_code:            string
  invite_code_expires_at: string | null
  status:                 string
  entry_fee:              number
  currency:               string
  created_at:             string
  updated_at:             string
}

// ── Group (Quiniela pool) ──────────────────────────────────────────────────────
export type GroupStatus = 'open' | 'in_progress' | 'finished'

// Returned by GET /api/v1/groups/me — enriched with membership fields.
export interface GroupResponse {
  id:                number
  name:              string
  group_status:      string
  membership_status: string
  role:              'owner' | 'member'
  invite_code:       string
  entry_fee:         number
  currency:          string
}

// Returned by GET /api/v1/groups/:id — full group detail.
export interface GroupDetailResponse {
  id:                    number
  name:                  string
  owner_user_id:         number
  invite_code:           string
  invite_code_expires_at: string | null
  status:                string
  entry_fee:             number
  currency:              string
  created_at:            string
  updated_at:            string
}

export interface MemberResponse {
  id:           number
  quiniela_id:  number
  user_id:      number
  display_name: string
  role:         'owner' | 'member'
  status:       'active' | 'pending' | 'left'
  paid:         boolean
  joined_at:    string | null
  created_at:   string
  updated_at:   string
}

export interface LeaderboardEntry {
  rank:          number
  user_id:       number
  display_name:  string
  points:        number
  prize_cents:   number
  is_current:    boolean
}

// ── Tournament ─────────────────────────────────────────────────────────────────
export interface StandingResponse {
  team:        string
  group_label: string
  played:      number
  wins:        number
  draws:       number
  losses:      number
  goals_for:   number
  goals_against: number
  points:      number
}

export interface SlotResponse {
  id:          number
  stage:       string
  label:       string
  team:        string | null
  confirmed:   boolean
  confirmed_at: string | null
}

// ── KYC ───────────────────────────────────────────────────────────────────────
export type KYCStatus = 'unverified' | 'pending' | 'under_review' | 'approved' | 'rejected' | 'expired' | 'escalated'
export type KYCDocumentType = 'gov_id' | 'selfie' | 'proof_of_address' | 'proof_of_funds'
export type KYCDocumentStatus = 'pending' | 'approved' | 'rejected'

export interface KYCProfileResponse {
  id:              number
  status:          KYCStatus
  tier:            number
  full_name:       string | null
  date_of_birth:   string | null
  nationality:     string | null
  document_type:   string | null
  document_number: string | null
  address_line:    string | null
  city:            string | null
  country:         string | null
  postal_code:     string | null
  rejection_reason: string | null
  risk_score:      number | null
  submitted_at:    string | null
  reviewed_at:     string | null
  created_at:      string
}

export interface KYCDocumentResponse {
  id:            number
  document_type: KYCDocumentType
  status:        KYCDocumentStatus
  content_type:  string
  file_size:     number
  verified_at:   string | null
  created_at:    string
}

export interface KYCRequirementsResponse {
  required_documents: KYCDocumentType[]
  pending_documents:  KYCDocumentType[]
  submitted_documents: KYCDocumentType[]
}

export interface KYCEventResponse {
  id:         number
  event_type: string
  actor_id:   number | null
  notes:      string | null
  created_at: string
}

// ── Payments ──────────────────────────────────────────────────────────────────
export interface BankResponse {
  id:   number
  name: string
}

export interface BankAccountTypeResponse {
  id:   number
  name: string
}

export interface AdminBankResponse {
  id:     number
  name:   string
  active: boolean
}

export type BankTransferStatus = 'pending' | 'approved' | 'rejected'
export type WithdrawalStatus   = 'pending' | 'approved' | 'rejected' | 'processed'

export interface BankTransferResponse {
  id:            number
  amount_cents:  number
  currency:      string
  status:        BankTransferStatus
  notes:         string | null
  reviewed_by:   number | null
  reviewed_at:   string | null
  created_at:    string
}

export interface PaymentIntentResponse {
  token:         string          // opaque single-use token (PayPal custom_id)
  amount_cents:  number
  currency:      string
  expires_at:    string
  redirect_url?: string          // only set for Recurrente checkouts
}

export interface PayPalOrderResponse {
  id: string
}

export interface WithdrawalResponse {
  id:              number
  amount_cents:    number
  currency:        string
  method:          string
  status:          WithdrawalStatus
  payout_reference: string | null
  notes:           string | null
  created_at:      string
}

export interface WithdrawalLimits {
  min_gtq_cents: number
  max_gtq_cents: number
  min_usd_cents: number
}

// ── Notifications ─────────────────────────────────────────────────────────────
export interface NotificationResponse {
  id:         number
  event_type: string
  title:      string
  body:       string
  action_url: string | null
  read:       boolean
  created_at: string
}

export interface InboxResponse {
  notifications: NotificationResponse[]
  unread_count:  number
  total:         number
}

export interface PreferenceResponse {
  event_type:    string
  channel_email: boolean
  channel_push:  boolean
  channel_inapp: boolean
}

// ── SSE notification payload (event_type inside data) ─────────────────────────
export interface SSENotificationPayload {
  id:         number
  event_type: string
  title:      string
  body:       string
  action_url: string | null
  read:       boolean
  created_at: string
}

// ── Admin ─────────────────────────────────────────────────────────────────────
export interface AdminUserResponse extends UserResponse {
  groups_count: number
  balance_cents: number
}

export interface DashboardStatsResponse {
  total_users:       number
  active_users_7d:   number
  total_groups:      number
  active_groups:     number
  pending_kyc:       number
  pending_withdrawals: number
  pending_bank_transfers: number
  total_balance_cents: number
}

export interface SSEStatsResponse {
  connected_users: number
  total_connections: number
  dropped_events:  number
}

export interface SystemParamResponse {
  key:         string
  value:       string
  description: string
  is_runtime:  boolean
  updated_at:  string
}

export interface ScoringRuleResponse {
  phase:           string
  exact_score:     number
  correct_outcome: number
  correct_goals:   number
  updated_at:      string
}

export interface CircuitBreakerResponse {
  name:       string
  state:      'closed' | 'open' | 'half_open'
  failures:   number
  last_failure_at: string | null
}

export interface N8nWorkflowResponse {
  id:     string
  name:   string
  active: boolean
}
