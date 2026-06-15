"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, ChevronDown, ChevronUp, Clock } from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { LoadingState } from "@/components/shared/LoadingState";
import type { TodayFixture } from "@/app/api/live/today/route";
import type {
  FixtureDetail,
  FixtureEvent,
  FixtureLineup,
} from "@/app/api/live/fixture/[id]/route";

// ── Status helpers ─────────────────────────────────────────────────────────────

const LIVE_STATUSES = new Set(["1H", "HT", "2H", "ET", "PEN_LIVE", "BT"]);
const DONE_STATUSES = new Set(["FT", "AET", "PEN"]);

function isLive(status: string) {
  return LIVE_STATUSES.has(status);
}
function isDone(status: string) {
  return DONE_STATUSES.has(status);
}

// ── Polling interval computation ───────────────────────────────────────────────
//
// State machine:
//  LIVE        → fast poll every 30s
//  PRE_MATCH   → slow poll every 60s (starts 10 min before kickoff)
//  IDLE/DONE   → false (no polling; last data stays on screen until page reload)

const LIVE_POLL_MS = 30_000;
const PRE_MATCH_POLL_MS = 60_000;
const PRE_MATCH_WINDOW_MS = 10 * 60 * 1_000; // 10 min
const MAX_SLEEP_MS = 60 * 60 * 1_000; // 1 h cap against clock skew

export function computeFeedInterval(
  fixtures: TodayFixture[],
  now: number,
): number | false {
  if (fixtures.length === 0) return false;

  // At least one live match → fast poll
  if (fixtures.some((f) => isLive(f.status))) return LIVE_POLL_MS;

  // Upcoming = not started and not finished
  const upcoming = fixtures.filter(
    (f) => !isLive(f.status) && !isDone(f.status),
  );

  // All done (or only cancelled/postponed) → stop polling, keep last data
  if (upcoming.length === 0) return false;

  const kickoffTimes = upcoming
    .map((f) => new Date(f.kickoffAt).getTime())
    .filter((t) => !Number.isNaN(t));
  if (kickoffTimes.length === 0) return false;

  const msToKickoff = Math.min(...kickoffTimes) - now;

  // Inside the 10-min pre-match window (or kickoff already past but still NS)
  if (msToKickoff <= PRE_MATCH_WINDOW_MS) return PRE_MATCH_POLL_MS;

  // Sleep until the window opens; cap at 1 h to guard against stale data
  return Math.min(msToKickoff - PRE_MATCH_WINDOW_MS, MAX_SLEEP_MS);
}

// ── Event icon ─────────────────────────────────────────────────────────────────

function EventIcon({
  type,
  detail,
}: Readonly<{ type: string; detail: string }>) {
  if (type === "Goal") return <span className="text-sm">⚽</span>;
  if (type === "Card" && detail.toLowerCase().includes("yellow"))
    return <span className="text-sm">🟨</span>;
  if (type === "Card") return <span className="text-sm">🟥</span>;
  if (type === "subst") return <span className="text-sm">🔄</span>;
  return <span className="text-sm">📋</span>;
}

// ── Lineup panel ───────────────────────────────────────────────────────────────

interface LineupPanelProps {
  readonly lineup: FixtureLineup;
}

