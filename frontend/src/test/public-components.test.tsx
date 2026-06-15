import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { I18nProvider } from "@/lib/i18n";
import type { FixtureEvent } from "@/app/api/live/fixture/[id]/route";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-query", () => ({
  useQuery: vi.fn(),
  useMutation: vi.fn().mockReturnValue({ mutate: vi.fn(), isPending: false }),
  useQueryClient: vi.fn().mockReturnValue({ invalidateQueries: vi.fn() }),
}));

// Shallow-mock LoadingState so we don't need to resolve its full dep tree.
vi.mock("@/components/shared/LoadingState", () => ({
  LoadingState: ({ rows }: { rows?: number }) => (
    <div data-testid="loading-state" data-rows={rows} />
  ),
}));

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useQuery } from "@tanstack/react-query";
import { GroupCodeLookup } from "@/components/public/GroupCodeLookup";
import {
  LiveMatchFeed,
  computeFeedInterval,
} from "@/components/public/LiveMatchFeed";
import type { TodayFixture } from "@/app/api/live/today/route";

// ── Fetch mock ────────────────────────────────────────────────────────────────

const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderLookup() {
  return render(
    <I18nProvider>
      <GroupCodeLookup />
    </I18nProvider>,
  );
}

function renderFeed() {
  return render(
    <I18nProvider>
      <LiveMatchFeed />
    </I18nProvider>,
  );
}

// toLocaleDateString("sv") returns YYYY-MM-DD in the *local* timezone — the same
// format the LiveMatchFeed component uses for its `localDate` state. Using local
// time without a Z suffix ensures the filter always passes regardless of the
// system timezone offset (e.g. UTC-6 for Guatemala).
const TODAY_LOCAL = new Date().toLocaleDateString("sv"); // "YYYY-MM-DD" in local TZ

function makeFixture(overrides: Partial<TodayFixture> = {}): TodayFixture {
  return {
    id: 1,
    homeTeam: "Brazil",
    awayTeam: "Germany",
    homeLogo: "",
    awayLogo: "",
    homeScore: null,
    awayScore: null,
    status: "NS",
    elapsed: null,
    kickoffAt: `${TODAY_LOCAL}T20:00:00`, // local time — toLocaleDateString("sv") = TODAY_LOCAL
    round: "Group Stage - 1",
    venue: null,
    ...overrides,
  };
}

// ── GroupCodeLookup ───────────────────────────────────────────────────────────

describe("GroupCodeLookup – initial render", () => {
  it("renders the input and Ver button", () => {
    renderLookup();
    expect(screen.getByRole("textbox")).toBeInTheDocument();
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  it("Ver button is disabled when input is empty", () => {
    renderLookup();
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("Ver button becomes enabled when user types a code", async () => {
    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "ABC" } });
    expect(screen.getByRole("button")).not.toBeDisabled();
  });

  it("uppercases typed text", async () => {
    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "abc123" },
    });
    expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe(
      "ABC123",
    );
  });
});

describe("GroupCodeLookup – lookup success", () => {
  beforeEach(() => mockFetch.mockReset());

  it("renders the leaderboard table after a successful fetch", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          group_name: "Copa Familia",
          entries: [
            {
              user_id: 1,
              user_name: "Jugador1",
              total_points: 30,
              rank: 1,
              prize_winner: true,
              round_points: {},
            },
            {
              user_id: 2,
              user_name: "Jugador2",
              total_points: 20,
              rank: 2,
              prize_winner: false,
              round_points: {},
            },
          ],
        }),
        { status: 200 },
      ),
    );

    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "FAM001" },
    });
    fireEvent.click(screen.getByRole("button"));

    await waitFor(() =>
      expect(screen.getByText("Copa Familia")).toBeInTheDocument(),
    );
    expect(screen.getByText("Jugador1")).toBeInTheDocument();
    expect(screen.getByText("Jugador2")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
    expect(screen.getByText("20")).toBeInTheDocument();
  });
});

