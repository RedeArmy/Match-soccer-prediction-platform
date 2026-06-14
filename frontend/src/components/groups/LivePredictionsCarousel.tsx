'use client'

import React, { useState } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useQuery } from '@tanstack/react-query'
import { Activity, ChevronDown, ChevronUp } from 'lucide-react'
import { api } from '@/lib/api'
import type { LivePredictionsResponse, MatchResponse, UserLivePrediction } from '@/lib/api-types'
import { HorizontalCarousel } from '@/components/shared/HorizontalCarousel'
import { useI18n } from '@/lib/i18n'

interface Props {
  readonly groupId: number
}

// ── Public component ─────────────────────────────────────────────────────────

export function LivePredictionsCarousel({ groupId }: Props) {
  const { getToken } = useAuth()
  const { t } = useI18n()

  const { data, isLoading } = useQuery<LivePredictionsResponse>({
    queryKey: ['group-live-predictions', groupId],
    queryFn: async () => {
      const token = await getToken()
      return api.getGroupLivePredictions(token!, groupId)
    },
    // Poll every 30 s so the carousel stays current during live matches.
    refetchInterval: 30_000,
    // Don't keep stale data visible past one minute when the tab is backgrounded.
    staleTime: 60_000,
  })

  // Don't render anything while loading or when no live matches exist.
  if (isLoading) return null
  if (!data || data.live_matches.length === 0) return null

  return (
    <section>
      <div className="mb-3 flex items-center gap-2">
        <Activity className="h-4 w-4 text-red-400 animate-pulse" />
        <h2 className="text-sm font-semibold uppercase tracking-wide text-red-400">
          {t('groups.livePredictionsTitle')}
        </h2>
        <span className="rounded-full bg-red-500/20 px-2 py-0.5 text-[10px] font-semibold text-red-400">
          {t('common.live')}
        </span>
      </div>

      {data.user_predictions.length === 0 ? (
        <p className="rounded-xl border border-white/10 bg-white/[0.02] px-4 py-6 text-center text-sm text-text-muted">
          {t('groups.livePredictionsEmpty')}
        </p>
      ) : (
        <HorizontalCarousel
          itemWidth="w-64"
          gap="gap-3"
          scrollAmount={280}
          ariaLabelLeft={t('common.scrollLeft')}
          ariaLabelRight={t('common.scrollRight')}
        >
          {data.user_predictions.map((row) => (
            <UserPredictionCard
              key={row.user_id}
              row={row}
              liveMatches={data.live_matches}
              t={t}
            />
          ))}
        </HorizontalCarousel>
      )}
    </section>
  )
}

// ── User prediction card ─────────────────────────────────────────────────────

interface CardProps {
  readonly row: UserLivePrediction
  readonly liveMatches: MatchResponse[]
  readonly t: ReturnType<typeof useI18n>['t']
}

function UserPredictionCard({ row, liveMatches, t }: CardProps) {
  const [open, setOpen] = useState(false)

  const initials = row.display_name
    .split(' ')
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? '')
    .join('')

  const hasPredictions = row.predictions.some((p) => p.has_prediction)

  return (
    <article className="flex flex-col rounded-2xl border border-white/10 bg-[#0d1f35] overflow-hidden">
      {/* Live pulse bar */}
      <div className="h-1 bg-gradient-to-r from-red-500 via-red-400 to-orange-400 animate-pulse" />

      {/* Card header: avatar + name + toggle */}
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-white/[0.03]"
        aria-expanded={open}
        aria-label={`${open ? t('groups.hidePredictions') : t('groups.showPredictions')} ${row.display_name}`}
      >
        {/* Initials avatar */}
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-white/10 bg-white/[0.06] text-xs font-bold text-text-secondary">
          {initials}
        </div>

        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold text-white">{row.display_name}</p>
          <p className="text-[11px] text-text-muted">
            {hasPredictions
              ? `${row.predictions.filter((p) => p.has_prediction).length} / ${liveMatches.length} ${t('groups.predCount')}`
              : t('groups.noPrediction')}
          </p>
        </div>

        {open ? (
          <ChevronUp className="h-4 w-4 shrink-0 text-text-muted" />
        ) : (
          <ChevronDown className="h-4 w-4 shrink-0 text-text-muted" />
        )}
      </button>

      {/* Dropdown: prediction per live match */}
      {open && (
        <div className="border-t border-white/10 divide-y divide-white/[0.06]">
          {liveMatches.map((match) => (
            <MatchPredictionRow
              key={match.id}
              match={match}
              snap={row.predictions.find((p) => p.match_id === match.id)}
              t={t}
            />
          ))}
        </div>
      )}
    </article>
  )
}

// ── Match prediction row ──────────────────────────────────────────────────────

interface MatchPredictionRowProps {
  readonly match: MatchResponse
  readonly snap: { home_score: number; away_score: number; has_prediction: boolean } | undefined
  readonly t: ReturnType<typeof useI18n>['t']
}

function MatchPredictionRow({ match, snap, t }: MatchPredictionRowProps) {
  return (
    <div className="px-4 py-3">
      <div className="mb-2 flex items-center justify-between gap-2 text-xs text-text-muted">
        <span className="truncate font-medium text-text-secondary">{match.home_team}</span>
        <span className="shrink-0 text-[10px] uppercase tracking-wide">{t('groups.predictionVs')}</span>
        <span className="truncate text-right font-medium text-text-secondary">{match.away_team}</span>
      </div>
      {snap?.has_prediction ? (
        <div className="flex items-center justify-center gap-3">
          <span className="font-score w-8 text-right text-xl font-bold tabular-nums text-white">
            {snap.home_score}
          </span>
          <span className="text-xs text-text-muted">—</span>
          <span className="font-score w-8 text-xl font-bold tabular-nums text-white">
            {snap.away_score}
          </span>
        </div>
      ) : (
        <p className="text-center text-xs italic text-text-muted/60">{t('groups.noPrediction')}</p>
      )}
    </div>
  )
}

