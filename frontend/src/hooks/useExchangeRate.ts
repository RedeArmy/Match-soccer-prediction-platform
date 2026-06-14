"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { PublicExchangeRate } from "@/lib/api-types";

export function useExchangeRate() {
  return useQuery<PublicExchangeRate>({
    queryKey: ["exchange-rate"],
    queryFn: () => api.getExchangeRate(),
    refetchInterval: 60_000, // refresh every 60s in the background
    staleTime: 60_000,
  });
}
