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

const GEO_GT = new Response(JSON.stringify({ country: "GT" }), {
  status: 200,
  headers: { "Content-Type": "application/json" },
});
const GEO_MX = new Response(JSON.stringify({ country: "MX" }), {
  status: 200,
  headers: { "Content-Type": "application/json" },
});
const GEO_DE = new Response(JSON.stringify({ country: "DE" }), {
  status: 200,
  headers: { "Content-Type": "application/json" },
});
const GEO_US = new Response(JSON.stringify({ country: "US" }), {
  status: 200,
  headers: { "Content-Type": "application/json" },
});

describe("I18nProvider – stored preference", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  it("applies stored 'en' locale without fetching geo", async () => {
    const spy = vi.spyOn(globalThis, "fetch");
    globalThis.localStorage.setItem("quiniela-locale", "en");

    renderProvider();

    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("en"),
    );
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });

  it("applies stored 'es' locale without fetching geo", async () => {
    const spy = vi.spyOn(globalThis, "fetch");
    globalThis.localStorage.setItem("quiniela-locale", "es");

    renderProvider();

    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("es"),
    );
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});

describe("I18nProvider – geo detection (Hispanic countries → es)", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  it("sets 'es' for Guatemala (GT)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(GEO_GT.clone());
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("es"),
    );
    vi.restoreAllMocks();
  });

  it("sets 'es' for Mexico (MX)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(GEO_MX.clone());
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("es"),
    );
    vi.restoreAllMocks();
  });
});

describe("I18nProvider – geo detection (non-Hispanic countries → en)", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  it("sets 'en' for Germany (DE)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(GEO_DE.clone());
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("en"),
    );
    vi.restoreAllMocks();
  });

  it("sets 'en' for United States (US)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(GEO_US.clone());
    renderProvider();
    await waitFor(() =>
      expect(screen.getByTestId("locale")).toHaveTextContent("en"),
    );
    vi.restoreAllMocks();
  });
});

describe("I18nProvider – geo detection fallback", () => {
  beforeEach(() => globalThis.localStorage.clear());
  afterEach(cleanup);

  it("keeps default 'es' when fetch throws", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValueOnce(
      new Error("network error"),
    );
    renderProvider();
    // Give the failed async IIFE time to settle.
    await new Promise((r) => setTimeout(r, 30));
    expect(screen.getByTestId("locale")).toHaveTextContent("es");
    vi.restoreAllMocks();
  });

  it("persists detected locale to localStorage", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(GEO_DE.clone());
    renderProvider();
    await waitFor(() =>
      expect(globalThis.localStorage.getItem("quiniela-locale")).toBe("en"),
    );
    vi.restoreAllMocks();
  });
});
