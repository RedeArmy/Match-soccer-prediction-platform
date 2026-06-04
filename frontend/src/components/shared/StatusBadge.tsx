import { cn } from '@/lib/utils'

type BadgeVariant = 'active' | 'upcoming' | 'ended' | 'pending' | 'approved' | 'rejected' | 'connected' | 'reconnecting' | 'failed'

const variants: Record<BadgeVariant, string> = {
  active: 'badge-active',
  upcoming: 'badge-upcoming',
  ended: 'badge-ended',
  pending: 'badge-pending',
  approved: 'badge-approved',
  rejected: 'badge-rejected',
  connected: 'badge-approved',
  reconnecting: 'badge-upcoming',
  failed: 'badge-rejected',
}

const labels: Record<string, string> = {
  active: 'Activo',
  upcoming: 'Proximo',
  ended: 'Finalizado',
  pending: 'Pendiente',
  approved: 'Aprobado',
  rejected: 'Rechazado',
  connected: 'Conectado',
  reconnecting: 'Reconectando',
  failed: 'Error',
  in_progress: 'En curso',
  open: 'Abierto',
  finished: 'Finalizado',
  scheduled: 'Programado',
  cancelled: 'Cancelado',
  unverified: 'Sin verificar',
  submitted: 'Enviado',
  under_review: 'En revision',
}

interface StatusBadgeProps {
  readonly status: string
  readonly className?: string
  readonly size?: 'sm' | 'md'
}

export function StatusBadge({ status, className, size = 'md' }: StatusBadgeProps) {
  const variant = variants[status as BadgeVariant] ?? 'badge-ended'
  const label = labels[status] ?? status

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
