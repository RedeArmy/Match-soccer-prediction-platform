import { describe, expect, it } from "vitest";
import {
  forcedExtraAnswer,
  halftimeGoalsFromPeriod,
  halftimeInputBounds,
  halftimeSideBounds,
  isAnswerStillValid,
  teamScoresAnswerFromHalftime,
  teamScoresValidValues,
  validFirstScorerAnswers,
  validTeamScoresAnswers,
} from "@/lib/extras";

describe("validFirstScorerAnswers", () => {
  it("forces none for a 0-0 scoreline", () => {
    expect(validFirstScorerAnswers(0, 0)).toEqual(["none"]);
  });

  it("forces the scoring team when only one side scores", () => {
    expect(validFirstScorerAnswers(1, 0)).toEqual(["home"]);
    expect(validFirstScorerAnswers(0, 2)).toEqual(["away"]);
  });

  it("excludes none but allows both teams when both score", () => {
    expect(validFirstScorerAnswers(1, 1)).toEqual(["home", "away"]);
  });
});

describe("validTeamScoresAnswers", () => {
  it("forces none for 0 predicted goals", () => {
    expect(validTeamScoresAnswers(0)).toEqual(["none"]);
  });

  it("excludes none and both_halves for exactly 1 goal", () => {
    expect(validTeamScoresAnswers(1)).toEqual(["first_half", "second_half"]);
  });

  it("excludes only none for 2 or more goals", () => {
    expect(validTeamScoresAnswers(2)).toEqual([
      "first_half",
      "second_half",
      "both_halves",
    ]);
    expect(validTeamScoresAnswers(5)).toEqual([
      "first_half",
      "second_half",
      "both_halves",
    ]);
  });
});

describe("forcedExtraAnswer", () => {
  it("forces first_scorer for 0-0 and one-sided scorelines", () => {
    expect(forcedExtraAnswer("first_scorer", 0, 0)).toBe("none");
    expect(forcedExtraAnswer("first_scorer", 1, 0)).toBe("home");
    expect(forcedExtraAnswer("first_scorer", 0, 3)).toBe("away");
  });

  it("does not force first_scorer when both teams score", () => {
    expect(forcedExtraAnswer("first_scorer", 1, 1)).toBeNull();
  });

  it("forces a team's scores-in answer to none only when that side is 0", () => {
    expect(forcedExtraAnswer("home_team_scores", 0, 5)).toBe("none");
    expect(forcedExtraAnswer("home_team_scores", 1, 5)).toBeNull();
    expect(forcedExtraAnswer("away_team_scores", 5, 0)).toBe("none");
    expect(forcedExtraAnswer("away_team_scores", 5, 2)).toBeNull();
  });

  it("forces halftime_result to 0-0 only when the full prediction is 0-0", () => {
    expect(forcedExtraAnswer("halftime_result", 0, 0)).toBe("0-0");
    expect(forcedExtraAnswer("halftime_result", 1, 0)).toBeNull();
    expect(forcedExtraAnswer("halftime_result", 0, 1)).toBeNull();
  });
});

describe("halftimeInputBounds", () => {
  it("caps the max to the predicted goal count", () => {
    expect(halftimeInputBounds(0)).toEqual({ min: 0, max: 0 });
    expect(halftimeInputBounds(3)).toEqual({ min: 0, max: 3 });
  });
});

describe("teamScoresAnswerFromHalftime", () => {
  it("returns none when neither half has goals", () => {
    expect(teamScoresAnswerFromHalftime(0, 0)).toBe("none");
  });

  it("returns first_half when all goals arrived by half-time", () => {
    expect(teamScoresAnswerFromHalftime(2, 2)).toBe("first_half");
  });

  it("returns second_half when goals only arrive after half-time", () => {
    expect(teamScoresAnswerFromHalftime(0, 1)).toBe("second_half");
  });

  it("returns both_halves when goals arrive in each half", () => {
    expect(teamScoresAnswerFromHalftime(1, 2)).toBe("both_halves");
  });

  it("returns null for missing or inconsistent data", () => {
    expect(teamScoresAnswerFromHalftime(null, 2)).toBeNull();
    expect(teamScoresAnswerFromHalftime(2, null)).toBeNull();
    expect(teamScoresAnswerFromHalftime(3, 1)).toBeNull(); // halftime > fulltime
  });
});

