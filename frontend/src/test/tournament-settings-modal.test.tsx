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

vi.mock("@/lib/api", () => ({
  api: {
    updateRequireApproval: vi.fn().mockResolvedValue({}),
    setTournamentMode: vi.fn().mockResolvedValue({}),
  },
}));

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { TournamentSettingsModal } from "@/components/groups/TournamentSettingsModal";
import type { GroupDetailResponse } from "@/lib/api-types";
import { api } from "@/lib/api";

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
    require_approval: true,
    score_from_zero: false,
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

function goToModeTab() {
  fireEvent.click(screen.getByRole("button", { name: /modo de torneo/i }));
}

function goToSettingsTab() {
  fireEvent.click(screen.getByRole("button", { name: /^ajustes$/i }));
}

// ── Tests: tab navigation ─────────────────────────────────────────────────────

describe("TournamentSettingsModal – tab navigation", () => {
  beforeEach(noopMutation);

  it("opens on Ajustes tab by default", () => {
    renderModal();
    expect(
      screen.getByText("Requerir aprobación"),
    ).toBeInTheDocument();
  });

  it("switches to Modo de torneo tab on click and shows mode checkboxes", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    goToModeTab();
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes.length).toBe(2);
  });

  it("backdrop click calls onClose from any tab", () => {
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

// ── Tests: Ajustes tab (require_approval) ─────────────────────────────────────

describe("TournamentSettingsModal – Ajustes tab", () => {
  beforeEach(noopMutation);

  it("save button is disabled when require_approval is unchanged", () => {
    renderModal({ group: makeGroup({ require_approval: true }) });
    const save = screen.getByRole("button", { name: /guard|save/i });
    expect(save).toBeDisabled();
  });

  it("save button enables when require_approval toggled", () => {
    renderModal({ group: makeGroup({ require_approval: true }) });
    const [cb] = screen.getAllByRole("checkbox");
    fireEvent.click(cb);
    const save = screen.getByRole("button", { name: /guard|save/i });
    expect(save).not.toBeDisabled();
  });

  it("require_approval checkbox starts checked when group has it enabled", () => {
    renderModal({ group: makeGroup({ require_approval: true }) });
    const [cb] = screen.getAllByRole("checkbox");
    expect(cb).toBeChecked();
  });

  it("require_approval checkbox starts unchecked when group has it disabled", () => {
    renderModal({ group: makeGroup({ require_approval: false }) });
    const [cb] = screen.getAllByRole("checkbox");
    expect(cb).not.toBeChecked();
  });

  it("toggling require_approval updates checkbox state", () => {
    renderModal({ group: makeGroup({ require_approval: true }) });
    const [cb] = screen.getAllByRole("checkbox");
    expect(cb).toBeChecked();
    fireEvent.click(cb);
    expect(cb).not.toBeChecked();
  });

  it("shows generic error on approval mutation failure", () => {
    vi.mocked(useMutation).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: { message: "server error" },
    } as never);
    renderModal();
    expect(
      screen.getByText("No se pudo guardar la configuración."),
    ).toBeInTheDocument();
  });
});

// ── Tests: Modo de torneo tab ─────────────────────────────────────────────────

describe("TournamentSettingsModal – free group, not enough members", () => {
  beforeEach(noopMutation);

  it("shows not-ready warning banner in mode tab", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 3 });
    goToModeTab();
    expect(screen.getByText(/5/)).toBeInTheDocument();
  });

  it("save button in mode tab is disabled when modes unchanged", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 3 });
    goToModeTab();
    const save = screen.getByRole("button", { name: /guard|save/i });
    expect(save).toBeDisabled();
  });
});

describe("TournamentSettingsModal – free group, enough members (canUpgrade)", () => {
  beforeEach(noopMutation);

  it("shows premium-ready banner in mode tab", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    goToModeTab();
    expect(screen.getByText(/premium|activ/i)).toBeInTheDocument();
  });

  it("save button becomes enabled after toggling a mode", () => {
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    goToModeTab();
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
    goToModeTab();
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
    goToModeTab();
    expect(screen.queryByText(/premium/i)).toBeNull();
  });

  it("mode_general checkbox is disabled (already active)", () => {
    renderModal({ group: makeGroup({ is_premium: true, mode_general: true }) });
    goToModeTab();
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
    goToModeTab();
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes[1]).not.toBeDisabled();
  });
});

describe("TournamentSettingsModal – mutation states (mode tab)", () => {
  it("shows spinner in mode tab when mutation is pending", () => {
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
    goToModeTab();
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
    goToModeTab();
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
    goToModeTab();
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
    goToModeTab();
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
    goToModeTab();
    expect(screen.getByText(/saldo suficiente/)).toBeInTheDocument();
  });
});

describe("TournamentSettingsModal – mutation states (settings tab)", () => {
  it("shows spinner in settings tab when mutation is pending", () => {
    vi.mocked(useMutation).mockReturnValue({
      mutate: vi.fn(),
      isPending: true,
      isError: false,
      error: null,
    } as never);
    const { container } = renderModal();
    goToSettingsTab();
    expect(container.querySelector(".animate-spin")).toBeInTheDocument();
  });
});

// ── Tests: mutation callbacks and onClick handlers ────────────────────────────
// These tests capture the mutation options passed to useMutation and invoke the
// callback bodies directly, covering the async mutationFn and onSuccess code
// paths that the mock otherwise prevents from running.