describe("GroupCodeLookup – lookup error states", () => {
  beforeEach(() => mockFetch.mockReset());

  it("shows not-found error on 404", async () => {
    mockFetch.mockResolvedValueOnce(new Response("", { status: 404 }));

    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "NOPE00" },
    });
    fireEvent.click(screen.getByRole("button"));

    await waitFor(() =>
      expect(screen.getByText(/no se encontró|not found/i)).toBeInTheDocument(),
    );
  });

  it("shows generic error on non-ok response", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { message: "Internal server error" } }),
        { status: 500 },
      ),
    );

    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "ERR001" },
    });
    fireEvent.click(screen.getByRole("button"));

    await waitFor(() =>
      expect(screen.getByText("Internal server error")).toBeInTheDocument(),
    );
  });

  it("shows network error when fetch throws", async () => {
    mockFetch.mockRejectedValueOnce(new Error("network failure"));

    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "NET001" },
    });
    fireEvent.click(screen.getByRole("button"));

    await waitFor(() =>
      expect(
        screen.getByText(/pudo conectar|could not connect/i),
      ).toBeInTheDocument(),
    );
  });
});

describe("GroupCodeLookup – close button", () => {
  beforeEach(() => mockFetch.mockReset());

  it("resets back to input form after clicking the back button", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ group_name: "Liga Test", entries: [] }), {
        status: 200,
      }),
    );

    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "LIG001" },
    });
    fireEvent.click(screen.getByRole("button"));

    await waitFor(() =>
      expect(screen.getByText("Liga Test")).toBeInTheDocument(),
    );

    // Click the back (ArrowLeft) button — its aria-label contains "cerrar" or similar
    const backButton = screen.getByRole("button");
    fireEvent.click(backButton);

    await waitFor(() =>
      expect(screen.getByRole("textbox")).toBeInTheDocument(),
    );
  });
});

describe("GroupCodeLookup – empty entries", () => {
  beforeEach(() => mockFetch.mockReset());

  it("shows empty state message when group has no entries", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ group_name: "Vacío", entries: [] }), {
        status: 200,
      }),
    );

    renderLookup();
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "VAC001" },
    });
    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(screen.getByText("Vacío")).toBeInTheDocument());
    expect(screen.getByText(/puntuaciones|no scores/i)).toBeInTheDocument();
  });
});

// ── LiveMatchFeed ─────────────────────────────────────────────────────────────

