import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { I18nProvider } from "@/lib/i18n";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("@/lib/api", () => ({
  api: {
    getSlots: vi.fn(),
  },
}));

import { api } from "@/lib/api";
import { KnockoutBracket } from "@/components/public/KnockoutBracket";

// Pre-seed locale so I18nProvider skips the geo round-trip.
beforeAll(() => {
  globalThis.localStorage.setItem("quiniela-locale", "es");
  globalThis.localStorage.setItem("quiniela-locale-source", "explicit");
});

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeSlot(
  id: number,
  label: string,
  description: string,
  team: string | null = null,
) {
  return {
    id,
    label,
    description,
    team,
    confirmed_at: team ? new Date().toISOString() : null,
  };
}

function renderBracket() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <KnockoutBracket />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("KnockoutBracket", () => {
  it("renders nothing when there are no slots at all", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([] as never);

    const { container } = renderBracket();

    await vi.waitFor(() =>
      expect(container.querySelector("section")).toBeNull(),
    );
  });

  it("renders nothing when no slot has a team", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1.° Grupo A"),
      makeSlot(2, "r32_01_b", "2.° Grupo B"),
    ] as never);

    const { container } = renderBracket();

    await vi.waitFor(() =>
      expect(container.querySelector("section")).toBeNull(),
    );
  });

  it("renders the bracket when only one slot of a match is confirmed", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1.° Grupo A", "Sudáfrica"),
      makeSlot(2, "r32_01_b", "2.° Grupo B"), // second slot still pending
    ] as never);

    renderBracket();

    // Phase is visible as soon as any slot has a team
    expect(await screen.findByText("Dieciseisavos")).toBeInTheDocument();
    expect(screen.getByText("Sudáfrica")).toBeInTheDocument();
    // Unconfirmed slot shows its placeholder description
    expect(screen.getByText("2.° Grupo B")).toBeInTheDocument();
  });

  it("renders the bracket section when a match has both slots confirmed", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "fin_01_a", "Campeón", "Argentina"),
      makeSlot(2, "fin_01_b", "Subcampeón", "Francia"),
    ] as never);

    renderBracket();

    expect(await screen.findByText("Rondas Finales")).toBeInTheDocument();
    expect(screen.getByText("Final")).toBeInTheDocument();
  });

  it("shows confirmed team name inside the phase accordion", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "fin_01_a", "Campeón", "Brasil"),
      makeSlot(2, "fin_01_b", "Subcampeón", "Francia"),
    ] as never);

    renderBracket();

    expect(await screen.findByText("Brasil")).toBeInTheDocument();
    expect(screen.getByText("Francia")).toBeInTheDocument();
  });

  it("collapses a phase accordion when the header is clicked", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "fin_01_a", "Campeón", "Alemania"),
      makeSlot(2, "fin_01_b", "Subcampeón", "Brasil"),
    ] as never);

    renderBracket();

    // Phase is open by default (first phase, defaultOpen=true)
    expect(await screen.findByText("Alemania")).toBeInTheDocument();

    // Click the phase header to collapse
    fireEvent.click(screen.getByRole("button", { name: /Final/i }));

    // Team name no longer visible after collapse
    expect(screen.queryByText("Alemania")).toBeNull();
  });

  it("shows only phases that have at least one confirmed slot; others stay hidden", async () => {
    // r32 has confirmed teams in some slots; later phases have no teams at all → they are hidden.
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1.° Grupo A", "México"),
      makeSlot(2, "r32_01_b", "2.° Grupo B", "USA"), // M01 complete → r32 visible
      makeSlot(3, "r32_02_a", "1.° Grupo C", "Brasil"),
      makeSlot(4, "r32_02_b", "2.° Grupo D"), // M02 incomplete
      makeSlot(5, "r16_01_a", "Ganador R32 M01"),
      makeSlot(6, "r16_01_b", "Ganador R32 M02"), // no teams → r16 hidden
      makeSlot(7, "fin_01_a", "Ganador SF M01"),
      makeSlot(8, "fin_01_b", "Ganador SF M02"), // no teams → fin hidden
    ] as never);

    renderBracket();

    // Only r32 is visible (has a complete match)
    expect(await screen.findByText("Dieciseisavos")).toBeInTheDocument();
    expect(screen.getByText("México")).toBeInTheDocument();
    expect(screen.getByText("Estados Unidos")).toBeInTheDocument();

    // Incomplete slot in r32 shows placeholder description
    expect(screen.getByText("2.° Grupo D")).toBeInTheDocument();

    // Later phases with no complete match are not rendered
    expect(screen.queryByText("Octavos")).toBeNull();
    expect(screen.queryByText("Final")).toBeNull();
  });

  it("shows all six phases once each has at least one confirmed match", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1.° Grupo A", "México"),
      makeSlot(2, "r32_01_b", "2.° Grupo B", "USA"),
      makeSlot(3, "r16_01_a", "Ganador R32 M01", "México"),
      makeSlot(4, "r16_01_b", "Ganador R32 M02", "USA"),
      makeSlot(5, "qf_01_a", "Ganador R16 M01", "México"),
      makeSlot(6, "qf_01_b", "Ganador R16 M02", "USA"),
      makeSlot(7, "sf_01_a", "Ganador QF M01", "México"),
      makeSlot(8, "sf_01_b", "Ganador QF M02", "USA"),
      makeSlot(9, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(10, "tp_01_b", "Perdedor SF M02", "Alemania"),
      makeSlot(11, "fin_01_a", "Ganador SF M01", "México"),
      makeSlot(12, "fin_01_b", "Ganador SF M02", "USA"),
    ] as never);

    renderBracket();

    expect(await screen.findByText("Dieciseisavos")).toBeInTheDocument();
    expect(screen.getByText("Octavos")).toBeInTheDocument();
    expect(screen.getByText("Cuartos")).toBeInTheDocument();
    expect(screen.getByText("Semis")).toBeInTheDocument();
    expect(screen.getByText("3.er Lugar")).toBeInTheDocument();
    expect(screen.getByText("Final")).toBeInTheDocument();
  });

  it("renders multiple phases when each has a fully-confirmed match", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "sf_01_a", "SF1A", "Holanda"),
      makeSlot(2, "sf_01_b", "SF1B", "Alemania"),
      makeSlot(3, "fin_01_a", "Campeón", "España"),
      makeSlot(4, "fin_01_b", "Subcampeón", "Francia"),
    ] as never);

    renderBracket();

    expect(await screen.findByText("Semis")).toBeInTheDocument();
    expect(screen.getByText("Final")).toBeInTheDocument();
  });
});

