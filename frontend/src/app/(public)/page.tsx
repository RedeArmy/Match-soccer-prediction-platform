import Link from 'next/link'
import { serverAPI } from '@/lib/api'
import { formatRate, formatDateTime } from '@/lib/utils'
import { ImagePlaceholder } from '@/components/shared/ImagePlaceholder'
import { UserPlus, Target, Trophy, TrendingUp } from 'lucide-react'
import { ExchangeRateTicker } from '@/components/exchange/RateTicker'

export const revalidate = 60

async function getExchangeRate() {
  try {
    return await serverAPI().getExchangeRate()
  } catch {
    return null
  }
}

export default async function LandingPage() {
  const rate = await getExchangeRate()

  return (
    <div className="flex flex-col">

      {/* ── Hero ─────────────────────────────────────────────────────────── */}
      <section className="hero-bg relative min-h-[85dvh] flex flex-col items-center justify-center text-center px-4 py-16">
        <div className="relative z-10 max-w-3xl mx-auto space-y-6">

          {/* Rate pill */}
          {rate && (
            <div className="inline-flex items-center gap-2 bg-blue-900/80 border border-blue-700 px-3 py-1.5 rounded-full text-xs text-text-secondary mb-2">
              <TrendingUp className="w-3 h-3 text-gold-400" />
              <span className="font-score">
                1 USD = <span className="text-gold-400 font-medium">Q{parseFloat(rate.sell_rate).toFixed(2)}</span> GTQ
              </span>
              {rate.stale && <span className="text-red-400 text-[10px]">desactualizado</span>}
            </div>
          )}

          <h1 className="font-display text-6xl sm:text-8xl text-white leading-none">
            QUINIELA
            <br />
            <span className="bg-gold-gradient bg-clip-text text-transparent">
              MUNDIAL 2026
            </span>
          </h1>

          <p className="text-lg sm:text-xl text-text-secondary max-w-xl mx-auto">
            Predice los resultados. Compite con tus amigos. Gana premios reales en quetzales.
          </p>

          <div className="flex flex-col sm:flex-row gap-3 justify-center pt-2">
            <Link href="/sign-up" className="btn-gold text-base px-8 py-3 inline-block">
              Regístrate Gratis
            </Link>
            <Link href="/tournaments" className="btn-ghost text-base px-8 py-3 inline-block">
              Ver Torneos
            </Link>
          </div>
        </div>

        {/* Hero banner placeholder */}
        <div className="absolute inset-0 z-0 opacity-20">
          <ImagePlaceholder
            aspectRatio="21/9"
            label="Tournament hero banner"
            dataSrc="/images/hero-banner.jpg"
            rounded={false}
            className="w-full h-full"
          />
        </div>
      </section>

      {/* ── Exchange rate strip ───────────────────────────────────────────── */}
      {rate && (
        <div className="bg-blue-900 border-y border-blue-800">
          <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8 py-3 flex flex-wrap items-center justify-between gap-2 text-sm">
            <div className="flex items-center gap-3">
              <TrendingUp className="w-4 h-4 text-gold-400" />
              <span className="text-text-secondary">Tipo de cambio:</span>
              <span className="font-score text-gold-400 font-medium">
                Q{parseFloat(rate.sell_rate).toFixed(4)} / USD
              </span>
              <span className="text-text-muted text-xs">
                (compra: Q{parseFloat(rate.buy_rate).toFixed(4)})
              </span>
            </div>
            <span className="text-text-muted text-xs">
              Actualizado: {formatDateTime(rate.effective_at)}
              {rate.stale && <span className="ml-1 text-red-400">⚠ desactualizado</span>}
            </span>
          </div>
        </div>
      )}

      {/* ── How it works ─────────────────────────────────────────────────── */}
      <section className="py-20 px-4">
        <div className="mx-auto max-w-5xl">
          <h2 className="font-display text-4xl text-center text-white mb-12">CÓMO FUNCIONA</h2>

          <div className="grid md:grid-cols-3 gap-8">
            {[
              { step: '01', icon: UserPlus, title: 'Regístrate',     desc: 'Crea tu cuenta en segundos con email o Google.' },
              { step: '02', icon: Target,   title: 'Predice',        desc: 'Elige el marcador de cada partido antes de que empiece.' },
              { step: '03', icon: Trophy,   title: 'Gana Premios',   desc: 'Acumula puntos y comparte el pozo de tu quiniela.' },
            ].map(({ step, icon: Icon, title, desc }) => (
              <div key={step} className="card p-6 flex flex-col items-center text-center gap-4">
                <ImagePlaceholder aspectRatio="1/1" label="Step illustration" className="w-20 h-20" />
                <div className="w-10 h-10 rounded-full bg-gold-500/10 border border-gold-500/30 flex items-center justify-center">
                  <Icon className="w-5 h-5 text-gold-400" />
                </div>
                <div>
                  <div className="font-score text-gold-600 text-xs mb-1">{step}</div>
                  <h3 className="font-display text-xl text-white mb-2">{title}</h3>
                  <p className="text-text-secondary text-sm">{desc}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Live rate ticker (client-side, auto-refreshes) ────────────────── */}
      <ExchangeRateTicker />

    </div>
  )
}
