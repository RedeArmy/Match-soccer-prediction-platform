import { NextRequest, NextResponse } from 'next/server'

const BASE_URL = 'https://v3.football.api-sports.io'

// ── API-Football v3 detailed response types ────────────────────────────────────

interface AFPlayerItem {
  player: {
    id:     number
    name:   string
    number: number
    pos:    string // "G" | "D" | "M" | "F"
  }
}

interface AFLineup {
  team:        { name: string }
  formation:   string
  startXI:     AFPlayerItem[]
  substitutes: AFPlayerItem[]
}

interface AFEvent {
  time:    { elapsed: number; extra: number | null }
  team:    { name: string }
  player:  { name: string }
  assist:  { name: string | null } | null
  type:    string   // "Goal" | "Card" | "subst" | "Var"
  detail:  string   // "Normal Goal" | "Yellow Card" | "Red Card" | …
}

interface AFFixtureDetail {
  fixture: {
    id:     number
    date:   string
    status: { short: string; elapsed: number | null }
    venue:  { name: string | null; city: string | null }
  }
  league:  { round: string }
  teams:   { home: { name: string; logo: string }; away: { name: string; logo: string } }
  goals:   { home: number | null; away: number | null }
  score:   { halftime: { home: number | null; away: number | null } }
  lineups: AFLineup[]
  events:  AFEvent[]
}

interface AFDetailResponse {
  response: AFFixtureDetail[]
}

// ── Public shape returned to the browser ─────────────────────────────────────

export interface FixturePlayer {
  name:   string
  number: number
  pos:    string
}

export interface FixtureLineup {
  teamName:    string
  formation:   string
  startXI:     FixturePlayer[]
  substitutes: FixturePlayer[]
}

export interface FixtureEvent {
  elapsed: number
  team:    string
  player:  string
  assist:  string | null
  type:    string
  detail:  string
}

export interface FixtureDetail {
  id:             number
  homeTeam:       string
  awayTeam:       string
  homeLogo:       string
  awayLogo:       string
  homeScore:      number | null
  awayScore:      number | null
  halftimeHome:   number | null
  halftimeAway:   number | null
  status:         string
  elapsed:        number | null
  kickoffAt:      string
  round:          string
  venue:          string | null
  lineups:        FixtureLineup[]
  events:         FixtureEvent[]
}

// ── Route handler ─────────────────────────────────────────────────────────────

type Context = { params: Promise<{ id: string }> }

export async function GET(_req: NextRequest, ctx: Context): Promise<NextResponse> {
  const { id } = await ctx.params

  const fixtureId = Number(id)
  if (!Number.isInteger(fixtureId) || fixtureId <= 0) {
    return NextResponse.json({ error: 'Invalid fixture id' }, { status: 400 })
  }

  const apiKey = process.env.FOOTBALL_API_KEY
  if (!apiKey) {
    return NextResponse.json({ error: 'Live data unavailable' }, { status: 503 })
  }

  const url = new URL('/fixtures', BASE_URL)
  url.searchParams.set('id',      String(fixtureId))
  url.searchParams.set('events',  'true')
  url.searchParams.set('lineups', 'true')

  let upstream: Response
  try {
    upstream = await fetch(url.toString(), {
      headers: { 'x-apisports-key': apiKey },
      cache:   'no-store', // fixture detail must always be fresh
    })
  } catch (err) {
    console.error('[live/fixture] fetch error', err)
    return NextResponse.json({ error: 'Upstream unavailable' }, { status: 502 })
  }

  if (!upstream.ok) {
    return NextResponse.json({ error: 'Upstream error' }, { status: upstream.status })
  }

  const data: AFDetailResponse = await upstream.json()
  const item = data.response?.[0]
  if (!item) {
    return NextResponse.json({ error: 'Fixture not found' }, { status: 404 })
  }

  const lineups: FixtureLineup[] = (item.lineups ?? []).map((l) => ({
    teamName:  l.team.name,
    formation: l.formation,
    startXI:   l.startXI.map((p) => ({
      name:   p.player.name,
      number: p.player.number,
      pos:    p.player.pos,
    })),
    substitutes: l.substitutes.map((p) => ({
      name:   p.player.name,
      number: p.player.number,
      pos:    p.player.pos,
    })),
  }))

  const events: FixtureEvent[] = (item.events ?? []).map((ev) => ({
    elapsed: ev.time.elapsed,
    team:    ev.team.name,
    player:  ev.player.name,
    assist:  ev.assist?.name ?? null,
    type:    ev.type,
    detail:  ev.detail,
  }))

  const detail: FixtureDetail = {
    id:           item.fixture.id,
    homeTeam:     item.teams.home.name,
    awayTeam:     item.teams.away.name,
    homeLogo:     item.teams.home.logo,
    awayLogo:     item.teams.away.logo,
    homeScore:    item.goals.home,
    awayScore:    item.goals.away,
    halftimeHome: item.score.halftime.home,
    halftimeAway: item.score.halftime.away,
    status:       item.fixture.status.short,
    elapsed:      item.fixture.status.elapsed,
    kickoffAt:    item.fixture.date,
    round:        item.league.round,
    venue:        item.fixture.venue.name,
    lineups,
    events,
  }

  return NextResponse.json({ fixture: detail })
}
