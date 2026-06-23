import { render, waitFor } from "@testing-library/react";
import { vi, describe, it, expect, beforeEach } from "vitest";
import { TimezoneSync } from "@/components/shared/TimezoneSync";
import { I18nProvider } from "@/lib/i18n";
import { api } from "@/lib/api";

// Declare mocks via vi.hoisted so they are available inside vi.mock factories,
// which are hoisted to the top of the module before other imports resolve.
const { mockGetToken } = vi.hoisted(() => ({
  mockGetToken: vi.fn().mockResolvedValue("test-token"),
}));

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi
    .fn()
    .mockReturnValue({ getToken: mockGetToken, isSignedIn: true }),
}));

vi.mock("@/lib/api", () => ({
  api: { updateMe: vi.fn().mockResolvedValue({}) },
}));

// Stub Intl so I18nProvider detects "America/Guatemala" instead of jsdom's "UTC".
vi.spyOn(Intl, "DateTimeFormat").mockReturnValue({
  resolvedOptions: () => ({ timeZone: "America/Guatemala" }),
} as unknown as Intl.DateTimeFormat);

function renderSync() {
  return render(
    <I18nProvider>
      <TimezoneSync />
    </I18nProvider>,
  );
}

describe("TimezoneSync", () => {
  beforeEach(() => {
    vi.mocked(api.updateMe).mockClear();
    mockGetToken.mockClear();
  });

  it("calls api.updateMe with the browser timezone when signed in", async () => {
    renderSync();

    await waitFor(() => {
      expect(api.updateMe).toHaveBeenCalledWith("test-token", {
        timezone: "America/Guatemala",
      });
    });
  });

  it("only fires once even if the component re-renders", async () => {
    const { rerender } = renderSync();
    await waitFor(() => expect(api.updateMe).toHaveBeenCalledTimes(1));

    rerender(
      <I18nProvider>
        <TimezoneSync />
      </I18nProvider>,
    );

    expect(api.updateMe).toHaveBeenCalledTimes(1);
  });
});
