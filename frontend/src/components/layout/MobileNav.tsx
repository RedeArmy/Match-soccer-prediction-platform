'use client'

import Link from 'next/link'
import { SignedIn, SignedOut, useClerk } from '@clerk/nextjs'
import { X, Home, Trophy, LayoutDashboard, Wallet, User, LogOut } from 'lucide-react'
import { cn } from '@/lib/utils'

interface MobileNavProps {
  open:    boolean
  onClose: () => void
}

const navItems = [
  { href: '/',            label: 'Inicio',      icon: Home,            public: true  },
  { href: '/tournaments', label: 'Torneos',     icon: Trophy,          public: true  },
  { href: '/dashboard',   label: 'Dashboard',   icon: LayoutDashboard, public: false },
  { href: '/balance',     label: 'Balance',     icon: Wallet,          public: false },
  { href: '/profile',     label: 'Perfil',      icon: User,            public: false },
]

export function MobileNav({ open, onClose }: MobileNavProps) {
  const { signOut } = useClerk()

  return (
    <>
      {/* Backdrop */}
      <div
        role="button"
        tabIndex={0}
        aria-label="Cerrar menú"
        className={cn(
          'fixed inset-0 z-40 bg-blue-950/80 backdrop-blur-sm transition-opacity md:hidden',
          open ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none',
        )}
        onClick={onClose}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onClose() }}
      />

      {/* Drawer */}
      <aside
        className={cn(
          'fixed inset-y-0 right-0 z-50 w-72 bg-blue-900 border-l border-blue-700 flex flex-col',
          'transition-transform duration-300 md:hidden',
          open ? 'translate-x-0' : 'translate-x-full',
        )}
      >
        <div className="flex items-center justify-between p-4 border-b border-blue-700">
          <span className="font-display text-xl text-gold-400">Menú</span>
          <button onClick={onClose} className="p-1 text-text-secondary hover:text-text-primary">
            <X className="w-5 h-5" />
          </button>
        </div>

        <nav className="flex-1 overflow-y-auto p-4 space-y-1">
          {navItems.map(({ href, label, icon: Icon, public: isPublic }) => (
            <NavLink key={href} href={href} label={label} icon={<Icon className="w-4 h-4" />} isPublic={isPublic} onClose={onClose} />
          ))}
        </nav>

        <div className="p-4 border-t border-blue-700 space-y-2">
          <SignedOut>
            <Link href="/sign-in" onClick={onClose} className="btn-ghost w-full text-center block py-2 text-sm">
              Iniciar sesión
            </Link>
            <Link href="/sign-up" onClick={onClose} className="btn-gold w-full text-center block py-2 text-sm">
              Registrarse
            </Link>
          </SignedOut>
          <SignedIn>
            <button
              onClick={() => { signOut(); onClose() }}
              className="flex items-center gap-2 w-full text-sm text-red-400 hover:text-red-300 py-2 px-3 rounded-lg hover:bg-red-400/10 transition-colors"
            >
              <LogOut className="w-4 h-4" />
              Cerrar sesión
            </button>
          </SignedIn>
        </div>
      </aside>
    </>
  )
}

function NavLink({ href, label, icon, isPublic, onClose }: {
  href: string; label: string; icon: React.ReactNode; isPublic: boolean; onClose: () => void
}) {
  const content = (
    <Link
      href={href}
      onClick={onClose}
      className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-text-secondary hover:text-text-primary hover:bg-blue-800 transition-colors text-sm"
    >
      {icon}
      {label}
    </Link>
  )

  if (isPublic) return content
  return <SignedIn>{content}</SignedIn>
}
