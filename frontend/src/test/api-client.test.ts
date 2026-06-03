import { describe, it, expect, vi, beforeEach } from 'vitest'

// Hoist Clerk mock so it is available before module imports
vi.mock('@clerk/nextjs/server', () => ({ auth: vi.fn() }))

// ── Fetch mock ────────────────────────────────────────────────────────────────

function makeResponse(
  body: unknown,
  status = 200,
  headers: Record<string, string> = {},
): Response {
  const bodyStr = body === null ? null : JSON.stringify(body)
  return new Response(bodyStr, {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  })
}

const mockFetch = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', mockFetch)

// Import after mocks are in place
import { api, serverAPI } from '@/lib/api'
import { getServerToken } from '@/lib/auth'
import { auth } from '@clerk/nextjs/server'

// ── api (browser singleton) ───────────────────────────────────────────────────

describe('api – GET request without token', () => {
  beforeEach(() => mockFetch.mockReset())

  it('does not send Authorization header', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ buy_rate: '7.72', sell_rate: '7.80', effective_at: '', stale: false }))
    await api.getExchangeRate()
    const [, init] = mockFetch.mock.calls[0]
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Authorization']).toBeUndefined()
  })
})

describe('api – GET request with token', () => {
  beforeEach(() => mockFetch.mockReset())

  it('sends Authorization: Bearer <token>', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ available_cents: 0, reserved_cents: 0, pending_cents: 0 }))
    await api.getBalance('tok_test_123')
    const [, init] = mockFetch.mock.calls[0]
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Authorization']).toBe('Bearer tok_test_123')
  })
})

describe('api – error response with parseable body', () => {
  beforeEach(() => mockFetch.mockReset())

  it('throws with .message, .code, .status', async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ error: { message: 'Not found', code: 'ERR_NOT_FOUND' } }, 404),
    )
    const err = await api.getBalance('tok').catch(e => e)
    expect(err.message).toBe('Not found')
    expect(err.code).toBe('ERR_NOT_FOUND')
    expect(err.status).toBe(404)
  })
})

describe('api – error response with unparseable body', () => {
  beforeEach(() => mockFetch.mockReset())

  it('throws with fallback message and ERR_UNKNOWN code', async () => {
    mockFetch.mockResolvedValueOnce(new Response('bad gateway', { status: 503 }))
    const err = await api.getBalance('tok').catch(e => e)
    expect(err.message).toBe('HTTP 503')
    expect(err.code).toBe('ERR_UNKNOWN')
    expect(err.status).toBe(503)
  })
})

describe('api – 204 No Content', () => {
  beforeEach(() => mockFetch.mockReset())

  it('returns empty object', async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
    const result = await api.leaveGroup('tok', 42)
    expect(result).toEqual({})
  })
})

describe('api – getLedger with cursor and limit', () => {
  beforeEach(() => mockFetch.mockReset())

  it('includes cursor and limit in query string', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ data: [], next_cursor: '', has_more: false }))
    await api.getLedger('tok', 'abc123', 25)
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('cursor=abc123')
    expect(String(url)).toContain('limit=25')
  })
})

describe('api – getLedger without cursor', () => {
  beforeEach(() => mockFetch.mockReset())

  it('does not include cursor= param', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ data: [], next_cursor: '', has_more: false }))
    await api.getLedger('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).not.toContain('cursor=')
  })
})

describe('api – POST with JSON body (submitPrediction)', () => {
  beforeEach(() => mockFetch.mockReset())

  it('uses method=POST and sends JSON body', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, match_id: 10, home_score: 2, away_score: 1 }))
    await api.submitPrediction('tok', { match_id: 10, home_score: 2, away_score: 1 })
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      match_id: 10, home_score: 2, away_score: 1,
    })
  })
})

describe('api – PATCH (updateMe)', () => {
  beforeEach(() => mockFetch.mockReset())

  it('uses method=PATCH', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, display_name: 'New Name' }))
    await api.updateMe('tok', { display_name: 'New Name' })
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('PATCH')
  })
})

describe('api – FormData upload (uploadKYCDocument)', () => {
  beforeEach(() => mockFetch.mockReset())

  it('sends FormData body without Content-Type in custom headers', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 5, status: 'pending' }))
    const fd = new FormData()
    fd.append('file', new Blob(['data'], { type: 'image/jpeg' }), 'doc.jpg')
    await api.uploadKYCDocument('tok', fd)
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).body).toBe(fd)
    // FormData requests must NOT set Content-Type manually (browser sets multipart boundary)
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Content-Type']).toBeUndefined()
  })
})

