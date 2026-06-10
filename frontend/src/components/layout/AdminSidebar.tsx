'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { UserButton, SignOutButton } from '@clerk/nextjs'
import {
  LayoutDashboard, Users, ShieldCheck, Trophy,
  Wallet, TrendingUp, Activity, Settings, LogOut,
} from 'lucide-react'
import { cn } from '@/lib/utils'

const navItems = [
  { href: '/admin/dashboard',     label: 'Dashboard',    icon: LayoutDashboard },
  { href: '/admin/users',         label: 'Usuarios',     icon: Users           },
  { href: '/admin/kyc/queue',     label: 'KYC Queue',    icon: ShieldCheck     },
  { href: '/admin/tournaments',   label: 'Torneos',      icon: Trophy          },
  { href: '/admin/withdrawals',   label: 'Retiros',      icon: Wallet          },
  { href: '/admin/exchange-rate', label: 'Tipo de cambio', icon: TrendingUp    },
  { href: '/admin/observability', label: 'Observabilidad', icon: Activity      },
  { href: '/admin/system-params', label: 'Parámetros',   icon: Settings        },
]

export function AdminSidebar() {
  const pathname = usePathname()

  return (
    <aside className="flex flex-col w-64 min-h-screen border-r border-blue-800/60 bg-blue-950">
      {/* Brand */}
      <div className="p-4 border-b border-blue-800/60">
        <Link href="/admin/dashboard" className="flex items-center gap-2">
          <span className="font-display text-xl text-gold-400">QM</span>
          <span className="text-xs text-text-muted font-medium">Admin</span>
        </Link>
      </div>

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto p-3 space-y-0.5">
        {navItems.map(({ href, label, icon: Icon }) => {
          const active = pathname.startsWith(href)
          return (
            <Link
              key={href}
              href={href}
              className={cn(
                'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors',
                active
                  ? 'bg-blue-800 text-gold-400 font-medium'
                  : 'text-text-secondary hover:text-text-primary hover:bg-blue-900',
              )}
            >
              <Icon className="w-4 h-4 shrink-0" />
              {label}
            </Link>
          )
        })}
      </nav>

      {/* User */}
      <div className="p-4 border-t border-blue-800/60 flex items-center gap-3">
        <UserButton appearance={{ elements: { avatarBox: 'w-8 h-8' } }} />
        <span className="flex-1 text-xs text-text-muted">Admin</span>
        <SignOutButton>
          <button
            type="button"
            title="Cerrar sesión"
            className="rounded p-1.5 text-text-muted transition-colors hover:bg-blue-900 hover:text-red-400"
          >
            <LogOut className="h-4 w-4" />
          </button>
        </SignOutButton>
      </div>
    </aside>
  )
}
