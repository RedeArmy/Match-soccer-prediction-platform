'use client'

import { useAuth, useUser } from '@clerk/nextjs'
import { useQuery } from '@tanstack/react-query'
import { ShieldAlert, Trophy } from 'lucide-react'
import Link from 'next/link'
import { api } from '@/lib/api'
import { useSSE } from '@/hooks/useSSE'
import { useKYCStatus } from '@/hooks/useKYCStatus'
import { BalanceCard } from '@/components/balance/BalanceCard'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { EmptyState } from '@/components/shared/EmptyState'
import { PredictionPanel } from '@/components/predictions/PredictionPanel'
import { formatGTQ, formatDate } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'

export default function DashboardPage() {
  const { getToken } = useAuth()
  const { user } = useUser()
  const { t } = useI18n()
  useSSE()

  const { data: kyc } = useKYCStatus()
  const { data: groups, isLoading: loadingGroups } = useQuery({
    queryKey: ['my-groups'],
    queryFn: async () => {
      const token = await getToken()
      return api.getMyGroups(token!)
    },
  })

  const { data: ledger, isLoading: loadingLedger } = useQuery({
    queryKey: ['ledger-preview'],
    queryFn: async () => {
      const token = await getToken()
      return api.getLedger(token!, undefined, 5)
    },
  })

  const kycApproved = kyc?.status === 'approved'
  const displayName = (user?.firstName ?? t('dashboard.player')).toUpperCase()

  return (
    <div className="space-y-8">
      <section className="panel overflow-hidden">
        <div className="wc26-stripe" />
        <div className="flex flex-col justify-between gap-4 p-5 sm:flex-row sm:items-end">
          <div>
            <p className="text-xs uppercase text-gold-300">{t('dashboard.commandTitle')}</p>
            <h1 className="mt-2 font-display text-4xl text-white">
              {t('dashboard.hello')}, {displayName}
            </h1>
            <p className="mt-1 text-sm text-text-secondary">{t('dashboard.commandCopy')}</p>
          </div>
          <Link href="/tournaments" className="btn-ghost px-4 py-2 text-sm">
            {t('dashboard.exploreTournaments')}
          </Link>
        </div>
      </section>

      <div className="grid gap-6 lg:grid-cols-5">
        <div className="space-y-4 lg:col-span-2">
          <BalanceCard />

          {!kycApproved && (
            <div className="card p-4">
              <div className="flex items-start gap-3">
                <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0 text-gold-400" />
                <div className="min-w-0">
                  <p className="text-sm font-medium text-text-primary">{t('dashboard.kycTitle')}</p>
                  <p className="mt-0.5 text-xs text-text-secondary">
                    {t('dashboard.kycCopy')}
                    {kyc?.status && (
                      <span className="ml-1">
                        <StatusBadge status={kyc.status} size="sm" />
                      </span>
                    )}
                  </p>
                  <Link href="/kyc" className="btn-gold mt-3 px-3 py-1.5 text-xs">
                    {t('dashboard.kycAction')}
                  </Link>
                </div>
              </div>
            </div>
          )}
        </div>

        <div className="space-y-6 lg:col-span-3">
          <section>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
                {t('dashboard.myPools')}
              </h2>
              <Link href="/tournaments" className="text-xs text-gold-400 hover:text-gold-300">
                {t('dashboard.exploreTournaments')}
              </Link>
            </div>

            {loadingGroups && <LoadingState rows={2} />}
            {!loadingGroups && groups?.length === 0 && (
              <EmptyState
                title={t('dashboard.noPools')}
                description={t('dashboard.noPoolsDesc')}
                icon={<Trophy className="h-8 w-8" />}
                action={<Link href="/tournaments" className="btn-gold px-4 py-2 text-sm">{t('common.tournaments')}</Link>}
              />
            )}
            {!loadingGroups && (groups?.length ?? 0) > 0 && (
              <div className="space-y-2">
                {groups?.map((group) => (
                  <div key={group.id} className="card flex items-center justify-between gap-3 p-4">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-text-primary">{group.name}</p>
                      <p className="text-xs text-text-muted">
                        {group.member_count} {t('dashboard.participants')}
                      </p>
                    </div>
                    <div className="flex shrink-0 items-center gap-3">
                      <StatusBadge status={group.status} size="sm" />
                      <Link href={`/tournaments/${group.id}`} className="text-xs text-gold-400 hover:text-gold-300">
                        {t('common.viewAll')}
                      </Link>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          <section>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
                {t('dashboard.recentTransactions')}
              </h2>
              <Link href="/balance" className="text-xs text-gold-400 hover:text-gold-300">
                {t('common.viewAll')}
              </Link>
            </div>

            {loadingLedger && <LoadingState rows={3} />}
            {!loadingLedger && (ledger?.data?.length ?? 0) === 0 && (
              <p className="py-4 text-center text-sm text-text-muted">{t('dashboard.noTransactions')}</p>
            )}
            {!loadingLedger && (ledger?.data?.length ?? 0) > 0 && (
              <div className="space-y-1.5">
                {ledger?.data?.map((entry) => (
                  <div key={entry.id} className="card flex items-center justify-between gap-2 p-3">
                    <div className="min-w-0">
                      <p className="truncate text-xs text-text-primary">{entry.description || entry.type}</p>
                      <p className="text-[10px] text-text-muted">{formatDate(entry.created_at)}</p>
                    </div>
                    <span className={`shrink-0 font-score text-sm font-medium ${entry.type === 'credit' ? 'text-green-400' : 'text-red-400'}`}>
                      {entry.type === 'credit' ? '+' : '-'}
                      {formatGTQ(entry.amount_cents)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </section>
        </div>
      </div>

      <PredictionPanel />
    </div>
  )
}
