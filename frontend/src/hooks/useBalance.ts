'use client'

import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@clerk/nextjs'
import { api } from '@/lib/api'
import type { BalanceResponse } from '@/lib/api-types'

export function useBalance() {
  const { getToken } = useAuth()

  return useQuery<BalanceResponse>({
    queryKey: ['balance'],
    queryFn:  async () => {
      const token = await getToken()
      return api.getBalance(token!)
    },
    staleTime: 30_000,
  })
}
