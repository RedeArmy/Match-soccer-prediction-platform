import { NextRequest, NextResponse } from "next/server";

const BASE_URL = "https://v3.football.api-sports.io";

// ── API-Football v3 response types ────────────────────────────────────────────

interface AFPlayerItem {
  player: {
    id: number;
    name: string;
    number: number;
    pos: string; // "G" | "D" | "M" | "F"
  };
}

interface AFLineup {
  team: { name: string };
  formation: string;
  startXI: AFPlayerItem[];
  substitutes: AFPlayerItem[];
}

interface AFEvent {
  time: { elapsed: number; extra: number | null };
  team: { name: string };
  player: { name: string };
  assist: { name: string | null } | null;
  type: string; // "Goal" | "Card" | "subst" | "Var"
  detail: string; // "Normal Goal" | "Yellow Card" | "Red Card" | …
}

// /fixtures?id=X — basic match data only (id must be used alone)
interface AFFixtureBase {
  fixture: {
    id: number;
    date: string;
    status: { short: string; elapsed: number | null };
    venue: { name: string | null; city: string | null };
  };
  league: { round: string; country: string };
  teams: {
    home: { name: string; logo: string };
    away: { name: string; logo: string };
  };
  goals: { home: number | null; away: number | null };
  score: { halftime: { home: number | null; away: number | null } };
}

// ── Public shape returned to the browser ─────────────────────────────────────

export interface FixturePlayer {
  name: string;
  number: number;
  pos: string;
}

export interface FixtureLineup {
  teamName: string;
  formation: string;
  startXI: FixturePlayer[];
  substitutes: FixturePlayer[];
}

export interface FixtureEvent {
  elapsed: number;
  team: string;
  player: string;
  assist: string | null;
  type: string;
  detail: string;
}

export interface FixtureDetail {
  id: number;
  homeTeam: string;
  awayTeam: string;
  homeLogo: string;
  awayLogo: string;
  homeScore: number | null;
  awayScore: number | null;
  halftimeHome: number | null;
  halftimeAway: number | null;
  status: string;
  elapsed: number | null;
  kickoffAt: string;
  round: string;
  venue: string | null;
  city: string | null;
  country: string | null;
  lineups: FixtureLineup[];
  events: FixtureEvent[];
}

// ── Route handler ─────────────────────────────────────────────────────────────

type Context = { params: Promise<{ id: string }> };

export async function GET(
  _req: NextRequest,
  ctx: Context,
): Promise<NextResponse> {
  const { id } = await ctx.params;

  const fixtureId = Number(id);
  if (!Number.isInteger(fixtureId) || fixtureId <= 0) {
    return NextResponse.json({ error: "Invalid fixture id" }, { status: 400 });
  }

  const apiKey = process.env.FOOTBALL_API_KEY;
  if (!apiKey) {
    return NextResponse.json(
      { error: "Live data unavailable" },
      { status: 503 },
    );
  }

  const af = { "x-apisports-key": apiKey };

  // API-Football v3: the `id` param must be used alone on /fixtures.
  // Events and lineups live on their own sub-endpoints.
  const fixtureUrl = new URL("/fixtures", BASE_URL);
  fixtureUrl.searchParams.set("id", String(fixtureId));

  const eventsUrl = new URL("/fixtures/events", BASE_URL);
  eventsUrl.searchParams.set("fixture", String(fixtureId));

  const lineupsUrl = new URL("/fixtures/lineups", BASE_URL);
  lineupsUrl.searchParams.set("fixture", String(fixtureId));

  let fixtureRes: Response, eventsRes: Response, lineupsRes: Response;
  try {
    [fixtureRes, eventsRes, lineupsRes] = await Promise.all([
      fetch(fixtureUrl.toString(), { headers: af, cache: "no-store" }),
      fetch(eventsUrl.toString(), { headers: af, cache: "no-store" }),
      fetch(lineupsUrl.toString(), { headers: af, cache: "no-store" }),
    ]);
  } catch (err) {
    console.error("[live/fixture] fetch error", fixtureId, err);
    return NextResponse.json(
      { error: "Upstream unavailable" },
      { status: 502 },
    );
  }

  if (!fixtureRes.ok) {
    const body = await fixtureRes.text().catch(() => "");
    console.error("[live/fixture] upstream error", fixtureId, fixtureRes.status, body);
    return NextResponse.json(
      { error: "Upstream error" },
      { status: fixtureRes.status },
    );
  }

  const fixtureData: { response: AFFixtureBase[] } = await fixtureRes.json();
  const item = fixtureData.response?.[0];
  if (!item) {
    console.error(
      "[live/fixture] empty response from API-Football",
      fixtureId,
      JSON.stringify(fixtureData),
    );
    return NextResponse.json({ error: "Fixture not found" }, { status: 404 });
  }

  // Events and lineups are best-effort — if the sub-endpoint fails or the plan
  // doesn't include them, we return the base fixture data with empty arrays.
  let rawEvents: AFEvent[] = [];
  let rawLineups: AFLineup[] = [];

  if (eventsRes.ok) {
    const d: { response?: AFEvent[] } = await eventsRes.json().catch(() => ({}));
    rawEvents = d.response ?? [];
  }
  if (lineupsRes.ok) {
    const d: { response?: AFLineup[] } = await lineupsRes.json().catch(() => ({}));
    rawLineups = d.response ?? [];
  }

  const lineups: FixtureLineup[] = rawLineups.map((l) => ({
    teamName: l.team?.name ?? "",
    formation: l.formation ?? "",
    startXI: (l.startXI ?? []).map((p) => ({
      name: p.player?.name ?? "",
      number: p.player?.number ?? 0,
      pos: p.player?.pos ?? "",
    })),
    substitutes: (l.substitutes ?? []).map((p) => ({
      name: p.player?.name ?? "",
      number: p.player?.number ?? 0,
      pos: p.player?.pos ?? "",
    })),
  }));

  const events: FixtureEvent[] = rawEvents.map((ev) => ({
    elapsed: ev.time?.elapsed ?? 0,
    team: ev.team?.name ?? "",
    player: ev.player?.name ?? "",
    assist: ev.assist?.name ?? null,
    type: ev.type ?? "",
    detail: ev.detail ?? "",
  }));

  const detail: FixtureDetail = {
    id: item.fixture.id,
    homeTeam: item.teams.home.name,
    awayTeam: item.teams.away.name,
    homeLogo: item.teams.home.logo,
    awayLogo: item.teams.away.logo,
    homeScore: item.goals.home,
    awayScore: item.goals.away,
    halftimeHome: item.score?.halftime?.home ?? null,
    halftimeAway: item.score?.halftime?.away ?? null,
    status: item.fixture.status.short,
    elapsed: item.fixture.status.elapsed,
    kickoffAt: item.fixture.date,
    round: item.league.round,
    venue: item.fixture.venue?.name ?? null,
    city: item.fixture.venue?.city ?? null,
    country: item.league.country ?? null,
    lineups,
    events,
  };

  return NextResponse.json({ fixture: detail });
}
