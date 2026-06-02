'use client'

import { useAuth } from '@clerk/nextjs'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { LoadingState } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { Activity, Wifi, AlertTriangle } from 'lucide-react'

export default function AdminObservabilityPage() {
  const { getToken } = useAuth()

  const { data: sseStats, isLoading: sseLoading } = useQuery({
    queryKey: ['admin-sse-stats'],
    queryFn:  async () => {
      const token = await getToken()
      return api.adminGetSSEStats(token!)
    },
    refetchInterval: 15_000,
  })

  const { data: breakers, isLoading: breakersLoading } = useQuery({
    queryKey: ['admin-circuit-breakers'],
    queryFn:  async () => {
      const token = await getToken()
      return api.adminGetCircuitBreakers(token!)
    },
    refetchInterval: 30_000,
  })

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl text-white">OBSERVABILIDAD</h1>

      {/* SSE stats */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide flex items-center gap-2">
          <Wifi className="w-4 h-4 text-blue-400" />
          SSE Hub
        </h2>
        {sseLoading ? (
          <LoadingState rows={1} />
        ) : sseStats ? (
          <div className="grid sm:grid-cols-3 gap-4">
            {[
              { label: 'Usuarios conectados', value: sseStats.connected_users },
              { label: 'Conexiones totales',  value: sseStats.total_connections },
              { label: 'Eventos descartados', value: sseStats.dropped_events },
            ].map(({ label, value }) => (
              <div key={label} className="card p-4">
                <p className="text-xs text-text-muted">{label}</p>
                <p className="font-score text-2xl text-white">{value}</p>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-text-muted text-sm">No disponible</p>
        )}
      </section>

      {/* Circuit breakers */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide flex items-center gap-2">
          <Activity className="w-4 h-4 text-gold-400" />
          Circuit Breakers
        </h2>
        {breakersLoading ? (
          <LoadingState rows={2} />
        ) : !breakers?.length ? (
          <p className="text-text-muted text-sm">Sin circuit breakers registrados</p>
        ) : (
          <div className="card divide-y divide-blue-800/50">
            {breakers.map(b => (
              <div key={b.name} className="flex items-center justify-between gap-3 px-4 py-3">
                <div>
                  <p className="text-sm font-mono text-text-primary">{b.name}</p>
                  <p className="text-xs text-text-muted">Fallos: {b.failures}</p>
                </div>
                <StatusBadge
                  status={b.state === 'closed' ? 'approved' : b.state === 'open' ? 'rejected' : 'pending'}
                  size="sm"
                />
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Link to Grafana/Prometheus */}
      <section className="card p-4 flex items-center gap-3 text-sm text-text-secondary">
        <AlertTriangle className="w-4 h-4 text-gold-400 shrink-0" />
        <span>
          Para métricas detalladas, errores de tracing y logs, accede al stack de{' '}
          <strong className="text-text-primary">Grafana / Prometheus</strong> configurado en{' '}
          <code className="text-xs bg-blue-800 px-1 rounded">docker-compose.observability.yml</code>
        </span>
      </section>
    </div>
  )
}
