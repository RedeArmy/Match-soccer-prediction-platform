// @vitest-environment node
import { describe, it, expect, vi, beforeEach } from 'vitest'

// Hoist Clerk mock
vi.mock('@clerk/nextjs/server', () => ({ auth: vi.fn() }))

// Mock next/server so NextResponse/NextRequest work in node env without full Next.js runtime
vi.mock('next/server', () => {
  class MockNextResponse extends Response {
    static json(data: unknown, init?: ResponseInit) {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        ...(init?.headers != null ? (init.headers as Record<string, string>) : {}),
      }
      return new MockNextResponse(JSON.stringify(data), { ...init, headers })
    }
  }

  class MockNextRequest extends Request {
    nextUrl: URL
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(input, init)
      this.nextUrl = new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : (input as Request).url)
    }
  }

  return {
    NextResponse: MockNextResponse,
    NextRequest:  MockNextRequest,
  }
})

import { auth } from '@clerk/nextjs/server'
import { NextRequest } from 'next/server'

// ── Fetch mock ────────────────────────────────────────────────────────────────

const mockFetch = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', mockFetch)

// Use the mocked NextRequest so .nextUrl is available
function makeReq(url = 'http://localhost/api/v1/matches', init?: RequestInit): NextRequest {
  return new NextRequest(url, init as ConstructorParameters<typeof NextRequest>[1])
}

// ── health/route ──────────────────────────────────────────────────────────────

describe('GET /api/health', () => {
  it('returns { status: ok, service: frontend }', async () => {
    const { GET } = await import('@/app/api/health/route')
    const res = await GET()
    const body = await res.json()
    expect(body).toEqual({ status: 'ok', service: 'frontend' })
  })
})

// ── [...path]/route (BFF proxy) ───────────────────────────────────────────────

type Context = { params: Promise<{ path: string[] }> }

function makeCtx(path: string[] = ['v1', 'matches']): Context {
  return { params: Promise.resolve({ path }) }
}

describe('[...path] proxy – GET without Clerk token', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue(null) } as never)
  })

  it('calls upstream fetch without Authorization header', async () => {
    mockFetch.mockResolvedValueOnce(new Response('[]', { status: 200 }))
    const { GET } = await import('@/app/api/[...path]/route')
    await GET(makeReq(), makeCtx())
    const [, init] = mockFetch.mock.calls[0]
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Authorization']).toBeUndefined()
  })
})

describe('[...path] proxy – GET with client Authorization header', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })

  it('forwards Authorization header from the incoming request', async () => {
    mockFetch.mockResolvedValueOnce(new Response('[]', { status: 200 }))
    const { GET } = await import('@/app/api/[...path]/route')
    const req = makeReq('http://localhost/api/v1/matches', {
      headers: { Authorization: 'Bearer clerk_token_abc' },
    })
    await GET(req, makeCtx())
    const [, init] = mockFetch.mock.calls[0]
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Authorization']).toBe('Bearer clerk_token_abc')
  })
})

describe('[...path] proxy – POST request', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue(null) } as never)
  })

  it('calls upstream with body from request.arrayBuffer()', async () => {
    mockFetch.mockResolvedValueOnce(new Response('{}', { status: 201 }))
    const { POST } = await import('@/app/api/[...path]/route')
    const bodyStr = JSON.stringify({ home_score: 1, away_score: 2 })
    const req = makeReq('http://localhost/api/v1/predictions', {
      method: 'POST',
      body: bodyStr,
      headers: { 'Content-Type': 'application/json' },
    })
    await POST(req, makeCtx(['v1', 'predictions']))
    const [, init] = mockFetch.mock.calls[0]
    expect((init as RequestInit).method).toBe('POST')
    expect((init as RequestInit).body).toBeDefined()
  })
})

describe('[...path] proxy – upstream fetch throws', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue(null) } as never)
  })

  it('returns 502 with ERR_UPSTREAM code', async () => {
    mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    const { GET } = await import('@/app/api/[...path]/route')
    const res = await GET(makeReq(), makeCtx())
    expect(res.status).toBe(502)
    const body = await res.json()
    expect(body.error.code).toBe('ERR_UPSTREAM')
  })
})

