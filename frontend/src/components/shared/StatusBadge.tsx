'use client'

import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

type BadgeVariant = 'active' | 'upcoming' | 'ended' | 'pending' | 'approved' | 'rejected' | 'connected' | 'reconnecting' | 'failed' | 'under_review' | 'free' | 'premium' | 'cancelled' | 'in_progress' | 'scheduled' | 'finished'

const variants: Record<BadgeVariant, string> = {
  active:       'badge-active',
  upcoming:     'badge-upcoming',
  ended:        'badge-ended',
  pending:      'badge-pending',
  approved:     'badge-approved',
  rejected:     'badge-rejected',
  connected:    'badge-approved',
  reconnecting: 'badge-upcoming',
  failed:       'badge-rejected',
  under_review: 'badge-review',
  free:         'badge-ended',
  premium:      'badge-premium',
  // Match statuses
  scheduled:    'badge-upcoming',
  in_progress:  'badge-active',
  finished:     'badge-ended',
  cancelled:    'badge-rejected',
}

interface StatusBadgeProps {
  readonly status: string
  readonly className?: string
  readonly size?: 'sm' | 'md'
}

export function StatusBadge({ status, className, size = 'md' }: StatusBadgeProps) {
  const { t } = useI18n()
  const variant = variants[status as BadgeVariant] ?? 'badge-ended'
  const i18nKey = `status.${status}`
  const translated = t(i18nKey)
  // t() returns the key itself when no entry exists; fall back to the raw status string.
  const label = translated === i18nKey ? status : translated

  return (
    <span
      className={cn(
        'inline-flex items-center rounded font-medium',
        size === 'sm' ? 'px-1.5 py-0.5 text-[10px]' : 'px-2.5 py-0.5 text-xs',
        variant,
        className,
      )}
    >
      {label}
    </span>
  )
}