describe("halftimeGoalsFromPeriod", () => {
  it("pins none and second_half to 0", () => {
    expect(halftimeGoalsFromPeriod("none", 0)).toBe(0);
    expect(halftimeGoalsFromPeriod("second_half", 3)).toBe(0);
  });

  it("pins first_half to the full-time goal count", () => {
    expect(halftimeGoalsFromPeriod("first_half", 2)).toBe(2);
  });

  it("leaves both_halves unresolved (range, not a single value)", () => {
    expect(halftimeGoalsFromPeriod("both_halves", 3)).toBeNull();
  });
});

describe("halftimeSideBounds", () => {
  it("pins to a single disabled value when the period is first_half/second_half/none", () => {
    expect(halftimeSideBounds(2, "first_half")).toEqual({
      min: 2,
      max: 2,
      disabled: true,
    });
    expect(halftimeSideBounds(2, "second_half")).toEqual({
      min: 0,
      max: 0,
      disabled: true,
    });
    expect(halftimeSideBounds(0, "none")).toEqual({
      min: 0,
      max: 0,
      disabled: true,
    });
  });

  it("narrows (but doesn't pin) the range for both_halves", () => {
    expect(halftimeSideBounds(3, "both_halves")).toEqual({
      min: 1,
      max: 2,
      disabled: false,
    });
  });

  it("disables both_halves when fewer than 2 goals are predicted", () => {
    expect(halftimeSideBounds(1, "both_halves")).toEqual({
      min: 1,
      max: 1,
      disabled: true,
    });
  });

  it("leaves the full range open with no period chosen yet", () => {
    expect(halftimeSideBounds(3, null)).toEqual({
      min: 0,
      max: 3,
      disabled: false,
    });
  });

  it("disables the input entirely when 0 goals are predicted", () => {
    expect(halftimeSideBounds(0, null)).toEqual({
      min: 0,
      max: 0,
      disabled: true,
    });
  });
});

describe("teamScoresValidValues", () => {
  it("forces none purely from the main score when goals are 0", () => {
    expect(teamScoresValidValues(0, null)).toEqual(["none"]);
    expect(teamScoresValidValues(0, "both_halves")).toEqual(["none"]);
  });

  it("locks to the derived period when the halftime input was directly edited", () => {
    expect(teamScoresValidValues(2, "first_half")).toEqual(["first_half"]);
  });

  it("keeps the full set open when both_halves is chosen (not itself a lock)", () => {
    expect(teamScoresValidValues(3, "both_halves")).toEqual([
      "first_half",
      "second_half",
      "both_halves",
    ]);
  });

  it("keeps the full set open when nothing has been chosen yet", () => {
    expect(teamScoresValidValues(2, null)).toEqual([
      "first_half",
      "second_half",
      "both_halves",
    ]);
  });
});

describe("isAnswerStillValid", () => {
  it("invalidates a stale first_scorer answer after the scoreline changes", () => {
    expect(isAnswerStillValid("first_scorer", "home", 1, 0)).toBe(true);
    expect(isAnswerStillValid("first_scorer", "none", 1, 0)).toBe(false);
  });

  it("invalidates a stale team-scores answer after the scoreline changes", () => {
    // Was valid when away=0 ("none"), stale once away becomes 1
    expect(isAnswerStillValid("away_team_scores", "none", 1, 0)).toBe(true);
    expect(isAnswerStillValid("away_team_scores", "none", 1, 1)).toBe(false);
  });

  it("invalidates a stale halftime scoreline exceeding the new prediction", () => {
    expect(isAnswerStillValid("halftime_result", "1-0", 2, 1)).toBe(true);
    expect(isAnswerStillValid("halftime_result", "2-0", 1, 1)).toBe(false);
  });

  it("rejects a malformed halftime answer", () => {
    expect(isAnswerStillValid("halftime_result", "bogus", 1, 1)).toBe(false);
  });
});
