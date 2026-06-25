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
  it("renders nothing when no slots have confirmed teams", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "fin_01_a", "Campeón"),
      makeSlot(2, "fin_01_b", "Subcampeón"),
    ] as never);

    const { container } = renderBracket();

    // Wait for data to load; section should not appear since no teams confirmed
    await vi.waitFor(() =>
      expect(container.querySelector("section")).toBeNull(),
    );
  });

  it("renders the bracket section when at least one slot has a confirmed team", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "fin_01_a", "Campeón", "Argentina"),
      makeSlot(2, "fin_01_b", "Subcampeón"),
    ] as never);

    renderBracket();

    expect(await screen.findByText("Llave del Torneo")).toBeInTheDocument();
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
      makeSlot(2, "fin_01_b", "Subcampeón"),
    ] as never);

    renderBracket();

    // Phase is open by default (first phase, defaultOpen=true)
    expect(await screen.findByText("Alemania")).toBeInTheDocument();

    // Click the phase header to collapse
    fireEvent.click(screen.getByRole("button", { name: /Final/i }));

    // Team name no longer visible after collapse
    expect(screen.queryByText("Alemania")).toBeNull();
  });

  it("shows all phases with slots once any slot is confirmed, even if later phases have no teams yet", async () => {
    // Simulate the real seeded state: all 6 phases have slots, but only r32 has
    // confirmed teams so far (first group just completed).
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1.° Grupo A", "México"),
      makeSlot(2, "r32_01_b", "2.° Grupo B", "USA"),
      makeSlot(3, "r16_01_a", "Ganador R32 M01"),
      makeSlot(4, "r16_01_b", "Ganador R32 M02"),
      makeSlot(5, "qf_01_a", "Ganador R16 M01"),
      makeSlot(6, "qf_01_b", "Ganador R16 M02"),
      makeSlot(7, "sf_01_a", "Ganador QF M01"),
      makeSlot(8, "sf_01_b", "Ganador QF M02"),
      makeSlot(9, "tp_01_a", "Perdedor SF M01"),
      makeSlot(10, "tp_01_b", "Perdedor SF M02"),
      makeSlot(11, "fin_01_a", "Ganador SF M01"),
      makeSlot(12, "fin_01_b", "Ganador SF M02"),
    ] as never);

    renderBracket();

    // All six phase accordions must be visible once the knockout stage has begun
    expect(await screen.findByText("Dieciseisavos")).toBeInTheDocument();
    expect(screen.getByText("Octavos")).toBeInTheDocument();
    expect(screen.getByText("Cuartos")).toBeInTheDocument();
    expect(screen.getByText("Semis")).toBeInTheDocument();
    expect(screen.getByText("3.er Lugar")).toBeInTheDocument();
    expect(screen.getByText("Final")).toBeInTheDocument();

    // The confirmed teams appear in the open r32 accordion (first in PHASE_ORDER)
    expect(screen.getByText("México")).toBeInTheDocument();

    // Opening a later phase reveals its placeholder descriptions
    fireEvent.click(screen.getByRole("button", { name: /Octavos/i }));
    expect(screen.getByText("Ganador R32 M01")).toBeInTheDocument();
  });

  it("renders multiple phases when slots from different phases have confirmed teams", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "sf_01_a", "SF1A", "Holanda"),
      makeSlot(2, "sf_01_b", "SF1B"),
      makeSlot(3, "fin_01_a", "Campeón", "España"),
      makeSlot(4, "fin_01_b", "Subcampeón"),
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
      makeSlot(2, "r32_01_b", "2.° Grupo B"),
    ] as never);
    renderBracket();
    expect(await screen.findByText("2.° Grupo B")).toBeInTheDocument();
  });

  it("muestra 'Mejor 3.° (p.2)' sin cambios", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_13_a", "Mejor 3.° (p.1)", "Uruguay"),
      makeSlot(2, "r32_13_b", "Mejor 3.° (p.2)"),
    ] as never);
    renderBracket();
    expect(await screen.findByText("Mejor 3.° (p.2)")).toBeInTheDocument();
  });

  it("muestra 'Ganador R32 M02' sin cambios en slot no confirmado", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Ganador R32 M01", "México"),
      makeSlot(2, "r16_01_b", "Ganador R32 M02"),
    ] as never);
    renderBracket();
    // r16 is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Ganador R32 M02")).toBeInTheDocument();
  });

  it("muestra 'Perdedor SF M02' sin cambios en slot no confirmado", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(2, "tp_01_b", "Perdedor SF M02"),
    ] as never);
    renderBracket();
    // tp is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Perdedor SF M02")).toBeInTheDocument();
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
      makeSlot(2, "r32_01_b", "2.° Grupo B"),
    ] as never);
    renderBracket();
    expect(await screen.findByText("2nd Group B")).toBeInTheDocument();
    expect(screen.queryByText("2.° Grupo B")).toBeNull();
    // confirmed slot still shows the team name, not the description
    expect(screen.getByText("México")).toBeInTheDocument();
  });

  it("translates 'Mejor 3.° (p.2)' → 'Best 3rd (p.2)'", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_13_a", "Mejor 3.° (p.1)", "Uruguay"),
      makeSlot(2, "r32_13_b", "Mejor 3.° (p.2)"),
    ] as never);
    renderBracket();
    expect(await screen.findByText("Best 3rd (p.2)")).toBeInTheDocument();
    expect(screen.queryByText("Mejor 3.° (p.2)")).toBeNull();
  });

  it("translates 'Ganador R32 M02' → 'Winner R32 M02'", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Ganador R32 M01", "México"),
      makeSlot(2, "r16_01_b", "Ganador R32 M02"),
    ] as never);
    renderBracket();
    // r16 is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Winner R32 M02")).toBeInTheDocument();
    expect(screen.queryByText("Ganador R32 M02")).toBeNull();
  });

  it("translates 'Perdedor SF M02' → 'Loser SF M02'", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(2, "tp_01_b", "Perdedor SF M02"),
    ] as never);
    renderBracket();
    // tp is the only visible phase → defaultOpen=true, content visible without click
    expect(await screen.findByText("Loser SF M02")).toBeInTheDocument();
    expect(screen.queryByText("Perdedor SF M02")).toBeNull();
  });
});
