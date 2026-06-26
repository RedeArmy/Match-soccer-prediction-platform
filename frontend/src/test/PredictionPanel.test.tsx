import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { PredictionPanel } from "@/components/predictions/PredictionPanel";
import { api } from "@/lib/api";

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi
    .fn()
    .mockReturnValue({ getToken: vi.fn().mockResolvedValue("tok") }),
}));

// Stub the system-clock endpoint so PredictionPanel's systemClockQuery resolves.
vi.stubGlobal(
  "fetch",
  vi.fn().mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ now: new Date().toISOString() }),
  } as unknown as Response),
);

vi.mock("@/lib/api", () => ({
  api: {
    getMatches: vi.fn(),
    getMyPredictions: vi.fn(),
    submitPrediction: vi.fn(),
    updatePrediction: vi.fn(),
    getSlots: vi.fn().mockResolvedValue([]),
  },
}));

const futureKickoff = "2099-06-11T18:00:00Z";
const pastKickoff = "2026-01-01T18:00:00Z";

const scheduledMatch = {
  id: 10,
  home_team: "Canada",
  away_team: "Mexico",
  home_score: null,
  away_score: null,
  status: "scheduled",
  kickoff_at: futureKickoff,
  stadium: { name: "Toronto Stadium" },
  phase: "group_stage",
  group_label: "A",
};

const lockedMatch = {
  ...scheduledMatch,
  id: 11,
  home_team: "USA",
  away_team: "Guatemala",
  status: "in_progress",
  kickoff_at: pastKickoff,
  stadium: null,
  phase: null,
  group_label: "B",
};

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <PredictionPanel />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