function LineupPanel({ lineup }: LineupPanelProps) {
  const { t, teamName } = useI18n();

  const posSortOrder: Record<string, number> = { G: 0, D: 1, M: 2, F: 3 };
  const sorted = [...lineup.startXI].sort(
    (a, b) => (posSortOrder[a.pos] ?? 9) - (posSortOrder[b.pos] ?? 9),
  );

  return (
    <div className="flex-1 min-w-0">
      <p className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-text-muted">
        {teamName(lineup.teamName)}
        {lineup.formation && (
          <span className="ml-2 text-gold-400">{lineup.formation}</span>
        )}
      </p>
      <div className="space-y-0.5">
        {sorted.map((p) => (
          <div
            key={`${p.number}-${p.name}`}
            className="flex items-center gap-1.5 text-xs"
          >
            <span className="w-5 text-center font-bold tabular-nums text-text-muted">
              {p.number}
            </span>
            <span
              className={cn(
                "truncate",
                p.pos === "G" ? "text-gold-300" : "text-text-primary",
              )}
            >
              {p.name}
            </span>
            <span className="ml-auto shrink-0 text-[9px] text-text-muted">
              {p.pos}
            </span>
          </div>
        ))}
      </div>
      {lineup.substitutes.length > 0 && (
        <details className="mt-2">
          <summary className="cursor-pointer text-[10px] text-text-muted hover:text-text-secondary">
            {t("tournaments.liveSubs")} ({lineup.substitutes.length})
          </summary>
          <div className="mt-1 space-y-0.5 pl-2">
            {lineup.substitutes.map((p) => (
              <div
                key={`sub-${p.number}-${p.name}`}
                className="flex items-center gap-1.5 text-xs text-text-muted"
              >
                <span className="w-5 text-center tabular-nums">{p.number}</span>
                <span className="truncate">{p.name}</span>
              </div>
            ))}
          </div>
        </details>
      )}
    </div>
  );
}

// ── Events list ────────────────────────────────────────────────────────────────

function EventsList({ events }: Readonly<{ events: FixtureEvent[] }>) {
  const { t, teamName } = useI18n();

  if (events.length === 0) {
    return (
      <p className="py-4 text-center text-xs text-text-muted">
        {t("tournaments.liveNoEvents")}
      </p>
    );
  }

  return (
    <div className="space-y-1">
      {events.map((ev) => (
        <div
          key={`${ev.elapsed}-${ev.type}-${ev.player}`}
          className="flex items-center gap-2 rounded px-2 py-1.5 text-xs hover:bg-white/[0.03]"
        >
          <span className="w-8 shrink-0 text-center font-bold tabular-nums text-text-muted">
            {ev.elapsed}&apos;
          </span>
          <EventIcon type={ev.type} detail={ev.detail} />
          <div className="min-w-0 flex-1">
            <span className="font-medium text-text-primary">{ev.player}</span>
            {ev.assist && ev.type !== "subst" && (
              <span className="text-text-muted"> ({ev.assist})</span>
            )}
            {ev.type === "subst" && ev.assist && (
              <span className="text-green-400"> ↑ {ev.assist}</span>
            )}
          </div>
          <span className="shrink-0 text-[10px] text-text-muted">
            {teamName(ev.team)}
          </span>
        </div>
      ))}
    </div>
  );
}

// ── Fixture detail panel ──────────────────────────────────────────────────────

function FixtureDetailPanel({
  fixtureId,
}: Readonly<{ fixtureId: number }>) {
  const { t } = useI18n();
  const { data, isLoading, isError } = useQuery<{ fixture?: FixtureDetail }>({
    queryKey: ["live-fixture", fixtureId],
    queryFn: () =>
      fetch(`/api/live/fixture/${fixtureId}`).then((r) => {
        if (!r.ok) throw new Error(`${r.status}`);
        return r.json();
      }),
    // Only poll while the match is live; pre-match and post-match details are static.
    refetchInterval: (query) => {
      const fixture = query.state.data?.fixture;
      return fixture && isLive(fixture.status) ? LIVE_POLL_MS : false;
    },
    refetchOnWindowFocus: false,
    staleTime: 15_000,
  });

  if (isLoading)
    return (
      <div className="px-4 pb-4">
        <LoadingState rows={3} />
      </div>
    );

  // Show inline message rather than collapsing — the user clicked to see info.
  if (isError || !data?.fixture)
    return (
      <div className="border-t border-white/10 px-4 pb-4 pt-3">
        <p className="text-center text-xs text-text-muted">
          {t("tournaments.liveNoData")}
        </p>
      </div>
    );

  const { fixture } = data;

  return (
    <div className="border-t border-white/10 px-4 pb-4 pt-3 space-y-4">
      {/* Venue */}
      {fixture.venue && (
        <p className="text-center text-[11px] text-text-muted">
          📍 {fixture.venue}
        </p>
      )}

      {/* Halftime score */}
      {(fixture.halftimeHome != null || fixture.halftimeAway != null) && (
        <p className="text-center text-[11px] text-text-muted">
          {t("tournaments.liveHalftime")}: {fixture.halftimeHome ?? 0} –{" "}
          {fixture.halftimeAway ?? 0}
        </p>
      )}

      {/* Events */}
      {fixture.events.length > 0 && (
        <section>
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-text-muted">
            {t("tournaments.liveEvents")}
          </p>
          <EventsList events={fixture.events} />
        </section>
      )}

      {/* Lineups */}
      {fixture.lineups.length > 0 && (
        <section>
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-text-muted">
            {t("tournaments.liveLineups")}
          </p>
          <div className="flex gap-4">
            {fixture.lineups.map((l) => (
              <LineupPanel key={l.teamName} lineup={l} />
            ))}
          </div>
        </section>
      )}

      {fixture.lineups.length === 0 && fixture.events.length === 0 && (
        <p className="text-center text-xs text-text-muted">
          {t("tournaments.liveNoData")}
        </p>
      )}
    </div>
  );
}

