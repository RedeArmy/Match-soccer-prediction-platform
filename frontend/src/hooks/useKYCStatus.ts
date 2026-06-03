'use client'

import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@clerk/nextjs'
import { api } from '@/lib/api'
import type { KYCProfileResponse } from '@/lib/api-types'

export function useKYCStatus() {
  const { getToken } = useAuth()

  return useQuery<KYCProfileResponse>({
    queryKey: ['kyc'],
    queryFn:  async () => {
      const token = await getToken()
      return api.getKYCStatus(token!)
    },
    staleTime: 60_000,
  })
}
