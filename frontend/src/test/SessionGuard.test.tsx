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

describe("SessionGuard – reactive path (wcq:session-expired event)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSession = null;
    mockMaxAge(undefined);
  });

  it("calls signOut when wcq:session-expired is dispatched", () => {
    render(<SessionGuard />);
    act(() => {
      globalThis.dispatchEvent(new Event("wcq:session-expired"));
    });
    expect(signOutMock).toHaveBeenCalledOnce();
    expect(signOutMock).toHaveBeenCalledWith({ redirectUrl: "/sign-in" });
  });

  it("removes the event listener on unmount", () => {
    const { unmount } = render(<SessionGuard />);
    unmount();
    act(() => {
      globalThis.dispatchEvent(new Event("wcq:session-expired"));
    });
    expect(signOutMock).not.toHaveBeenCalled();
  });
});

describe("SessionGuard – proactive timer (session.createdAt + maxAge)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
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