// ── Match card ─────────────────────────────────────────────────────────────────

interface MatchCardProps {
  readonly fixture: TodayFixture;
  readonly expanded: boolean;
  readonly onToggle: () => void;
}

function MatchCard({ fixture, expanded, onToggle }: MatchCardProps) {
  const { t, teamName } = useI18n();

  const live = isLive(fixture.status);
  const done = isDone(fixture.status);
  const kickoff = new Date(fixture.kickoffAt);
  const timeStr = kickoff.toLocaleTimeString("es-GT", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: true,
  });

  function statusLabel(): string {
    if (!live && !done) return "";
    if (fixture.status === "HT") return t("tournaments.liveHT");
    if (live && fixture.elapsed != null) return `${fixture.elapsed}'`;
    if (live) return t("tournaments.liveInPlay");
    return "FT";
  }

  let chipClass: string;
  if (live) {
    chipClass = "bg-green-500/20 text-green-300";
  } else if (done) {
    chipClass = "bg-white/10 text-text-muted";
  } else {
    chipClass = "bg-blue-500/10 text-blue-300";
  }

  return (
    <div
      className={cn(
        "overflow-hidden rounded-xl border transition-colors",
        live
          ? "border-green-500/30 bg-green-500/5"
          : "border-white/10 bg-white/[0.02] hover:border-white/20",
      )}
    >
      <button
        type="button"
        className="w-full p-3 text-left"
        onClick={onToggle}
        aria-expanded={expanded}
      >
        <div className="flex items-center gap-2">
          {/* Live pulse */}
          {live && (
            <span className="relative flex h-2 w-2 shrink-0">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-green-400" />
            </span>
          )}

          {/* Status/time chip */}
          <span
            className={cn(
              "shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold tabular-nums",
              chipClass,
            )}
          >
            {live || done ? (
              statusLabel()
            ) : (
              <span className="flex items-center gap-1">
                <Clock className="h-2.5 w-2.5" />
                {timeStr}
              </span>
            )}
          </span>

          {/* Teams + score */}
          <div className="flex flex-1 items-center justify-between gap-2 min-w-0">
            <span className="truncate text-sm font-medium text-white">
              {teamName(fixture.homeTeam)}
            </span>

            {live || done ? (
              <span className="shrink-0 text-base font-bold tabular-nums text-white">
                {fixture.homeScore ?? 0} – {fixture.awayScore ?? 0}
              </span>
            ) : (
              <span className="shrink-0 text-xs text-text-muted">vs</span>
            )}

            <span className="truncate text-right text-sm font-medium text-white">
              {teamName(fixture.awayTeam)}
            </span>
          </div>

          {/* Expand chevron */}
          <span className="shrink-0 text-text-muted">
            {expanded ? (
              <ChevronUp className="h-4 w-4" />
            ) : (
              <ChevronDown className="h-4 w-4" />
            )}
          </span>
        </div>

        {fixture.round && (
          <p className="mt-1 pl-4 text-[10px] text-text-muted">
            {fixture.round}
          </p>
        )}
      </button>

      {expanded && <FixtureDetailPanel fixtureId={fixture.id} />}
    </div>
  );
}

