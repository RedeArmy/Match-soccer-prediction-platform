'use client'

import { useAuth } from '@clerk/nextjs'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useExchangeRate } from '@/hooks/useExchangeRate'
import { BalanceCard } from '@/components/balance/BalanceCard'
import { LoadingState } from '@/components/shared/LoadingState'
import { formatGTQ, formatUSD, formatDate, gtqToUSD } from '@/lib/utils'
import { useState, useRef, useEffect } from 'react'
import type { LedgerEntry } from '@/lib/api-types'

export default function BalancePage() {
  const { getToken } = useAuth()
  const { data: rate } = useExchangeRate()
  const [showUSD, setShowUSD] = useState(false)
  const sentinelRef = useRef<HTMLDivElement>(null)

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteQuery({
    queryKey: ['ledger'],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const token = await getToken()
      return api.getLedger(token!, pageParam, 50)
    },
    getNextPageParam: page => page.has_more ? page.next_cursor : undefined,
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

  const allEntries = data?.pages.flatMap(p => p.data) ?? []

  function display(cents: number) {
    if (showUSD && rate) return formatUSD(Math.round(gtqToUSD(cents / 100, rate.sell_rate) * 100))
    return formatGTQ(cents)
  }

  const typeColors: Record<string, string> = {
    credit:  'text-green-400',
    debit:   'text-red-400',
    reserve: 'text-gold-400',
    release: 'text-blue-400',
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="font-display text-3xl text-white">BALANCE</h1>
        <button
          onClick={() => setShowUSD(v => !v)}
          className="text-sm text-text-muted hover:text-gold-400 transition-colors"
        >
          Mostrar en {showUSD ? 'GTQ' : 'USD'}
        </button>
      </div>

      <BalanceCard />

      {/* Transaction history */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide">
          Historial de transacciones
        </h2>

        {isLoading ? (
          <LoadingState rows={6} />
        ) : !allEntries.length ? (
          <p className="text-text-muted text-sm text-center py-8">Sin transacciones aún</p>
        ) : (
          <div className="card divide-y divide-blue-800/50">
            {allEntries.map((entry: LedgerEntry) => (
              <div key={entry.id} className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="min-w-0">
                  <p className="text-sm text-text-primary truncate">
                    {entry.description || entry.type}
                  </p>
                  <p className="text-xs text-text-muted">{formatDate(entry.created_at)}</p>
                </div>
                <div className="text-right shrink-0">
                  <p className={`font-score text-sm font-medium ${typeColors[entry.type] ?? 'text-text-primary'}`}>
                    {['credit', 'release'].includes(entry.type) ? '+' : '-'}{display(entry.amount_cents)}
                  </p>
                  <p className="text-[10px] text-text-muted">{entry.currency}</p>
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
