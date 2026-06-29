"use client";

import { useEffect } from "react";
import { useClerk, useSession } from "@clerk/nextjs";

export function SessionGuard() {
  const { signOut } = useClerk();
  const { session } = useSession();

  // Reactive: handle backend-initiated expiry signalled via HTTP 401.
  // api.ts dispatches "wcq:session-expired" on any 401 response, which triggers
  // immediate sign-out even when the max-age timer has not yet fired.
  useEffect(() => {
    function handleSessionExpired() {
      void signOut({ redirectUrl: "/sign-in" });
    }
    globalThis.addEventListener("wcq:session-expired", handleSessionExpired);
    return () => {
      globalThis.removeEventListener("wcq:session-expired", handleSessionExpired);
    };
  }, [signOut]);

  // Proactive: schedule sign-out when session.createdAt + maxAge is reached.
  // This fires even when the user is idle and no API calls are being made.
  //
  // session.createdAt is stable across Clerk's ~60 s JWT refreshes (unlike
  // token.iat which is always recent). It mirrors the backend's fva-derived
  // SessionStartedAt for password / passkey / email-code flows. For OAuth /
  // social flows the backend derives the origin from session_starts; this timer
  // provides the same enforcement on the browser side.
  //
  // NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS is read inside the effect (rather than
  // as a module constant) to keep it patchable by vi.stubEnv in tests.
  // It must match auth.session_max_age_seconds in backend system_params.
  useEffect(() => {
    const maxAgeSecs = Number(
      process.env.NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS ?? 0,
    );
    const maxAgeMs = maxAgeSecs > 0 ? maxAgeSecs * 1_000 : null;

    if (!maxAgeMs) {
      if (process.env.NODE_ENV === "development") {
        console.warn(
          "[SessionGuard] NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS is not set or is zero. " +
            "The proactive session timer is disabled — users will only be signed out " +
            "via HTTP 401 responses. Add the variable to .env to enable it.",
        );
      }
      return;
    }

    if (!session?.createdAt) return;

    const expiresAt = session.createdAt.getTime() + maxAgeMs;
    const remaining = expiresAt - Date.now();

    if (remaining <= 0) {
      void signOut({ redirectUrl: "/sign-in" });
      return;
    }

    const timer = setTimeout(() => {
      void signOut({ redirectUrl: "/sign-in" });
    }, remaining);

    return () => clearTimeout(timer);
  }, [session?.createdAt, signOut]);

  return null;
}
