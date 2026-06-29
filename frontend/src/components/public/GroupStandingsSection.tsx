"use client";

import { Fragment } from "react";
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

// ── Spanish team name translations ────────────────────────────────────────────

const TEAM_NAMES_ES: Record<string, string> = {
  Algeria: "Argelia",
  Argentina: "Argentina",
  Australia: "Australia",
  Austria: "Austria",
  Belgium: "Bélgica",
  "Bosnia and Herzegovina": "Bosnia y Herzegovina",
  Brazil: "Brasil",
  Canada: "Canadá",
  "Cape Verde": "Cabo Verde",
  Colombia: "Colombia",
  Croatia: "Croacia",
  Curaçao: "Curazao",
  Czechia: "Chequia",
  "DR Congo": "RD Congo",
  Ecuador: "Ecuador",
  Egypt: "Egipto",
  England: "Inglaterra",
  France: "Francia",
  Germany: "Alemania",
  Ghana: "Ghana",
  Haiti: "Haití",
  Iran: "Irán",
  Iraq: "Irak",
  "Ivory Coast": "Costa de Marfil",
  Japan: "Japón",
  Jordan: "Jordania",
  Mexico: "México",
  Morocco: "Marruecos",
  Netherlands: "Países Bajos",
  "New Zealand": "Nueva Zelanda",
  Nigeria: "Nigeria",
  Norway: "Noruega",
  Panama: "Panamá",
  Paraguay: "Paraguay",
  Portugal: "Portugal",
  Qatar: "Catar",
  "Saudi Arabia": "Arabia Saudita",
  Scotland: "Escocia",
  Senegal: "Senegal",
  "South Africa": "Sudáfrica",
  "South Korea": "Corea del Sur",
  Spain: "España",
  Sweden: "Suecia",
  Switzerland: "Suiza",
  Tunisia: "Túnez",
  Türkiye: "Turquía",
  "United States": "Estados Unidos",
  Uruguay: "Uruguay",
  Uzbekistan: "Uzbekistán",
};

