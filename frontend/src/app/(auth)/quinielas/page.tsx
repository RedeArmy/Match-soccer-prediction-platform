'use client'

import { Clock, Trophy, Users } from 'lucide-react'
import Link from 'next/link'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { formatCountdown } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'
import { useCurrency } from '@/hooks/useCurrency'

const featuredGroups = [
  {
    id: 1,
    name: 'World Cup 2026 Global Pool',
    status: 'open',
    prize_pool_cents: 2500000,
    entry_fee_cents: 5000,
    member_count: 128,
    max_members: 500,
    deadline_at: '2026-06-11T18:00:00Z',
  },
  {
    id: 2,
    name: 'Americas Knockout Challenge',
    status: 'upcoming',
    prize_pool_cents: 1200000,
    entry_fee_cents: 2500,
    member_count: 64,
    max_members: 256,
    deadline_at: '2026-06-28T18:00:00Z',
  },
  {
    id: 3,
    name: 'Finals Elite Quiniela',
    status: 'upcoming',
    prize_pool_cents: 800000,
    entry_fee_cents: 10000,
    member_count: 24,
    max_members: 100,
    deadline_at: '2026-07-04T18:00:00Z',
  },
]

export default function QuinielasPage() {
  const { t } = useI18n()
  const { fmt } = useCurrency()

  return (
    <div className="px-4 py-8">
      <div className="mx-auto max-w-7xl">
        <section className="panel mb-8 overflow-hidden">
          <div className="wc26-stripe" />
          <div className="flex flex-col gap-4 p-5 md:flex-row md:items-end md:justify-between">
            <div>
              <p className="text-xs uppercase text-gold-300">{t('common.event')}</p>
              <h1 className="mt-2 font-display text-4xl text-white sm:text-5xl">{t('common.quinielas')}</h1>
              <p className="mt-1 max-w-2xl text-text-secondary">{t('tournaments.subtitle')}</p>
            </div>
            <Link href="/dashboard" className="btn-gold w-full py-2 text-sm md:w-auto">
              {t('groups.createTitle')}
            </Link>
          </div>
        </section>

        <div className="mb-4 flex items-center justify-between gap-3">
          <div>
            <p className="text-xs uppercase text-gold-300">{t('tournaments.marketplace')}</p>
            <h2 className="text-2xl font-semibold text-white">{t('tournaments.availablePools')}</h2>
          </div>
          <span className="hidden rounded-full border border-white/10 bg-white/[0.04] px-3 py-1 text-xs text-text-muted sm:inline-flex">
            {featuredGroups.length} {t('tournaments.options')}
          </span>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {featuredGroups.map((group) => (
            <article key={group.id} className="card overflow-hidden transition-colors hover:border-gold-400/30">
              <div className="h-2 bg-gradient-to-r from-red-500 via-gold-400 to-green-400" />
              <div className="p-5">
                <div className="mb-4 flex items-start justify-between gap-2">
                  <h2 className="text-lg font-semibold leading-tight text-white">{group.name}</h2>
                  <StatusBadge status={group.status} size="sm" />
                </div>

                <div className="grid grid-cols-2 gap-2 text-xs text-text-muted">
                  <div className="flex items-center gap-1.5">
                    <Trophy className="h-3.5 w-3.5 text-gold-400" />
                    <span suppressHydrationWarning>{t('tournaments.prize')}: {fmt(group.prize_pool_cents)}</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Users className="h-3.5 w-3.5 text-blue-300" />
                    <span>{group.member_count}/{group.max_members} {t('tournaments.members')}</span>
                  </div>
                  <div className="col-span-2">
                    {t('tournaments.entry')}: <span suppressHydrationWarning className="text-gold-400">{fmt(group.entry_fee_cents)}</span>
                  </div>
                  <div className="col-span-2 flex items-center gap-1.5">
                    <Clock className="h-3.5 w-3.5 text-text-muted" />
                    <span suppressHydrationWarning>{formatCountdown(group.deadline_at)}</span>
                  </div>
                </div>

                <Link href="/dashboard" className="btn-gold mt-5 w-full py-2 text-center text-sm">
                  {group.status === 'open' ? t('tournaments.enter') : t('tournaments.details')}
                </Link>
              </div>
            </article>
          ))}
        </div>
      </div>
    </div>
  )
}
