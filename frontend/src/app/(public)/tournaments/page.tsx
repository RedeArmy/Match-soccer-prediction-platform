'use client'

import { GroupCodeLookup } from '@/components/public/GroupCodeLookup'
import { LiveMatchFeed } from '@/components/public/LiveMatchFeed'
import { useI18n } from '@/lib/i18n'

export default function TournamentsPage() {
  const { t } = useI18n()

  return (
    <div className="px-4 py-8">
      <div className="mx-auto max-w-7xl space-y-8">
        {/* Hero header */}
        <section className="panel overflow-hidden">
          <div className="wc26-stripe" />
          <div className="p-5">
            <p className="text-xs uppercase text-gold-300">{t('common.event')}</p>
            <h1 className="mt-2 font-display text-4xl text-white sm:text-5xl">{t('tournaments.title')}</h1>
            <p className="mt-1 max-w-2xl text-text-secondary">{t('tournaments.subtitle')}</p>
          </div>
        </section>

        {/* Group code lookup above live feed; side-by-side on large screens */}
        <div className="grid gap-6 lg:grid-cols-[380px_1fr]">
          <GroupCodeLookup />
          <LiveMatchFeed />
        </div>
      </div>
    </div>
  )
}
