'use client'

import { useState, useRef, useEffect } from 'react'
import { useSearchParams } from 'next/navigation'
import { useAuth } from '@clerk/nextjs'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useCurrency } from '@/hooks/useCurrency'
import { useExchangeRate } from '@/hooks/useExchangeRate'
import { BalanceCard } from '@/components/balance/BalanceCard'
import { LoadingState } from '@/components/shared/LoadingState'
import { formatDate, formatGTQ, usdToGTQ, ledgerKindKey, isVisibleLedgerKind } from '@/lib/utils'
import type { LedgerEntry } from '@/lib/api-types'
import { useI18n } from '@/lib/i18n'
import { CheckCircle, X } from 'lucide-react'

function getLedgerColor(kind: string, deltaCents: number): string {
  if (kind === 'withdrawal_release') return 'text-amber-400'
  return deltaCents >= 0 ? 'text-green-400' : 'text-red-400'
}

export default function BalancePage() {
  const { getToken } = useAuth()
  const { fmt, isUSD } = useCurrency()
  const { t } = useI18n()
  const sentinelRef = useRef<HTMLDivElement>(null)

  // PayPal success redirect params
  const searchParams = useSearchParams()
  const paypalSuccess = searchParams.get('paypal') === 'success'
  const usdAmount = paypalSuccess ? Number(searchParams.get('usd') ?? '0') : 0
  const [bannerVisible, setBannerVisible] = useState(paypalSuccess)
  const [refetchEnabled, setRefetchEnabled] = useState(paypalSuccess)
  const { data: rate } = useExchangeRate()

  const gtqEstimate =
    rate && usdAmount > 0
      ? Math.round(usdToGTQ(usdAmount, rate.buy_rate) * 100)
      : null

  // Poll ledger for 15 s after PayPal redirect so the webhook-credited entry appears promptly
  useEffect(() => {
    if (!paypalSuccess) return
    const timer = setTimeout(() => setRefetchEnabled(false), 15_000)
    return () => clearTimeout(timer)
  }, [paypalSuccess])

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteQuery({
    queryKey: ['ledger'],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const token = await getToken()
      return api.getLedger(token!, pageParam, 50)
    },
    getNextPageParam: () => undefined,
    refetchInterval: refetchEnabled ? 2_000 : false,
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

  const allEntries = (data?.pages.flat() ?? []).filter(e => isVisibleLedgerKind(e.kind))

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl text-white">BALANCE</h1>

      {/* PayPal success banner */}
      {bannerVisible && (
        <div className="flex items-start gap-3 p-4 bg-green-900/50 border border-green-700/50 rounded-xl">
          <CheckCircle className="w-5 h-5 text-green-400 shrink-0 mt-0.5" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium text-green-300">
              Pago PayPal de ${usdAmount.toFixed(2)} USD recibido
            </p>
            {gtqEstimate !== null && (
              <p className="text-xs text-green-400/80 mt-0.5">
                ≈ {formatGTQ(gtqEstimate)} GTQ al tipo de compra
              </p>
            )}
            <p className="text-xs text-text-muted mt-0.5">
              La acreditación aparecerá en el historial en breve.
            </p>
          </div>
          <button
            type="button"
            onClick={() => setBannerVisible(false)}
            className="text-text-muted hover:text-text-secondary"
            aria-label="Cerrar"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

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
                  <p className={`font-score text-sm font-medium ${getLedgerColor(entry.kind, entry.delta_cents)}`}>
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
