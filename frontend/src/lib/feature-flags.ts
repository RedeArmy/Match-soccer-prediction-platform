// KNOCKOUT_PHASE_FLAGS gates which phases are visible in the UI.
// All set to true: visibility is driven by whether matches exist in the fetched
// data. Set a phase to false here to force-hide it even when matches exist
// (e.g. during a data-integrity incident).
const KNOCKOUT_PHASE_FLAGS: Record<string, boolean> = {
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

// Matches bracket placeholder codes seeded for knockout fixtures:
//   Group position  →  1A  2B  3ABCDF   (digits + group letters A–L)
//   Match result    →  W73  W89  L101   (W/L prefix + match number)
// These are replaced by actual team names when an admin confirms each slot.
const KNOCKOUT_PLACEHOLDER_RE = /^(?:\d+[A-L]+|[WL]\d+)$/i;

export function isKnockoutPlaceholder(
  name: string | null | undefined,
): boolean {
  if (!name) return true;
  return KNOCKOUT_PLACEHOLDER_RE.test(name.trim());
}

// Tabs are ordered Final-first so that newly confirmed phases appear to the
// left of earlier rounds as the bracket fills in.
const KNOCKOUT_TAB_ORDER = [
  "final",
  "semi_final",
  "third_place",
  "quarter_final",
  "round_of_16",
] as const;

// Returns which knockout phases should appear as tabs.
// A phase is visible only when at least one match in that phase has both
// home_team and away_team set to confirmed team names (not bracket placeholders).
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
          !isKnockoutPlaceholder(m.home_team) &&
          !isKnockoutPlaceholder(m.away_team),
      )
      .map((m) => m.phase as string),
  );
  return KNOCKOUT_TAB_ORDER.filter((p) => confirmedPhases.has(p));
}
