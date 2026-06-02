// BFF transparent proxy — all browser API requests route through here.
// The Go backend URL is never sent to the browser.
// Clerk JWT is injected server-side before forwarding.
import { auth } from '@clerk/nextjs/server'
import { NextRequest, NextResponse } from 'next/server'

const BACKEND = process.env.BACKEND_INTERNAL_URL!

// Hop-by-hop headers that must NOT be forwarded to the client
const HOP_HEADERS = new Set([
  'content-encoding',
  'transfer-encoding',
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'te',
  'trailers',
  'upgrade',
])

type Context = { params: Promise<{ path: string[] }> }

async function proxy(req: NextRequest, ctx: Context, method: string): Promise<NextResponse> {
  const { path } = await ctx.params
  const { getToken } = await auth()
  const token = await getToken()

  const upstreamPath = path.join('/')
  const url = `${BACKEND}/${upstreamPath}${req.nextUrl.search}`

  const headers: Record<string, string> = {
    'X-Request-ID':    req.headers.get('x-request-id') ?? crypto.randomUUID(),
    'X-Forwarded-For': req.headers.get('x-forwarded-for') ?? '',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  // Forward Content-Type only when the request has a body
  const ct = req.headers.get('content-type')
  if (ct) headers['Content-Type'] = ct

  const body = ['GET', 'HEAD'].includes(method)
    ? undefined
    : await req.arrayBuffer()

  let upstream: Response
  try {
    upstream = await fetch(url, { method, headers, body, cache: 'no-store' })
  } catch (err) {
    console.error('[BFF proxy] upstream fetch failed', { url, err })
    return NextResponse.json(
      { error: { schema_version: 1, code: 'ERR_UPSTREAM', message: 'Backend unavailable' } },
      { status: 502 },
    )
  }

  const resHeaders = new Headers()
  upstream.headers.forEach((v, k) => {
    if (!HOP_HEADERS.has(k.toLowerCase())) {
      resHeaders.set(k, v)
    }
  })

  return new NextResponse(upstream.body, {
    status:  upstream.status,
    headers: resHeaders,
  })
}

export const GET    = (req: NextRequest, ctx: Context) => proxy(req, ctx, 'GET')
export const POST   = (req: NextRequest, ctx: Context) => proxy(req, ctx, 'POST')
export const PUT    = (req: NextRequest, ctx: Context) => proxy(req, ctx, 'PUT')
export const PATCH  = (req: NextRequest, ctx: Context) => proxy(req, ctx, 'PATCH')
export const DELETE = (req: NextRequest, ctx: Context) => proxy(req, ctx, 'DELETE')
