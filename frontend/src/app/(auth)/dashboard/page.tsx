import { auth } from '@clerk/nextjs/server'
import { headers } from 'next/headers'
import { redirect } from 'next/navigation'
import DashboardContent from './DashboardContent'

export const dynamic = 'force-dynamic'

export default async function DashboardPage() {
  const { getToken } = await auth()
  const token = await getToken()

  if (!token) {
    redirect('/sign-in')
  }

  // Middleware resolves the role via /users/me and forwards it as x-user-role
  // so we can skip a redundant fetch on the happy path. If the header is absent
  // (middleware fetch failed or was bypassed) we fall back to fetching here.
  const reqHeaders = await headers()
  const roleFromMiddleware = reqHeaders.get('x-user-role')

  if (roleFromMiddleware) {
    if (roleFromMiddleware === 'admin') redirect('/admin/dashboard')
  } else {
    const res = await fetch(`${process.env.BACKEND_INTERNAL_URL}/api/v1/users/me`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: 'no-store',
    })
    if (res.ok) {
      const me = await res.json() as { role?: string }
      if (me.role === 'admin') redirect('/admin/dashboard')
    }
  }

  return <DashboardContent />
}
