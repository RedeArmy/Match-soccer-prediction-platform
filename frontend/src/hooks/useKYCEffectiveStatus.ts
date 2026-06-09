'use client'

import { useAuth } from '@clerk/nextjs'
import { useQuery } from '@tanstack/react-query'
import { useKYCStatus } from './useKYCStatus'
import { api } from '@/lib/api'
import type { KYCStatus } from '@/lib/api-types'

/**
 * Extends useKYCStatus with a derived `effectiveStatus` and `hasPendingReview`
 * flag. When the KYC profile is in the "pending" state AND the user already has
 * documents stored on the server, the effective status is promoted to
 * "under_review" so that steppers and badges reflect step 3 without waiting for
 * an admin to manually transition the record.
 *
 * Both the KYC page and the dashboard use this hook so the badge and stepper
 * remain consistent regardless of which page the user visits first.
 *
 * The kyc-documents query shares its React Query cache key with the KYC page,
 * so a prior navigation to /kyc means no extra network request here.
 */
export function useKYCEffectiveStatus() {
  const { getToken } = useAuth()
  const { data: kyc, isLoading } = useKYCStatus()

  const { data: docs, isLoading: isDocsLoading } = useQuery({
    queryKey: ['kyc-documents'],
    queryFn: async () => {
      const token = await getToken()
      return api.getKYCDocuments(token!)
    },
    enabled: kyc?.status === 'pending',
    staleTime: 30_000,
  })

  // Docs have been uploaded but admin hasn't moved the profile to under_review yet.
  const hasPendingReview =
    kyc?.status === 'pending' && !isDocsLoading && (docs?.length ?? 0) > 0

  const effectiveStatus: KYCStatus = hasPendingReview
    ? 'under_review'
    : (kyc?.status ?? 'unverified')

  return { kyc, docs, isLoading, isDocsLoading, hasPendingReview, effectiveStatus }
}
