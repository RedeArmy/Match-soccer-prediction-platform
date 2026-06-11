'use client'

import { useState, useMemo } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle, XCircle, ExternalLink, AlertTriangle } from 'lucide-react'
import { api } from '@/lib/api'
import type { BankTransferResponse, BankTransferStatus } from '@/lib/api-types'
import { cn, formatGTQ, formatDateTime, formatRelative } from '@/lib/utils'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import {
  AdminPageHeader, AdminModalOverlay, AdminPagination,
  ModalHeader, ModalCancelButton, ModalErrorLine, InfoRow,
} from '@/components/admin/shared'

const PAGE_SIZE = 15

type TabKey = 'all' | BankTransferStatus

const TABS: { key: TabKey; label: string }[] = [
  { key: 'pending',  label: 'Pendientes' },
  { key: 'all',      label: 'Todos' },
  { key: 'approved', label: 'Aprobados' },
  { key: 'rejected', label: 'Rechazados' },
]

function centsToQ(cents: number): string {
  return formatGTQ(cents)
}

function proofDownloadUrl(id: number): string {
  return `/api/v1/admin/bank-transfers/${id}/download`
}

// ── Approve modal ─────────────────────────────────────────────────────────────

interface ApproveModalProps {
  readonly item: BankTransferResponse
  readonly overrideRaw: string
  readonly notes: string
  readonly onOverrideChange: (v: string) => void
  readonly onNotesChange: (v: string) => void
  readonly isBusy: boolean
  readonly error: string
  readonly onSubmit: () => void
  readonly onClose: () => void
}