describe("LiveMatchFeed – loading state", () => {
  it("renders LoadingState while query is pending", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: true,
      isError: false,
      data: undefined,
    } as never);
    renderFeed();
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – error state", () => {
  it("renders error message when query fails", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: true,
      data: undefined,
    } as never);
    renderFeed();
    // The error key resolves to a non-empty translated string.
    const panel = document.querySelector(".panel");
    expect(panel).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – empty state", () => {
  it("renders empty message when no fixtures returned", () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: { fixtures: [] },
    } as never);
    renderFeed();
    const panel = document.querySelector(".panel");
    expect(panel).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – fixture list", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        fixtures: [
          makeFixture({
            id: 1,
            homeTeam: "Brazil",
            awayTeam: "Germany",
            status: "NS",
          }),
          makeFixture({
            id: 2,
            homeTeam: "France",
            awayTeam: "Spain",
            status: "1H",
            elapsed: 30,
            homeScore: 1,
            awayScore: 0,
          }),
        ],
      },
    } as never);
  });

  it("renders team names for all fixtures (translated to active locale)", () => {
    renderFeed();
    // I18nProvider defaults to 'es'; teamName() translates via teamTranslations dict.
    expect(screen.getByText("Brasil")).toBeInTheDocument();
    expect(screen.getByText("Alemania")).toBeInTheDocument();
    expect(screen.getByText("Francia")).toBeInTheDocument();
    expect(screen.getByText("España")).toBeInTheDocument();
  });

  it("shows LIVE badge when at least one fixture is live", () => {
    renderFeed();
    // The liveBadge key renders a span with a green badge class.
    const badge = document.querySelector(".bg-green-500\\/20");
    expect(badge).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – expand/collapse fixture", () => {
  beforeEach(() => {
    // First call: today's fixtures; subsequent calls (fixture detail): return fixture stub.
    vi.mocked(useQuery).mockImplementation(
      ({ queryKey }: { queryKey: readonly unknown[] }) => {
        if (queryKey[0] === "live-today") {
          return {
            isLoading: false,
            isError: false,
            data: {
              fixtures: [
                makeFixture({
                  id: 7,
                  homeTeam: "Brazil",
                  awayTeam: "Germany",
                  status: "1H",
                  elapsed: 22,
                }),
              ],
            },
          } as never;
        }
        // live-fixture detail query
        return {
          isLoading: false,
          isError: false,
          data: {
            fixture: {
              id: 7,
              homeTeam: "Brazil",
              awayTeam: "Germany",
              homeLogo: "",
              awayLogo: "",
              homeScore: 1,
              awayScore: 0,
              halftimeHome: null,
              halftimeAway: null,
              status: "1H",
              elapsed: 22,
              kickoffAt: "2026-06-10T20:00:00Z",
              round: "Group Stage - 1",
              venue: null,
              lineups: [],
              events: [],
            },
          },
        } as never;
      },
    );
  });

  it("toggles fixture detail on click", async () => {
    renderFeed();
    const button = screen.getByRole("button", { name: /brasil|alemania/i });
    fireEvent.click(button);
    await waitFor(() =>
      expect(button).toHaveAttribute("aria-expanded", "true"),
    );
    fireEvent.click(button);
    await waitFor(() =>
      expect(button).toHaveAttribute("aria-expanded", "false"),
    );
  });
});

// ── MatchCard status variants ─────────────────────────────────────────────────

describe("LiveMatchFeed – MatchCard FT (done) status", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        fixtures: [
          makeFixture({ id: 40, status: "FT", homeScore: 2, awayScore: 1 }),
        ],
      },
    } as never);
  });

  it("shows FT label in the status chip", () => {
    renderFeed();
    expect(screen.getByText("FT")).toBeInTheDocument();
  });

  it("shows score for done fixtures", () => {
    renderFeed();
    expect(screen.getByText("2 – 1")).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – MatchCard HT (halftime) status", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        fixtures: [
          makeFixture({ id: 41, status: "HT", homeScore: 1, awayScore: 0 }),
        ],
      },
    } as never);
  });

  it("shows halftime label (MT in Spanish locale) in status chip", () => {
    renderFeed();
    expect(screen.getByText("MT")).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – MatchCard live without elapsed", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        fixtures: [
          makeFixture({
            id: 42,
            status: "2H",
            elapsed: null,
            homeScore: 0,
            awayScore: 0,
          }),
        ],
      },
    } as never);
  });

  it("shows in-play label when elapsed is null", () => {
    renderFeed();
    expect(screen.getByText("En juego")).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – MatchCard no round", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: { fixtures: [makeFixture({ id: 43, round: "" })] },
    } as never);
  });

  it("renders the fixture card without a round paragraph", () => {
    renderFeed();
    expect(screen.getByText("Brasil")).toBeInTheDocument();
  });
});

// ── FixtureDetailPanel states ─────────────────────────────────────────────────

describe("LiveMatchFeed – FixtureDetailPanel loading", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockImplementation(
      ({ queryKey }: { queryKey: readonly unknown[] }) => {
        if (queryKey[0] === "live-today")
          return {
            isLoading: false,
            isError: false,
            data: {
              fixtures: [makeFixture({ id: 50, status: "1H", elapsed: 15 })],
            },
          } as never;
        return { isLoading: true, isError: false, data: undefined } as never;
      },
    );
  });

  it("shows LoadingState while detail query is loading", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() =>
      expect(screen.getAllByTestId("loading-state").length).toBeGreaterThan(0),
    );
  });
});