describe('api – createPaymentIntent', () => {
  beforeEach(() => mockFetch.mockReset())

  it('sends Idempotency-Key header', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 'pi_abc', status: 'created' }))
    await api.createPaymentIntent('tok', { amount_cents: 1000, currency: 'GTQ', provider: 'test' }, 'idem_key_xyz')
    const [, init] = mockFetch.mock.calls[0]
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Idempotency-Key']).toBe('idem_key_xyz')
  })
})

describe('api – adminGetKYCQueue with cursor + status', () => {
  beforeEach(() => mockFetch.mockReset())

  it('includes cursor and status in query string', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ data: [], next_cursor: '', has_more: false }))
    await api.adminGetKYCQueue('tok', 'cur_xyz', 'pending')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('cursor=cur_xyz')
    expect(String(url)).toContain('status=pending')
  })
})

// ── Additional API method coverage ───────────────────────────────────────────

describe('api – group methods', () => {
  beforeEach(() => mockFetch.mockReset())

  it('getMyGroups sends GET to /api/v1/groups/me', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getMyGroups('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/groups/me')
  })

  it('getGroup sends GET to /api/v1/groups/:id', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 5 }))
    await api.getGroup('tok', 5)
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/groups/5')
  })

  it('createGroup sends POST with name', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 7, name: 'Mi Grupo' }))
    await api.createGroup('tok', { name: 'Mi Grupo' })
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({ name: 'Mi Grupo' })
  })

  it('joinGroup sends POST to /api/v1/groups/join', async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await api.joinGroup('tok', 'INVITE123')
    const [url, init] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/groups/join')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('joinGroupWithBalance sends POST to /api/v1/groups/join-with-balance', async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await api.joinGroupWithBalance('tok', 'INVITE123')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/groups/join-with-balance')
  })

  it('leaveGroup sends DELETE', async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await api.leaveGroup('tok', 3)
    const [url, init] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/groups/3/members/me')
    expect((init as RequestInit).method).toBe('DELETE')
  })

  it('getGroupLeaderboard includes cursor param when provided', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ data: [], next_cursor: '', has_more: false }))
    await api.getGroupLeaderboard('tok', 1, 'cursor_abc')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('cursor=cursor_abc')
  })

  it('getGroupMembers sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getGroupMembers('tok', 1)
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/groups/1/members')
  })
})

describe('api – match and prediction methods', () => {
  beforeEach(() => mockFetch.mockReset())

  it('getMatches sends GET to /api/v1/matches', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getMatches('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/matches')
  })

  it('getMyPredictions sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getMyPredictions('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/predictions/me')
  })

  it('updatePrediction sends PATCH', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1 }))
    await api.updatePrediction('tok', 1, { home_score: 1, away_score: 0 })
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('PATCH')
  })
})

describe('api – KYC methods', () => {
  beforeEach(() => mockFetch.mockReset())

  it('getKYCRequirements sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ documents: [] }))
    await api.getKYCRequirements('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/kyc/requirements')
  })

  it('getKYCDocuments sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getKYCDocuments('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/kyc/documents')
  })

  it('getKYCEvents without cursor sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ events: [], next_cursor: '' }))
    await api.getKYCEvents('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/kyc/events')
    expect(String(url)).not.toContain('cursor')
  })

  it('getKYCEvents with cursor appends ?cursor=', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ events: [], next_cursor: '' }))
    await api.getKYCEvents('tok', 'cur_xyz')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('cursor=cur_xyz')
  })

  it('submitKYC sends POST', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, status: 'pending' }))
    await api.submitKYC('tok', {
      full_name: 'Test', date_of_birth: '1990-01-01', nationality: 'GT',
      document_type: 'passport', document_number: 'AB123',
      address_line: 'Calle 1', city: 'Guatemala', country: 'GT', postal_code: '01001',
    })
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
  })
})

describe('api – payment and withdrawal methods', () => {
  beforeEach(() => mockFetch.mockReset())

  it('uploadBankTransfer sends FormData with idempotency key', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, status: 'pending' }))
    const fd = new FormData()
    fd.append('receipt', new Blob(['img'], { type: 'image/jpeg' }), 'r.jpg')
    await api.uploadBankTransfer('tok', fd, 'idem_key_bt')
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).body).toBe(fd)
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Idempotency-Key']).toBe('idem_key_bt')
  })

  it('getMyBankTransfers sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getMyBankTransfers('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/bank-transfers')
  })

  it('createWithdrawal sends POST with idempotency key', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1 }))
    await api.createWithdrawal('tok', { amount_cents: 500, method: 'bank' }, 'idem_wdl')
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Idempotency-Key']).toBe('idem_wdl')
  })

  it('getMyWithdrawals sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getMyWithdrawals('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/withdrawals')
  })
})

