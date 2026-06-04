'use client'

import { Languages } from 'lucide-react'
import { useI18n, type Locale } from '@/lib/i18n'
import { cn } from '@/lib/utils'

const locales: Locale[] = ['es', 'en']

export function LanguageSwitcher({ compact = false }: Readonly<{ compact?: boolean }>) {
  const { locale, setLocale, t } = useI18n()

  return (
    <div
      className={cn(
        'inline-flex items-center border border-white/10 bg-white/[0.04] p-1',
        compact ? 'w-full justify-between rounded-md' : 'rounded',
      )}
      aria-label={t('common.language')}
    >
      <Languages className={cn('text-text-muted', compact ? 'ml-2 h-4 w-4' : 'h-3.5 w-3.5')} />
      <div className="flex">
        {locales.map((item) => (
          <button
            key={item}
            type="button"
            onClick={() => setLocale(item)}
            className={cn(
              'rounded px-2 py-1 text-[11px] font-semibold uppercase transition-colors',
              locale === item ? 'bg-gold-400 text-blue-950' : 'text-text-muted hover:text-text-primary',
            )}
            aria-pressed={locale === item}
          >
            {item}
          </button>
        ))}
      </div>
    </div>
  )
}
