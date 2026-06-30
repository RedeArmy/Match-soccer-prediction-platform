import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Providers } from "@/app/providers";
import { I18nProvider, useI18n } from "@/lib/i18n";

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi.fn().mockReturnValue({
    getToken: vi.fn().mockResolvedValue(null),
    isSignedIn: false,
  }),
  useClerk: vi.fn().mockReturnValue({
    signOut: vi.fn(),
  }),
  useSession: vi.fn().mockReturnValue({ session: null }),
}));

describe("Providers", () => {
  it("renders children inside app providers", () => {
    render(
      <Providers>
        <span>Provider child</span>
      </Providers>,
    );

    expect(screen.getByText("Provider child")).toBeInTheDocument();
  });
});

function I18nProbe() {
  const { teamName, phaseName, formatKickoff, timeZone } = useI18n();

  return (
    <div>
      <span>{teamName("Canada")}</span>
      <span>{teamName("USA")}</span>
      <span>{phaseName("group_stage")}</span>
      <span>{formatKickoff("2026-06-11T18:00:00Z")}</span>
      <span>{timeZone}</span>
    </div>
  );
}

describe("I18nProvider localisation helpers", () => {
  it("localises team names, match phases, and kickoff date-times", () => {
    render(
      <I18nProvider>
        <I18nProbe />
      </I18nProvider>,
    );

    expect(screen.getByText("Canadá")).toBeInTheDocument();
    expect(screen.getByText("Estados Unidos")).toBeInTheDocument();
    expect(screen.getByText("Fase de Grupos")).toBeInTheDocument();
    expect(screen.getByText(/2026/)).toBeInTheDocument();
  });
});

function AccountTypeProbe() {
  const { accountTypeName } = useI18n();
  return (
    <div>
      <span data-testid="known">{accountTypeName("ahorros gtq")}</span>
      <span data-testid="null">{accountTypeName(null)}</span>
      <span data-testid="unknown">{accountTypeName("custom type")}</span>
    </div>
  );
}

describe("I18nProvider accountTypeName helper", () => {
  it("translates known account types, returns dash for null, and falls back for unknown", () => {
    render(
      <I18nProvider>
        <AccountTypeProbe />
      </I18nProvider>,
    );

    expect(screen.getByTestId("known").textContent).toBe("Ahorros GTQ");
    expect(screen.getByTestId("null").textContent).toBe("—");
    expect(screen.getByTestId("unknown").textContent).toBe("custom type");
  });
});
