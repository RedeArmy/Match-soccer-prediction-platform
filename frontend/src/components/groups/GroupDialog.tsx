'use client'

import { useState } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Plus, Users, X } from 'lucide-react'
import { api } from '@/lib/api'
import type { QuinielaResponse } from '@/lib/api-types'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'

type Tab = 'create' | 'join'

interface Props {
  open: boolean
  defaultTab?: Tab
  onClose: () => void
}

export function GroupDialog({ open, defaultTab = 'create', onClose }: Readonly<Props>) {
  const { getToken } = useAuth()
  const queryClient = useQueryClient()
  const { t } = useI18n()

  const [tab, setTab] = useState<Tab>(defaultTab)
  const [created, setCreated] = useState<QuinielaResponse | null>(null)
  const [copied, setCopied] = useState(false)

  const [name, setName] = useState('')
  const [inviteCode, setInviteCode] = useState('')

  const createMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken()
      return api.createGroup(token!, { name: name.trim() })
    },
    onSuccess: (data) => {
      setCreated(data)
      queryClient.invalidateQueries({ queryKey: ['my-groups'] })
    },
  })

  const joinMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken()
      return api.joinGroup(token!, inviteCode.trim().toUpperCase())
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['my-groups'] })
      handleClose()
    },
  })

  function handleClose() {
    setCreated(null)
    setCopied(false)
    setName('')
    setInviteCode('')
    createMutation.reset()
    joinMutation.reset()
    onClose()
  }

  async function copyCode(code: string) {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={handleClose}
      />

      <div className="relative w-full max-w-md rounded-2xl border border-white/10 bg-[#0b1929] p-6 shadow-2xl">
        <div className="mb-5 flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-gold-300">
              {t('groups.eyebrow')}
            </p>
            <h2 className="mt-0.5 text-lg font-semibold text-white">
              {created
                ? t('groups.createTitle')
                : tab === 'create'
                  ? t('groups.createTitle')
                  : t('groups.joinTitle')}
            </h2>
          </div>
          <button
            type="button"
            onClick={handleClose}
            className="mt-0.5 rounded p-1 text-text-muted transition-colors hover:text-white"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {!created && (
          <div className="mb-5 flex gap-1 rounded-xl border border-white/10 bg-white/[0.03] p-1">
            {(['create', 'join'] as Tab[]).map((key) => (
              <button
                key={key}
                type="button"
                onClick={() => setTab(key)}
                className={cn(
                  'flex-1 rounded-lg py-2 text-sm font-medium transition-colors',
                  tab === key
                    ? 'bg-gold-400 text-blue-950'
                    : 'text-text-muted hover:text-text-primary',
                )}
              >
                {key === 'create' ? t('groups.tabCreate') : t('groups.tabJoin')}
              </button>
            ))}
          </div>
        )}

        {created && (
          <div className="space-y-4">
            <div className="rounded-xl border border-green-400/20 bg-green-400/10 p-4 text-center">
              <Check className="mx-auto mb-2 h-8 w-8 text-green-400" />
              <p className="text-sm font-medium text-green-200">{t('groups.createSuccess')}</p>
              <p className="mt-0.5 text-xs text-text-muted">{created.name}</p>
            </div>

            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                {t('groups.inviteCodeLabel')}
              </p>
              <div className="flex items-center gap-3 rounded-xl border border-white/15 bg-white/[0.05] px-4 py-3">
                <span className="flex-1 font-mono text-2xl font-bold tracking-widest text-gold-300">
                  {created.invite_code}
                </span>
                <button
                  type="button"
                  onClick={() => copyCode(created.invite_code)}
                  className="rounded p-1 text-text-muted transition-colors hover:text-white"
                >
                  {copied
                    ? <Check className="h-4 w-4 text-green-400" />
                    : <Copy className="h-4 w-4" />}
                </button>
              </div>
              <p className="mt-2 text-[11px] text-text-muted">{t('groups.inviteCodeHint')}</p>
            </div>

            <button type="button" onClick={handleClose} className="btn-gold w-full py-2.5 text-sm">
              {t('groups.done')}
            </button>
          </div>
        )}

        {!created && tab === 'create' && (
          <form
            className="space-y-4"
            onSubmit={(e) => { e.preventDefault(); createMutation.mutate() }}
          >
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-text-muted">
                {t('groups.nameLabel')}
              </label>
              <input
                className="input-base"
                placeholder={t('groups.namePlaceholder')}
                maxLength={60}
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>

            {createMutation.isError && (
              <p className="rounded border border-red-400/20 bg-red-400/10 px-3 py-2 text-xs text-red-300">
                {t('groups.createError')}
              </p>
            )}

            <button
              type="submit"
              disabled={!name.trim() || createMutation.isPending}
              className="btn-gold w-full py-2.5 text-sm disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Plus className="h-4 w-4" />
              {createMutation.isPending ? t('common.saving') : t('groups.createAction')}
            </button>
          </form>
        )}

        {!created && tab === 'join' && (
          <form
            className="space-y-4"
            onSubmit={(e) => { e.preventDefault(); joinMutation.mutate() }}
          >
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-text-muted">
                {t('groups.inviteCodeLabel')}
              </label>
              <input
                className="input-base font-mono tracking-widest"
                placeholder="XXXXXXXXXX"
                maxLength={12}
                required
                value={inviteCode}
                onChange={(e) => setInviteCode(e.target.value.toUpperCase())}
              />
            </div>

            {joinMutation.isError && (
              <p className="rounded border border-red-400/20 bg-red-400/10 px-3 py-2 text-xs text-red-300">
                {t('groups.joinError')}
              </p>
            )}

            <button
              type="submit"
              disabled={!inviteCode.trim() || joinMutation.isPending}
              className="btn-gold w-full py-2.5 text-sm disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Users className="h-4 w-4" />
              {joinMutation.isPending ? t('common.saving') : t('groups.joinAction')}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
