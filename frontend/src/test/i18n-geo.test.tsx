import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import React from "react";
import { I18nProvider, useI18n } from "@/lib/i18n";

// Minimal consumer that exposes the current locale for assertions.
function LocaleProbe() {
  const { locale } = useI18n();
  return <span data-testid="locale">{locale}</span>;
}

function renderProvider() {
  return render(
    <I18nProvider>
      <LocaleProbe />
    </I18nProvider>,
  );
}

function geoRes(country: string) {
  return new Response(JSON.stringify({ country }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// ── Explicit stored preference ────────────────────────────────────────────────

describe("I18nProvider – stored preference", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  it("applies explicit 'en' locale without fetching geo", async () => {
    const spy = vi.spyOn(globalThis, "fetch");
    globalThis.localStorage.setItem("quiniela-locale", "en");
    globalThis.localStorage.setItem("quiniela-locale-source", "explicit");

    renderProvider();

    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("en"),
    );
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });

  it("applies explicit 'es' locale without fetching geo", async () => {
    const spy = vi.spyOn(globalThis, "fetch");
    globalThis.localStorage.setItem("quiniela-locale", "es");
    globalThis.localStorage.setItem("quiniela-locale-source", "explicit");

    renderProvider();

    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("es"),
    );
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });

  it("re-runs geo detection when locale was auto-detected (no explicit source)", async () => {
    // Locale stored but source is NOT "explicit" → should re-fetch geo and update
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(geoRes("GT"));
    globalThis.localStorage.setItem("quiniela-locale", "en"); // stale auto-detected value
    // quiniela-locale-source is NOT set

    renderProvider();

    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("es"),
    );
    vi.restoreAllMocks();
  });
});

// ── Spanish-speaking countries → "es" ────────────────────────────────────────
// All 21 ISO 3166-1 alpha-2 codes where Spanish is an official language must
// produce locale "es" so the UI is displayed in Spanish by default.

describe("I18nProvider – geo detection (all Spanish-speaking countries → es)", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  const hispanicCountries: Array<[string, string]> = [
    ["AR", "Argentina"],
    ["BO", "Bolivia"],
    ["CL", "Chile"],
    ["CO", "Colombia"],
    ["CR", "Costa Rica"],
    ["CU", "Cuba"],
    ["DO", "Dominican Republic"],
    ["EC", "Ecuador"],
    ["ES", "Spain"],
    ["GQ", "Equatorial Guinea"],
    ["GT", "Guatemala"],
    ["HN", "Honduras"],
    ["MX", "Mexico"],
    ["NI", "Nicaragua"],
    ["PA", "Panama"],
    ["PE", "Peru"],
    ["PR", "Puerto Rico"],
    ["PY", "Paraguay"],
    ["SV", "El Salvador"],
    ["UY", "Uruguay"],
    ["VE", "Venezuela"],
  ];

  it.each(hispanicCountries)("sets 'es' for %s (%s)", async (code) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(geoRes(code));
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("es"),
    );
    vi.restoreAllMocks();
    cleanup();
  });

  it("handles lowercase country code (e.g. 'gt' from a geo provider variant)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(geoRes("gt"));
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("es"),
    );
    vi.restoreAllMocks();
  });
});

// ── Non-Spanish-speaking countries → "en" ────────────────────────────────────
// Users from any country NOT in the Spanish-speaking set must see English.
// Includes important non-obvious cases: Brazil (Portuguese, not Spanish),
// Philippines (Filipino+English) and Portugal (Portuguese).

describe("I18nProvider – geo detection (non-Spanish-speaking countries → en)", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  const nonHispanicCountries: Array<[string, string]> = [
    ["US", "United States"],
    ["CA", "Canada"],
    ["GB", "United Kingdom"],
    ["DE", "Germany"],
    ["FR", "France"],
    ["IT", "Italy"],
    ["JP", "Japan"],
    ["CN", "China"],
    ["KR", "South Korea"],
    ["IN", "India"],
    ["AU", "Australia"],
    ["BR", "Brazil (Portuguese, not Spanish)"],
    ["PT", "Portugal (Portuguese, not Spanish)"],
    ["PH", "Philippines"],
    ["NG", "Nigeria"],
    ["ZA", "South Africa"],
    ["RU", "Russia"],
    ["NL", "Netherlands"],
    ["SE", "Sweden"],
    ["PL", "Poland"],
  ];

  it.each(nonHispanicCountries)("sets 'en' for %s (%s)", async (code) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(geoRes(code));
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("en"),
    );
    vi.restoreAllMocks();
    cleanup();
  });
});

// ── Fallback and persistence behaviour ───────────────────────────────────────

describe("I18nProvider – geo detection fallback", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  it("keeps default 'es' when fetch throws (safe fallback for GT-primary app)", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValueOnce(
      new Error("network error"),
    );
    renderProvider();
    await new Promise((r) => setTimeout(r, 30));
    expect(screen.getByTestId("locale")).toHaveTextContent("es");
    vi.restoreAllMocks();
  });

  it("keeps default 'es' when geo returns no country field", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({}), { status: 200 }),
    );
    renderProvider();
    await new Promise((r) => setTimeout(r, 30));
    expect(screen.getByTestId("locale")).toHaveTextContent("es");
    vi.restoreAllMocks();
  });

  it("does NOT persist auto-detected locale as explicit preference", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(geoRes("DE"));
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("en"),
    );
    // locale-source must remain absent so next session re-runs geo detection
    expect(
      globalThis.localStorage.getItem("quiniela-locale-source"),
    ).toBeNull();
    vi.restoreAllMocks();
  });

  it("initial render shows 'es' before geo resolves (React default state)", () => {
    // fetch never resolves in this test — validates the synchronous default
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => undefined));
    renderProvider();
    expect(screen.getByTestId("locale")).toHaveTextContent("es");
    vi.restoreAllMocks();
  });
});
