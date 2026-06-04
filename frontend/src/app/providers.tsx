'use client'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { I18nProvider } from '@/lib/i18n'

export function Providers({ children }: Readonly<{ children: React.ReactNode }>) {
  const [queryClient] = useState(() =>
    new QueryClient({
      defaultOptions: {
        queries: {
          staleTime:            60_000,
          gcTime:               300_000,
          retry:                1,
          refetchOnWindowFocus: false,
        },
      },
    })
  )

  return (
    <QueryClientProvider client={queryClient}>
      <I18nProvider>{children}</I18nProvider>
    </QueryClientProvider>
  )
}
