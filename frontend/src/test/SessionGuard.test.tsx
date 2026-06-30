import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, act } from "@testing-library/react";

// signOut spy shared across all tests in this file.
const signOutMock = vi.fn();
// Mutable session state — tests override via mockUseSession.
let mockSession: { createdAt: Date | null } | null = null;

vi.mock("@clerk/nextjs", () => ({
  useClerk: () => ({ signOut: signOutMock }),
  useSession: () => ({ session: mockSession }),
}));

// Default fetch mock: returns a network-error-like rejection so the component
// falls back to the NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS env var. Individual
// tests override this to exercise the live-fetch path.
const fetchMock = vi.fn<() => Promise<Response>>(() =>
  Promise.reject(new Error("fetch disabled in tests")),
);
vi.stubGlobal("fetch", fetchMock);

// Helper: set NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS before importing the module
// under test. Because Vitest hoists vi.mock() calls, we control the env via
// vi.stubEnv and re-import after each mutation.
function mockMaxAge(seconds: number | undefined) {
  if (seconds !== undefined) {
    vi.stubEnv("NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS", String(seconds));
  } else {
    vi.stubEnv("NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS", "");
  }
}

import { SessionGuard } from "@/components/shared/SessionGuard";
import { dispatchSessionExpired } from "@/lib/session-bus";

describe("SessionGuard – reactive path (session-bus event)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSession = null;
    mockMaxAge(undefined);
    fetchMock.mockRejectedValue(new Error("fetch disabled in tests"));
  });

  it("calls signOut when dispatchSessionExpired is called", () => {
    render(<SessionGuard />);
    act(() => {
      dispatchSessionExpired();
    });
    expect(signOutMock).toHaveBeenCalledOnce();
    expect(signOutMock).toHaveBeenCalledWith({ redirectUrl: "/sign-in" });
  });

  it("removes the event listener on unmount", () => {
    const { unmount } = render(<SessionGuard />);
    unmount();
    act(() => {
      dispatchSessionExpired();
    });
    expect(signOutMock).not.toHaveBeenCalled();
  });
});

describe("SessionGuard – proactive timer (session.createdAt + maxAge)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    fetchMock.mockRejectedValue(new Error("fetch disabled in tests"));
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllEnvs();
  });

  it("calls signOut immediately when session is already expired", () => {
    mockMaxAge(3600); // 1 hour
    // Session started 2 hours ago → already expired.
    mockSession = { createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000) };

    render(<SessionGuard />);

    expect(signOutMock).toHaveBeenCalledOnce();
    expect(signOutMock).toHaveBeenCalledWith({ redirectUrl: "/sign-in" });
  });

  it("schedules signOut to fire after remaining time", async () => {
    mockMaxAge(3600); // 1 hour
    // Session started 30 minutes ago → 30 minutes remaining.
    mockSession = { createdAt: new Date(Date.now() - 30 * 60 * 1000) };

    render(<SessionGuard />);

    // Not yet expired.
    expect(signOutMock).not.toHaveBeenCalled();

    // Advance past the remaining 30 minutes.
    act(() => {
      vi.advanceTimersByTime(31 * 60 * 1000);
    });

    expect(signOutMock).toHaveBeenCalledOnce();
    expect(signOutMock).toHaveBeenCalledWith({ redirectUrl: "/sign-in" });
  });

  it("clears the timer on unmount (no signOut after unmount)", () => {
    mockMaxAge(3600);
    mockSession = { createdAt: new Date(Date.now() - 30 * 60 * 1000) };

    const { unmount } = render(<SessionGuard />);
    unmount();

    act(() => {
      vi.advanceTimersByTime(40 * 60 * 1000);
    });

    expect(signOutMock).not.toHaveBeenCalled();
  });

  it("does nothing when NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS is not set", () => {
    mockMaxAge(undefined); // env var absent → SESSION_MAX_AGE_MS = null
    mockSession = { createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000) };

    render(<SessionGuard />);

    act(() => {
      vi.advanceTimersByTime(24 * 60 * 60 * 1000); // 24 hours
    });

    expect(signOutMock).not.toHaveBeenCalled();
  });

  it("does nothing when session is null (not yet loaded)", () => {
    mockMaxAge(3600);
    mockSession = null;

    render(<SessionGuard />);

    act(() => {
      vi.advanceTimersByTime(2 * 60 * 60 * 1000);
    });

    expect(signOutMock).not.toHaveBeenCalled();
  });

  it("does nothing when session.createdAt is null", () => {
    mockMaxAge(3600);
    mockSession = { createdAt: null };

    render(<SessionGuard />);

    act(() => {
      vi.advanceTimersByTime(2 * 60 * 60 * 1000);
    });

    expect(signOutMock).not.toHaveBeenCalled();
  });

  // VULN-012: invalid Date guard
  it("does nothing when session.createdAt is an invalid Date (NaN timestamp)", () => {
    mockMaxAge(3600);
    mockSession = { createdAt: new Date(NaN) };

    render(<SessionGuard />);

    act(() => {
      vi.advanceTimersByTime(2 * 60 * 60 * 1000);
    });

    expect(signOutMock).not.toHaveBeenCalled();
  });

  it("emits console.warn in development when session.createdAt is an invalid Date", () => {
    vi.stubEnv("NODE_ENV", "development");
    mockMaxAge(3600);
    mockSession = { createdAt: new Date(NaN) };
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

    render(<SessionGuard />);

    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining("session.createdAt is an invalid Date"),
    );
    warnSpy.mockRestore();
    vi.unstubAllEnvs();
  });

  it("emits a console.warn in development mode when maxAge env var is absent", () => {
    vi.stubEnv("NODE_ENV", "development");
    mockMaxAge(undefined);
    mockSession = { createdAt: new Date() };
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

    render(<SessionGuard />);

    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining("NEXT_PUBLIC_SESSION_MAX_AGE_SECONDS"),
    );
    warnSpy.mockRestore();
    vi.unstubAllEnvs();
  });

  it("does not emit console.warn in test/production mode when maxAge env var is absent", () => {
    // NODE_ENV is "test" by default in Vitest — no warning should appear.
    mockMaxAge(undefined);
    mockSession = { createdAt: new Date() };
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

    render(<SessionGuard />);

    expect(warnSpy).not.toHaveBeenCalled();
    warnSpy.mockRestore();
  });
});

