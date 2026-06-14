"use client";

import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createSSEManager, type SSEConnectionStatus } from "@/lib/sse";
import type { SSENotificationPayload } from "@/lib/api-types";

export function useSSE() {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<SSEConnectionStatus>("connecting");
  const [lastNotification, setLastNotification] =
    useState<SSENotificationPayload | null>(null);
  const managerRef = useRef<ReturnType<typeof createSSEManager> | null>(null);

  useEffect(() => {
    managerRef.current = createSSEManager({
      onNotification(payload) {
        setLastNotification(payload);

        // Invalidate relevant queries so UI refreshes automatically
        queryClient.invalidateQueries({ queryKey: ["balance"] });
        queryClient.invalidateQueries({ queryKey: ["notifications"] });

        if (payload.event_type?.startsWith("score.")) {
          queryClient.invalidateQueries({ queryKey: ["leaderboard"] });
        }
        if (payload.event_type?.startsWith("kyc.")) {
          queryClient.invalidateQueries({ queryKey: ["kyc"] });
        }
      },
      onConnectionChange: setStatus,
    });

    return () => {
      managerRef.current?.disconnect();
    };
  }, [queryClient]);

  return { status, lastNotification };
}