describe("TournamentSettingsModal – mutation callbacks", () => {
  it("settingsMutation mutationFn calls api.updateRequireApproval when requireApproval is dirty", async () => {
    // settingsMutation is the FIRST useMutation call (odd indices: 1, 3, 5, …).
    // We toggle the checkbox after the initial render so the closure captured on
    // the subsequent re-render has isApprovalDirty=true.
    let settingsOptions: Record<string, unknown> | null = null;
    let totalCount = 0;
    vi.mocked(useMutation).mockImplementation((opts: unknown) => {
      totalCount++;
      if (totalCount % 2 === 1) settingsOptions = opts as Record<string, unknown>;
      return { mutate: vi.fn(), isPending: false, isError: false, error: null } as never;
    });
    renderModal({ group: makeGroup({ require_approval: true }) });
    // Toggle require-approval to make isApprovalDirty=true (true→false)
    const [cb] = screen.getAllByRole("checkbox");
    fireEvent.click(cb);
    // settingsOptions now holds the re-render closure with isApprovalDirty=true
    expect(settingsOptions).not.toBeNull();
    await (settingsOptions!.mutationFn as () => Promise<unknown>)();
    expect(vi.mocked(api.updateRequireApproval)).toHaveBeenCalledWith("tok", 1, false);
  });

  it("settingsMutation onSuccess calls queryClient.setQueryData and onClose", () => {
    const setQueryData = vi.fn();
    const onClose = vi.fn();
    vi.mocked(useQueryClient).mockReturnValue({ setQueryData, invalidateQueries: vi.fn() } as never);
    let settingsOptions: Record<string, unknown> | null = null;
    let callCount = 0;
    vi.mocked(useMutation).mockImplementation((opts: unknown) => {
      callCount++;
      if (callCount === 1) settingsOptions = opts as Record<string, unknown>;
      return { mutate: vi.fn(), isPending: false, isError: false, error: null } as never;
    });
    renderModal({ group: makeGroup(), onClose });
    const updated = makeGroup({ require_approval: false });
    (settingsOptions!.onSuccess as (u: GroupDetailResponse) => void)(updated);
    expect(setQueryData).toHaveBeenCalledWith(["group", 1], updated);
    expect(onClose).toHaveBeenCalled();
  });

  it("modeMutation mutationFn calls api.setTournamentMode", async () => {
    // The modeMutation is the SECOND useMutation call in the component
    let modeOptions: Record<string, unknown> | null = null;
    let callCount = 0;
    vi.mocked(useMutation).mockImplementation((opts: unknown) => {
      callCount++;
      if (callCount === 2) modeOptions = opts as Record<string, unknown>;
      return { mutate: vi.fn(), isPending: false, isError: false, error: null } as never;
    });
    renderModal({ group: makeGroup({ mode_general: false, mode_round: false }) });
    expect(modeOptions).not.toBeNull();
    await (modeOptions!.mutationFn as () => Promise<unknown>)();
    expect(vi.mocked(api.setTournamentMode)).toHaveBeenCalledWith(
      "tok",
      1,
      { mode_general: false, mode_round: false },
    );
  });

  it("modeMutation onSuccess calls queryClient.setQueryData and onClose", () => {
    const setQueryData = vi.fn();
    const onClose = vi.fn();
    vi.mocked(useQueryClient).mockReturnValue({ setQueryData, invalidateQueries: vi.fn() } as never);
    let modeOptions: Record<string, unknown> | null = null;
    let callCount = 0;
    vi.mocked(useMutation).mockImplementation((opts: unknown) => {
      callCount++;
      if (callCount === 2) modeOptions = opts as Record<string, unknown>;
      return { mutate: vi.fn(), isPending: false, isError: false, error: null } as never;
    });
    renderModal({ group: makeGroup(), onClose });
    const updated = makeGroup({ mode_general: true });
    (modeOptions!.onSuccess as (u: GroupDetailResponse) => void)(updated);
    expect(setQueryData).toHaveBeenCalledWith(["group", 1], updated);
    expect(onClose).toHaveBeenCalled();
  });

  it("approval save button onClick fires mutate when dirty", () => {
    const mutate = vi.fn();
    vi.mocked(useMutation).mockReturnValue({
      mutate,
      isPending: false,
      isError: false,
      error: null,
    } as never);
    renderModal({ group: makeGroup({ require_approval: true }) });
    // Toggle to make it dirty
    const [cb] = screen.getAllByRole("checkbox");
    fireEvent.click(cb);
    const save = screen.getByRole("button", { name: /guard|save/i });
    fireEvent.click(save);
    expect(mutate).toHaveBeenCalled();
  });

  it("mode save button onClick fires mutate when dirty", () => {
    const mutate = vi.fn();
    vi.mocked(useMutation).mockReturnValue({
      mutate,
      isPending: false,
      isError: false,
      error: null,
    } as never);
    renderModal({ group: makeGroup({ is_premium: false }), memberCount: 6 });
    goToModeTab();
    const [cb] = screen.getAllByRole("checkbox");
    fireEvent.click(cb);
    const save = screen.getByRole("button", { name: /guard|save/i });
    fireEvent.click(save);
    expect(mutate).toHaveBeenCalled();
  });
});
