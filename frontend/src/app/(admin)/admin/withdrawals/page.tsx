'use client'

import { useState, useMemo } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  RefreshCw,
  CheckCircle,
  XCircle,
  ArrowRight,
  ChevronLeft,
  ChevronRight,
  AlertTriangle,
  X,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { WithdrawalResponse, WithdrawalStatus } from '@/lib/api-types'
import { cn, formatGTQ, formatUSD, formatRelative, formatDateTime } from '@/lib/utils'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'

const PAGE_SIZE = 15

type TabKey = 'all' | WithdrawalStatus
type ActionType = 'approve' | 'reject' | 'process'

interface ActionModal {
  type: ActionType
  item: WithdrawalResponse
}

const TABS: { key: TabKey; label: string }[] = [
  { key: 'all',       label: 'Todos' },
  { key: 'pending',   label: 'Pendientes' },
  { key: 'approved',  label: 'Aprobados' },
  { key: 'rejected',  label: 'Rechazados' },
  { key: 'processed', label: 'Procesados' },
]

const METHOD_LABEL: Record<string, string> = {
  bank_gt: 'Banco GT',
  paypal:  'PayPal',
}

function formatAmount(cents: number, currency: string): string {
  return currency === 'USD' ? formatUSD(cents) : formatGTQ(cents)
}

function payoutSummary(method: string, details: Record<string, string> | null): string {
  if (!details) return '—'
  if (method === 'bank_gt') {
    const bank = details.bank_name ?? ''
    const acct = details.account_number ?? ''
    if (bank && acct) return `${bank} · ${acct}`
    return bank || acct || '—'
  }
  if (method === 'paypal') return details.paypal_email ?? '—'
  return Object.values(details).join(', ')
}

function getPageNumbers(current: number, total: number): (number | '...')[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages = new Set<number>([1, total, current])
  if (current > 1) pages.add(current - 1)
  if (current < total) pages.add(current + 1)
  const sorted = Array.from(pages).sort((a, b) => a - b)
  const result: (number | '...')[] = []
  for (let i = 0; i < sorted.length; i++) {
    if (i > 0 && sorted[i] - sorted[i - 1] > 1) result.push('...')
    result.push(sorted[i])
  }
  return result
}

// ── InfoRow helper ────────────────────────────────────────────────────────────

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex justify-between items-center py-1.5 border-b border-white/5 last:border-0">
      <span className="text-white/50 text-sm">{label}</span>
      <span className="text-white text-sm font-medium">{value}</span>
    </div>
  )
}

// ── Action modal content ──────────────────────────────────────────────────────

interface ModalProps {
  modal: ActionModal
  notes: string
  onNotesChange: (v: string) => void
  error: string
  isBusy: boolean
  onSubmit: () => void
  onClose: () => void
}

