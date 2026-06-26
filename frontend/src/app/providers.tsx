"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { I18nProvider } from "@/lib/i18n";
import { TimezoneSync } from "@/components/shared/TimezoneSync";
import { SessionGuard } from "@/components/shared/SessionGuard";

export function Providers({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 60_000,
            gcTime: 300_000,
            retry: (failureCount, error) => {
              if ((error as { status?: number })?.status === 401) return false;
              return failureCount < 1;
            },
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <SessionGuard />
        <TimezoneSync />
        {children}
      </I18nProvider>
    </QueryClientProvider>
  );
}
