// Webhook relay — forwards Clerk webhook events to the Go backend unchanged.
// Svix signature headers (svix-id, svix-timestamp, svix-signature) must reach
// the backend intact for HMAC verification; we must NOT add an Authorization
// header and must NOT buffer-transform the body.
import { NextRequest, NextResponse } from "next/server";

const BACKEND = process.env.BACKEND_INTERNAL_URL!;

const HOP_HEADERS = new Set([
  "content-encoding",
  "transfer-encoding",
  "connection",
  "keep-alive",
]);

export async function POST(req: NextRequest): Promise<NextResponse> {
  const url = `${BACKEND}/webhooks/clerk`;

  const headers: Record<string, string> = {};
  req.headers.forEach((v, k) => {
    if (!HOP_HEADERS.has(k.toLowerCase())) {
      headers[k] = v;
    }
  });

  const body = await req.arrayBuffer();

  let upstream: Response;
  try {
    upstream = await fetch(url, {
      method: "POST",
      headers,
      body,
      cache: "no-store",
    });
  } catch (err) {
    console.error("[webhook relay] upstream fetch failed", err);
    return NextResponse.json({ error: "Backend unavailable" }, { status: 502 });
  }

  const resHeaders = new Headers();
  upstream.headers.forEach((v, k) => {
    if (!HOP_HEADERS.has(k.toLowerCase())) {
      resHeaders.set(k, v);
    }
  });

  return new NextResponse(upstream.body, {
    status: upstream.status,
    headers: resHeaders,
  });
}
