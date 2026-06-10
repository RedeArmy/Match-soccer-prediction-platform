'use client'

import { useState } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Search, ShieldOff, ShieldCheck, ChevronDown, ChevronUp, X } from 'lucide-react'
import { api } from '@/lib/api'
import type { AdminUserResponse, AdminUserProfileResponse } from '@/lib/api-types'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { LoadingState, LoadingSpinner } from '@/components/shared/LoadingState'
import { formatDate, formatDateTime } from '@/lib/utils'
import { cn } from '@/lib/utils'

type BanFilter = 'all' | 'active' | 'banned'

const banFilters: { key: BanFilter; label: string }[] = [
  { key: 'all',    label: 'Todos'    },
  { key: 'active', label: 'Activos'  },
  { key: 'banned', label: 'Baneados' },
]

const roles = ['', 'user', 'admin']

// ── Ban modal ─────────────────────────────────────────────────────────────────

interface BanModalProps {
  user: AdminUserResponse
  onConfirm: (reason: string) => void
  onCancel: () => void
  isPending: boolean
}

function BanModal({ user, onConfirm, onCancel, isPending }: BanModalProps) {
  const [reason, setReason] = useState('')
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-blue-950/80 backdrop-blur-sm px-4">
      <div className="card p-6 w-full max-w-sm space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="font-semibold text-white">Banear usuario</h3>
          <button type="button" onClick={onCancel} className="text-text-muted hover:text-white">
            <X className="w-4 h-4" />
          </button>
        </div>
        <p className="text-sm text-text-muted">
          <span className="text-white font-medium">{user.name}</span>
          {' '}({user.email})
        </p>
        <textarea
          value={reason}
          onChange={e => setReason(e.target.value)}
          placeholder="Motivo del baneo (requerido)..."
          rows={3}
          className="input-base resize-none"
        />
        <div className="flex gap-2">
          <button type="button" onClick={onCancel} className="btn-ghost flex-1 text-sm py-2">
            Cancelar
          </button>
          <button
            type="button"
            onClick={() => onConfirm(reason)}
            disabled={isPending || !reason.trim()}
            className="flex-1 text-sm py-2 bg-red-500 hover:bg-red-400 disabled:opacity-50 text-white rounded-lg transition-colors flex items-center justify-center gap-1"
          >
            {isPending ? <LoadingSpinner size={16} /> : 'Banear'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Profile detail panel ──────────────────────────────────────────────────────

interface ProfilePanelProps {
  userId: number
  currentRole: string
}

function ProfilePanel({ userId, currentRole }: ProfilePanelProps) {
  const { getToken } = useAuth()
  const qc = useQueryClient()
  const [roleValue, setRoleValue] = useState(currentRole)

  const { data, isLoading } = useQuery<AdminUserProfileResponse>({
    queryKey: ['admin-user-profile', userId],
    queryFn: async () => {
      const token = await getToken()
      return api.adminGetUserProfile(token!, userId)
    },
  })

  const setRole = useMutation({
    mutationFn: async (role: string) => {
      const token = await getToken()
      return api.adminSetUserRole(token!, userId, role)
    },
    onSuccess: (updated) => {
      setRoleValue(updated.role)
      qc.invalidateQueries({ queryKey: ['admin-users'] })
    },
  })

  if (isLoading) return <div className="py-4 px-5"><LoadingState rows={3} /></div>
  if (!data) return null

  return (
    <div className="bg-blue-900/30 border-t border-blue-800/50 px-5 py-4 space-y-5">

      {/* Role change */}
      <div>
        <p className="text-xs font-semibold uppercase tracking-wide text-text-muted mb-2">
          Rol de plataforma
        </p>
        <div className="flex items-center gap-3">
          <select
            value={roleValue}
            onChange={e => setRoleValue(e.target.value)}
            className="input-base text-sm"
          >
            <option value="user">user</option>
            <option value="admin">admin</option>
          </select>
          <button
            type="button"
            disabled={setRole.isPending || roleValue === currentRole}
            onClick={() => setRole.mutate(roleValue)}
            className="px-3 py-1.5 rounded-lg bg-gold-400/80 hover:bg-gold-400 text-bg-base text-xs font-medium disabled:opacity-40 transition-colors flex items-center gap-1"
          >
            {setRole.isPending ? <LoadingSpinner size={14} /> : 'Guardar rol'}
          </button>
          {setRole.isError && (
            <span className="text-xs text-red-400">
              {(setRole.error as { message?: string })?.message ?? 'Error'}
            </span>
          )}
          {setRole.isSuccess && (
            <span className="text-xs text-green-400">Rol actualizado</span>
          )}
        </div>
      </div>

      {/* Memberships */}
      <div>
        <p className="text-xs font-semibold uppercase tracking-wide text-text-muted mb-2">
          Grupos ({data.memberships.length})
        </p>
        {data.memberships.length === 0 ? (
          <p className="text-xs text-text-muted italic">Sin grupos</p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {data.memberships.map(m => (
              <span
                key={m.id}
                className="flex items-center gap-1.5 rounded-lg border border-blue-700 bg-blue-800/50 px-2.5 py-1 text-xs text-text-secondary"
              >
                <span className="font-medium text-white">#{m.quiniela_id}</span>
                <StatusBadge status={m.status} size="sm" />
                {m.role === 'owner' && (
                  <span className="text-gold-400 text-[10px]">owner</span>
                )}
                {m.joined_at && (
                  <span className="text-text-muted">{formatDate(m.joined_at)}</span>
                )}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Payments */}
      <div>
        <p className="text-xs font-semibold uppercase tracking-wide text-text-muted mb-2">
          Pagos ({data.payments.length})
        </p>
        {data.payments.length === 0 ? (
          <p className="text-xs text-text-muted italic">Sin pagos</p>
        ) : (
          <div className="space-y-1">
            {data.payments.map(p => (
              <div
                key={p.id}
                className="flex items-center justify-between text-xs text-text-secondary rounded bg-blue-800/30 px-3 py-1.5"
              >
                <span>
                  <span className="text-white font-medium">
                    {p.currency} {(p.amount / 100).toFixed(2)}
                  </span>
                  {' — '}Quiniela #{p.quiniela_id}
                </span>
                <div className="flex items-center gap-2">
                  <StatusBadge status={p.status} size="sm" />
                  <span className="text-text-muted">{formatDate(p.created_at)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ── User row ──────────────────────────────────────────────────────────────────

interface UserRowProps {
  user: AdminUserResponse
  onBan: (user: AdminUserResponse) => void
  onUnban: (id: number) => void
  isMutating: boolean
}

function UserRow({ user, onBan, onUnban, isMutating }: UserRowProps) {
  const [expanded, setExpanded] = useState(false)
  const isBanned = Boolean(user.banned_at)

  return (
    <div className="border-b border-blue-800/40 last:border-0">
      <div className={cn('flex items-center gap-3 px-4 py-3', isBanned && 'opacity-60')}>

        {/* Expand toggle */}
        <button
          type="button"
          onClick={() => setExpanded(v => !v)}
          className="shrink-0 text-text-muted hover:text-white transition-colors"
          title={expanded ? 'Contraer' : 'Ver detalle'}
        >
          {expanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        </button>

        {/* Main info */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <p className="text-sm font-medium text-white">{user.name}</p>
            <StatusBadge status={user.role} size="sm" />
            {isBanned && <StatusBadge status="rejected" size="sm" />}
          </div>
          <p className="text-xs text-text-muted mt-0.5 truncate">{user.email}</p>
          {isBanned && user.ban_reason && (
            <p className="text-xs text-red-400 mt-0.5 truncate">Motivo: {user.ban_reason}</p>
          )}
        </div>

        {/* Metadata */}
        <div className="hidden md:flex flex-col items-end gap-0.5 shrink-0 text-right">
          <p className="text-xs text-text-muted">ID #{user.id}</p>
          <p className="text-xs text-text-muted">{formatDateTime(user.created_at)}</p>
        </div>

        {/* Actions */}
        <div className="shrink-0">
          {isBanned ? (
            <button
              type="button"
              onClick={() => onUnban(user.id)}
              disabled={isMutating}
              title="Quitar baneo"
              className="flex items-center gap-1 rounded px-2 py-1 text-xs text-green-400 hover:bg-green-400/10 transition-colors disabled:opacity-50"
            >
              <ShieldCheck className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Desbanear</span>
            </button>
          ) : (
            <button
              type="button"
              onClick={() => onBan(user)}
              disabled={isMutating}
              title="Banear usuario"
              className="flex items-center gap-1 rounded px-2 py-1 text-xs text-red-400 hover:bg-red-400/10 transition-colors disabled:opacity-50"
            >
              <ShieldOff className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Banear</span>
            </button>
          )}
        </div>
      </div>

      {/* Lazy-loaded detail panel — only mounted when expanded */}
      {expanded && <ProfilePanel userId={user.id} currentRole={user.role} />}
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function AdminUsersPage() {
  const { getToken } = useAuth()
  const qc = useQueryClient()

  const [search, setSearch]       = useState('')
  const [banFilter, setBanFilter] = useState<BanFilter>('all')
  const [role, setRole]           = useState('')
  const [banTarget, setBanTarget] = useState<AdminUserResponse | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-users', search, banFilter, role],
    queryFn: async () => {
      const token = await getToken()
      const banned = banFilter === 'all' ? undefined : banFilter === 'banned'
      return api.adminListUsers(token!, {
        search:  search || undefined,
        banned,
        role:    role || undefined,
      })
    },
  })

  const ban = useMutation({
    mutationFn: async ({ id, reason }: { id: number; reason: string }) => {
      const token = await getToken()
      return api.adminBanUser(token!, id, reason)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-users'] })
      setBanTarget(null)
    },
  })

  const unban = useMutation({
    mutationFn: async (id: number) => {
      const token = await getToken()
      return api.adminUnbanUser(token!, id)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-users'] }),
  })

  const users = data?.data ?? []
  const isMutating = ban.isPending || unban.isPending

  return (
    <div className="space-y-5">
      <h1 className="font-display text-3xl text-white">USUARIOS</h1>

      {/* ── Filters ── */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 min-w-48">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-muted pointer-events-none" />
          <input
            type="text"
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Buscar por nombre o email…"
            className="input-base pl-9 w-full text-sm"
          />
        </div>
        <select
          value={role}
          onChange={e => setRole(e.target.value)}
          className="input-base text-sm"
        >
          {roles.map(r => (
            <option key={r} value={r}>{r || 'Todos los roles'}</option>
          ))}
        </select>
      </div>

      {/* ── Ban filter tabs ── */}
      <div className="flex gap-1 p-1 bg-blue-900 rounded-xl w-fit">
        {banFilters.map(({ key, label }) => (
          <button
            key={key}
            type="button"
            onClick={() => setBanFilter(key)}
            className={cn(
              'px-3 py-1.5 rounded-lg text-xs transition-colors',
              banFilter === key
                ? 'bg-blue-700 text-white font-medium'
                : 'text-text-muted hover:text-text-secondary',
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {/* ── User list ── */}
      {isLoading ? (
        <LoadingState rows={8} />
      ) : (
        <div className="card divide-y divide-blue-800/40 overflow-hidden">
          {users.length === 0 ? (
            <p className="text-center text-sm text-text-muted py-10">
              No se encontraron usuarios
            </p>
          ) : (
            users.map(u => (
              <UserRow
                key={u.id}
                user={u}
                onBan={setBanTarget}
                onUnban={id => unban.mutate(id)}
                isMutating={isMutating}
              />
            ))
          )}
        </div>
      )}

      {data?.has_more && (
        <p className="text-center text-xs text-text-muted">
          Mostrando primeros 50 resultados — usa la búsqueda para filtrar más
        </p>
      )}

      {banTarget && (
        <BanModal
          user={banTarget}
          onConfirm={reason => ban.mutate({ id: banTarget.id, reason })}
          onCancel={() => setBanTarget(null)}
          isPending={ban.isPending}
        />
      )}
    </div>
  )
}
