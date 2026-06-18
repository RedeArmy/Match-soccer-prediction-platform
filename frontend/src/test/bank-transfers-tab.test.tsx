import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { I18nProvider } from "@/lib/i18n";
import { BankTransfersTab } from "@/components/admin/BankTransfersTab";
import type { BankTransferResponse } from "@/lib/api-types";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi.fn().mockReturnValue({ getToken: vi.fn().mockResolvedValue("tok") }),
}));

vi.mock("@/lib/api", () => ({
  api: {
    adminListBankTransfers: vi.fn(),
    adminApproveBankTransfer: vi.fn(),
    adminRejectBankTransfer: vi.fn(),
  },
}));

import { api } from "@/lib/api";

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

function renderTab() {
  return render(
    <QueryClientProvider client={makeClient()}>
      <I18nProvider>
        <BankTransfersTab />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

function transfer(overrides: Partial<BankTransferResponse> = {}): BankTransferResponse {
  return {
    id: 1,
    user_id: 42,
    amount_cents: 10000,
    currency: "GTQ",
    content_type: "image/jpeg",
    file_size: 12345,
    status: "pending",
    reviewed_by: null,
    notes: null,
    approved_amount_cents: null,
    approved_at: null,
    created_at: "2026-06-01T12:00:00Z",
    updated_at: "2026-06-01T12:00:00Z",
    ...overrides,
  };
}

// ── Loading ───────────────────────────────────────────────────────────────────

describe("BankTransfersTab – loading state", () => {
  it("shows loading skeleton while fetching", () => {
    vi.mocked(api.adminListBankTransfers).mockReturnValue(new Promise(() => {}));
    renderTab();
    // AdminContentState renders <LoadingState /> which uses animate-pulse skeleton
    expect(document.querySelector(".animate-pulse")).toBeInTheDocument();
  });
});

// ── Error ─────────────────────────────────────────────────────────────────────

describe("BankTransfersTab – error state", () => {
  it("renders error message when fetch fails", async () => {
    vi.mocked(api.adminListBankTransfers).mockRejectedValueOnce(new Error("db down"));
    renderTab();
    await waitFor(() =>
      expect(screen.getByText(/Error al cargar/i)).toBeInTheDocument(),
    );
  });
});

// ── Empty ─────────────────────────────────────────────────────────────────────

describe("BankTransfersTab – empty pending", () => {
  beforeEach(() => vi.mocked(api.adminListBankTransfers).mockResolvedValue([]));

  it("shows empty state for pending tab", async () => {
    renderTab();
    await waitFor(() =>
      expect(
        screen.getByText(/No hay comprobantes pendientes/i),
      ).toBeInTheDocument(),
    );
  });

  it("shows total count of 0", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByText(/0 comprobantes en total/i)).toBeInTheDocument(),
    );
  });
});

// ── With data ─────────────────────────────────────────────────────────────────

describe("BankTransfersTab – with pending transfers", () => {
  beforeEach(() =>
    vi.mocked(api.adminListBankTransfers).mockResolvedValue([
      transfer({ id: 1, user_id: 10, status: "pending", amount_cents: 5000 }),
      transfer({ id: 2, user_id: 11, status: "approved", amount_cents: 10000, approved_amount_cents: 10000 }),
    ]),
  );

  it("renders the transfers table", async () => {
    renderTab();
    await waitFor(() => expect(screen.getByText("#1")).toBeInTheDocument());
  });

  it("shows pending transfer with Aprobar and Rechazar buttons", async () => {
    renderTab();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Aprobar/i })).toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: /Rechazar/i })).toBeInTheDocument();
  });

  it("switches to all tab and shows both transfers", async () => {
    renderTab();
    await waitFor(() => screen.getByText("#1"));
    fireEvent.click(screen.getByRole("button", { name: /Todos/i }));
    await waitFor(() => expect(screen.getByText("#2")).toBeInTheDocument());
  });

  it("switches to approved tab and shows only approved transfer", async () => {
    renderTab();
    await waitFor(() => screen.getByText("#1"));
    fireEvent.click(screen.getByRole("button", { name: /Aprobados/i }));
    await waitFor(() => expect(screen.getByText("#2")).toBeInTheDocument());
    expect(screen.queryByText("#1")).not.toBeInTheDocument();
  });
});

// ── Approve modal ─────────────────────────────────────────────────────────────

