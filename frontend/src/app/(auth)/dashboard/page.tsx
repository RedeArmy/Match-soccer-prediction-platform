'use client'

import { useState, useRef } from 'react'
import { useAuth, useUser } from '@clerk/nextjs'
import { useQuery } from '@tanstack/react-query'
import { ArrowDownLeft, ArrowUpRight, Bell, ChevronLeft, ChevronRight, Plus, ShieldAlert, ShieldCheck, Trophy, Users } from 'lucide-react'
import Link from 'next/link'
import { api } from '@/lib/api'
import { useSSE } from '@/hooks/useSSE'
import { useKYCEffectiveStatus } from '@/hooks/useKYCEffectiveStatus'
import { BalanceCard } from '@/components/balance/BalanceCard'
import { GroupDialog } from '@/components/groups/GroupDialog'
import { PendingApprovalsDialog } from '@/components/groups/PendingApprovalsDialog'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { EmptyState } from '@/components/shared/EmptyState'
import { PredictionPanel } from '@/components/predictions/PredictionPanel'
import { formatDate, ledgerKindKey, isVisibleLedgerKind } from '@/lib/utils'
import { useCurrency } from '@/hooks/useCurrency'
import { useI18n } from '@/lib/i18n'
import type { LedgerEntry } from '@/lib/api-types'

type DialogTab = 'create' | 'join'

