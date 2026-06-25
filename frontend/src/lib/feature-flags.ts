// KNOCKOUT_PHASE_FLAGS gates which phases are visible in the UI.
// All set to true: visibility is driven by whether matches exist in the fetched
// data. Set a phase to false here to force-hide it even when matches exist
// (e.g. during a data-integrity incident).
const KNOCKOUT_PHASE_FLAGS: Record<string, boolean> = {
  round_of_32: true,
  round_of_16: true,
  quarter_final: true,
  semi_final: true,
  third_place: true,
  final: true,
};

export function isPhaseVisible(phase: string | null | undefined): boolean {
  if (!phase || phase === "group_stage") return true;
  return KNOCKOUT_PHASE_FLAGS[phase] ?? false;
}

// Tabs are ordered Final-first so that newly confirmed phases appear to the
// left of earlier rounds as the bracket fills in.
const KNOCKOUT_TAB_ORDER = [
  "final",
  "semi_final",
  "third_place",
  "quarter_final",
  "round_of_16",
  "round_of_32",
] as const;

// Returns which knockout phases should appear as tabs.
// A phase is visible only when at least one match in that phase has both
// home_team and away_team confirmed (non-empty strings).
export function visibleKnockoutPhases(
  matches: { phase: string | null; home_team?: string; away_team?: string }[],
): string[] {
  const confirmedPhases = new Set(
    matches
      .filter(
        (m) =>
          m.phase &&
          m.phase !== "group_stage" &&
          isPhaseVisible(m.phase) &&
          m.home_team &&
          m.away_team,
      )
      .map((m) => m.phase as string),
  );
  return KNOCKOUT_TAB_ORDER.filter((p) => confirmedPhases.has(p));
}
