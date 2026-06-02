import { cn } from '@/lib/utils'

interface EmptyStateProps {
  title:       string
  description?: string
  action?:     React.ReactNode
  className?:  string
  icon?:       React.ReactNode
}

export function EmptyState({ title, description, action, className, icon }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center py-16 text-center', className)}>
      {icon && <div className="mb-4 text-blue-500">{icon}</div>}
      <h3 className="text-lg font-semibold text-text-primary mb-1">{title}</h3>
      {description && (
        <p className="text-sm text-text-secondary max-w-xs">{description}</p>
      )}
      {action && <div className="mt-6">{action}</div>}
    </div>
  )
}
