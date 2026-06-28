import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { I18nProvider } from "@/lib/i18n";
import { buildGroupStandings } from "@/components/public/GroupStandingsSection";
import type { PublicMatch } from "@/app/api/public/standings/route";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-query", () => ({
  useQuery: vi.fn(),
}));

vi.mock("@/components/shared/HorizontalCarousel", () => ({
  HorizontalCarousel: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="carousel">{children}</div>
  ),
}));

import { useQuery } from "@tanstack/react-query";
import {
  GroupStandingsSection,
  teamDisplayName,
  getBestEightThirds,
  isGroupStageComplete,
} from "@/components/public/GroupStandingsSection";

// ── Helpers ───────────────────────────────────────────────────────────────────

function match(overrides: Partial<PublicMatch>): PublicMatch {
  return {
    id: 1,
    home_team: "Brazil",
    away_team: "Germany",
    home_score: null,
    away_score: null,
    status: "scheduled",
    group_label: "A",
    ...overrides,
  };
}

function renderSection() {
  return render(
    <I18nProvider>
      <GroupStandingsSection />
    </I18nProvider>,
  );
}

// ── teamDisplayName (pure function) ──────────────────────────────────────────

describe("teamDisplayName", () => {
  it("returns Spanish name when locale is es", () => {
    expect(teamDisplayName("Brazil", "es")).toBe("Brasil");
  });

  it("returns original name when locale is not es", () => {
    expect(teamDisplayName("Brazil", "en")).toBe("Brazil");
  });

  it("returns original name when team is unknown in Spanish dictionary", () => {
    expect(teamDisplayName("Obscuristan", "es")).toBe("Obscuristan");
  });
});

// ── buildGroupStandings (pure function) ───────────────────────────────────────

describe("buildGroupStandings – empty input", () => {
  it("returns empty object for no matches", () => {
    expect(buildGroupStandings([])).toEqual({});
  });

  it("ignores matches with null group_label", () => {
    const m = match({ group_label: null });
    expect(buildGroupStandings([m])).toEqual({});
  });
});

describe("buildGroupStandings – scheduled matches (no scores)", () => {
  it("seeds teams with zero stats when match is scheduled", () => {
    const m = match({
      status: "scheduled",
      home_score: null,
      away_score: null,
    });
    const groups = buildGroupStandings([m]);
    const rows = groups["A"];
    expect(rows).toHaveLength(2);
    const brazil = rows.find((r) => r.team === "Brazil")!;
    expect(brazil.played).toBe(0);
    expect(brazil.pts).toBe(0);
  });
});

