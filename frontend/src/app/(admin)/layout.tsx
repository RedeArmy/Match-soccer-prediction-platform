import { auth } from '@clerk/nextjs/server'
import { redirect } from 'next/navigation'
import { AdminSidebar } from '@/components/layout/AdminSidebar'

export const dynamic = 'force-dynamic'

export default async function AdminLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const { getToken } = await auth()
  const token = await getToken()

  if (!token) {
    redirect('/sign-in')
  }

  const res = await fetch(`${process.env.BACKEND_INTERNAL_URL}/api/v1/users/me`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store',
  })

  if (!res.ok) {
    redirect('/dashboard')
  }

  const me = await res.json() as { role?: string }
  if (me.role !== 'admin') {
    redirect('/dashboard')
  }

  return (
    <div className="flex min-h-screen">
      <AdminSidebar />
      <main className="flex-1 overflow-auto bg-blue-950">
        <div className="max-w-6xl mx-auto px-6 py-8">{children}</div>
      </main>
    </div>
  )
}