describe('[...path] proxy – forwards Idempotency-Key header', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue(null) } as never)
  })

  it('forwards Idempotency-Key from incoming request to upstream', async () => {
    mockFetch.mockResolvedValueOnce(new Response('{}', { status: 201 }))
    const { POST } = await import('@/app/api/[...path]/route')
    const req = makeReq('http://localhost/api/v1/withdrawals', {
      method: 'POST',
      body: '{}',
      headers: { 'Idempotency-Key': 'idem_abc_123' },
    })
    await POST(req, makeCtx(['v1', 'withdrawals']))
    const [, init] = mockFetch.mock.calls[0]
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['Idempotency-Key']).toBe('idem_abc_123')
  })
})

describe('[...path] proxy – upstream non-OK response is proxied', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue(null) } as never)
  })

  it('returns the upstream status code when upstream responds with 4xx', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'not found' } }), {
        status: 404,
        headers: { 'content-type': 'application/json' },
      }),
    )
    const { GET } = await import('@/app/api/[...path]/route')
    const res = await GET(makeReq(), makeCtx())
    expect(res.status).toBe(404)
  })
})

describe('[...path] proxy – hop-by-hop header stripping', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue(null) } as never)
  })

  it('strips connection header from proxied response', async () => {
    const upstream = new Response('{}', {
      status: 200,
      headers: { 'connection': 'keep-alive', 'x-custom': 'foo' },
    })
    mockFetch.mockResolvedValueOnce(upstream)
    const { GET } = await import('@/app/api/[...path]/route')
    const res = await GET(makeReq(), makeCtx())
    expect(res.headers.get('connection')).toBeNull()
  })

  it('forwards non-hop-by-hop headers', async () => {
    const upstream = new Response('{}', {
      status: 200,
      headers: { 'connection': 'keep-alive', 'x-custom': 'foo' },
    })
    mockFetch.mockResolvedValueOnce(upstream)
    const { GET } = await import('@/app/api/[...path]/route')
    const res = await GET(makeReq(), makeCtx())
    expect(res.headers.get('x-custom')).toBe('foo')
  })
})

// ── webhooks/clerk/route ──────────────────────────────────────────────────────

describe('clerk webhook relay – happy path', () => {
  beforeEach(() => mockFetch.mockReset())

  it('forwards request body to backend and returns upstream status', async () => {
    mockFetch.mockResolvedValueOnce(new Response('{}', { status: 200 }))
    const { POST } = await import('@/app/webhooks/clerk/route')
    const req = makeReq('http://localhost/webhooks/clerk', {
      method: 'POST',
      body: JSON.stringify({ type: 'user.created' }),
      headers: {
        'content-type': 'application/json',
        'svix-id': 'msg_abc',
        'svix-timestamp': '1234567890',
        'svix-signature': 'v1,abc123',
      },
    })
    const res = await POST(req)
    expect(res.status).toBe(200)
    expect(mockFetch).toHaveBeenCalledOnce()
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/webhooks/clerk')
  })

  it('strips hop-by-hop headers from the upstream response', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response('{}', {
        status: 200,
        headers: { 'transfer-encoding': 'chunked', 'x-request-id': 'abc' },
      }),
    )
    const { POST } = await import('@/app/webhooks/clerk/route')
    const req = makeReq('http://localhost/webhooks/clerk', { method: 'POST', body: '{}' })
    const res = await POST(req)
    expect(res.headers.get('transfer-encoding')).toBeNull()
    expect(res.headers.get('x-request-id')).toBe('abc')
  })
})

