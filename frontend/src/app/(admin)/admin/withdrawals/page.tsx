'use client'

import { useState, useMemo } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle, XCircle, ArrowRight, AlertTriangle } from 'lucide-react'
import { api } from '@/lib/api'
import type { WithdrawalResponse, WithdrawalStatus } from '@/lib/api-types'
import { cn, formatGTQ, formatUSD, formatRelative, formatDateTime } from '@/lib/utils'
import { StatusBadge } from '@/components/shared/StatusBadge'
import {
  AdminPageHeader, AdminModalOverlay, AdminPagination, AdminContentState,
  ModalHeader, ModalCancelButton, ModalErrorLine, InfoRow,
} from '@/components/admin/shared'

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

// ── Action modal content ──────────────────────────────────────────────────────

interface ModalProps {
  readonly modal: ActionModal
  readonly notes: string
  readonly onNotesChange: (v: string) => void
  readonly error: string
  readonly isBusy: boolean
  readonly onSubmit: () => void
  readonly onClose: () => void
}

function ActionModalContent({ modal, notes, onNotesChange, error, isBusy, onSubmit, onClose }: ModalProps) {
  const { type, item } = modal
  const amountLabel = formatAmount(item.amount_cents, item.currency)
  const destinationLabel = payoutSummary(item.method, item.payout_details)
  const methodLabel = METHOD_LABEL[item.method] ?? item.method

  const config = {
    approve: {
      title:            `Aprobar retiro #${item.id}`,
      btnLabel:         'Aprobar',
      btnClass:         'bg-emerald-600 hover:bg-emerald-500 text-white',
      notesLabel:       'Notas (opcional)',
      notesPlaceholder: 'Observaciones para el equipo...',
      required:         false,
      warning:          'La aprobación deducirá el monto del balance disponible del usuario.',
    },
    reject: {
      title:            `Rechazar retiro #${item.id}`,
      btnLabel:         'Rechazar',
      btnClass:         'bg-red-600 hover:bg-red-500 text-white',
      notesLabel:       'Motivo de rechazo (requerido)',
      notesPlaceholder: 'Ingresa el motivo del rechazo...',
      required:         true,
      warning:          'El saldo reservado será liberado de vuelta al balance del usuario.',
    },
    process: {
      title:            `Confirmar pago #${item.id}`,
      btnLabel:         'Confirmar pago',
      btnClass:         'bg-blue-600 hover:bg-blue-500 text-white',
      notesLabel:       '',
      notesPlaceholder: '',
      required:         false,
      warning:          'Esta acción marca el retiro como pagado. Es irreversible.',
    },
  }[type]

  return (
    <>
      <ModalHeader title={config.title} onClose={onClose} />

      <div className="bg-white/5 rounded-lg px-4 py-1">
        <InfoRow label="Usuario"    value={<span className="font-mono text-xs">#{item.user_id}</span>} />
        <InfoRow label="Monto"      value={amountLabel} />
        <InfoRow label="Método"     value={methodLabel} />
        <InfoRow label="Destino"    value={<span className="font-mono text-xs">{destinationLabel}</span>} />
        {item.notes && type === 'process' && (
          <InfoRow label="Notas admin" value={item.notes} />
        )}
        <InfoRow label="Solicitado" value={
          <span title={formatDateTime(item.created_at)} className="text-white/70">
            {formatRelative(item.created_at)}
          </span>
        } />
      </div>

      <div className="flex items-start gap-2 p-3 rounded-lg bg-amber-500/10 border border-amber-500/20">
        <AlertTriangle className="h-4 w-4 text-amber-400 mt-0.5 shrink-0" />
        <p className="text-amber-300 text-xs">{config.warning}</p>
      </div>

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

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
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

  const [tab, setTab]                 = useState<TabKey>('all')
  const [page, setPage]               = useState(1)
  const [actionModal, setActionModal] = useState<ActionModal | null>(null)
  const [notes, setNotes]             = useState('')
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
    for (const w of withdrawals) result[w.status] = (result[w.status] ?? 0) + 1
    return result
  }, [withdrawals])

  const pageCount  = Math.ceil(filtered.length / PAGE_SIZE)
  const safePage   = Math.min(Math.max(page, 1), Math.max(pageCount, 1))
  const paginated  = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)
  const rangeStart = filtered.length === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1
  const rangeEnd   = Math.min(safePage * PAGE_SIZE, filtered.length)

  function changeTab(key: TabKey) { setTab(key); setPage(1) }

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

  const emptyMsg = tab === 'all'
    ? 'No se han registrado solicitudes aún.'
    : `No hay retiros con estado "${tab}".`

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
      <AdminPageHeader
        title="Retiros"
        subtitle={`${withdrawals.length} solicitudes en total`}
        onRefresh={refetch}
        isLoading={isLoading}
      />

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
      <AdminContentState
        isLoading={isLoading}
        error={error}
        isEmpty={filtered.length === 0}
        emptyTitle="No hay retiros"
        emptyMessage={emptyMsg}
        errorMessage="Error al cargar los retiros. Intenta actualizar la página."
      >
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

          <AdminPagination
            page={safePage}
            pageCount={pageCount}
            rangeStart={rangeStart}
            rangeEnd={rangeEnd}
            total={filtered.length}
            itemLabel="retiros"
            onPageChange={setPage}
          />
        </>
      </AdminContentState>

      {actionModal && (
        <AdminModalOverlay onClose={closeModal}>
          <ActionModalContent
            modal={actionModal}
            notes={notes}
            onNotesChange={setNotes}
            error={actionError}
            isBusy={isBusy}
            onSubmit={submitAction}
            onClose={closeModal}
          />
        </AdminModalOverlay>
      )}
    </div>
  )
}
