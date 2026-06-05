'use client'

import { useMemo, useState } from 'react'
import { ArrowRight, CheckCircle2, Circle, Crown, Shield, Sparkles, Trophy, Users } from 'lucide-react'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import groupsData from './groups.json'

type Team = {
  code: string
  name: string
  seed: string
  accent: string
}

type Group = {
  id: string
  label: string
  venue: string
  teams: Team[]
}

const groups: Group[] = groupsData as Group[]

const phases = ['quiniela.phaseGroups', 'quiniela.phaseThirds', 'quiniela.phaseKnockout']

export function WorldCupPoolBuilder() {
  const { t } = useI18n()
  const [selectedGroup, setSelectedGroup] = useState(groups[0].id)
  const [picks, setPicks] = useState<Record<string, string[]>>({
    A: ['MEX', 'KOR'],
    B: ['CAN'],
    C: ['BRA', 'MAR'],
  })

  const activeGroup = groups.find((group) => group.id === selectedGroup) ?? groups[0]
  const activePicks = picks[activeGroup.id] ?? []
  const completedGroups = groups.filter((group) => (picks[group.id] ?? []).length >= 2).length
  const totalPicks = Object.values(picks).reduce((total, groupPicks) => total + groupPicks.length, 0)
  const progress = Math.round((completedGroups / groups.length) * 100)

  const projectedQualifiers = useMemo(
    () =>
      groups.flatMap((group) =>
        (picks[group.id] ?? []).slice(0, 2).map((code, index) => ({
          group: group.id,
          code,
          rank: index + 1,
          team: group.teams.find((team) => team.code === code)?.name ?? code,
        })),
      ),
    [picks],
  )

  const togglePick = (code: string) => {
    setPicks((current) => {
      const currentGroupPicks = current[activeGroup.id] ?? []
      const exists = currentGroupPicks.includes(code)
      const nextPicks = exists
        ? currentGroupPicks.filter((item) => item !== code)
        : [...currentGroupPicks, code].slice(-2)

      return {
        ...current,
        [activeGroup.id]: nextPicks,
      }
    })
  }

  return (
    <section className="overflow-hidden rounded-2xl border border-white/10 bg-[#07111F] shadow-2xl shadow-black/20">
      <div className="grid lg:grid-cols-[minmax(0,0.95fr)_minmax(360px,0.55fr)]">
        <div className="relative overflow-hidden p-5 sm:p-7">
          <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(212,160,23,0.18),transparent_32%),radial-gradient(circle_at_bottom_right,rgba(46,109,180,0.16),transparent_36%)]" />
          <div className="relative">
            <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <div className="inline-flex items-center gap-2 rounded-full border border-gold-400/25 bg-gold-400/10 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-gold-200">
                  <Sparkles className="h-3.5 w-3.5" />
                  {t('quiniela.eyebrow')}
                </div>
                <h2 className="mt-4 max-w-2xl font-display text-4xl leading-none text-white sm:text-5xl">
                  {t('quiniela.title')}
                </h2>
                <p className="mt-3 max-w-2xl text-sm leading-6 text-text-secondary sm:text-base">
                  {t('quiniela.subtitle')}
                </p>
              </div>

              <div className="rounded-2xl border border-white/10 bg-white/[0.04] p-4 sm:min-w-44">
                <p className="text-xs uppercase text-text-muted">{t('quiniela.progress')}</p>
                <div className="mt-2 flex items-end gap-2">
                  <span className="font-score text-3xl font-semibold text-white">{progress}%</span>
                  <span className="pb-1 text-xs text-text-muted">{completedGroups}/{groups.length}</span>
                </div>
                <div className="mt-3 h-2 overflow-hidden rounded-full bg-white/10">
                  <div className="h-full rounded-full bg-gradient-to-r from-red-500 via-gold-400 to-green-400" style={{ width: `${progress}%` }} />
                </div>
              </div>
            </div>

            <div className="mb-5 flex gap-2 overflow-x-auto pb-1 no-scrollbar" aria-label={t('quiniela.groupTabs')}>
              {groups.map((group) => (
                <GroupTab
                  key={group.id}
                  group={group}
                  isActive={group.id === activeGroup.id}
                  isComplete={(picks[group.id] ?? []).length >= 2}
                  onClick={() => setSelectedGroup(group.id)}
                />
              ))}
            </div>

            <div className="rounded-2xl border border-white/10 bg-black/20 p-4 sm:p-5">
              <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
                <div>
                  <p className="text-xs uppercase tracking-wide text-text-muted">{activeGroup.venue}</p>
                  <h3 className="mt-1 text-2xl font-semibold text-white">{activeGroup.label}</h3>
                </div>
                <p className="text-sm text-text-secondary">{t('quiniela.selectTwo')}</p>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                {activeGroup.teams.map((team) => (
                  <TeamCard
                    key={team.code}
                    team={team}
                    selectedIndex={activePicks.indexOf(team.code)}
                    onToggle={() => togglePick(team.code)}
                  />
                ))}
              </div>
            </div>
          </div>
        </div>

        <aside className="border-t border-white/10 bg-[#0D1420] p-5 sm:p-7 lg:border-l lg:border-t-0">
          <div className="wc26-stripe mb-5" />
          <h3 className="flex items-center gap-2 text-lg font-semibold text-white">
            <Trophy className="h-5 w-5 text-gold-300" />
            {t('quiniela.summaryTitle')}
          </h3>
          <p className="mt-2 text-sm leading-6 text-text-secondary">{t('quiniela.summaryCopy')}</p>

          <div className="mt-6 grid grid-cols-2 gap-3">
            <Metric icon={<Shield className="h-4 w-4" />} label={t('quiniela.completed')} value={`${completedGroups}/${groups.length}`} />
            <Metric icon={<Users className="h-4 w-4" />} label={t('quiniela.picks')} value={String(totalPicks)} />
          </div>

          <div className="mt-6 space-y-2">
            {phases.map((phase, index) => (
              <PhaseStep key={phase} label={t(phase)} step={index + 1} isActive={index === 0} />
            ))}
          </div>

          <div className="mt-6 rounded-2xl border border-white/10 bg-black/20 p-4">
            <p className="mb-3 flex items-center gap-2 text-sm font-semibold text-white">
              <Crown className="h-4 w-4 text-gold-300" />
              {t('quiniela.qualifiers')}
            </p>
            <div className="space-y-2">
              {projectedQualifiers.length === 0 && (
                <p className="text-sm text-text-muted">{t('quiniela.emptyQualifiers')}</p>
              )}
              {projectedQualifiers.slice(0, 8).map((pick) => (
                <div key={`${pick.group}-${pick.code}`} className="flex items-center justify-between rounded-lg bg-white/[0.035] px-3 py-2 text-sm">
                  <span className="truncate text-text-secondary">
                    {pick.team}
                  </span>
                  <span className="font-score text-xs text-gold-300">
                    {pick.group}{pick.rank}
                  </span>
                </div>
              ))}
            </div>
          </div>

          <button type="button" className="btn-gold mt-6 w-full py-3 text-sm">
            {t('quiniela.continue')}
            <ArrowRight className="h-4 w-4" />
          </button>
        </aside>
      </div>
    </section>
  )
}

