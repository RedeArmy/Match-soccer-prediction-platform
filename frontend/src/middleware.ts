import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
import type { ClerkMiddlewareAuth } from "@clerk/nextjs/server";
import { NextRequest, NextResponse } from "next/server";

const isPublicRoute = createRouteMatcher([
  "/",
  "/sign-in(.*)",
  "/sign-up(.*)",
  "/tournaments(.*)",
  "/api/health",
  // Exchange rate public — proxied from /api/exchange-rate (backend) via BFF
  "/api/exchange-rate(.*)",
  // BFF catch-all proxy — auth is enforced by the Go backend, not here.
  // Clerk's dev-mode protect-rewrite would return 404 for API calls made
  // before the dev-browser cookie syncs; letting the BFF handle auth avoids
  // that race and keeps authentication logic in one place (the backend).
  "/api/v1/(.*)",
  // Public group leaderboard preview (no auth required — backend enforces limits).
  "/api/public/(.*)",
  // Live match feed proxied to api-football.com (no user session needed).
  "/api/live/(.*)",
  // Clerk webhook relay — authenticated by Svix HMAC, not by session
  "/webhooks/(.*)",
]);

// generateNonce returns a 128-bit cryptographically random base64 string.
// Using crypto.getRandomValues (available in Edge runtime) rather than
// Math.random so the nonce cannot be predicted or brute-forced.
function generateNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return btoa(String.fromCodePoint(...bytes));
}

// clerkFrontendApiOrigin derives the Clerk Frontend API origin from the
// publishable key. Clerk encodes the hostname in the third segment of the key
// as base64 with a trailing '$' separator, e.g.
//   pk_live_Y2xlcmsua2luaWVsYS51ayQ → atob(...) → 'clerk.kiniela.uk$'
// The derived origin must appear in both script-src (the SDK is served from
// this domain) and connect-src (all Clerk auth API calls go here).
function clerkFrontendApiOrigin(): string {
  const key = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY ?? "";
  const segment = key.split("_")[2] ?? "";
  if (!segment) return "";
  try {
    return `https://${atob(segment).replace("$", "")}`;
  } catch {
    return "";
  }
}

// buildCSP constructs the Content-Security-Policy header value for a given nonce.
//
// script-src strategy:
//   'nonce-{nonce}' + 'strict-dynamic' — the root layout passes the nonce to
//   ClerkProvider and Next.js auto-attaches it to its inline bootstrap scripts
//   via the x-nonce request header. 'strict-dynamic' trusts scripts dynamically
//   loaded by those nonce-carrying scripts (Next.js chunks, Clerk sub-resources).
//   'unsafe-inline' is a fallback for pre-CSP3 browsers; CSP3 browsers ignore
//   it when 'strict-dynamic' is present.
//
// style-src retains 'unsafe-inline' because the root layout injects a
// <style> block for CSS custom properties that cannot carry a nonce.
function buildCSP(nonce: string): string {
  const clerkApi = clerkFrontendApiOrigin();
  const clerkApiEntry = clerkApi ? ` ${clerkApi}` : "";
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic' 'unsafe-inline' 'unsafe-eval' https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev https://*.paypal.com https://www.paypalobjects.com${clerkApiEntry}`,
    "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
    "img-src 'self' data: blob: https:",
    "font-src 'self' https://fonts.gstatic.com",
    `connect-src 'self' https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev https://clerk-telemetry.com https://*.paypal.com https://*.sandbox.paypal.com https://www.sandbox.paypal.com${clerkApiEntry}`,
    `frame-src https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev https://*.paypal.com https://*.sandbox.paypal.com https://www.sandbox.paypal.com https://challenges.cloudflare.com${clerkApiEntry}`,
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
}

// isDashboardEntry returns true only for the exact /dashboard path so that
// the role-check fetch does not fire on every sub-route.
const isDashboardEntry = createRouteMatcher(["/dashboard"]);

// resolveDashboardRedirect fetches /users/me and returns:
//   - a 307 redirect to /admin/dashboard for admin users
//   - a NextResponse.next with x-user-role header for regular users
//   - null when the token is absent or the backend is unreachable
async function resolveDashboardRedirect(
  auth: ClerkMiddlewareAuth,
  req: NextRequest,
): Promise<NextResponse | null> {
  const { getToken } = await auth();
  const token = await getToken();
  if (!token) return null;

  const backendUrl = process.env.BACKEND_INTERNAL_URL;
  if (!backendUrl) return null;

  try {
    const meRes = await fetch(`${backendUrl}/api/v1/users/me`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!meRes.ok) return null;

    const me = (await meRes.json()) as { role?: string };
    if (me.role === "admin") {
      return NextResponse.redirect(new URL("/admin/dashboard", req.url));
    }
    // Pass role to DashboardPage via a request header so it can skip
    // its own /users/me fetch.
    const reqHeaders = new Headers(req.headers);
    reqHeaders.set("x-user-role", me.role ?? "user");
    return NextResponse.next({ request: { headers: reqHeaders } });
  } catch {
    // Backend unreachable — return null; DashboardPage handles the fallback.
    return null;
  }
}

export default clerkMiddleware(
  async (auth, req: NextRequest) => {
    if (!isPublicRoute(req)) {
      await auth.protect();
    }

    // Fast admin redirect — runs before any page renders so the browser never
    // loads /dashboard for admin users. The backend /users/me call is cheap
    // (same-network, no DB query beyond a primary-key lookup).
    if (isDashboardEntry(req)) {
      const redirect = await resolveDashboardRedirect(auth, req);
      if (redirect) return redirect;
    }

    // CSP with per-request nonce is enforced only in production.
    // In development (Turbopack), the edge runtime does not hot-reload middleware
    // modules, so stale compiled code would serve the wrong CSP regardless of
    // file changes. Turbopack also requires 'unsafe-eval' for HMR, which defeats
    // the purpose of a strict script-src policy. Development therefore runs
    // without CSP so third-party SDKs (PayPal, Clerk) can load unrestricted.
    if (process.env.NODE_ENV !== "production") {
      const devRes = NextResponse.next();
      devRes.headers.set("X-Dev-Middleware", "v2-no-csp");
      return devRes;
    }

    const nonce = generateNonce();
    const csp = buildCSP(nonce);

    const requestHeaders = new Headers(req.headers);
    requestHeaders.set("x-nonce", nonce);

    const response = NextResponse.next({
      request: { headers: requestHeaders },
    });
    response.headers.set("Content-Security-Policy", csp);
    return response;
  },
  { clockSkewInMs: 10_000 },
);

export const config = {
  matcher: [
    // Skip Next.js internals and static files.
    // Written as a plain string literal (not String.raw) because Next.js
    // parses this config statically and does not evaluate JS expressions.
    "/((?!_next|[^?]*\\.(?:html?|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)", // NOSONAR
    // Always run for API routes
    "/(api|trpc)(.*)",
  ],
};
