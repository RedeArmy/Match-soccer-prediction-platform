'use client'

import Link from 'next/link'
import { useBalance } from '@/hooks/useBalance'
import { useExchangeRate } from '@/hooks/useExchangeRate'
import { formatGTQ, formatUSD, gtqToUSD } from '@/lib/utils'
import { LoadingState } from '@/components/shared/LoadingState'
import { Wallet, ArrowDownCircle, ArrowUpCircle } from 'lucide-react'
import { useState } from 'react'

export function BalanceCard() {
  const { data: balance, isLoading } = useBalance()
  const { data: rate } = useExchangeRate()
  const [showUSD, setShowUSD] = useState(false)

  if (isLoading) return <LoadingState rows={1} className="h-40" />

  const available = balance?.available_cents ?? 0
  const reserved  = balance?.reserved_cents  ?? 0
  const pending   = balance?.pending_cents   ?? 0

  function displayAmount(cents: number) {
    if (showUSD && rate) {
      return formatUSD(gtqToUSD(cents / 100, rate.sell_rate) * 100)
    }
    return formatGTQ(cents)
  }

  return (
    <div className="card-elevated p-5 space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Wallet className="w-4 h-4 text-gold-400" />
          <span className="text-sm font-semibold text-text-secondary">Balance</span>
        </div>
        <button
          onClick={() => setShowUSD(v => !v)}
          className="text-xs text-text-muted hover:text-gold-400 transition-colors"
        >
          {showUSD ? '→ GTQ' : '→ USD'}
        </button>
      </div>

      {/* Available */}
      <div>
        <p className="text-xs text-text-muted mb-0.5">Disponible</p>
        <p className="font-score text-3xl text-white font-semibold leading-none">
          {displayAmount(available)}
        </p>
      </div>

      {/* Sub-totals */}
      <div className="flex gap-4 text-xs text-text-muted">
        {reserved > 0 && (
          <div>
            <span className="text-gold-400">Reservado: </span>
            {displayAmount(reserved)}
          </div>
        )}
        {pending > 0 && (
          <div>
            <span className="text-blue-300">Pendiente: </span>
            {displayAmount(pending)}
          </div>
        )}
      </div>

      {/* Actions */}
      <div className="flex gap-2 pt-1">
        <Link href="/balance/deposit" className="btn-gold flex-1 flex items-center justify-center gap-1.5 text-sm py-2">
          <ArrowDownCircle className="w-4 h-4" />
          Depositar
        </Link>
        <Link href="/balance/withdraw" className="btn-ghost flex-1 flex items-center justify-center gap-1.5 text-sm py-2">
          <ArrowUpCircle className="w-4 h-4" />
          Retirar
        </Link>
      </div>
    </div>
  )
}