export default function DashboardPage() {
  const { getToken } = useAuth()
  const { user } = useUser()
  const { t } = useI18n()
  const { fmt } = useCurrency()
  useSSE()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [dialogTab, setDialogTab] = useState<DialogTab>('create')
  const [pendingGroupId, setPendingGroupId] = useState<number | null>(null)
  const [pendingGroupName, setPendingGroupName] = useState('')

  const { kyc, effectiveStatus: kycEffectiveStatus } = useKYCEffectiveStatus()
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
      return api.getLedger(token!, undefined, 10)
    },
  })

  function openDialog(tab: DialogTab) {
    setDialogTab(tab)
    setDialogOpen(true)
  }

  const txScrollRef = useRef<HTMLDivElement>(null)
  function scrollTx(dir: 'left' | 'right') {
    txScrollRef.current?.scrollBy({ left: dir === 'left' ? -196 : 196, behavior: 'smooth' })
  }

  const kycApproved = kyc?.status === 'approved'
  const displayName = (user?.firstName ?? t('dashboard.player')).toUpperCase()
  const visibleLedger = (ledger ?? []).filter(e => isVisibleLedgerKind(e.kind))

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
          <Link href="/quinielas" className="btn-ghost px-4 py-2 text-sm">
            {t('dashboard.exploreTournaments')}
          </Link>
        </div>
      </section>

      <div className="grid gap-6 lg:grid-cols-5">
        <div className="space-y-4 lg:col-span-2">
          <BalanceCard />

          {kycApproved ? (
            <div className="card p-4 border border-green-500/20 bg-green-500/5">
              <div className="flex items-center gap-3">
                <ShieldCheck className="h-5 w-5 shrink-0 text-green-400" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-sm font-medium text-green-300">{t('dashboard.kycVerifiedTitle')}</p>
                    <StatusBadge status="approved" size="sm" />
                  </div>
                  <p className="mt-0.5 text-xs text-text-secondary">
                    {t('dashboard.kycVerifiedCopy')}
                  </p>
                </div>
              </div>
            </div>
          ) : (
            <div className="card p-4">
              <div className="flex items-start gap-3">
                <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0 text-gold-400" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-sm font-medium text-text-primary">{t('dashboard.kycTitle')}</p>
                    {kyc?.status && <StatusBadge status={kycEffectiveStatus} size="sm" />}
                  </div>
                  <p className="mt-0.5 text-xs text-text-secondary text-center">
                    {t('dashboard.kycCopy')}
                  </p>
                  <div className="mt-3 flex justify-center">
                    <Link href="/kyc" className="btn-gold px-3 py-1.5 text-xs">
                      {t('dashboard.kycAction')}
                    </Link>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        <div className="space-y-6 lg:col-span-3">
          <section>
            <div className="mb-3 flex items-center justify-between gap-3">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
                {t('dashboard.myPools')}
              </h2>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => openDialog('join')}
                  className="btn-ghost flex items-center gap-1.5 px-3 py-1.5 text-xs"
                >
                  <Users className="h-3.5 w-3.5" />
                  {t('groups.joinBtn')}
                </button>
                <button
                  type="button"
                  onClick={() => openDialog('create')}
                  className="btn-gold flex items-center gap-1.5 px-3 py-1.5 text-xs"
                >
                  <Plus className="h-3.5 w-3.5" />
                  {t('groups.createBtn')}
                </button>
              </div>
            </div>

            {loadingGroups && <LoadingState rows={2} />}
            {!loadingGroups && groups?.length === 0 && (
              <EmptyState
                title={t('dashboard.noPools')}
                description={t('dashboard.noPoolsDesc')}
                icon={<Trophy className="h-8 w-8" />}
                action={
                  <button
                    type="button"
                    onClick={() => openDialog('create')}
                    className="btn-gold px-4 py-2 text-sm"
                  >
                    {t('groups.createAction')}
                  </button>
                }
              />
            )}
            {!loadingGroups && (groups?.length ?? 0) > 0 && (
              <div className="space-y-2">
                {groups?.map((group) => (
                  <GroupCard
                    key={group.id}
                    group={group}
                    onOpenPending={(id, name) => { setPendingGroupId(id); setPendingGroupName(name) }}
                    t={t}
                  />
                ))}
              </div>
            )}
          </section>

        </div>
      </div>

      {/* ── Últimas transacciones — carrusel full-width ── */}
      <section>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
            {t('dashboard.recentTransactions')}
          </h2>
          {visibleLedger.length > 0 && (
            <div className="ml-auto flex items-center gap-1">
              <button
                type="button"
                onClick={() => scrollTx('left')}
                className="rounded-lg p-1 text-text-muted transition-colors hover:bg-blue-800 hover:text-text-primary"
                aria-label="Anterior"
              >
                <ChevronLeft className="h-4 w-4" />
              </button>
              <button
                type="button"
                onClick={() => scrollTx('right')}
                className="rounded-lg p-1 text-text-muted transition-colors hover:bg-blue-800 hover:text-text-primary"
                aria-label="Siguiente"
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          )}
        </div>

        {loadingLedger && <LoadingState rows={1} />}
        {!loadingLedger && visibleLedger.length === 0 && (
          <p className="py-4 text-center text-sm text-text-muted">{t('dashboard.noTransactions')}</p>
        )}
        {!loadingLedger && visibleLedger.length > 0 && (
          <div
            ref={txScrollRef}
            className="no-scrollbar flex gap-4 overflow-x-auto scroll-smooth pb-1 [scroll-snap-type:x_mandatory]"
          >
            {visibleLedger.map((entry) => (
              <TxCard key={entry.id} entry={entry} t={t} />
            ))}
          </div>
        )}

        {!loadingLedger && visibleLedger.length > 0 && (
          <div className="mt-3 flex justify-end">
            <Link href="/balance" className="text-xs text-gold-400 hover:text-gold-300">
              {t('common.viewAll')}
            </Link>
          </div>
        )}
      </section>

      <PredictionPanel />

      <GroupDialog
        open={dialogOpen}
        defaultTab={dialogTab}
        onClose={() => setDialogOpen(false)}
      />

      <PendingApprovalsDialog
        groupId={pendingGroupId ?? 0}
        groupName={pendingGroupName}
        open={pendingGroupId !== null}
        onClose={() => setPendingGroupId(null)}
      />
    </div>
  )
}

// ── TxCard ────────────────────────────────────────────────────────────────────

interface TxCardProps {
  entry: LedgerEntry
  t: ReturnType<typeof useI18n>['t']
}

