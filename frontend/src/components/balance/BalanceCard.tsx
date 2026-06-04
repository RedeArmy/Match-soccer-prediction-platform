'use client'

import Link from 'next/link'
import { useState } from 'react'
import { ArrowDownCircle, ArrowUpCircle, Wallet } from 'lucide-react'
import { useBalance } from '@/hooks/useBalance'
import { useExchangeRate } from '@/hooks/useExchangeRate'
import { formatGTQ, formatUSD, gtqToUSD } from '@/lib/utils'
import { LoadingState } from '@/components/shared/LoadingState'
import { useI18n } from '@/lib/i18n'

export function BalanceCard() {
  const { data: balance, isLoading } = useBalance()
  const { data: rate } = useExchangeRate()
  const [showUSD, setShowUSD] = useState(false)
  const { t } = useI18n()

  if (isLoading) return <LoadingState rows={1} className="h-40" />

  const available = balance?.available_cents ?? 0
  const reserved = balance?.reserved_cents ?? 0
  const pending = balance?.pending_cents ?? 0

  function displayAmount(cents: number) {
    if (showUSD && rate) {
      return formatUSD(gtqToUSD(cents / 100, rate.sell_rate) * 100)
    }
    return formatGTQ(cents)
  }

  return (
    <div className="card-elevated space-y-4 p-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Wallet className="h-4 w-4 text-gold-400" />
          <span className="text-sm font-semibold text-text-secondary">{t('balanceCard.title')}</span>
        </div>
        <button
          onClick={() => setShowUSD((value) => !value)}
          className="text-xs text-text-muted transition-colors hover:text-gold-400"
        >
          {showUSD ? '-> GTQ' : '-> USD'}
        </button>
      </div>

      <div>
        <p className="mb-0.5 text-xs text-text-muted">{t('balanceCard.available')}</p>
        <p className="font-score text-3xl font-semibold leading-none text-white">
          {displayAmount(available)}
        </p>
      </div>

      <div className="flex gap-4 text-xs text-text-muted">
        {reserved > 0 && (
          <div>
            <span className="text-gold-400">{t('balanceCard.reserved')}: </span>
            {displayAmount(reserved)}
          </div>
        )}
        {pending > 0 && (
          <div>
            <span className="text-blue-300">{t('balanceCard.pending')}: </span>
            {displayAmount(pending)}
          </div>
        )}
      </div>

      <div className="flex gap-2 pt-1">
        <Link href="/balance/deposit" className="btn-gold flex-1 py-2 text-sm">
          <ArrowDownCircle className="h-4 w-4" />
          {t('balanceCard.deposit')}
        </Link>
        <Link href="/balance/withdraw" className="btn-ghost flex-1 py-2 text-sm">
          <ArrowUpCircle className="h-4 w-4" />
          {t('balanceCard.withdraw')}
        </Link>
      </div>
    </div>
  )
}
