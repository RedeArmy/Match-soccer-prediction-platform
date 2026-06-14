import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { I18nProvider } from "@/lib/i18n";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi
    .fn()
    .mockReturnValue({ getToken: vi.fn().mockResolvedValue("tok") }),
  useUser: vi.fn().mockReturnValue({ user: { firstName: "Jugador" } }),
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: vi.fn(),
  useMutation: vi.fn().mockReturnValue({ mutate: vi.fn(), isPending: false }),
  useQueryClient: vi.fn().mockReturnValue({ invalidateQueries: vi.fn() }),
}));

vi.mock("@/hooks/useSSE", () => ({ useSSE: vi.fn() }));

vi.mock("@/hooks/useKYCEffectiveStatus", () => ({
  useKYCEffectiveStatus: vi.fn(),
}));

vi.mock("@/hooks/useExchangeRate", () => ({
  useExchangeRate: vi.fn().mockReturnValue({ data: undefined }),
}));

// Shallow-mock heavy sub-components to avoid their own dependency chains
vi.mock("@/components/balance/BalanceCard", () => ({
  BalanceCard: () => <div data-testid="balance-card" />,
}));
vi.mock("@/components/groups/GroupDialog", () => ({
  GroupDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="group-dialog" /> : null,
}));
vi.mock("@/components/groups/PendingApprovalsDialog", () => ({
  PendingApprovalsDialog: () => null,
}));
vi.mock("@/components/predictions/PredictionPanel", () => ({
  PredictionPanel: () => <div data-testid="prediction-panel" />,
}));

vi.mock("next/link", () => ({
  default: ({
    children,
    href,
    className,
  }: {
    children: React.ReactNode;
    href: string;
    className?: string;
  }) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
}));

vi.mock("next/navigation", () => ({
  usePathname: vi.fn().mockReturnValue("/dashboard"),
}));

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useQuery } from "@tanstack/react-query";
import { useKYCEffectiveStatus } from "@/hooks/useKYCEffectiveStatus";
import DashboardContent from "@/app/(auth)/dashboard/DashboardContent";
import type { GroupResponse, LedgerEntry } from "@/lib/api-types";

// ── Fixtures ──────────────────────────────────────────────────────────────────

const mockGroup: GroupResponse = {
  id: 1,
  name: "Mi Kiniela",
  group_status: "open",
  membership_status: "active",
  role: "owner",
  invite_code: "abc",
  entry_fee: 0,
  currency: "GTQ",
  is_premium: false,
  mode_general: true,
  mode_round: false,
  active_member_count: 5,
};

const mockLedgerEntry: LedgerEntry = {
  id: 1,
  delta_cents: 5000,
  kind: "webhook_recurrente",
  balance_after: 5000,
  ref_id: null,
  ref_type: null,
  created_at: "2026-06-01T10:00:00Z",
};

function kycApproved() {
  vi.mocked(useKYCEffectiveStatus).mockReturnValue({
    kyc: { status: "approved" } as never,
    effectiveStatus: "approved",
    isLoading: false,
    isDocsLoading: false,
    hasPendingReview: false,
    docs: [],
  });
}

function kycPending() {
  vi.mocked(useKYCEffectiveStatus).mockReturnValue({
    kyc: { status: "pending" } as never,
    effectiveStatus: "pending",
    isLoading: false,
    isDocsLoading: false,
    hasPendingReview: false,
    docs: [],
  });
}

function setQueries({
  groups,
  ledger,
  groupsLoading = false,
  ledgerLoading = false,
}: {
  groups?: GroupResponse[];
  ledger?: LedgerEntry[];
  groupsLoading?: boolean;
  ledgerLoading?: boolean;
}) {
  vi.mocked(useQuery).mockImplementation(
    ({ queryKey }: { queryKey: readonly unknown[] }) => {
      if (queryKey[0] === "my-groups")
        return { data: groups, isLoading: groupsLoading } as never;
      if (queryKey[0] === "ledger-preview")
        return { data: ledger, isLoading: ledgerLoading } as never;
      return { data: undefined, isLoading: false } as never;
    },
  );
}

