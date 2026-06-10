'use client'

import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@clerk/nextjs'
import { AlertTriangle, CheckCircle, Loader2, Settings, X } from 'lucide-react'
import { api } from '@/lib/api'
import type { GroupDetailResponse } from '@/lib/api-types'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

// Matches domain.MinMembersPerGroup / system param group.min_members_for_active
const PREMIUM_MIN_MEMBERS = 5

interface Props {
  readonly group: GroupDetailResponse
  readonly memberCount: number
  readonly onClose: () => void
}

export function TournamentSettingsModal({ group, memberCount, onClose }: Props) {
  const { getToken } = useAuth()
  const { t } = useI18n()
  const queryClient = useQueryClient()

  const isFree = !group.is_premium
  const canUpgrade = isFree && memberCount >= PREMIUM_MIN_MEMBERS

  const [modeGeneral, setModeGeneral] = useState(group.mode_general)
  const [modeRound, setModeRound] = useState(group.mode_round)

  const mutation = useMutation({
    mutationFn: async () => {
      const token = await getToken()
      return api.setTournamentMode(token!, group.id, { mode_general: modeGeneral, mode_round: modeRound })
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(['group', group.id], updated)
      onClose()
    },
  })

  const isDirty = modeGeneral !== group.mode_general || modeRound !== group.mode_round
  const modesDisabled = isFree && !canUpgrade

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button
        type="button"
        aria-label="Close"
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="relative w-full max-w-sm rounded-2xl border border-white/10 bg-[#0b1929] p-6 shadow-2xl">
        {/* Header */}
        <div className="mb-5 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Settings className="h-4 w-4 text-gold-300" />
            <p className="text-sm font-semibold uppercase tracking-wide text-gold-300">
              {t('group.settings')}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded p-1 text-text-muted transition-colors hover:text-white"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <h2 className="mb-4 text-lg font-semibold text-white">{t('group.tournamentMode')}</h2>

        {/* Premium eligibility banner — only shown when group is free */}
        {isFree && (
          canUpgrade ? (
            <div className="mb-4 flex items-start gap-2.5 rounded-xl border border-gold-400/30 bg-gold-400/5 px-3 py-2.5">
              <CheckCircle className="mt-0.5 h-4 w-4 shrink-0 text-gold-400" />
              <p className="text-[11px] leading-relaxed text-gold-300">
                {t('group.premiumReady')}
              </p>
            </div>
          ) : (
            <div className="mb-4 flex items-start gap-2.5 rounded-xl border border-yellow-400/20 bg-yellow-400/5 px-3 py-2.5">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-400" />
              <p className="text-[11px] leading-relaxed text-yellow-300">
                {t('group.premiumNotReady').replace('{min}', String(PREMIUM_MIN_MEMBERS))}
              </p>
            </div>
          )
        )}

        {/* Mode toggles */}
        <div className="space-y-3">
          <ModeToggle
            id="mode-general"
            checked={modeGeneral}
            onChange={setModeGeneral}
            label={t('group.modeGeneral')}
            description={t('group.modeGeneralDesc')}
            disabled={modesDisabled || (!isFree && group.mode_general)}
          />
          <ModeToggle
            id="mode-round"
            checked={modeRound}
            onChange={setModeRound}
            label={t('group.modeRound')}
            description={t('group.modeRoundDesc')}
            disabled={modesDisabled || (!isFree && group.mode_round)}
          />
        </div>

        {mutation.isError && (
          <p className="mt-3 rounded-lg border border-red-400/20 bg-red-400/5 px-3 py-2 text-[11px] text-red-400">
            {(mutation.error as { message?: string })?.message ?? 'Error'}
          </p>
        )}

        <button
          type="button"
          disabled={!isDirty || modesDisabled || mutation.isPending}
          onClick={() => mutation.mutate()}
          className={cn(
            'mt-5 flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium transition-colors',
            isDirty && !modesDisabled
              ? 'bg-gold-400/80 text-bg-base hover:bg-gold-400 disabled:opacity-50'
              : 'cursor-not-allowed bg-white/5 text-text-muted',
          )}
        >
          {mutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
          {t('common.save')}
        </button>
      </div>
    </div>
  )
}

interface ModeToggleProps {
  readonly id: string
  readonly checked: boolean
  readonly onChange: (v: boolean) => void
  readonly label: string
  readonly description: string
  readonly disabled: boolean
}

function ModeToggle({ id, checked, onChange, label, description, disabled }: ModeToggleProps) {
  return (
    <label
      htmlFor={id}
      className={cn(
        'flex cursor-pointer items-start gap-3 rounded-xl border p-3 transition-colors',
        checked
          ? 'border-gold-400/30 bg-gold-400/5'
          : 'border-white/10 bg-white/[0.02] hover:border-white/20',
        disabled && 'cursor-not-allowed opacity-50',
      )}
    >
      <div className="relative mt-0.5 shrink-0">
        <input
          id={id}
          type="checkbox"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
          className="sr-only"
        />
        <div
          className={cn(
            'h-4 w-4 rounded border-2 transition-colors',
            checked
              ? 'border-gold-400 bg-gold-400'
              : 'border-white/30 bg-transparent',
          )}
        >
          {checked && (
            <svg viewBox="0 0 12 10" className="h-full w-full p-0.5 text-bg-base" fill="none" stroke="currentColor" strokeWidth="2">
              <polyline points="1,5 4,9 11,1" />
            </svg>
          )}
        </div>
      </div>
      <div className="min-w-0">
        <p className="text-sm font-medium text-white">{label}</p>
        <p className="mt-0.5 text-[11px] text-text-muted">{description}</p>
      </div>
    </label>
  )
}