describe("LiveMatchFeed – FixtureDetailPanel error", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockImplementation(
      ({ queryKey }: { queryKey: readonly unknown[] }) => {
        if (queryKey[0] === "live-today")
          return {
            isLoading: false,
            isError: false,
            data: {
              fixtures: [makeFixture({ id: 51, status: "1H", elapsed: 20 })],
            },
          } as never;
        return { isLoading: false, isError: true, data: undefined } as never;
      },
    );
  });

  it("shows no-data message when detail query fails (card stays expanded)", async () => {
    renderFeed();
    const btn = screen.getByRole("button");
    fireEvent.click(btn);
    await waitFor(() =>
      expect(
        screen.getByText("Información del partido aún no disponible."),
      ).toBeInTheDocument(),
    );
    expect(btn).toHaveAttribute("aria-expanded", "true");
  });
});

describe("LiveMatchFeed – FixtureDetailPanel missing fixture", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockImplementation(
      ({ queryKey }: { queryKey: readonly unknown[] }) => {
        if (queryKey[0] === "live-today")
          return {
            isLoading: false,
            isError: false,
            data: {
              fixtures: [makeFixture({ id: 52, status: "1H", elapsed: 20 })],
            },
          } as never;
        return { isLoading: false, isError: false, data: {} } as never;
      },
    );
  });

  it("shows no-data message when fixture is absent from response (card stays expanded)", async () => {
    renderFeed();
    const btn = screen.getByRole("button");
    fireEvent.click(btn);
    await waitFor(() =>
      expect(
        screen.getByText("Información del partido aún no disponible."),
      ).toBeInTheDocument(),
    );
    expect(btn).toHaveAttribute("aria-expanded", "true");
  });
});

describe("LiveMatchFeed – FixtureDetailPanel with halftime score", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockImplementation(
      ({ queryKey }: { queryKey: readonly unknown[] }) => {
        if (queryKey[0] === "live-today")
          return {
            isLoading: false,
            isError: false,
            data: {
              fixtures: [
                makeFixture({
                  id: 53,
                  status: "FT",
                  homeScore: 3,
                  awayScore: 1,
                }),
              ],
            },
          } as never;
        return {
          isLoading: false,
          isError: false,
          data: {
            fixture: {
              id: 53,
              homeTeam: "Brazil",
              awayTeam: "Germany",
              homeLogo: "",
              awayLogo: "",
              homeScore: 3,
              awayScore: 1,
              halftimeHome: 1,
              halftimeAway: 0,
              status: "FT",
              elapsed: null,
              kickoffAt: "2026-06-10T20:00:00Z",
              round: "Group Stage",
              venue: null,
              lineups: [],
              events: [],
            },
          },
        } as never;
      },
    );
  });

  it("renders the halftime score section", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() =>
      expect(screen.getByText(/Medio tiempo/)).toBeInTheDocument(),
    );
  });

  it("shows empty-events message when both events and lineups are empty", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() =>
      expect(screen.getByText("Sin eventos aún.")).toBeInTheDocument(),
    );
    expect(
      screen.queryByText("Información del partido aún no disponible."),
    ).toBeNull();
  });
});

// ── EventIcon branches via FixtureDetailPanel ─────────────────────────────────

function makeDetailFixture(events: FixtureEvent[], id: number) {
  return {
    id,
    homeTeam: "Brazil",
    awayTeam: "Germany",
    homeLogo: "",
    awayLogo: "",
    homeScore: 1,
    awayScore: 0,
    halftimeHome: null,
    halftimeAway: null,
    status: "1H",
    elapsed: 30,
    kickoffAt: "2026-06-10T20:00:00Z",
    round: "Group Stage",
    venue: null,
    lineups: [],
    events,
  };
}

