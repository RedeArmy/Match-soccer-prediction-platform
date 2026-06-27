"use client";

import { useState } from "react";
import { ChevronDown } from "lucide-react";
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
}

function SlotRow({ slot }: SlotRowProps) {
  const { slotDesc, teamName } = useI18n();
  const hasTeam = !!slot?.team;
  return (
    <div className="flex items-center gap-2 px-3 py-2">
      <span
        className={cn(
          "flex-1 truncate text-sm",
          hasTeam ? "font-semibold text-white" : "italic text-text-muted",
        )}
      >
        {hasTeam ? teamName(slot!.team) : slotDesc(slot?.description)}
      </span>
    </div>
  );
}

interface MatchPairProps {
  readonly home: TournamentSlotResponse | undefined;
  readonly away: TournamentSlotResponse | undefined;
}

function MatchPair({ home, away }: MatchPairProps) {
  const kickoff = formatKickoff(
    home?.match_kickoff_at ?? away?.match_kickoff_at,
  );
  return (
    <div className="overflow-hidden rounded-lg border border-white/10 bg-white/[0.03]">
      {kickoff && (
        <div className="border-b border-white/5 px-3 pt-1.5 pb-0.5">
          <span className="text-[10px] font-medium uppercase tracking-wide text-text-muted/60">
            {kickoff}
          </span>
        </div>
      )}
      <SlotRow slot={home} />
      <div className="h-px bg-white/10" />
      <SlotRow slot={away} />
    </div>
  );
}

interface PhaseAccordionProps {
  readonly labelKey: string;
  readonly slots: TournamentSlotResponse[];
  readonly t: (key: string) => string;
  readonly defaultOpen: boolean;
}

function PhaseAccordion({
  labelKey,
  slots,
  t,
  defaultOpen,
}: PhaseAccordionProps) {
  const [open, setOpen] = useState(defaultOpen);

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
    <div className="overflow-hidden rounded-xl border border-white/10">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition-colors hover:bg-white/[0.03]"
      >
        <div className="flex items-center gap-2">
          <span className="font-semibold text-white">{t(labelKey)}</span>
          <span className="rounded-full bg-white/10 px-2 py-0.5 text-[10px] tabular-nums text-text-muted">
            {confirmedCount}/{slots.length}
          </span>
        </div>
        <ChevronDown
          className={cn(
            "h-4 w-4 shrink-0 text-text-muted transition-transform duration-200",
            open && "rotate-180",
          )}
        />
      </button>

      {open && (
        <div className="grid grid-cols-1 gap-3 border-t border-white/10 p-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {pairs.map(({ num, home, away }) => (
            <MatchPair key={num} home={home} away={away} />
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
              t={t}
              defaultOpen={idx === 0}
            />
          ))}
        </div>
      </div>
    </section>
  );
}