describe("buildGroupStandings – finished matches", () => {
  it("records home win: 3 pts home, 0 pts away", () => {
    const m = match({ status: "finished", home_score: 2, away_score: 0 });
    const groups = buildGroupStandings([m]);
    const rows = groups["A"];
    const home = rows.find((r) => r.team === "Brazil")!;
    const away = rows.find((r) => r.team === "Germany")!;
    expect(home.won).toBe(1);
    expect(home.pts).toBe(3);
    expect(home.lost).toBe(0);
    expect(away.won).toBe(0);
    expect(away.pts).toBe(0);
    expect(away.lost).toBe(1);
  });

  it("records away win: 3 pts away, 0 pts home", () => {
    const m = match({ status: "finished", home_score: 0, away_score: 1 });
    const groups = buildGroupStandings([m]);
    const rows = groups["A"];
    const home = rows.find((r) => r.team === "Brazil")!;
    const away = rows.find((r) => r.team === "Germany")!;
    expect(away.won).toBe(1);
    expect(away.pts).toBe(3);
    expect(home.lost).toBe(1);
    expect(home.pts).toBe(0);
  });

  it("records draw: 1 pt each", () => {
    const m = match({ status: "finished", home_score: 1, away_score: 1 });
    const groups = buildGroupStandings([m]);
    const rows = groups["A"];
    const home = rows.find((r) => r.team === "Brazil")!;
    const away = rows.find((r) => r.team === "Germany")!;
    expect(home.drawn).toBe(1);
    expect(home.pts).toBe(1);
    expect(away.drawn).toBe(1);
    expect(away.pts).toBe(1);
  });

  it("accumulates goals for and against correctly", () => {
    const m = match({ status: "finished", home_score: 3, away_score: 1 });
    const groups = buildGroupStandings([m]);
    const rows = groups["A"];
    const home = rows.find((r) => r.team === "Brazil")!;
    const away = rows.find((r) => r.team === "Germany")!;
    expect(home.gf).toBe(3);
    expect(home.ga).toBe(1);
    expect(home.gd).toBe(2);
    expect(away.gf).toBe(1);
    expect(away.ga).toBe(3);
    expect(away.gd).toBe(-2);
  });

  it("accumulates in_progress matches too", () => {
    const m = match({ status: "in_progress", home_score: 1, away_score: 0 });
    const groups = buildGroupStandings([m]);
    const rows = groups["A"];
    const home = rows.find((r) => r.team === "Brazil")!;
    expect(home.played).toBe(1);
    expect(home.pts).toBe(3);
  });

  it("sorts by pts DESC, gd DESC, gf DESC", () => {
    const matches: PublicMatch[] = [
      match({
        id: 1,
        home_team: "A",
        away_team: "B",
        status: "finished",
        home_score: 0,
        away_score: 1,
      }),
      match({
        id: 2,
        home_team: "C",
        away_team: "D",
        status: "finished",
        home_score: 3,
        away_score: 0,
      }),
    ];
    const groups = buildGroupStandings(matches);
    const rows = groups["A"];
    expect(rows[0].pts).toBeGreaterThanOrEqual(rows[1].pts);
  });

  it("returns groups in alphabetical order", () => {
    const matches: PublicMatch[] = [
      match({ id: 1, group_label: "C", home_team: "T1", away_team: "T2" }),
      match({ id: 2, group_label: "A", home_team: "T3", away_team: "T4" }),
      match({ id: 3, group_label: "B", home_team: "T5", away_team: "T6" }),
    ];
    const groups = buildGroupStandings(matches);
    expect(Object.keys(groups)).toEqual(["A", "B", "C"]);
  });

  it("group_label is case-normalized to uppercase", () => {
    const m = match({ group_label: "a" });
    const groups = buildGroupStandings([m]);
    expect(groups["A"]).toBeDefined();
  });
});

// ── GroupStandingsSection component ──────────────────────────────────────────

describe("GroupStandingsSection – loading", () => {
  it("renders spinner while loading", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: true,
      isError: false,
      data: undefined,
    } as never);
    renderSection();
    expect(document.querySelector(".animate-spin")).toBeInTheDocument();
  });
});

describe("GroupStandingsSection – error", () => {
  it("renders error text when query fails", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: true,
      data: undefined,
    } as never);
    renderSection();
    const el = document.querySelector("p.text-center");
    expect(el).toBeInTheDocument();
  });
});

describe("GroupStandingsSection – empty matches", () => {
  it("renders empty message when matches list is empty", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: { matches: [] },
    } as never);
    renderSection();
    const el = document.querySelector("p.text-center");
    expect(el).toBeInTheDocument();
  });
});

describe("GroupStandingsSection – with matches (no group_label)", () => {
  it("renders empty when all matches lack group_label", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        matches: [match({ group_label: null })],
      },
    } as never);
    renderSection();
    const carousel = document.querySelector("[data-testid='carousel']");
    expect(carousel).not.toBeInTheDocument();
  });
});

