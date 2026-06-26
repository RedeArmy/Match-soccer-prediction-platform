"use client";

import { useEffect } from "react";
import { useClerk } from "@clerk/nextjs";

export function SessionGuard() {
  const { signOut } = useClerk();

  useEffect(() => {
    function handleSessionExpired() {
      void signOut({ redirectUrl: "/sign-in" });
    }
    window.addEventListener("wcq:session-expired", handleSessionExpired);
    return () => {
      window.removeEventListener("wcq:session-expired", handleSessionExpired);
    };
  }, [signOut]);

  return null;
}