describe("BankTransfersTab – approve modal", () => {
  beforeEach(() =>
    vi.mocked(api.adminListBankTransfers).mockResolvedValue([
      transfer({ id: 5, status: "pending", amount_cents: 20000 }),
    ]),
  );

  it("opens approve modal when Aprobar is clicked", async () => {
    renderTab();
    await waitFor(() => screen.getByRole("button", { name: /Aprobar/i }));
    fireEvent.click(screen.getByRole("button", { name: /Aprobar/i }));
    await waitFor(() =>
      expect(screen.getByText(/Aprobar Transferencia #5/i)).toBeInTheDocument(),
    );
  });

  it("closes approve modal on Cancelar", async () => {
    renderTab();
    await waitFor(() => screen.getByRole("button", { name: /Aprobar/i }));
    fireEvent.click(screen.getByRole("button", { name: /Aprobar/i }));
    await waitFor(() => screen.getByText(/Aprobar Transferencia #5/i));
    fireEvent.click(screen.getByRole("button", { name: /Cancelar/i }));
    await waitFor(() =>
      expect(
        screen.queryByText(/Aprobar Transferencia #5/i),
      ).not.toBeInTheDocument(),
    );
  });

  it("calls adminApproveBankTransfer on submit", async () => {
    vi.mocked(api.adminApproveBankTransfer).mockResolvedValueOnce({} as never);

    renderTab();
    await waitFor(() => screen.getByRole("button", { name: /Aprobar/i }));
    fireEvent.click(screen.getByRole("button", { name: /Aprobar/i }));
    await waitFor(() => screen.getByText(/Aprobar Transferencia #5/i));
    fireEvent.click(screen.getByRole("button", { name: /Aprobar y Acreditar/i }));
    await waitFor(() =>
      expect(vi.mocked(api.adminApproveBankTransfer)).toHaveBeenCalledWith(
        "tok",
        5,
        expect.objectContaining({}),
      ),
    );
  });

  it("shows validation error for invalid override amount", async () => {
    renderTab();
    await waitFor(() => screen.getByRole("button", { name: /Aprobar/i }));
    fireEvent.click(screen.getByRole("button", { name: /Aprobar/i }));
    await waitFor(() => screen.getByText(/Aprobar Transferencia #5/i));
    fireEvent.change(screen.getByLabelText(/Monto a acreditar/i), {
      target: { value: "-5" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Aprobar y Acreditar/i }));
    await waitFor(() =>
      expect(
        screen.getByText(/El monto debe ser positivo/i),
      ).toBeInTheDocument(),
    );
  });
});

// ── Reject modal ──────────────────────────────────────────────────────────────

describe("BankTransfersTab – reject modal", () => {
  beforeEach(() =>
    vi.mocked(api.adminListBankTransfers).mockResolvedValue([
      transfer({ id: 7, status: "pending" }),
    ]),
  );

  it("opens reject modal when Rechazar is clicked", async () => {
    renderTab();
    await waitFor(() => screen.getByRole("button", { name: /Rechazar/i }));
    fireEvent.click(screen.getByRole("button", { name: /Rechazar/i }));
    await waitFor(() =>
      expect(
        screen.getByText(/Rechazar Transferencia #7/i),
      ).toBeInTheDocument(),
    );
  });

  it("calls adminRejectBankTransfer with notes on submit", async () => {
    vi.mocked(api.adminRejectBankTransfer).mockResolvedValueOnce({} as never);

    renderTab();
    await waitFor(() => screen.getByRole("button", { name: /Rechazar/i }));
    fireEvent.click(screen.getByRole("button", { name: /Rechazar/i }));
    await waitFor(() => screen.getByText(/Rechazar Transferencia #7/i));

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "Comprobante ilegible" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Rechazar Transferencia/i }));

    await waitFor(() =>
      expect(vi.mocked(api.adminRejectBankTransfer)).toHaveBeenCalledWith(
        "tok",
        7,
        "Comprobante ilegible",
      ),
    );
  });

  it("requires notes before reject submit – button disabled", async () => {
    renderTab();
    await waitFor(() => screen.getByRole("button", { name: /Rechazar/i }));
    fireEvent.click(screen.getByRole("button", { name: /Rechazar/i }));
    await waitFor(() => screen.getByText(/Rechazar Transferencia #7/i));
    const submitBtn = screen.getByRole("button", { name: /Rechazar Transferencia/i });
    expect(submitBtn).toBeDisabled();
  });
});

// ── Approved with override ────────────────────────────────────────────────────

describe("BankTransfersTab – approved transfer with override amount", () => {
  beforeEach(() =>
    vi.mocked(api.adminListBankTransfers).mockResolvedValue([
      transfer({
        id: 9,
        status: "approved",
        amount_cents: 5000,
        approved_amount_cents: 7500,
      }),
    ]),
  );

  it("shows the approved transfer after switching to All tab", async () => {
    renderTab();
    // On the pending tab the approved transfer is hidden; switch to "all"
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /Todos/i }),
      ).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole("button", { name: /Todos/i }));
    await waitFor(() => expect(screen.getByText("#9")).toBeInTheDocument());
  });
});

// ── Refresh button ────────────────────────────────────────────────────────────

describe("BankTransfersTab – refresh button", () => {
  it("is enabled after load and does not throw on click", async () => {
    vi.mocked(api.adminListBankTransfers).mockResolvedValue([]);
    renderTab();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /Actualizar/i }),
      ).not.toBeDisabled(),
    );
    // Clicking should not throw
    fireEvent.click(screen.getByRole("button", { name: /Actualizar/i }));
    expect(
      screen.getByRole("button", { name: /Actualizar/i }),
    ).toBeInTheDocument();
  });
});
