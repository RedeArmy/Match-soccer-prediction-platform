"use client";

import { useQuery } from "@tanstack/react-query";
import { useI18n } from "@/lib/i18n";
import { HorizontalCarousel } from "@/components/shared/HorizontalCarousel";
import type { PublicMatch } from "@/app/api/public/standings/route";

// ── Flag map (keyed by lowercase English team name) ───────────────────────────

const FLAGS: Record<string, string> = {
  // Group A
  mexico: "🇲🇽",
  "south africa": "🇿🇦",
  "south korea": "🇰🇷",
  czechia: "🇨🇿",
  "czech republic": "🇨🇿",
  // Group B
  canada: "🇨🇦",
  switzerland: "🇨🇭",
  japan: "🇯🇵",
  nigeria: "🇳🇬",
  // Group C
  brazil: "🇧🇷",
  morocco: "🇲🇦",
  haiti: "🇭🇹",
  scotland: "🏴󠁧󠁢󠁳󠁣󠁴󠁿",
  // Group D
  "united states": "🇺🇸",
  usa: "🇺🇸",
  paraguay: "🇵🇾",
  australia: "🇦🇺",
  tunisia: "🇹🇳",
  // Group E
  germany: "🇩🇪",
  "ivory coast": "🇨🇮",
  "cote d'ivoire": "🇨🇮",
  "côte d'ivoire": "🇨🇮",
  curaçao: "🇨🇼",
  curacao: "🇨🇼",
  ecuador: "🇪🇨",
  // Group F
  netherlands: "🇳🇱",
  sweden: "🇸🇪",
  // japan already above
  // Group G
  belgium: "🇧🇪",
  egypt: "🇪🇬",
  iran: "🇮🇷",
  "new zealand": "🇳🇿",
  // Group H
  spain: "🇪🇸",
  "cape verde": "🇨🇻",
  "saudi arabia": "🇸🇦",
  uruguay: "🇺🇾",
  // Group I
  france: "🇫🇷",
  senegal: "🇸🇳",
  iraq: "🇮🇶",
  norway: "🇳🇴",
  // Group J
  argentina: "🇦🇷",
  algeria: "🇩🇿",
  austria: "🇦🇹",
  jordan: "🇯🇴",
  // Group K
  portugal: "🇵🇹",
  "dr congo": "🇨🇩",
  uzbekistan: "🇺🇿",
  colombia: "🇨🇴",
  // Group L
  england: "🏴󠁧󠁢󠁥󠁮󠁧󠁿",
  croatia: "🇭🇷",
  ghana: "🇬🇭",
  panama: "🇵🇦",
  // misc (seed / test data)
  "bosnia and herzegovina": "🇧🇦",
  qatar: "🇶🇦",
  türkiye: "🇹🇷",
  turkiye: "🇹🇷",
};

function flag(name: string): string {
  return FLAGS[name.toLowerCase()] ?? "🏳️";
}

// ── Standings calculation ─────────────────────────────────────────────────────

interface TeamRow {
  team: string;
  played: number;
  won: number;
  drawn: number;
  lost: number;
  gf: number; // goals for
  ga: number; // goals against
  gd: number; // goal difference
  pts: number;
}