describe("PredictionPanel", () => {
  beforeEach(() => {
    vi.mocked(api.getMatches).mockReset();
    vi.mocked(api.getMyPredictions).mockReset();
    vi.mocked(api.submitPrediction).mockReset();
    vi.mocked(api.updatePrediction).mockReset();
    vi.mocked(api.submitPrediction).mockResolvedValue({
      id: 1,
      match_id: scheduledMatch.id,
      home_score: 2,
      away_score: 1,
      points: null,
      scored_at: null,
      created_at: "",
    });
    vi.mocked(api.updatePrediction).mockResolvedValue({
      id: 99,
      match_id: scheduledMatch.id,
      home_score: 3,
      away_score: 1,
      points: 5,
      scored_at: null,
      created_at: "",
    });
  });

  it("renders the empty state when no matches exist", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([]);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    // by-group view (default) with no matches shows the group-level empty state
    expect(
      await screen.findByText("Sin partidos en este grupo"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Este grupo no tiene partidos/),
    ).toBeInTheDocument();
  });

  it("submits a new prediction for a scheduled match", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    expect(await screen.findByText("Canadá")).toBeInTheDocument();
    const inputs = screen.getAllByRole("spinbutton");
    fireEvent.change(inputs[0], { target: { value: "2" } });
    fireEvent.change(inputs[1], { target: { value: "1" } });
    fireEvent.click(screen.getByRole("button", { name: /Guardar prediccion/ }));

    await waitFor(() => {
      expect(api.submitPrediction).toHaveBeenCalledWith("tok", {
        match_id: scheduledMatch.id,
        home_score: 2,
        away_score: 1,
      });
    });
    expect(await screen.findByText("Prediccion guardada")).toBeInTheDocument();
  });

  it("updates an existing prediction and filters saved matches", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      scheduledMatch,
      lockedMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([
      {
        id: 99,
        match_id: scheduledMatch.id,
        home_score: 1,
        away_score: 1,
        points: 3,
        scored_at: null,
        created_at: "",
      },
    ]);

    renderPanel();

    expect(await screen.findByText("Canadá")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Guardados" }));
    expect(screen.queryByText("Estados Unidos")).toBeNull();

    const inputs = screen.getAllByRole("spinbutton");
    fireEvent.change(inputs[0], { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: /Actualizar/ }));

    await waitFor(() => {
      expect(api.updatePrediction).toHaveBeenCalledWith("tok", 99, {
        home_score: 3,
        away_score: 1,
      });
    });
  });

  it("filters match predictions by selected group", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      scheduledMatch,
      lockedMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    // Default: group A selected — Canada (group A) visible, USA (group B) not
    expect(await screen.findByText("Canadá")).toBeInTheDocument();
    expect(screen.queryByText("Estados Unidos")).toBeNull();

    // Switch to group B
    fireEvent.click(screen.getByRole("button", { name: /^B/ }));

    expect(screen.getByText("Estados Unidos")).toBeInTheDocument();
    expect(screen.queryByText("Canadá")).toBeNull();

    // Switch back to group A
    fireEvent.click(screen.getByRole("button", { name: /^A/ }));

    expect(screen.getByText("Canadá")).toBeInTheDocument();
    expect(screen.queryByText("Estados Unidos")).toBeNull();
  });

  it("shows error state when getMatches rejects", async () => {
    vi.mocked(api.getMatches).mockRejectedValueOnce(new Error("Network error"));
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    expect(
      await screen.findByText("No se pudieron cargar los partidos"),
    ).toBeInTheDocument();
  });

  it('filters past matches with the "past" filter', async () => {
    const finishedMatch = {
      ...scheduledMatch,
      id: 15,
      home_team: "Argentina",
      away_team: "Chile",
      status: "finished",
      home_score: 2,
      away_score: 1,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      scheduledMatch,
      finishedMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá"); // wait for data to load in group A

    // Switch to Past filter — only finished match (Argentina) should appear
    fireEvent.click(screen.getByRole("button", { name: "Pasados" }));
    expect(await screen.findByText("Argentina")).toBeInTheDocument();
    expect(screen.queryByText("Canadá")).toBeNull();
  });

  it("shows today-filter empty state when no matches are scheduled today", async () => {
    const farFutureMatch = {
      ...scheduledMatch,
      id: 20,
      kickoff_at: "2099-12-31T18:00:00Z",
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([farFutureMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá"); // wait for data to load

    // Switch to by-day view
    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));

    // No matches today → noTodayMatches empty state
    expect(
      await screen.findByText("Sin partidos para hoy"),
    ).toBeInTheDocument();
  });

  it("shows mutation error feedback on submission failure", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);
    vi.mocked(api.submitPrediction).mockRejectedValueOnce(
      new Error("Server error"),
    );

    renderPanel();

    expect(await screen.findByText("Canadá")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Guardar prediccion/ }));

    expect(
      await screen.findByText("No se pudo guardar la prediccion"),
    ).toBeInTheDocument();
  });

  it("shows points placeholder when finished match has no scored prediction", async () => {
    const finishedMatch = {
      ...scheduledMatch,
      id: 30,
      status: "finished",
      home_score: 1,
      away_score: 0,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([finishedMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    // Switch to past filter to see the finished match
    fireEvent.click(screen.getByRole("button", { name: "Pasados" }));
    await screen.findByText("Canadá");

    // No prediction → points should show "–"
    expect(screen.getByText("–")).toBeInTheDocument();
  });

  it("shows allSavedToday empty state in by-day+pending view", async () => {
    // Use noon *local* time so the match is always "today" regardless of the
    // system timezone offset. The PredictionPanel filters by local date using the
    // i18n timeZone (which inherits the JSDOM environment's system timezone).
    // toLocaleDateString("sv") + "T12:00:00" (no Z) → parsed as local noon.
    const todayLocal = new Date().toLocaleDateString("sv");
    const todayKickoff = `${todayLocal}T12:00:00`;
    const todayMatch = {
      ...scheduledMatch,
      id: 40,
      kickoff_at: todayKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([todayMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([
      {
        id: 1,
        match_id: todayMatch.id,
        home_score: 2,
        away_score: 0,
        points: null,
        scored_at: null,
        created_at: "",
      },
    ]);

    renderPanel();
    await screen.findByText("Canadá");

    // Switch to by-day, then pending — all today's matches have predictions
    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));
    fireEvent.click(screen.getByRole("button", { name: "Pendientes" }));

    expect(
      await screen.findByText("Todas las predicciones del día guardadas"),
    ).toBeInTheDocument();
  });

  it("locks inputs when kickoff has passed but status is still scheduled (pendingSync)", async () => {
    const pastKickoffLocal = `${new Date().toLocaleDateString("sv")}T00:00:00`;
    const pastKickoffMatch = {
      ...scheduledMatch,
      id: 50,
      home_team: "Brazil",
      away_team: "Portugal",
      status: "scheduled",
      kickoff_at: pastKickoffLocal,
      phase: null,
      group_label: "C",
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      pastKickoffMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /^C/ }));
    expect(await screen.findByText("Brasil")).toBeInTheDocument();

    // Inputs must be disabled and save button disabled even though DB says "scheduled"
    expect(screen.getAllByRole("spinbutton")[0]).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /Guardar prediccion/ }),
    ).toBeDisabled();
    // Amber "Iniciando..." badge must appear
    expect(screen.getByText("Iniciando...")).toBeInTheDocument();
  });

  it("renders in_progress matches before scheduled ones even when kickoff is later", async () => {
    // earlierScheduled has an earlier kickoff than liveMatch; without the
    // live-first sort, it would be rendered first.
    const earlierScheduled = {
      ...scheduledMatch,
      id: 60,
      home_team: "France",
      away_team: "Spain",
      status: "scheduled",
      kickoff_at: "2026-06-14T10:00:00Z",
      group_label: "A",
    };
    const liveMatch = {
      ...scheduledMatch,
      id: 61,
      home_team: "Brazil",
      away_team: "Argentina",
      status: "in_progress",
      kickoff_at: "2026-06-14T14:00:00Z",
      group_label: "A",
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      earlierScheduled,
      liveMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    const { container } = renderPanel();

    // Wait for both teams to be in the DOM (group A is default view)
    await screen.findByText("Francia");
    await screen.findByText("Brasil");

    const articles = container.querySelectorAll("article");
    expect(articles[0].textContent).toContain("Brasil");
    expect(articles[1].textContent).toContain("Francia");
  });

  it("switches to by-day view and resets past filter to all", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá");

    // Enter past filter first
    fireEvent.click(screen.getByRole("button", { name: "Pasados" }));
    // Switch to Partidos view — should reset filter to "Todos" and hide "Pasados" chip
    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));

    // "Pasados" chip should no longer be visible in by-day mode
    expect(screen.queryByRole("button", { name: "Pasados" })).toBeNull();
  });

  it("shows range empty state when no matches exist in a date range", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá");

    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));
    fireEvent.click(screen.getByRole("button", { name: "Abrir calendario" }));

    // Click a far-future day to set start, then click another to set range end
    // Use aria-label to pick a day button in the calendar; clicking
    // non-match days creates a range with no matches in it.
    const dayButtons = screen
      .getAllByRole("button", {
        name: (name) => /^\d{4}-\d{2}-\d{2}$/.test(name),
      })
      .filter((btn) => !btn.hasAttribute("disabled"));

    if (dayButtons.length >= 2) {
      fireEvent.click(dayButtons[0]); // first click → pendingStart
      fireEvent.click(dayButtons[1]); // second click → range end

      // If these days have no matches → range empty state
      // (The test verifies the range-selection path runs without error)
      await screen.findByText(/Sin partidos/);
    } else {
      // Calendar may only show today (match date = today scenario already covered)
      expect(screen.queryByText(/Sin partidos/)).not.toBeNull();
    }
  });

  it("shows saved filter empty state in by-day mode", async () => {
    const todayLocal = new Date().toLocaleDateString("sv");
    const todayKickoff = `${todayLocal}T14:00:00`;
    const todayMatch = {
      ...scheduledMatch,
      id: 70,
      kickoff_at: todayKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([todayMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá");

    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));
    fireEvent.click(screen.getByRole("button", { name: "Guardados" }));

    expect(
      await screen.findByText("Aún no tienes predicciones guardadas hoy"),
    ).toBeInTheDocument();
  });

  it("navigates calendar to next month and back", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá");

    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));
    fireEvent.click(screen.getByRole("button", { name: "Abrir calendario" }));

    const nextBtn = screen.getByRole("button", {
      name: (name) => /next|siguiente|próximo/i.test(name) || name === "",
    });

    // Use aria-label from the calendar buttons
    const allButtons = screen.getAllByRole("button");
    const nextMonthBtn = allButtons.find(
      (b) => b.getAttribute("aria-label") === "Mes siguiente",
    );
    const prevMonthBtn = allButtons.find(
      (b) => b.getAttribute("aria-label") === "Mes anterior",
    );

    if (nextMonthBtn && prevMonthBtn) {
      fireEvent.click(nextMonthBtn);
      // prev month should now be enabled (we moved forward)
      expect(prevMonthBtn).not.toBeDisabled();

      fireEvent.click(prevMonthBtn);
    }
    // Calendar still renders without error
    expect(
      screen.getByRole("button", { name: "Partidos" }),
    ).toBeInTheDocument();
    void nextBtn;
  });

  it("closes calendar dropdown when clicking outside the container", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá");

    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));
    fireEvent.click(screen.getByRole("button", { name: "Abrir calendario" }));
    expect(
      screen.getByRole("button", { name: "Mes siguiente" }),
    ).toBeInTheDocument();

    // Simulate a mousedown outside the calendar container
    fireEvent.mouseDown(document.body);

    expect(screen.queryByRole("button", { name: "Mes siguiente" })).toBeNull();
  });

  it("shows Partidos de hoy button when a non-today date is selected and resets to today on click", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    await screen.findByText("Canadá");

    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));
    fireEvent.click(screen.getByRole("button", { name: "Abrir calendario" }));

    // Today is pre-selected → button should not be visible
    expect(
      screen.queryByRole("button", { name: "Partidos de hoy" }),
    ).toBeNull();

    const todayLocal = new Date().toLocaleDateString("sv");
    const futureDays = screen
      .getAllByRole("button", {
        name: (name) => /^\d{4}-\d{2}-\d{2}$/.test(name),
      })
      .filter(
        (btn) =>
          !btn.hasAttribute("disabled") &&
          btn.getAttribute("aria-label") !== todayLocal,
      );

    if (futureDays.length > 0) {
      fireEvent.click(futureDays[0]);
      expect(
        screen.getByRole("button", { name: "Partidos de hoy" }),
      ).toBeInTheDocument();
      // Clicking it resets the selection to today
      fireEvent.click(screen.getByRole("button", { name: "Partidos de hoy" }));
      expect(
        screen.queryByRole("button", { name: "Partidos de hoy" }),
      ).toBeNull();
    }
  });

  it("shows knockout phase tab when matches with that phase are present", async () => {
    const koMatch = {
      ...scheduledMatch,
      id: 20,
      home_team: "Argentina",
      away_team: "Brazil",
      phase: "round_of_16",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      scheduledMatch,
      koMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    await screen.findByText("Canadá");
    // The round_of_16 tab renders as "Octavos" in Spanish
    expect(
      screen.getByRole("button", { name: /Octavos/i }),
    ).toBeInTheDocument();
  });

  it("filters matches by knockout phase when tab is clicked", async () => {
    const koMatch = {
      ...scheduledMatch,
      id: 20,
      home_team: "Argentina",
      away_team: "Brazil",
      phase: "round_of_16",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      scheduledMatch,
      koMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    await screen.findByText("Canadá");
    fireEvent.click(screen.getByRole("button", { name: /Octavos/i }));

    // After switching to round_of_16 tab, the knockout match teams are shown
    expect(await screen.findByText("Argentina")).toBeInTheDocument();
    // Group stage match is no longer visible in the phase filter view
    expect(screen.queryByText("Canadá")).toBeNull();
  });

  it("hides unconfirmed knockout matches (empty teams) within an active phase tab", async () => {
    const confirmedKo = {
      ...scheduledMatch,
      id: 21,
      home_team: "France",
      away_team: "Spain",
      phase: "round_of_16",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    // Same phase but teams not yet determined
    const pendingKo = {
      ...scheduledMatch,
      id: 22,
      home_team: "",
      away_team: "",
      phase: "round_of_16",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      confirmedKo,
      pendingKo,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    // Tab appears because at least one match is confirmed
    expect(
      await screen.findByRole("button", { name: /Octavos/i }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Octavos/i }));

    // Only the confirmed match renders — the empty-team one is suppressed
    expect(await screen.findByText("Francia")).toBeInTheDocument();
    // One article only (not two)
    expect(document.querySelectorAll("article")).toHaveLength(1);
  });

  it("hides knockout matches with bracket placeholder codes and suppresses the tab when all are placeholders", async () => {
    // All matches in round_of_32 still have unresolved bracket codes
    const placeholderMatch1 = {
      ...scheduledMatch,
      id: 30,
      home_team: "2A",
      away_team: "2B",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    const placeholderMatch2 = {
      ...scheduledMatch,
      id: 31,
      home_team: "1E",
      away_team: "3ABCDF",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      scheduledMatch,
      placeholderMatch1,
      placeholderMatch2,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    await screen.findByText("Canadá");
    // round_of_32 tab must NOT appear — every match has placeholder codes
    expect(screen.queryByRole("button", { name: /Dieciseisavos/i })).toBeNull();
  });

  it("shows knockout tab and hides placeholder matches when phase has mix of confirmed and placeholder", async () => {
    const confirmedKo = {
      ...scheduledMatch,
      id: 32,
      home_team: "Brazil",
      away_team: "Argentina",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    const placeholderKo = {
      ...scheduledMatch,
      id: 33,
      home_team: "W73",
      away_team: "W75",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      confirmedKo,
      placeholderKo,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();

    // Tab appears because at least one match is confirmed
    expect(
      await screen.findByRole("button", { name: /Dieciseisavos/i }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Dieciseisavos/i }));

    // Only the confirmed match renders
    expect(await screen.findByText("Brasil")).toBeInTheDocument();
    expect(document.querySelectorAll("article")).toHaveLength(1);
  });

  it("enriches knockout match teams from confirmed slot auto_source codes", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      {
        id: 100,
        label: "winner_group_a",
        description: "1er Grupo A",
        auto_source: "1A",
        team: "Mexico",
        created_at: "",
        updated_at: "",
      },
    ]);
    const koMatch = {
      ...scheduledMatch,
      id: 90,
      home_team: "1A",
      away_team: "Brazil",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: futureKickoff,
    };
    // useSlots cross-invalidates ["matches"] when it detects confirmed slot teams,
    // so getMatches may be called more than once — use mockResolvedValue (not Once).
    vi.mocked(api.getMatches).mockResolvedValue([koMatch] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValue([]);

    renderPanel();

    // After enrichment "1A" → "Mexico" (i18n: "México"); phase tab becomes visible
    // because the match now has a confirmed team name instead of a placeholder code.
    expect(
      await screen.findByRole("button", { name: /Dieciseisavos/i }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Dieciseisavos/i }));

    expect(await screen.findByText("México")).toBeInTheDocument();
    expect(screen.queryByText("1A")).toBeNull();
  });

  it("hides placeholder-team matches in by-day view", async () => {
    const todayLocal = new Date().toLocaleDateString("sv");
    const todayKickoff = `${todayLocal}T14:00:00`;
    const confirmedMatch = {
      ...scheduledMatch,
      id: 80,
      home_team: "Brazil",
      away_team: "Argentina",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: todayKickoff,
    };
    const placeholderMatch = {
      ...scheduledMatch,
      id: 81,
      home_team: "W73",
      away_team: "1A",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: todayKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      confirmedMatch,
      placeholderMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));

    // Confirmed teams appear; placeholder match is suppressed
    expect(await screen.findByText("Brasil")).toBeInTheDocument();
    expect(screen.queryByText("W73")).toBeNull();
    expect(screen.queryByText("1A")).toBeNull();
  });

  it("omits calendar dot for days where all matches have placeholder teams", async () => {
    const todayLocal = new Date().toLocaleDateString("sv");
    const todayKickoff = `${todayLocal}T14:00:00`;
    const placeholderOnlyMatch = {
      ...scheduledMatch,
      id: 82,
      home_team: "2C",
      away_team: "3ABCDF",
      phase: "round_of_32",
      group_label: null,
      kickoff_at: todayKickoff,
    };
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      placeholderOnlyMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([]);

    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Partidos" }));

    // Calendar opens — today's cell must not have the gold dot
    fireEvent.click(screen.getByRole("button", { name: "Abrir calendario" }));
    const todayBtn = screen.getByRole("button", { name: todayLocal });
    const cell = todayBtn.closest("div");
    // The dot is a <span> sibling inside the cell; it must not exist
    expect(cell?.querySelector("span.rounded-full")).toBeNull();
  });

  it("disables score editing for locked matches and filters pending matches", async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([
      scheduledMatch,
      lockedMatch,
    ] as never);
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([
      {
        id: 99,
        match_id: scheduledMatch.id,
        home_score: 1,
        away_score: 1,
        points: null,
        scored_at: null,
        created_at: "",
      },
    ]);

    renderPanel();

    // Default: group A; Canada has a prediction
    expect(await screen.findByText("Canadá")).toBeInTheDocument();

    // Navigate to group B where the locked USA/Guatemala match lives
    fireEvent.click(screen.getByRole("button", { name: /^B/ }));
    expect(screen.getByText("Estados Unidos")).toBeInTheDocument();

    // Pendientes filter: Canada (group A, has prediction) is not shown;
    // USA (group B, in_progress) remains visible with EN VIVO badge and locked inputs
    fireEvent.click(screen.getByRole("button", { name: "Pendientes" }));
    expect(screen.queryByText("Canadá")).toBeNull();
    expect(screen.getByText("EN VIVO")).toBeInTheDocument();
    expect(screen.getAllByRole("spinbutton")[0]).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /Guardar prediccion/ }),
    ).toBeDisabled();
  });
});