function mockQueryWithDetail(fixtureId: number, events: FixtureEvent[]) {
  vi.mocked(useQuery).mockImplementation(
    ({ queryKey }: { queryKey: readonly unknown[] }) => {
      if (queryKey[0] === "live-today")
        return {
          isLoading: false,
          isError: false,
          data: {
            fixtures: [
              makeFixture({ id: fixtureId, status: "1H", elapsed: 30 }),
            ],
          },
        } as never;
      return {
        isLoading: false,
        isError: false,
        data: { fixture: makeDetailFixture(events, fixtureId) },
      } as never;
    },
  );
}

describe("LiveMatchFeed – EventIcon Goal (⚽)", () => {
  beforeEach(() =>
    mockQueryWithDetail(60, [
      {
        elapsed: 22,
        type: "Goal",
        detail: "Normal Goal",
        player: "Neymar",
        assist: "Vini Jr",
        team: "Brazil",
      },
    ]),
  );

  it("renders ⚽ for Goal events and shows non-subst assist in parentheses", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() => expect(screen.getByText("⚽")).toBeInTheDocument());
    expect(screen.getByText("Neymar")).toBeInTheDocument();
    expect(screen.getByText("(Vini Jr)")).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – EventIcon yellow Card (🟨)", () => {
  beforeEach(() =>
    mockQueryWithDetail(61, [
      {
        elapsed: 40,
        type: "Card",
        detail: "Yellow Card",
        player: "Müller",
        assist: null,
        team: "Germany",
      },
    ]),
  );

  it("renders 🟨 for yellow card events", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() => expect(screen.getByText("🟨")).toBeInTheDocument());
  });
});

describe("LiveMatchFeed – EventIcon red Card (🟥)", () => {
  beforeEach(() =>
    mockQueryWithDetail(62, [
      {
        elapsed: 45,
        type: "Card",
        detail: "Red Card",
        player: "Klose",
        assist: null,
        team: "Germany",
      },
    ]),
  );

  it("renders 🟥 for red card events", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() => expect(screen.getByText("🟥")).toBeInTheDocument());
  });
});

describe("LiveMatchFeed – EventIcon substitution (🔄)", () => {
  beforeEach(() =>
    mockQueryWithDetail(63, [
      {
        elapsed: 55,
        type: "subst",
        detail: "Substitution",
        player: "Ronaldo",
        assist: "Firmino",
        team: "Brazil",
      },
    ]),
  );

  it("renders 🔄 for substitution events and shows player-in with ↑", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() => expect(screen.getByText("🔄")).toBeInTheDocument());
    expect(screen.getByText(/↑ Firmino/)).toBeInTheDocument();
  });
});

describe("LiveMatchFeed – EventIcon default (📋)", () => {
  beforeEach(() =>
    mockQueryWithDetail(64, [
      {
        elapsed: 25,
        type: "Var",
        detail: "Goal cancelled",
        player: "Neymar",
        assist: null,
        team: "Brazil",
      },
    ]),
  );

  it("renders 📋 default icon for VAR and unknown event types", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() => expect(screen.getByText("📋")).toBeInTheDocument());
  });
});

// ── LineupPanel via FixtureDetailPanel ────────────────────────────────────────

