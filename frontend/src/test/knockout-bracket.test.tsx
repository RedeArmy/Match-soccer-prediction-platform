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

  it("renders nothing when slots exist but no match has both teams confirmed", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Clasificado R16 M01", "México"),
      makeSlot(2, "r16_01_b", "Clasificado R16 M02"), // second slot still pending
    ] as never);

    const { container } = renderBracket();

    await vi.waitFor(() =>
      expect(container.querySelector("section")).toBeNull(),
    );
  });

  it("renders the bracket section when a match has both slots confirmed", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "fin_01_a", "Campeón", "Argentina"),
      makeSlot(2, "fin_01_b", "Subcampeón", "Francia"),
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

  it("shows only phases that have at least one fully-confirmed match; others stay hidden", async () => {
    // r16 has one complete match (M01 both confirmed) plus unconfirmed slots.
    // Later phases have slots defined but no complete match → they are hidden.
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Clasificado R16 M01", "México"),
      makeSlot(2, "r16_01_b", "Clasificado R16 M02", "USA"), // M01 complete → r16 visible
      makeSlot(3, "r16_02_a", "Clasificado R16 M03", "Brasil"),
      makeSlot(4, "r16_02_b", "Clasificado R16 M04"), // M02 incomplete
      makeSlot(5, "qf_01_a", "Ganador R16 M01"),
      makeSlot(6, "qf_01_b", "Ganador R16 M02"), // no teams → qf hidden
      makeSlot(7, "fin_01_a", "Ganador SF M01"),
      makeSlot(8, "fin_01_b", "Ganador SF M02"), // no teams → fin hidden
    ] as never);

    renderBracket();

    // Only r16 is visible (has a complete match)
    expect(await screen.findByText("Octavos")).toBeInTheDocument();
    expect(screen.getByText("México")).toBeInTheDocument();
    expect(screen.getByText("USA")).toBeInTheDocument();

    // Incomplete slot in r16 shows placeholder description
    expect(screen.getByText("Clasificado R16 M04")).toBeInTheDocument();

    // Later phases with no complete match are not rendered
    expect(screen.queryByText("Cuartos")).toBeNull();
    expect(screen.queryByText("Final")).toBeNull();
  });

  it("shows all five phases once each has at least one confirmed match", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Clasificado R16 M01", "México"),
      makeSlot(2, "r16_01_b", "Clasificado R16 M02", "USA"),
      makeSlot(3, "qf_01_a", "Ganador R16 M01", "México"),
      makeSlot(4, "qf_01_b", "Ganador R16 M02", "USA"),
      makeSlot(5, "sf_01_a", "Ganador QF M01", "México"),
      makeSlot(6, "sf_01_b", "Ganador QF M02", "USA"),
      makeSlot(7, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(8, "tp_01_b", "Perdedor SF M02", "Alemania"),
      makeSlot(9, "fin_01_a", "Ganador SF M01", "México"),
      makeSlot(10, "fin_01_b", "Ganador SF M02", "USA"),
    ] as never);

    renderBracket();

    expect(await screen.findByText("Octavos")).toBeInTheDocument();
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

  it("muestra 'Clasificado R16 M02' sin cambios para slot sin clasificado", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Clasificado R16 M01", "México"),
      makeSlot(2, "r16_01_b", "Clasificado R16 M02"), // unconfirmed → shows description
      makeSlot(3, "r16_02_a", "Clasificado R16 M03", "Brasil"),
      makeSlot(4, "r16_02_b", "Clasificado R16 M04", "Argentina"), // M02 complete → r16 visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("Clasificado R16 M02")).toBeInTheDocument();
  });

  it("muestra 'Ganador R16 M02' sin cambios en slot no confirmado de QF", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "qf_01_a", "Ganador R16 M01", "México"),
      makeSlot(2, "qf_01_b", "Ganador R16 M02"), // unconfirmed → shows description
      makeSlot(3, "qf_02_a", "Ganador R16 M03", "Brasil"),
      makeSlot(4, "qf_02_b", "Ganador R16 M04", "Argentina"), // M02 complete → qf visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("Ganador R16 M02")).toBeInTheDocument();
  });

  it("muestra 'Perdedor SF M02' sin cambios en slot no confirmado", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(2, "tp_01_b", "Perdedor SF M02"), // unconfirmed → shows description
      makeSlot(3, "tp_02_a", "Perdedor SF M03", "Alemania"),
      makeSlot(4, "tp_02_b", "Perdedor SF M04", "Francia"), // M02 complete → tp visible
    ] as never);
    renderBracket();
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

  it("shows unconfirmed r16 slot description as-is (no translation needed)", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r16_01_a", "Clasificado R16 M01", "México"),
      makeSlot(2, "r16_01_b", "Clasificado R16 M02"), // unconfirmed → shows description
      makeSlot(3, "r16_02_a", "Clasificado R16 M03", "Brasil"),
      makeSlot(4, "r16_02_b", "Clasificado R16 M04", "Argentina"), // M02 complete → r16 visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("Clasificado R16 M02")).toBeInTheDocument();
    // confirmed slot still shows the team name, not the description
    expect(screen.getByText("México")).toBeInTheDocument();
  });

  it("translates 'Ganador R16 M02' → 'Winner R16 M02'", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "qf_01_a", "Ganador R16 M01", "México"),
      makeSlot(2, "qf_01_b", "Ganador R16 M02"), // unconfirmed → shows translated desc
      makeSlot(3, "qf_02_a", "Ganador R16 M03", "Brasil"),
      makeSlot(4, "qf_02_b", "Ganador R16 M04", "Argentina"), // M02 complete → qf visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("Winner R16 M02")).toBeInTheDocument();
    expect(screen.queryByText("Ganador R16 M02")).toBeNull();
  });

  it("translates 'Perdedor SF M02' → 'Loser SF M02'", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "tp_01_a", "Perdedor SF M01", "Holanda"),
      makeSlot(2, "tp_01_b", "Perdedor SF M02"), // unconfirmed → shows translated desc
      makeSlot(3, "tp_02_a", "Perdedor SF M03", "Alemania"),
      makeSlot(4, "tp_02_b", "Perdedor SF M04", "Francia"), // M02 complete → tp visible
    ] as never);
    renderBracket();
    expect(await screen.findByText("Loser SF M02")).toBeInTheDocument();
    expect(screen.queryByText("Perdedor SF M02")).toBeNull();
  });
});
