"use client";

import { useEffect, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";

// useSlots fetches tournament bracket slots and keeps them fresh every 30 s.
//
// When new slots are confirmed (auto_source → team mapping grows), the hook
// invalidates ["matches"] so that PredictionPanel and other match consumers
// immediately re-fetch and see the propagated team names without waiting for
// their own poll interval.
export function useSlots() {
  const qc = useQueryClient();
  const confirmedRef = useRef(new Set<string>());

  const query = useQuery({
    queryKey: ["tournament-slots"],
    queryFn: () => api.getSlots(),
    staleTime: 15_000,
    refetchInterval: 30_000,
  });

  useEffect(() => {
    const slots = query.data ?? [];
    let hasNew = false;
    for (const s of slots) {
      if (s.auto_source && s.team && !confirmedRef.current.has(s.auto_source)) {
        hasNew = true;
        confirmedRef.current.add(s.auto_source);
      }
    }
    if (hasNew) {
      void qc.invalidateQueries({ queryKey: ["matches"] });
    }
  }, [query.data, qc]);

  const teamByAutoSource = useMemo(() => {
    const map = new Map<string, string>();
    for (const s of query.data ?? []) {
      if (s.auto_source && s.team) map.set(s.auto_source, s.team);
    }
    return map;
  }, [query.data]);

  return {
    slots: query.data ?? [],
    teamByAutoSource,
    isLoading: query.isLoading,
  };
}
