'use client'

import Link from 'next/link'
import { useExchangeRate } from '@/hooks/useExchangeRate'
import { formatDateTime } from '@/lib/utils'
import { ExchangeRateTicker } from '@/components/exchange/RateTicker'
import { useI18n } from '@/lib/i18n'
import { ArrowRight, CircleDollarSign, ShieldCheck, Target, TrendingUp, Trophy, Users } from 'lucide-react'

export default function LandingPage() {
  const { data: rate } = useExchangeRate()
  const { t } = useI18n()

  const steps = [
    { step: '01', icon: Users, title: t('landing.step1Title'), desc: t('landing.step1Desc'), accent: 'text-blue-300' },
    { step: '02', icon: Target, title: t('landing.step2Title'), desc: t('landing.step2Desc'), accent: 'text-green-300' },
    { step: '03', icon: Trophy, title: t('landing.step3Title'), desc: t('landing.step3Desc'), accent: 'text-gold-300' },
  ]

  return (
    <div className="flex flex-col">
      <section className="hero-bg relative min-h-[78dvh] px-4 py-16">
        <div className="relative z-10 mx-auto grid max-w-7xl items-center gap-10 lg:grid-cols-[1.05fr_0.95fr]">
          <div className="space-y-7">
            <div className="inline-flex items-center gap-2 rounded border border-white/10 bg-white/[0.04] px-3 py-1.5 text-xs uppercase text-text-secondary">
              <ShieldCheck className="h-3.5 w-3.5 text-green-300" />
              {t('landing.eyebrow')}
            </div>

            <div className="space-y-4">
              <h1 className="max-w-3xl font-display text-5xl leading-[0.95] text-white sm:text-7xl">
                {t('landing.title')}
              </h1>
              <p className="max-w-2xl text-balance text-lg text-text-secondary sm:text-xl">
                {t('landing.subtitle')}
              </p>
            </div>

            <div className="flex flex-col gap-3 sm:flex-row">
              <Link href="/dashboard" className="btn-gold px-7 py-3 text-base">
                {t('landing.primary')}
                <ArrowRight className="h-4 w-4" />
              </Link>
              <Link href="/tournaments" className="btn-ghost px-7 py-3 text-base">
                {t('landing.secondary')}
              </Link>
            </div>

            {rate && (
              <div className="flex max-w-xl flex-wrap items-center gap-3 rounded border border-white/10 bg-black/18 p-3 text-sm">
                <TrendingUp className="h-4 w-4 text-gold-400" />
                <span className="text-text-secondary">{t('landing.rate')}</span>
                <span className="font-score font-semibold text-gold-300">
                  Q{Number.parseFloat(rate.sell_rate).toFixed(4)} / USD
                </span>
                <span className="text-xs text-text-muted">
                  {t('common.updated')}: {formatDateTime(rate.effective_at)}
                </span>
                {rate.stale && <span className="text-xs text-red-300">{t('common.stale')}</span>}
              </div>
            )}
          </div>

          <div className="panel p-5">
            <div className="wc26-stripe mb-5" />
            <div className="grid gap-3">
              {[
                { icon: Target, label: t('common.predictions'), value: '64+', color: 'text-green-300' },
                { icon: Trophy, label: t('common.tournaments'), value: '48', color: 'text-gold-300' },
                { icon: CircleDollarSign, label: t('common.balance'), value: 'GTQ', color: 'text-blue-300' },
              ].map(({ icon: Icon, label, value, color }) => (
                <div key={label} className="flex items-center justify-between rounded border border-white/10 bg-white/[0.035] p-4">
                  <div className="flex items-center gap-3">
                    <Icon className={`h-5 w-5 ${color}`} />
                    <span className="text-sm text-text-secondary">{label}</span>
                  </div>
                  <span className="font-score text-xl font-semibold text-white">{value}</span>
                </div>
              ))}
            </div>
            <div className="mt-5 rounded border border-white/10 bg-[#070A0F]/40 p-4">
              <p className="text-sm font-semibold text-text-primary">{t('landing.opsTitle')}</p>
              <p className="mt-1 text-sm text-text-secondary">{t('landing.opsCopy')}</p>
            </div>
          </div>
        </div>
      </section>

      <section className="px-4 py-16">
        <div className="mx-auto max-w-7xl">
          <div className="mb-8 flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
            <div>
              <p className="text-xs uppercase text-gold-300">{t('common.event')}</p>
              <h2 className="mt-2 font-display text-4xl text-white">{t('landing.howTitle')}</h2>
            </div>
            <div className="hidden h-px flex-1 bg-white/10 sm:block" />
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            {steps.map(({ step, icon: Icon, title, desc, accent }) => (
              <article key={step} className="card p-5">
                <div className="mb-5 flex items-center justify-between">
                  <span className="font-score text-xs text-text-muted">{step}</span>
                  <Icon className={`h-5 w-5 ${accent}`} />
                </div>
                <h3 className="text-lg font-semibold text-white">{title}</h3>
                <p className="mt-2 text-sm text-text-secondary">{desc}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <ExchangeRateTicker />
    </div>
  )
}
