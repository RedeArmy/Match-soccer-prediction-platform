// Knockout phase visibility flags.
// Set a phase to true once its fixtures are confirmed and predictions should open.
// group_stage is always visible and is not listed here.
const KNOCKOUT_PHASE_FLAGS: Record<string, boolean> = {
  round_of_32: false,
  round_of_16: false,
  quarter_final: false,
  semi_final: false,
  third_place: false,
  final: false,
};

export function isPhaseVisible(phase: string | null | undefined): boolean {
  if (!phase || phase === "group_stage") return true;
  return KNOCKOUT_PHASE_FLAGS[phase] ?? false;
}
