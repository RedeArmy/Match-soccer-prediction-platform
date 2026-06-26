"use client";

import { useEffect } from "react";
import { useClerk } from "@clerk/nextjs";

export function SessionGuard() {
  const { signOut } = useClerk();

  useEffect(() => {
    function handleSessionExpired() {
      void signOut({ redirectUrl: "/sign-in" });
    }
    globalThis.addEventListener("wcq:session-expired", handleSessionExpired);
    return () => {
      globalThis.removeEventListener("wcq:session-expired", handleSessionExpired);
    };
  }, [signOut]);

  return null;
}
