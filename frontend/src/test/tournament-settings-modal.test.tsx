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
  useMutation: vi.fn(),
  useQueryClient: vi
    .fn()
    .mockReturnValue({ setQueryData: vi.fn(), invalidateQueries: vi.fn() }),
}));

vi.mock("next/link", () => ({
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useMutation } from "@tanstack/react-query";
import { TournamentSettingsModal } from "@/components/groups/TournamentSettingsModal";
import type { GroupDetailResponse } from "@/lib/api-types";

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeGroup(
  overrides: Partial<GroupDetailResponse> = {},
): GroupDetailResponse {
  return {
    id: 1,
    name: "Test Pool",
    owner_user_id: 1,
    invite_code: "abc",
    invite_code_expires_at: null,
    status: "active",
    entry_fee: 0,
    currency: "GTQ",
    is_premium: false,
    mode_general: false,
    mode_round: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function noopMutation() {
  vi.mocked(useMutation).mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  } as never);
}

function renderModal(
  props: Partial<Parameters<typeof TournamentSettingsModal>[0]> = {},
) {
  const defaults = {
    group: makeGroup(),
    memberCount: 3,
    onClose: vi.fn(),
  };
  return render(
    <I18nProvider>
      <TournamentSettingsModal {...defaults} {...props} />
    </I18nProvider>,
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("TournamentSettingsModal – free group, not enough members", () => {
  beforeEach(noopMutation);

  it("shows not-ready warning banner", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 3 });
    expect(screen.getByText(/5/)).toBeInTheDocument();
  });

  it("save button is disabled when modes unchanged", () => {
    renderModal();
    const save = screen.getByRole("button", { name: /guard|save/i });
    expect(save).toBeDisabled();
  });

  it("backdrop click calls onClose", () => {
    const onClose = vi.fn();
    const { container } = renderModal({ onClose });
    const backdrop = container.querySelector('button[aria-label="Close"]');
    fireEvent.click(backdrop!);
    expect(onClose).toHaveBeenCalled();
  });

  it("X button calls onClose", () => {
    const onClose = vi.fn();
    renderModal({ onClose });
    const buttons = screen.getAllByRole("button");
    const xBtn = buttons.find((b) => b.querySelector("svg"));
    fireEvent.click(xBtn!);
    expect(onClose).toHaveBeenCalled();
  });
});

describe("TournamentSettingsModal – free group, enough members (canUpgrade)", () => {
  beforeEach(noopMutation);

  it("shows premium-ready banner", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    expect(screen.getByText(/premium|activ/i)).toBeInTheDocument();
  });

  it("save button becomes enabled after toggling a mode", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    const checkboxes = screen.getAllByRole("checkbox");
    fireEvent.click(checkboxes[0]);
    const save = screen.getByRole("button", { name: /guard|save/i });
    expect(save).not.toBeDisabled();
  });

  it("toggling mode_general checkbox changes its checked state", () => {
    renderModal({
      group: makeGroup({ is_premium: false, mode_general: false }),
      memberCount: 6,
    });
    const [generalCb] = screen.getAllByRole("checkbox");
    expect(generalCb).not.toBeChecked();
    fireEvent.click(generalCb);
    expect(generalCb).toBeChecked();
  });
});

describe("TournamentSettingsModal – premium group", () => {
  beforeEach(noopMutation);

  it("does not show any eligibility banner", () => {
    renderModal({
      group: makeGroup({
        is_premium: true,
        mode_general: true,
        mode_round: false,
      }),
    });
    expect(screen.queryByText(/premium/i)).toBeNull();
  });

  it("mode_general checkbox is disabled (already active)", () => {
    renderModal({ group: makeGroup({ is_premium: true, mode_general: true }) });
    const [generalCb] = screen.getAllByRole("checkbox");
    expect(generalCb).toBeDisabled();
  });

  it("mode_round checkbox is enabled when not yet active", () => {
    renderModal({
      group: makeGroup({
        is_premium: true,
        mode_general: true,
        mode_round: false,
      }),
    });
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes[1]).not.toBeDisabled();
  });
});

describe("TournamentSettingsModal – mutation states", () => {
  it("shows spinner when mutation is pending", () => {
    vi.mocked(useMutation).mockReturnValue({
      mutate: vi.fn(),
      isPending: true,
      isError: false,
      error: null,
    } as never);
    const { container } = renderModal({
      group: makeGroup({ is_premium: true }),
      memberCount: 6,
    });
    expect(container.querySelector(".animate-spin")).toBeInTheDocument();
  });

  it("shows generic settings error for unknown error messages", () => {
    vi.mocked(useMutation).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: { message: "Server error" },
    } as never);
    renderModal({ group: makeGroup({ is_premium: true }), memberCount: 6 });
    expect(
      screen.getByText("No se pudo guardar la configuración."),
    ).toBeInTheDocument();
  });

  it("shows generic settings error when error has no message", () => {
    vi.mocked(useMutation).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: {},
    } as never);
    renderModal({ group: makeGroup({ is_premium: true }), memberCount: 6 });
    expect(
      screen.getByText("No se pudo guardar la configuración."),
    ).toBeInTheDocument();
  });

  it("shows min-members error when backend reports requires/members", () => {
    vi.mocked(useMutation).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: { message: "premium mode requires at least 5 active members" },
    } as never);
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    expect(
      screen.getByText(/5 miembros activos/),
    ).toBeInTheDocument();
  });

  it("shows balance error when backend reports insufficient balance", () => {
    vi.mocked(useMutation).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: { message: "insufficient available balance" },
    } as never);
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    expect(screen.getByText(/saldo suficiente/)).toBeInTheDocument();
  });
});
