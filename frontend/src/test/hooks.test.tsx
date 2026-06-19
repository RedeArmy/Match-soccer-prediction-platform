import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// ── Hoist mocks ───────────────────────────────────────────────────────────────

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    getExchangeRate: vi.fn(),
    getBalance: vi.fn(),
    getKYCStatus: vi.fn(),
  },
}));

vi.mock("@/lib/sse", () => ({
  createSSEManager: vi.fn(),
}));

vi.mock("@/lib/i18n", () => ({
  useI18n: vi.fn(),
}));

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useAuth } from "@clerk/nextjs";
import { api } from "@/lib/api";
import { createSSEManager } from "@/lib/sse";
import { useI18n } from "@/lib/i18n";
import { useExchangeRate } from "@/hooks/useExchangeRate";
import { useBalance } from "@/hooks/useBalance";
import { useKYCStatus } from "@/hooks/useKYCStatus";
import { useSSE } from "@/hooks/useSSE";
import { useCurrency } from "@/hooks/useCurrency";

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

// ── useExchangeRate ───────────────────────────────────────────────────────────

describe("useExchangeRate", () => {
  beforeEach(() => vi.mocked(api.getExchangeRate).mockReset());

  it("returns data when getExchangeRate resolves", async () => {
    const rate = {
      buy_rate: "7.72",
      sell_rate: "7.80",
      effective_at: "2026-01-01T00:00:00Z",
      stale: false,
    };
    vi.mocked(api.getExchangeRate).mockResolvedValueOnce(rate);

    const { result } = renderHook(() => useExchangeRate(), {
      wrapper: makeWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(rate);
  });

  it("has no data when getExchangeRate rejects", async () => {
    vi.mocked(api.getExchangeRate).mockRejectedValueOnce(
      new Error("network error"),
    );

    const { result } = renderHook(() => useExchangeRate(), {
      wrapper: makeWrapper(),
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });
});

// ── useBalance ────────────────────────────────────────────────────────────────

describe("useBalance", () => {
  beforeEach(() => {
    vi.mocked(api.getBalance).mockReset();
    vi.mocked(useAuth).mockReturnValue({
      getToken: vi.fn().mockResolvedValue("tok_abc"),
    } as never);
  });

  it("returns balance data when getBalance resolves", async () => {
    const balance = {
      balance_cents: 50000,
      available_cents: 50000,
      reserved_cents: 0,
      pending_cents: 0,
      currency: "GTQ",
      balance_currency: "GTQ",
    };
    vi.mocked(api.getBalance).mockResolvedValueOnce(balance);

    const { result } = renderHook(() => useBalance(), {
      wrapper: makeWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(balance);
    expect(vi.mocked(api.getBalance)).toHaveBeenCalledWith("tok_abc");
  });
});

// ── useKYCStatus ──────────────────────────────────────────────────────────────

describe("useKYCStatus", () => {
  beforeEach(() => {
    vi.mocked(api.getKYCStatus).mockReset();
    vi.mocked(useAuth).mockReturnValue({
      getToken: vi.fn().mockResolvedValue("tok_abc"),
    } as never);
  });

  it("returns KYC data when getKYCStatus resolves", async () => {
    const kyc = {
      id: 1,
      user_id: "u1",
      status: "approved",
      tier: 1,
      created_at: "",
      updated_at: "",
    };
    vi.mocked(api.getKYCStatus).mockResolvedValueOnce(kyc as never);

    const { result } = renderHook(() => useKYCStatus(), {
      wrapper: makeWrapper(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(kyc);
    expect(vi.mocked(api.getKYCStatus)).toHaveBeenCalledWith("tok_abc");
  });
});

// ── useSSE ────────────────────────────────────────────────────────────────────

describe("useSSE", () => {
  beforeEach(() => {
    vi.mocked(createSSEManager).mockReset();
  });

  it("calls createSSEManager on mount and returns connecting status", () => {
    const disconnect = vi.fn();
    vi.mocked(createSSEManager).mockReturnValue({ disconnect });

    const { result } = renderHook(() => useSSE(), { wrapper: makeWrapper() });
    expect(vi.mocked(createSSEManager)).toHaveBeenCalled();
    expect(result.current.status).toBe("connecting");
  });

  it("updates status when onConnectionChange callback is called", async () => {
    const disconnect = vi.fn();
    vi.mocked(createSSEManager).mockReturnValue({ disconnect });

    const { result } = renderHook(() => useSSE(), { wrapper: makeWrapper() });
    const { onConnectionChange } = vi.mocked(createSSEManager).mock.calls[0][0];

    act(() => {
      onConnectionChange?.("connected");
    });

    expect(result.current.status).toBe("connected");
  });

  it("calls disconnect on unmount", () => {
    const disconnect = vi.fn();
    vi.mocked(createSSEManager).mockReturnValue({ disconnect });

    const { unmount } = renderHook(() => useSSE(), { wrapper: makeWrapper() });
    unmount();
    expect(disconnect).toHaveBeenCalledOnce();
  });

  it("sets lastNotification when onNotification callback is called", async () => {
    const disconnect = vi.fn();
    vi.mocked(createSSEManager).mockReturnValue({ disconnect });

    const { result } = renderHook(() => useSSE(), { wrapper: makeWrapper() });
    const { onNotification } = vi.mocked(createSSEManager).mock.calls[0][0];

    const payload = {
      id: 42,
      event_type: "score.updated",
      title: "Test notification",
      body: "msg",
      action_url: null,
      read: false,
      created_at: "2026-01-01T00:00:00Z",
    };

    act(() => {
      onNotification?.(payload);
    });

    expect(result.current.lastNotification).toEqual(payload);
  });

  it("invalidates kyc queries when event_type starts with kyc.", async () => {
    const disconnect = vi.fn();
    vi.mocked(createSSEManager).mockReturnValue({ disconnect });

    const { result } = renderHook(() => useSSE(), { wrapper: makeWrapper() });
    const { onNotification } = vi.mocked(createSSEManager).mock.calls[0][0];

    const payload = {
      id: 43,
      event_type: "kyc.approved",
      title: "KYC approved",
      body: "msg",
      action_url: null,
      read: false,
      created_at: "2026-01-01T00:00:00Z",
    };

    act(() => {
      onNotification?.(payload);
    });

    expect(result.current.lastNotification).toEqual(payload);
  });
});

// ── useCurrency ───────────────────────────────────────────────────────────────

describe("useCurrency", () => {
  beforeEach(() => {
    vi.mocked(api.getExchangeRate).mockReset();
    vi.mocked(api.getExchangeRate).mockResolvedValue({
      buy_rate: "7.72",
      sell_rate: "7.80",
      effective_at: "2026-01-01T00:00:00Z",
      stale: false,
    });
  });

  it("formats in GTQ when locale is es", () => {
    vi.mocked(useI18n).mockReturnValue({ locale: "es" } as never);

    const { result } = renderHook(() => useCurrency(), {
      wrapper: makeWrapper(),
    });
    expect(result.current.isUSD).toBe(false);
    expect(result.current.fmt(10000)).toContain("Q");
  });

  it("formats in USD when locale is en", () => {
    vi.mocked(useI18n).mockReturnValue({ locale: "en" } as never);

    const { result } = renderHook(() => useCurrency(), {
      wrapper: makeWrapper(),
    });
    expect(result.current.isUSD).toBe(true);
    expect(result.current.fmt(780)).toContain("$");
  });
});