function ActionModalContent({ modal, notes, onNotesChange, error, isBusy, onSubmit, onClose }: ModalProps) {
  const { type, item } = modal
  const amountLabel = formatAmount(item.amount_cents, item.currency)
  const destinationLabel = payoutSummary(item.method, item.payout_details)
  const methodLabel = METHOD_LABEL[item.method] ?? item.method

  const config = {
    approve: {
      title:       `Aprobar retiro #${item.id}`,
      btnLabel:    'Aprobar',
      btnClass:    'bg-emerald-600 hover:bg-emerald-500 text-white',
      notesLabel:  'Notas (opcional)',
      notesPlaceholder: 'Observaciones para el equipo...',
      required:    false,
      warning:     'La aprobación deducirá el monto del balance disponible del usuario.',
    },
    reject: {
      title:       `Rechazar retiro #${item.id}`,
      btnLabel:    'Rechazar',
      btnClass:    'bg-red-600 hover:bg-red-500 text-white',
      notesLabel:  'Motivo de rechazo (requerido)',
      notesPlaceholder: 'Ingresa el motivo del rechazo...',
      required:    true,
      warning:     'El saldo reservado será liberado de vuelta al balance del usuario.',
    },
    process: {
      title:       `Confirmar pago #${item.id}`,
      btnLabel:    'Confirmar pago',
      btnClass:    'bg-blue-600 hover:bg-blue-500 text-white',
      notesLabel:  '',
      notesPlaceholder: '',
      required:    false,
      warning:     'Esta acción marca el retiro como pagado. Es irreversible.',
    },
  }[type]

  return (
    <>
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <h2 className="text-lg font-semibold text-white">{config.title}</h2>
        <button onClick={onClose} className="text-white/40 hover:text-white/80 transition-colors shrink-0 mt-0.5">
          <X className="h-5 w-5" />
        </button>
      </div>

      {/* Details */}
      <div className="bg-white/5 rounded-lg px-4 py-1">
        <InfoRow label="Usuario"     value={<span className="font-mono text-xs">#{item.user_id}</span>} />
        <InfoRow label="Monto"       value={amountLabel} />
        <InfoRow label="Método"      value={methodLabel} />
        <InfoRow label="Destino"     value={<span className="font-mono text-xs">{destinationLabel}</span>} />
        {item.notes && type === 'process' && (
          <InfoRow label="Notas admin" value={item.notes} />
        )}
        <InfoRow label="Solicitado"  value={
          <span title={formatDateTime(item.created_at)} className="text-white/70">
            {formatRelative(item.created_at)}
          </span>
        } />
      </div>

      {/* Warning */}
      <div className="flex items-start gap-2 p-3 rounded-lg bg-amber-500/10 border border-amber-500/20">
        <AlertTriangle className="h-4 w-4 text-amber-400 mt-0.5 shrink-0" />
        <p className="text-amber-300 text-xs">{config.warning}</p>
      </div>

      {/* Notes input — approve and reject only */}
      {type !== 'process' && (
        <div className="space-y-1.5">
          <label className="block text-sm font-medium text-white/70">{config.notesLabel}</label>
          <textarea
            rows={3}
            value={notes}
            onChange={e => onNotesChange(e.target.value)}
            placeholder={config.notesPlaceholder}
            className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder:text-white/30 resize-none focus:outline-none focus:ring-1 focus:ring-emerald-500/50"
          />
        </div>
      )}

      {/* Error */}
      {error && (
        <p className="text-red-400 text-sm flex items-center gap-1.5">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          {error}
        </p>
      )}

      {/* Actions */}
      <div className="flex justify-end gap-3">
        <button
          onClick={onClose}
          disabled={isBusy}
          className="px-4 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/70 text-sm font-medium transition-colors disabled:opacity-50"
        >
          Cancelar
        </button>
        <button
          onClick={onSubmit}
          disabled={isBusy || (config.required && !notes.trim())}
          className={cn(
            'px-4 py-2 rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed',
            config.btnClass,
          )}
        >
          {isBusy ? 'Procesando...' : config.btnLabel}
        </button>
      </div>
    </>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function AdminWithdrawalsPage() {
  const { getToken } = useAuth()
  const qc = useQueryClient()

  const [tab, setTab]               = useState<TabKey>('all')
  const [page, setPage]             = useState(1)
  const [actionModal, setActionModal] = useState<ActionModal | null>(null)
  const [notes, setNotes]           = useState('')
  const [actionError, setActionError] = useState('')

  const { data: withdrawals = [], isLoading, error, refetch } = useQuery({
    queryKey: ['admin', 'withdrawals'],
    queryFn: async () => {
      const token = await getToken()
      return api.adminListWithdrawals(token!)
    },
  })

  const filtered = useMemo(() => (
    tab === 'all' ? withdrawals : withdrawals.filter(w => w.status === tab)
  ), [withdrawals, tab])

  const counts = useMemo(() => {
    const result: Record<string, number> = { all: withdrawals.length }
    for (const w of withdrawals) {
      result[w.status] = (result[w.status] ?? 0) + 1
    }
    return result
  }, [withdrawals])

  const pageCount = Math.ceil(filtered.length / PAGE_SIZE)
  const safePage  = Math.min(Math.max(page, 1), Math.max(pageCount, 1))
  const paginated = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)
  const rangeStart = filtered.length === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1
  const rangeEnd   = Math.min(safePage * PAGE_SIZE, filtered.length)

  function changeTab(key: TabKey) {
    setTab(key)
    setPage(1)
  }

  function openAction(type: ActionType, item: WithdrawalResponse) {
    setActionModal({ type, item })
    setNotes('')
    setActionError('')
  }

  function closeModal() {
    setActionModal(null)
    setNotes('')
    setActionError('')
  }

  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin', 'withdrawals'] })

  const approveMutation = useMutation({
    mutationFn: async ({ id, notes }: { id: number; notes: string }) => {
      const token = await getToken()
      return api.adminApproveWithdrawal(token!, id, notes)
    },
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: unknown) => {
      setActionError(e instanceof Error ? e.message : 'Error al aprobar el retiro')
    },
  })

  const rejectMutation = useMutation({
    mutationFn: async ({ id, notes }: { id: number; notes: string }) => {
      const token = await getToken()
      return api.adminRejectWithdrawal(token!, id, notes)
    },
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: unknown) => {
      setActionError(e instanceof Error ? e.message : 'Error al rechazar el retiro')
    },
  })

  const processMutation = useMutation({
    mutationFn: async (id: number) => {
      const token = await getToken()
      return api.adminProcessWithdrawal(token!, id)
    },
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: unknown) => {
      setActionError(e instanceof Error ? e.message : 'Error al procesar el retiro')
    },
  })

  const isBusy = approveMutation.isPending || rejectMutation.isPending || processMutation.isPending

  function submitAction() {
    if (!actionModal) return
    const { type, item } = actionModal
    if (type === 'approve') {
      approveMutation.mutate({ id: item.id, notes })
    } else if (type === 'reject') {
      if (!notes.trim()) { setActionError('El motivo de rechazo es requerido'); return }
      rejectMutation.mutate({ id: item.id, notes })
    } else {
      processMutation.mutate(item.id)
    }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Retiros</h1>
          <p className="text-sm text-white/50 mt-1">{withdrawals.length} solicitudes en total</p>
        </div>
        <button
          onClick={() => refetch()}
          disabled={isLoading}
          className="flex items-center gap-2 px-3 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white text-sm transition-colors disabled:opacity-50"
        >
          <RefreshCw className={cn('h-4 w-4', isLoading && 'animate-spin')} />
          Actualizar
        </button>
      </div>

      {/* Status tabs */}
      <div className="flex flex-wrap gap-2">
        {TABS.map(t => {
          const count = counts[t.key === 'all' ? 'all' : t.key] ?? 0
          const isActive = tab === t.key
          return (
            <button
              key={t.key}
              onClick={() => changeTab(t.key)}
              className={cn(
                'flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-medium transition-colors',
                isActive
                  ? 'bg-emerald-500/20 text-emerald-400 ring-1 ring-emerald-500/40'
                  : 'bg-white/5 text-white/60 hover:bg-white/10 hover:text-white',
              )}
            >
              {t.label}
              <span className={cn(
                'text-xs px-1.5 py-0.5 rounded-full tabular-nums',
                isActive ? 'bg-emerald-500/30 text-emerald-300' : 'bg-white/10 text-white/40',
              )}>
                {count}
              </span>
            </button>
          )
        })}
      </div>

      {/* Content */}
      {isLoading ? (
        <LoadingState />
      ) : error ? (
        <div className="text-center py-12 text-red-400 text-sm">
          Error al cargar los retiros. Intenta actualizar la página.
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-16 text-white/40">
          <p className="text-lg font-medium">No hay retiros</p>
          <p className="text-sm mt-1">
            {tab === 'all'
              ? 'No se han registrado solicitudes aún.'
              : `No hay retiros con estado "${tab}".`}
          </p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto rounded-xl border border-white/10">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/10 bg-white/5 text-white/50 text-left">
                  <th className="px-4 py-3 font-medium">#</th>
                  <th className="px-4 py-3 font-medium">Usuario</th>
                  <th className="px-4 py-3 font-medium">Monto</th>
                  <th className="px-4 py-3 font-medium">Método</th>
                  <th className="px-4 py-3 font-medium">Destino</th>
                  <th className="px-4 py-3 font-medium">Estado</th>
                  <th className="px-4 py-3 font-medium">Solicitado</th>
                  <th className="px-4 py-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {paginated.map(item => (
                  <tr
                    key={item.id}
                    className="border-b border-white/5 hover:bg-white/[0.03] transition-colors"
                  >
                    <td className="px-4 py-3 text-white/50 font-mono text-xs">#{item.id}</td>

                    <td className="px-4 py-3">
                      <span className="text-white/70 font-mono text-xs bg-white/5 px-2 py-0.5 rounded">
                        #{item.user_id}
                      </span>
                    </td>

                    <td className="px-4 py-3 font-semibold text-white tabular-nums">
                      {formatAmount(item.amount_cents, item.currency)}
                    </td>

                    <td className="px-4 py-3 text-white/70">
                      {METHOD_LABEL[item.method] ?? item.method}
                    </td>

                    <td
                      className="px-4 py-3 text-white/60 font-mono text-xs max-w-[200px] truncate"
                      title={payoutSummary(item.method, item.payout_details)}
                    >
                      {payoutSummary(item.method, item.payout_details)}
                    </td>

                    <td className="px-4 py-3">
                      <StatusBadge status={item.status} />
                    </td>

                    <td
                      className="px-4 py-3 text-white/50 text-xs"
                      title={formatDateTime(item.created_at)}
                    >
                      {formatRelative(item.created_at)}
                    </td>

                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        {item.status === 'pending' && (
                          <>
                            <button
                              onClick={() => openAction('approve', item)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-emerald-500/15 hover:bg-emerald-500/25 text-emerald-400 text-xs font-medium transition-colors"
                            >
                              <CheckCircle className="h-3.5 w-3.5" />
                              Aprobar
                            </button>
                            <button
                              onClick={() => openAction('reject', item)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-red-500/15 hover:bg-red-500/25 text-red-400 text-xs font-medium transition-colors"
                            >
                              <XCircle className="h-3.5 w-3.5" />
                              Rechazar
                            </button>
                          </>
                        )}
                        {item.status === 'approved' && (
                          <button
                            onClick={() => openAction('process', item)}
                            className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-blue-500/15 hover:bg-blue-500/25 text-blue-400 text-xs font-medium transition-colors"
                          >
                            <ArrowRight className="h-3.5 w-3.5" />
                            Procesar
                          </button>
                        )}
                        {(item.status === 'rejected' || item.status === 'processed') && item.notes && (
                          <span
                            className="text-white/30 text-xs italic max-w-[120px] truncate"
                            title={item.notes}
                          >
                            {item.notes}
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {pageCount > 1 && (
            <div className="flex items-center justify-between text-sm">
              <span className="text-white/40">
                {rangeStart}–{rangeEnd} de {filtered.length} retiros
              </span>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={safePage === 1}
                  className="p-1.5 rounded-md hover:bg-white/10 text-white/60 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                >
                  <ChevronLeft className="h-4 w-4" />
                </button>
                {getPageNumbers(safePage, pageCount).map((n, i) =>
                  n === '...' ? (
                    <span key={`e${i}`} className="px-2 text-white/30">…</span>
                  ) : (
                    <button
                      key={n}
                      onClick={() => setPage(n as number)}
                      aria-current={n === safePage ? 'page' : undefined}
                      className={cn(
                        'w-8 h-8 rounded-md text-sm font-medium transition-colors',
                        n === safePage
                          ? 'bg-emerald-500/20 text-emerald-400 ring-1 ring-emerald-500/40'
                          : 'hover:bg-white/10 text-white/60',
                      )}
                    >
                      {n}
                    </button>
                  ),
                )}
                <button
                  onClick={() => setPage(p => Math.min(pageCount, p + 1))}
                  disabled={safePage === pageCount}
                  className="p-1.5 rounded-md hover:bg-white/10 text-white/60 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                >
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            </div>
          )}
        </>
      )}

      {/* Action Modal */}
      {actionModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={closeModal}
          />
          <div className="relative z-10 w-full max-w-md bg-[#1a1a2e] border border-white/10 rounded-xl shadow-2xl p-6 space-y-5">
            <ActionModalContent
              modal={actionModal}
              notes={notes}
              onNotesChange={setNotes}
              error={actionError}
              isBusy={isBusy}
              onSubmit={submitAction}
              onClose={closeModal}
            />
          </div>
        </div>
      )}
    </div>
  )
}