// ── slot description i18n ──────────────────────────────────────────────────────

describe("slot descriptions – español", () => {
  beforeEach(() => {
    localStorage.setItem("quiniela-locale", "es");
    localStorage.setItem("quiniela-locale-source", "explicit");
  });

  it("muestra '1.° Grupo A' sin cambios para slot sin clasificado", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1.° Grupo A", "México"),
      makeSlot(2, "r32_01_b", "2.° Grupo B"), // unconfirmed → shows description
      makeSlot(3, "r32_02_a", "1.° Grupo C", "Brasil"),
      makeSlot(4, "r32_02_b", "2.° Grupo D", "Argentina"), // M02 complete → r32 visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("2.° Grupo B")).toBeInTheDocument();
  });

  it("muestra 'Mejor 3.° (p.2)' sin cambios", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_13_a", "Mejor 3.° (p.1)", "Uruguay"),
      makeSlot(2, "r32_13_b", "Mejor 3.° (p.2)"), // unconfirmed → shows description
      makeSlot(3, "r32_01_a", "1.° Grupo A", "Brasil"),
      makeSlot(4, "r32_01_b", "2.° Grupo B", "Argentina"), // M01 complete → r32 visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("Mejor 3.° (p.2)")).toBeInTheDocument();
  });

  it("muestra 'Ganador M02' (sin código de fase) en slot no confirmado", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Ganador R32 M01", "México"),
      makeSlot(2, "r16_01_b", "Ganador R32 M02"), // unconfirmed → shows description without phase code
      makeSlot(3, "r16_02_a", "Ganador R32 M03", "Brasil"),
      makeSlot(4, "r16_02_b", "Ganador R32 M04", "Argentina"), // M02 complete → r16 visible
    ] as never);
    renderBracket();
    // r16 is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Ganador M02")).toBeInTheDocument();
  });

  it("muestra 'Perdedor M02' (sin código de fase) en slot no confirmado", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(2, "tp_01_b", "Perdedor SF M02"), // unconfirmed → shows description without phase code
      makeSlot(3, "tp_02_a", "Perdedor SF M03", "Alemania"),
      makeSlot(4, "tp_02_b", "Perdedor SF M04", "Francia"), // M02 complete → tp visible
    ] as never);
    renderBracket();
    // tp is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Perdedor M02")).toBeInTheDocument();
  });
});