describe('clerk webhook relay – hop-by-hop header stripping in request', () => {
  beforeEach(() => mockFetch.mockReset())

  it('strips hop-by-hop headers from forwarded request', async () => {
    mockFetch.mockResolvedValueOnce(new Response('{}', { status: 200 }))
    const { POST } = await import('@/app/webhooks/clerk/route')
    const req = makeReq('http://localhost/webhooks/clerk', {
      method: 'POST',
      body: '{}',
      headers: {
        'connection': 'keep-alive',
        'x-custom-header': 'preserved',
      },
    })
    await POST(req)
    const [, init] = mockFetch.mock.calls[0]
    const headers = (init as RequestInit).headers as Record<string, string>
    expect(headers['connection']).toBeUndefined()
    expect(headers['x-custom-header']).toBe('preserved')
  })
})

describe('clerk webhook relay – upstream fetch throws', () => {
  beforeEach(() => mockFetch.mockReset())

  it('returns 502 when backend is unreachable', async () => {
    mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    const { POST } = await import('@/app/webhooks/clerk/route')
    const req = makeReq('http://localhost/webhooks/clerk', { method: 'POST', body: '{}' })
    const res = await POST(req)
    expect(res.status).toBe(502)
    const body = await res.json()
    expect(body.error).toBe('Backend unavailable')
  })
})

// ── notifications/stream/route ────────────────────────────────────────────────

// ── webhooks/paypal/route ─────────────────────────────────────────────────────

describe('paypal webhook relay – happy path', () => {
  beforeEach(() => mockFetch.mockReset())

  it('forwards request body to backend and returns upstream status', async () => {
    process.env.BACKEND_INTERNAL_URL = 'http://backend:8080'
    mockFetch.mockResolvedValueOnce(new Response('{}', { status: 200 }))
    const { POST } = await import('@/app/webhooks/paypal/route')
    const req = makeReq('http://localhost/webhooks/paypal', {
      method: 'POST',
      body: JSON.stringify({ event_type: 'PAYMENT.CAPTURE.COMPLETED' }),
      headers: {
        'content-type': 'application/json',
        'paypal-transmission-id': 'txn_abc',
      },
    })
    const res = await POST(req)
    expect(res.status).toBe(200)
    expect(mockFetch).toHaveBeenCalledOnce()
    const [url] = mockFetch.mock.calls[0]
    expect(String(url)).toContain('/webhooks/paypal')
  })
})

describe('paypal webhook relay – strips hop-by-hop headers', () => {
  beforeEach(() => mockFetch.mockReset())

  it('strips connection header from proxied response', async () => {
    process.env.BACKEND_INTERNAL_URL = 'http://backend:8080'
    mockFetch.mockResolvedValueOnce(
      new Response('{}', {
        status: 200,
        headers: { 'connection': 'keep-alive', 'x-request-id': 'req_1' },
      }),
    )
    const { POST } = await import('@/app/webhooks/paypal/route')
    const req = makeReq('http://localhost/webhooks/paypal', { method: 'POST', body: '{}' })
    const res = await POST(req)
    expect(res.headers.get('connection')).toBeNull()
    expect(res.headers.get('x-request-id')).toBe('req_1')
  })
})

describe('paypal webhook relay – upstream fetch throws', () => {
  beforeEach(() => mockFetch.mockReset())

  it('returns 502 when backend is unreachable', async () => {
    process.env.BACKEND_INTERNAL_URL = 'http://backend:8080'
    mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    const { POST } = await import('@/app/webhooks/paypal/route')
    const req = makeReq('http://localhost/webhooks/paypal', { method: 'POST', body: '{}' })
    const res = await POST(req)
    expect(res.status).toBe(502)
    const body = await res.json()
    expect(body.error).toBe('Backend unavailable')
  })
})

describe('SSE stream proxy – no token', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue(null) } as never)
  })

  it('returns 401', async () => {
    const { GET } = await import('@/app/api/notifications/stream/route')
    const res = await GET()
    expect(res.status).toBe(401)
    const body = await res.json()
    expect(body.error.code).toBe('ERR_UNAUTHORISED')
  })
})

