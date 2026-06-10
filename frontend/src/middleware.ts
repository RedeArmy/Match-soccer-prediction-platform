import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
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

// buildCSP constructs the Content-Security-Policy header value for a given nonce.
//
// script-src strategy:
//   'nonce-{nonce}' + 'strict-dynamic' — modern browsers (CSP Level 3+) require
//   every inline script to carry the nonce. 'strict-dynamic' additionally trusts
//   scripts loaded by already-trusted scripts, so Clerk and Next.js can load
//   their own sub-resources without being whitelisted individually.
//   'unsafe-inline' — kept as fallback for browsers that do not support
//   'strict-dynamic'. In CSP Level 3+ browsers 'unsafe-inline' is silently
//   ignored when 'strict-dynamic' is present, so it provides no weakening there.
//
// style-src retains 'unsafe-inline' because the root layout injects a
// <style> block for CSS custom properties that cannot carry a nonce through
// next.config.ts-level headers. Style injection attacks are significantly
// lower-risk than script injection on a sports pool app.
function buildCSP(nonce: string): string {
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic' 'unsafe-inline' 'unsafe-eval' https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev https://*.paypal.com https://www.paypalobjects.com`,
    "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
    "img-src 'self' data: blob: https:",
    "font-src 'self' https://fonts.gstatic.com",
    "connect-src 'self' https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev https://clerk-telemetry.com https://*.paypal.com https://*.sandbox.paypal.com https://www.sandbox.paypal.com",
    "frame-src https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev https://*.paypal.com https://*.sandbox.paypal.com https://www.sandbox.paypal.com",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
}

export default clerkMiddleware(async (auth, req: NextRequest) => {
  if (!isPublicRoute(req)) {
    await auth.protect();
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

  const response = NextResponse.next({ request: { headers: requestHeaders } });
  response.headers.set("Content-Security-Policy", csp);
  return response;
}, { clockSkewInMs: 10_000 });

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
