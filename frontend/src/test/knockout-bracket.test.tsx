import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { beforeAll, describe, expect, it, vi } from "vitest";
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
  return { id, label, description, team, confirmed_at: team ? new Date().toISOString() : null };
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
