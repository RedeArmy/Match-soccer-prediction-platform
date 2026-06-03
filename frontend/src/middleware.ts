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
  // Webhook endpoints never go through Next.js middleware
]);

const isAdminRoute = createRouteMatcher(["/admin(.*)"]);

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
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic' 'unsafe-inline' https://clerk.com https://*.clerk.com`,
    "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
    "img-src 'self' data: blob: https:",
    "font-src 'self' https://fonts.gstatic.com",
    "connect-src 'self' https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev",
    "frame-src https://clerk.com https://*.clerk.com https://*.clerk.accounts.dev",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
}

export default clerkMiddleware(async (auth, req: NextRequest) => {
  // Generate a fresh nonce for every request. The nonce is set on:
  //   - the forwarded request headers ('x-nonce') so that Clerk's server
  //     ClerkProvider and any layout server component can read it via headers()
  //   - the response headers ('Content-Security-Policy') so the browser
  //     enforces the policy for the rendered page
  //
  // When auth.protect() redirects an unauthenticated user to /sign-in, this
  // function exits early and the NextResponse.next() block below never runs.
  // The 302 redirect itself carries no HTML body so CSP is irrelevant there.
  // When the browser follows the redirect and loads /sign-in (a public route),
  // the middleware runs again and the CSP IS set on that response.
  const nonce = generateNonce();
  const csp = buildCSP(nonce);

  if (!isPublicRoute(req)) {
    await auth.protect();
  }

  if (isAdminRoute(req)) {
    const { sessionClaims } = await auth();
    const meta = sessionClaims?.metadata as { role?: string } | undefined;
    if (meta?.role !== "admin") {
      return NextResponse.redirect(new URL("/dashboard", req.url));
    }
  }

  // Forward nonce to server components via request header.
  // Clerk's server ClerkProvider reads 'x-nonce' automatically and applies it
  // to all Clerk-injected scripts, so no layout.tsx change is needed.
  const requestHeaders = new Headers(req.headers);
  requestHeaders.set("x-nonce", nonce);

  const response = NextResponse.next({ request: { headers: requestHeaders } });
  response.headers.set("Content-Security-Policy", csp);
  return response;
});

export const config = {
  matcher: [
    // Skip Next.js internals and static files.
    // Written as a plain string literal (not String.raw) because Next.js
    // parses this config statically and does not evaluate JS expressions.
    "/((?!_next|[^?]*\\.(?:html?|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)",
    // Always run for API routes
    "/(api|trpc)(.*)",
  ],
};
