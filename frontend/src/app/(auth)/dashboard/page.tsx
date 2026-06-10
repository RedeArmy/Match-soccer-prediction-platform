import { auth } from '@clerk/nextjs/server'
import { redirect } from 'next/navigation'
import DashboardContent from './DashboardContent'

export const dynamic = 'force-dynamic'

export default async function DashboardPage() {
  const { getToken } = await auth()
  const token = await getToken()

  if (!token) {
    redirect('/sign-in')
  }

  const res = await fetch(`${process.env.BACKEND_INTERNAL_URL}/api/v1/users/me`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store',
  })

  if (res.ok) {
    const me = await res.json() as { role?: string }
    if (me.role === 'admin') {
      redirect('/admin/dashboard')
    }
  }

  return <DashboardContent />
}
