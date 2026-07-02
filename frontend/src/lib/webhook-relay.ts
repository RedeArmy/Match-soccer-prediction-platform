import { NextRequest, NextResponse } from "next/server";

const HOP_HEADERS = new Set([
  "content-encoding",
  "transfer-encoding",
  "connection",
  "keep-alive",
]);

// Always forwarded — the backend needs the original media type to parse the
// raw body, and this value is never attacker-influenced in a way the
// signature check downstream doesn't already cover.
const BASE_FORWARD_HEADERS = ["content-type"];

/**
 * relayWebhook forwards a POST body to the Go backend for signature
 * verification and processing.
 *
 * Only `signatureHeaders` (the specific provider headers the backend's
 * HMAC/RSA verification middleware reads — e.g. svix-* or PAYPAL-*) plus
 * content-type are forwarded, rather than every header the client sent.
 * The caller is Next.js middleware (frontend/src/middleware.ts) sitting in
 * front of this route, not an arbitrary browser, but forwarding the full
 * header set unfiltered would still let a caller inject headers the backend
 * was never designed to see on this path (Host, Cookie, Authorization,
 * X-Forwarded-*) for no functional benefit — the explicit allowlist keeps
 * this relay a narrow pass-through of exactly what signature verification
 * requires.
 */
export async function relayWebhook(
  req: NextRequest,
  backendPath: string,
  logTag: string,
  signatureHeaders: string[],
): Promise<NextResponse> {
  const url = `${process.env.BACKEND_INTERNAL_URL!}${backendPath}`;

  const allowed = new Set(
    [...BASE_FORWARD_HEADERS, ...signatureHeaders].map((h) => h.toLowerCase()),
  );
  const headers: Record<string, string> = {};
  req.headers.forEach((v, k) => {
    if (allowed.has(k.toLowerCase())) {
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
    console.error(`[${logTag}] upstream fetch failed`, err);
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
