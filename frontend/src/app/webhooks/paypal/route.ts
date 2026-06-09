// Webhook relay — forwards PayPal webhook events to the Go backend unchanged.
// PayPal RSA signature headers (PAYPAL-TRANSMISSION-ID, PAYPAL-TRANSMISSION-TIME,
// PAYPAL-CERT-URL, PAYPAL-TRANSMISSION-SIG, PAYPAL-AUTH-ALGO) must reach the
// backend intact for certificate verification; we must NOT add an Authorization
// header and must NOT buffer-transform the body.
import { NextRequest, NextResponse } from 'next/server'

const BACKEND = process.env.BACKEND_INTERNAL_URL!

const HOP_HEADERS = new Set([
  'content-encoding',
  'transfer-encoding',
  'connection',
  'keep-alive',
])

export async function POST(req: NextRequest): Promise<NextResponse> {
  const url = `${BACKEND}/webhooks/paypal`

  const headers: Record<string, string> = {}
  req.headers.forEach((v, k) => {
    if (!HOP_HEADERS.has(k.toLowerCase())) {
      headers[k] = v
    }
  })

  const body = await req.arrayBuffer()

  let upstream: Response
  try {
    upstream = await fetch(url, { method: 'POST', headers, body, cache: 'no-store' })
  } catch (err) {
    console.error('[paypal webhook relay] upstream fetch failed', err)
    return NextResponse.json({ error: 'Backend unavailable' }, { status: 502 })
  }

  const resHeaders = new Headers()
  upstream.headers.forEach((v, k) => {
    if (!HOP_HEADERS.has(k.toLowerCase())) {
      resHeaders.set(k, v)
    }
  })

  return new NextResponse(upstream.body, {
    status: upstream.status,
    headers: resHeaders,
  })
}
