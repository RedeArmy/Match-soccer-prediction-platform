"use client";

import { useState, useEffect } from "react";
import { useClerk, useSession } from "@clerk/nextjs";
import { onSessionExpired } from "@/lib/session-bus";

// JavaScript's setTimeout stores its delay as a 32-bit signed integer.
// Values above 2^31 − 1 ms (~24.8 days) overflow to a negative number,
// firing the callback immediately. This helper chunks large delays into
// MAX_SAFE_TIMEOUT_MS slices so any positive duration works correctly.
const MAX_SAFE_TIMEOUT_MS = 2 ** 31 - 1;

function clampedTimeout(callback: () => void, ms: number): () => void {
  if (ms <= MAX_SAFE_TIMEOUT_MS) {
    const id = setTimeout(callback, ms);
    return () => clearTimeout(id);
  }
  // Delegate the active cancel via an object so the returned function
  // always cancels whichever timer is currently scheduled.
  const ref = { cancel: () => {} };
  const id = setTimeout(() => {
    ref.cancel = clampedTimeout(callback, ms - MAX_SAFE_TIMEOUT_MS);
  }, MAX_SAFE_TIMEOUT_MS);
  ref.cancel = () => clearTimeout(id);
  return () => ref.cancel();
}

export function SessionGuard() {
  const { signOut } = useClerk();
  const { session } = useSession();

  // maxAgeMs is fetched from /api/session-config on mount so it always reflects
  // the backend's live auth.session_max_age_seconds rather than the compile-time
  // NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS. The env var initialises the state to
  // avoid a "timer disabled" flash before the fetch resolves.
  const [maxAgeMs, setMaxAgeMs] = useState<number | null>(() => {
    const secs = Number(process.env.NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS ?? 0);
    return secs > 0 ? secs * 1_000 : null;
  });

  useEffect(() => {
    void fetch("/api/session-config")
      .then((r) => r.json())
      .then((data: { session_max_age_seconds?: number }) => {
        const secs = data.session_max_age_seconds ?? 0;
        setMaxAgeMs(secs > 0 ? secs * 1_000 : null);
      })
      .catch(() => {
        // Keep using the compile-time value already in state.
      });
  }, []);

  // Reactive: handle backend-initiated expiry signalled via HTTP 401.
  // api.ts calls dispatchSessionExpired() (session-bus) on any 401 response,
  // which triggers immediate sign-out even when the max-age timer has not fired.
  useEffect(() => {
    return onSessionExpired(() => {
      void signOut({ redirectUrl: "/sign-in" });
    });
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
  // setTimeout is throttled by browsers (up to 1 h effective delay) when the
  // tab is in the background, and is suspended entirely when the OS sleeps the
  // process. visibilitychange and focus listeners re-arm the timer from the real
  // current time whenever the tab returns to the foreground, ensuring that a
  // session that expired during a laptop sleep is caught immediately on wake.
  useEffect(() => {
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

    // Guard against an invalid Date object. Clerk types createdAt as Date|null;
    // null is handled above, but a Date whose getTime() returns NaN would make
    // expiresAt NaN and cause the timer to fire immediately with a wrong delay.
    if (isNaN(session.createdAt.getTime())) {
      if (process.env.NODE_ENV === "development") {
        console.warn(
          "[SessionGuard] session.createdAt is an invalid Date; proactive timer disabled. " +
            "This is unexpected — check the Clerk session object.",
        );
      }
      return;
    }

    const expiresAt = session.createdAt.getTime() + maxAgeMs;

    // arm() schedules sign-out for the remaining window, or fires immediately
    // if the session has already expired. Returns a cancel function.
    const arm = (): (() => void) => {
      const remaining = expiresAt - Date.now();
      if (remaining <= 0) {
        void signOut({ redirectUrl: "/sign-in" });
        return () => {};
      }
      return clampedTimeout(() => void signOut({ redirectUrl: "/sign-in" }), remaining);
    };

    let cancel = arm();

    // Re-arm whenever the tab becomes visible or the window regains focus.
    // Skipping the "hidden" transition avoids cancelling a running timer
    // unnecessarily when the user switches away from the tab.
    const recheck = (): void => {
      if (document.visibilityState === "hidden") return;
      cancel();
      cancel = arm();
    };

    document.addEventListener("visibilitychange", recheck);
    window.addEventListener("focus", recheck);

    return () => {
      cancel();
      document.removeEventListener("visibilitychange", recheck);
      window.removeEventListener("focus", recheck);
    };
  }, [session?.createdAt, signOut, maxAgeMs]);

  return null;
}
