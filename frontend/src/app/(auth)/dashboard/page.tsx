'use client'

import { useAuth, useUser } from '@clerk/nextjs'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useSSE } from '@/hooks/useSSE'
import { useKYCStatus } from '@/hooks/useKYCStatus'
import { BalanceCard } from '@/components/balance/BalanceCard'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { EmptyState } from '@/components/shared/EmptyState'
import { formatGTQ, formatDate } from '@/lib/utils'
import { ShieldAlert, Trophy } from 'lucide-react'
import Link from 'next/link'

export default function DashboardPage() {
  const { getToken } = useAuth()
  const { user } = useUser()
  useSSE()  // maintain SSE connection for real-time updates

  const { data: kyc } = useKYCStatus()
  const { data: groups, isLoading: loadingGroups } = useQuery({
    queryKey: ['my-groups'],
    queryFn:  async () => {
      const token = await getToken()
      return api.getMyGroups(token!)
    },
  })

  const { data: ledger, isLoading: loadingLedger } = useQuery({
    queryKey: ['ledger-preview'],
    queryFn:  async () => {
      const token = await getToken()
      return api.getLedger(token!, undefined, 5)
    },
  })

  const kycApproved = kyc?.status === 'approved'

  return (
    <div className="space-y-8">

      {/* Greeting */}
      <div>
        <h1 className="font-display text-3xl text-white">
          HOLA, {(user?.firstName ?? 'JUGADOR').toUpperCase()}
        </h1>
        <p className="text-text-secondary text-sm mt-1">Dashboard personal</p>
      </div>

      <div className="grid lg:grid-cols-5 gap-6">

        {/* Left column — balance + KYC */}
        <div className="lg:col-span-2 space-y-4">
          <BalanceCard />

          {!kycApproved && (
            <div className="card p-4 border-l-4 border-l-gold-500 flex gap-3 items-start">
              <ShieldAlert className="w-5 h-5 text-gold-400 shrink-0 mt-0.5" />
              <div className="min-w-0">
                <p className="text-sm font-medium text-text-primary">Verifica tu identidad</p>
                <p className="text-xs text-text-secondary mt-0.5">
                  Completa tu KYC para poder hacer retiros.
                  {kyc?.status && (
                    <span className="ml-1">
                      Estado: <StatusBadge status={kyc.status} size="sm" />
                    </span>
                  )}
                </p>
                <Link href="/kyc" className="btn-gold inline-block text-xs px-3 py-1.5 mt-2">
                  Verificar ahora
                </Link>
              </div>
            </div>
          )}
        </div>

        {/* Right column — groups + recent ledger */}
        <div className="lg:col-span-3 space-y-6">

          {/* My groups */}
          <section>
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide">Mis Quinielas</h2>
              <Link href="/tournaments" className="text-xs text-gold-400 hover:text-gold-300">
                Explorar torneos →
              </Link>
            </div>

            {loadingGroups ? (
              <LoadingState rows={2} />
            ) : !groups?.length ? (
              <EmptyState
                title="Aún no tienes quinielas"
                description="Únete a un torneo para empezar a predecir"
                icon={<Trophy className="w-8 h-8" />}
                action={
                  <Link href="/tournaments" className="btn-gold text-sm px-4 py-2 inline-block">
                    Ver torneos
                  </Link>
                }
              />
            ) : (
              <div className="space-y-2">
                {groups.map(g => (
                  <div key={g.id} className="card p-4 flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-text-primary truncate">{g.name}</p>
                      <p className="text-xs text-text-muted">{g.member_count} participantes</p>
                    </div>
                    <div className="flex items-center gap-3 shrink-0">
                      <StatusBadge status={g.status} size="sm" />
                      <Link
                        href={`/tournaments/${g.id}`}
                        className="text-xs text-gold-400 hover:text-gold-300"
                      >
                        Ver →
                      </Link>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          {/* Recent transactions */}
          <section>
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide">Últimas transacciones</h2>
              <Link href="/balance" className="text-xs text-gold-400 hover:text-gold-300">Ver todo →</Link>
            </div>

            {loadingLedger ? (
              <LoadingState rows={3} />
            ) : !ledger?.data.length ? (
              <p className="text-sm text-text-muted py-4 text-center">Sin transacciones aún</p>
            ) : (
              <div className="space-y-1.5">
                {ledger.data.map(entry => (
                  <div key={entry.id} className="card p-3 flex items-center justify-between gap-2">
                    <div className="min-w-0">
                      <p className="text-xs text-text-primary truncate">{entry.description || entry.type}</p>
                      <p className="text-[10px] text-text-muted">{formatDate(entry.created_at)}</p>
                    </div>
                    <span className={`font-score text-sm font-medium shrink-0 ${
                      entry.type === 'credit' ? 'text-green-400' : 'text-red-400'
                    }`}>
                      {entry.type === 'credit' ? '+' : '-'}{formatGTQ(entry.amount_cents)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </section>

        </div>
      </div>
    </div>
  )
}
