import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { GroupDialog } from "@/components/groups/GroupDialog";

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

vi.mock("@clerk/nextjs", () => ({
  useAuth: vi
    .fn()
    .mockReturnValue({ getToken: vi.fn().mockResolvedValue("tok") }),
}));

vi.mock("@/lib/api", () => ({
  api: {
    createGroup: vi.fn(),
    joinGroup: vi.fn(),
    checkGroupName: vi.fn(),
  },
}));

import { api } from "@/lib/api";
import { act } from "@testing-library/react";

function render(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>{ui}</I18nProvider>
    </QueryClientProvider>,
  );
}

describe("GroupDialog", () => {
  beforeEach(() => {
    vi.mocked(api.createGroup).mockReset();
    vi.mocked(api.joinGroup).mockReset();
    vi.mocked(api.checkGroupName).mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders nothing when open=false", () => {
    const { container } = render(
      <GroupDialog open={false} onClose={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders create tab by default", () => {
    render(<GroupDialog open={true} onClose={vi.fn()} />);
    // h2 heading shows the tab title
    expect(
      screen.getByRole("heading", { name: "Crear kiniela" }),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/Ej\. Amigos/)).toBeInTheDocument();
  });

  it('renders join tab when defaultTab="join"', () => {
    render(<GroupDialog open={true} defaultTab="join" onClose={vi.fn()} />);
    expect(
      screen.getByRole("heading", { name: "Unirse a kiniela" }),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText("XXXXXXXXXX")).toBeInTheDocument();
  });

  it("switches between create and join tabs", () => {
    render(<GroupDialog open={true} onClose={vi.fn()} />);
    expect(screen.getByPlaceholderText(/Ej\. Amigos/)).toBeInTheDocument();

    // Tab buttons are labelled "Crear" and "Unirse" (short labels)
    fireEvent.click(screen.getByRole("button", { name: "Unirse" }));
    expect(screen.getByPlaceholderText("XXXXXXXXXX")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Crear" }));
    expect(screen.getByPlaceholderText(/Ej\. Amigos/)).toBeInTheDocument();
  });

  it("calls onClose when backdrop is clicked", () => {
    const onClose = vi.fn();
    const { container } = render(<GroupDialog open={true} onClose={onClose} />);
    const backdrop = container.querySelector(".absolute.inset-0");
    fireEvent.click(backdrop!);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onClose when X button is clicked", () => {
    const onClose = vi.fn();
    render(<GroupDialog open={true} onClose={onClose} />);
    fireEvent.click(screen.getByRole("button", { name: "" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("create submit button disabled when name is empty", () => {
    render(<GroupDialog open={true} onClose={vi.fn()} />);
    const submit = screen.getByRole("button", { name: /Crear kiniela/ });
    expect(submit).toBeDisabled();
  });

  it("create submit button enabled when name has text", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: true });

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Mi grupo" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(
      screen.getByRole("button", { name: /Crear kiniela/ }),
    ).not.toBeDisabled();
  });

  it("shows success state with invite code after create", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: true });
    vi.mocked(api.createGroup).mockResolvedValueOnce({
      id: 1,
      name: "Mi grupo",
      invite_code: "ABC123XYZW",
      owner_user_id: 10,
      status: "open",
      entry_fee: 0,
      currency: "GTQ",
      created_at: "",
      updated_at: "",
    } as never);

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Mi grupo" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByRole("button", { name: /Crear kiniela/ }));

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(screen.getByText("¡Kiniela creada!")).toBeInTheDocument();
    expect(screen.getByText("ABC123XYZW")).toBeInTheDocument();
  });

  it("shows error message when create fails", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: true });
    vi.mocked(api.createGroup).mockRejectedValueOnce(
      new Error("network error"),
    );

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Mi grupo" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByRole("button", { name: /Crear kiniela/ }));

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(
      screen.getByText("No se pudo crear la kiniela."),
    ).toBeInTheDocument();
  });

  it("calls onClose when Done button is clicked after create", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: true });
    const onClose = vi.fn();
    vi.mocked(api.createGroup).mockResolvedValueOnce({
      id: 1,
      name: "Mi grupo",
      invite_code: "ABC123XYZW",
      owner_user_id: 10,
      status: "open",
      entry_fee: 0,
      currency: "GTQ",
      created_at: "",
      updated_at: "",
    } as never);

    render(<GroupDialog open={true} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Mi grupo" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByRole("button", { name: /Crear kiniela/ }));

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(screen.getByText("¡Kiniela creada!")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Listo" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("join submit button disabled when invite code is empty", () => {
    const { container } = render(
      <GroupDialog open={true} defaultTab="join" onClose={vi.fn()} />,
    );
    // The form's submit button (type=submit) is the actual join action, not the tab button
    const submitBtn = container.querySelector<HTMLButtonElement>(
      'form button[type="submit"]',
    )!;
    expect(submitBtn).toBeDisabled();
  });

  it("join submit button enabled when invite code is typed", () => {
    const { container } = render(
      <GroupDialog open={true} defaultTab="join" onClose={vi.fn()} />,
    );
    fireEvent.change(screen.getByPlaceholderText("XXXXXXXXXX"), {
      target: { value: "ABC123" },
    });
    const submitBtn = container.querySelector<HTMLButtonElement>(
      'form button[type="submit"]',
    )!;
    expect(submitBtn).not.toBeDisabled();
  });

  it("uppercases invite code input", () => {
    render(<GroupDialog open={true} defaultTab="join" onClose={vi.fn()} />);
    const input = screen.getByPlaceholderText("XXXXXXXXXX");
    fireEvent.change(input, { target: { value: "abc123" } });
    expect((input as HTMLInputElement).value).toBe("ABC123");
  });

  it("calls onClose after join succeeds", async () => {
    const onClose = vi.fn();
    vi.mocked(api.joinGroup).mockResolvedValueOnce(undefined as never);

    const { container } = render(
      <GroupDialog open={true} defaultTab="join" onClose={onClose} />,
    );
    fireEvent.change(screen.getByPlaceholderText("XXXXXXXXXX"), {
      target: { value: "ABC123" },
    });
    const submitBtn = container.querySelector<HTMLButtonElement>(
      'form button[type="submit"]',
    )!;
    fireEvent.click(submitBtn);

    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  });

  it("shows error message when join fails", async () => {
    vi.mocked(api.joinGroup).mockRejectedValueOnce(new Error("invalid code"));

    const { container } = render(
      <GroupDialog open={true} defaultTab="join" onClose={vi.fn()} />,
    );
    fireEvent.change(screen.getByPlaceholderText("XXXXXXXXXX"), {
      target: { value: "BADCODE" },
    });
    const submitBtn = container.querySelector<HTMLButtonElement>(
      'form button[type="submit"]',
    )!;
    fireEvent.click(submitBtn);

    expect(
      await screen.findByText("Código inválido o grupo no disponible."),
    ).toBeInTheDocument();
  });

  it("create and join forms both render within the dialog", () => {
    const { container } = render(<GroupDialog open={true} onClose={vi.fn()} />);
    expect(container.querySelector(".fixed.inset-0")).toBeInTheDocument();
    // Tab buttons exist for switching
    const tabBtns = within(
      container.querySelector(".mb-5.flex.gap-1")! as HTMLElement,
    ).getAllByRole("button");
    expect(tabBtns).toHaveLength(2);
  });

  // ── Name availability (debounced) ──────────────────────────────────────────

  it("shows available feedback after debounce when name is free", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: true });

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Nuevo Grupo" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(screen.getByText("Nombre disponible")).toBeInTheDocument();
  });

  it("shows taken feedback after debounce when name is taken", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: false });

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Nombre Repetido" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(
      screen.getByText(/Ya existe un grupo con ese nombre/),
    ).toBeInTheDocument();
  });

  it("disables submit when name state is taken", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: false });

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Ocupado" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(
      screen.getByRole("button", { name: /Crear kiniela/ }),
    ).toBeDisabled();
  });

  it("skips availability check when name is shorter than 2 chars", async () => {
    vi.useFakeTimers();

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "A" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(api.checkGroupName).not.toHaveBeenCalled();
  });

  // ── createErrorKey ──────────────────────────────────────────────────────────

  it("shows nameTaken error when create fails with 409 name conflict", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: true });
    const conflictError = Object.assign(
      new Error("a group with this name already exists"),
      { status: 409 },
    );
    vi.mocked(api.createGroup).mockRejectedValueOnce(conflictError);

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Nombre Repetido" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByRole("button", { name: /Crear kiniela/ }));

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(
      screen.getByText(/Ya existe un grupo con ese nombre/),
    ).toBeInTheDocument();
  });

  it("shows generic create error for non-name 409 conflicts", async () => {
    vi.useFakeTimers();
    vi.mocked(api.checkGroupName).mockResolvedValueOnce({ available: true });
    const conflictError = Object.assign(new Error("other conflict"), {
      status: 409,
    });
    vi.mocked(api.createGroup).mockRejectedValueOnce(conflictError);

    render(<GroupDialog open={true} onClose={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText(/Ej\. Amigos/), {
      target: { value: "Mi grupo" },
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    fireEvent.click(screen.getByRole("button", { name: /Crear kiniela/ }));

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(
      screen.getByText("No se pudo crear la kiniela."),
    ).toBeInTheDocument();
  });

  // ── Double-click prevention ─────────────────────────────────────────────────

  it("does not call joinGroup a second time when form is re-submitted while pending", async () => {
    let resolveFn!: () => void;
    vi.mocked(api.joinGroup).mockReturnValueOnce(
      new Promise<never>((res) => {
        resolveFn = res as () => void;
      }),
    );

    const { container } = render(
      <GroupDialog open={true} defaultTab="join" onClose={vi.fn()} />,
    );
    fireEvent.change(screen.getByPlaceholderText("XXXXXXXXXX"), {
      target: { value: "ABC123" },
    });

    const form = container.querySelector("form")!;
    fireEvent.submit(form);

    await waitFor(() => {
      expect(
        container.querySelector<HTMLButtonElement>('form button[type="submit"]'),
      ).toBeDisabled();
    });

    // Try to submit again while still pending
    fireEvent.submit(form);

    resolveFn();
    expect(api.joinGroup).toHaveBeenCalledTimes(1);
  });
});