describe('SSE stream proxy – upstream throws', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue('tok_sse') } as never)
  })

  it('returns 502', async () => {
    mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    const { GET } = await import('@/app/api/notifications/stream/route')
    const res = await GET()
    expect(res.status).toBe(502)
    const body = await res.json()
    expect(body.error.code).toBe('ERR_UPSTREAM')
  })
})

describe('SSE stream proxy – upstream responds', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue('tok_sse') } as never)
  })

  it('proxies body with content-type: text/event-stream', async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('data: {}\n\n'))
        controller.close()
      },
    })
    mockFetch.mockResolvedValueOnce(new Response(stream, { status: 200 }))
    const { GET } = await import('@/app/api/notifications/stream/route')
    const res = await GET()
    expect(res.status).toBe(200)
    expect(res.headers.get('content-type')).toContain('text/event-stream')
  })
})

// ── live/today/route ──────────────────────────────────────────────────────────

describe('GET /api/live/today – no API key', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    delete process.env.FOOTBALL_API_KEY
  })

  it('returns { fixtures: [] } gracefully when key is absent', async () => {
    const { GET } = await import('@/app/api/live/today/route')
    const res = await GET()
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.fixtures).toEqual([])
  })
})

describe('GET /api/live/today – upstream OK', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_live_today'
  })

  it('maps api-football response to TodayFixture array', async () => {
    const afPayload = {
      results: 1,
      errors:  [],
      response: [
        {
          fixture: { id: 42, date: '2026-06-10T20:00:00Z', status: { short: '1H', elapsed: 30 }, venue: { name: 'Estadio X', city: null } },
          league:  { round: 'Group Stage - 1' },
          teams:   { home: { id: 1, name: 'Brazil', logo: 'https://logo/br.png' }, away: { id: 2, name: 'Germany', logo: 'https://logo/de.png' } },
          goals:   { home: 1, away: 0 },
        },
      ],
    }
    mockFetch.mockResolvedValueOnce(new Response(JSON.stringify(afPayload), { status: 200 }))
    const { GET } = await import('@/app/api/live/today/route')
    const res = await GET()
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.fixtures).toHaveLength(1)
    expect(body.fixtures[0].id).toBe(42)
    expect(body.fixtures[0].homeTeam).toBe('Brazil')
    expect(body.fixtures[0].status).toBe('1H')
    expect(body.fixtures[0].elapsed).toBe(30)
    expect(body.fixtures[0].homeScore).toBe(1)
    expect(body.fixtures[0].venue).toBe('Estadio X')
  })
})

describe('GET /api/live/today – upstream fetch throws', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_live_today'
  })

  it('returns { fixtures: [] } on network error', async () => {
    mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    const { GET } = await import('@/app/api/live/today/route')
    const res = await GET()
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.fixtures).toEqual([])
  })
})

describe('GET /api/live/today – upstream non-OK', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_live_today'
  })

  it('returns { fixtures: [] } on upstream 429', async () => {
    mockFetch.mockResolvedValueOnce(new Response('', { status: 429 }))
    const { GET } = await import('@/app/api/live/today/route')
    const res = await GET()
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.fixtures).toEqual([])
  })
})

describe('GET /api/live/today – empty response array', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_live_today'
  })

  it('returns { fixtures: [] } when no matches scheduled today', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ results: 0, errors: [], response: [] }), { status: 200 }),
    )
    const { GET } = await import('@/app/api/live/today/route')
    const res = await GET()
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.fixtures).toEqual([])
  })
})

// ── live/fixture/[id]/route ───────────────────────────────────────────────────

type FixtureContext = { params: Promise<{ id: string }> }
function makeFixtureCtx(id: string): FixtureContext {
  return { params: Promise.resolve({ id }) }
}

describe('GET /api/live/fixture/[id] – no API key', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    delete process.env.FOOTBALL_API_KEY
  })

  it('returns 503 when FOOTBALL_API_KEY is absent', async () => {
    const { GET } = await import('@/app/api/live/fixture/[id]/route')
    const res = await GET(makeReq('http://localhost/api/live/fixture/42'), makeFixtureCtx('42'))
    expect(res.status).toBe(503)
  })
})