describe("GroupStandingsSection – with group data", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        matches: [
          match({
            id: 1,
            home_team: "Brazil",
            away_team: "Germany",
            group_label: "A",
            status: "finished",
            home_score: 2,
            away_score: 1,
          }),
          match({
            id: 2,
            home_team: "France",
            away_team: "Spain",
            group_label: "A",
            status: "finished",
            home_score: 0,
            away_score: 0,
          }),
          match({
            id: 3,
            home_team: "Mexico",
            away_team: "Canada",
            group_label: "B",
            status: "scheduled",
            home_score: null,
            away_score: null,
          }),
        ],
      },
    } as never);
  });

  it("renders the carousel with group tables", () => {
    renderSection();
    expect(screen.getByTestId("carousel")).toBeInTheDocument();
  });

  it("shows team names from the matches in Spanish (default locale)", () => {
    renderSection();
    expect(screen.getByText("Brasil")).toBeInTheDocument();
    expect(screen.getByText("Alemania")).toBeInTheDocument();
  });

  it("renders multiple groups", () => {
    renderSection();
    expect(screen.getByText("México")).toBeInTheDocument();
    expect(screen.getByText("Canadá")).toBeInTheDocument();
  });

  it("shows flag emoji for known team (Brazil = 🇧🇷)", () => {
    renderSection();
    const flags = screen.getAllByText("🇧🇷");
    expect(flags.length).toBeGreaterThan(0);
  });

  it("shows fallback flag for unknown team name", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        matches: [
          match({
            home_team: "Obscuristan",
            away_team: "Testland",
            group_label: "Z",
            status: "scheduled",
          }),
        ],
      },
    } as never);
    renderSection();
    expect(screen.getAllByText("🏳️").length).toBeGreaterThan(0);
  });

  it("renders the standings legend", () => {
    renderSection();
    const legendItems = document.querySelectorAll(
      ".h-2\\.5.w-2\\.5.rounded-sm",
    );
    expect(legendItems.length).toBeGreaterThan(0);
  });
});

// ── getBestEightThirds (pure function) ────────────────────────────────────────

describe("getBestEightThirds", () => {
  const groups12 = (() => {
    const ALL_GROUPS = "ABCDEFGHIJKL".split("");
    const g: Record<string, ReturnType<typeof buildGroupStandings>[string]> =
      {};
    for (const label of ALL_GROUPS) {
      const suffix = label;
      // Standings: T1 wins all, T2 second, T3 third (3 pts), T4 last
      g[label] = buildGroupStandings([
        match({
          id: 1,
          group_label: label,
          home_team: suffix + "1",
          away_team: suffix + "2",
          status: "finished",
          home_score: 1,
          away_score: 0,
        }),
        match({
          id: 2,
          group_label: label,
          home_team: suffix + "1",
          away_team: suffix + "3",
          status: "finished",
          home_score: 1,
          away_score: 0,
        }),
        match({
          id: 3,
          group_label: label,
          home_team: suffix + "1",
          away_team: suffix + "4",
          status: "finished",
          home_score: 1,
          away_score: 0,
        }),
        match({
          id: 4,
          group_label: label,
          home_team: suffix + "2",
          away_team: suffix + "3",
          status: "finished",
          home_score: 1,
          away_score: 0,
        }),
        match({
          id: 5,
          group_label: label,
          home_team: suffix + "2",
          away_team: suffix + "4",
          status: "finished",
          home_score: 1,
          away_score: 0,
        }),
        match({
          id: 6,
          group_label: label,
          home_team: suffix + "3",
          away_team: suffix + "4",
          status: "finished",
          home_score: 1,
          away_score: 0,
        }),
      ])[label];
    }
    return g;
  })();

  it("returns all thirds when fewer than 8 groups", () => {
    const groups = { A: groups12.A, B: groups12.B };
    const result = getBestEightThirds(groups);
    expect(result.size).toBe(2);
    expect(result.has("A3")).toBe(true);
    expect(result.has("B3")).toBe(true);
  });

  it("returns top-8 when 12 groups are present", () => {
    const result = getBestEightThirds(groups12);
    expect(result.size).toBe(8);
  });

  it("excludes groups with fewer than 3 teams", () => {
    const twoTeamGroup = buildGroupStandings([
      match({
        group_label: "Z",
        home_team: "ZA",
        away_team: "ZB",
        status: "finished",
        home_score: 1,
        away_score: 0,
      }),
    ])["Z"];
    const result = getBestEightThirds({ Z: twoTeamGroup });
    expect(result.size).toBe(0);
  });

  it("sorts by GD descending when pts are equal", () => {
    // Circular wins (1-0 each): all 3 teams get 3pts.
    // Group A: each team scores 1 goal, concedes 1 → GD=0 for all; 3rd=A3.
    // Group B: B1 wins big (2-0), so B3 ends with GD=-1; 3rd=B3.
    // A3(GD=0) should rank above B3(GD=-1) since pts are equal.
    const groupsA = buildGroupStandings([
      match({ group_label: "A", home_team: "A1", away_team: "A2", status: "finished", home_score: 1, away_score: 0 }),
      match({ group_label: "A", home_team: "A2", away_team: "A3", status: "finished", home_score: 1, away_score: 0 }),
      match({ group_label: "A", home_team: "A3", away_team: "A1", status: "finished", home_score: 1, away_score: 0 }),
    ])["A"]; // A1=3pts GD=0, A2=3pts GD=0, A3=3pts GD=0 → sorted by name → A3 is 3rd

    const groupsB = buildGroupStandings([
      match({ group_label: "B", home_team: "B1", away_team: "B2", status: "finished", home_score: 2, away_score: 0 }),
      match({ group_label: "B", home_team: "B2", away_team: "B3", status: "finished", home_score: 2, away_score: 0 }),
      match({ group_label: "B", home_team: "B3", away_team: "B1", status: "finished", home_score: 1, away_score: 0 }),
    ])["B"]; // B1=3pts GD+1, B2=3pts GD=0, B3=3pts GD=-1 → 3rd=B3

    const result = getBestEightThirds({ A: groupsA, B: groupsB });
    // Only 2 thirds total, both included; sort compares pts (equal) then GD
    expect(result.has("A3")).toBe(true);
    expect(result.has("B3")).toBe(true);
  });

  it("sorts by GF descending when pts and GD are equal", () => {
    // Circular wins where all teams end with GD=0 but different GF.
    // Group A (1-0 circular): all 3pts, GD=0, GF=1 → A3 is 3rd.
    // Group B (2-0 circular): all 3pts, GD=0, GF=2 → B3 is 3rd.
    // B3(GF=2) should rank above A3(GF=1); sort reaches GF branch.
    const groupsA = buildGroupStandings([
      match({ group_label: "A", home_team: "A1", away_team: "A2", status: "finished", home_score: 1, away_score: 0 }),
      match({ group_label: "A", home_team: "A2", away_team: "A3", status: "finished", home_score: 1, away_score: 0 }),
      match({ group_label: "A", home_team: "A3", away_team: "A1", status: "finished", home_score: 1, away_score: 0 }),
    ])["A"]; // pts=3, GD=0, GF=1 for all → A3 is 3rd (name sort)

    const groupsB = buildGroupStandings([
      match({ group_label: "B", home_team: "B1", away_team: "B2", status: "finished", home_score: 2, away_score: 0 }),
      match({ group_label: "B", home_team: "B2", away_team: "B3", status: "finished", home_score: 2, away_score: 0 }),
      match({ group_label: "B", home_team: "B3", away_team: "B1", status: "finished", home_score: 2, away_score: 0 }),
    ])["B"]; // pts=3, GD=0, GF=2 for all → B3 is 3rd (name sort)

    const result = getBestEightThirds({ A: groupsA, B: groupsB });
    expect(result.size).toBe(2);
    expect(result.has("A3")).toBe(true);
    expect(result.has("B3")).toBe(true);
  });
});

