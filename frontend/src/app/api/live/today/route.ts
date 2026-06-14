import { NextResponse } from "next/server";

const BASE_URL = "https://v3.football.api-sports.io";

// ── API-Football v3 response types ────────────────────────────────────────────

interface AFTeam {
  id: number;
  name: string;
  logo: string;
}

interface AFStatus {
  short: string; // "NS", "1H", "HT", "2H", "FT", "CANC", "PST", …
  elapsed: number | null;
}

interface AFFixture {
  id: number;
  date: string;
  status: AFStatus;
  venue: { name: string | null; city: string | null };
}

interface AFGoals {
  home: number | null;
  away: number | null;
}

interface AFLeague {
  round: string;
}

interface AFItem {
  fixture: AFFixture;
  league: AFLeague;
  teams: { home: AFTeam; away: AFTeam };
  goals: AFGoals;
}

interface AFResponse {
  results: number;
  response: AFItem[];
  errors: unknown;
}

// ── Public shape returned to the browser ─────────────────────────────────────

export interface TodayFixture {
  id: number;
  homeTeam: string;
  awayTeam: string;
  homeLogo: string;
  awayLogo: string;
  homeScore: number | null;
  awayScore: number | null;
  status: string;
  elapsed: number | null;
  kickoffAt: string;
  round: string;
  venue: string | null;
}

// ── Route handler ─────────────────────────────────────────────────────────────

export async function GET(request: Request): Promise<NextResponse> {
  const apiKey = process.env.FOOTBALL_API_KEY;
  const leagueId = process.env.FOOTBALL_LEAGUE_ID ?? "1";
  const season = process.env.FOOTBALL_SEASON ?? "2026";

  if (!apiKey) {
    // Key not configured — return empty list so the page still renders.
    return NextResponse.json({ fixtures: [] });
  }

  // Prefer the date sent by the browser (YYYY-MM-DD in the user's local timezone)
  // so that a Guatemala user at 11 PM sees today's matches, not tomorrow's UTC date.
  const clientDate = new URL(request.url).searchParams.get("date");
  const datePattern = /^\d{4}-\d{2}-\d{2}$/;

  let today: string;
  if (clientDate && datePattern.test(clientDate)) {
    today = clientDate;
  } else {
    // Fallback: use the backend system clock (UTC) — respects the system.date
    // dev param but is off by up to 6 h for Guatemala users near midnight.
    try {
      const backendUrl =
        process.env.BACKEND_INTERNAL_URL ?? "http://localhost:8080";
      const clockRes = await fetch(`${backendUrl}/api/v1/system/clock`, {
        cache: "no-store",
      });
      if (clockRes.ok) {
        const { now } = (await clockRes.json()) as { now: string };
        today = now.slice(0, 10);
      } else {
        today = new Date().toISOString().slice(0, 10);
      }
    } catch {
      today = new Date().toISOString().slice(0, 10);
    }
  }

  const url = new URL("/fixtures", BASE_URL);
  url.searchParams.set("date", today);
  url.searchParams.set("league", leagueId);
  url.searchParams.set("season", season);

  let upstream: Response;
  try {
    upstream = await fetch(url.toString(), {
      headers: { "x-apisports-key": apiKey },
      next: { revalidate: 30 }, // CDN/ISR cache — matches can start anytime
    });
  } catch (err) {
    console.error("[live/today] fetch error", err);
    return NextResponse.json({ fixtures: [] }, { status: 200 });
  }

  if (!upstream.ok) {
    console.error("[live/today] upstream error", upstream.status);
    return NextResponse.json({ fixtures: [] }, { status: 200 });
  }

  const data: AFResponse = await upstream.json();

  const fixtures: TodayFixture[] = (data.response ?? []).map((item) => ({
    id: item.fixture.id,
    homeTeam: item.teams.home.name,
    awayTeam: item.teams.away.name,
    homeLogo: item.teams.home.logo,
    awayLogo: item.teams.away.logo,
    homeScore: item.goals.home,
    awayScore: item.goals.away,
    status: item.fixture.status.short,
    elapsed: item.fixture.status.elapsed,
    kickoffAt: item.fixture.date,
    round: item.league.round,
    venue: item.fixture.venue?.name ?? null,
  }));

  return NextResponse.json({ fixtures });
}
