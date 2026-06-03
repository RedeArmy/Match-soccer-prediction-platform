'use client'

import { useState } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { formatDate } from '@/lib/utils'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { LoadingState, LoadingSpinner } from '@/components/shared/LoadingState'
import { CheckCircle, XCircle } from 'lucide-react'

const statuses = ['', 'submitted', 'under_review', 'approved', 'rejected']

export default function AdminKYCQueuePage() {
  const { getToken } = useAuth()
  const qc = useQueryClient()
  const [filterStatus, setFilterStatus] = useState('')
  const [rejectTarget, setRejectTarget] = useState<number | null>(null)
  const [rejectReason, setRejectReason] = useState('')

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteQuery({
    queryKey: ['admin-kyc-queue', filterStatus],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const token = await getToken()
      return api.adminGetKYCQueue(token!, pageParam, filterStatus || undefined)
    },
    getNextPageParam: page => page.has_more ? page.next_cursor : undefined,
  })

  const approve = useMutation({
    mutationFn: async (profileID: number) => {
      const token = await getToken()
      return api.adminApproveKYC(token!, profileID)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-kyc-queue'] }),
  })

  const reject = useMutation({
    mutationFn: async ({ profileID, reason }: { profileID: number; reason: string }) => {
      const token = await getToken()
      return api.adminRejectKYC(token!, profileID, reason)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-kyc-queue'] })
      setRejectTarget(null)
      setRejectReason('')
    },
  })

  const profiles = data?.pages.flatMap(p => p.data) ?? []

  return (
    <div className="space-y-5">
      <h1 className="font-display text-3xl text-white">KYC QUEUE</h1>

      {/* Filter */}
      <div className="flex gap-1 p-1 bg-blue-900 rounded-xl w-fit">
        {statuses.map(s => (
          <button
            key={s}
            onClick={() => setFilterStatus(s)}
            className={`px-3 py-1.5 rounded-lg text-xs transition-colors ${
              filterStatus === s ? 'bg-blue-700 text-white font-medium' : 'text-text-muted hover:text-text-secondary'
            }`}
          >
            {s || 'Todos'}
          </button>
        ))}
      </div>

      {isLoading ? (
        <LoadingState rows={6} />
      ) : (
        <div className="card divide-y divide-blue-800/50">
          {!profiles.length && (
            <p className="text-text-muted text-sm text-center py-8">No hay perfiles en esta cola</p>
          )}
          {profiles.map(p => (
            <div key={p.id} className="flex items-center justify-between gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <p className="text-sm text-text-primary font-medium">{p.full_name ?? `Perfil #${p.id}`}</p>
                <div className="flex gap-3 text-xs text-text-muted mt-0.5">
                  {p.submitted_at && <span>{formatDate(p.submitted_at)}</span>}
                  <span>Tier {p.tier}</span>
                  {p.risk_score !== null && <span>Riesgo: {p.risk_score}</span>}
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <StatusBadge status={p.status} size="sm" />
                {(p.status === 'submitted' || p.status === 'under_review') && (
                  <>
                    <button
                      onClick={() => approve.mutate(p.id)}
                      disabled={approve.isPending}
                      className="p-1.5 rounded hover:bg-green-400/10 text-green-400 transition-colors"
                      title="Aprobar"
                    >
                      <CheckCircle className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => { setRejectTarget(p.id); setRejectReason('') }}
                      className="p-1.5 rounded hover:bg-red-400/10 text-red-400 transition-colors"
                      title="Rechazar"
                    >
                      <XCircle className="w-4 h-4" />
                    </button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {hasNextPage && (
        <button
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="btn-ghost w-full text-sm py-2 flex items-center justify-center gap-2"
        >
          {isFetchingNextPage ? <LoadingSpinner size={16} /> : 'Cargar más'}
        </button>
      )}

      {/* Reject dialog */}
      {rejectTarget !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-blue-950/80 backdrop-blur-sm px-4">
          <div className="card p-6 w-full max-w-sm space-y-4">
            <h3 className="font-semibold text-white">Rechazar perfil KYC</h3>
            <textarea
              value={rejectReason}
              onChange={e => setRejectReason(e.target.value)}
              placeholder="Motivo de rechazo..."
              rows={3}
              className="input-base resize-none"
            />
            <div className="flex gap-2">
              <button
                onClick={() => setRejectTarget(null)}
                className="btn-ghost flex-1 text-sm py-2"
              >
                Cancelar
              </button>
              <button
                onClick={() => reject.mutate({ profileID: rejectTarget, reason: rejectReason })}
                disabled={reject.isPending || !rejectReason.trim()}
                className="flex-1 text-sm py-2 bg-red-500 hover:bg-red-400 text-white rounded-lg transition-colors flex items-center justify-center gap-1"
              >
                {reject.isPending ? <LoadingSpinner size={16} /> : 'Rechazar'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
