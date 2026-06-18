import { NextResponse } from "next/server";

// Re-export the minimum shape the standings component needs.
export interface PublicMatch {
  id: number;
  home_team: string;
  away_team: string;
  home_score: number | null;
  away_score: number | null;
  status: string;
  group_label: string | null;
}

export async function GET(): Promise<NextResponse> {
  const base =
    process.env.BACKEND_INTERNAL_URL ?? "http://localhost:8080";

  try {
    const res = await fetch(`${base}/api/public/matches/group-stage`, {
      next: { revalidate: 60 },
    });
    if (!res.ok) {
      return NextResponse.json({ matches: [] });
    }
    const matches: PublicMatch[] = await res.json();
    return NextResponse.json({ matches });
  } catch {
    return NextResponse.json({ matches: [] });
  }
}
