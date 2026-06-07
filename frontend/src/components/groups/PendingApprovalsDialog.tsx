'use client'

import { useAuth } from '@clerk/nextjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Loader2, UserX, X } from 'lucide-react'
import { api } from '@/lib/api'
import type { MemberResponse } from '@/lib/api-types'
import { useI18n } from '@/lib/i18n'

interface Props {
  groupId: number
  groupName: string
  open: boolean
  onClose: () => void
}

export function PendingApprovalsDialog({ groupId, groupName, open, onClose }: Readonly<Props>) {
  const { getToken } = useAuth()
  const queryClient = useQueryClient()
  const { t } = useI18n()

  const membersQuery = useQuery({
    queryKey: ['group-members', groupId],
    queryFn: async () => {
      const token = await getToken()
      return api.getGroupMembers(token!, groupId)
    },
    enabled: open,
  })

  const pendingMembers = (membersQuery.data ?? []).filter((m) => m.status === 'pending')

  const approveMutation = useMutation({
    mutationFn: async (membershipId: number) => {
      const token = await getToken()
      return api.approveGroupMember(token!, groupId, membershipId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group-members', groupId] })
      queryClient.invalidateQueries({ queryKey: ['my-groups'] })
    },
  })

  const rejectMutation = useMutation({
    mutationFn: async (membershipId: number) => {
      const token = await getToken()
      return api.rejectGroupMember(token!, groupId, membershipId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group-members', groupId] })
      queryClient.invalidateQueries({ queryKey: ['my-groups'] })
    },
  })

  if (!open) return null

  const busy = (id: number) =>
    (approveMutation.isPending && approveMutation.variables === id) ||
    (rejectMutation.isPending && rejectMutation.variables === id)

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        role="button"
        tabIndex={0}
        aria-label="Close"
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onClose() }}
      />

      <div className="relative w-full max-w-md rounded-2xl border border-white/10 bg-[#0b1929] p-6 shadow-2xl">
        <div className="mb-5 flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-gold-300">
              {t('groups.pendingTitle')}
            </p>
            <h2 className="mt-0.5 text-lg font-semibold text-white">{groupName}</h2>
          </div>
          <button
            type="button"
            aria-label="Cerrar"
            onClick={onClose}
            className="mt-0.5 rounded p-1 text-text-muted transition-colors hover:text-white"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {membersQuery.isLoading && (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-text-muted" />
          </div>
        )}

        {!membersQuery.isLoading && pendingMembers.length === 0 && (
          <p className="py-8 text-center text-sm text-text-muted">
            {t('groups.noPending')}
          </p>
        )}

        {pendingMembers.length > 0 && (
          <ul className="space-y-2">
            {pendingMembers.map((m: MemberResponse) => (
              <li
                key={m.id}
                className="flex items-center gap-3 rounded-xl border border-white/10 bg-white/[0.03] px-4 py-3"
              >
                <span className="flex-1 truncate text-sm text-text-primary">{m.display_name}</span>

                <button
                  type="button"
                  disabled={busy(m.id)}
                  onClick={() => approveMutation.mutate(m.id)}
                  className="flex items-center gap-1 rounded-lg bg-green-500/80 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-green-500 disabled:opacity-50"
                >
                  {approveMutation.isPending && approveMutation.variables === m.id
                    ? <Loader2 className="h-3 w-3 animate-spin" />
                    : <Check className="h-3 w-3" />}
                  {t('groups.approve')}
                </button>

                <button
                  type="button"
                  disabled={busy(m.id)}
                  onClick={() => rejectMutation.mutate(m.id)}
                  className="flex items-center gap-1 rounded-lg border border-red-400/30 px-3 py-1.5 text-xs font-medium text-red-400 transition-colors hover:border-red-400/60 hover:bg-red-400/10 disabled:opacity-50"
                >
                  {rejectMutation.isPending && rejectMutation.variables === m.id
                    ? <Loader2 className="h-3 w-3 animate-spin" />
                    : <UserX className="h-3 w-3" />}
                  {t('groups.reject')}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