export function buildGroupStandings(
  matches: PublicMatch[],
): Record<string, TeamRow[]> {
  const groups: Record<string, Record<string, TeamRow>> = {};

  // Seed all teams (including scheduled matches so 0-stat teams appear).
  for (const m of matches) {
    if (!m.group_label) continue;
    const g = m.group_label.toUpperCase();
    if (!groups[g]) groups[g] = {};

    const init = (name: string) => {
      if (!groups[g][name]) {
        groups[g][name] = {
          team: name,
          played: 0,
          won: 0,
          drawn: 0,
          lost: 0,
          gf: 0,
          ga: 0,
          gd: 0,
          pts: 0,
        };
      }
    };
    init(m.home_team);
    init(m.away_team);

    // Accumulate only from settled matches with scores.
    if (
      (m.status === "finished" || m.status === "in_progress") &&
      m.home_score !== null &&
      m.away_score !== null
    ) {
      const h = groups[g][m.home_team];
      const a = groups[g][m.away_team];
      h.played++;
      a.played++;
      h.gf += m.home_score;
      h.ga += m.away_score;
      a.gf += m.away_score;
      a.ga += m.home_score;

      if (m.home_score > m.away_score) {
        h.won++;
        h.pts += 3;
        a.lost++;
      } else if (m.home_score < m.away_score) {
        a.won++;
        a.pts += 3;
        h.lost++;
      } else {
        h.drawn++;
        a.drawn++;
        h.pts++;
        a.pts++;
      }
      h.gd = h.gf - h.ga;
      a.gd = a.gf - a.ga;
    }
  }

  // Sort each group: pts DESC, gd DESC, gf DESC, name ASC.
  const result: Record<string, TeamRow[]> = {};
  for (const [g, teams] of Object.entries(groups)) {
    result[g] = Object.values(teams).sort((a, b) => {
      if (b.pts !== a.pts) return b.pts - a.pts;
      if (b.gd !== a.gd) return b.gd - a.gd;
      if (b.gf !== a.gf) return b.gf - a.gf;
      return a.team.localeCompare(b.team);
    });
  }

  // Return in alphabetical group order.
  return Object.fromEntries(
    Object.entries(result).sort(([a], [b]) => a.localeCompare(b)),
  );
}

// ── Position badge ─────────────────────────────────────────────────────────────
// Top 2 advance for sure, 3rd place might advance (8 best 3rd-place teams).

function rowHighlight(pos: number): string {
  if (pos <= 2) return "border-l-2 border-l-emerald-400 bg-emerald-500/[0.06]";
  if (pos === 3) return "border-l-2 border-l-amber-400 bg-amber-500/[0.04]";
  return "opacity-50";
}

function posBadge(pos: number) {
  if (pos <= 2)
    return (
      <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-emerald-500/20 text-[10px] font-bold text-emerald-300">
        {pos}
      </span>
    );
  if (pos === 3)
    return (
      <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-amber-500/20 text-[10px] font-bold text-amber-300">
        {pos}
      </span>
    );
  return (
    <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-white/8 text-[10px] font-medium text-text-muted">
      {pos}
    </span>
  );
}

// ── Group table ───────────────────────────────────────────────────────────────

function GroupTable({
  group,
  rows,
  t,
}: {
  group: string;
  rows: TeamRow[];
  t: (k: string) => string;
}) {
  return (
    <div className="card overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-2 border-b border-white/8 bg-white/[0.02] px-4 py-2.5">
        <span className="flex h-6 w-6 items-center justify-center rounded bg-gold-500/20 text-xs font-bold text-gold-300">
          {group}
        </span>
        <span className="text-xs font-semibold uppercase tracking-widest text-text-muted">
          {t("standings.group")} {group}
        </span>
      </div>

      {/* Col headers */}
      <div className="grid grid-cols-[auto_1fr_repeat(7,_auto)] items-center gap-x-3 border-b border-white/5 px-4 py-1.5 text-[10px] uppercase tracking-wide text-text-muted">
        <span className="w-5" />
        <span>{t("standings.team")}</span>
        <span className="w-5 text-center">{t("standings.played")}</span>
        <span className="w-5 text-center">{t("standings.won")}</span>
        <span className="w-5 text-center">{t("standings.drawn")}</span>
        <span className="w-5 text-center">{t("standings.lost")}</span>
        <span className="w-6 text-center">{t("standings.gf")}</span>
        <span className="w-6 text-center">{t("standings.ga")}</span>
        <span className="w-7 text-center font-semibold text-white/40">
          {t("standings.pts")}
        </span>
      </div>

      {/* Rows */}
      {rows.map((row, idx) => {
        const pos = idx + 1;
        return (
          <div
            key={row.team}
            className={`grid grid-cols-[auto_1fr_repeat(7,_auto)] items-center gap-x-3 border-b border-white/[0.04] px-4 py-2 last:border-0 transition-colors ${rowHighlight(pos)}`}
          >
            {posBadge(pos)}

            <div className="flex min-w-0 items-center gap-2">
              <span className="text-base leading-none" aria-hidden>
                {flag(row.team)}
              </span>
              <span className="truncate text-sm font-medium text-white">
                {row.team}
              </span>
            </div>

            <span className="w-5 text-center tabular-nums text-xs text-text-muted">
              {row.played}
            </span>
            <span className="w-5 text-center tabular-nums text-xs text-text-secondary">
              {row.won}
            </span>
            <span className="w-5 text-center tabular-nums text-xs text-text-secondary">
              {row.drawn}
            </span>
            <span className="w-5 text-center tabular-nums text-xs text-text-secondary">
              {row.lost}
            </span>
            <span className="w-6 text-center tabular-nums text-xs text-text-secondary">
              {row.gf}
            </span>
            <span className="w-6 text-center tabular-nums text-xs text-text-secondary">
              {row.ga}
            </span>
            <span className="w-7 text-center font-score tabular-nums text-sm font-bold text-white">
              {row.pts}
            </span>
          </div>
        );
      })}
    </div>
  );
}