function ApproveModal({
  item, overrideRaw, notes, onOverrideChange, onNotesChange,
  isBusy, error, onSubmit, onClose,
}: ApproveModalProps) {
  const overrideQ = Number.parseFloat(overrideRaw)
  const hasOverride = overrideRaw.trim() !== ''
  const overrideCents = hasOverride ? Math.round(overrideQ * 100) : null
  const overrideValid = !hasOverride || (!Number.isNaN(overrideQ) && overrideQ > 0)
  const amountDiffers = overrideCents !== null && overrideCents !== item.amount_cents

  return (
    <>
      <ModalHeader title={`Aprobar Transferencia #${item.id}`} onClose={onClose} />

      <div className="bg-white/5 rounded-lg px-4 py-1">
        <InfoRow label="Usuario"          value={<span className="font-mono text-xs">#{item.user_id}</span>} />
        <InfoRow label="Monto declarado"  value={<span className="font-semibold">{centsToQ(item.amount_cents)}</span>} />
        <InfoRow label="Moneda"           value={item.currency} />
        <InfoRow label="Enviado"          value={
          <span title={formatDateTime(item.created_at)}>{formatRelative(item.created_at)}</span>
        } />
        <InfoRow label="Comprobante" value={
          <a
            href={proofDownloadUrl(item.id)}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1 text-blue-400 hover:text-blue-300 transition-colors text-xs"
          >
            Ver archivo <ExternalLink className="h-3 w-3" />
          </a>
        } />
      </div>

      <div className="space-y-1.5">
        <label htmlFor="approve-override" className="block text-sm font-medium text-white/70">
          Monto a acreditar (Q){' '}
          <span className="ml-1.5 text-white/30 font-normal text-xs">— dejar vacío para usar el monto declarado</span>
        </label>
        <div className="relative">
          <span className="absolute left-3 top-1/2 -translate-y-1/2 text-white/40 text-sm">Q</span>
          <input
            id="approve-override"
            type="number"
            min={0.01}
            step={0.01}
            value={overrideRaw}
            onChange={e => onOverrideChange(e.target.value)}
            placeholder={String((item.amount_cents / 100).toFixed(2))}
            className="w-full bg-white/5 border border-white/10 rounded-lg pl-8 pr-3 py-2 text-sm text-white placeholder:text-white/25 focus:outline-none focus:ring-1 focus:ring-emerald-500/50"
          />
        </div>
        {hasOverride && !overrideValid && (
          <p className="text-red-400 text-xs">El monto debe ser positivo.</p>
        )}
        {amountDiffers && overrideValid && overrideCents !== null && (
          <div className="flex items-start gap-2 p-2.5 rounded-lg bg-amber-500/10 border border-amber-500/20">
            <AlertTriangle className="h-3.5 w-3.5 text-amber-400 mt-0.5 shrink-0" />
            <p className="text-amber-300 text-xs">
              El monto a acreditar ({centsToQ(overrideCents)}) difiere del monto declarado por el usuario ({centsToQ(item.amount_cents)}).
            </p>
          </div>
        )}
      </div>

      <div className="space-y-1.5">
        <label htmlFor="approve-notes" className="block text-sm font-medium text-white/70">Notas (opcional)</label>
        <textarea
          id="approve-notes"
          rows={2}
          value={notes}
          onChange={e => onNotesChange(e.target.value)}
          placeholder="Observaciones para el registro..."
          className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder:text-white/30 resize-none focus:outline-none focus:ring-1 focus:ring-emerald-500/50"
        />
      </div>

      <div className="flex items-start gap-2 p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20">
        <CheckCircle className="h-4 w-4 text-emerald-400 mt-0.5 shrink-0" />
        <p className="text-emerald-300 text-xs">
          Se acreditará <strong>{hasOverride && overrideValid && overrideCents ? centsToQ(overrideCents) : centsToQ(item.amount_cents)}</strong> al balance del usuario. Esta acción es irreversible.
        </p>
      </div>

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={onSubmit}
          disabled={isBusy || !overrideValid}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <CheckCircle className="h-4 w-4" />
          {isBusy ? 'Procesando...' : 'Aprobar y Acreditar'}
        </button>
      </div>
    </>
  )
}

// ── Reject modal ──────────────────────────────────────────────────────────────

interface RejectModalProps {
  readonly item: BankTransferResponse
  readonly notes: string
  readonly onNotesChange: (v: string) => void
  readonly isBusy: boolean
  readonly error: string
  readonly onSubmit: () => void
  readonly onClose: () => void
}

function RejectModal({ item, notes, onNotesChange, isBusy, error, onSubmit, onClose }: RejectModalProps) {
  return (
    <>
      <ModalHeader title={`Rechazar Transferencia #${item.id}`} onClose={onClose} />

      <div className="bg-white/5 rounded-lg px-4 py-1">
        <InfoRow label="Usuario"     value={<span className="font-mono text-xs">#{item.user_id}</span>} />
        <InfoRow label="Monto"       value={centsToQ(item.amount_cents)} />
        <InfoRow label="Enviado"     value={
          <span title={formatDateTime(item.created_at)}>{formatRelative(item.created_at)}</span>
        } />
        <InfoRow label="Comprobante" value={
          <a
            href={proofDownloadUrl(item.id)}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1 text-blue-400 hover:text-blue-300 transition-colors text-xs"
          >
            Ver archivo <ExternalLink className="h-3 w-3" />
          </a>
        } />
      </div>

      <div className="space-y-1.5">
        <label htmlFor="reject-notes" className="block text-sm font-medium text-white/70">
          Motivo de rechazo <span className="text-red-400">*</span>
        </label>
        <textarea
          id="reject-notes"
          rows={3}
          value={notes}
          onChange={e => onNotesChange(e.target.value)}
          placeholder="Describe el motivo del rechazo (comprobante ilegible, monto incorrecto, etc.)..."
          className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder:text-white/30 resize-none focus:outline-none focus:ring-1 focus:ring-red-500/50"
        />
      </div>

      <div className="flex items-start gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/20">
        <XCircle className="h-4 w-4 text-red-400 mt-0.5 shrink-0" />
        <p className="text-red-300 text-xs">
          El pago será marcado como rechazado. No se acreditará ningún monto al usuario.
        </p>
      </div>

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={onSubmit}
          disabled={isBusy || !notes.trim()}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-red-600 hover:bg-red-500 text-white text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <XCircle className="h-4 w-4" />
          {isBusy ? 'Rechazando...' : 'Rechazar Transferencia'}
        </button>
      </div>
    </>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────

const ACCENT_BY_STATUS: Record<string, string> = {
  pending:  'amber',
  approved: 'emerald',
  rejected: 'red',
}

type ModalState =
  | { kind: 'approve'; item: BankTransferResponse }
  | { kind: 'reject';  item: BankTransferResponse }
  | null

export default function AdminBankTransfersPage() {
  const { getToken } = useAuth()
  const qc = useQueryClient()

  const [tab, setTab]               = useState<TabKey>('pending')
  const [page, setPage]             = useState(1)
  const [modal, setModal]           = useState<ModalState>(null)
  const [modalError, setModalError] = useState('')
  const [notes, setNotes]           = useState('')
  const [overrideRaw, setOverrideRaw] = useState('')

  const { data: transfers = [], isLoading, error, refetch } = useQuery({
    queryKey: ['admin', 'bank-transfers'],
    queryFn: async () => {
      const token = await getToken()
      return api.adminListBankTransfers(token!)
    },
  })

  const filtered = useMemo(() => (
    tab === 'all' ? transfers : transfers.filter(t => t.status === tab)
  ), [transfers, tab])

  const counts = useMemo(() => {
    const result: Record<string, number> = { all: transfers.length }
    for (const t of transfers) result[t.status] = (result[t.status] ?? 0) + 1
    return result
  }, [transfers])

  const pageCount  = Math.ceil(filtered.length / PAGE_SIZE)
  const safePage   = Math.min(Math.max(page, 1), Math.max(pageCount, 1))
  const paginated  = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)
  const rangeStart = filtered.length === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1
  const rangeEnd   = Math.min(safePage * PAGE_SIZE, filtered.length)

  function changeTab(key: TabKey) { setTab(key); setPage(1) }

  function openApprove(item: BankTransferResponse) {
    setModal({ kind: 'approve', item })
    setModalError('')
    setNotes('')
    setOverrideRaw('')
  }

  function openReject(item: BankTransferResponse) {
    setModal({ kind: 'reject', item })
    setModalError('')
    setNotes('')
  }

  function closeModal() {
    setModal(null)
    setModalError('')
    setNotes('')
    setOverrideRaw('')
  }

  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin', 'bank-transfers'] })

  const approveMutation = useMutation({
    mutationFn: async ({ id, data }: { id: number; data: { notes?: string; approved_amount_cents?: number } }) => {
      const token = await getToken()
      return api.adminApproveBankTransfer(token!, id, data)
    },
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: unknown) => {
      setModalError(e instanceof Error ? e.message : 'Error al aprobar la transferencia')
    },
  })

  const rejectMutation = useMutation({
    mutationFn: async ({ id, notes }: { id: number; notes: string }) => {
      const token = await getToken()
      return api.adminRejectBankTransfer(token!, id, notes)
    },
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: unknown) => {
      setModalError(e instanceof Error ? e.message : 'Error al rechazar la transferencia')
    },
  })

  const isBusy = approveMutation.isPending || rejectMutation.isPending

  const emptyMsg = tab === 'pending'
    ? 'No hay comprobantes pendientes de revisión.'
    : `No hay transferencias con estado "${tab}".`

  function submitApprove() {
    if (modal?.kind !== 'approve') return
    const data: { notes?: string; approved_amount_cents?: number } = {}
    if (notes.trim()) data.notes = notes.trim()
    if (overrideRaw.trim()) {
      const q = Number.parseFloat(overrideRaw)
      if (!Number.isNaN(q) && q > 0) {
        data.approved_amount_cents = Math.round(q * 100)
      } else {
        setModalError('El monto a acreditar debe ser positivo')
        return
      }
    }
    approveMutation.mutate({ id: modal.item.id, data })
  }

  function submitReject() {
    if (modal?.kind !== 'reject') return
    if (!notes.trim()) { setModalError('El motivo de rechazo es requerido'); return }
    rejectMutation.mutate({ id: modal.item.id, notes: notes.trim() })
  }

  return (
    <div className="space-y-6">
      <AdminPageHeader
        title="Transferencias Bancarias"
        subtitle={`${transfers.length} comprobantes en total`}
        onRefresh={refetch}
        isLoading={isLoading}
      />

      {/* Tabs */}
      <div className="flex flex-wrap gap-2">
        {TABS.map(t => {
          const count = counts[t.key === 'all' ? 'all' : t.key] ?? 0
          const isActive = tab === t.key
          const accent = ACCENT_BY_STATUS[t.key] ?? 'blue'
          return (
            <button
              key={t.key}
              onClick={() => changeTab(t.key)}
              className={cn(
                'flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-medium transition-colors',
                isActive
                  ? `bg-${accent}-500/20 text-${accent}-400 ring-1 ring-${accent}-500/40`
                  : 'bg-white/5 text-white/60 hover:bg-white/10 hover:text-white',
              )}
            >
              {t.label}
              <span className={cn(
                'text-xs px-1.5 py-0.5 rounded-full tabular-nums',
                isActive ? `bg-${accent}-500/30 text-${accent}-300` : 'bg-white/10 text-white/40',
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
          Error al cargar las transferencias.
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-16 text-white/40">
          <p className="text-lg font-medium">Sin transferencias</p>
          <p className="text-sm mt-1">{emptyMsg}</p>
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
                  <th className="px-4 py-3 font-medium">Acreditado</th>
                  <th className="px-4 py-3 font-medium">Estado</th>
                  <th className="px-4 py-3 font-medium">Enviado</th>
                  <th className="px-4 py-3 font-medium">Comprobante</th>
                  <th className="px-4 py-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {paginated.map(item => {
                  const effective = item.approved_amount_cents ?? item.amount_cents
                  const hasOverride = item.approved_amount_cents !== null && item.approved_amount_cents !== item.amount_cents
                  return (
                    <tr
                      key={item.id}
                      className="border-b border-white/5 hover:bg-white/[0.03] transition-colors"
                    >
                      <td className="px-4 py-3 text-white/40 font-mono text-xs">#{item.id}</td>

                      <td className="px-4 py-3">
                        <span className="text-white/70 font-mono text-xs bg-white/5 px-2 py-0.5 rounded">
                          #{item.user_id}
                        </span>
                      </td>

                      <td className="px-4 py-3 tabular-nums">
                        <div className="font-semibold text-white">{centsToQ(item.amount_cents)}</div>
                        <div className="text-white/30 text-xs">{item.currency}</div>
                      </td>

                      <td className="px-4 py-3 tabular-nums">
                        {item.status === 'approved' ? (
                          <div>
                            <div className={cn('font-semibold', hasOverride ? 'text-amber-400' : 'text-emerald-400')}>
                              {centsToQ(effective)}
                            </div>
                            {hasOverride && (
                              <div className="text-white/30 text-xs">declarado: {centsToQ(item.amount_cents)}</div>
                            )}
                          </div>
                        ) : (
                          <span className="text-white/25">—</span>
                        )}
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
                        <a
                          href={proofDownloadUrl(item.id)}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-1 text-blue-400 hover:text-blue-300 text-xs transition-colors"
                          title={`Descargar comprobante #${item.id}`}
                        >
                          Ver <ExternalLink className="h-3 w-3" />
                        </a>
                      </td>

                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          {item.status === 'pending' && (
                            <>
                              <button
                                onClick={() => openApprove(item)}
                                className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-emerald-500/15 hover:bg-emerald-500/25 text-emerald-400 text-xs font-medium transition-colors"
                              >
                                <CheckCircle className="h-3.5 w-3.5" />
                                Aprobar
                              </button>
                              <button
                                onClick={() => openReject(item)}
                                className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-red-500/15 hover:bg-red-500/25 text-red-400 text-xs font-medium transition-colors"
                              >
                                <XCircle className="h-3.5 w-3.5" />
                                Rechazar
                              </button>
                            </>
                          )}
                          {item.status !== 'pending' && item.notes && (
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
                  )
                })}
              </tbody>
            </table>
          </div>

          <AdminPagination
            page={safePage}
            pageCount={pageCount}
            rangeStart={rangeStart}
            rangeEnd={rangeEnd}
            total={filtered.length}
            itemLabel="transferencias"
            onPageChange={setPage}
          />
        </>
      )}

      {modal && (
        <AdminModalOverlay onClose={closeModal} scrollable>
          {modal.kind === 'approve' ? (
            <ApproveModal
              item={modal.item}
              overrideRaw={overrideRaw}
              notes={notes}
              onOverrideChange={setOverrideRaw}
              onNotesChange={setNotes}
              isBusy={isBusy}
              error={modalError}
              onSubmit={submitApprove}
              onClose={closeModal}
            />
          ) : (
            <RejectModal
              item={modal.item}
              notes={notes}
              onNotesChange={setNotes}
              isBusy={isBusy}
              error={modalError}
              onSubmit={submitReject}
              onClose={closeModal}
            />
          )}
        </AdminModalOverlay>
      )}
    </div>
  )
}