function TxCard({ entry, t }: Readonly<TxCardProps>) {
  const { fmt, isUSD } = useCurrency()
  const isCredit = entry.delta_cents >= 0
  const isPrize = entry.kind === 'prize'
  const isRefund = entry.kind === 'withdrawal_release'

  const iconEl = isPrize
    ? <Trophy className="h-4 w-4 text-gold-400" />
    : isRefund
      ? <ArrowUpRight className="h-4 w-4 text-amber-400" />
      : isCredit
        ? <ArrowUpRight className="h-4 w-4 text-green-400" />
        : <ArrowDownLeft className="h-4 w-4 text-red-400" />

  const iconBg = isPrize
    ? 'bg-gold-400/20'
    : isRefund ? 'bg-amber-400/20' : isCredit ? 'bg-green-400/20' : 'bg-red-400/20'

  const amountColor = isPrize
    ? 'text-gold-400'
    : isRefund ? 'text-amber-400' : isCredit ? 'text-green-400' : 'text-red-400'

  const displayAmount = isUSD
    ? fmt(entry.delta_cents)
    : fmt(entry.delta_cents).replace(/^Q\s*/, 'GTQ ')

  return (
    <div className="card flex w-44 shrink-0 flex-col gap-3 p-4 [scroll-snap-align:center]">
      <div className="flex-1">
        <div className="flex items-center justify-between gap-2">
          <p className="truncate text-xs font-medium text-text-primary">
            {t(ledgerKindKey(entry.kind))}
          </p>
          <div className={`shrink-0 flex h-9 w-9 items-center justify-center rounded-full ${iconBg}`}>
            {iconEl}
          </div>
        </div>
        <p className="mt-1 text-[10px] text-text-muted">{formatDate(entry.created_at)}</p>
      </div>
      <div>
        <p className={`text-center font-score text-base font-bold ${amountColor}`}>
          {isCredit ? '+' : ''}{displayAmount}
        </p>
      </div>
    </div>
  )
}

// ── GroupCard ─────────────────────────────────────────────────────────────────

interface GroupCardProps {
  group: import('@/lib/api-types').GroupResponse
  onOpenPending: (id: number, name: string) => void
  t: ReturnType<typeof useI18n>['t']
}

function GroupCard({ group, onOpenPending, t }: Readonly<GroupCardProps>) {
  const { getToken } = useAuth()
  const pendingQuery = useQuery({
    queryKey: ['group-members', group.id],
    queryFn: async () => {
      const token = await getToken()
      return api.getGroupMembers(token!, group.id)
    },
    enabled: group.membership_status === 'active',
  })

  const pendingCount = group.membership_status === 'active'
    ? (pendingQuery.data ?? []).filter((m) => m.status === 'pending').length
    : 0

  const isPending = group.membership_status === 'pending'

  return (
    <div className="card flex items-center justify-between gap-3 p-4">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium text-text-primary">{group.name}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {/* Notification bell — first slot, hidden when no pending requests */}
        {!isPending && pendingCount > 0 && (
          <button
            type="button"
            onClick={() => onOpenPending(group.id, group.name)}
            className="relative rounded-lg p-1.5 text-gold-400 transition-colors hover:bg-gold-400/10"
            title={t('groups.pendingTitle')}
          >
            <Bell className="h-4 w-4" />
            <span className="absolute -right-0.5 -top-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[9px] font-bold text-white">
              {pendingCount}
            </span>
          </button>
        )}

        <StatusBadge status={group.group_status} size="sm" />

        {/* Pending badge — member's own request not yet approved */}
        {isPending ? (
          <span className="rounded-full border border-red-400/30 bg-red-400/10 px-2.5 py-0.5 text-[11px] font-semibold text-red-300">
            {t('groups.pendingBadge')}
          </span>
        ) : (
          <Link href={`/tournaments/${group.id}`} className="text-xs text-gold-400 hover:text-gold-300">
            {t('common.viewAll')}
          </Link>
        )}
      </div>
    </div>
  )
}
