"use client";

import { useState } from "react";
import { ChevronDown, Trophy } from "lucide-react";
import { useSlots } from "@/hooks/useSlots";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { TournamentSlotResponse } from "@/lib/api-types";

// Bracket label convention: {prefix}_{match:02d}_{side}
// e.g. r32_01_a, r16_03_b, qf_02_a, fin_01_b
const PHASE_ORDER = [
  { prefix: "r32", labelKey: "phaseRoundOf32" },
  { prefix: "r16", labelKey: "phaseRoundOf16" },
  { prefix: "qf", labelKey: "phaseQuarterFinal" },
  { prefix: "sf", labelKey: "phaseSemiF" },
  { prefix: "tp", labelKey: "phaseThirdPlace" },
  { prefix: "fin", labelKey: "phaseFinal" },
] as const;

function phasePrefix(label: string): string {
  return label.split("_")[0] ?? "";
}

function matchNum(label: string): number {
  const parts = label.split("_");
  return Number(parts.at(-2));
}

function side(label: string): string {
  const parts = label.split("_");
  return parts.at(-1) ?? "a";
}

function groupByPhase(
  slots: TournamentSlotResponse[],
): Map<string, TournamentSlotResponse[]> {
  const map = new Map<string, TournamentSlotResponse[]>();
  for (const slot of slots) {
    const prefix = phasePrefix(slot.label);
    if (!map.has(prefix)) map.set(prefix, []);
    map.get(prefix)!.push(slot);
  }
  return map;
}

function hasAnyConfirmedSlot(slots: TournamentSlotResponse[]): boolean {
  return slots.some((s) => s.team != null);
}

// Format a UTC ISO string to the viewer's local date/time.
function formatKickoff(iso: string | null | undefined): string | null {
  if (!iso) return null;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(iso));
}

interface SlotRowProps {
  readonly slot: TournamentSlotResponse | undefined;
  readonly score: number | null;
  readonly penaltyScore: number | null;
  readonly isWinner: boolean;
  readonly showScore: boolean;
}

function SlotRow({ slot, score, penaltyScore, isWinner, showScore }: SlotRowProps) {
  const { slotDesc, teamName } = useI18n();
  const hasTeam = !!slot?.team;
  const isLoser = showScore && hasTeam && !isWinner;
  let teamClass = "italic text-text-muted";
  if (hasTeam) teamClass = isWinner ? "font-semibold text-green-400" : "font-semibold text-white";
  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 py-2 transition-colors",
        isWinner && "bg-green-500/10",
        isLoser && "opacity-40",
      )}
    >
      <span
        className={cn("flex-1 truncate text-sm", teamClass)}
      >
        {hasTeam ? teamName(slot!.team) : slotDesc(slot?.description)}
      </span>
      {showScore && score !== null && (
        <span
          className={cn(
            "shrink-0 tabular-nums text-sm font-bold",
            isWinner ? "text-green-400" : "text-white/70",
          )}
        >
          {score}
          {penaltyScore !== null && (
            <span className="ml-0.5 text-[10px] font-normal text-white/40">
              ({penaltyScore})
            </span>
          )}
        </span>
      )}
    </div>
  );
}

interface MatchPairProps {
  readonly home: TournamentSlotResponse | undefined;
  readonly away: TournamentSlotResponse | undefined;
  readonly index: number;
  readonly isFinal?: boolean;
}

function MatchPair({ home, away, index, isFinal }: MatchPairProps) {
  const ref = home ?? away;
  const isFinished = ref?.match_status === "finished";
  const byPenalties = ref?.match_win_method === "penalties";

  const homeScore = ref?.match_home_score ?? null;
  const awayScore = ref?.match_away_score ?? null;
  const penHome = byPenalties ? (ref?.penalty_home_score ?? null) : null;
  const penAway = byPenalties ? (ref?.penalty_away_score ?? null) : null;

  let homeWins = false;
  let awayWins = false;
  if (isFinished && homeScore !== null && awayScore !== null) {
    if (byPenalties) {
      homeWins = (penHome ?? 0) > (penAway ?? 0);
      awayWins = (penAway ?? 0) > (penHome ?? 0);
    } else {
      homeWins = homeScore > awayScore;
      awayWins = awayScore > homeScore;
    }
  }

  const kickoff = formatKickoff(
    home?.match_kickoff_at ?? away?.match_kickoff_at,
  );
  return (
    <div
      className={cn(
        "overflow-hidden rounded-lg border",
        isFinal
          ? "border-gold-600/30 bg-gold-600/[0.05]"
          : "border-white/10 bg-white/[0.03]",
      )}
    >
      <div
        className={cn(
          "flex items-center justify-between border-b px-3 pt-1.5 pb-0.5",
          isFinal ? "border-gold-600/20" : "border-white/5",
        )}
      >
        <span
          className={cn(
            "text-[10px] font-medium uppercase tracking-wide",
            isFinal ? "text-gold-300/60" : "text-text-muted/60",
          )}
        >
          {kickoff && !isFinished ? kickoff : ""}
        </span>
        <span
          className={cn(
            "text-[10px] font-medium tabular-nums",
            isFinal ? "text-gold-300/50" : "text-text-muted/50",
          )}
        >
          M{String(index).padStart(2, "0")}
        </span>
      </div>
      <SlotRow
        slot={home}
        score={homeScore}
        penaltyScore={penHome}
        isWinner={homeWins}
        showScore={isFinished}
      />
      <div className={cn("h-px", isFinal ? "bg-gold-600/20" : "bg-white/10")} />
      <SlotRow
        slot={away}
        score={awayScore}
        penaltyScore={penAway}
        isWinner={awayWins}
        showScore={isFinished}
      />
    </div>
  );
}

