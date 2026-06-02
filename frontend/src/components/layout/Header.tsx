'use client'

import Link from 'next/link'
import { UserButton, SignedIn, SignedOut } from '@clerk/nextjs'
import { useExchangeRate } from '@/hooks/useExchangeRate'
import { formatRate } from '@/lib/utils'
import { TrendingUp, Menu } from 'lucide-react'
import { useState } from 'react'
import { MobileNav } from './MobileNav'

export function Header() {
  const { data: rate } = useExchangeRate()
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <>
      <header className="sticky top-0 z-40 border-b border-blue-800/60 bg-blue-950/90 backdrop-blur-md">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="flex h-14 items-center justify-between gap-4">

            {/* Logo */}
            <Link href="/" className="flex items-center gap-2 shrink-0">
              <span className="font-display text-2xl text-gold-400 leading-none">QM</span>
              <span className="hidden sm:block text-sm text-text-secondary font-medium">Mundial 2026</span>
            </Link>

            {/* Nav links — desktop */}
            <nav className="hidden md:flex items-center gap-6">
              <Link href="/tournaments" className="text-sm text-text-secondary hover:text-text-primary transition-colors">
                Torneos
              </Link>
              <SignedIn>
                <Link href="/dashboard" className="text-sm text-text-secondary hover:text-text-primary transition-colors">
                  Dashboard
                </Link>
                <Link href="/balance" className="text-sm text-text-secondary hover:text-text-primary transition-colors">
                  Balance
                </Link>
              </SignedIn>
            </nav>

            {/* Right side */}
            <div className="flex items-center gap-3">
              {/* Exchange rate ticker */}
              {rate && (
                <div className="hidden sm:flex items-center gap-1 text-xs text-text-muted bg-blue-900 px-2 py-1 rounded">
                  <TrendingUp className="w-3 h-3 text-gold-400" />
                  <span className="font-score">
                    Q{formatRate(rate.sell_rate)}/<span className="text-text-secondary">USD</span>
                  </span>
                  {rate.stale && <span className="text-red-400 ml-1">●</span>}
                </div>
              )}

              <SignedIn>
                <UserButton
                  appearance={{
                    elements: {
                      avatarBox: 'w-8 h-8',
                    },
                  }}
                />
              </SignedIn>

              <SignedOut>
                <Link href="/sign-in" className="btn-ghost text-sm py-1.5 px-3">
                  Iniciar sesión
                </Link>
                <Link href="/sign-up" className="btn-gold text-sm py-1.5 px-3">
                  Registrarse
                </Link>
              </SignedOut>

              {/* Mobile menu button */}
              <button
                className="md:hidden p-1.5 text-text-secondary hover:text-text-primary"
                onClick={() => setMobileOpen(true)}
                aria-label="Abrir menú"
              >
                <Menu className="w-5 h-5" />
              </button>
            </div>
          </div>
        </div>
      </header>

      <MobileNav open={mobileOpen} onClose={() => setMobileOpen(false)} />
    </>
  )
}
