import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { I18nProvider } from "@/lib/i18n";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi
    .fn()
    .mockReturnValue({ getToken: vi.fn().mockResolvedValue("tok") }),
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: vi.fn(),
}));

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useQuery } from "@tanstack/react-query";
import { LivePredictionsCarousel } from "@/components/groups/LivePredictionsCarousel";
import type { LivePredictionsResponse } from "@/lib/api-types";

// ── Fixtures ──────────────────────────────────────────────────────────────────

const liveMatch = {
  id: 5,
  home_team: "Brazil",
  away_team: "Argentina",
  home_score: null,
  away_score: null,
  status: "in_progress" as const,
  phase: "group_stage",
  group_label: null,
  stadium_id: null,
  stadium: null,
  win_method: null,
  round_number: null,
  kickoff_at: "2026-06-14T20:00:00Z",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-14T20:00:00Z",
};

const userRow = {
  user_id: 10,
  display_name: "Alice Doe",
  predictions: [
    { match_id: 5, home_score: 2, away_score: 1, has_prediction: true },
  ],
};

function mockQueryWith(
  data: LivePredictionsResponse | undefined,
  isLoading = false,
) {
  vi.mocked(useQuery).mockReturnValue({ data, isLoading } as never);
}

function renderCarousel() {
  return render(
    <I18nProvider>
      <LivePredictionsCarousel groupId={1} />
    </I18nProvider>,
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("LivePredictionsCarousel", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders nothing while loading", () => {
    mockQueryWith(undefined, true);
    const { container } = renderCarousel();
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when there are no live matches", () => {
    mockQueryWith({ live_matches: [], user_predictions: [] });
    const { container } = renderCarousel();
    expect(container.firstChild).toBeNull();
  });

  it("renders the section title when there are live matches", () => {
    mockQueryWith({ live_matches: [liveMatch], user_predictions: [] });
    renderCarousel();
    expect(screen.getByText("Predicciones en Vivo")).toBeTruthy();
  });

  it("shows empty message when live matches exist but no user predictions", () => {
    mockQueryWith({ live_matches: [liveMatch], user_predictions: [] });
    renderCarousel();
    expect(
      screen.getByText("No hay partidos en vivo en este momento."),
    ).toBeTruthy();
  });

  it("renders a card for each user with a live match", () => {
    mockQueryWith({ live_matches: [liveMatch], user_predictions: [userRow] });
    renderCarousel();
    expect(screen.getByText("Alice Doe")).toBeTruthy();
  });

  it("shows prediction count on the card header", () => {
    mockQueryWith({ live_matches: [liveMatch], user_predictions: [userRow] });
    renderCarousel();
    expect(screen.getByText("1 / 1 predicciones")).toBeTruthy();
  });

  it("shows noPrediction label when user has no submitted score", () => {
    const noScoreRow = {
      ...userRow,
      predictions: [
        { match_id: 5, home_score: 0, away_score: 0, has_prediction: false },
      ],
    };
    mockQueryWith({
      live_matches: [liveMatch],
      user_predictions: [noScoreRow],
    });
    renderCarousel();
    expect(screen.getByText("Sin predicción")).toBeTruthy();
  });

  it("accordion opens and shows match scores on click", () => {
    mockQueryWith({ live_matches: [liveMatch], user_predictions: [userRow] });
    renderCarousel();

    const toggle = screen.getByRole("button", {
      name: /Ver predicciones de Alice Doe/i,
    });
    fireEvent.click(toggle);

    expect(screen.getByText("Brazil")).toBeTruthy();
    expect(screen.getByText("Argentina")).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy();
  });

  it("accordion closes on second click", () => {
    mockQueryWith({ live_matches: [liveMatch], user_predictions: [userRow] });
    renderCarousel();

    const toggle = screen.getByRole("button", {
      name: /Ver predicciones de Alice Doe/i,
    });
    fireEvent.click(toggle);
    fireEvent.click(toggle);

    expect(screen.queryByText("Brazil")).toBeNull();
  });

  it("renders noPrediction inside open accordion when has_prediction is false", () => {
    const noScoreRow = {
      ...userRow,
      predictions: [
        { match_id: 5, home_score: 0, away_score: 0, has_prediction: false },
      ],
    };
    mockQueryWith({
      live_matches: [liveMatch],
      user_predictions: [noScoreRow],
    });
    renderCarousel();

    const toggle = screen.getByRole("button", {
      name: /Ver predicciones de Alice Doe/i,
    });
    fireEvent.click(toggle);

    const noPredTexts = screen.getAllByText("Sin predicción");
    expect(noPredTexts.length).toBeGreaterThanOrEqual(1);
  });

  it("shows multiple user cards", () => {
    const bobRow = {
      user_id: 20,
      display_name: "Bob Smith",
      predictions: [
        { match_id: 5, home_score: 0, away_score: 0, has_prediction: false },
      ],
    };
    mockQueryWith({
      live_matches: [liveMatch],
      user_predictions: [userRow, bobRow],
    });
    renderCarousel();

    expect(screen.getByText("Alice Doe")).toBeTruthy();
    expect(screen.getByText("Bob Smith")).toBeTruthy();
  });
});