interface PhaseAccordionProps {
  readonly labelKey: string;
  readonly slots: TournamentSlotResponse[];
  readonly prefix: string;
  readonly t: (key: string) => string;
  readonly defaultOpen: boolean;
}

function PhaseAccordion({
  labelKey,
  slots,
  prefix,
  t,
  defaultOpen,
}: PhaseAccordionProps) {
  const [open, setOpen] = useState(defaultOpen);

  const isFinal = prefix === "fin";
  const confirmedCount = slots.filter((s) => s.team !== null).length;

  const byMatch = new Map<
    number,
    { a?: TournamentSlotResponse; b?: TournamentSlotResponse }
  >();
  for (const slot of slots) {
    const num = matchNum(slot.label);
    const s = side(slot.label);
    if (!byMatch.has(num)) byMatch.set(num, {});
    const entry = byMatch.get(num)!;
    if (s === "a") entry.a = slot;
    else entry.b = slot;
  }
  const pairs = [...byMatch.entries()]
    .sort(([a], [b]) => a - b)
    .map(([num, { a, b }]) => ({ num, home: a, away: b }));

  return (
    <div
      className={cn(
        "overflow-hidden rounded-xl border",
        isFinal ? "border-gold-600/40 bg-gold-600/[0.02]" : "border-white/10",
      )}
    >
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className={cn(
          "flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition-colors",
          isFinal
            ? "hover:bg-gold-600/[0.05]"
            : "hover:bg-white/[0.03]",
        )}
      >
        <div className="flex items-center gap-2">
          {isFinal && (
            <Trophy className="h-4 w-4 shrink-0 text-gold-300" />
          )}
          <span
            className={cn(
              "font-semibold",
              isFinal ? "text-gold-200" : "text-white",
            )}
          >
            {t(labelKey)}
          </span>
          <span
            className={cn(
              "rounded-full px-2 py-0.5 text-[10px] tabular-nums",
              isFinal
                ? "bg-gold-600/20 text-gold-300"
                : "bg-white/10 text-text-muted",
            )}
          >
            {confirmedCount}/{slots.length}
          </span>
        </div>
        <ChevronDown
          className={cn(
            "h-4 w-4 shrink-0 transition-transform duration-200",
            isFinal ? "text-gold-300/70" : "text-text-muted",
            open && "rotate-180",
          )}
        />
      </button>

      {open && (
        <div
          className={cn(
            "grid grid-cols-1 gap-3 border-t p-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4",
            isFinal ? "border-gold-600/25" : "border-white/10",
          )}
        >
          {pairs.map(({ num, home, away }) => (
            <MatchPair key={num} index={num} home={home} away={away} isFinal={isFinal} />
          ))}
        </div>
      )}
    </div>
  );
}

export function KnockoutBracket() {
  const { t } = useI18n();
  const { slots, isLoading } = useSlots();
  const byPhase = groupByPhase(slots);

  const visiblePhases = PHASE_ORDER.filter((p) =>
    hasAnyConfirmedSlot(byPhase.get(p.prefix) ?? []),
  );

  if (isLoading) {
    return (
      <section className="px-4 py-16 bg-white/[0.012]">
        <div className="mx-auto max-w-7xl space-y-8">
          <div className="text-center">
            <div className="mx-auto h-12 w-56 animate-pulse rounded bg-white/10" />
          </div>
          {[1, 2].map((i) => (
            <div key={i} className="h-12 animate-pulse rounded-xl bg-white/5" />
          ))}
        </div>
      </section>
    );
  }

  if (visiblePhases.length === 0) return null;

  return (
    <section className="px-4 py-16 bg-white/[0.012]">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="text-center">
          <h2 className="font-display text-4xl text-white sm:text-5xl">
            {t("bracketTitle")}
          </h2>
        </div>
        <div className="space-y-3">
          {visiblePhases.map((phase, idx) => (
            <PhaseAccordion
              key={phase.prefix}
              labelKey={phase.labelKey}
              slots={byPhase.get(phase.prefix) ?? []}
              prefix={phase.prefix}
              t={t}
              defaultOpen={idx === 0}
            />
          ))}
        </div>
      </div>
    </section>
  );
}