describe('api – notification methods', () => {
  beforeEach(() => mockFetch.mockReset())

  it('getInbox sends GET with limit and offset', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ items: [], total: 0 }))
    await api.getInbox('tok', 20, 10)
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('limit=20')
    expect(String(url)).toContain('offset=10')
  })

  it('markRead sends POST with ids', async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await api.markRead('tok', [1, 2, 3])
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({ ids: [1, 2, 3] })
  })

  it('markAllRead sends POST with mark_all', async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
    await api.markAllRead('tok')
    const [, init] = mockFetch.mock.calls[0]
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({ mark_all: true })
  })

  it('getPreferences sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.getPreferences('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/notifications/preferences')
  })
})

describe('api – admin methods', () => {
  beforeEach(() => mockFetch.mockReset())

  it('adminGetStats sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ users: 0 }))
    await api.adminGetStats('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/stats')
  })

  it('adminApproveKYC sends POST', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, status: 'approved' }))
    await api.adminApproveKYC('tok', 42)
    const [url, init] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/profiles/42/approve')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('adminRejectKYC sends POST with reason', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, status: 'rejected' }))
    await api.adminRejectKYC('tok', 42, 'Docs expired')
    const [, init] = mockFetch.mock.calls[0]
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({ reason: 'Docs expired' })
  })

  it('adminGetExchangeRate sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ sell_rate: '7.80' }))
    await api.adminGetExchangeRate('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/exchange-rate/current')
  })

  it('adminGetExchangeRateHistory without cursor sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ data: [], next_cursor: '', has_more: false }))
    await api.adminGetExchangeRateHistory('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/exchange-rate/history')
    expect(String(url)).not.toContain('cursor=')
  })

  it('adminGetExchangeRateHistory with cursor appends ?cursor=', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ data: [], next_cursor: '', has_more: false }))
    await api.adminGetExchangeRateHistory('tok', 'cur_hist')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('cursor=cur_hist')
  })

  it('adminOverrideExchangeRate sends POST', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ sell_rate: '7.90' }))
    await api.adminOverrideExchangeRate('tok', { reference_rate: '7.85', reason: 'Manual' })
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
  })

  it('adminRefreshExchangeRate sends POST', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ sell_rate: '7.80' }))
    await api.adminRefreshExchangeRate('tok')
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
  })

  it('adminGetSSEStats sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ connections: 0 }))
    await api.adminGetSSEStats('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/notifications/sse/stats')
  })

  it('adminGetSystemParams sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.adminGetSystemParams('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/system-params')
  })

  it('adminGetScoringRules sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.adminGetScoringRules('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/scoring-rules')
  })

  it('adminGetCircuitBreakers sends GET', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]))
    await api.adminGetCircuitBreakers('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/observability/circuit-breakers')
  })

  it('adminGetKYCQueue without params sends GET with empty query string', async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ data: [], next_cursor: '', has_more: false }))
    await api.adminGetKYCQueue('tok')
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/api/v1/admin/kyc/queue')
  })
})

// ── serverAPI ─────────────────────────────────────────────────────────────────

describe('serverAPI()', () => {
  it('throws when BACKEND_INTERNAL_URL is unset', () => {
    const orig = process.env.BACKEND_INTERNAL_URL
    delete process.env.BACKEND_INTERNAL_URL
    expect(() => serverAPI()).toThrow('BACKEND_INTERNAL_URL is not set')
    process.env.BACKEND_INTERNAL_URL = orig
  })

  it('returns a client when BACKEND_INTERNAL_URL is set', () => {
    process.env.BACKEND_INTERNAL_URL = 'http://backend:8080'
    const client = serverAPI()
    expect(client).toBeDefined()
    expect(typeof client.getExchangeRate).toBe('function')
  })
})

// ── getServerToken ────────────────────────────────────────────────────────────

describe('getServerToken()', () => {
  it('returns token from Clerk auth()', async () => {
    const mockGetToken = vi.fn().mockResolvedValue('server_tok_abc')
    vi.mocked(auth).mockResolvedValue({ getToken: mockGetToken } as never)
    const token = await getServerToken()
    expect(token).toBe('server_tok_abc')
  })

  it('returns null when Clerk returns null', async () => {
    const mockGetToken = vi.fn().mockResolvedValue(null)
    vi.mocked(auth).mockResolvedValue({ getToken: mockGetToken } as never)
    const token = await getServerToken()
    expect(token).toBeNull()
  })
})
