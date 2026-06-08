'use client'

import { useAuth } from '@clerk/nextjs'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useCurrency } from '@/hooks/useCurrency'
import { BalanceCard } from '@/components/balance/BalanceCard'
import { LoadingState } from '@/components/shared/LoadingState'
import { formatDate } from '@/lib/utils'
import { useRef, useEffect } from 'react'
import type { LedgerEntry } from '@/lib/api-types'
import { useI18n } from '@/lib/i18n'
import { ledgerKindKey } from '@/lib/utils'

export default function BalancePage() {
  const { getToken } = useAuth()
  const { fmt, isUSD } = useCurrency()
  const { t } = useI18n()
  const sentinelRef = useRef<HTMLDivElement>(null)

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteQuery({
    queryKey: ['ledger'],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const token = await getToken()
      return api.getLedger(token!, pageParam, 50)
    },
    getNextPageParam: () => undefined,
  })

  // Infinite scroll
  useEffect(() => {
    const el = sentinelRef.current
    if (!el) return
    const observer = new IntersectionObserver(entries => {
      if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
        fetchNextPage()
      }
    }, { threshold: 0.5 })
    observer.observe(el)
    return () => observer.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const allEntries = data?.pages.flat() ?? []

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl text-white">BALANCE</h1>

      <BalanceCard />

      {/* Transaction history */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide">
          Historial de transacciones
        </h2>

        {isLoading && <LoadingState rows={6} />}
        {!isLoading && allEntries.length === 0 && (
          <p className="text-text-muted text-sm text-center py-8">Sin transacciones aún</p>
        )}
        {!isLoading && allEntries.length > 0 && (
          <div className="card divide-y divide-blue-800/50">
            {allEntries.map((entry: LedgerEntry) => (
              <div key={entry.id} className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="min-w-0">
                  <p className="text-sm text-text-primary truncate">
                    {t(ledgerKindKey(entry.kind))}
                  </p>
                  <p className="text-xs text-text-muted">{formatDate(entry.created_at)}</p>
                </div>
                <div className="text-right shrink-0">
                  <p className={`font-score text-sm font-medium ${entry.delta_cents >= 0 ? 'text-green-400' : 'text-red-400'}`}>
                    {entry.delta_cents >= 0 ? '+' : ''}{fmt(entry.delta_cents)}
                  </p>
                  <p className="text-[10px] text-text-muted">{isUSD ? 'USD' : 'GTQ'}</p>
                </div>
              </div>
            ))}
          </div>
        )}

        <div ref={sentinelRef} className="h-4" />
        {isFetchingNextPage && <LoadingState rows={2} />}
      </section>
    </div>
  )
}
