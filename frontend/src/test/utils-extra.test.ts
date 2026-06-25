import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  formatDate,
  formatDateTime,
  formatRelative,
  formatCountdown,
  truncate,
  initials,
  cn,
} from "@/lib/utils";
import { isPhaseVisible, visibleKnockoutPhases } from "@/lib/feature-flags";

describe("cn (class name merger)", () => {
  it("merges class names", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("removes conflicting tailwind classes (last wins)", () => {
    expect(cn("text-red-400", "text-blue-400")).toBe("text-blue-400");
  });

  it("handles conditional false values", () => {
    expect(cn("foo", false && "bar")).toBe("foo");
  });
});

describe("formatDate", () => {
  it("returns a non-empty string for a valid ISO date", () => {
    const result = formatDate("2026-06-01T10:00:00Z");
    expect(typeof result).toBe("string");
    expect(result.length).toBeGreaterThan(0);
  });

  it("includes the year", () => {
    const result = formatDate("2026-06-01T10:00:00Z");
    expect(result).toContain("2026");
  });
});

describe("formatDateTime", () => {
  it("returns a longer string than formatDate (includes time)", () => {
    const date = formatDate("2026-06-01T10:00:00Z");
    const dateTime = formatDateTime("2026-06-01T10:00:00Z");
    expect(dateTime.length).toBeGreaterThan(date.length);
  });
});

describe("formatRelative", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "ahora" or similar for a time a few seconds ago', () => {
    vi.setSystemTime(new Date("2026-06-01T10:00:10Z"));
    const result = formatRelative("2026-06-01T10:00:00Z");
    expect(typeof result).toBe("string");
    expect(result.length).toBeGreaterThan(0);
  });

  it("returns a relative string for a future date within 1h", () => {
    vi.setSystemTime(new Date("2026-06-01T10:00:00Z"));
    const result = formatRelative("2026-06-01T10:30:00Z");
    expect(result).toMatch(/min/);
  });

  it("falls back to formatted date for dates > 7 days away", () => {
    vi.setSystemTime(new Date("2026-06-01T10:00:00Z"));
    const result = formatRelative("2026-05-01T10:00:00Z");
    expect(result).toContain("2026");
  });
});

describe("formatCountdown", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns Finalizado for past date", () => {
    vi.setSystemTime(new Date("2026-06-01T10:00:00Z"));
    expect(formatCountdown("2026-05-31T10:00:00Z")).toBe("Finalizado");
  });

  it("shows days and hours for multi-day future", () => {
    vi.setSystemTime(new Date("2026-06-01T10:00:00Z"));
    const result = formatCountdown("2026-06-04T10:00:00Z");
    expect(result).toMatch(/3d/);
  });

  it("shows hours and minutes when less than a day", () => {
    vi.setSystemTime(new Date("2026-06-01T10:00:00Z"));
    const result = formatCountdown("2026-06-01T16:30:00Z");
    expect(result).toMatch(/6h/);
  });

  it("shows minutes only when less than an hour", () => {
    vi.setSystemTime(new Date("2026-06-01T10:00:00Z"));
    const result = formatCountdown("2026-06-01T10:45:00Z");
    expect(result).toMatch(/45m/);
  });
});

describe("truncate", () => {
  it("returns original string when shorter than maxLen", () => {
    expect(truncate("hello", 10)).toBe("hello");
  });

  it("truncates and appends ellipsis when longer than maxLen", () => {
    expect(truncate("hello world", 5)).toBe("hello…");
  });

  it("returns exact string when length equals maxLen", () => {
    expect(truncate("hello", 5)).toBe("hello");
  });
});

describe("initials", () => {
  it("extracts first letter of each word (max 2)", () => {
    expect(initials("Carlos López")).toBe("CL");
  });

  it("handles single name", () => {
    expect(initials("Carlos")).toBe("C");
  });

  it("ignores words beyond the second", () => {
    expect(initials("Juan Carlos López García")).toBe("JC");
  });

  it("uppercases letters", () => {
    expect(initials("ana beatriz")).toBe("AB");
  });
});

describe("isPhaseVisible", () => {
  it("returns true for null", () => {
    expect(isPhaseVisible(null)).toBe(true);
  });

  it("returns true for undefined", () => {
    expect(isPhaseVisible(undefined)).toBe(true);
  });

  it("returns true for group_stage", () => {
    expect(isPhaseVisible("group_stage")).toBe(true);
  });

  it("returns true for all known knockout phases (all flags enabled)", () => {
    expect(isPhaseVisible("round_of_16")).toBe(true);
    expect(isPhaseVisible("round_of_32")).toBe(true);
    expect(isPhaseVisible("quarter_final")).toBe(true);
    expect(isPhaseVisible("semi_final")).toBe(true);
    expect(isPhaseVisible("third_place")).toBe(true);
    expect(isPhaseVisible("final")).toBe(true);
  });

  it("returns false for an unrecognised phase", () => {
    expect(isPhaseVisible("unknown_phase")).toBe(false);
  });
});

describe("visibleKnockoutPhases", () => {
  it("returns empty array when given no matches", () => {
    expect(visibleKnockoutPhases([])).toEqual([]);
  });

  it("returns empty array when only group_stage matches present", () => {
    expect(visibleKnockoutPhases([{ phase: "group_stage" }, { phase: null }])).toEqual([]);
  });

  it("returns empty array when knockout matches have no confirmed teams", () => {
    const matches = [
      { phase: "round_of_16", home_team: "", away_team: "" },
      { phase: "semi_final", home_team: "Brazil", away_team: "" },
      { phase: "final" },
    ];
    expect(visibleKnockoutPhases(matches)).toEqual([]);
  });

  it("returns phases in Final-first canonical order regardless of input order", () => {
    const confirmed = { home_team: "Brazil", away_team: "Argentina" };
    const matches = [
      { phase: "quarter_final", ...confirmed },
      { phase: "round_of_16", ...confirmed },
      { phase: "semi_final", ...confirmed },
    ];
    expect(visibleKnockoutPhases(matches)).toEqual([
      "semi_final",
      "quarter_final",
      "round_of_16",
    ]);
  });

  it("deduplicates repeated phase values", () => {
    const confirmed = { home_team: "Spain", away_team: "France" };
    const matches = [
      { phase: "final", ...confirmed },
      { phase: "final", ...confirmed },
      { phase: "third_place", ...confirmed },
    ];
    expect(visibleKnockoutPhases(matches)).toEqual(["final", "third_place"]);
  });

  it("ignores unrecognised phase strings even when teams are confirmed", () => {
    const match = { phase: "unknown_phase", home_team: "X", away_team: "Y" };
    expect(visibleKnockoutPhases([match])).toEqual([]);
  });
});
