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

// Returns which knockout phases have at least one match in the data.
// Used to decide which phase tabs to show in PredictionPanel.
export function visibleKnockoutPhases(matches: { phase: string | null }[]): string[] {
  const KNOCKOUT_ORDER = [
    "round_of_32",
    "round_of_16",
    "quarter_final",
    "semi_final",
    "third_place",
    "final",
  ] as const;
  const presentPhases = new Set(
    matches
      .filter((m) => m.phase && m.phase !== "group_stage" && isPhaseVisible(m.phase))
      .map((m) => m.phase as string),
  );
  return KNOCKOUT_ORDER.filter((p) => presentPhases.has(p));
}
