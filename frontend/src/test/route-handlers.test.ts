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
import { NextRequest, NextResponse } from 'next/server'

// ── Fetch mock ────────────────────────────────────────────────────────────────

const mockFetch = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', mockFetch)

// Use the mocked NextRequest so .nextUrl is available
function makeReq(url = 'http://localhost/api/v1/matches', init?: RequestInit): NextRequest {
  return new NextRequest(url, init)
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

describe('[...path] proxy – GET with Clerk token', () => {
  beforeEach(() => {
    mockFetch.mockReset()
    vi.mocked(auth).mockResolvedValue({ getToken: vi.fn().mockResolvedValue('clerk_token_abc') } as never)
  })

  it('forwards Authorization: Bearer header', async () => {
    mockFetch.mockResolvedValueOnce(new Response('[]', { status: 200 }))
    const { GET } = await import('@/app/api/[...path]/route')
    await GET(makeReq(), makeCtx())
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

// ── notifications/stream/route ────────────────────────────────────────────────

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