describe("SessionGuard – visibilitychange / focus recheck (VULN-008)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    fetchMock.mockRejectedValue(new Error("fetch disabled in tests"));
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllEnvs();
    // Restore visibilityState to default for subsequent tests.
    Object.defineProperty(document, "visibilityState", {
      value: "visible",
      configurable: true,
    });
  });

  it("signs out immediately on visibilitychange when session expired during OS sleep", () => {
    mockMaxAge(3600); // 1 h
    // Session started 30 min ago; 30 min remaining at render time.
    mockSession = { createdAt: new Date(Date.now() - 30 * 60 * 1000) };

    render(<SessionGuard />);
    expect(signOutMock).not.toHaveBeenCalled();

    // Simulate 40 min passing without the timer firing (OS sleep / throttled tab).
    vi.setSystemTime(Date.now() + 40 * 60 * 1000);

    // Tab becomes visible again — recheck recalculates remaining (< 0) → signs out.
    Object.defineProperty(document, "visibilityState", {
      value: "visible",
      configurable: true,
    });
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });

    expect(signOutMock).toHaveBeenCalledOnce();
    expect(signOutMock).toHaveBeenCalledWith({ redirectUrl: "/sign-in" });
  });

  it("does not cancel the running timer when the tab becomes hidden", () => {
    mockMaxAge(3600);
    // Session started 30 min ago; 30 min remaining.
    mockSession = { createdAt: new Date(Date.now() - 30 * 60 * 1000) };

    render(<SessionGuard />);

    // Tab goes to background — recheck must be a no-op (visibilityState hidden).
    Object.defineProperty(document, "visibilityState", {
      value: "hidden",
      configurable: true,
    });
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });

    // Session not yet expired — no sign-out.
    expect(signOutMock).not.toHaveBeenCalled();

    // The original timer fires after the remaining 30+ min.
    act(() => {
      vi.advanceTimersByTime(31 * 60 * 1000);
    });
    expect(signOutMock).toHaveBeenCalledOnce();
  });

  it("signs out immediately on window focus when session expired while unfocused", () => {
    mockMaxAge(3600);
    mockSession = { createdAt: new Date(Date.now() - 30 * 60 * 1000) };

    render(<SessionGuard />);
    expect(signOutMock).not.toHaveBeenCalled();

    // 40 min pass without the timer firing.
    vi.setSystemTime(Date.now() + 40 * 60 * 1000);

    act(() => {
      window.dispatchEvent(new Event("focus"));
    });

    expect(signOutMock).toHaveBeenCalledOnce();
    expect(signOutMock).toHaveBeenCalledWith({ redirectUrl: "/sign-in" });
  });

  it("removes visibilitychange and focus listeners on unmount", () => {
    mockMaxAge(3600);
    mockSession = { createdAt: new Date(Date.now() - 30 * 60 * 1000) };

    const { unmount } = render(<SessionGuard />);
    unmount();

    // Advance clock past expiry and fire visibilitychange — no sign-out after unmount.
    vi.setSystemTime(Date.now() + 40 * 60 * 1000);
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });

    expect(signOutMock).not.toHaveBeenCalled();
  });
});

describe("SessionGuard – live session config fetch (DT-009)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllEnvs();
  });

  it("signs out when live max age from /api/session-config is shorter than env var", async () => {
    // Env var: 1h. Live fetch: 10 min. Session started 30 min ago.
    // With env-var alone (1h): not expired. With live 10 min: expired.
    // Verifies that the fetched value replaces the compile-time env var.
    mockMaxAge(3600); // 1h from env
    mockSession = { createdAt: new Date(Date.now() - 30 * 60 * 1000) }; // 30 min ago

    fetchMock.mockResolvedValueOnce({
      json: () => Promise.resolve({ session_max_age_seconds: 600 }), // 10 min
      ok: true,
    } as Response);

    render(<SessionGuard />);

    // Before fetch resolves: env-var 1h → not yet expired.
    expect(signOutMock).not.toHaveBeenCalled();

    // Flush fetch promise chain so setMaxAgeMs(600_000) fires and React re-renders.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // After re-render with 10-min max age: 30-min-old session is expired → signOut.
    expect(signOutMock).toHaveBeenCalledOnce();
  });

  it("falls back to env var when /api/session-config fetch fails", () => {
    mockMaxAge(3600);
    mockSession = { createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000) }; // 2h ago

    fetchMock.mockRejectedValueOnce(new Error("network error"));

    render(<SessionGuard />);

    // Env-var-based 1h limit → 2h session is already expired → fires immediately.
    expect(signOutMock).toHaveBeenCalledOnce();
  });
});
