'use client'

import Link from 'next/link'
import { UserButton, SignedIn, SignedOut } from '@clerk/nextjs'
import { useExchangeRate } from '@/hooks/useExchangeRate'
import { formatRate } from '@/lib/utils'
import { LayoutDashboard, Menu, TrendingUp, Trophy, Wallet } from 'lucide-react'
import { useEffect, useState } from 'react'
import { MobileNav } from './MobileNav'
import { LanguageSwitcher } from './LanguageSwitcher'
import { useI18n } from '@/lib/i18n'

export function Header() {
  const { data: rate } = useExchangeRate()
  const [mobileOpen, setMobileOpen] = useState(false)
  const { t } = useI18n()

  return (
    <>
      <header className="sticky top-0 z-40 border-b border-white/10 bg-[#070A0F]/88 backdrop-blur-md">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="flex h-16 items-center justify-between gap-4">
            <Link href="/" className="flex shrink-0 items-center gap-2">
              <span className="grid h-9 w-9 place-items-center rounded border border-gold-400/40 bg-gold-400/10 font-display text-xl leading-none text-gold-300">
                K26
              </span>
              <span className="hidden sm:block">
                <span className="block text-sm font-semibold text-text-primary">{t('common.brand')}</span>
                <span className="block text-[11px] uppercase text-text-muted">{t('common.event')}</span>
              </span>
            </Link>

            <nav className="hidden items-center gap-1 rounded border border-white/10 bg-white/[0.03] p-1 md:flex">
              <SignedOut>
                <Link href="/tournaments" className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-text-secondary transition-colors hover:bg-white/[0.05] hover:text-text-primary">
                  <Trophy className="h-3.5 w-3.5" />
                  {t('common.tournaments')}
                </Link>
              </SignedOut>
              <SignedIn>
                <Link href="/quinielas" className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-text-secondary transition-colors hover:bg-white/[0.05] hover:text-text-primary">
                  <Trophy className="h-3.5 w-3.5" />
                  {t('common.kinielas')}
                </Link>
                <Link href="/dashboard" className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-text-secondary transition-colors hover:bg-white/[0.05] hover:text-text-primary">
                  <LayoutDashboard className="h-3.5 w-3.5" />
                  {t('common.dashboard')}
                </Link>
                <Link href="/balance" className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-text-secondary transition-colors hover:bg-white/[0.05] hover:text-text-primary">
                  <Wallet className="h-3.5 w-3.5" />
                  {t('common.balance')}
                </Link>
              </SignedIn>
            </nav>

            <div className="flex items-center gap-3">
              {rate && (
                <div className="hidden items-center gap-1 rounded border border-white/10 bg-white/[0.04] px-2 py-1 text-xs text-text-muted lg:flex">
                  <TrendingUp className="h-3 w-3 text-gold-400" />
                  <span className="font-score">
                    Q{formatRate(rate.sell_rate)}/<span className="text-text-secondary">USD</span>
                  </span>
                  {rate.stale && <span className="ml-1 text-red-400">●</span>}
                </div>
              )}

              <AuthSection t={t} />

              <button
                type="button"
                className="inline-flex h-9 w-9 items-center justify-center rounded border border-white/10 bg-white/[0.04] text-text-secondary transition-colors hover:border-gold-400/40 hover:text-text-primary md:hidden"
                onClick={() => setMobileOpen(true)}
                aria-label={t('common.openMenu')}
              >
                <Menu className="h-5 w-5" />
              </button>
            </div>
          </div>
        </div>
      </header>

      <MobileNav open={mobileOpen} onClose={() => setMobileOpen(false)} />
    </>
  )
}

function AuthSection({ t }: Readonly<{ t: (key: string) => string }>) {
  const [mounted, setMounted] = useState(false)
  useEffect(() => setMounted(true), [])

  if (!mounted) return <div className="flex items-center gap-3" />

  return (
    <>
      <div className="hidden sm:block">
        <LanguageSwitcher />
      </div>
      <SignedIn>
        <UserButton appearance={{ elements: { avatarBox: 'w-8 h-8' } }} />
      </SignedIn>
      <SignedOut>
        <Link href="/sign-in" className="btn-ghost px-3 py-1.5 text-sm">
          {t('common.signIn')}
        </Link>
        <Link href="/sign-up" className="btn-gold hidden px-3 py-1.5 text-sm sm:inline-flex">
          {t('common.signUp')}
        </Link>
      </SignedOut>
    </>
  )
}