describe('GET /api/live/fixture/[id] – invalid id', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_fixture'
  })

  it('returns 400 for non-numeric id', async () => {
    const { GET } = await import('@/app/api/live/fixture/[id]/route')
    const res = await GET(makeReq('http://localhost/api/live/fixture/abc'), makeFixtureCtx('abc'))
    expect(res.status).toBe(400)
  })

  it('returns 400 for zero id', async () => {
    const { GET } = await import('@/app/api/live/fixture/[id]/route')
    const res = await GET(makeReq('http://localhost/api/live/fixture/0'), makeFixtureCtx('0'))
    expect(res.status).toBe(400)
  })
})

describe('GET /api/live/fixture/[id] – upstream OK', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_fixture'
  })

  it('returns mapped FixtureDetail on success', async () => {
    const afPayload = {
      response: [
        {
          fixture: { id: 42, date: '2026-06-10T20:00:00Z', status: { short: '1H', elapsed: 35 }, venue: { name: 'AT&T Stadium', city: 'Arlington' } },
          league:  { round: 'Group Stage - 1' },
          teams:   {
            home: { name: 'Brazil',  logo: 'https://logo/br.png' },
            away: { name: 'Germany', logo: 'https://logo/de.png' },
          },
          goals: { home: 1, away: 0 },
          score: { halftime: { home: null, away: null } },
          lineups: [
            {
              team: { name: 'Brazil' },
              formation: '4-3-3',
              startXI: [{ player: { id: 1, name: 'Alisson', number: 1, pos: 'G' } }],
              substitutes: [],
            },
          ],
          events: [
            {
              time:   { elapsed: 22, extra: null },
              team:   { name: 'Brazil' },
              player: { name: 'Vinicius Jr.' },
              assist: { name: 'Rodrygo' },
              type:   'Goal',
              detail: 'Normal Goal',
            },
          ],
        },
      ],
    }
    mockFetch.mockResolvedValueOnce(new Response(JSON.stringify(afPayload), { status: 200 }))
    const { GET } = await import('@/app/api/live/fixture/[id]/route')
    const res = await GET(makeReq('http://localhost/api/live/fixture/42'), makeFixtureCtx('42'))
    expect(res.status).toBe(200)
    const body = await res.json()
    expect(body.fixture.id).toBe(42)
    expect(body.fixture.homeTeam).toBe('Brazil')
    expect(body.fixture.lineups).toHaveLength(1)
    expect(body.fixture.lineups[0].formation).toBe('4-3-3')
    expect(body.fixture.events).toHaveLength(1)
    expect(body.fixture.events[0].player).toBe('Vinicius Jr.')
    expect(body.fixture.events[0].assist).toBe('Rodrygo')
  })
})

describe('GET /api/live/fixture/[id] – upstream fetch throws', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_fixture'
  })

  it('returns 502 on network error', async () => {
    mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    const { GET } = await import('@/app/api/live/fixture/[id]/route')
    const res = await GET(makeReq('http://localhost/api/live/fixture/99'), makeFixtureCtx('99'))
    expect(res.status).toBe(502)
  })
})

describe('GET /api/live/fixture/[id] – upstream non-OK', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_fixture'
  })

  it('proxies upstream status code on error', async () => {
    mockFetch.mockResolvedValueOnce(new Response('', { status: 503 }))
    const { GET } = await import('@/app/api/live/fixture/[id]/route')
    const res = await GET(makeReq('http://localhost/api/live/fixture/5'), makeFixtureCtx('5'))
    expect(res.status).toBe(503)
  })
})

describe('GET /api/live/fixture/[id] – empty response array', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    process.env.FOOTBALL_API_KEY = 'test_key_fixture'
  })

  it('returns 404 when api-football returns no items', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ response: [] }), { status: 200 }),
    )
    const { GET } = await import('@/app/api/live/fixture/[id]/route')
    const res = await GET(makeReq('http://localhost/api/live/fixture/1'), makeFixtureCtx('1'))
    expect(res.status).toBe(404)
  })
})
