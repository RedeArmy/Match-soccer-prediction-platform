"use client";

import { useEffect, useRef } from "react";
import { useAuth } from "@clerk/nextjs";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";

export function TimezoneSync() {
  const { getToken, isSignedIn } = useAuth();
  const { timeZone } = useI18n();
  const synced = useRef(false);

  useEffect(() => {
    if (!isSignedIn || !timeZone || timeZone === "UTC" || synced.current) {
      return;
    }
    synced.current = true;
    void (async () => {
      try {
        const token = await getToken();
        if (token) {
          await api.updateMe(token, { timezone: timeZone });
        }
      } catch {
        // silent — timezone sync is best-effort
      }
    })();
  }, [isSignedIn, timeZone, getToken]);

  return null;
}