function renderDashboard() {
  return render(
    <I18nProvider>
      <DashboardContent />
    </I18nProvider>,
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("DashboardContent – greeting", () => {
  beforeEach(() => {
    kycPending();
    setQueries({ groups: [], ledger: [] });
  });

  it("renders the user first name in the heading", () => {
    renderDashboard();
    expect(screen.getByText(/JUGADOR/)).toBeInTheDocument();
  });

  it("renders BalanceCard", () => {
    renderDashboard();
    expect(screen.getByTestId("balance-card")).toBeInTheDocument();
  });

  it("renders PredictionPanel", () => {
    renderDashboard();
    expect(screen.getByTestId("prediction-panel")).toBeInTheDocument();
  });
});

describe("DashboardContent – KYC status card", () => {
  beforeEach(() => setQueries({ groups: [], ledger: [] }));

  it("shows verified card when KYC is approved", () => {
    kycApproved();
    renderDashboard();
    expect(screen.getByText("Identidad verificada")).toBeInTheDocument();
  });

  it("shows KYC CTA card when not approved", () => {
    kycPending();
    renderDashboard();
    expect(screen.getByText("Verifica tu identidad")).toBeInTheDocument();
  });

  it("shows status badge when KYC has a status", () => {
    kycPending();
    renderDashboard();
    // pendingBadge is 'Pendiente'; StatusBadge for 'pending' also renders the same word
    expect(screen.getAllByText(/Pendiente/i).length).toBeGreaterThan(0);
  });
});

describe("DashboardContent – groups section", () => {
  beforeEach(() => {
    kycPending();
    setQueries({ ledger: [] });
  });

  it("shows loading skeleton while groups are loading", () => {
    setQueries({ ledger: [], groupsLoading: true });
    const { container } = renderDashboard();
    // LoadingState renders rows with animate-pulse
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });

  it("shows empty state when groups list is empty", () => {
    setQueries({ groups: [], ledger: [] });
    renderDashboard();
    expect(screen.getByText("Aun no tienes kinielas")).toBeInTheDocument();
  });

  it("renders a card per group when groups exist", () => {
    setQueries({
      groups: [mockGroup, { ...mockGroup, id: 2, name: "Segunda Kiniela" }],
      ledger: [],
    });
    renderDashboard();
    expect(screen.getByText("Mi Kiniela")).toBeInTheDocument();
    expect(screen.getByText("Segunda Kiniela")).toBeInTheDocument();
  });

  it("opens GroupDialog when create button is clicked", () => {
    setQueries({ groups: [], ledger: [] });
    renderDashboard();
    // The header "Crear" button (not the EmptyState action which is also "Crear")
    const createBtns = screen.getAllByRole("button", { name: "Crear" });
    fireEvent.click(createBtns[0]);
    expect(screen.getByTestId("group-dialog")).toBeInTheDocument();
  });

  it("opens GroupDialog when join button is clicked", () => {
    setQueries({ groups: [], ledger: [] });
    renderDashboard();
    const joinBtn = screen.getByRole("button", { name: "Unirse" });
    fireEvent.click(joinBtn);
    expect(screen.getByTestId("group-dialog")).toBeInTheDocument();
  });
});

describe("DashboardContent – transactions section", () => {
  beforeEach(() => {
    kycPending();
    setQueries({ groups: [] });
  });

  it("shows loading skeleton while ledger is loading", () => {
    setQueries({ groups: [], ledgerLoading: true });
    const { container } = renderDashboard();
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });

  it("shows no-transactions message when ledger is empty", () => {
    setQueries({ groups: [], ledger: [] });
    renderDashboard();
    expect(screen.getByText("Sin transacciones aun")).toBeInTheDocument();
  });

  it("renders transaction cards for visible ledger entries", () => {
    setQueries({ groups: [], ledger: [mockLedgerEntry] });
    renderDashboard();
    // TxCard renders a credit amount with + sign
    expect(screen.getByText(/\+/)).toBeInTheDocument();
  });

  it("renders debit entry with correct styling cue", () => {
    setQueries({
      groups: [],
      ledger: [{ ...mockLedgerEntry, delta_cents: -2000, kind: "entry_fee" }],
    });
    renderDashboard();
    // Negative delta — no + prefix, amount is formatted
    expect(screen.queryByText(/\+/)).toBeNull();
  });

  it("renders prize entry", () => {
    setQueries({
      groups: [],
      ledger: [{ ...mockLedgerEntry, kind: "prize", delta_cents: 10000 }],
    });
    renderDashboard();
    expect(screen.getByText(/\+/)).toBeInTheDocument();
  });

  it("renders withdrawal_release as refund", () => {
    setQueries({
      groups: [],
      ledger: [
        { ...mockLedgerEntry, kind: "withdrawal_release", delta_cents: 3000 },
      ],
    });
    renderDashboard();
    expect(screen.getByText(/\+/)).toBeInTheDocument();
  });
});

describe("DashboardContent – GroupCard pending membership", () => {
  beforeEach(() => {
    kycPending();
    setQueries({ ledger: [] });
  });

  it("shows pending badge for a pending-membership group", () => {
    const pendingGroup: GroupResponse = {
      ...mockGroup,
      membership_status: "pending",
    };
    setQueries({ groups: [pendingGroup], ledger: [] });
    renderDashboard();
    // GroupCard renders span with pendingBadge text: "Pendiente"
    expect(screen.getAllByText("Pendiente").length).toBeGreaterThan(0);
  });

  it("renders view-all link for an active group", () => {
    setQueries({ groups: [mockGroup], ledger: [] });
    renderDashboard();
    const links = screen.getAllByRole("link");
    const tournamentLink = links.find((l) =>
      l.getAttribute("href")?.startsWith("/tournaments/"),
    );
    expect(tournamentLink).toBeDefined();
  });
});
