import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// ── Hoist mocks ───────────────────────────────────────────────────────────────

vi.mock("@/lib/api", () => ({
  api: {
    getSlots: vi.fn(),
  },
}));

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { api } from "@/lib/api";
import { useSlots } from "@/hooks/useSlots";

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
  return { qc, Wrapper };
}

function makeSlot(
  id: number,
  label: string,
  autoSource: string | null,
  team: string | null,
) {
  return {
    id,
    label,
    description: label,
    team,
    auto_source: autoSource,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
}

// ── useSlots ──────────────────────────────────────────────────────────────────

describe("useSlots", () => {
  beforeEach(() => vi.mocked(api.getSlots).mockReset());

  it("starts with empty slots and isLoading true", () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([]);
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useSlots(), { wrapper: Wrapper });
    expect(result.current.slots).toEqual([]);
    expect(result.current.isLoading).toBe(true);
  });

  it("returns slots and builds teamByAutoSource map when data resolves", async () => {
    const slots = [
      makeSlot(1, "r32_01_a", "1A", "Mexico"),
      makeSlot(2, "r32_01_b", "2B", "Brazil"),
      makeSlot(3, "r32_02_a", null, null),
    ];
    vi.mocked(api.getSlots).mockResolvedValueOnce(slots as never);
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useSlots(), { wrapper: Wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.slots).toHaveLength(3);
    expect(result.current.teamByAutoSource.get("1A")).toBe("Mexico");
    expect(result.current.teamByAutoSource.get("2B")).toBe("Brazil");
    expect(result.current.teamByAutoSource.size).toBe(2);
  });

  it("invalidates matches query when a new confirmed slot appears", async () => {
    const slot = makeSlot(1, "r32_01_a", "1A", "Mexico");
    vi.mocked(api.getSlots).mockResolvedValueOnce([slot] as never);
    const { qc, Wrapper } = makeWrapper();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    renderHook(() => useSlots(), { wrapper: Wrapper });

    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["matches"] }),
      ),
    );
  });

  it("does not invalidate matches when slot has no auto_source", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", null, "Mexico"),
    ] as never);
    const { qc, Wrapper } = makeWrapper();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    renderHook(() => useSlots(), { wrapper: Wrapper });

    await waitFor(() => expect(vi.mocked(api.getSlots)).toHaveBeenCalled());
    expect(invalidateSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["matches"] }),
    );
  });

  it("does not invalidate matches when slot has no team", async () => {
    vi.mocked(api.getSlots).mockResolvedValueOnce([
      makeSlot(1, "r32_01_a", "1A", null),
    ] as never);
    const { qc, Wrapper } = makeWrapper();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    renderHook(() => useSlots(), { wrapper: Wrapper });

    await waitFor(() => expect(vi.mocked(api.getSlots)).toHaveBeenCalled());
    expect(invalidateSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["matches"] }),
    );
  });
});
