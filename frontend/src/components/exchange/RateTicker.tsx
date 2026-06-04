'use client'

import { useExchangeRate } from '@/hooks/useExchangeRate'
import { formatDateTime } from '@/lib/utils'
import { TrendingUp } from 'lucide-react'
import { useI18n } from '@/lib/i18n'

export function ExchangeRateTicker() {
  const { data: rate, isLoading } = useExchangeRate()
  const { t } = useI18n()

  if (isLoading || !rate) return null

  return (
    <section className="border-y border-white/10 bg-white/[0.025] px-4 py-4">
      <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-3 text-sm">
        <div className="flex items-center gap-2">
          <TrendingUp className="h-4 w-4 text-gold-400" />
          <span className="text-text-secondary">{t('ticker.live')}:</span>
          <span className="font-score font-semibold text-gold-400">
            Q{Number.parseFloat(rate.sell_rate).toFixed(4)} / USD
          </span>
        </div>
        <span className="text-xs text-text-muted">
          {t('common.updated')}: {formatDateTime(rate.effective_at)}
          {rate.stale && (
            <span className="ml-2 text-red-400">{t('ticker.outdated')}</span>
          )}
        </span>
      </div>
    </section>
  )
}
