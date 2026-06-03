'use client'

import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@clerk/nextjs'
import { api } from '@/lib/api'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { LoadingState } from '@/components/shared/LoadingState'
import { EmptyState } from '@/components/shared/EmptyState'
import { ImagePlaceholder } from '@/components/shared/ImagePlaceholder'
import { formatGTQ, formatCountdown } from '@/lib/utils'
import { Trophy, Users, Clock } from 'lucide-react'
import Link from 'next/link'

export default function TournamentsPage() {
  const { getToken, isSignedIn } = useAuth()

  const { data: groups, isLoading } = useQuery({
    queryKey: ['public-groups'],
    queryFn:  async () => {
      if (isSignedIn) {
        const token = await getToken()
        return api.getMyGroups(token!)
      }
      return []
    },
  })

  return (
    <div className="py-8 px-4">
      <div className="mx-auto max-w-6xl">

        {/* Header */}
        <div className="mb-8">
          <h1 className="font-display text-4xl text-white mb-2">TORNEOS</h1>
          <p className="text-text-secondary">Explora las quinielas disponibles y únete a las que más te interesen</p>
        </div>

        {/* Content */}
        {isLoading && <LoadingState rows={6} />}
        {!isLoading && groups?.length === 0 && (
          <EmptyState
            title="No hay torneos disponibles"
            description="Vuelve pronto para ver las próximas quinielas"
            icon={<Trophy className="w-10 h-10" />}
          />
        )}
        {!isLoading && (groups?.length ?? 0) > 0 && (
          <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-5">
            {groups?.map(g => (
              <div key={g.id} className="card overflow-hidden group hover:border-blue-600/80 transition-colors">
                <ImagePlaceholder
                  aspectRatio="16/9"
                  label={`${g.name} banner`}
                  className="rounded-none"
                />

                <div className="p-5 space-y-3">
                  <div className="flex items-start justify-between gap-2">
                    <h2 className="font-display text-xl text-white leading-tight">{g.name}</h2>
                    <StatusBadge status={g.status} size="sm" />
                  </div>

                  <div className="grid grid-cols-2 gap-2 text-xs text-text-muted">
                    <div className="flex items-center gap-1.5">
                      <Trophy className="w-3 h-3 text-gold-400" />
                      <span>Pozo: {formatGTQ(g.prize_pool_cents)}</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Users className="w-3 h-3 text-blue-400" />
                      <span>{g.member_count}/{g.max_members ?? '∞'}</span>
                    </div>
                    {g.entry_fee_cents > 0 && (
                      <div className="col-span-2">
                        Inscripción: <span className="text-gold-400">{formatGTQ(g.entry_fee_cents)}</span>
                      </div>
                    )}
                    {g.deadline_at && (
                      <div className="flex items-center gap-1.5 col-span-2">
                        <Clock className="w-3 h-3 text-text-muted" />
                        <span>{formatCountdown(g.deadline_at)}</span>
                      </div>
                    )}
                  </div>

                  <Link
                    href={`/tournaments/${g.id}`}
                    className="btn-gold w-full text-center text-sm py-2 block mt-2"
                  >
                    {g.status === 'open' ? 'Entrar' : 'Ver detalles'}
                  </Link>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