// ── Main component ─────────────────────────────────────────────────────────────

export function LiveMatchFeed() {
  const { t } = useI18n();
  const [expandedId, setExpandedId] = useState<number | null>(null);

  // Track the local date so that when midnight passes and the date changes,
  // the query key changes and React Query automatically fetches the new day's
  // fixtures — even if the user left the tab open overnight and polling stopped
  // because all previous-day matches had finished.
  const [localDate, setLocalDate] = useState(() =>
    new Date().toLocaleDateString("sv"),
  );
  useEffect(() => {
    const id = setInterval(() => {
      setLocalDate(new Date().toLocaleDateString("sv"));
    }, 60_000);
    return () => clearInterval(id);
  }, []);

  const { data, isLoading, isError } = useQuery<{ fixtures: TodayFixture[] }>({
    queryKey: ["live-today", localDate],
    queryFn: () =>
      fetch(`/api/live/today?date=${localDate}`).then((r) => r.json()),
    // Always fetch on mount (page load, reload, SPA navigation) so the user
    // sees an up-to-date schedule even when no match is live.
    refetchOnMount: "always",
    // Smart interval: live→30s, pre-match window→60s, idle/done→false.
    // This controls background polling only; the mount fetch above is separate.
    refetchInterval: (query) =>
      computeFeedInterval(query.state.data?.fixtures ?? [], Date.now()),
    refetchOnWindowFocus: false,
    staleTime: 20_000,
  });

  const fixtures = data?.fixtures ?? [];

  // Filter to the user's local date and sort: live first, then upcoming by
  // kickoff time ascending, then finished. The route returns two UTC days so
  // that a UTC-6 user at 23:00 local sees matches kicking off at 01:00 UTC
  // (next UTC day but still tonight locally). Client-side filtering removes
  // any matches that genuinely belong to a different local date.
  const displayFixtures = useMemo(() => {
    const forToday = fixtures.filter(
      (f) => new Date(f.kickoffAt).toLocaleDateString("sv") === localDate,
    );
    const sortOrder = (f: TodayFixture) =>
      isLive(f.status) ? 0 : isDone(f.status) ? 2 : 1;
    return [...forToday].sort((a, b) => {
      const diff = sortOrder(a) - sortOrder(b);
      if (diff !== 0) return diff;
      return new Date(a.kickoffAt).getTime() - new Date(b.kickoffAt).getTime();
    });
  }, [fixtures, localDate]);

  return (
    <div className="panel p-5">
      <div className="mb-4 flex items-center gap-2">
        <Activity className="h-5 w-5 text-green-400" />
        <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
          {t("tournaments.liveTitle")}
        </h2>
        {displayFixtures.some((f) => isLive(f.status)) && (
          <span className="ml-auto rounded-full bg-green-500/20 px-2 py-0.5 text-[10px] font-semibold text-green-300">
            {t("tournaments.liveBadge")}
          </span>
        )}
      </div>

      {isLoading && <LoadingState rows={3} />}

      {isError && (
        <p className="py-6 text-center text-xs text-text-muted">
          {t("tournaments.liveError")}
        </p>
      )}

      {!isLoading && !isError && displayFixtures.length === 0 && (
        <p className="py-6 text-center text-xs text-text-muted">
          {t("tournaments.liveEmpty")}
        </p>
      )}

      {displayFixtures.length > 0 && (
        <div className="space-y-2">
          {displayFixtures.map((f) => (
            <MatchCard
              key={f.id}
              fixture={f}
              expanded={expandedId === f.id}
              onToggle={() =>
                setExpandedId((prev) => (prev === f.id ? null : f.id))
              }
            />
          ))}
        </div>
      )}
    </div>
  );
}
