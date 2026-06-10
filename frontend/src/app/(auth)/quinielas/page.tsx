'use client'

import { useState } from 'react'
import { Plus, Users } from 'lucide-react'
import { FeaturedPoolsSection } from '@/components/groups/FeaturedPoolsSection'
import { GroupDialog } from '@/components/groups/GroupDialog'
import { useI18n } from '@/lib/i18n'

export default function QuinielasPage() {
  const { t } = useI18n()
  const [dialog, setDialog] = useState<'create' | 'join' | null>(null)

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
            <div className="flex w-full flex-col gap-2 sm:flex-row md:w-auto">
              <button
                type="button"
                onClick={() => setDialog('join')}
                className="flex w-full items-center justify-center gap-2 rounded-lg border border-white/15 px-4 py-2 text-sm font-medium text-text-primary transition-colors hover:border-white/30 hover:text-white sm:w-auto"
              >
                <Users className="h-4 w-4" />
                {t('tournaments.joinPool')}
              </button>
              <button
                type="button"
                onClick={() => setDialog('create')}
                className="btn-gold flex w-full items-center justify-center gap-2 py-2 text-sm sm:w-auto"
              >
                <Plus className="h-4 w-4" />
                {t('groups.createTitle')}
              </button>
            </div>
          </div>
        </section>

        <FeaturedPoolsSection />
      </div>

      {dialog && (
        <GroupDialog
          open
          defaultTab={dialog}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  )
}
