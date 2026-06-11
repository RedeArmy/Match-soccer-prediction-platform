'use client'

import { useState, useMemo } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle, CheckCircle2, ChevronLeft, ChevronRight,
  Edit2, History, RotateCcw, Search, X, Zap,
} from 'lucide-react'
import { api } from '@/lib/api'
import type { SystemParamResponse } from '@/lib/api-types'
import { cn, formatDateTime, formatRelative } from '@/lib/utils'
import { LoadingState, LoadingSpinner } from '@/components/shared/LoadingState'

// ── Constants ─────────────────────────────────────────────────────────────────

const PAGE_SIZE = 15

// ── Helpers ──────────────────────────────────────────────────────────────────

function isSensitive(key: string): boolean {
  return key.startsWith('scoring.') || key.startsWith('payment.')
}

const TYPE_CHIP: Record<string, string> = {
  int:    'bg-blue-500/20 text-blue-300',
  float:  'bg-orange-500/20 text-orange-300',
  bool:   'bg-green-500/20 text-green-300',
  string: 'bg-purple-500/20 text-purple-300',
}

// Returns page numbers (and '...' gaps) for the pagination bar.
function getPageNumbers(current: number, total: number): (number | '...')[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)

  const set = new Set(
    [1, total, current - 1, current, current + 1].filter((n) => n >= 1 && n <= total),
  )
  const sorted = Array.from(set).sort((a, b) => a - b)

  const result: (number | '...')[] = []
  for (let i = 0; i < sorted.length; i++) {
    result.push(sorted[i])
    if (i < sorted.length - 1 && sorted[i + 1] - sorted[i] > 1) {
      result.push('...')
    }
  }
  return result
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function AdminSystemParamsPage() {
  const { getToken } = useAuth()
  const qc = useQueryClient()

  const [search, setSearch]               = useState('')
  const [activeCategory, setActiveCategory] = useState('all')
  const [page, setPage]                   = useState(1)
  const [editingKey, setEditingKey]       = useState<string | null>(null)
  const [editValue, setEditValue]         = useState('')
  const [editError, setEditError]         = useState('')
  const [resettingKey, setResettingKey]   = useState<string | null>(null)
  const [historyKey, setHistoryKey]       = useState<string | null>(null)

  // ── Queries ───────────────────────────────────────────────────────────────

  const { data: params = [], isLoading } = useQuery({
    queryKey: ['admin-system-params'],
    queryFn: async () => {
      const token = await getToken()
      return api.adminGetSystemParams(token!)
    },
  })

  const { data: history, isLoading: historyLoading } = useQuery({
    queryKey: ['admin-system-param-history', historyKey],
    queryFn: async () => {
      const token = await getToken()
      return api.adminGetSystemParamHistory(token!, historyKey!)
    },
    enabled: historyKey !== null,
  })

  // ── Mutations ─────────────────────────────────────────────────────────────

  const setMutation = useMutation({
    mutationFn: async ({ key, value }: { key: string; value: string }) => {
      const token = await getToken()
      return api.adminSetSystemParam(token!, key, value)
    },
    onSuccess: (updated) => {
      qc.setQueryData<SystemParamResponse[]>(['admin-system-params'], (old) =>
        old?.map((p) => (p.key === updated.key ? updated : p)) ?? [],
      )
      setEditingKey(null)
      setEditError('')
    },
    onError: (e: Error) => setEditError(e.message),
  })

  const resetMutation = useMutation({
    mutationFn: async (key: string) => {
      const token = await getToken()
      return api.adminResetSystemParam(token!, key)
    },
    onSuccess: (updated) => {
      qc.setQueryData<SystemParamResponse[]>(['admin-system-params'], (old) =>
        old?.map((p) => (p.key === updated.key ? updated : p)) ?? [],
      )
      setResettingKey(null)
    },
    onError: () => setResettingKey(null),
  })

  // ── Derived ───────────────────────────────────────────────────────────────

  const categories = useMemo(() => {
    const seen = new Set<string>()
    for (const p of params) if (p.category) seen.add(p.category)
    return ['all', ...Array.from(seen).sort()]
  }, [params])

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    return params.filter((p) => {
      if (activeCategory !== 'all' && p.category !== activeCategory) return false
      if (q && !p.key.toLowerCase().includes(q) && !p.description.toLowerCase().includes(q)) return false
      return true
    })
  }, [params, search, activeCategory])

  const pageCount  = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const safePage   = Math.min(page, pageCount)
  const paginated  = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)
  const rangeStart = filtered.length === 0 ? 0 : (safePage - 1) * PAGE_SIZE + 1
  const rangeEnd   = Math.min(safePage * PAGE_SIZE, filtered.length)

  // ── Filter helpers (reset page on change) ────────────────────────────────

  function applySearch(value: string) {
    setSearch(value)
    setPage(1)
  }

  function applyCategory(cat: string) {
    setActiveCategory(cat)
    setPage(1)
  }

  // ── Edit helpers ──────────────────────────────────────────────────────────

  function startEdit(key: string, currentValue: string) {
    setEditingKey(key)
    setEditValue(currentValue)
    setEditError('')
  }

  function cancelEdit() {
    setEditingKey(null)
    setEditError('')
  }

  function saveEdit() {
    if (!editingKey) return
    setMutation.mutate({ key: editingKey, value: editValue })
  }

  // ── Render ────────────────────────────────────────────────────────────────

  const historyParam = params.find((p) => p.key === historyKey)

  return (
    <div className="space-y-6">

      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="font-display text-3xl text-white">PARÁMETROS</h1>
          {!isLoading && (
            <p className="mt-1 text-xs text-text-muted">
              {params.length} parámetros · {params.filter((p) => p.value !== p.default_value).length} modificados
            </p>
          )}
        </div>
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" />
          <input
            type="search"
            placeholder="Buscar clave o descripción..."
            value={search}
            onChange={(e) => applySearch(e.target.value)}
            className="input-base w-72 py-2 pl-9 text-sm"
          />
        </div>
      </div>

      {/* Category filter */}
      {!isLoading && categories.length > 2 && (
        <div className="flex flex-wrap gap-1.5">
          {categories.map((cat) => {
            const count = cat === 'all' ? params.length : params.filter((p) => p.category === cat).length
            return (
              <button
                key={cat}
                type="button"
                onClick={() => applyCategory(cat)}
                className={cn(
                  'rounded-full px-3 py-1 text-xs font-medium transition-colors',
                  activeCategory === cat
                    ? 'bg-blue-700 text-white'
                    : 'bg-blue-900/50 text-text-secondary hover:bg-blue-800 hover:text-text-primary',
                )}
              >
                {cat === 'all' ? 'Todos' : cat}
                <span className="ml-1.5 text-[10px] opacity-60">{count}</span>
              </button>
            )
          })}
        </div>
      )}

      {/* Table */}
      {isLoading && <LoadingState rows={8} />}

      {!isLoading && filtered.length === 0 && (
        <p className="py-12 text-center text-sm text-text-muted">
          {search ? 'Sin resultados para la búsqueda.' : 'No hay parámetros.'}
        </p>
      )}

      {!isLoading && filtered.length > 0 && (
        <>
          <div className="card overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-sm">
                <thead>
                  <tr className="border-b border-blue-800/50 text-[10px] font-semibold uppercase tracking-wide text-text-muted">
                    <th className="px-4 py-3 text-left">Parámetro</th>
                    <th className="px-4 py-3 text-left">Valor actual</th>
                    <th className="px-4 py-3 text-left">Default</th>
                    <th className="px-4 py-3 text-left">Tipo</th>
                    <th className="px-4 py-3 text-center">Runtime</th>
                    <th className="px-4 py-3 text-right">Actualizado</th>
                    <th className="px-4 py-3" />
                  </tr>
                </thead>
                <tbody>
                  {paginated.map((param) => {
                    const isEditing   = editingKey === param.key
                    const isResetting = resettingKey === param.key
                    const isDirty     = param.value !== param.default_value
                    const sensitive   = isSensitive(param.key)

                    return (
                      <tr
                        key={param.key}
                        className={cn(
                          'border-b border-blue-800/30 transition-colors',
                          isEditing ? 'bg-blue-800/20' : 'hover:bg-blue-900/30',
                        )}
                      >
                        {/* Key + description */}
                        <td className="max-w-[280px] px-4 py-3">
                          <div className="flex items-start gap-2">
                            {sensitive && (
                              <span title="Parámetro sensible (scoring / payment)">
                                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-orange-400" />
                              </span>
                            )}
                            <div className="min-w-0">
                              <p className="truncate font-mono text-xs text-text-primary">{param.key}</p>
                              {param.description && (
                                <p className="mt-0.5 line-clamp-2 text-[10px] leading-relaxed text-text-muted">
                                  {param.description}
                                </p>
                              )}
                            </div>
                          </div>
                        </td>

                        {/* Current value — editable when active */}
                        <td className="px-4 py-3">
                          {isEditing ? (
                            <div className="space-y-1.5">
                              <input
                                type="text"
                                value={editValue}
                                onChange={(e) => setEditValue(e.target.value)}
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter') saveEdit()
                                  if (e.key === 'Escape') cancelEdit()
                                }}
                                className="input-base w-full max-w-[200px] py-1 text-xs"
                                autoFocus
                              />
                              {sensitive && (
                                <p className="flex items-center gap-1 text-[10px] text-orange-400">
                                  <AlertTriangle className="h-3 w-3 shrink-0" />
                                  Parámetro sensible — afecta scoring o pagos en producción
                                </p>
                              )}
                              {editError && (
                                <p className="text-[10px] text-red-400">{editError}</p>
                              )}
                            </div>
                          ) : (
                            <span
                              className={cn(
                                'font-mono text-xs',
                                param.value === ''
                                  ? 'italic text-text-muted'
                                  : isDirty
                                  ? 'font-semibold text-gold-300'
                                  : 'text-text-primary',
                              )}
                            >
                              {param.value === '' ? '(vacío)' : param.value}
                            </span>
                          )}
                        </td>

                        {/* Default */}
                        <td className="px-4 py-3">
                          <span className="font-mono text-xs text-text-muted">
                            {param.default_value === '' ? '—' : param.default_value}
                          </span>
                        </td>

                        {/* Type */}
                        <td className="px-4 py-3">
                          <span
                            className={cn(
                              'rounded px-1.5 py-0.5 text-[10px] font-medium',
                              TYPE_CHIP[param.type] ?? 'bg-blue-800/40 text-text-secondary',
                            )}
                          >
                            {param.type}
                          </span>
                        </td>

                        {/* Runtime badge */}
                        <td className="px-4 py-3 text-center">
                          {param.is_runtime && (
                            <span title="Toma efecto inmediato sin reiniciar">
                              <Zap className="mx-auto h-3.5 w-3.5 text-gold-400" />
                            </span>
                          )}
                        </td>

                        {/* Updated at */}
                        <td className="px-4 py-3 text-right">
                          <span className="whitespace-nowrap text-[10px] text-text-muted">
                            {formatRelative(param.updated_at)}
                          </span>
                        </td>

                        {/* Actions */}
                        <td className="px-4 py-3">
                          {isEditing ? (
                            <div className="flex items-center gap-1.5">
                              <button
                                type="button"
                                onClick={saveEdit}
                                disabled={setMutation.isPending}
                                className="flex items-center gap-1 rounded bg-blue-600 px-2 py-1 text-xs text-white transition-colors hover:bg-blue-500 disabled:opacity-50"
                              >
                                {setMutation.isPending
                                  ? <LoadingSpinner size={12} />
                                  : <CheckCircle2 className="h-3 w-3" />}
                                Guardar
                              </button>
                              <button
                                type="button"
                                onClick={cancelEdit}
                                className="rounded px-2 py-1 text-xs text-text-muted hover:text-text-primary"
                              >
                                Cancelar
                              </button>
                            </div>
                          ) : (
                            <div className="flex items-center justify-end gap-0.5">
                              <button
                                type="button"
                                onClick={() => startEdit(param.key, param.value)}
                                className="rounded p-1.5 text-text-muted transition-colors hover:bg-blue-800 hover:text-white"
                                title="Editar valor"
                              >
                                <Edit2 className="h-3.5 w-3.5" />
                              </button>

                              {isDirty && (
                                <button
                                  type="button"
                                  onClick={() => {
                                    setResettingKey(param.key)
                                    resetMutation.mutate(param.key)
                                  }}
                                  disabled={isResetting && resetMutation.isPending}
                                  className="rounded p-1.5 text-text-muted transition-colors hover:bg-orange-500/20 hover:text-orange-300 disabled:opacity-40"
                                  title="Restaurar valor por defecto"
                                >
                                  {isResetting && resetMutation.isPending
                                    ? <LoadingSpinner size={14} />
                                    : <RotateCcw className="h-3.5 w-3.5" />}
                                </button>
                              )}

                              <button
                                type="button"
                                onClick={() => setHistoryKey(param.key)}
                                className="rounded p-1.5 text-text-muted transition-colors hover:bg-blue-800 hover:text-blue-300"
                                title="Ver historial de cambios"
                              >
                                <History className="h-3.5 w-3.5" />
                              </button>
                            </div>
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {/* Pagination bar */}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-text-muted">
              {rangeStart}–{rangeEnd} de {filtered.length} parámetros
            </p>

            <div className="flex items-center gap-1">
              {/* Previous */}
              <button
                type="button"
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={safePage === 1}
                className="flex h-8 w-8 items-center justify-center rounded text-text-muted transition-colors hover:bg-blue-800 hover:text-white disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-text-muted"
                aria-label="Página anterior"
              >
                <ChevronLeft className="h-4 w-4" />
              </button>

              {/* Page numbers */}
              {getPageNumbers(safePage, pageCount).map((n, i) =>
                n === '...'
                  ? (
                    <span key={`ellipsis-${i}`} className="flex h-8 w-8 items-center justify-center text-xs text-text-muted">
                      …
                    </span>
                  )
                  : (
                    <button
                      key={n}
                      type="button"
                      onClick={() => setPage(n)}
                      className={cn(
                        'flex h-8 w-8 items-center justify-center rounded text-xs font-medium transition-colors',
                        safePage === n
                          ? 'bg-blue-700 text-white'
                          : 'text-text-secondary hover:bg-blue-800 hover:text-white',
                      )}
                      aria-current={safePage === n ? 'page' : undefined}
                    >
                      {n}
                    </button>
                  ),
              )}

              {/* Next */}
              <button
                type="button"
                onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
                disabled={safePage === pageCount}
                className="flex h-8 w-8 items-center justify-center rounded text-text-muted transition-colors hover:bg-blue-800 hover:text-white disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-text-muted"
                aria-label="Página siguiente"
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          </div>
        </>
      )}

      {/* History slide-over */}
      {historyKey && (
        <div className="fixed inset-0 z-50 flex items-start justify-end p-4">
          <button
            type="button"
            aria-label="Cerrar historial"
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setHistoryKey(null)}
          />
          <div className="relative z-10 flex h-[calc(100vh-2rem)] w-full max-w-md flex-col overflow-hidden rounded-2xl border border-blue-700/50 bg-[#0D1F3C] shadow-2xl">

            {/* Panel header */}
            <div className="flex shrink-0 items-start justify-between gap-3 border-b border-blue-800/50 px-5 py-4">
              <div className="min-w-0">
                <p className="text-[10px] font-semibold uppercase tracking-wide text-text-muted">
                  Historial de cambios
                </p>
                <p className="mt-0.5 truncate font-mono text-sm text-white">{historyKey}</p>
                {historyParam?.description && (
                  <p className="mt-1 text-[11px] text-text-muted">{historyParam.description}</p>
                )}
              </div>
              <button
                type="button"
                onClick={() => setHistoryKey(null)}
                className="shrink-0 rounded p-1.5 text-text-muted transition-colors hover:text-white"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {/* Panel body */}
            <div className="flex-1 space-y-2 overflow-y-auto px-5 py-4">
              {historyLoading && <LoadingState rows={4} />}

              {!historyLoading && (!history?.data || history.data.length === 0) && (
                <p className="py-10 text-center text-xs text-text-muted">
                  Sin cambios registrados para este parámetro.
                </p>
              )}

              {!historyLoading && history?.data.map((entry) => (
                <div
                  key={entry.id}
                  className="space-y-2 rounded-lg border border-blue-800/40 bg-blue-900/20 px-4 py-3"
                >
                  <div className="flex items-center justify-between gap-2">
                    <span
                      className={cn(
                        'rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase',
                        entry.action === 'reset'
                          ? 'bg-orange-500/20 text-orange-300'
                          : 'bg-blue-500/20 text-blue-300',
                      )}
                    >
                      {entry.action}
                    </span>
                    <span className="text-[10px] text-text-muted">
                      {formatDateTime(entry.changed_at)}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 text-xs">
                    <span className="font-mono text-red-400/80 line-through">
                      {entry.old_value === '' ? '(vacío)' : entry.old_value}
                    </span>
                    <span className="text-text-muted">→</span>
                    <span className="font-mono text-green-400">
                      {entry.new_value === '' ? '(vacío)' : entry.new_value}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
