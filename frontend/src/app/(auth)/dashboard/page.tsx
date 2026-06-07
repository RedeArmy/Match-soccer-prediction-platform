'use client'

import { useState } from 'react'
import { useAuth, useUser } from '@clerk/nextjs'
import { useQuery } from '@tanstack/react-query'
import { Bell, Plus, ShieldAlert, Trophy, Users } from 'lucide-react'
import Link from 'next/link'
import { api } from '@/lib/api'
import { useSSE } from '@/hooks/useSSE'
import { useKYCStatus } from '@/hooks/useKYCStatus'
import { BalanceCard } from '@/components/balance/BalanceCard'
import { GroupDialog } from '@/components/groups/GroupDialog'
import { PendingApprovalsDialog } from '@/components/groups/PendingApprovalsDialog'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { EmptyState } from '@/components/shared/EmptyState'
import { PredictionPanel } from '@/components/predictions/PredictionPanel'
import { formatDate } from '@/lib/utils'
import { useCurrency } from '@/hooks/useCurrency'
import { useI18n } from '@/lib/i18n'

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

  function openDialog(tab: DialogTab) {
    setDialogTab(tab)
    setDialogOpen(true)
  }

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
          <Link href="/quinielas" className="btn-ghost px-4 py-2 text-sm">
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
            {!loadingLedger && (ledger?.length ?? 0) === 0 && (
              <p className="py-4 text-center text-sm text-text-muted">{t('dashboard.noTransactions')}</p>
            )}
            {!loadingLedger && (ledger?.length ?? 0) > 0 && (
              <div className="space-y-1.5">
                {ledger?.map((entry) => (
                  <div key={entry.id} className="card flex items-center justify-between gap-2 p-3">
                    <div className="min-w-0">
                      <p className="truncate text-xs text-text-primary">{entry.description || entry.type}</p>
                      <p className="text-[10px] text-text-muted">{formatDate(entry.created_at)}</p>
                    </div>
                    <span className={`shrink-0 font-score text-sm font-medium ${entry.type === 'credit' ? 'text-green-400' : 'text-red-400'}`}>
                      {entry.type === 'credit' ? '+' : '-'}
                      {fmt(entry.amount_cents)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </section>
        </div>
      </div>

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
