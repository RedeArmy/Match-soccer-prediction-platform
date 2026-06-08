'use client'

import Link from 'next/link'
import { FeaturedPoolsSection } from '@/components/groups/FeaturedPoolsSection'
import { useI18n } from '@/lib/i18n'

export default function QuinielasPage() {
  const { t } = useI18n()

  return (
    <div className="px-4 py-8">
      <div className="mx-auto max-w-7xl">
        <section className="panel mb-8 overflow-hidden">
          <div className="wc26-stripe" />
          <div className="flex flex-col gap-4 p-5 md:flex-row md:items-end md:justify-between">
            <div>
              <p className="text-xs uppercase text-gold-300">{t('common.event')}</p>
              <h1 className="mt-2 font-display text-4xl text-white sm:text-5xl">{t('common.kinielas')}</h1>
              <p className="mt-1 max-w-2xl text-text-secondary">{t('tournaments.subtitle')}</p>
            </div>
            <Link href="/dashboard" className="btn-gold w-full py-2 text-sm md:w-auto">
              {t('groups.createTitle')}
            </Link>
          </div>
        </section>

        <FeaturedPoolsSection />
      </div>
    </div>
  )
}