describe("LiveMatchFeed – LineupPanel with formation and substitutes", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockImplementation(
      ({ queryKey }: { queryKey: readonly unknown[] }) => {
        if (queryKey[0] === "live-today")
          return {
            isLoading: false,
            isError: false,
            data: {
              fixtures: [makeFixture({ id: 70, status: "1H", elapsed: 20 })],
            },
          } as never;
        return {
          isLoading: false,
          isError: false,
          data: {
            fixture: {
              id: 70,
              homeTeam: "Brazil",
              awayTeam: "Germany",
              homeLogo: "",
              awayLogo: "",
              homeScore: 1,
              awayScore: 0,
              halftimeHome: null,
              halftimeAway: null,
              status: "1H",
              elapsed: 20,
              kickoffAt: "2026-06-10T20:00:00Z",
              round: "Group Stage",
              venue: null,
              events: [],
              lineups: [
                {
                  teamName: "Brazil",
                  formation: "4-3-3",
                  startXI: [
                    { name: "Alisson", number: 1, pos: "G" },
                    { name: "Neymar", number: 10, pos: "F" },
                  ],
                  substitutes: [{ name: "Gabriel Jesus", number: 9, pos: "F" }],
                },
              ],
            },
          },
        } as never;
      },
    );
  });

  it("renders lineups section with team name and formation", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() =>
      expect(screen.getByText("Alineaciones")).toBeInTheDocument(),
    );
    expect(screen.getByText("4-3-3")).toBeInTheDocument();
    expect(screen.getByText("Alisson")).toBeInTheDocument();
    expect(screen.getByText("Neymar")).toBeInTheDocument();
  });

  it("renders substitutes summary with count", async () => {
    renderFeed();
    fireEvent.click(screen.getByRole("button"));
    await waitFor(() =>
      expect(screen.getByText("Alineaciones")).toBeInTheDocument(),
    );
    expect(screen.getByText(/Suplentes \(1\)/)).toBeInTheDocument();
  });
});

// ── LiveMatchFeed – non-live fixtures (NS / upcoming) ─────────────────────────
//
// These tests validate that the feed renders scheduled matches correctly even
// when there are no live matches (refetchInterval returns false/long sleep).
// The "refetchOnMount: always" setting ensures the API is always called on
// page load or navigation — these tests confirm the resulting data is shown.

describe("LiveMatchFeed – scheduled (NS) fixtures are shown without live badge", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        fixtures: [
          makeFixture({
            id: 10,
            homeTeam: "Mexico",
            awayTeam: "Argentina",
            status: "NS",
            kickoffAt: `${TODAY_LOCAL}T21:00:00`,
          }),
          makeFixture({
            id: 11,
            homeTeam: "USA",
            awayTeam: "Canada",
            status: "NS",
            kickoffAt: `${TODAY_LOCAL}T23:00:00`,
          }),
        ],
      },
    } as never);
  });

  it("renders team names for all upcoming fixtures (translated to active locale)", () => {
    renderFeed();
    expect(screen.getByText("México")).toBeInTheDocument();
    expect(screen.getByText("Argentina")).toBeInTheDocument(); // same in both locales
    expect(screen.getByText("Estados Unidos")).toBeInTheDocument();
    expect(screen.getByText("Canadá")).toBeInTheDocument();
  });

  it("does NOT show the LIVE badge when no match is live", () => {
    renderFeed();
    const badge = document.querySelector(".bg-green-500\\/20.rounded-full");
    expect(badge).not.toBeInTheDocument();
  });

  it("shows 'vs' separator (not a score) for unstarted matches", () => {
    renderFeed();
    const vsLabels = screen.getAllByText("vs");
    expect(vsLabels.length).toBe(2);
  });
});

describe("LiveMatchFeed – finished (FT) fixtures are shown, polling is stopped", () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError: false,
      data: {
        fixtures: [
          makeFixture({
            id: 20,
            homeTeam: "Brazil",
            awayTeam: "Germany",
            status: "FT",
            homeScore: 2,
            awayScore: 1,
          }),
        ],
      },
    } as never);
  });

  it("renders team names and final score for a finished match", () => {
    renderFeed();
    expect(screen.getByText("Brasil")).toBeInTheDocument();
    expect(screen.getByText("Alemania")).toBeInTheDocument();
    expect(screen.getByText("2 – 1")).toBeInTheDocument();
  });

  it("computeFeedInterval returns false (no further polling) for FT-only list", () => {
    expect(
      computeFeedInterval(
        [
          makeFixture({
            id: 20,
            status: "FT",
            kickoffAt: "2026-06-14T20:00:00Z",
          }),
        ],
        NOW,
      ),
    ).toBe(false);
  });
});