// ── Legend ────────────────────────────────────────────────────────────────────

function StandingsLegend({ t }: { t: (k: string) => string }) {
  return (
    <div className="flex flex-wrap items-center gap-4 text-xs text-text-muted">
      <span className="flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-sm bg-emerald-500/40 ring-1 ring-emerald-500/50" />
        {t("standings.legendAdvanced")}
      </span>
      <span className="flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-sm bg-amber-500/40 ring-1 ring-amber-500/50" />
        {t("standings.legendMaybe")}
      </span>
      <span className="flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-sm bg-white/10 ring-1 ring-white/15" />
        {t("standings.legendEliminated")}
      </span>
    </div>
  );
}

// ── Section ───────────────────────────────────────────────────────────────────

export function GroupStandingsSection() {
  const { t } = useI18n();

  const { data, isLoading, isError } = useQuery<{ matches: PublicMatch[] }>({
    queryKey: ["public-standings"],
    queryFn: () => fetch("/api/public/standings").then((r) => r.json()),
    staleTime: 60_000,
    refetchInterval: 120_000,
  });

  const groups = data?.matches
    ? buildGroupStandings(data.matches)
    : null;

  const groupKeys = groups ? Object.keys(groups) : [];

  return (
    <section className="px-4 py-16 bg-white/[0.012]">
      <div className="mx-auto max-w-7xl space-y-8">
        {/* Title */}
        <div className="text-center">
          <p className="text-xs uppercase tracking-widest text-gold-300">
            {t("standings.eyebrow")}
          </p>
          <h2 className="mt-2 font-display text-4xl text-white sm:text-5xl">
            {t("standings.title")}
          </h2>
          <p className="mt-3 text-sm text-text-muted">
            {t("standings.subtitle")}
          </p>
        </div>

        {isLoading && (
          <div className="flex justify-center py-12">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-gold-400 border-t-transparent" />
          </div>
        )}

        {isError && (
          <p className="text-center text-sm text-text-muted py-8">
            {t("standings.error")}
          </p>
        )}

        {groups && groupKeys.length === 0 && !isLoading && (
          <p className="text-center text-sm text-text-muted py-8">
            {t("standings.empty")}
          </p>
        )}

        {groups && groupKeys.length > 0 && (
          <>
            <StandingsLegend t={t} />
            <HorizontalCarousel
              itemWidth="w-96"
              gap="gap-4"
              scrollAmount={400}
              ariaLabelLeft={t("common.scrollLeft")}
              ariaLabelRight={t("common.scrollRight")}
            >
              {groupKeys.map((g) => (
                <GroupTable key={g} group={g} rows={groups[g]} t={t} />
              ))}
            </HorizontalCarousel>
          </>
        )}
      </div>
    </section>
  );
}
