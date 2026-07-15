import type { ExtraFirstScorerAnswer, ExtraTeamScoresAnswer, ExtraType } from "@/lib/api-types";

// Mirrors the backend cross-validation in
// internal/domain/validators.go (ValidateExtraAnswerAgainstPrediction) and
// internal/service/extra_scoring_service.go (teamScoresAnswer) — the set of
// extra-prediction answers that are even logically possible given the
// user's own scoreline prediction (home, away) for the match. Kept in sync
// with the backend so the UI never offers a choice the API would reject.

// validFirstScorerAnswers: a team that is predicted to score 0 goals cannot
// have been the first scorer; if neither team scores, "none" is the only
// valid answer; if both score, "none" is impossible (someone scored first).
export function validFirstScorerAnswers(
  home: number,
  away: number,
): ExtraFirstScorerAnswer[] {
  if (home === 0 && away === 0) return ["none"];
  if (home === 0) return ["away"];
  if (away === 0) return ["home"];
  return ["home", "away"];
}

// validTeamScoresAnswers: the number of goals a team is predicted to score
// bounds which periods are even possible for that team's "scores in" guess.
export function validTeamScoresAnswers(
  predictedGoals: number,
): ExtraTeamScoresAnswer[] {
  if (predictedGoals === 0) return ["none"];
  if (predictedGoals === 1) return ["first_half", "second_half"];
  return ["first_half", "second_half", "both_halves"];
}

// teamScoresValidValues combines two independent forcing sources into the
// options a team's "scores in" select should currently offer: the main
// scoreline prediction alone (validTeamScoresAnswers) may already force a
// single answer (e.g. 0 goals -> "none"); separately, if the user has
// directly entered an exact half-time split for this team (period !== null
// and not "both_halves"), that pins the answer too. Either source narrowing
// to one option takes priority over showing the full set.
export function teamScoresValidValues(
  fulltimeGoals: number,
  period: ExtraTeamScoresAnswer | null,
): ExtraTeamScoresAnswer[] {
  const base = validTeamScoresAnswers(fulltimeGoals);
  if (base.length === 1) return base;
  if (period && period !== "both_halves" && base.includes(period)) {
    return [period];
  }
  return base;
}

// forcedExtraAnswer returns the single valid answer for extraType given the
// draft (home, away) scoreline, or null when more than one answer remains
// possible (the user must still choose). first_scorer and the team-scores
// extras can be forced down to one value directly from the main scoreline;
// halftime_result is only ever fully forced in the 0-0 case (each side's
// half-time score is otherwise a range, not a single value — see
// halftimeInputBounds).
export function forcedExtraAnswer(
  extraType: ExtraType,
  home: number,
  away: number,
): string | null {
  switch (extraType) {
    case "first_scorer": {
      const options = validFirstScorerAnswers(home, away);
      return options.length === 1 ? options[0] : null;
    }
    case "home_team_scores":
      return home === 0 ? "none" : null;
    case "away_team_scores":
      return away === 0 ? "none" : null;
    case "halftime_result":
      return home === 0 && away === 0 ? "0-0" : null;
    default:
      return null;
  }
}

// halftimeInputBounds returns the [min, max] range a team's half-time goal
// count can legally take given its own predicted final score — it can never
// be negative or exceed the final score.
export function halftimeInputBounds(
  predictedGoals: number,
): { min: number; max: number } {
  return { min: 0, max: Math.max(0, predictedGoals) };
}

// teamScoresAnswerFromHalftime derives which period (if any) a team scored
// in from its half-time and full-time goal counts — always a single
// deterministic value given both numbers. Mirrors teamScoresAnswer in
// internal/service/extra_scoring_service.go exactly (same branches, same
// nil/inconsistent-data guard).
export function teamScoresAnswerFromHalftime(
  halftimeGoals: number | null | undefined,
  fulltimeGoals: number | null | undefined,
): ExtraTeamScoresAnswer | null {
  if (
    halftimeGoals == null ||
    fulltimeGoals == null ||
    fulltimeGoals < halftimeGoals
  )
    return null;
  const scoredFirstHalf = halftimeGoals > 0;
  const scoredSecondHalf = fulltimeGoals > halftimeGoals;
  if (scoredFirstHalf && scoredSecondHalf) return "both_halves";
  if (scoredFirstHalf) return "first_half";
  if (scoredSecondHalf) return "second_half";
  return "none";
}

// halftimeGoalsFromPeriod is the inverse derivation used when the user picks
// a period directly: "first_half" and "second_half" (and "none") each pin
// the team's half-time goal count to one exact value. "both_halves" only
// narrows it to a range (at least 1, strictly less than the final score), so
// it returns null — the caller keeps whatever value is already set, clamped
// into that range.
export function halftimeGoalsFromPeriod(
  period: ExtraTeamScoresAnswer,
  fulltimeGoals: number,
): number | null {
  switch (period) {
    case "none":
      return 0;
    case "first_half":
      return fulltimeGoals;
    case "second_half":
      return 0;
    case "both_halves":
      return null;
  }
}

// halftimeSideBounds computes one team's half-time input range and whether
// it should be disabled, given that team's full-time goal prediction and its
// currently chosen/derived "scores in" period. A period of "first_half",
// "second_half", or "none" pins the half-time count to one exact value (the
// input is disabled, showing that value); "both_halves" only narrows the
// range (needs at least 1 goal in each half); no period yet chosen leaves
// the full [0, fulltimeGoals] range open.
export function halftimeSideBounds(
  fulltimeGoals: number,
  period: ExtraTeamScoresAnswer | null,
): { min: number; max: number; disabled: boolean } {
  if (period && period !== "both_halves") {
    const pinned = halftimeGoalsFromPeriod(period, fulltimeGoals) ?? 0;
    return { min: pinned, max: pinned, disabled: true };
  }
  if (period === "both_halves") {
    return {
      min: Math.min(1, fulltimeGoals),
      max: Math.max(1, fulltimeGoals - 1),
      disabled: fulltimeGoals < 2,
    };
  }
  return { min: 0, max: fulltimeGoals, disabled: fulltimeGoals === 0 };
}

// isAnswerStillValid checks whether a previously-submitted extra answer is
// still consistent with the current draft scoreline — used to stop
// pre-filling/trusting a now-stale answer after the user edits the main
// score (e.g. changing 1-0 to 1-1 invalidates a saved away_team_scores of
// "none"). There's no delete-extra endpoint, so "clearing" a stale answer
// means treating it as unset for display purposes; the stale database row
// is simply overwritten the next time the user picks (or re-picks) a value.
export function isAnswerStillValid(
  extraType: ExtraType,
  answer: string,
  home: number,
  away: number,
): boolean {
  switch (extraType) {
    case "first_scorer":
      return (validFirstScorerAnswers(home, away) as string[]).includes(
        answer,
      );
    case "home_team_scores":
      return (validTeamScoresAnswers(home) as string[]).includes(answer);
    case "away_team_scores":
      return (validTeamScoresAnswers(away) as string[]).includes(answer);
    case "halftime_result": {
      const m = /^(\d{1,2})-(\d{1,2})$/.exec(answer);
      if (!m) return false;
      const h = Number(m[1]);
      const a = Number(m[2]);
      return h <= home && a <= away;
    }
    default:
      return true;
  }
}