// ── computeFeedInterval ───────────────────────────────────────────────────────

function fixture(
  status: string,
  kickoffAt = "2099-01-01T00:00:00Z",
): TodayFixture {
  return {
    id: 1,
    homeTeam: "A",
    awayTeam: "B",
    homeLogo: "",
    awayLogo: "",
    homeScore: null,
    awayScore: null,
    status,
    elapsed: null,
    kickoffAt,
    round: "R1",
    venue: null,
  };
}

const NOW = new Date("2026-06-14T20:00:00Z").getTime();

describe("computeFeedInterval – no fixtures", () => {
  it("returns false when fixture list is empty", () => {
    expect(computeFeedInterval([], NOW)).toBe(false);
  });
});

describe("computeFeedInterval – live match", () => {
  it("returns 30 000 ms for a 1H match", () => {
    expect(computeFeedInterval([fixture("1H")], NOW)).toBe(30_000);
  });

  it("returns 30 000 ms for HT", () => {
    expect(computeFeedInterval([fixture("HT")], NOW)).toBe(30_000);
  });

  it("live takes priority over upcoming matches", () => {
    const kickoff = new Date(NOW + 5 * 60 * 1_000).toISOString();
    expect(
      computeFeedInterval([fixture("1H"), fixture("NS", kickoff)], NOW),
    ).toBe(30_000);
  });
});

describe("computeFeedInterval – all matches done", () => {
  it("returns false when all are FT", () => {
    expect(computeFeedInterval([fixture("FT"), fixture("AET")], NOW)).toBe(
      false,
    );
  });
});

describe("computeFeedInterval – pre-match window (≤ 10 min to kickoff)", () => {
  it("returns 60 000 ms when kickoff is 5 min away", () => {
    const kickoff = new Date(NOW + 5 * 60 * 1_000).toISOString();
    expect(computeFeedInterval([fixture("NS", kickoff)], NOW)).toBe(60_000);
  });

  it("returns 60 000 ms when kickoff already passed but status still NS", () => {
    const kickoff = new Date(NOW - 2 * 60 * 1_000).toISOString();
    expect(computeFeedInterval([fixture("NS", kickoff)], NOW)).toBe(60_000);
  });

  it("returns 60 000 ms when kickoff is exactly 10 min away", () => {
    const kickoff = new Date(NOW + 10 * 60 * 1_000).toISOString();
    expect(computeFeedInterval([fixture("NS", kickoff)], NOW)).toBe(60_000);
  });
});

describe("computeFeedInterval – idle (kickoff > 10 min away)", () => {
  it("sleeps until the 10-min window opens (30 min away → 20 min sleep)", () => {
    const kickoff = new Date(NOW + 30 * 60 * 1_000).toISOString();
    expect(computeFeedInterval([fixture("NS", kickoff)], NOW)).toBe(
      20 * 60 * 1_000,
    );
  });

  it("caps sleep at 1 h when kickoff is very far away", () => {
    const kickoff = new Date(NOW + 5 * 60 * 60 * 1_000).toISOString(); // 5 h away
    expect(computeFeedInterval([fixture("NS", kickoff)], NOW)).toBe(
      60 * 60 * 1_000,
    );
  });

  it("uses soonest upcoming kickoff when multiple NS matches exist", () => {
    const soon = new Date(NOW + 25 * 60 * 1_000).toISOString(); // 25 min
    const later = new Date(NOW + 90 * 60 * 1_000).toISOString(); // 90 min
    // sleep = 25min - 10min = 15 min
    expect(
      computeFeedInterval([fixture("NS", later), fixture("NS", soon)], NOW),
    ).toBe(15 * 60 * 1_000);
  });
});
