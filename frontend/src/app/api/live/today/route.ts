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

// ── Helpers ───────────────────────────────────────────────────────────────────

function addDays(dateStr: string, n: number): string {
  const d = new Date(`${dateStr}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}

function mapItem(item: AFItem): TodayFixture {
  return {
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
  };
}

async function fetchDay(
  date: string,
  leagueId: string,
  season: string,
  apiKey: string,
): Promise<TodayFixture[]> {
  const url = new URL("/fixtures", BASE_URL);
  url.searchParams.set("date", date);
  url.searchParams.set("league", leagueId);
  url.searchParams.set("season", season);

  let upstream: Response;
  try {
    upstream = await fetch(url.toString(), {
      headers: { "x-apisports-key": apiKey },
      next: { revalidate: 30 },
    });
  } catch (err) {
    console.error("[live/today] fetch error for", date, err);
    return [];
  }

  if (!upstream.ok) {
    console.error("[live/today] upstream error for", date, upstream.status);
    return [];
  }

  const data: AFResponse = await upstream.json();
  return (data.response ?? []).map(mapItem);
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

  // Fetch two consecutive UTC days: the user's local date plus the next day.
  // A user in UTC-6 at e.g. 23:00 local sends date=Jun-14; a match kicking off
  // at 01:00 UTC Jun-15 is still "tonight" for them — that fixture lives on the
  // Jun-15 api-football date. Fetching both days and letting the client filter
  // by its local date ensures it appears correctly.
  const [primary, secondary] = await Promise.all([
    fetchDay(today, leagueId, season, apiKey),
    fetchDay(addDays(today, 1), leagueId, season, apiKey),
  ]);

  // Deduplicate by fixture id (api-football should never duplicate, but be safe)
  const seen = new Set<number>();
  const fixtures = [...primary, ...secondary].filter((f) => {
    if (seen.has(f.id)) return false;
    seen.add(f.id);
    return true;
  });

  return NextResponse.json({ fixtures });
}
