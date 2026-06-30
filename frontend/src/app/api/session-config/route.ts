import { NextResponse } from "next/server";

const FALLBACK_SECS = Number(
  process.env.NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS ?? 0,
);

export async function GET(): Promise<NextResponse> {
  const base = process.env.BACKEND_INTERNAL_URL ?? "http://localhost:8080";

  try {
    const res = await fetch(`${base}/api/session-config`, {
      next: { revalidate: 30 },
    });
    if (!res.ok) {
      return NextResponse.json({
        session_max_age_seconds: FALLBACK_SECS || 604800,
      });
    }
    const data = (await res.json()) as { session_max_age_seconds: number };
    return NextResponse.json(data);
  } catch {
    return NextResponse.json({
      session_max_age_seconds: FALLBACK_SECS || 604800,
    });
  }
}
