import type { NextConfig } from 'next'

// NEXT_PUBLIC_APP_URL must be set in production to the exact origin
// (scheme + host + optional port) of the deployed frontend, e.g.
// "https://quinielamundial.gt". Without it, Next.js only allows
// same-origin Server Actions, which blocks requests in production.
// Falls back to localhost for local development.
const appUrl = process.env.NEXT_PUBLIC_APP_URL

const nextConfig: NextConfig = {
  output: 'standalone',
  experimental: {
    serverActions: {
      allowedOrigins: appUrl ? [appUrl] : ['http://localhost:3000'],
    },
  },
  images: {
    formats: ['image/avif', 'image/webp'],
    remotePatterns: [],
  },
  // Security headers — static headers that do not depend on per-request state.
  // Content-Security-Policy is intentionally absent here: it is generated
  // per-request in middleware.ts so it can include a cryptographic nonce.
  // A static CSP from next.config.ts cannot contain a nonce and would require
  // 'unsafe-inline' for all scripts, defeating XSS protection.
  async headers() {
    return [
      {
        source: '/(.*)',
        headers: [
          { key: 'Strict-Transport-Security', value: 'max-age=31536000; includeSubDomains' },
          { key: 'X-Content-Type-Options',    value: 'nosniff' },
          { key: 'X-Frame-Options',           value: 'DENY' },
          { key: 'Referrer-Policy',           value: 'strict-origin-when-cross-origin' },
        ],
      },
    ]
  },
}

export default nextConfig