function GroupTab({
  group,
  isActive,
  isComplete,
  onClick,
}: Readonly<{ group: Group; isActive: boolean; isComplete: boolean; onClick: () => void }>) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex min-w-24 items-center justify-center gap-2 rounded-full border px-4 py-2 text-sm font-semibold transition-colors',
        isActive
          ? 'border-gold-400 bg-gold-400 text-blue-950'
          : 'border-white/10 bg-white/[0.035] text-text-secondary hover:border-gold-400/40 hover:text-white',
      )}
    >
      {isComplete ? <CheckCircle2 className="h-4 w-4" /> : <Circle className="h-4 w-4" />}
      {group.label.replace('Grupo ', '')}
    </button>
  )
}

function TeamCard({
  team,
  selectedIndex,
  onToggle,
}: Readonly<{ team: Team; selectedIndex: number; onToggle: () => void }>) {
  const selected = selectedIndex >= 0
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        'group rounded-2xl border p-4 text-left transition-all hover:-translate-y-0.5',
        selected
          ? 'border-gold-400/70 bg-gold-400/12 shadow-lg shadow-gold-400/10'
          : 'border-white/10 bg-white/[0.035] hover:border-white/25 hover:bg-white/[0.06]',
      )}
    >
      <div className="flex items-center gap-3">
        <span className={cn('grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-gradient-to-br text-sm font-black text-blue-950', team.accent)}>
          {team.code}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-base font-semibold text-white">{team.name}</span>
          <span className="mt-0.5 block text-xs text-text-muted">{team.seed}</span>
        </span>
        <span className={cn('grid h-8 w-8 place-items-center rounded-full border text-xs font-bold', selected ? 'border-gold-400 bg-gold-400 text-blue-950' : 'border-white/15 text-text-muted')}>
          {selected ? selectedIndex + 1 : '+'}
        </span>
      </div>
    </button>
  )
}

function PhaseStep({
  label,
  step,
  isActive,
}: Readonly<{ label: string; step: number; isActive: boolean }>) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-white/10 bg-white/[0.035] p-3">
      <span className={cn('grid h-8 w-8 place-items-center rounded-full text-xs font-bold', isActive ? 'bg-gold-400 text-blue-950' : 'bg-white/10 text-text-muted')}>
        {step}
      </span>
      <span className="text-sm font-medium text-text-primary">{label}</span>
    </div>
  )
}

function Metric({ icon, label, value }: Readonly<{ icon: React.ReactNode; label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.035] p-4">
      <div className="mb-3 text-gold-300">{icon}</div>
      <p className="text-xs uppercase text-text-muted">{label}</p>
      <p className="mt-1 font-score text-2xl font-semibold text-white">{value}</p>
    </div>
  )
}