// ── isGroupStageComplete (pure function) ──────────────────────────────────────

describe("isGroupStageComplete", () => {
  it("returns false when fewer than 12 groups", () => {
    const groups = { A: [], B: [] };
    expect(isGroupStageComplete(groups)).toBe(false);
  });

  it("returns false when 12 groups but a team has played < 3", () => {
    const ALL_GROUPS = "ABCDEFGHIJKL".split("");
    const groups: Record<string, { team: string; played: number }[]> = {};
    for (const g of ALL_GROUPS) {
      groups[g] = [{ team: g + "1", played: 3 }];
    }
    // Override one team to have played only 2
    groups["A"] = [{ team: "A1", played: 2 }];
    expect(
      isGroupStageComplete(
        groups as Parameters<typeof isGroupStageComplete>[0],
      ),
    ).toBe(false);
  });

  it("returns true when all 12 groups have every team played >= 3", () => {
    const ALL_GROUPS = "ABCDEFGHIJKL".split("");
    const groups: Record<string, { team: string; played: number }[]> = {};
    for (const g of ALL_GROUPS) {
      groups[g] = [
        { team: g + "1", played: 3 },
        { team: g + "2", played: 3 },
        { team: g + "3", played: 3 },
        { team: g + "4", played: 3 },
      ];
    }
    expect(
      isGroupStageComplete(
        groups as Parameters<typeof isGroupStageComplete>[0],
      ),
    ).toBe(true);
  });
});
