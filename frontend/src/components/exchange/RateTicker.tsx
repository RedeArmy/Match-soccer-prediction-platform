'use client'

import { useExchangeRate } from '@/hooks/useExchangeRate'
import { formatDateTime } from '@/lib/utils'
import { TrendingUp } from 'lucide-react'

// Full-width strip that live-refreshes every 60s via React Query
export function ExchangeRateTicker() {
  const { data: rate, isLoading } = useExchangeRate()

  if (isLoading || !rate) return null

  return (
    <section className="bg-blue-900/50 border-y border-blue-800 py-4 px-4">
      <div className="mx-auto max-w-7xl flex flex-wrap items-center justify-between gap-3 text-sm">
        <div className="flex items-center gap-2">
          <TrendingUp className="w-4 h-4 text-gold-400" />
          <span className="text-text-secondary">Tipo de cambio en tiempo real:</span>
          <span className="font-score text-gold-400 font-semibold">
            Q{parseFloat(rate.sell_rate).toFixed(4)} / USD
          </span>
        </div>
        <span className="text-text-muted text-xs">
          Actualizado: {formatDateTime(rate.effective_at)}
          {rate.stale && (
            <span className="ml-2 text-red-400">⚠ tipo de cambio desactualizado</span>
          )}
        </span>
      </div>
    </section>
  )
}