export function teamDisplayName(name: string, locale: string): string {
  if (locale === "es") return TEAM_NAMES_ES[name] ?? name;
  return name;
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

// ── Best-third helpers ────────────────────────────────────────────────────────

// getBestEightThirds returns the set of team names that are among the top-8
// third-placed teams when ranked by FIFA criteria (pts → GD → GF → name).
export function getBestEightThirds(
  groups: Record<string, TeamRow[]>,
): Set<string> {
  const thirds: TeamRow[] = [];
  for (const rows of Object.values(groups)) {
    if (rows.length >= 3) thirds.push(rows[2]);
  }
  thirds.sort((a, b) => {
    if (b.pts !== a.pts) return b.pts - a.pts;
    if (b.gd !== a.gd) return b.gd - a.gd;
    if (b.gf !== a.gf) return b.gf - a.gf;
    return a.team.localeCompare(b.team);
  });
  return new Set(thirds.slice(0, 8).map((t) => t.team));
}

// isGroupStageComplete returns true when all 12 groups are present and every
// team in every group has played 3 matches (= all group-stage matches finished).
export function isGroupStageComplete(
  groups: Record<string, TeamRow[]>,
): boolean {
  const keys = Object.keys(groups);
  if (keys.length < 12) return false;
  return keys.every((g) => groups[g].every((t) => t.played >= 3));
}

// ── Position badge ─────────────────────────────────────────────────────────────
// Top 2 advance for sure. 3rd place: if group stage is complete, only the
// top-8 thirds advance (amber); the rest are treated as eliminated (dim).

// Per-cell helpers — the flat grid has no row wrapper to style, so each helper
// is applied to every cell in the row individually.
function rowBg(
  pos: number,
  team: string,
  bestThirds: Set<string>,
  complete: boolean,
): string {
  if (pos <= 2) return "bg-emerald-500/[0.06]";
  if (pos === 3 && (!complete || bestThirds.has(team)))
    return "bg-amber-500/[0.04]";
  return "";
}
function rowAccent(
  pos: number,
  team: string,
  bestThirds: Set<string>,
  complete: boolean,
): string {
  if (pos <= 2) return "border-l-2 border-l-emerald-400";
  if (pos === 3 && (!complete || bestThirds.has(team)))
    return "border-l-2 border-l-amber-400";
  return "";
}
function rowDim(
  pos: number,
  team: string,
  bestThirds: Set<string>,
  complete: boolean,
): string {
  if (pos > 3) return "opacity-50";
  if (pos === 3 && complete && !bestThirds.has(team)) return "opacity-50";
  return "";
}

function posBadge(
  pos: number,
  team: string,
  bestThirds: Set<string>,
  complete: boolean,
) {
  if (pos <= 2)
    return (
      <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-emerald-500/20 text-[10px] font-bold text-emerald-300">
        {pos}
      </span>
    );
  if (pos === 3 && (!complete || bestThirds.has(team)))
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
// Single flat grid: header cells and all row cells are direct children of the
// same grid container so CSS shares one column-sizing pass across every row,
// guaranteeing perfect alignment without per-row grids diverging.
//
// Columns: pos-badge | team-name (1fr) | J | G | E | P | GF | GA | PTS
// The 1fr team-name column is identical in every row because the outer card
// has a fixed pixel width, so 1fr resolves to the same value for all rows.
// "Bosnia and Herzegovina" (the longest name) may be truncated by `truncate`;
// all shorter names — including "Costa de Marfil" / "Estados Unidos" — display
// in full.

function GroupTable({
  group,
  rows,
  t,
  locale,
  bestThirds,
  groupStageComplete,
}: Readonly<{
  group: string;
  rows: TeamRow[];
  t: (k: string) => string;
  locale: string;
  bestThirds: Set<string>;
  groupStageComplete: boolean;
}>) {
  const COL = "grid-cols-[20px_1fr_28px_28px_28px_28px_28px_28px_32px]";
  const HDR =
    "border-b border-white/5 py-1.5 text-[10px] uppercase tracking-wide text-text-muted";
  const NUM = "flex items-center justify-center tabular-nums text-xs";

  return (
    <div className="card overflow-hidden">
      {/* Group label */}
      <div className="flex items-center gap-2 border-b border-white/8 bg-white/[0.02] px-4 py-2.5">
        <span className="flex h-6 w-6 items-center justify-center rounded bg-gold-500/20 text-xs font-bold text-gold-300">
          {group}
        </span>
        <span className="text-xs font-semibold uppercase tracking-widest text-text-muted">
          {t("standings.group")} {group}
        </span>
      </div>

      {/* Single shared grid — header + all data rows */}
      <div className={`grid ${COL} gap-x-2 px-4`}>
        {/* Column headers (9 cells) */}
        <span className={HDR} />
        <span className={HDR}>{t("standings.team")}</span>
        <span className={`${HDR} text-center`}>{t("standings.played")}</span>
        <span className={`${HDR} text-center`}>{t("standings.won")}</span>
        <span className={`${HDR} text-center`}>{t("standings.drawn")}</span>
        <span className={`${HDR} text-center`}>{t("standings.lost")}</span>
        <span className={`${HDR} text-center`}>{t("standings.gf")}</span>
        <span className={`${HDR} text-center`}>{t("standings.ga")}</span>
        <span className={`${HDR} text-center font-semibold text-white/40`}>
          {t("standings.pts")}
        </span>

        {/* Data rows — React.Fragment lets cells be direct grid children */}
        {rows.map((row, idx) => {
          const pos = idx + 1;
          const sep =
            idx < rows.length - 1 ? "border-b border-white/[0.04]" : "";
          const bg = rowBg(pos, row.team, bestThirds, groupStageComplete);
          const dim = rowDim(pos, row.team, bestThirds, groupStageComplete);
          const accent = rowAccent(
            pos,
            row.team,
            bestThirds,
            groupStageComplete,
          );

          return (
            <Fragment key={row.team}>
              <div
                className={`flex items-center py-2 ${sep} ${bg} ${accent} ${dim}`}
              >
                {posBadge(pos, row.team, bestThirds, groupStageComplete)}
              </div>
              <div
                className={`flex min-w-0 items-center gap-2 overflow-hidden py-2 ${sep} ${bg} ${dim}`}
              >
                <span className="shrink-0 text-base leading-none" aria-hidden>
                  {flag(row.team)}
                </span>
                <span className="truncate text-sm font-medium text-white">
                  {teamDisplayName(row.team, locale)}
                </span>
              </div>
              <div
                className={`${NUM} py-2 text-text-muted ${sep} ${bg} ${dim}`}
              >
                {row.played}
              </div>
              <div
                className={`${NUM} py-2 text-text-secondary ${sep} ${bg} ${dim}`}
              >
                {row.won}
              </div>
              <div
                className={`${NUM} py-2 text-text-secondary ${sep} ${bg} ${dim}`}
              >
                {row.drawn}
              </div>
              <div
                className={`${NUM} py-2 text-text-secondary ${sep} ${bg} ${dim}`}
              >
                {row.lost}
              </div>
              <div
                className={`${NUM} py-2 text-text-secondary ${sep} ${bg} ${dim}`}
              >
                {row.gf}
              </div>
              <div
                className={`${NUM} py-2 text-text-secondary ${sep} ${bg} ${dim}`}
              >
                {row.ga}
              </div>
              <div
                className={`flex items-center justify-center py-2 font-score tabular-nums text-sm font-bold text-white ${sep} ${bg} ${dim}`}
              >
                {row.pts}
              </div>
            </Fragment>
          );
        })}
      </div>
    </div>
  );
}

// ── Legend ────────────────────────────────────────────────────────────────────

function StandingsLegend({
  t,
  groupStageComplete,
}: Readonly<{ t: (k: string) => string; groupStageComplete: boolean }>) {
  return (
    <div className="flex flex-wrap items-center gap-4 text-xs text-text-muted">
      <span className="flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-sm bg-emerald-500/40 ring-1 ring-emerald-500/50" />
        {t("standings.legendAdvanced")}
      </span>
      <span className="flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-sm bg-amber-500/40 ring-1 ring-amber-500/50" />
        {groupStageComplete
          ? t("standings.legendBestThird")
          : t("standings.legendMaybe")}
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
  const { t, locale } = useI18n();

  const { data, isLoading, isError } = useQuery<{ matches: PublicMatch[] }>({
    queryKey: ["public-standings"],
    queryFn: () => fetch("/api/public/standings").then((r) => r.json()),
    staleTime: 60_000,
    refetchInterval: 120_000,
  });

  const groups = data?.matches ? buildGroupStandings(data.matches) : null;

  const groupKeys = groups ? Object.keys(groups) : [];
  const groupStageComplete = groups ? isGroupStageComplete(groups) : false;
  const bestThirds = groups ? getBestEightThirds(groups) : new Set<string>();

  return (
    <section className="px-4 py-16 bg-white/[0.012]">
      <div className="mx-auto max-w-7xl space-y-8">
        {/* Title */}
        <div className="text-center">
          <h2 className="font-display text-4xl text-white sm:text-5xl">
            {t("standings.title")}
          </h2>
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
            <StandingsLegend t={t} groupStageComplete={groupStageComplete} />
            <HorizontalCarousel
              itemWidth="w-[520px]"
              gap="gap-4"
              scrollAmount={400}
              ariaLabelLeft={t("common.scrollLeft")}
              ariaLabelRight={t("common.scrollRight")}
            >
              {groupKeys.map((g) => (
                <GroupTable
                  key={g}
                  group={g}
                  rows={groups[g]}
                  t={t}
                  locale={locale}
                  bestThirds={bestThirds}
                  groupStageComplete={groupStageComplete}
                />
              ))}
            </HorizontalCarousel>
          </>
        )}
      </div>
    </section>
  );
}