describe("slot descriptions – English", () => {
  beforeEach(() => {
    localStorage.setItem("quiniela-locale", "en");
    localStorage.setItem("quiniela-locale-source", "explicit");
  });

  afterEach(() => {
    localStorage.setItem("quiniela-locale", "es");
    localStorage.setItem("quiniela-locale-source", "explicit");
  });

  it("translates '1.° Grupo A' → '1st Group A' for unconfirmed slot", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1.° Grupo A", "México"),
      makeSlot(2, "r32_01_b", "2.° Grupo B"), // unconfirmed → shows translated desc
      makeSlot(3, "r32_02_a", "1.° Grupo C", "Brasil"),
      makeSlot(4, "r32_02_b", "2.° Grupo D", "Argentina"), // M02 complete → r32 visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("2nd Group B")).toBeInTheDocument();
    expect(screen.queryByText("2.° Grupo B")).toBeNull();
    // confirmed slot shows the team name translated to the active locale (en)
    expect(screen.getByText("Mexico")).toBeInTheDocument();
  });

  it("translates 'Mejor 3.° (p.2)' → 'Best 3rd (p.2)'", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_13_a", "Mejor 3.° (p.1)", "Uruguay"),
      makeSlot(2, "r32_13_b", "Mejor 3.° (p.2)"), // unconfirmed → shows translated desc
      makeSlot(3, "r32_01_a", "1.° Grupo A", "Brasil"),
      makeSlot(4, "r32_01_b", "2.° Grupo B", "Argentina"), // M01 complete → r32 visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("Best 3rd (p.2)")).toBeInTheDocument();
    expect(screen.queryByText("Mejor 3.° (p.2)")).toBeNull();
  });

  it("translates 'Ganador R32 M02' → 'Winner M02' (phase code stripped)", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Ganador R32 M01", "México"),
      makeSlot(2, "r16_01_b", "Ganador R32 M02"), // unconfirmed → shows translated desc without phase code
      makeSlot(3, "r16_02_a", "Ganador R32 M03", "Brasil"),
      makeSlot(4, "r16_02_b", "Ganador R32 M04", "Argentina"), // M02 complete → r16 visible
    ] as never);
    renderBracket();
    // r16 is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Winner M02")).toBeInTheDocument();
    expect(screen.queryByText("Winner R32 M02")).toBeNull();
    expect(screen.queryByText("Ganador R32 M02")).toBeNull();
  });

  it("translates 'Perdedor SF M02' → 'Loser M02' (phase code stripped)", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(2, "tp_01_b", "Perdedor SF M02"), // unconfirmed → shows translated desc without phase code
      makeSlot(3, "tp_02_a", "Perdedor SF M03", "Alemania"),
      makeSlot(4, "tp_02_b", "Perdedor SF M04", "Francia"), // M02 complete → tp visible
    ] as never);
    renderBracket();
    // tp is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Loser M02")).toBeInTheDocument();
    expect(screen.queryByText("Loser SF M02")).toBeNull();
    expect(screen.queryByText("Perdedor SF M02")).toBeNull();
  });
});
