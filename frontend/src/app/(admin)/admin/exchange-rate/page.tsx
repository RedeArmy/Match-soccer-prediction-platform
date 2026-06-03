'use client'

import { useState } from 'react'
import { useAuth } from '@clerk/nextjs'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { formatDateTime, formatRate } from '@/lib/utils'
import { LoadingState, LoadingSpinner } from '@/components/shared/LoadingState'
import { StatusBadge } from '@/components/shared/StatusBadge'
import {
  LineChart, Line, XAxis, YAxis, Tooltip, Legend,
  ResponsiveContainer, ReferenceLine,
} from 'recharts'

export default function AdminExchangeRatePage() {
  const { getToken } = useAuth()
  const qc = useQueryClient()
  const [overrideRate, setOverrideRate] = useState('')
  const [overrideReason, setOverrideReason] = useState('')
  const [formError, setFormError] = useState('')

  const { data: rate, isLoading } = useQuery({
    queryKey: ['admin-rate'],
    queryFn:  async () => {
      const token = await getToken()
      return api.adminGetExchangeRate(token!)
    },
    refetchInterval: 60_000,
  })

  const { data: history } = useQuery({
    queryKey: ['admin-rate-history'],
    queryFn:  async () => {
      const token = await getToken()
      return api.adminGetExchangeRateHistory(token!)
    },
  })

  const override = useMutation({
    mutationFn: async () => {
      const token = await getToken()
      return api.adminOverrideExchangeRate(token!, {
        reference_rate: overrideRate,
        reason:         overrideReason,
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-rate'] })
      qc.invalidateQueries({ queryKey: ['admin-rate-history'] })
      setOverrideRate('')
      setOverrideReason('')
    },
    onError: (e: Error) => setFormError(e.message),
  })

  const refresh = useMutation({
    mutationFn: async () => {
      const token = await getToken()
      return api.adminRefreshExchangeRate(token!)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-rate'] })
    },
  })

  const chartData = history?.data.map(e => ({
    date:      new Date(e.effective_at).toLocaleDateString('es-GT'),
    reference: parseFloat(e.reference_rate),
    compra:    parseFloat(e.buy_rate),
    venta:     parseFloat(e.sell_rate),
    override:  e.is_override,
  })).reverse() ?? []

  const overrideDates = chartData.filter(d => d.override).map(d => d.date)

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl text-white">TIPO DE CAMBIO</h1>

      {isLoading && <LoadingState rows={3} />}
      {!isLoading && rate && (
        <div className="grid sm:grid-cols-3 gap-4">
          {[
            { label: 'Referencia',  value: formatRate(rate.reference_rate), color: 'text-text-primary' },
            { label: 'Compra (buy)',  value: `Q${formatRate(rate.buy_rate)}`,  color: 'text-green-400'   },
            { label: 'Venta (sell)', value: `Q${formatRate(rate.sell_rate)}`, color: 'text-gold-400'    },
          ].map(({ label, value, color }) => (
            <div key={label} className="card p-5">
              <p className="text-xs text-text-muted mb-1">{label}</p>
              <p className={`font-score text-2xl font-semibold ${color}`}>{value}</p>
              <p className="text-xs text-text-muted mt-2">
                Fuente: <span className="text-text-secondary">{rate.source}</span>
              </p>
            </div>
          ))}
        </div>
      )}

      {rate && (
        <div className="flex items-center gap-3 text-xs text-text-muted">
          <span>Actualizado: {formatDateTime(rate.fetched_at)}</span>
          {rate.stale && <StatusBadge status="pending" size="sm" />}
          <button
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending}
            className="btn-ghost text-xs px-3 py-1 flex items-center gap-1"
          >
            {refresh.isPending ? <LoadingSpinner size={14} /> : '↻ Actualizar ahora'}
          </button>
        </div>
      )}

      {/* History chart */}
      {chartData.length > 0 && (
        <div className="card p-5 space-y-3">
          <h2 className="text-sm font-semibold text-text-secondary">Historial 30 días</h2>
          <ResponsiveContainer width="100%" height={240}>
            <LineChart data={chartData} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
              <XAxis dataKey="date" tick={{ fontSize: 10, fill: '#5A6A7E' }} tickLine={false} />
              <YAxis tick={{ fontSize: 10, fill: '#5A6A7E' }} tickLine={false} domain={['auto', 'auto']} />
              <Tooltip
                contentStyle={{ background: '#0D1F3C', border: '1px solid #1A3A6B', borderRadius: 8, fontSize: 12 }}
                labelStyle={{ color: '#A8B8CC' }}
              />
              <Legend wrapperStyle={{ fontSize: 11 }} />
              {overrideDates.map(d => (
                <ReferenceLine key={d} x={d} stroke="#D4A017" strokeDasharray="3 3" />
              ))}
              <Line type="monotone" dataKey="reference" stroke="#A8B8CC" dot={false} strokeWidth={1.5} />
              <Line type="monotone" dataKey="compra"    stroke="#27AE60" dot={false} strokeWidth={1.5} />
              <Line type="monotone" dataKey="venta"     stroke="#D4A017" dot={false} strokeWidth={1.5} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}

      {/* Override form */}
      <div className="card p-6 space-y-4">
        <h2 className="text-sm font-semibold text-text-secondary">Override manual</h2>

        <div className="grid sm:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">
              Tasa de referencia (ej. 7.8500)
            </label>
            <input
              type="number"
              min="1" max="20" step="0.0001"
              value={overrideRate}
              onChange={e => setOverrideRate(e.target.value)}
              placeholder="7.8500"
              className="input-base"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">
              Razón (mín. 10 caracteres)
            </label>
            <input
              type="text"
              value={overrideReason}
              onChange={e => setOverrideReason(e.target.value)}
              placeholder="Corrección manual por..."
              className="input-base"
            />
          </div>
        </div>

        {formError && <p className="text-red-400 text-sm">{formError}</p>}

        <button
          onClick={() => {
            if (overrideReason.length < 10) { setFormError('La razón debe tener al menos 10 caracteres'); return }
            setFormError('')
            override.mutate()
          }}
          disabled={override.isPending || !overrideRate || overrideReason.length < 10}
          className="btn-gold flex items-center gap-2"
        >
          {override.isPending ? <LoadingSpinner size={18} /> : 'Guardar override'}
        </button>
      </div>
    </div>
  )
}
