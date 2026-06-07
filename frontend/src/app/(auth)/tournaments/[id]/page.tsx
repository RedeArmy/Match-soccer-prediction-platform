'use client'

import { useState } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Check, Copy, Crown, Loader2, LogOut, Medal, Trophy, UserCheck, UserX, Users, X } from 'lucide-react'
import Link from 'next/link'
import { useParams, useRouter } from 'next/navigation'
import { api } from '@/lib/api'
import type { MemberResponse } from '@/lib/api-types'
import { cn } from '@/lib/utils'
import { useCurrency } from '@/hooks/useCurrency'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { useI18n } from '@/lib/i18n'

export default function TournamentDetailPage() {
  const { getToken } = useAuth()
  const params = useParams<{ id: string }>()
  const router = useRouter()
  const queryClient = useQueryClient()
  const { t } = useI18n()
  const { fmt } = useCurrency()
  const groupId = Number(params.id)

  const [copied, setCopied] = useState(false)
  const [confirmLeave, setConfirmLeave] = useState(false)
  const [leaveError, setLeaveError] = useState<string | null>(null)
  const [approvingMember, setApprovingMember] = useState<MemberResponse | null>(null)

  const groupQuery = useQuery({
    queryKey: ['group', groupId],
    queryFn: async () => {
      const token = await getToken()
      return api.getGroup(token!, groupId)
    },
    enabled: !Number.isNaN(groupId),
  })

  const leaderboardQuery = useQuery({
    queryKey: ['group-leaderboard', groupId],
    queryFn: async () => {
      const token = await getToken()
      return api.getGroupLeaderboard(token!, groupId)
    },
    enabled: !Number.isNaN(groupId),
  })

  const membersQuery = useQuery({
    queryKey: ['group-members', groupId],
    queryFn: async () => {
      const token = await getToken()
      return api.getGroupMembers(token!, groupId)
    },
    enabled: !Number.isNaN(groupId),
  })

  const leaveMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken()
      return api.leaveGroup(token!, groupId)
    },
    onSuccess: () => {
      // Remove stale cache so the dashboard never shows the left group
      queryClient.removeQueries({ queryKey: ['my-groups'] })
      queryClient.removeQueries({ queryKey: ['group', groupId] })
      router.replace('/dashboard')
    },
    onError: (err: unknown) => {
      const msg = (err as { message?: string })?.message ?? 'Error al salir del grupo'
      setLeaveError(msg)
    },
  })

  const approveMutation = useMutation({
    mutationFn: async (membershipId: number) => {
      const token = await getToken()
      return api.approveGroupMember(token!, groupId, membershipId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group-members', groupId] })
      setApprovingMember(null)
    },
  })

  const rejectMutation = useMutation({
    mutationFn: async (membershipId: number) => {
      const token = await getToken()
      return api.rejectGroupMember(token!, groupId, membershipId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group-members', groupId] })
      setApprovingMember(null)
    },
  })

  async function copyCode(code: string) {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const group = groupQuery.data
  const entries = leaderboardQuery.data?.data ?? []
  const members = membersQuery.data ?? []
  const activeMembers = members.filter((m) => m.status === 'active')
  const pendingMembers = members.filter((m) => m.status === 'pending')
  const isLoading = groupQuery.isLoading

  if (!Number.isNaN(groupId) && groupQuery.isError) {
    return (
      <div className="py-20 text-center">
        <p className="text-text-muted">{t('common.notFound')}</p>
        <Link href="/dashboard" className="btn-gold mt-4 px-4 py-2 text-sm">
          {t('common.back')}
        </Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link href="/dashboard" className="rounded p-1 text-text-muted transition-colors hover:text-white">
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <div className="min-w-0">
          {isLoading ? (
            <div className="h-6 w-48 animate-pulse rounded bg-white/10" />
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-xl font-semibold text-white">{group?.name}</h1>
              {group?.status && <StatusBadge status={group.status} size="sm" />}
            </div>
          )}

        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {/* Invite code card */}
        {group?.invite_code && (
          <div className="card p-4">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
              {t('groups.inviteCodeLabel')}
            </p>
            <div className="flex items-center gap-2">
              <span className="flex-1 font-mono text-lg font-bold tracking-widest text-gold-300">
                {group.invite_code}
              </span>
              <button
                type="button"
                onClick={() => copyCode(group.invite_code)}
                className="rounded p-1 text-text-muted transition-colors hover:text-white"
              >
                {copied
                  ? <Check className="h-4 w-4 text-green-400" />
                  : <Copy className="h-4 w-4" />}
              </button>
            </div>
            <p className="mt-1 text-[11px] text-text-muted">{t('groups.inviteCodeHint')}</p>
          </div>
        )}

        {/* Members count */}
        <div className="card flex items-center gap-3 p-4">
          <Users className="h-8 w-8 shrink-0 text-blue-300" />
          <div className="min-w-0 flex-1">
            <div className="flex items-baseline gap-2">
              <p className="text-2xl font-bold text-white">{activeMembers.length}</p>
              {pendingMembers.length > 0 && (
                <span className="text-sm font-medium text-yellow-400">
                  +{pendingMembers.length}
                </span>
              )}
            </div>
            <p className="text-xs text-text-muted">{t('group.membersActive')}</p>
            {pendingMembers.length > 0 && (
              <p className="text-[10px] text-yellow-400/70">{t('group.membersPending')}</p>
            )}
          </div>
        </div>

        {/* Entry fee */}
        <div className="card flex items-center gap-3 p-4">
          <Trophy className="h-8 w-8 shrink-0 text-gold-400" />
          <div>
            <p className="text-2xl font-bold text-white" suppressHydrationWarning>
              {group ? fmt(group.entry_fee) : '—'}
            </p>
            <p className="text-xs text-text-muted">{t('groups.entryFeeLabel')}</p>
          </div>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
        {/* Leaderboard */}
        <section className="panel p-4 sm:p-5">
          <div className="mb-4 flex items-center gap-2">
            <Medal className="h-5 w-5 text-gold-300" />
            <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
              {t('group.leaderboard')}
            </h2>
          </div>

          {leaderboardQuery.isLoading && <LoadingState rows={5} />}

          {!leaderboardQuery.isLoading && entries.length === 0 && (
            <p className="py-8 text-center text-sm text-text-muted">{t('group.noScores')}</p>
          )}

          {entries.length > 0 && (
            <div className="space-y-1">
              {entries.map((entry) => (
                <div
                  key={entry.user_id}
                  className={cn(
                    'flex items-center gap-3 rounded-lg px-3 py-2.5',
                    entry.is_current
                      ? 'bg-gold-400/10 ring-1 ring-gold-400/30'
                      : 'hover:bg-white/[0.03]',
                  )}
                >
                  <RankBadge rank={entry.rank} />
                  <span className={cn('flex-1 truncate text-sm', entry.is_current ? 'font-semibold text-gold-200' : 'text-text-primary')}>
                    {entry.display_name}
                    {entry.is_current && (
                      <span className="ml-1.5 text-[10px] uppercase text-gold-400">({t('common.you')})</span>
                    )}
                  </span>
                  <span className="shrink-0 font-score text-sm font-medium text-white">
                    {entry.points} pts
                  </span>
                  {entry.prize_cents > 0 && (
                    <span className="shrink-0 text-xs text-green-300" suppressHydrationWarning>
                      {fmt(entry.prize_cents)}
                    </span>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>

        {/* Members list */}
        <section className="panel p-4 sm:p-5">
          <div className="mb-4 flex items-center gap-2">
            <Users className="h-5 w-5 text-blue-300" />
            <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
              {t('group.members')}
            </h2>
          </div>

          {membersQuery.isLoading && <LoadingState rows={3} />}

          {!membersQuery.isLoading && activeMembers.length === 0 && pendingMembers.length === 0 && (
            <p className="py-4 text-center text-sm text-text-muted">{t('group.noMembers')}</p>
          )}

          {/* Active members */}
          {activeMembers.length > 0 && (
            <div className="space-y-1">
              {activeMembers.map((member) => (
                <div key={member.id} className="flex items-center gap-2 rounded-lg px-2 py-2">
                  {member.role === 'owner' && (
                    <Crown className="h-3.5 w-3.5 shrink-0 text-gold-400" />
                  )}
                  <span className={cn('flex-1 truncate text-sm', member.role === 'owner' ? 'text-gold-200' : 'text-text-primary')}>
                    {member.display_name}
                  </span>
                </div>
              ))}
            </div>
          )}

          {/* Pending members */}
          {pendingMembers.length > 0 && (
            <div className="mt-3">
              <p className="mb-1.5 px-2 text-[10px] font-semibold uppercase tracking-wide text-yellow-400/70">
                {t('group.membersPending')}
              </p>
              <div className="space-y-1">
                {pendingMembers.map((member) => (
                  <div key={member.id} className="flex items-center gap-2 rounded-lg px-2 py-2">
                    <span className="flex-1 truncate text-sm text-text-secondary">
                      {member.display_name}
                    </span>
                    <button
                      type="button"
                      onClick={() => setApprovingMember(member)}
                      className="rounded-full border border-yellow-400/30 bg-yellow-400/10 px-2 py-0.5 text-[9px] font-semibold uppercase text-yellow-300 transition-colors hover:border-yellow-400/60 hover:bg-yellow-400/20"
                    >
                      {t('groups.pendingBadge')}
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}


          <div className="mt-6 border-t border-white/10 pt-4">
            {confirmLeave ? (
              <div className="rounded-xl border border-red-400/20 bg-red-400/5 p-4 space-y-3">
                <div className="flex items-start gap-2">
                  <LogOut className="mt-0.5 h-4 w-4 shrink-0 text-red-400" />
                  <div>
                    <p className="text-sm font-medium text-red-300">{t('group.leaveConfirm')}</p>
                    <p className="mt-0.5 text-[11px] text-text-muted">{t('group.leaveConfirmDetail')}</p>
                  </div>
                </div>
                {leaveError && (
                  <p className="rounded-lg bg-red-500/10 px-3 py-2 text-[11px] text-red-400 border border-red-400/20">
                    {leaveError}
                  </p>
                )}
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => { setConfirmLeave(false); setLeaveError(null) }}
                    className="flex-1 rounded-lg border border-white/10 px-3 py-2 text-xs text-text-muted hover:text-white"
                  >
                    {t('common.cancel')}
                  </button>
                  <button
                    type="button"
                    onClick={() => { setLeaveError(null); leaveMutation.mutate() }}
                    disabled={leaveMutation.isPending}
                    className="flex-1 rounded-lg bg-red-500/80 px-3 py-2 text-xs font-medium text-white hover:bg-red-500 disabled:opacity-50"
                  >
                    {leaveMutation.isPending ? t('common.saving') : t('group.leaveConfirmBtn')}
                  </button>
                </div>
              </div>
            ) : (
              <button
                type="button"
                onClick={() => setConfirmLeave(true)}
                className="flex w-full items-center justify-center gap-2 rounded-lg border border-red-400/20 px-3 py-2 text-xs text-red-400 transition-colors hover:border-red-400/40 hover:bg-red-400/10"
              >
                <LogOut className="h-3.5 w-3.5" />
                {t('group.leave')}
              </button>
            )}
          </div>
        </section>
      </div>

      {/* Single-member approval popup */}
      {approvingMember && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div
            role="button"
            tabIndex={0}
            aria-label="Close"
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setApprovingMember(null)}
            onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') setApprovingMember(null) }}
          />
          <div className="relative w-full max-w-sm rounded-2xl border border-white/10 bg-[#0b1929] p-6 shadow-2xl">
            <div className="mb-5 relative text-center">
              <button
                type="button"
                onClick={() => setApprovingMember(null)}
                className="absolute right-0 top-0 rounded p-1 text-text-muted transition-colors hover:text-white"
              >
                <X className="h-5 w-5" />
              </button>
              <p className="text-xs font-semibold uppercase tracking-wide text-gold-300">
                {t('groups.pendingTitle')}
              </p>
              <h2 className="mt-0.5 text-lg font-semibold text-white">{approvingMember.display_name}</h2>
            </div>

            <div className="flex gap-3">
              <button
                type="button"
                disabled={approveMutation.isPending || rejectMutation.isPending}
                onClick={() => approveMutation.mutate(approvingMember.id)}
                className="flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-green-500/80 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-green-500 disabled:opacity-50"
              >
                {approveMutation.isPending
                  ? <Loader2 className="h-4 w-4 animate-spin" />
                  : <UserCheck className="h-4 w-4" />}
                {t('groups.approve')}
              </button>
              <button
                type="button"
                disabled={approveMutation.isPending || rejectMutation.isPending}
                onClick={() => rejectMutation.mutate(approvingMember.id)}
                className="flex flex-1 items-center justify-center gap-1.5 rounded-lg border border-red-400/30 px-4 py-2.5 text-sm font-medium text-red-400 transition-colors hover:border-red-400/60 hover:bg-red-400/10 disabled:opacity-50"
              >
                {rejectMutation.isPending
                  ? <Loader2 className="h-4 w-4 animate-spin" />
                  : <UserX className="h-4 w-4" />}
                {t('groups.reject')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function RankBadge({ rank }: Readonly<{ rank: number }>) {
  if (rank === 1) return <span className="text-base">🥇</span>
  if (rank === 2) return <span className="text-base">🥈</span>
  if (rank === 3) return <span className="text-base">🥉</span>
  return (
    <span className="w-5 text-center text-xs font-bold text-text-muted">
      {rank}
    </span>
  )
}
